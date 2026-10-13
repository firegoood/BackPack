package webui

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/backpack/backpack/internal/enginectl"
	"github.com/backpack/backpack/internal/manage"
	"github.com/backpack/backpack/internal/manage/core"
	"github.com/backpack/backpack/internal/tunnel/peerauto"
)

// This authenticated panel route uses the existing private engine socket.
// Enrollment endpoints remain tunnel carriers, never a public admin listener.
func (s *server) handlePeerAuto(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if err := core.CheckName(name); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t, ok := core.Find(name)
	if !ok {
		http.Error(w, "tunnel not found", 404)
		return
	}
	if t.Transport != "auto" {
		http.Error(w, "Auto is not enabled for this tunnel", http.StatusConflict)
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var action peerauto.Action
		if err := d.Decode(&action); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := d.Decode(new(any)); err != io.EOF {
			http.Error(w, "unexpected trailing action data", 400)
			return
		}
		switch action.Operation {
		case "test-now", "switch", "lock", "unlock", "observe", "resume", "disable-carrier":
		default:
			http.Error(w, "unknown Auto action", 400)
			return
		}
		if err := enginectl.AskBody(name, enginectl.OpAutoAction, action, nil); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var status peerauto.Status
	if err := enginectl.Ask(name, enginectl.OpAutoStatus, &status); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, status)
}

// A narrowly scoped enrollment operation on the existing authenticated panel.
// It cannot apply old links, shell commands, arbitrary config paths or actions.
func (s *server) handlePeerEnrollment(w http.ResponseWriter, r *http.Request) {
	servePeerEnrollment(w, r, manage.CreateTunnel)
}
func servePeerEnrollment(w http.ResponseWriter, r *http.Request, create func(manage.NewTunnel) (string, bool, error)) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Link      string `json:"link"`
		Name      string `json:"name"`
		Transport string `json:"transport"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTunnelBody))
	d.DisallowUnknownFields()
	if err := d.Decode(&request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		http.Error(w, "unexpected trailing enrollment data", 400)
		return
	}
	n, err := manage.PeerNewTunnel(request.Link, request.Name, request.Transport)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// The shared create lifecycle validates again, refuses name collisions,
	// writes private config and reports actual service start status.
	service, active, err := create(n)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"name": n.Name, "service": service, "status": "created", "started": active})
}
