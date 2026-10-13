//go:build linux

package enginectl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestControlSocketRejectsUnsafeOwnershipLock(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink", "public-file"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "engine.sock")
			var err error
			switch kind {
			case "fifo":
				err = unix.Mkfifo(path+".lock", 0600)
			case "symlink":
				target := filepath.Join(t.TempDir(), "private-target")
				if err = os.WriteFile(target, nil, 0600); err == nil {
					err = os.Symlink(target, path+".lock")
				}
			case "public-file":
				err = os.WriteFile(path+".lock", nil, 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			if release, err := claimSocket(path); err == nil {
				release()
				t.Fatal("unsafe ownership lock was admitted")
			}
			if time.Since(start) > time.Second {
				t.Fatal("ownership refusal blocked shutdown")
			}
		})
	}
}

func TestDuplicateEngineCannotReplaceLiveControlSocket(t *testing.T) {
	name := serveTest(t, &fakeEngine{status: Status{Name: "original"}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Serve(ctx, name, &fakeEngine{status: Status{Name: "duplicate"}}); err == nil {
		t.Fatal("duplicate socket owner admitted")
	}
	var status Status
	if err := Ask(name, OpStatus, &status); err != nil || status.Name != "original" {
		t.Fatal("original engine's socket was replaced")
	}
}

type cancellableAuto struct {
	fakeEngine
	started chan struct{}
}

func (c *cancellableAuto) AutoStatus() (any, error) {
	return map[string]string{"state": "RECOVERING"}, nil
}
func (c *cancellableAuto) AutoAction(ctx context.Context, _ json.RawMessage) error {
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}
func TestControlShutdownCancelsAndJoinsAutoAction(t *testing.T) {
	previous := Dir
	Dir = t.TempDir()
	defer func() { Dir = previous }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := &cancellableAuto{started: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, "auto-test", engine) }()
	until := time.Now().Add(3 * time.Second)
	for Ask("auto-test", OpPing, nil) != nil {
		if time.Now().After(until) {
			t.Fatal("socket not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	request := make(chan error, 1)
	go func() { request <- AskBody("auto-test", OpAutoAction, map[string]string{"operation": "test-now"}, nil) }()
	select {
	case <-engine.started:
	case <-time.After(3 * time.Second):
		t.Fatal("action never started")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("action outlived server")
	}
	select {
	case <-request:
	case <-time.After(3 * time.Second):
		t.Fatal("caller outlived socket shutdown")
	}
}
