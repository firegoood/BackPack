package peerauto

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpiredOrForeignPrepareCannotAuthorizeCurrentTraffic(t *testing.T) {
	m, e, _ := followerForState(t)
	generation := m.store.snapshot().Generation + 1
	tx := fresh()
	if m.transition(e, message{Op: "PREPARE_SWITCH", Generation: generation, Transaction: tx}).Op != "READY" {
		t.Fatal("prepare")
	}
	m.prepared.expires = time.Now().Add(-time.Second)
	if m.transition(e, message{Op: "COMMIT", Generation: generation, Transaction: tx, Nonce: fresh()}).Op != "REJECT" || m.active != nil {
		t.Fatal("expired prepare admitted traffic")
	}
	m.prepared = nil
	foreign := &candidate{spec: e.spec, ctx: e.ctx, probeAt: e.probeAt}
	foreign.spec.ID = "other"
	m.endpoints[foreign.spec.ID] = foreign
	if m.transition(foreign, message{Op: "COMMIT", Generation: generation, Transaction: tx, Nonce: fresh()}).Op != "REJECT" {
		t.Fatal("different endpoint committed another preparation")
	}
	delete(m.endpoints, e.spec.ID)
	if m.transition(e, message{Op: "PREPARE_SWITCH", Generation: generation + 1, Transaction: fresh()}).Op != "REJECT" {
		t.Fatal("replaced session retained switch authority")
	}
}

func TestProbePressureAndInvalidActionPreserveProduction(t *testing.T) {
	m, e, _ := followerForState(t)
	m.iran = true
	e.ctx, e.cancel = context.WithCancel(m.ctx)
	m.mu.Lock()
	m.installRouteLocked(e, 2, fresh())
	m.mu.Unlock()
	for range m.policy.MaxConnections / 2 {
		m.connections <- struct{}{}
	}
	before := m.store.snapshot()
	result := make(chan error, 1)
	m.handleAction(actionRequest{action: Action{Operation: "test-now", Candidate: e.spec.ID}, result: result})
	if err := <-result; !errors.Is(err, errProbePressure) {
		t.Fatalf("expected skipped pressure: %v", err)
	}
	m.handleAction(actionRequest{action: Action{Operation: "arbitrary-shell", Candidate: e.spec.ID}, result: result})
	if <-result == nil {
		t.Fatal("unknown command admitted")
	}
	if e.ctx.Err() != nil || m.active == nil || m.store.snapshot().Generation != before.Generation || e.failedProbes != 0 {
		t.Fatal("probe/action failure mutated production or carrier failure count")
	}
}

func TestExpiredQueuedActionDoesNotChangePolicy(t *testing.T) {
	m, _, _ := followerForState(t)
	m.iran = true
	ctx, cancel := context.WithCancel(m.ctx)
	cancel()
	result := make(chan error, 1)
	m.handleAction(actionRequest{ctx: ctx, action: Action{Operation: "observe"}, result: result})
	if !errors.Is(<-result, context.Canceled) || m.Snapshot().ObserveOnly {
		t.Fatal("expired queued request changed policy")
	}
}

func TestUncommittedPreparationExpiryRestoresPreviousRouteStatus(t *testing.T) {
	m, e, _ := followerForState(t)
	r := message{Op: "PREPARE_SWITCH", Generation: m.store.snapshot().Generation + 1, Transaction: fresh()}
	if m.transition(e, r).Op != "READY" {
		t.Fatal("prepare")
	}
	m.mu.Lock()
	m.prepared.expires = time.Now().Add(-time.Second)
	m.mu.Unlock()
	if m.transition(e, message{Op: "STATUS"}).Op != "STATUS" || m.store.snapshot().Intent != "" || m.Snapshot().State == "SWITCHING" {
		t.Fatal("expired preparation remained active")
	}
}

