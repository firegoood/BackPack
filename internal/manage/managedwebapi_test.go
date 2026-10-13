package manage

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/backpack/backpack/internal/tunnel/l3"
)

func panelHTTPSFixture(t *testing.T, transport, role string) NewTunnel {
	t.Helper()
	binary, cert, key := managedWizardFixture(t)
	m := &ManagedHTTPS{Binary: binary, InternalPort: "3080", ServerName: "localhost"}
	if transport == "naive" {
		m.Username, m.Password = "panel-user", "panel-password"
	} else {
		m.UUID = "00112233-4455-4677-8899-aabbccddeeff"
	}
	if transport == "reality" {
		private, public, err := managedRealityKey("")
		if err != nil {
			t.Fatal(err)
		}
		m.ShortID = "0123456789abcdef"
		if role == "server" {
			m.Target, m.PrivateKey = "cover.example:443", private
		} else {
			m.PublicKey = public
		}
	} else {
		if transport == "xhttp" {
			m.Path, m.Host = "/panel-path", "localhost"
		}
		if role == "server" {
			m.Certificate, m.Key = cert, key
		} else {
			m.CAFile = cert
		}
	}
	return NewTunnel{Role: role, Transport: transport, Name: "panel-" + transport + "-" + role,
		Token: "panel-token", TunnelPort: "19443", ServerAddr: "127.0.0.1", Ports: "2087", Preset: PresetTurbo, Managed: m}
}

func usePanelRealityProbe(t *testing.T, probe realitySetupProbe) {
	t.Helper()
	old := webRealityProbe
	webRealityProbe = probe
	t.Cleanup(func() { webRealityProbe = old })
}

func TestPanelHTTPSCreatesBothRolesAndPublicSettings(t *testing.T) {
	probes := 0
	usePanelRealityProbe(t, func(ctx context.Context, target, sni, binary string) error {
		probes++
		if _, ok := ctx.Deadline(); !ok || target != "cover.example:443" || sni != "localhost" || binary == "" {
			t.Fatal("REALITY did not check the chosen cover and helper with a deadline")
		}
		return nil
	})
	for _, tr := range []string{"naive", "xhttp", "reality"} {
		for _, role := range []string{"server", "client"} {
			t.Run(tr+"/"+role, func(t *testing.T) {
				n := panelHTTPSFixture(t, tr, role)
				s, err := specFromNew(n)
				if err != nil {
					t.Fatal(err)
				}
				if err := validateManagedSpec(s); err != nil {
					t.Fatalf("CLI/engine validator refused the panel config: %v", err)
				}
				if selectedTransport(s) != tr || s.Transport != "tcp" || s.AcceptUDP {
					t.Fatal("managed transport must wrap reverse TCP without UDP")
				}
				inner := s.BindAddr
				if role == "client" {
					inner = s.RemoteAddr
				}
				if inner != "127.0.0.1:3080" || addrPort(managedEndpoint(s)) != "19443" {
					t.Fatal("public HTTPS endpoint and private reverse listener got mixed up")
				}
				shown := tunnelSettingsFrom(s)
				if shown.Transport != tr || shown.TunnelPort != "19443" || shown.Managed.InternalPort != "3080" {
					t.Fatalf("edit opens on wrong endpoints: %+v", shown)
				}
				if shown.BindHost == "127.0.0.1" || role == "client" && shown.ServerHost != "127.0.0.1" {
					t.Fatal("edit opened on the private engine address")
				}
				raw, err := json.Marshal(shown)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "panel-password") || strings.Contains(string(raw), `"privateKey"`) {
					t.Fatal("settings exposed helper secrets")
				}
				var edit TunnelEdit
				if err := json.Unmarshal([]byte(`{"managed":{"password":"","privateKey":""},"tunnelPort":"20443"}`), &edit); err != nil {
					t.Fatal(err)
				}
				changed, ok, err := editedReverseSpec(s, edit)
				if err != nil || !ok {
					t.Fatalf("edit: %v, changed=%v", err, ok)
				}
				if addrPort(managedEndpoint(changed)) != "20443" || managedHTTPSOf(changed).InternalPort != "3080" {
					t.Fatal("editing public port changed the private port")
				}
				before, after := managedHTTPSOf(s), managedHTTPSOf(changed)
				if before.Password != after.Password || before.PrivateKey != after.PrivateKey || before.UUID != after.UUID {
					t.Fatal("editing a port cleared or regenerated an identity")
				}
				unchanged, ok, err := editedReverseSpec(s, TunnelEdit{})
				if err != nil || ok || !reflect.DeepEqual(unchanged, s) {
					t.Fatalf("empty edit changed the config: %v, changed=%v", err, ok)
				}
			})
		}
	}
	if probes != 3 {
		t.Fatalf("REALITY server create/edit must check the cover; got %d probes", probes)
	}
}

