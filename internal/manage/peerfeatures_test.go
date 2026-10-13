package manage

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/backpack/backpack/config"
)

func peerTestNew(auto bool) NewTunnel {
	n := NewTunnel{Name: "peer-test", Role: "server", Transport: "waticmp", Token: strings.Repeat("e", 64), Ports: "127.0.0.1:23457=127.0.0.1:23458",
		WatICMP: &config.WatICMPConfig{Enabled: true, PeerIP: "192.0.2.2", TunnelID: "native-test-pair-identity", Direction: "reverse", MaxPayload: 512, MaxStreams: 128, Paths: 4}}
	if auto {
		n.Transport = "auto"
		n.WatICMP = nil
		n.Auto = &config.AutoConfig{Enabled: true, ObserveOnly: true, TunnelID: "auto-test-pair-identity", IranID: "auto-test-iran-identity", KharejID: "auto-test-kharej-identity",
			Candidates: []config.AutoCandidate{{ID: "native", Transport: config.WATICMP, IranIP: "192.0.2.1", KharejIP: "192.0.2.2", Direction: "reverse", MaxPayload: 512},
				{ID: "noise", Transport: config.STEALTH, Endpoint: "192.0.2.1:24443", LocalBind: "0.0.0.0:24443"}}}
	}
	return n
}
func TestPeerSetupRenderEnrollmentAndMirror(t *testing.T) {
	for _, auto := range []bool{false, true} {
		n := peerTestNew(auto)
		spec, err := specFromPeerNew(n)
		if err != nil {
			t.Fatal(err)
		}
		var cfg config.Config
		if _, err = toml.Decode(spec.Render(), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Auto.Enabled != auto || cfg.Server.WatICMP.Direction != "reverse" {
			t.Fatal("feature settings lost on render")
		}
		cfg.Server.TLSKeyFile = "/secret/private.key"
		raw, err := sharePeerLink(spec.Name, "192.0.2.1", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(raw, "backpack://3.") {
			t.Fatal("new enrollment masquerades as old format")
		}
		link, err := DecodeShareLink(raw)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(link.AutoEnrollment+link.WatEnrollment, "/secret/") || strings.Contains(link.AutoEnrollment, "0.0.0.0:24443") {
			t.Fatal("local-only material exported")
		}
		if link.PeerNeedsAddress() {
			t.Fatal("self-contained enrollment requires legacy direct address bootstrap")
		}
		imported, err := PeerNewTunnel(raw, "imported-peer", n.Transport)
		if err != nil || imported.Role != "client" || imported.Name != "imported-peer" || imported.Token != n.Token {
			t.Fatalf("web enrollment lost credential/name/role: %v", err)
		}
		if _, err := PeerNewTunnel(raw, "imported-peer", "tcp"); err == nil {
			t.Fatal("enrollment accepted as a different method")
		}
		peer, err := kharejFromLink(link, LinkApplyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if peer.Role != "client" || peer.Auto.Enabled != auto || peer.WatICMP.PeerIP != "192.0.2.1" || peer.WatICMP.Direction != "reverse" {
			t.Fatal("incorrect role/address/direction mirroring")
		}
		if auto && (peer.Auto.IranID != spec.Auto.IranID || peer.Auto.KharejID != spec.Auto.KharejID || strings.Join(peer.Auto.Ports, ",") != n.Ports) {
			t.Fatal("paired authority or backend allowlist changed")
		}
		if err = peer.ValidatePeerFeatures(); err != nil {
			t.Fatal(err)
		}
		// HMAC binds every public root field as well as the approved endpoint manifest.
		link.Host = "198.51.100.1"
		if validatePeerLink(link) == nil {
			t.Fatal("modified enrollment accepted with old MAC")
		}
	}
}
func TestAutoSetupIsExplicitAndRejectsIgnoredLegacyFields(t *testing.T) {
	n := peerTestNew(true)
	n.Auto = nil
	if _, err := specFromPeerNew(n); err == nil {
		t.Fatal("Auto selected without enrollment")
	}
	n = peerTestNew(true)
	n.Conn = &ConnTune{}
	if _, err := specFromPeerNew(n); err == nil {
		t.Fatal("legacy connection drawer silently ignored")
	}
	n = peerTestNew(true)
	n.Auto.Candidates[1].Endpoint = "192.0.2.1:23457"
	if _, err := specFromPeerNew(n); err == nil {
		t.Fatal("candidate steals a user port")
	}
	n = peerTestNew(true)
	n.Auto.AllowPlaintextFallback = true
	if _, err := specFromPeerNew(n); err == nil {
		t.Fatal("unverified plaintext candidate policy accepted")
	}
}
func TestNativeConnectionTestSettingsPreserveBothDirections(t *testing.T) {
	for _, tr := range []string{"waticmp", "waticmp-reverse"} {
		if !ctSkippableCase("reverse/" + tr) {
			t.Fatal("native local failure cannot be reported as skipped")
		}
		dir := "normal"
		if tr == "waticmp-reverse" {
			dir = "reverse"
		}
		w := watEnrollment{IranIP: "192.0.2.1", KharejIP: "192.0.2.2", Settings: config.WatICMPConfig{Enabled: true, TunnelID: "test-peer-native-identity", Direction: dir, MaxPayload: 512}}
		b, _ := json.Marshal(w)
		encoded := gzipB64(b)
		if len("carrier "+encoded) > 1200 {
			t.Fatal("native enrollment exceeds test coordinator packet budget")
		}
		if ConnTestName(tr) == strings.ToUpper(tr) {
			t.Fatal("native case lacks distinct visible direction label")
		}
	}
}
