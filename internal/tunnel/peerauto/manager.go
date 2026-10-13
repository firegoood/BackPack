package peerauto

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/portmap"
	"github.com/backpack/backpack/internal/tunnel/probe"
	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"github.com/backpack/backpack/internal/utils/network"
	"github.com/sirupsen/logrus"
)

type candidate struct {
	spec            config.AutoCandidate
	session         carrierSession
	w               *wire
	ctx             context.Context
	cancel          context.CancelFunc
	data            chan net.Conn
	mu              sync.Mutex // one caller/transaction on a control stream
	lastSeen        atomic.Int64
	peerGeneration  uint64
	probeAt         time.Time
	probeScore      float64
	samples         []float64
	quarantineUntil time.Time
	failedProbes    int
	nextProbe       time.Time
	baseline        float64
}
type route struct {
	endpoint   *candidate
	generation uint64
	ticket     string
	ctx        context.Context
	cancel     context.CancelFunc
	retired    atomic.Bool
}
type preparation struct {
	endpoint    *candidate
	generation  uint64
	transaction string
	expires     time.Time
}

type CandidateStatus struct {
	ID              string    `json:"id"`
	Transport       string    `json:"transport"`
	Direction       string    `json:"direction,omitempty"`
	Ready           bool      `json:"ready"`
	Reason          string    `json:"reason,omitempty"`
	UsefulMbps      float64   `json:"usefulMbps"`
	RTTms           float64   `json:"rttMs"`
	LastProbe       time.Time `json:"lastProbe,omitempty"`
	QuarantineUntil time.Time `json:"quarantineUntil,omitempty"`
	Native          any       `json:"native,omitempty"`
}
type Status struct {
	Version     int               `json:"version"`
	TunnelID    string            `json:"tunnelId"`
	PeerID      string            `json:"peerId"`
	AuthorityID string            `json:"authorityId"`
	State       string            `json:"state"`
	Active      string            `json:"active"`
	LastGood    string            `json:"lastGood"`
	Generation  uint64            `json:"generation"`
	Reason      string            `json:"reason,omitempty"`
	LastSwitch  time.Time         `json:"lastSwitch,omitempty"`
	NextSwitch  time.Time         `json:"nextEligibleSwitch,omitempty"`
	ObserveOnly bool              `json:"observeOnly"`
	ManualLock  string            `json:"manualLock,omitempty"`
	History     []Event           `json:"history"`
	Candidates  []CandidateStatus `json:"candidates"`
	Flows       FlowStatus        `json:"flows"`
}

// Stream outcomes are reported after closure. Interrupted includes route/session
// fencing; delivered bytes cannot prove that an application transaction finished.
type FlowStatus struct {
	Live          int    `json:"live"`
	Admitted      uint64 `json:"admitted"`
	Completed     uint64 `json:"completed"`
	Drained       uint64 `json:"drained"`
	Interrupted   uint64 `json:"interrupted"`
	Rejected      uint64 `json:"rejected"`
	UDPQueueDrops uint64 `json:"udpQueueDrops"`
}
type Manager struct {
	ctx                                                                                    context.Context
	cancel                                                                                 context.CancelFunc
	policy                                                                                 config.AutoConfig
	iran                                                                                   bool
	token, manifest, incarnation                                                           string
	store                                                                                  *stateStore
	log                                                                                    *logrus.Logger
	mappings                                                                               []portmap.Mapping
	mu                                                                                     sync.Mutex
	endpoints                                                                              map[string]*candidate
	peerIncarnation                                                                        string
	active                                                                                 *route
	prepared                                                                               *preparation
	status                                                                                 Status
	recentTx                                                                               map[string]time.Time
	workers                                                                                sync.WaitGroup
	connections                                                                            chan struct{}
	actions                                                                                chan actionRequest
	pendingProbes                                                                          map[string]*pendingProbe
	unavailableSince                                                                       time.Time
	lastHealth                                                                             time.Time
	dataFailures                                                                           int
	runtimeLock                                                                            string
	runtimeDisabled                                                                        map[string]bool
	endpointCancel                                                                         map[string]context.CancelFunc
	degradedSamples                                                                        int
	tls                                                                                    network.TLSSettings
	flowAdmitted, flowCompleted, flowDrained, flowInterrupted, flowRejected, udpQueueDrops atomic.Uint64
}

