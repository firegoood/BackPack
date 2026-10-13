package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/waticmp"
)

func ctWatCase(tr string) bool { return tr == "waticmp" || tr == "waticmp-reverse" }

// Keep the ordinary test configuration within a single packet. Native peer
// addresses/settings are fetched through the existing authenticated per-carrier
// exchange, just like HTTPS trust material; neither side opens an admin port.
func (s *ConnTestIran) startWatCase(c *connTestCase, used map[int]bool, o ConnTestOptions) {
	if ok, why := waticmp.Availability(); !ok {
		c.skip = "SKIPPED: " + why
		return
	}
	iran, peer := net.ParseIP(s.link.Host), net.ParseIP(strings.TrimSpace(o.WatPeerIPv4))
	if iran == nil || iran.To4() == nil || peer == nil || peer.To4() == nil {
		c.skip = "Wat needs explicit Iran and Kharej IPv4 addresses; fill the optional Kharej IPv4 before starting"
		return
	}
	c.entry = ctPickPort(used, true)
	if c.entry == 0 {
		c.skip = "no free local forwarded port"
		return
	}
	dir := "normal"
	if c.tr == "waticmp-reverse" {
		dir = "reverse"
	}
	settings := config.WatICMPConfig{Enabled: true, PeerIP: peer.String(), TunnelID: ctCaseToken(s.link.Tok, "wat-id", c.tr), Direction: dir, Paths: 4, MaxStreams: 128, MaxPayload: 512, MaxBps: 1_000_000}
	token := ctCaseToken(s.link.Tok, "reverse", c.tr)
	spec := TunnelSpec{Name: c.name, Role: "server", Transport: "waticmp", Token: token, BindAddr: "0.0.0.0:0", WatICMP: settings,
		Ports: []string{fmt.Sprintf("127.0.0.1:%d=127.0.0.1:%d", c.entry, s.link.TCP)}, LogLevel: "info", Heartbeat: 5, ConnectionPool: 4}
	if err := spec.ValidatePeerFeatures(); err != nil {
		c.skip = err.Error()
		return
	}
	engine, err := startCTEngine(s.dir, c.name, ctQuiet(spec.Render()))
	if err != nil {
		c.skip = "native engine could not start: " + err.Error()
		return
	}
	s.engines = append(s.engines, engine)
	c.link = ShareLink{V: 1, Kind: "reverse", From: "iran", Name: c.name, Tr: c.tr, Tok: token, Port: "0", Ports: strings.Join(spec.Ports, ", ")}
	enrollment := watEnrollment{IranIP: iran.String(), KharejIP: peer.String(), Settings: settings}
	enrollment.Settings.PeerIP = ""
	body, _ := json.Marshal(enrollment)
	s.coord.mu.Lock()
	if s.coord.carriers == nil {
		s.coord.carriers = map[string]string{}
	}
	s.coord.carriers[c.tr] = gzipB64(body)
	s.coord.mu.Unlock()
}

func ctWatClient(ctx context.Context, link ConnTestLink, c ShareLink) (TunnelSpec, error) {
	if ok, why := waticmp.Availability(); !ok {
		return TunnelSpec{}, fmt.Errorf("SKIPPED: %s", why)
	}
	response, err := ctAskContext(ctx, link.Host, link.Coord, "carrier "+link.Tok+" "+c.Tr)
	if err != nil || !strings.HasPrefix(response, "carrier ") {
		return TunnelSpec{}, fmt.Errorf("native enrollment unavailable: %v", err)
	}
	body, err := unGzipB64(strings.TrimPrefix(response, "carrier "))
	var w watEnrollment
	if err != nil || len(body) > 4096 || strictJSON(body, &w) != nil {
		return TunnelSpec{}, fmt.Errorf("damaged native test enrollment")
	}
	w.Settings.PeerIP = w.IranIP
	spec := TunnelSpec{Name: c.Name, Role: "client", Transport: "waticmp", Token: c.Tok, WatICMP: w.Settings,
		RemoteAddr: net.JoinHostPort(w.IranIP, "0"), LogLevel: "info", Heartbeat: 5, ConnectionPool: 4}
	if c.Tr == "waticmp-reverse" && w.Settings.Direction != "reverse" || c.Tr == "waticmp" && w.Settings.Direction != "normal" {
		return spec, fmt.Errorf("native test direction mismatch")
	}
	if net.ParseIP(w.KharejIP) == nil || w.Settings.TunnelID != ctCaseToken(link.Tok, "wat-id", c.Tr) || c.Port != strconv.Itoa(0) {
		return spec, fmt.Errorf("native test identity mismatch")
	}
	return spec, spec.ValidatePeerFeatures()
}
