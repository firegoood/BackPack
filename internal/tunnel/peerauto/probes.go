package peerauto

import (
	"context"
	"errors"
	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/probe"
	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"time"
)

type probeConnection struct {
	w    *wire
	done chan struct{}
}
type pendingProbe struct {
	ctx                        context.Context
	candidate, peerIncarnation string
	ch                         chan probeConnection
}

func (m *Manager) deliverProbe(ctx context.Context, w *wire) {
	m.mu.Lock()
	p := m.pendingProbes[w.base.Transaction]
	m.mu.Unlock()
	if p == nil {
		return
	}
	pc := probeConnection{w: w, done: make(chan struct{})}
	select {
	case p.ch <- pc:
	case <-ctx.Done():
		return
	case <-p.ctx.Done():
		return
	}
	select {
	case <-pc.done:
	case <-ctx.Done():
	case <-p.ctx.Done():
	}
}
func probeWatConnect(ctx context.Context, a config.AutoConfig, p config.AutoCandidate, token, tx string, iran bool) (carrierSession, error) {
	a.TunnelID += ":probe:" + tx
	s := watSettings(a, p, token, iran)
	s.Token = string(keyed([]byte(token), "ephemeral-probe:", tx))
	s.MaxStreams = 2
	s.MaxBps = 256000
	s.HandshakeTimeout = 10 * time.Second
	w, e := waticmp.Connect(ctx, iran, s)
	if e != nil {
		return nil, e
	}
	return watCarrier{w}, nil
}
func (m *Manager) respondIsolatedProbe(ctx context.Context, e *candidate, tx string, probeBytes int) (digest string, release func(), err error) {
	var s carrierSession
	if e.spec.Transport == config.WATICMP {
		s, err = probeWatConnect(ctx, m.policy, e.spec, m.token, tx, false)
	} else {
		s, err = dialCarrier(ctx, m.policy, e.spec, m.token)
	}
	if err != nil {
		return "", nil, err
	}
	c, err := s.Open(ctx)
	if err != nil {
		s.Close()
		return "", nil, err
	}
	stop := context.AfterFunc(ctx, func() { c.Close(); s.Close() })
	cleanup := func() { stop(); c.Close(); s.Close() }
	release = cleanup
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = c.Write([]byte{0}); err != nil {
		return "", nil, err
	}
	w, _, err := m.authenticate(c, s, e.spec.ID, tx)
	if err != nil {
		return "", nil, err
	}
	for _, size := range probeSizes(probeBytes) {
		digest, err = probe.RespondPaced(c, size)
		if err != nil {
			return "", nil, err
		}
	}
	r, err := w.receive()
	if err != nil || r.Op != "PROBE_RESULT" || r.Transaction != tx || r.Digest != digest || r.Bytes != probeBytes {
		return "", nil, errors.New("isolated session integrity result rejected")
	}
	if err = w.send(message{Op: "PROBE_RESULT", Transaction: tx, Digest: digest, Bytes: probeBytes}); err != nil {
		return "", nil, err
	}
	// Keep this session alive until the main authenticated control exchange
	// confirms the authority received the final result. Wat Write is buffered:
	// closing the entire session immediately would discard its last response.
	keep = true
	return digest, release, nil
}
func probeSizes(n int) []int {
	if n == 512 {
		return []int{512}
	}
	return []int{512, 512, 512, 32768}
}