func New(ctx context.Context, cfg *config.Config, statePath string) (*Manager, error) {
	if !cfg.Auto.Enabled {
		return nil, errors.New("peer Auto is not enabled")
	}
	if e := Validate(cfg); e != nil {
		return nil, e
	}
	a := Defaults(cfg.Auto)
	h := manifestHash(a)
	store, e := openStore(statePath, h)
	if e != nil {
		return nil, e
	}
	mctx, cancel := context.WithCancel(ctx)
	m := &Manager{ctx: mctx, cancel: cancel, policy: a, iran: cfg.Server.BindAddr != "", manifest: h, incarnation: fresh(), store: store,
		log: logrus.New(), endpoints: map[string]*candidate{}, recentTx: map[string]time.Time{}, connections: make(chan struct{}, a.MaxConnections), actions: make(chan actionRequest, 8), pendingProbes: map[string]*pendingProbe{}}
	m.token = cfg.Client.Token
	m.tls = tlsSettings(cfg)
	m.runtimeLock = a.ManualLock
	m.runtimeDisabled = map[string]bool{}
	m.endpointCancel = map[string]context.CancelFunc{}
	if m.iran {
		m.token = cfg.Server.Token
	}
	m.mappings, e = portmap.Expand(a.Ports, "127.0.0.1")
	if e != nil {
		m.Close()
		return nil, e
	}
	peer := a.IranID
	if m.iran {
		peer = a.KharejID
	}
	m.status = Status{Version: wireVersion, TunnelID: a.TunnelID, PeerID: peer, AuthorityID: a.IranID, State: "RECOVERING", LastGood: store.snapshot().LastGood, Generation: store.snapshot().Generation, ObserveOnly: a.ObserveOnly, ManualLock: a.ManualLock}
	for _, p := range a.Candidates {
		reason := "awaiting authenticated peer"
		if p.Disabled {
			reason = "disabled by enrollment policy"
		}
		m.status.Candidates = append(m.status.Candidates, CandidateStatus{ID: p.ID, Transport: string(p.Transport), Direction: p.Direction, Reason: reason})
	}
	return m, nil
}
func (m *Manager) Close() { m.cancel(); m.workers.Wait(); m.store.Close() }
func (m *Manager) Snapshot() Status {
	st := m.store.snapshot()
	next := m.nextSwitch(st, time.Now(), false)
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	if s.State == "DEGRADED" {
		next = maxTime(next, s.LastSwitch.Add(time.Duration(m.policy.MinimumDwell)*time.Second))
	}
	if next.After(time.Now()) {
		s.NextSwitch = next
	} else {
		s.NextSwitch = time.Time{}
	}
	s.Flows = FlowStatus{len(m.connections), m.flowAdmitted.Load(), m.flowCompleted.Load(), m.flowDrained.Load(), m.flowInterrupted.Load(), m.flowRejected.Load(), m.udpQueueDrops.Load()}
	s.Candidates = append([]CandidateStatus(nil), s.Candidates...)
	for i := range s.Candidates {
		if e := m.endpoints[s.Candidates[i].ID]; e != nil {
			if native, ok := e.session.(watCarrier); ok {
				s.Candidates[i].Native = native.c.Diagnostic(e.spec.Direction)
			}
		}
	}
	s.History = append([]Event(nil), s.History...)
	return s
}
func (m *Manager) setState(state, reason string) {
	m.mu.Lock()
	m.status.State, m.status.Reason = state, reason
	m.mu.Unlock()
}
func (m *Manager) candidateStatus(id string, update func(*CandidateStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.status.Candidates {
		if m.status.Candidates[i].ID == id {
			update(&m.status.Candidates[i])
			return
		}
	}
}
func (m *Manager) launch(f func()) { m.workers.Add(1); go func() { defer m.workers.Done(); f() }() }
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Start opens only explicitly enrolled data carriers. Ready passive endpoints
// persist independently of the active route: rendezvous has no wall-clock or
// current-control-channel dependency. There is one switching authority.
func (m *Manager) Start() error {
	if m.iran {
		if err := m.bindUsers(); err != nil {
			return err
		}
	}
	for _, p := range m.policy.Candidates {
		if p.Disabled {
			continue
		}
		p := p
		m.launch(func() { m.endpointLoop(p) })
	}
	if m.iran {
		return m.leaderLoop()
	}
	<-m.ctx.Done()
	return nil
}
func (m *Manager) endpointLoop(p config.AutoCandidate) {
	epctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	m.mu.Lock()
	if m.runtimeDisabled[p.ID] {
		m.mu.Unlock()
		return
	}
	m.endpointCancel[p.ID] = cancel
	m.mu.Unlock()
	if p.Transport == config.WATICMP {
		if ok, reason := waticmp.Availability(); !ok {
			m.candidateStatus(p.ID, func(s *CandidateStatus) { s.Ready = false; s.Reason = "SKIPPED: " + reason })
			return
		}
	}
	var accept acceptCarrier
	var closeListener func()
	if m.iran {
		var e error
		accept, closeListener, e = listenCarrier(epctx, m.policy, p, m.token, m.tls)
		if e != nil {
			m.candidateStatus(p.ID, func(s *CandidateStatus) { s.Reason = "endpoint unavailable: " + e.Error() })
			return
		}
		defer closeListener()
		// One main peer plus one separately authenticated ephemeral probe.
		// Both use the same explicitly permitted data endpoint, never user ports.
		permits := make(chan struct{}, 2)
		for epctx.Err() == nil {
			select {
			case permits <- struct{}{}:
			case <-epctx.Done():
				return
			}
			s, err := accept(epctx)
			if err != nil {
				<-permits
				if !pause(epctx, time.Second) {
					return
				}
				continue
			}
			m.launch(func() { defer func() { <-permits }(); defer s.Close(); m.runSession(p, s) })
		}
		return
	}
	backoff := time.Second
	for epctx.Err() == nil {
		var s carrierSession
		var err error
		if m.iran {
			s, err = accept(epctx)
		} else {
			s, err = dialCarrier(epctx, m.policy, p, m.token)
		}
		if err != nil {
			m.candidateStatus(p.ID, func(s *CandidateStatus) { s.Ready = false; s.Reason = "carrier handshake unavailable" })
			if !pause(epctx, backoff) {
				return
			}
			backoff = min(backoff*2, 15*time.Second)
			continue
		}
		backoff = time.Second
		// Serial sessions per accepting endpoint bound authentication/raw sockets.
		// A failed peer reconnects after liveness fencing releases its session.
		m.runSession(p, s)
		s.Close()
		if !pause(m.ctx, time.Second) {
			return
		}
	}
}
func (m *Manager) runSession(p config.AutoCandidate, s carrierSession) {
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { s.Close() })
	defer stop()
	var ctrl net.Conn
	var err error
	if m.iran {
		ctrl, err = s.Accept(ctx)
	} else {
		ctrl, err = s.Open(ctx)
	}
	if err != nil {
		return
	}
	defer ctrl.Close()
	_ = ctrl.SetDeadline(time.Now().Add(10 * time.Second))
	if !m.iran {
		_, err = ctrl.Write([]byte{0})
	} else {
		var b [1]byte
		_, err = io.ReadFull(ctrl, b[:])
		if err == nil && b[0] != 0 {
			err = errors.New("expected typed peer control stream")
		}
	}
	if err != nil {
		return
	}
	w, peer, err := m.authenticate(ctrl, s, p.ID, "")
	if err != nil {
		m.candidateStatus(p.ID, func(s *CandidateStatus) { s.Ready = false; s.Reason = "peer identity/version/policy rejected" })
		return
	}
	if w.base.Transaction != "" {
		m.deliverProbe(ctx, w)
		return
	}
	e := &candidate{spec: p, session: s, w: w, ctx: ctx, cancel: cancel, data: make(chan net.Conn, 4), peerGeneration: peer.Generation}
	e.lastSeen.Store(time.Now().UnixNano())
	m.mu.Lock()
	if m.runtimeDisabled[p.ID] {
		m.mu.Unlock()
		return
	}
	if old := m.endpoints[p.ID]; old != nil && old.ctx.Err() == nil {
		m.mu.Unlock()
		return
	}
	// A second process with the same enrolled identity cannot evict a live
	// incarnation. Transfer is permitted only after all old sessions are stale.
	if m.peerIncarnation != "" && m.peerIncarnation != w.base.PeerIncarnation {
		for _, old := range m.endpoints {
			if time.Since(time.Unix(0, old.lastSeen.Load())) < 20*time.Second {
				m.mu.Unlock()
				return
			}
		}
		m.fenceLocked()
		m.prepared = nil
		for id, old := range m.endpoints {
			old.cancel()
			delete(m.endpoints, id)
		}
	}
	m.peerIncarnation = w.base.PeerIncarnation
	m.endpoints[p.ID] = e
	m.mu.Unlock()
	m.candidateStatus(p.ID, func(s *CandidateStatus) { s.Ready = true; s.Reason = "authenticated enrolled endpoint" })
	defer func() {
		m.mu.Lock()
		if m.endpoints[p.ID] == e {
			delete(m.endpoints, p.ID)
			for i := range m.status.Candidates {
				if m.status.Candidates[i].ID == p.ID {
					m.status.Candidates[i].Ready = false
					m.status.Candidates[i].Reason = "peer control path disconnected"
				}
			}
		}
		if m.active != nil && m.active.endpoint == e {
			m.fenceLocked()
			m.status.State = "RECOVERING"
		}
		m.mu.Unlock()
		for {
			select {
			case c := <-e.data:
				c.Close()
			default:
				return
			}
		}
	}()
	if m.iran {
		// Receive pre-opened data streams while the single leader owns control.
		done := make(chan struct{})
		go func() { defer close(done); m.acceptData(e) }()
		<-ctx.Done()
		s.Close()
		<-done
	} else {
		done := make(chan struct{})
		go func() { defer close(done); m.openData(e) }()
		m.follow(e)
		cancel()
		s.Close()
		<-done
	}
}