func TestPanelHTTPSServerGeneratesMatchingPeerIdentity(t *testing.T) {
	usePanelRealityProbe(t, func(context.Context, string, string, string) error { return nil })
	for _, tr := range []string{"naive", "xhttp", "reality"} {
		t.Run(tr, func(t *testing.T) {
			n := panelHTTPSFixture(t, tr, "server")
			n.Managed.Password, n.Managed.UUID, n.Managed.PrivateKey, n.Managed.ShortID = "", "", "", ""
			if tr == "xhttp" {
				n.Managed.Path = ""
			}
			s, err := specFromNew(n)
			if err != nil {
				t.Fatal(err)
			}
			m := managedHTTPSOf(s)
			if tr == "naive" && m.Password == "" || tr != "naive" && m.UUID == "" || tr == "xhttp" && m.Path == "" || tr == "reality" && (m.PrivateKey == "" || m.PublicKey == "" || m.ShortID == "") {
				t.Fatal("server did not generate its identity")
			}
			raw, err := shareLinkOf(s.Name, "localhost", decode(t, s.Render()))
			if err != nil {
				t.Fatal(err)
			}
			link, err := DecodeShareLink(raw)
			if err != nil {
				t.Fatal(err)
			}
			if link.Tr != tr || link.Port != "19443" || link.InnerPort != "3080" || link.Tok != s.Token {
				t.Fatal("peer setup link lost the transport or either port")
			}
			if tr == "naive" && link.HelperPassword != m.Password || tr == "xhttp" && (link.HelperUUID != m.UUID || link.HelperPath != m.Path) || tr == "reality" && link.HelperPublicKey != m.PublicKey {
				t.Fatal("peer setup link lost the matching identity")
			}
			if strings.Contains(link.HelperCA, "PRIVATE KEY") || tr == "reality" && link.HelperPublicKey == m.PrivateKey {
				t.Fatal("peer setup link exposed a server private key")
			}
			peer := MirrorForPeer(link).ToNewTunnel()
			if peer.Managed == nil || peer.Managed.InternalPort != "3080" || peer.Transport != tr {
				t.Fatal("managed node form lost the HTTPS settings")
			}
			// A node gets JSON, with no browser to translate helper settings.
			serialized, err := json.Marshal(peer)
			if err != nil {
				t.Fatal(err)
			}
			var received NewTunnel
			if err := json.Unmarshal(serialized, &received); err != nil {
				t.Fatal(err)
			}
			received.Managed.Binary = n.Managed.Binary // fixture for the peer's installed helper
			client, err := specFromNew(received)
			if err != nil {
				t.Fatalf("peer/node form refused: %v", err)
			}
			if client.Role != "client" || selectedTransport(client) != tr || managedEndpoint(client) != "localhost:19443" {
				t.Fatal("managed node did not build the matching client endpoint")
			}
			if tr != "reality" {
				trust, err := os.ReadFile(managedHTTPSOf(client).CAFile)
				if err != nil || string(trust) != link.HelperCA {
					t.Fatal("managed node did not preserve public certificate trust")
				}
			}
		})
	}
}

