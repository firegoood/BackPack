package peerauto

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

const wireVersion = 1

var errPeerRejected = errors.New("peer rejected coordination transaction")

var operations = map[string]bool{"HELLO": true, "CAPABILITIES": true, "STATE_SYNC": true, "STATUS": true, "PROBE_OFFER": true, "PROBE_READY": true, "PROBE_START": true, "PROBE_RESULT": true, "PREPARE_SWITCH": true, "READY": true, "REJECT": true, "COMMIT": true, "COMMIT_ACK": true, "ABORT": true, "DRAIN": true, "RECOVER": true}

type message struct {
	Version         int    `json:"version"`
	Op              string `json:"op"`
	Tunnel          string `json:"tunnel"`
	Sender          string `json:"sender"`
	Receiver        string `json:"receiver"`
	Manifest        string `json:"manifest"`
	Candidate       string `json:"candidate"`
	Incarnation     string `json:"incarnation"`
	PeerIncarnation string `json:"peerIncarnation"`
	Nonce           string `json:"nonce,omitempty"`
	Sequence        uint64 `json:"sequence"`
	Generation      uint64 `json:"generation"`
	Transaction     string `json:"transaction,omitempty"`
	Active          string `json:"active,omitempty"`
	Bytes           int    `json:"bytes,omitempty"`
	Digest          string `json:"digest,omitempty"`
	Reason          string `json:"reason,omitempty"`
	MAC             string `json:"mac"`
}

