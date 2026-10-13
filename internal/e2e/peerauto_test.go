package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/peerauto"
)

func TestPeerCoordinatedNativeSwitchKeepsUserListener(t *testing.T) {
	if os.Getenv("WATICMP_LIVE") != "1" {
		t.Skip("isolated Linux raw socket test")
	}
	testPeerCoordinatedSwitchKeepsUserListener(t, config.WATICMP, true)
}

func TestPeerCoordinatedAuthenticatedAdapters(t *testing.T) {
	for _, transport := range []config.TransportType{config.QUIC, config.WSS, config.WSSMUX} {
		t.Run(string(transport), func(t *testing.T) {
			t.Run("sole-candidate", func(t *testing.T) {
				testPeerCoordinatedSwitchKeepsUserListener(t, transport, false)
			})
			t.Run("handover", func(t *testing.T) {
				testPeerCoordinatedSwitchKeepsUserListener(t, transport, true)
			})
		})
	}
}

func testPeerCoordinatedSwitchKeepsUserListener(t *testing.T, transport config.TransportType, withAlternate bool) {
	t.Helper()
	backend := startEchoBackend(t)
	udpBackend := startUDPEcho(t)
	entry, udpEntry, noise := freePort(t), freePort(t), freePort(t)
	endpoint := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	primary := config.AutoCandidate{ID: "primary", Transport: transport, Endpoint: endpoint}
	if transport == config.WATICMP {
		primary.IranIP, primary.KharejIP, primary.Direction = "127.0.0.1", "127.0.0.1", "reverse"
	}
	maps := []string{fmt.Sprintf("127.0.0.1:%d=%s", entry, backend.addr), fmt.Sprintf("127.0.0.1:%d=%s", udpEntry, udpBackend)}
	a := config.AutoConfig{Enabled: true, TunnelID: "e2e-enrolled-tunnel", IranID: "e2e-enrolled-iran", KharejID: "e2e-enrolled-kharej", Ports: maps, AcceptUDP: true,
		Candidates: []config.AutoCandidate{primary, {ID: "noise", Transport: config.STEALTH, Endpoint: fmt.Sprintf("127.0.0.1:%d", noise)}}}
	if !withAlternate {
		a.Candidates = a.Candidates[:1]
	}
	token := "0123456789abcdef0123456789abcdef"
	srv := &config.Config{Auto: a, Server: config.ServerConfig{Transport: transport, BindAddr: endpoint, Token: token, Ports: maps}}
	cli := &config.Config{Auto: a, Client: config.ClientConfig{Transport: transport, RemoteAddr: endpoint, Token: token}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, err := peerauto.New(ctx, srv, filepath.Join(t.TempDir(), "iran.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := peerauto.New(ctx, cli, filepath.Join(t.TempDir(), "kharej.json"))
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
			case e := <-done:
				if e != nil {
					t.Error(e)
				}
			case <-time.After(10 * time.Second):
				t.Error("coordinator shutdown did not join")
			}
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for m.Snapshot().Active != primary.ID && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if m.Snapshot().Active != primary.ID {
		t.Fatalf("no %s bootstrap: %+v; peer=%+v", transport, m.Snapshot(), p.Snapshot())
	}
	tun := &tunnel{Entry: fmt.Sprintf("127.0.0.1:%d", entry)}
	if err = tun.roundTrip(randomPayload(t, 32768)); err != nil {
		t.Fatal(err)
	}
	if err = udpRoundTrip(fmt.Sprintf("127.0.0.1:%d", udpEntry), randomPayload(t, 8000)); err != nil {
		t.Fatal(err)
	}
	actionCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	// A live production stream stays on its original session during a fresh
	// probe and a successful handover; it must be accounted as a bounded drain.
	held, err := net.DialTimeout("tcp", tun.Entry, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	checkHeld := func() {
		t.Helper()
		_ = held.SetDeadline(time.Now().Add(5 * time.Second))
		payload := randomPayload(t, 4096)
		if _, err := held.Write(payload); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(held, got); err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("active user was interrupted: %v", err)
		}
	}
	checkHeld()
	beforeProbe := m.Snapshot().Generation
	if err = m.Action(actionCtx, peerauto.Action{Operation: "test-now", Candidate: primary.ID}); err != nil {
		t.Fatal(err)
	}
	checkHeld()
	if m.Snapshot().Generation != beforeProbe {
		t.Fatal("probe changed production generation")
	}
	if !withAlternate {
		if len(m.Snapshot().Candidates) != 1 || p.Snapshot().Active != primary.ID || m.Snapshot().Generation != p.Snapshot().Generation {
			t.Fatalf("sole %s candidate did not agree: %+v %+v", transport, m.Snapshot(), p.Snapshot())
		}
		t.Logf("sole %s bootstrap and isolated probe passed at generation %d", transport, m.Snapshot().Generation)
		return
	}
	if err = m.Action(actionCtx, peerauto.Action{Operation: "switch", Candidate: "noise"}); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Active != "noise" || p.Snapshot().Active != "noise" || m.Snapshot().Generation != p.Snapshot().Generation {
		t.Fatalf("peers disagree: %+v %+v", m.Snapshot(), p.Snapshot())
	}
	if err = tun.roundTrip(randomPayload(t, 32768)); err != nil {
		t.Fatal(err)
	}
	if err = udpRoundTrip(fmt.Sprintf("127.0.0.1:%d", udpEntry), randomPayload(t, 8000)); err != nil {
		t.Fatal(err)
	}
	checkHeld()
	held.Close()
	deadline = time.Now().Add(5 * time.Second)
	for m.Snapshot().Flows.Drained == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if m.Snapshot().Flows.Drained == 0 {
		t.Fatal("successful old stream drain was not counted")
	}
	t.Logf("real %s-to-Noise commit: %+v", transport, m.Snapshot())
}
