// Native Wat ICMP uses the attributed Session/Stream engine with fresh peer authentication.
// Forwarded sockets, pool admission, UDP flows, heartbeat and backend relay use
// the existing reverse-transport lifecycle. It has no smux or helper process.
package transport

import (
	"context"
	"crypto/subtle"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/backpack/backpack/internal/controlwire"
	"github.com/backpack/backpack/internal/metrics"
	"github.com/backpack/backpack/internal/utils"
	"github.com/backpack/backpack/internal/utils/handlers"
	"github.com/backpack/backpack/internal/web"

	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"github.com/sirupsen/logrus"
)

type watGen struct {
	ctx              context.Context
	tunnelChannel    chan net.Conn // ready data streams waiting for a local conn
	localChannel     chan LocalTCPConn
	reqNewConnChan   chan struct{}
	usageMonitor     *web.Usage
	bye              atomic.Pointer[farewell]
	seat             clientSeat
	client           atomic.Pointer[waticmp.Conn]
	clearDiagnostics func()
}

type watClaim struct {
	ctrl net.Conn
	conn *waticmp.Conn
}

type WatTransport struct {
	lifecycle

	config         *WatConfig
	watSettings    waticmp.Settings
	controlChannel netControl
	limits         *limiter
}

type WatConfig struct {
	Wat            waticmp.Settings
	BindAddr       string
	SnifferLog     string
	Token          string
	Ports          []string
	AcceptUDP      bool
	Sniffer        bool
	ChannelSize    int
	WebPort        int
	Heartbeat      time.Duration
	KeepAlive      time.Duration
	SO_RCVBUF      int
	SO_SNDBUF      int
	ProxyProtocol  bool
	MaxConnections int
	BandwidthMbps  int
}

func (c *WatConfig) settings() waticmp.Settings { s := c.Wat; s.Token = c.Token; return s }

func NewWatServer(parentCtx context.Context, config *WatConfig, logger *logrus.Logger) *WatTransport {
	server := &WatTransport{
		config:      config,
		watSettings: config.settings(),
		lifecycle: lifecycle{
			parentctx: parentCtx,
			logger:    logger,
			usage:     usageSpec{webPort: config.WebPort, snifferLog: config.SnifferLog, sniffer: config.Sniffer},
		},
		limits: newLimiter(Limits{MaxConnections: config.MaxConnections, BandwidthMbps: config.BandwidthMbps}),
	}

	server.firstGeneration()

	return server
}

func (s *WatTransport) Start() {
	s.start(s.newGen(s.run.context()))
}

func (s *WatTransport) start(g *watGen) {
	go sweepTunnelConns(g.ctx, g.tunnelChannel)
	if s.config.WebPort > 0 {
		go g.usageMonitor.Monitor()
	}
	s.status.set("Disconnected (WATICMP)")

	handshake := make(chan watClaim)

	go s.tunnelListener(g, handshake)

	var first watClaim
	select {
	case <-g.ctx.Done():
		return
	case first = <-handshake:
	}
	s.seatClient(g, first)
	s.serveGeneration(s.forwarder(g), func() { s.handleLoop(g) })
}

func (s *WatTransport) seatClient(g *watGen, claim watClaim) {
	g.seat.sit(g.ctx,
		func() { s.vacate(g) },
		func() {
			g.bye.Store(newFarewell())
			g.client.Store(claim.conn)
			g.clearDiagnostics = metrics.SetTransportDiagnostics(func() any { return claim.conn.Diagnostic(s.watSettings.Direction) })
			s.controlChannel.Set(claim.ctrl)
			s.seated(claim.ctrl.RemoteAddr().String())
			s.status.set("Connected (WATICMP)")
			s.logger.Info("control channel successfully established.")
		},
		func(ctx context.Context, lost func()) {
			loop := s.control(g, ctx, lost)
			loop.link = controlwire.Net(claim.ctrl)
			loop.run()
		})
}

func (s *WatTransport) vacate(g *watGen) {
	if g.clearDiagnostics != nil {
		g.clearDiagnostics()
	}
	s.controlChannel.Close()
	s.controlChannel.Clear()
	if old := g.client.Swap(nil); old != nil {
		_ = old.CloseWithError(0, "replaced by a newer connection from the client")
	}
	drainTunnelConns(g.tunnelChannel)
	s.status.set("Disconnected (WATICMP)")
	metrics.ClearPeer()
}