func TestPanelHTTPSRejectsUnsafeOrInvalidInputs(t *testing.T) {
	for label, change := range map[string]func(*NewTunnel){
		"same private/public port":  func(n *NewTunnel) { n.Managed.InternalPort = n.TunnelPort },
		"invalid private port":      func(n *NewTunnel) { n.Managed.InternalPort = "0" },
		"missing helper":            func(n *NewTunnel) { n.Managed.Binary = "/does/not/exist" },
		"invalid UUID":              func(n *NewTunnel) { n.Managed.UUID = "invalid" },
		"invalid SNI":               func(n *NewTunnel) { n.Managed.ServerName = "127.0.0.1" },
		"root XHTTP path":           func(n *NewTunnel) { n.Managed.Path = "/" },
		"only one certificate file": func(n *NewTunnel) { n.Managed.Key = "" },
		"UDP forwarding":            func(n *NewTunnel) { n.Tune = &FineTune{AcceptUDP: true} },
		"HTTPS fields on plain TCP": func(n *NewTunnel) { n.Transport = "tcp" },
	} {
		t.Run(label, func(t *testing.T) {
			n := panelHTTPSFixture(t, "xhttp", "server")
			change(&n)
			if _, err := specFromNew(n); err == nil {
				t.Fatal("invalid form accepted")
			}
		})
	}
	usePanelRealityProbe(t, func(context.Context, string, string, string) error { return errors.New("unreachable cover") })
	if _, err := specFromNew(panelHTTPSFixture(t, "reality", "server")); err == nil || !strings.Contains(err.Error(), "unreachable cover") {
		t.Fatal("REALITY accepted a cover the CLI probe refused")
	}
	var m ManagedHTTPS
	if err := json.Unmarshal([]byte(`{"ignoreTlsErrors":true}`), &m); err == nil {
		t.Fatal("unknown HTTPS field accepted")
	}
}

