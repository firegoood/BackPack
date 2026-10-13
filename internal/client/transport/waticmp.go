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
	"github.com/backpack/backpack/internal/utils/network"

	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"github.com/sirupsen/logrus"
)

type WatTransport struct {
	lifecycle

	config      *WatConfig
	watSettings waticmp.Settings

	connMu           sync.Mutex
	watConn          *waticmp.Conn
	clearDiagnostics func()
}

type WatConfig struct {
	Wat            waticmp.Settings
	RemoteAddr     string
	Endpoints      *network.Endpoints
	Token          string
	SnifferLog     string
	Sniffer        bool
	KeepAlive      time.Duration
	RetryInterval  time.Duration
	DialTimeOut    time.Duration
	ConnPoolSize   int
	WebPort        int
	AggressivePool bool
	SO_RCVBUF      int
	SO_SNDBUF      int
}

func (c *WatConfig) settings() waticmp.Settings { s := c.Wat; s.Token = c.Token; return s }

func NewWatClient(parentCtx context.Context, config *WatConfig, logger *logrus.Logger) *WatTransport {
	client := &WatTransport{
		config:      config,
		watSettings: config.settings(),
	}
	client.firstGeneration(parentCtx, logger, usageSpec{webPort: config.WebPort, snifferLog: config.SnifferLog, sniffer: config.Sniffer})
	return client
}

func (c *WatTransport) setWATICMPConn(conn *waticmp.Conn) {
	c.connMu.Lock()
	if c.clearDiagnostics != nil {
		c.clearDiagnostics()
		c.clearDiagnostics = nil
	}
	if conn != nil {
		c.clearDiagnostics = metrics.SetTransportDiagnostics(func() any { return conn.Diagnostic(c.watSettings.Direction) })
	}
	c.watConn = conn
	c.connMu.Unlock()
}

func (c *WatTransport) getWATICMPConn() *waticmp.Conn {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.watConn
}

func (c *WatTransport) clearWATICMPConn(conn *waticmp.Conn) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.watConn == conn {
		if c.clearDiagnostics != nil {
			c.clearDiagnostics()
			c.clearDiagnostics = nil
		}
		c.watConn = nil
	}
}

func (c *WatTransport) Start() {
	if c.config.WebPort > 0 {
		c.state.Go(c.state.Usage().Monitor)
	}

	c.status.set("Disconnected (WATICMP)")

	c.state.Go(c.channelDialer)
}

func (c *WatTransport) Restart() {
	c.restart(func() {
		if qc := c.getWATICMPConn(); qc != nil {
			_ = qc.CloseWithError(0, "restart")
			c.setWATICMPConn(nil)
		}
	}, nil, c.Start)
}