func (m *Manager) follow(e *candidate) {
	for e.ctx.Err() == nil {
		_ = e.w.c.SetDeadline(time.Now().Add(90 * time.Second))
		r, err := e.w.receive()
		if err != nil {
			return
		}
		e.lastSeen.Store(time.Now().UnixNano())
		if r.Op == "PROBE_OFFER" {
			if err = m.followProbe(e, r); err != nil {
				m.log.WithField("candidate", e.spec.ID).WithError(err).Warn("enrolled probe responder stopped")
				// A separately isolated probe failure cannot close a healthy
				// production session. The authority consumes this typed rejection.
				reason := "isolated candidate probe unavailable"
				if errors.Is(err, errProbePressure) || errors.Is(err, errProbeBusy) {
					reason = errProbePressure.Error()
				}
				if e.w.send(message{Op: "REJECT", Transaction: r.Transaction, Reason: reason}) != nil {
					return
				}
			}
			continue
		}
		reply := m.transition(e, r)
		if err = e.w.send(reply); err != nil {
			return
		}
	}
}

// transition is the follower's sole durable state mutator. No proposal can
// change files, addresses, commands or backend authorization.
func (m *Manager) transition(e *candidate, r message) message {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store.snapshot()
	out := message{Op: "REJECT", Generation: st.Generation, Transaction: r.Transaction, Active: st.Active, Reason: "invalid or stale transition"}
	if e.ctx.Err() != nil || m.endpoints[e.spec.ID] != e {
		return out
	}
	if r.Op == "STATUS" || r.Op == "STATE_SYNC" || r.Op == "RECOVER" {
		if p := m.prepared; p != nil && time.Now().After(p.expires) {
			m.prepared = nil
			st.Intent, st.Transaction = "", ""
			if m.store.save(st) != nil {
				out.Op = "REJECT"
				out.Reason = "durable preparation expiry failed"
				return out
			}
			if m.active != nil {
				m.status.State = "HEALTHY"
			} else {
				m.status.State = "RECOVERING"
			}
			m.status.Reason = "uncommitted preparation expired"
		}
		out.Op = "STATUS"
		out.Transaction = r.Transaction
		return out
	}
	if r.Op == "ABORT" {
		out.Op = "ABORT"
		if m.prepared != nil && m.prepared.endpoint == e && m.prepared.transaction == r.Transaction {
			m.prepared = nil
			st.Intent = ""
			st.Transaction = ""
			if m.store.save(st) == nil {
				out.Op = "ABORT"
				if m.active != nil {
					m.status.State = "HEALTHY"
				} else {
					m.status.State = "RECOVERING"
				}
				m.status.Reason = "uncommitted preparation aborted"
			} else {
				out.Op, out.Reason = "REJECT", "durable abort failed"
			}
		}
		return out
	}
	if r.Op == "DRAIN" {
		out.Op = "DRAIN"
		return out
	}
	if r.Op == "PREPARE_SWITCH" {
		if p := m.prepared; p != nil && time.Now().Before(p.expires) {
			if p.endpoint == e && p.generation == r.Generation && p.transaction == r.Transaction {
				out.Op = "READY"
			}
			return out
		}
		if r.Generation <= st.Generation || r.Generation > st.Generation+1024 || len(r.Transaction) != 64 || m.policy.ManualLock != "" && m.policy.ManualLock != e.spec.ID || time.Since(e.probeAt) > 2*time.Minute || e.probeAt.IsZero() {
			return out
		}
		if _, used := m.recentTx[r.Transaction]; used {
			return out
		}
		st.Generation = r.Generation
		st.Intent = e.spec.ID
		st.Transaction = r.Transaction
		if err := m.store.save(st); err != nil {
			out.Reason = "durable prepare failed"
			return out
		}
		m.prepared = &preparation{endpoint: e, generation: r.Generation, transaction: r.Transaction, expires: time.Now().Add(15 * time.Second)}
		m.recentTx[r.Transaction] = time.Now()
		m.status.State = "SWITCHING"
		out.Op = "READY"
		out.Generation = r.Generation
		return out
	}
	if r.Op == "COMMIT" {
		if m.active != nil && m.active.endpoint == e && m.active.generation == r.Generation && st.Transaction == r.Transaction && m.active.ticket == r.Nonce {
			out.Op = "COMMIT_ACK"
			return out
		}
		p := m.prepared
		if p == nil || p.endpoint != e || p.generation != r.Generation || p.transaction != r.Transaction || time.Now().After(p.expires) || len(r.Nonce) != 64 {
			return out
		}
		st = recordCommit(st, e.spec.ID, time.Now())
		if err := m.store.save(st); err != nil {
			out.Reason = "durable commit failed"
			return out
		}
		m.installRouteLocked(e, r.Generation, r.Nonce)
		m.prepared = nil
		m.status.State, m.status.Active, m.status.LastGood, m.status.Generation = "HEALTHY", e.spec.ID, e.spec.ID, r.Generation
		m.status.LastSwitch = time.Now()
		m.status.Reason = "Iran authority commit confirmed"
		m.status.NextSwitch = time.Time{}
		out.Op, out.Generation, out.Active = "COMMIT_ACK", r.Generation, e.spec.ID
		return out
	}
	return out
}

