package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
)

// Exercise the production stream pool, control authentication and forwarded
// TCP/UDP framing, rather than only the Wat Session API. Raw sockets are opt-in.
func TestWatICMPForwardedTraffic(t *testing.T) {
	if os.Getenv("WATICMP_LIVE") != "1" {
		t.Skip("requires isolated Linux CAP_NET_RAW; set WATICMP_LIVE=1")
	}
	for _, direction := range []string{"normal", "reverse"} {
		t.Run(direction, func(t *testing.T) {
			for _, udp := range []bool{false, true} {
				t.Run(fmt.Sprintf("udp=%v", udp), func(t *testing.T) {
					backend := ""
					if udp {
						backend = startUDPEcho(t)
					} else {
						backend = startEchoBackend(t).addr
					}
					entry, marker := freePort(t), freePort(t)
					token := "native-e2e-secret-0123456789abcdef0123"
					srv := baseServerConfig("waticmp", marker, entry, backend, token)
					cli := baseClientConfig("waticmp", fmt.Sprintf("127.0.0.1:%d", marker), token, nil)
					settings := config.WatICMPConfig{Enabled: true, PeerIP: "127.0.0.1",
						TunnelID: fmt.Sprintf("production-e2e-%s-%d", direction, entry), Direction: direction}
					srv.WatICMP, cli.WatICMP = settings, settings
					srv.AcceptUDP = &udp
					tun := runPair(t, srv, cli, entry, marker)
					payload := randomPayload(t, 8000)
					if !udp {
						if err := tun.waitReady(tunnelReadyTimeout); err != nil {
							t.Fatalf("native TCP not ready: %v", err)
						}
						if err := tun.roundTrip(payload); err != nil {
							t.Fatal(err)
						}
						return
					}
					deadline := time.Now().Add(tunnelReadyTimeout)
					var last error
					for time.Now().Before(deadline) {
						last = udpRoundTrip(tun.Entry, payload)
						if last == nil {
							return
						}
						time.Sleep(250 * time.Millisecond)
					}
					t.Fatalf("native UDP not ready: %v", last)
				})
			}
		})
	}
}