func (c *WatTransport) channelDialer() {
	c.logger.Info("attempting to establish a new wat control channel connection...")

	bo := newBackoff(c.config.RetryInterval)

	for {
		select {
		case <-c.state.Ctx().Done():
			return
		default:
			conn, err := waticmp.Connect(c.state.Ctx(), false, c.watSettings)
			if err != nil {
				c.logger.Errorf("channel dialer: %v", err)
				if next := c.config.Endpoints.Rotate(); c.config.Endpoints.Len() > 1 {
					c.logger.Infof("trying next server endpoint: %s", next)
				}
				bo.Wait(c.state.Ctx())
				continue
			}

			stream, err := conn.OpenStreamSync(c.state.Ctx())
			if err != nil {
				c.logger.Errorf("failed to open control stream: %v", err)
				_ = conn.CloseWithError(0, "control open failed")
				bo.Wait(c.state.Ctx())
				continue
			}
			control := stream

			proof, err := watClientProof(conn, c.config.Token)
			if err != nil {
				c.logger.Errorf("could not bind the credential to the WATICMP session: %v", err)
				_ = conn.CloseWithError(0, "binding failed")
				bo.Wait(c.state.Ctx())
				continue
			}
			if err := utils.SendBinaryTransportStringWithin(control, proof, utils.SG_Chan, 10*time.Second); err != nil {
				c.logger.Errorf("failed to send the control channel claim: %v", err)
				_ = conn.CloseWithError(0, "claim send failed")
				bo.Wait(c.state.Ctx())
				continue
			}

			if err := control.SetReadDeadline(time.Now().Add(c.config.DialTimeOut)); err != nil {
				c.logger.Errorf("failed to set read deadline: %v", err)
				_ = conn.CloseWithError(0, "deadline failed")
				bo.Wait(c.state.Ctx())
				continue
			}

			message, answer, err := utils.ReceiveBinaryTransportString(control)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					c.logger.Warn("timeout while waiting for control channel response")
				} else {
					c.logger.Errorf("failed to receive control channel response: %v", err)
				}
				_ = conn.CloseWithError(0, "no response")
				if next := c.config.Endpoints.Rotate(); c.config.Endpoints.Len() > 1 {
					c.logger.Infof("trying next server endpoint: %s", next)
				}
				bo.Wait(c.state.Ctx())
				continue
			}

			control.SetReadDeadline(time.Time{})

			if why, refused := refusalReason(message, answer); refused {
				c.logger.Errorf("%s. Both peers require native Wat ICMP protocol v1; retrying...", why)
				_ = conn.CloseWithError(0, "refused")
				bo.Wait(c.state.Ctx())
				continue
			}
			want, err := watServerProof(conn, c.config.Token)
			if err != nil || subtle.ConstantTimeCompare([]byte(message), []byte(want)) != 1 {
				c.logger.Errorf("the peer did not prove the native Wat ICMP session binding; retrying...")
				_ = conn.CloseWithError(0, "bad proof")
				bo.Wait(c.state.Ctx())
				continue
			}

			c.setWATICMPConn(conn)
			if !c.state.SetConn(control) {
				_ = conn.CloseWithError(0, "generation ended before admission")
				c.clearWATICMPConn(conn)
				return
			}
			context.AfterFunc(c.state.Ctx(), func() {
				time.AfterFunc(100*time.Millisecond, func() { _ = conn.CloseWithError(0, "generation ended") })
			})
			c.logger.Info("control channel established successfully")

			metrics.ReportPeer(conn.RemoteAddr().String())

			c.status.set("Connected (WATICMP)")

			c.state.Go(c.poolMaintainer)
			loop := c.control()
			c.state.Go(func() {
				loop.run()
				_ = conn.CloseWithError(0, "generation ended")
				c.clearWATICMPConn(conn)
			})

			return
		}
	}
}

func (c *WatTransport) poolMaintainer() {
	poolSizer{
		mux:        true,
		ctx:        c.state.Ctx(),
		log:        c.logger,
		size:       c.config.ConnPoolSize,
		aggressive: c.config.AggressivePool,
		open:       &c.poolConnections,
		taken:      &c.loadConnections,
		shrink:     c.controlFlow,
		dial:       c.tunnelDialer,
		spawn:      c.state.Go,
		pending:    &c.dialingConnections,
		maxSize:    c.config.ConnPoolSize * poolGrowthLimit,
	}.maintain()
}

func (c *WatTransport) control() controlLoop {
	return c.lifecycle.control(controlwire.Net(c.state.Conn()), c.config.KeepAlive, c.tunnelDialer, c.Restart)
}

func (c *WatTransport) tunnelDialer() {
	ready, ok := c.beginPoolDial(c.config.ConnPoolSize * poolGrowthLimit)
	if !ok {
		return
	}
	defer ready()

	qc := c.getWATICMPConn()
	if qc == nil {
		return
	}

	stream, err := qc.OpenStreamSync(c.state.Ctx())
	if err != nil {
		c.logger.Errorf("failed to open tunnel stream: %v", err)
		return
	}
	data := stream
	defer c.state.Own(data)()

	proof, err := watClientProof(qc, c.config.Token)
	if err != nil {
		c.logger.Errorf("could not bind the credential to the WATICMP session: %v", err)
		data.Close()
		return
	}
	if err := utils.SendBinaryTransportStringWithin(data, proof, utils.SG_TCP, 10*time.Second); err != nil {
		c.logger.Errorf("failed to announce tunnel stream: %v", err)
		data.Close()
		return
	}

	atomic.AddInt32(&c.poolConnections, 1)
	ready()

	remoteAddr, err := utils.ReceiveBinaryString(data)
	atomic.AddInt32(&c.poolConnections, -1)
	if err != nil {
		c.logger.Tracef("tunnel stream closed before use: %v", err)
		data.Close()
		return
	}

	c.relayStream(data, remoteAddr, c.backend())
}

func (c *WatTransport) backend() backendOpts {
	return backendOpts{
		dialTimeout: c.config.DialTimeOut,
		keepAlive:   c.config.KeepAlive,
		rcvBuf:      c.config.SO_RCVBUF,
		sndBuf:      c.config.SO_SNDBUF,
		mss:         0,
		sniffer:     c.config.Sniffer,
	}
}

func watClientProof(c *waticmp.Conn, _ string) (string, error) { return c.Proof("client"), nil }
func watServerProof(c *waticmp.Conn, _ string) (string, error) { return c.Proof("server"), nil }
