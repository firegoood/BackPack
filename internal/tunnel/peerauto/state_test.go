package peerauto

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
)

func stateConfig() *config.Config {
	return &config.Config{Server: config.ServerConfig{BindAddr: "127.0.0.1:23456", Transport: config.STEALTH, Token: "0123456789abcdef0123456789abcdef", Ports: []string{"23457=127.0.0.1:23458"}}, Auto: config.AutoConfig{Enabled: true, TunnelID: "state-test-tunnel", IranID: "state-test-iran-id", KharejID: "state-test-kharej-id", Ports: []string{"23457=127.0.0.1:23458"}, Candidates: []config.AutoCandidate{{ID: "noise", Transport: config.STEALTH, Endpoint: "127.0.0.1:23456"}}}}
}
func followerForState(t *testing.T) (*Manager, *candidate, string) {
	t.Helper()
	c := stateConfig()
	c.Client = config.ClientConfig{RemoteAddr: c.Server.BindAddr, Token: c.Server.Token, Transport: config.STEALTH}
	c.Server = config.ServerConfig{}
	path := filepath.Join(t.TempDir(), "state.json")
	m, err := New(context.Background(), c, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	e := &candidate{spec: c.Auto.Candidates[0], ctx: m.ctx, probeAt: time.Now()}
	m.endpoints[e.spec.ID] = e
	return m, e, path
}
func TestTwoPhaseFencingAndDuplicateCommit(t *testing.T) {
	m, e, _ := followerForState(t)
	base := m.store.snapshot().Generation
	tx := fresh()
	commit := message{Op: "COMMIT", Generation: base + 1, Transaction: tx, Nonce: fresh()}
	if r := m.transition(e, commit); r.Op != "REJECT" || m.active != nil {
		t.Fatal("unprepared commit admitted traffic")
	}
	prepare := message{Op: "PREPARE_SWITCH", Generation: base + 1, Transaction: tx}
	if r := m.transition(e, prepare); r.Op != "READY" || m.active != nil {
		t.Fatalf("prepare must be durable and nonactive: %+v", r)
	}
	if m.store.snapshot().Intent != e.spec.ID {
		t.Fatal("intent not persisted")
	}
	if r := m.transition(e, prepare); r.Op != "READY" {
		t.Fatal("duplicate prepare not idempotent")
	}
	if r := m.transition(e, commit); r.Op != "COMMIT_ACK" {
		t.Fatalf("commit rejected: %+v", r)
	}
	if r := m.transition(e, commit); r.Op != "COMMIT_ACK" {
		t.Fatal("lost ACK cannot be retried")
	}
	if r := m.transition(e, message{Op: "PREPARE_SWITCH", Generation: base, Transaction: fresh()}); r.Op != "REJECT" {
		t.Fatal("old generation admitted")
	}
	commit.Nonce = fresh()
	if r := m.transition(e, commit); r.Op != "REJECT" {
		t.Fatal("conflicting commit ticket admitted")
	}
}
func TestCrashAfterPrepareDoesNotRestoreActiveAuthorization(t *testing.T) {
	m, e, path := followerForState(t)
	gen := m.store.snapshot().Generation + 1
	tx := fresh()
	if m.transition(e, message{Op: "PREPARE_SWITCH", Generation: gen, Transaction: tx}).Op != "READY" {
		t.Fatal("prepare failed")
	}
	m.Close()
	s, err := openStore(path, m.manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.snapshot().Generation <= gen || s.snapshot().Intent != e.spec.ID {
		t.Fatal("crash boundary not fenced")
	}
	if m.active != nil {
		t.Fatal("prepared snapshot authorized production")
	}
}
func TestDuplicateControllerAndPrivateState(t *testing.T) {
	m, _, path := followerForState(t)
	if s, e := openStore(path, m.manifest); e == nil {
		s.Close()
		t.Fatal("concurrent controller admitted")
	}
	st, e := os.Stat(path)
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal("state permissions")
	}
}
func TestMessageIdentityReplayAndBounds(t *testing.T) {
	k := []byte("0123456789abcdef0123456789abcdef")
	m := message{Version: 1, Op: "COMMIT", Generation: 2, Transaction: fresh()}
	m = signMessage(m, k)
	if !validMessage(m, k) {
		t.Fatal("valid signature rejected")
	}
	m.Candidate = "foreign"
	if validMessage(m, k) {
		t.Fatal("candidate substitution passed MAC")
	}
	if validMessage(signMessage(m, []byte("other")), k) {
		t.Fatal("foreign peer key admitted")
	}
	m.Version = 2
	if validMessage(signMessage(m, k), k) {
		t.Fatal("unknown protocol version admitted")
	}
}