func TestPanelHTTPSChecksPublicListenerAndForwardConflicts(t *testing.T) {
	n := panelHTTPSFixture(t, "naive", "server")
	n.Ports = n.TunnelPort
	s, err := specFromNew(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseBusyForwards(s); err == nil {
		t.Fatal("HTTPS listener conflicts with a forwarded port")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	n.TunnelPort, n.Ports = listener.Addr().String(), "2087"
	s, err = specFromNew(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseBusyForwards(s); err == nil {
		t.Fatal("busy public HTTPS listener accepted")
	}
}

func TestPanelHTTPSGeneratedCertificateAndPartialEdits(t *testing.T) {
	n := panelHTTPSFixture(t, "naive", "server")
	n.Name = "panel-generated-https"
	n.Managed.Certificate, n.Managed.Key = "", ""
	n.Managed.ServerName = "127.0.0.1"
	s, err := specFromNew(n)
	if err != nil {
		t.Fatal(err)
	}
	if managedCertificateHost(s.NaiveServer.Certificate) != "127.0.0.1" {
		t.Fatal("generated private certificate lacks the address the peer dials")
	}
	var edit TunnelEdit
	if err := json.Unmarshal([]byte(`{"managed":{"internalPort":"3180","password":"new-password"}}`), &edit); err != nil {
		t.Fatal(err)
	}
	updated, changed, err := editedReverseSpec(s, edit)
	if err != nil || !changed {
		t.Fatalf("partial edit: %v, changed=%v", err, changed)
	}
	if updated.BindAddr != "127.0.0.1:3180" || updated.NaiveServer.Listen != s.NaiveServer.Listen || updated.NaiveServer.Password != "new-password" || updated.NaiveServer.Certificate != s.NaiveServer.Certificate {
		t.Fatal("partial edit lost an omitted setting or moved the public listener")
	}
	tcp, changed, err := editedReverseSpec(s, TunnelEdit{Transport: "tcp"})
	if err != nil || !changed || selectedTransport(tcp) != "tcp" || tcp.BindAddr != s.NaiveServer.Listen {
		t.Fatal("switching back to TCP did not retire the helper and restore the public endpoint")
	}
	n = panelHTTPSFixture(t, "xhttp", "client")
	s, err = specFromNew(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"managed":{"caFile":""}}`), &edit); err != nil {
		t.Fatal(err)
	}
	updated, changed, err = editedReverseSpec(s, edit)
	if err != nil || !changed || updated.XrayClient.CAFile != "" || updated.XrayClient.UUID != s.XrayClient.UUID {
		t.Fatal("explicitly clearing CA trust lost the client's identity")
	}
}

func TestPanelDirectMenuIncludesSpoofAndSNIWithWorkingForms(t *testing.T) {
	if !reflect.DeepEqual(PanelDirectCarriers(), DirectCarriers()) {
		t.Fatal("panel direct carrier choices differ from CLI")
	}
	for _, tr := range []string{"spoof", "sni"} {
		for _, side := range []string{"iran", "kharej"} {
			n := NewDirectTunnel{Side: side, Carrier: tr, Name: "panel-" + tr + "-" + side,
				Token: "panel-token", PeerAddr: "203.0.113.9", TunnelPort: "19000", Ports: "2087",
				SpoofPeerIP: "203.0.113.8", SNIDomain: "www.example.com"}
			s, err := n.spec()
			if err != nil {
				t.Fatal(err)
			}
			cfg := decode(t, s.Render())
			engine, err := l3.ConfigFromTable(cfg.L3)
			if err == nil {
				err = engine.Validate()
			}
			if err != nil {
				t.Fatalf("%s/%s: %v", tr, side, err)
			}
			if tr == "sni" && cfg.L3.SNIDomain != n.SNIDomain || tr == "spoof" && cfg.L3.SpoofConfig.SpoofPeerIP != n.SpoofPeerIP {
				t.Fatal("direct carrier lost its required settings")
			}
		}
	}
}

func TestPanelNaiveRefusesCertificateHostnameMismatch(t *testing.T) {
	n := panelHTTPSFixture(t, "naive", "server")
	n.Managed.ServerName = "wrong.example"
	if _, err := specFromNew(n); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Naive accepted a hostname its certificate cannot authenticate: %v", err)
	}
	n.Managed.ServerName = "localhost"
	s, err := specFromNew(n)
	if err != nil {
		t.Fatal(err)
	}
	cfg := decode(t, s.Render())
	if _, err := shareLinkOf(s.Name, "panel.example", cfg); err == nil {
		t.Fatal("Naive exported a peer address that cannot pass TLS verification")
	}
	for _, host := range []string{"localhost", "127.0.0.1"} {
		if _, err := shareLinkOf(s.Name, host, cfg); err != nil {
			t.Fatalf("certificate-matching Naive address %q was refused: %v", host, err)
		}
	}
}

func TestPanelHTTPSSwitchRetiresKCPOnlyPreset(t *testing.T) {
	usePanelRealityProbe(t, func(context.Context, string, string, string) error { return nil })
	for _, transport := range []string{"naive", "xhttp", "reality"} {
		n := panelHTTPSFixture(t, transport, "server")
		old := TunnelSpec{Name: n.Name, Role: "server", Transport: "kcp", BindAddr: "0.0.0.0:19443", Token: n.Token, Ports: []string{"2087"}}
		ApplyPreset(&old, PresetThroughput)
		for _, preset := range []string{"", PresetTurbo, PresetThroughput} {
			t.Run(transport+"/"+defaultString(preset, "keep-tuning"), func(t *testing.T) {
				edit := TunnelEdit{Transport: transport, Managed: n.Managed, Preset: preset}
				// The edit dialog also posts its unchanged displayed tuning.
				raw, err := json.Marshal(tuneOf(old))
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &edit.Tune); err != nil {
					t.Fatal(err)
				}
				s, changed, err := editedReverseSpec(old, edit)
				if preset == PresetThroughput {
					if err == nil {
						t.Fatal("HTTPS accepted the KCP-only profile simply because the old tunnel used it")
					}
					return
				}
				if err != nil || !changed || selectedTransport(s) != transport || !presetSuitsTransport(s.Preset, "tcp") {
					t.Fatalf("HTTPS switch kept an incompatible profile: preset=%q, changed=%v, error=%v", s.Preset, changed, err)
				}
				if preset == "" && (s.Preset != "" || s.Heartbeat != old.Heartbeat || s.ConnectionPool != old.ConnectionPool) {
					t.Fatal("retiring the unsupported profile label changed local tuning")
				}
				if preset == PresetTurbo && s.Preset != PresetTurbo {
					t.Fatal("unchanged old tuning overwrote the newly selected profile")
				}
			})
		}
	}
}
