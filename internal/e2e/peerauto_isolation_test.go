//go:build linux

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/peerauto"
)

func TestPeerAutoIndependentPairsAndBackendOutage(t *testing.T) {
	type pair struct {
		iran, kharej *peerauto.Manager
		cancel       context.CancelFunc
		tun          *tunnel
		backend      *echoBackend
	}
	pairs := make([]pair, 0, 2)
	for i := range 2 {
		backend := startEchoBackend(t)
		entry, endpoint := freePort(t), freePort(t)
		maps := []string{fmt.Sprintf("127.0.0.1:%d=%s", entry, backend.addr)}
		a := config.AutoConfig{Enabled: true, TunnelID: fmt.Sprintf("isolated-pair-tunnel-%d", i), IranID: fmt.Sprintf("isolated-pair-iran-%d", i), KharejID: fmt.Sprintf("isolated-pair-kharej-%d", i), Ports: maps, Candidates: []config.AutoCandidate{{ID: "noise", Transport: config.STEALTH, Endpoint: fmt.Sprintf("127.0.0.1:%d", endpoint)}}}
		token := fmt.Sprintf("separate-pair-%d-enrollment-key-0123456789", i)
		ctx, cancel := context.WithCancel(t.Context())
		m, err := peerauto.New(ctx, &config.Config{Auto: a, Server: config.ServerConfig{BindAddr: a.Candidates[0].Endpoint, Transport: config.STEALTH, Token: token, Ports: maps}}, filepath.Join(t.TempDir(), "iran.json"))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		p, err := peerauto.New(ctx, &config.Config{Auto: a, Client: config.ClientConfig{RemoteAddr: a.Candidates[0].Endpoint, Transport: config.STEALTH, Token: token}}, filepath.Join(t.TempDir(), "kharej.json"))
		if err != nil {
			cancel()
			m.Close()
			t.Fatal(err)
		}
		done := make(chan error, 2)
		go func() { done <- m.Start() }()
		go func() { done <- p.Start() }()
		t.Cleanup(func() {
			cancel()
			m.Close()
			p.Close()
			for range 2 {
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(10 * time.Second):
					t.Error("pair did not join")
				}
			}
		})
		pairs = append(pairs, pair{m, p, cancel, &tunnel{Entry: fmt.Sprintf("127.0.0.1:%d", entry)}, backend})
	}
	for _, p := range pairs {
		until := time.Now().Add(50 * time.Second)
		for p.iran.Snapshot().Active != "noise" && time.Now().Before(until) {
			time.Sleep(100 * time.Millisecond)
		}
		if p.iran.Snapshot().Active != "noise" || p.iran.Snapshot().Generation != p.kharej.Snapshot().Generation {
			t.Fatal("pair failed to bootstrap")
		}
		if err := p.tun.roundTrip(randomPayload(t, 32768)); err != nil {
			t.Fatal(err)
		}
	}
	first, second := pairs[0], pairs[1]
	gen := second.iran.Snapshot().Generation
	first.cancel()
	first.iran.Close()
	first.kharej.Close()
	if err := second.tun.roundTrip(randomPayload(t, 32768)); err != nil {
		t.Fatal("other pair disrupted:", err)
	}
	second.backend.listener.Close()
	if err := second.tun.roundTrip(randomPayload(t, 128)); err == nil {
		t.Fatal("closed backend reported as passing")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := second.iran.Action(ctx, peerauto.Action{Operation: "test-now", Candidate: "noise"}); err != nil {
		t.Fatal("backend outage corrupted isolated carrier probe:", err)
	}
	if second.iran.Snapshot().Generation != gen || second.iran.Snapshot().Active != "noise" {
		t.Fatal("backend/other-peer failure changed healthy carrier")
	}
}