func keyed(key []byte, parts ...string) []byte {
	h := hmac.New(sha256.New, key)
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}
func fresh() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func signMessage(m message, key []byte) message {
	m.MAC = ""
	b, _ := json.Marshal(m)
	m.MAC = hex.EncodeToString(keyed(key, "github.com/backpack/backpack/peer-auto/v1/control:", string(b)))
	return m
}
func validMessage(m message, key []byte) bool {
	got, e := hex.DecodeString(m.MAC)
	want, _ := hex.DecodeString(signMessage(m, key).MAC)
	return e == nil && hmac.Equal(got, want) && m.Version == wireVersion && operations[m.Op] && m.Generation <= 1<<62 && len(m.Reason) <= 160
}
func writeMessage(c net.Conn, m message, key []byte) error {
	b, e := json.Marshal(signMessage(m, key))
	if e != nil {
		return e
	}
	if len(b) > 4096 {
		return errors.New("coordination frame too large")
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	_, e = io.Copy(c, bytes.NewReader(append(n[:], b...)))
	return e
}
func readMessage(c net.Conn) (message, error) {
	var m message
	var n [4]byte
	if _, e := io.ReadFull(c, n[:]); e != nil {
		return m, e
	}
	size := binary.BigEndian.Uint32(n[:])
	if size == 0 || size > 4096 {
		return m, errors.New("invalid coordination size")
	}
	b := make([]byte, size)
	if _, e := io.ReadFull(c, b); e != nil {
		return m, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&m); e != nil {
		return m, e
	}
	if d.Decode(new(any)) != io.EOF {
		return m, errors.New("trailing coordination data")
	}
	return m, nil
}

type wire struct {
	c                net.Conn
	key              []byte
	base             message
	sendSeq, recvSeq uint64
}

func (w *wire) send(m message) error {
	if w.sendSeq >= 1<<62 {
		return errors.New("control sequence budget exhausted")
	}
	w.sendSeq++
	m.Version = wireVersion
	m.Tunnel = w.base.Tunnel
	m.Sender = w.base.Sender
	m.Receiver = w.base.Receiver
	m.Manifest = w.base.Manifest
	m.Candidate = w.base.Candidate
	m.Incarnation = w.base.Incarnation
	m.PeerIncarnation = w.base.PeerIncarnation
	m.Sequence = w.sendSeq
	return writeMessage(w.c, m, w.key)
}
func (w *wire) receive() (message, error) {
	m, e := readMessage(w.c)
	if e != nil {
		return m, e
	}
	if w.recvSeq >= 1<<62 || !validMessage(m, w.key) || m.Sequence != w.recvSeq+1 || m.Tunnel != w.base.Tunnel || m.Manifest != w.base.Manifest || m.Candidate != w.base.Candidate || m.Sender != w.base.Receiver || m.Receiver != w.base.Sender || m.Incarnation != w.base.PeerIncarnation || m.PeerIncarnation != w.base.Incarnation {
		return m, errors.New("coordination identity, version, policy or replay check failed")
	}
	w.recvSeq++
	return m, nil
}
func (w *wire) ask(m message, expect string) (message, error) {
	_ = w.c.SetDeadline(time.Now().Add(8 * time.Second))
	defer w.c.SetDeadline(time.Time{})
	if e := w.send(m); e != nil {
		return message{}, e
	}
	r, e := w.receive()
	if e == nil && r.Op == "REJECT" && r.Transaction == m.Transaction {
		if r.Reason == errProbePressure.Error() {
			return r, errProbePressure
		}
		return r, errPeerRejected
	}
	if e == nil && (r.Op != expect || r.Transaction != m.Transaction) {
		e = errors.New("peer rejected or mismatched coordination transaction")
	}
	return r, e
}

// Bilateral fresh challenges and carrier binding prevent old snapshot/replayed
// HELLO material from admitting either control messages or production streams.
func (m *Manager) authenticate(c net.Conn, s carrierSession, id, probeTx string) (*wire, message, error) {
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	defer c.SetDeadline(time.Time{})
	binding, e := s.Binding(m.token)
	if e != nil {
		return nil, message{}, e
	}
	key := keyed([]byte(m.token), "github.com/backpack/backpack/peer-auto/v1/enrollment:", m.policy.TunnelID, binding)
	local, remote := m.policy.KharejID, m.policy.IranID
	if m.iran {
		local, remote = remote, local
	}
	base := message{Version: wireVersion, Tunnel: m.policy.TunnelID, Sender: local, Receiver: remote, Manifest: m.manifest, Candidate: id, Incarnation: m.incarnation, Generation: m.store.snapshot().Generation, Nonce: fresh(), Transaction: probeTx}
	var peer message
	if !m.iran {
		base.Op = "HELLO"
		if e = writeMessage(c, base, key); e == nil {
			peer, e = readMessage(c)
		}
	} else {
		peer, e = readMessage(c)
	}
	if e != nil {
		return nil, peer, e
	}
	if !validMessage(peer, key) || peer.Tunnel != base.Tunnel || peer.Sender != remote || peer.Receiver != local || peer.Manifest != base.Manifest || peer.Candidate != id || len(peer.Nonce) != 64 || len(peer.Incarnation) != 64 {
		return nil, peer, errors.New("peer enrollment/capabilities mismatch; upgrade and enroll both peers")
	}
	if m.iran {
		if peer.Op != "HELLO" || peer.PeerIncarnation != "" {
			return nil, peer, errors.New("invalid initial HELLO")
		}
		if peer.Transaction != "" {
			m.mu.Lock()
			p := m.pendingProbes[peer.Transaction]
			allowed := p != nil && p.candidate == id && p.peerIncarnation == peer.Incarnation
			m.mu.Unlock()
			if !allowed {
				return nil, peer, errors.New("probe session was not authorized by this enrolled peer")
			}
			base.Transaction = peer.Transaction
		}
		base.Op = "CAPABILITIES"
		base.PeerIncarnation = peer.Incarnation
		if e = writeMessage(c, base, key); e != nil {
			return nil, peer, e
		}
	} else if peer.Op != "CAPABILITIES" || peer.PeerIncarnation != base.Incarnation || peer.Transaction != probeTx {
		return nil, peer, errors.New("invalid peer challenge")
	}
	first, second := base.Nonce, peer.Nonce
	if m.iran {
		first, second = second, first
	}
	base.PeerIncarnation = peer.Incarnation
	w := &wire{c: c, base: base, key: keyed(key, "fresh-session:", first, second, m.policy.IranID, m.policy.KharejID)}
	if !m.iran {
		e = w.send(message{Op: "STATE_SYNC", Generation: base.Generation, Active: m.store.snapshot().Active})
		if e == nil {
			peer, e = w.receive()
		}
	} else {
		peer, e = w.receive()
		if e == nil && peer.Op != "STATE_SYNC" {
			e = errors.New("missing challenge confirmation")
		}
		if e == nil {
			e = w.send(message{Op: "STATE_SYNC", Generation: base.Generation, Active: m.store.snapshot().Active})
		}
	}
	return w, peer, e
}
