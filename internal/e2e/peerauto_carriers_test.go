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

func TestPeerAutoSingleApprovedCarrierBootstrap(t *testing.T) {
	for _, carrier := range []config.TransportType{config.STEALTH, config.QUIC, config.WSS, config.WSSMUX} {
		t.Run(string(carrier), func(t *testing.T) {
			backend := startEchoBackend(t)
			entry, endpoint := freePort(t), freePort(t)
			maps := []string{fmt.Sprintf("127.0.0.1:%d=%s", entry, backend.addr)}
			a := config.AutoConfig{Enabled: true, TunnelID: "single-carrier-tunnel", IranID: "single-carrier-iran", KharejID: "single-carrier-kharej", Ports: maps, Candidates: []config.AutoCandidate{{ID: "sole", Transport: carrier, Endpoint: fmt.Sprintf("127.0.0.1:%d", endpoint)}}}
			token := "0123456789abcdef0123456789abcdef"
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			m, err := peerauto.New(ctx, &config.Config{Auto: a, Server: config.ServerConfig{BindAddr: a.Candidates[0].Endpoint, Transport: carrier, Token: token, Ports: maps}}, filepath.Join(t.TempDir(), "iran.json"))
			if err != nil {
				t.Fatal(err)
			}
			p, err := peerauto.New(ctx, &config.Config{Auto: a, Client: config.ClientConfig{RemoteAddr: a.Candidates[0].Endpoint, Transport: carrier, Token: token}}, filepath.Join(t.TempDir(), "kharej.json"))
			if err != nil {
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
						t.Error("carrier workers did not join")
					}
				}
			})
			until := time.Now().Add(30 * time.Second)
			for m.Snapshot().Active != "sole" && time.Now().Before(until) {
				time.Sleep(100 * time.Millisecond)
			}
			if m.Snapshot().Active != "sole" || p.Snapshot().Generation != m.Snapshot().Generation {
				t.Fatalf("no shared approved bootstrap: %+v peer=%+v", m.Snapshot(), p.Snapshot())
			}
			tun := &tunnel{Entry: fmt.Sprintf("127.0.0.1:%d", entry)}
			if err = tun.roundTrip(randomPayload(t, 32768)); err != nil {
				t.Fatal(err)
			}
			gen := m.Snapshot().Generation
			if err = m.Action(ctx, peerauto.Action{Operation: "test-now", Candidate: "sole"}); err != nil {
				t.Fatal(err)
			}
			if m.Snapshot().Generation != gen || p.Snapshot().Generation != gen {
				t.Fatal("probe-only operation rewrote production")
			}
			if err = tun.roundTrip(randomPayload(t, 32768)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
