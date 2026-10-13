package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/backpack/backpack/internal/manage"
)

func TestPeerEnrollmentIsStrictPrivateAndMirrorsTheIranManifest(t *testing.T) {
	isolateAccess(t)
	l := manage.ShareLink{Name: "web-peer", Kind: "reverse", From: "iran", Tok: strings.Repeat("k", 32), Tr: "waticmp", Port: "0", Host: "192.0.2.1", Ports: "443",
		WatEnrollment: `{"iranIp":"192.0.2.1","kharejIp":"192.0.2.2","settings":{"enabled":true,"tunnelId":"web-enrolled-native","direction":"reverse"}}`}
	raw, err := l.Encode()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"name": "imported", "transport": "waticmp", "link": raw})
	var captured *manage.NewTunnel
	create := func(n manage.NewTunnel) (string, bool, error) {
		captured = &n
		return "fullpack-imported.service", true, nil
	}
	s := &server{sessions: newSessionStore()}
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { servePeerEnrollment(w, r, create) })
	read, _, err := IssueToken("read", ScopeRead, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	write, _, err := IssueToken("write", ScopeWrite, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", read, write} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tunnel/peer-enrollment", bytes.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		h(w, r)
		if token != write {
			if w.Code < 400 || captured != nil {
				t.Fatal("unauthorized enrollment reached create")
			}
			continue
		}
		if w.Code != 200 || captured == nil || captured.Role != "client" || captured.WatICMP.PeerIP != "192.0.2.1" || captured.WatICMP.Direction != "reverse" {
			t.Fatalf("mirror failed: status=%d body=%s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), l.Tok) {
			t.Fatal("response disclosed enrollment secret")
		}
	}
	for _, invalid := range []string{
		string(body) + `{}`, `{"link":"backpack://1.bad","name":"x","transport":"waticmp"}`,
		`{"link":"` + raw + `","name":"x","transport":"auto"}`,
		`{"link":"` + raw + `","name":"x","transport":"waticmp","command":"reboot"}`,
		`{"link":"` + strings.Repeat("A", maxTunnelBody) + `"}`,
	} {
		captured = nil
		w := httptest.NewRecorder()
		servePeerEnrollment(w, httptest.NewRequest("POST", "/", strings.NewReader(invalid)), create)
		if w.Code != 400 || captured != nil {
			t.Fatal("invalid enrollment reached create")
		}
	}
}