func (m *Manager) followProbe(e *candidate, r message) error {
	if m.pressure() {
		return errProbePressure
	}
	if (r.Bytes != 32768 && r.Bytes != 512) || len(r.Transaction) != 64 {
		return errors.New("probe budget/transaction rejected")
	}
	if !m.acquireProbe() {
		return errProbeBusy
	}
	defer releaseProbe()
	m.mu.Lock()
	for tx, at := range m.recentTx {
		if time.Since(at) > time.Hour {
			delete(m.recentTx, tx)
		}
	}
	if len(m.recentTx) >= 256 {
		m.mu.Unlock()
		return errors.New("probe replay budget exhausted")
	}
	_, used := m.recentTx[r.Transaction]
	if !used {
		m.recentTx[r.Transaction] = time.Now()
	}
	m.mu.Unlock()
	if used {
		return errors.New("replayed probe transaction")
	}
	if err := e.w.send(message{Op: "PROBE_READY", Transaction: r.Transaction, Bytes: r.Bytes}); err != nil {
		return err
	}
	start, err := e.w.receive()
	if err != nil || start.Op != "PROBE_START" || start.Transaction != r.Transaction {
		return errors.New("invalid probe start")
	}
	if err = e.w.send(message{Op: "PROBE_READY", Transaction: r.Transaction}); err != nil {
		return err
	}
	// Fresh transport Session ID/keys, independent from the production/control
	// session. The typed offer is its one-use authorization; endpoints remain
	// exactly those in the enrolled manifest.
	probeCtx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	digest, release, err := m.respondIsolatedProbe(probeCtx, e, r.Transaction, r.Bytes)
	if err != nil {
		return err
	}
	defer release()
	result, err := e.w.receive()
	if err != nil || result.Op != "PROBE_RESULT" || result.Transaction != r.Transaction || result.Digest != digest || result.Bytes != r.Bytes {
		return errors.New("bidirectional probe result not confirmed")
	}
	if r.Bytes == 32768 {
		e.probeAt = time.Now()
	}
	return e.w.send(message{Op: "PROBE_RESULT", Transaction: r.Transaction, Digest: digest, Bytes: r.Bytes})
}
func (m *Manager) probe(e *candidate) error {
	return m.exchangeProbe(e, 32768)
}
func (m *Manager) exchangeProbe(e *candidate, probeBytes int) (err error) {
	if m.pressure() {
		return errProbePressure
	}
	if !m.acquireProbe() {
		return errProbeBusy
	}
	defer releaseProbe()
	e.mu.Lock()
	defer e.mu.Unlock()
	tx := fresh()
	probeCtx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	pending := &pendingProbe{ctx: probeCtx, candidate: e.spec.ID, peerIncarnation: e.w.base.PeerIncarnation, ch: make(chan probeConnection, 1)}
	m.mu.Lock()
	m.pendingProbes[tx] = pending
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.pendingProbes, tx); m.mu.Unlock() }()
	if e.spec.Transport == config.WATICMP {
		// ICMP has no listener port. Only after a local authenticated offer can
		// this separate, transaction-scoped raw identity allocate a socket.
		m.launch(func() {
			s, err := probeWatConnect(probeCtx, m.policy, e.spec, m.token, tx, true)
			if err == nil {
				defer s.Close()
				m.runSession(e.spec, s)
			}
		})
	}
	if _, err := e.w.ask(message{Op: "PROBE_OFFER", Transaction: tx, Bytes: probeBytes}, "PROBE_READY"); err != nil {
		if !errors.Is(err, errPeerRejected) && !errors.Is(err, errProbePressure) {
			e.cancel()
		}
		return fmt.Errorf("probe offer: %w", err)
	}
	if _, err := e.w.ask(message{Op: "PROBE_START", Transaction: tx}, "PROBE_READY"); err != nil {
		if !errors.Is(err, errPeerRejected) && !errors.Is(err, errProbePressure) {
			e.cancel()
		}
		return fmt.Errorf("probe start: %w", err)
	}
	aligned := false
	defer func() {
		if err == nil {
			return
		}
		cancel()
		// Join the responder's rejection before further control requests. This
		// preserves frame order and production sessions after an isolated failure.
		if !aligned {
			_ = e.w.c.SetDeadline(time.Now().Add(16 * time.Second))
			r, drainErr := e.w.receive()
			_ = e.w.c.SetDeadline(time.Time{})
			if drainErr != nil || r.Op != "REJECT" || r.Transaction != tx {
				e.cancel()
				return
			}
		}
		if _, abortErr := e.w.ask(message{Op: "ABORT", Transaction: tx}, "ABORT"); abortErr != nil {
			e.cancel()
		}
	}()
	var rtt time.Duration
	var elapsed time.Duration
	var digest string
	var pc probeConnection
	select {
	case pc = <-pending.ch:
	case <-probeCtx.Done():
		return fmt.Errorf("isolated session readiness: %w", probeCtx.Err())
	}
	defer close(pc.done)
	for _, size := range probeSizes(probeBytes) {
		d, h, err := probe.ExchangePaced(probeCtx, pc.w.c, size)
		if err != nil {
			return fmt.Errorf("isolated exchange (%d bytes): %w", size, err)
		}
		if size == 512 {
			rtt += d
		} else {
			elapsed = d
		}
		digest = h
	}
	// The responder waits for this fresh session's integrity confirmation
	// before either endpoint can mark the probe successful.
	if _, err := pc.w.ask(message{Op: "PROBE_RESULT", Transaction: tx, Digest: digest, Bytes: probeBytes}, "PROBE_RESULT"); err != nil {
		return fmt.Errorf("isolated integrity confirmation: %w", err)
	}
	if _, askErr := e.w.ask(message{Op: "PROBE_RESULT", Transaction: tx, Digest: digest, Bytes: probeBytes}, "PROBE_RESULT"); askErr != nil {
		aligned = errors.Is(askErr, errPeerRejected)
		if !aligned {
			e.cancel()
			aligned = true
		}
		return fmt.Errorf("control probe result: %w", askErr)
	}
	e.lastSeen.Store(time.Now().UnixNano())
	if probeBytes == 512 {
		return nil
	}
	e.probeAt = time.Now()
	mbps := float64(32768*8) / elapsed.Seconds() / 1e6
	e.probeScore = mbps / max(float64(rtt.Milliseconds())/3, 1)
	e.samples = append(e.samples, e.probeScore)
	if len(e.samples) > 3 {
		e.samples = e.samples[len(e.samples)-3:]
	}
	if e.baseline == 0 && len(e.samples) >= 3 {
		e.baseline = median(e.samples)
	}
	m.candidateStatus(e.spec.ID, func(s *CandidateStatus) {
		s.UsefulMbps = mbps
		s.RTTms = float64(rtt.Microseconds()) / 3000
		s.LastProbe = e.probeAt
		s.Reason = "both peers confirmed random echo and 32 KiB bulk integrity"
	})
	return nil
}
func (m *Manager) activate(e *candidate, reason string) error {
	if !m.iran {
		return errors.New("only the enrolled Iran authority can switch")
	}
	if time.Since(e.probeAt) > time.Minute {
		e.samples = nil
	}
	for len(e.samples) < 3 {
		if err := m.probe(e); err != nil {
			return err
		}
		if len(e.samples) < 3 && !pause(e.ctx, time.Second) {
			return e.ctx.Err()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.w.ask(message{Op: "STATE_SYNC"}, "STATUS")
	if err != nil {
		return err
	}
	st := m.store.snapshot()
	next := max(st.Generation, r.Generation) + 1
	tx := fresh()
	st.Generation, st.Intent, st.Transaction = next, e.spec.ID, tx
	if err = m.store.save(st); err != nil {
		return err
	}
	m.setState("SWITCHING", reason)
	if _, err = e.w.ask(message{Op: "PREPARE_SWITCH", Generation: next, Transaction: tx}, "READY"); err != nil {
		// No COMMIT was sent. The previous agreed route remains usable;
		// explicitly retire the bounded preparation and durable local intent.
		if errors.Is(err, errPeerRejected) {
			_, _ = e.w.ask(message{Op: "ABORT", Generation: next, Transaction: tx}, "ABORT")
		} else {
			e.cancel()
		}
		st.Intent, st.Transaction = "", ""
		if saveErr := m.store.save(st); saveErr != nil {
			return saveErr
		}
		m.mu.Lock()
		m.status.Generation = next
		if m.active != nil {
			m.status.State = "HEALTHY"
		} else {
			m.status.State = "RECOVERING"
		}
		m.status.Reason = "prepare refused; previous agreed route retained"
		m.mu.Unlock()
		return err
	}
	ticket := fresh()
	ack, err := e.w.ask(message{Op: "COMMIT", Generation: next, Transaction: tx, Nonce: ticket}, "COMMIT_ACK")
	if err != nil || ack.Generation != next || ack.Active != e.spec.ID {
		// Follower may have durably committed. Do not assume ACK loss means
		// rollback. Fence local new traffic, reconnect, then reconcile at a
		// higher fresh epoch over any authorized working passive endpoint.
		m.mu.Lock()
		m.fenceLocked()
		m.mu.Unlock()
		m.setState("RECOVERING", "commit acknowledgement uncertain; new traffic fenced")
		if err == nil {
			err = errors.New("commit confirmation mismatch")
		}
		e.cancel()
		return err
	}
	st = recordCommit(st, e.spec.ID, time.Now())
	if err = m.store.save(st); err != nil {
		m.mu.Lock()
		m.fenceLocked()
		m.mu.Unlock()
		e.cancel()
		return err
	}
	m.mu.Lock()
	m.installRouteLocked(e, next, ticket)
	m.status.State, m.status.Active, m.status.LastGood, m.status.Generation = "HEALTHY", e.spec.ID, e.spec.ID, next
	m.status.LastSwitch = time.Now()
	m.status.NextSwitch = time.Time{}
	m.status.Reason = reason
	m.mu.Unlock()
	m.dataFailures = 0
	m.lastHealth = time.Now()
	m.event(e.spec.ID, "COMMITTED", reason)
	m.log.WithFields(logrus.Fields{"candidate": e.spec.ID, "generation": next, "reason": reason}).Info("peer transport commit confirmed")
	return nil
}

func (m *Manager) String() string {
	s := m.Snapshot()
	return fmt.Sprintf("%s active=%s generation=%d", s.State, s.Active, s.Generation)
}

func (m *Manager) installRouteLocked(e *candidate, generation uint64, ticket string) {
	ctx, cancel := context.WithCancel(e.ctx)
	old := m.active
	m.active = &route{endpoint: e, generation: generation, ticket: ticket, ctx: ctx, cancel: cancel}
	// Existing streams keep their already approved route for at most 30 s.
	// New streams require the newly agreed generation immediately.
	if old != nil && old.cancel != nil {
		old.retired.Store(true)
		time.AfterFunc(30*time.Second, old.cancel)
	}
}

// The route is never restored from a snapshot without a fresh peer commit.
// Cancel its lifetime too: clearing a pointer alone leaks old flow authority.
func (m *Manager) fenceLocked() {
	if m.active != nil {
		m.active.cancel()
	}
	m.active = nil
	m.status.Active = ""
}
