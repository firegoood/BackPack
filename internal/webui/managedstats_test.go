package webui

import (
	"reflect"
	"testing"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/manage"
	"github.com/backpack/backpack/internal/metrics"
)

func TestPanelHTTPSCardsAndProbesUseThePublicEndpoint(t *testing.T) {
	for _, transport := range []string{"naive", "xhttp", "reality"} {
		for _, role := range []string{"server", "client"} {
			t.Run(transport+"/"+role, func(t *testing.T) {
				engine := manage.Tunnel{Name: "managed", Role: role, Transport: "tcp", Addr: "127.0.0.1:3080", Ports: []string{"2087"}}
				public := "edge.example:19443"
				var cfg config.Config
				if transport == "naive" {
					cfg.Server.Naive = config.NaiveServerConfig{Binary: "/server/sing-box", Listen: public}
					cfg.Client.Naive = config.NaiveClientConfig{Binary: "/client/naive", Server: public}
				} else {
					cfg.Server.Xray = config.XrayServerConfig{Binary: "/server/xray", Mode: transport, Listen: public}
					cfg.Client.Xray = config.XrayClientConfig{Binary: "/client/xray", Mode: transport, Server: public}
				}
				shown := publicPanelTunnel(engine, cfg)
				if shown.Transport != transport || shown.Addr != public || tunnelPortOf(shown.Addr) != "19443" || manage.TunnelCarrier(shown) != transport {
					t.Fatalf("card/probe still describes private TCP: %+v", shown)
				}
				if engine.Addr != "127.0.0.1:3080" || engine.Transport != "tcp" || shown.Role != role || !reflect.DeepEqual(shown.Ports, engine.Ports) {
					t.Fatal("panel summary changed engine endpoints, direction or forwarded ports")
				}
			})
		}
	}
	plain := manage.Tunnel{Role: "server", Transport: "tcp", Addr: "0.0.0.0:443"}
	if !reflect.DeepEqual(publicPanelTunnel(plain, config.Config{}), plain) {
		t.Fatal("ordinary TCP summary changed")
	}
}

func TestPanelHTTPSDoesNotReportItsLocalHelperAsTheRemotePeer(t *testing.T) {
	for _, transport := range []string{"naive", "xhttp", "reality"} {
		for _, peer := range []string{"127.0.0.1:3080", "[::1]:3080"} {
			info := TunnelInfo{Ping: -1}
			observeInbound(&info, manage.Tunnel{Transport: transport, Addr: "0.0.0.0:19443"},
				manage.Health{State: "online"}, nil, metrics.Snapshot{Peer: peer}, nil)
			if info.Ping != -1 || info.PeerLocation != "" || info.State != "online" {
				t.Fatal("local helper became a remote peer or overrode engine health")
			}
		}
	}
}
