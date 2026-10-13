package peerauto

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"github.com/backpack/backpack/internal/utils/network"
	"github.com/quic-go/quic-go"
	"github.com/xtaci/smux"
)

// Session preserves native Wat multiplexing. Only byte-stream carriers receive
// smux; QUIC and Wat never acquire a second stream multiplexer.
type carrierSession interface {
	Open(context.Context) (net.Conn, error)
	Accept(context.Context) (net.Conn, error)
	Binding(string) (string, error)
	Close() error
}
type watCarrier struct{ c *waticmp.Conn }

func (w watCarrier) Open(ctx context.Context) (net.Conn, error)   { return w.c.OpenStreamSync(ctx) }
func (w watCarrier) Accept(ctx context.Context) (net.Conn, error) { return w.c.AcceptStream(ctx) }
func (w watCarrier) Binding(_ string) (string, error)             { return w.c.Proof("peer-auto-v1"), nil }
func (w watCarrier) Close() error                                 { return w.c.CloseWithError(0, "peer endpoint stopped") }

type quicCarrier struct{ c *quic.Conn }

func (q quicCarrier) Open(ctx context.Context) (net.Conn, error) {
	s, e := q.c.OpenStreamSync(ctx)
	if e != nil {
		return nil, e
	}
	return network.NewQUICStreamConn(s, q.c), nil
}
func (q quicCarrier) Accept(ctx context.Context) (net.Conn, error) {
	s, e := q.c.AcceptStream(ctx)
	if e != nil {
		return nil, e
	}
	return network.NewQUICStreamConn(s, q.c), nil
}
func (q quicCarrier) Binding(token string) (string, error) {
	return network.QUICClientProof(q.c, token)
}
func (q quicCarrier) Close() error { return q.c.CloseWithError(0, "peer endpoint stopped") }

type noiseCarrier struct{ s *smux.Session }

func (s noiseCarrier) Open(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.s.OpenStream()
}
func (s noiseCarrier) Accept(ctx context.Context) (net.Conn, error) {
	stop := context.AfterFunc(ctx, func() { s.s.Close() })
	defer stop()
	return s.s.AcceptStream()
}
func (s noiseCarrier) Binding(_ string) (string, error) { return "authenticated-Noise-NNpsk0", nil }
func (s noiseCarrier) Close() error                     { return s.s.Close() }
func multiplex(c net.Conn, iran bool) (carrierSession, error) {
	cfg := smux.DefaultConfig()
	cfg.MaxReceiveBuffer = 4 << 20
	cfg.MaxStreamBuffer = 128 << 10
	cfg.KeepAliveInterval = 5 * time.Second
	cfg.KeepAliveTimeout = 15 * time.Second
	var s *smux.Session
	var err error
	if iran {
		s, err = smux.Server(c, cfg)
	} else {
		s, err = smux.Client(c, cfg)
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return noiseCarrier{s}, nil
}

type acceptCarrier func(context.Context) (carrierSession, error)

func listenCarrier(ctx context.Context, a config.AutoConfig, p config.AutoCandidate, token string, tlsSettings network.TLSSettings) (acceptCarrier, func(), error) {
	bind := p.LocalBind
	if bind == "" {
		bind = p.Endpoint
	}
	switch p.Transport {
	case config.WATICMP:
		return func(c context.Context) (carrierSession, error) {
			w, e := waticmp.Connect(c, true, watSettings(a, p, token, true))
			if e != nil {
				return nil, e
			}
			return watCarrier{w}, nil
		}, func() {}, nil
	case config.QUIC:
		l, e := network.QUICListen(bind, network.QUICSettings{IncomingStreamLimit: int64(Defaults(a).MaxConnections + 16), KeepAlivePeriod: 5 * time.Second, MaxIdleTimeout: 15 * time.Second, SO_RCVBUF: 1 << 20, SO_SNDBUF: 1 << 20})
		if e != nil {
			return nil, nil, e
		}
		return func(c context.Context) (carrierSession, error) {
			q, e := l.Accept(c)
			if e != nil {
				return nil, e
			}
			return quicCarrier{q}, nil
		}, func() { l.Close() }, nil
	case config.WSS, config.WSSMUX:
		return listenWSS(ctx, p, token, tlsSettings)
	case config.STEALTH:
		l, e := net.Listen("tcp", bind)
		if e != nil {
			return nil, nil, e
		}
		stop := context.AfterFunc(ctx, func() { l.Close() })
		return func(c context.Context) (carrierSession, error) {
			r, e := l.Accept()
			if e != nil {
				return nil, e
			}
			stop := context.AfterFunc(c, func() { r.Close() })
			defer stop()
			n, e := network.NoiseServerConn(r, token, 10*time.Second)
			if e != nil {
				r.Close()
				return nil, e
			}
			return multiplex(n, true)
		}, func() { stop(); l.Close() }, nil
	}
	return nil, nil, fmt.Errorf("no authorized carrier adapter for %s", p.Transport)
}
func dialCarrier(ctx context.Context, a config.AutoConfig, p config.AutoCandidate, token string) (carrierSession, error) {
	switch p.Transport {
	case config.WATICMP:
		w, e := waticmp.Connect(ctx, false, watSettings(a, p, token, false))
		if e != nil {
			return nil, e
		}
		return watCarrier{w}, nil
	case config.QUIC:
		q, e := network.QUICDial(ctx, p.Endpoint, network.QUICSettings{IncomingStreamLimit: int64(Defaults(a).MaxConnections + 16), DialTimeout: 10 * time.Second, KeepAlivePeriod: 5 * time.Second, MaxIdleTimeout: 15 * time.Second, SO_RCVBUF: 1 << 20, SO_SNDBUF: 1 << 20})
		if e != nil {
			return nil, e
		}
		return quicCarrier{q}, nil
	case config.WSS, config.WSSMUX:
		w, e := network.WebSocketDialer(ctx, nil, p.Endpoint, "", "/peer-auto/v1", 10*time.Second, 5*time.Second, true, token, config.WSS, false, 1, 0, 0, 0)
		if e != nil {
			return nil, e
		}
		binding, e := network.WSSConnProof(w.UnderlyingConn(), token)
		if e != nil {
			w.Close()
			return nil, e
		}
		c, e := multiplex(newWSConn(w), false)
		if e != nil {
			return nil, e
		}
		return boundCarrier{carrierSession: c, binding: binding}, nil
	case config.STEALTH:
		r, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", p.Endpoint)
		if e != nil {
			return nil, e
		}
		stop := context.AfterFunc(ctx, func() { r.Close() })
		defer stop()
		n, e := network.NoiseClientConn(r, token, 10*time.Second)
		if e != nil {
			r.Close()
			return nil, e
		}
		return multiplex(n, false)
	}
	return nil, fmt.Errorf("no authorized carrier adapter for %s", p.Transport)
}