func TestStateSymlinkAndClonedSnapshotCannotReuseCurrentAuthorization(t *testing.T) {
	m, e, path := followerForState(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gen := m.store.snapshot().Generation + 1
	tx := fresh()
	if m.transition(e, message{Op: "PREPARE_SWITCH", Generation: gen, Transaction: tx}).Op != "READY" {
		t.Fatal("prepare")
	}
	if m.transition(e, message{Op: "COMMIT", Generation: gen, Transaction: tx, Nonce: fresh()}).Op != "COMMIT_ACK" {
		t.Fatal("commit")
	}
	oldInc := m.incarnation
	m.Close()
	if err = os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	c := stateConfig()
	c.Client.RemoteAddr = c.Server.BindAddr
	c.Client.Token = c.Server.Token
	c.Server.BindAddr = ""
	restarted, err := New(t.Context(), c, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.active != nil || restarted.incarnation == oldInc {
		t.Fatal("snapshot restored an active route or cryptographic incarnation")
	}
	symlink := filepath.Join(t.TempDir(), "state.json")
	if err = os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if s, err := openStore(symlink, m.manifest); err == nil {
		s.Close()
		t.Fatal("state symlink admitted")
	}
}

func TestReplayBoundToSequenceAndFreshIncarnation(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	base := message{Tunnel: "tunnel", Sender: "iran", Receiver: "kharej", Manifest: "manifest", Candidate: "wat", Incarnation: fresh(), PeerIncarnation: fresh()}
	reply := message{Version: 1, Op: "STATUS", Tunnel: base.Tunnel, Sender: base.Receiver, Receiver: base.Sender, Manifest: base.Manifest, Candidate: base.Candidate, Incarnation: base.PeerIncarnation, PeerIncarnation: base.Incarnation, Sequence: 1}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	w := wire{c: a, key: key, base: base}
	done := make(chan error, 1)
	go func() {
		if err := writeMessage(b, reply, key); err != nil {
			done <- err
			return
		}
		done <- writeMessage(b, reply, key)
	}()
	if _, err := w.receive(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.receive(); err == nil {
		t.Fatal("same signed frame replayed successfully")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	reply.Sequence = 2
	reply.PeerIncarnation = fresh()
	go func() { done <- writeMessage(b, reply, key) }()
	if _, err := w.receive(); err == nil {
		t.Fatal("cloned/restarted peer reused old control authorization")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLostCommitACKFencesAuthorityUntilFreshReconciliation(t *testing.T) {
	follower, fe, _ := followerForState(t)
	leader, err := New(t.Context(), stateConfig(), filepath.Join(t.TempDir(), "leader.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer leader.Close()
	ctx, cancel := context.WithCancel(leader.ctx)
	defer cancel()
	le := &candidate{spec: fe.spec, ctx: ctx, cancel: cancel, probeAt: time.Now(), samples: []float64{1, 1, 1}}
	leader.endpoints[le.spec.ID] = le
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	base := message{Tunnel: leader.policy.TunnelID, Sender: leader.policy.IranID, Receiver: leader.policy.KharejID, Manifest: leader.manifest, Candidate: le.spec.ID, Incarnation: leader.incarnation, PeerIncarnation: follower.incarnation}
	key := keyed([]byte(leader.token), "test-fresh-authorized-session:", fresh())
	le.w = &wire{c: a, key: key, base: base}
	other := base
	other.Sender, other.Receiver = base.Receiver, base.Sender
	other.Incarnation, other.PeerIncarnation = base.PeerIncarnation, base.Incarnation
	fe.w = &wire{c: b, key: key, base: other}
	leader.mu.Lock()
	leader.installRouteLocked(le, 1, fresh())
	old := leader.active
	leader.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		for range 3 {
			request, e := fe.w.receive()
			if e != nil {
				done <- e
				return
			}
			response := follower.transition(fe, request)
			if request.Op == "COMMIT" {
				if response.Op != "COMMIT_ACK" {
					done <- errors.New("follower did not commit")
					return
				}
				b.Close()
				done <- nil
				return
			}
			if e = fe.w.send(response); e != nil {
				done <- e
				return
			}
		}
	}()
	if err = leader.activate(le, "test interrupted commit"); err == nil {
		t.Fatal("lost ACK reported as successful atomic commit")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if leader.active != nil || old.ctx.Err() == nil || follower.active == nil || leader.store.snapshot().Intent == "" {
		t.Fatal("partial commit did not fence the authority and retain durable reconciliation intent")
	}
	if leader.store.snapshot().Generation != follower.store.snapshot().Generation {
		t.Fatal("lost ACK changed the proposed generation")
	}
}

type frameConn struct{ *bytes.Reader }

func (c frameConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c frameConn) Close() error                     { return nil }
func (c frameConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c frameConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c frameConn) SetDeadline(time.Time) error      { return nil }
func (c frameConn) SetReadDeadline(time.Time) error  { return nil }
func (c frameConn) SetWriteDeadline(time.Time) error { return nil }
func FuzzControlFrame(f *testing.F) {
	b, _ := json.Marshal(message{Version: 1, Op: "STATUS"})
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	f.Add(append(n[:], b...))
	f.Add([]byte{255, 255, 255, 255})
	f.Add([]byte{0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			return
		}
		m, err := readMessage(frameConn{bytes.NewReader(b)})
		if err == nil && len(b) < 4 {
			t.Fatal("unframed input accepted")
		}
		_ = validMessage(m, []byte("key"))
		if err != nil && !errors.Is(err, io.EOF) {
			return
		}
	})
}