func (s *WatTransport) Restart() {
	s.restart(s.controlChannel.Close, func(ctx context.Context) {
		s.controlChannel.Clear()
		go s.start(s.newGen(ctx))
	})
}

func (s *WatTransport) newGen(ctx context.Context) *watGen {
	g := &watGen{
		ctx:            ctx,
		tunnelChannel:  make(chan net.Conn, s.config.ChannelSize),
		localChannel:   make(chan LocalTCPConn, s.config.ChannelSize),
		reqNewConnChan: make(chan struct{}, s.config.ChannelSize),
		usageMonitor:   s.usageMonitor(ctx),
	}
	g.bye.Store(newFarewell())
	return g
}

const watFarewellFlush = 150 * time.Millisecond

func (s *WatTransport) tunnelListener(g *watGen, handshake chan<- watClaim) {
	s.listeners.hold()
	defer s.listeners.release()

	listener, ok := bindTunnelPort(g.ctx, s.logger, s.config.BindAddr, func() (*waticmp.Listener, error) {
		return waticmp.Listen(g.ctx, s.watSettings)
	})
	if !ok {
		return
	}

	s.logger.Infof("server started successfully, listening on address: %s (WATICMP)", listener.Addr().String())

	var connsMu sync.Mutex
	conns := map[*waticmp.Conn]struct{}{}

	defer listener.Close()

	goodbye := func() {
		if s.controlChannel.IsSet() {
			g.bye.Load().wait(farewellWait)
		}
		connsMu.Lock()
		told := len(conns) > 0
		for conn := range conns {
			_ = conn.CloseWithError(0, "server stopping")
		}
		connsMu.Unlock()
		if told {
			time.Sleep(watFarewellFlush)
		}
	}

	for {
		conn, err := listener.Accept(g.ctx)
		if err != nil {
			if g.ctx.Err() != nil {
				goodbye()
				return
			}
			s.logger.Debugf("failed to accept wat connection on %s: %v", listener.Addr().String(), err)
			continue
		}

		connsMu.Lock()
		conns[conn] = struct{}{}
		connsMu.Unlock()

		go func() {
			s.handleConn(g, conn, handshake)
			connsMu.Lock()
			delete(conns, conn)
			connsMu.Unlock()
		}()
	}
}

func (s *WatTransport) handleConn(g *watGen, conn *waticmp.Conn, handshake chan<- watClaim) {
	gate := &watGate{pending: make(chan struct{}, watUnauthenticatedStreams)}
	for {
		stream, err := conn.AcceptStream(g.ctx)
		if err != nil {
			s.logger.Debugf("wat connection from %s closed: %v", conn.RemoteAddr(), err)
			return
		}
		reserved, ok := gate.admit()
		if !ok {
			s.logger.Warnf("closing the WATICMP connection from %s: it opened more than %d streams "+
				"before presenting the tunnel token", conn.RemoteAddr(), watUnauthenticatedStreams)
			_ = conn.CloseWithError(0, "unauthenticated")
			return
		}
		go s.acceptStream(g, conn, stream, handshake, gate, reserved)
	}
}

const watUnauthenticatedStreams = 8

type watGate struct {
	authed  atomic.Bool
	pending chan struct{}
}

func (q *watGate) admit() (reserved, ok bool) {
	if q.authed.Load() {
		return false, true
	}
	select {
	case q.pending <- struct{}{}:
		return true, true
	default:
		return false, false
	}
}

func (q *watGate) settled(authenticated bool, reserved bool) {
	if authenticated {
		q.authed.Store(true)
	}
	if reserved {
		<-q.pending
	}
}

func (s *WatTransport) acceptStream(g *watGen, conn *waticmp.Conn, stream *waticmp.Stream, handshake chan<- watClaim, gate *watGate, reserved bool) {
	wrapped := stream
	judged := false
	judge := func(ok bool) {
		if !judged {
			judged = true
			gate.settled(ok, reserved)
		}
	}
	defer judge(false)

	if err := stream.SetReadDeadline(time.Now().Add(controlClaimTimeout)); err != nil {
		wrapped.Close()
		return
	}
	token, signal, err := utils.ReceiveBinaryTransportString(wrapped)
	if err != nil {
		s.logger.Debugf("no announcement from %s: %v", conn.RemoteAddr(), err)
		wrapped.Close()
		return
	}
	stream.SetReadDeadline(time.Time{})

	bound := watProofMatches(conn, s.config.Token, token)
	if !bound {
		s.logger.Warnf("invalid security token received from %s — telling it so, rather than "+
			"closing without a word, which reads to the client exactly like an old server", conn.RemoteAddr())
		refuseControl(wrapped, utils.RefusedBadToken)
		return
	}
	judge(true)

	switch signal {
	case utils.SG_Chan:
		answer := s.config.Token
		if bound {
			proof, err := watServerProof(conn, s.config.Token)
			if err != nil {
				s.logger.Errorf("could not bind the answer to the WATICMP session: %v", err)
				wrapped.Close()
				return
			}
			answer = proof
		}
		if err := utils.SendBinaryTransportStringWithin(wrapped, answer, utils.SG_Chan, 10*time.Second); err != nil {
			s.logger.Errorf("failed to send security token: %v", err)
			wrapped.Close()
			return
		}

		if g.seat.serving() {
			s.logger.Warn("a new control channel claim arrived; adopting the new client in place")
			s.seatClient(g, watClaim{ctrl: wrapped, conn: conn})
			return
		}

		select {
		case handshake <- watClaim{ctrl: wrapped, conn: conn}:
		default:
			s.logger.Warnf("control channel handshake already in progress, discarding duplicate")
			wrapped.Close()
		}

	case utils.SG_TCP:
		if !s.controlChannel.IsSet() {
			s.logger.Debugf("data stream from %s arrived before a control channel, discarding", conn.RemoteAddr())
			wrapped.Close()
			return
		}
		if c := g.client.Load(); c != nil && c != conn {
			s.logger.Debugf("data stream from %s on a connection that is no longer the client's, discarding", conn.RemoteAddr())
			wrapped.Close()
			return
		}
		select {
		case g.tunnelChannel <- wrapped:
		default:
			s.logger.Warnf("tunnel channel is full, discarding data stream from %s", conn.RemoteAddr())
			wrapped.Close()
		}

	default:
		s.logger.Warnf("unexpected announcement signal %v from %s", signal, conn.RemoteAddr())
		wrapped.Close()
	}
}

func (s *WatTransport) control(g *watGen, ctx context.Context, lost func()) controlLoop {
	return controlLoop{
		ctx:           ctx,
		link:          controlwire.Net(s.controlChannel.Get()),
		beat:          s.config.Heartbeat,
		requests:      g.reqNewConnChan,
		log:           s.logger,
		restart:       lost,
		farewellFlush: watFarewellFlush,
		bye:           g.bye.Load(),
	}
}

func (s *WatTransport) handleLoop(g *watGen) {
	for {
		select {
		case <-g.ctx.Done():
			return

		case localConn := <-g.localChannel:
			if expired(localConn) {
				drop(localConn, s.limits, s.logger)
				continue
			}

			askStream := func() {
				select {
				case g.reqNewConnChan <- struct{}{}:
				default:
				}
			}
			askStream()

			pairing[net.Conn]{
				ctx: g.ctx, local: localConn, tunnel: g.tunnelChannel,
				limits: s.limits, log: s.logger, request: askStream,
				announce: func(st net.Conn, addr string) error {
					if err := st.SetWriteDeadline(time.Now().Add(pairingWait(localConn.timeCreated))); err != nil {
						return err
					}
					defer st.SetWriteDeadline(time.Time{})
					return utils.SendBinaryString(st, addr)
				},
				discard: func(st net.Conn) { st.Close() },
				relay: func(st net.Conn, local LocalTCPConn) {
					go func() {
						defer s.limits.release()
						handlers.TCPConnectionHandler(g.ctx,
							s.config.ProxyProtocol && !isUDPFlow(local.conn),
							local.conn, metrics.CountedConn(st), s.logger,
							g.usageMonitor, localForwardPort(local.conn), s.config.Sniffer)
					}()
				},
			}.run()
		}
	}
}

func (s *WatTransport) forwarder(g *watGen) portForwarder {
	return portForwarder{
		ctx: g.ctx, ports: s.config.Ports, acceptUDP: s.config.AcceptUDP,
		queue: g.localChannel, limits: s.limits, listeners: &s.listeners, log: s.logger,
		tune:   alwaysNodelay(s.logger),
		queued: nil,
	}
}

func watProofMatches(c *waticmp.Conn, _ string, p string) bool {
	return subtle.ConstantTimeCompare([]byte(p), []byte(c.Proof("client"))) == 1
}
func watServerProof(c *waticmp.Conn, _ string) (string, error) { return c.Proof("server"), nil }
