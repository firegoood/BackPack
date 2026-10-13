package peerauto

import (
	"context"
	"crypto/hmac"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/utils/network"
	"github.com/gorilla/websocket"
)

type boundCarrier struct {
	carrierSession
	binding string
}

func (c boundCarrier) Binding(string) (string, error) { return c.binding, nil }

// Framed WebSocket byte stream; the new version does not take raw bytes out
// from under the HTTP upgrade. The existing legacy WSS engines are untouched.
type wsConn struct {
	ws              *websocket.Conn
	reader          io.Reader
	readMu, writeMu sync.Mutex
}

func newWSConn(w *websocket.Conn) *wsConn { w.SetReadLimit(128 << 10); return &wsConn{ws: w} }
func (c *wsConn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	for {
		if c.reader == nil {
			kind, r, e := c.ws.NextReader()
			if e != nil {
				return 0, e
			}
			if kind != websocket.BinaryMessage {
				return 0, errors.New("expected binary peer stream")
			}
			c.reader = r
		}
		n, e := c.reader.Read(b)
		if e == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, e
	}
}
func (c *wsConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if e := c.ws.WriteMessage(websocket.BinaryMessage, b); e != nil {
		return 0, e
	}
	return len(b), nil
}
func (c *wsConn) Close() error         { return c.ws.Close() }
func (c *wsConn) LocalAddr() net.Addr  { return c.ws.LocalAddr() }
func (c *wsConn) RemoteAddr() net.Addr { return c.ws.RemoteAddr() }
func (c *wsConn) SetDeadline(t time.Time) error {
	if e := c.SetReadDeadline(t); e != nil {
		return e
	}
	return c.SetWriteDeadline(t)
}
func (c *wsConn) SetReadDeadline(t time.Time) error  { return c.ws.SetReadDeadline(t) }
func (c *wsConn) SetWriteDeadline(t time.Time) error { return c.ws.SetWriteDeadline(t) }

type limitedListener struct {
	net.Listener
	slots chan struct{}
}
type limitedConn struct {
	net.Conn
	once sync.Once
	free func()
}

func (c *limitedConn) Close() error { e := c.Conn.Close(); c.once.Do(c.free); return e }
func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		c, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		select {
		case l.slots <- struct{}{}:
			return &limitedConn{Conn: c, free: func() { <-l.slots }}, nil
		default:
			c.Close()
		}
	}
}

func listenWSS(ctx context.Context, p config.AutoCandidate, token string, settings network.TLSSettings) (acceptCarrier, func(), error) {
	settings.SelfSignedHost, _, _ = net.SplitHostPort(p.Endpoint)
	cfg, e := network.ServerTLSConfig(settings, func(string, ...any) {})
	if e != nil {
		return nil, nil, e
	}
	bind := p.LocalBind
	if bind == "" {
		bind = p.Endpoint
	}
	raw, e := net.Listen("tcp", bind)
	if e != nil {
		return nil, nil, e
	}
	listener := tls.NewListener(&limitedListener{Listener: raw, slots: make(chan struct{}, 8)}, cfg)
	accepted := make(chan carrierSession, 2)
	done := make(chan struct{})
	var handoff sync.Mutex
	closed := false
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 32768, CheckOrigin: func(*http.Request) bool { return true }}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/peer-auto/v1/") || r.TLS == nil {
			http.NotFound(w, r)
			return
		}
		proof, e := network.WSSServerProof(r.TLS, token)
		if e != nil || !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+proof)) {
			http.NotFound(w, r)
			return
		}
		answer, e := network.WSSServerAnswer(r.TLS, token)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		ws, e := upgrader.Upgrade(w, r, http.Header{network.WSSServerProofHeader: []string{answer}})
		if e != nil {
			return
		}
		s, e := multiplex(newWSConn(ws), true)
		if e != nil {
			return
		}
		session := boundCarrier{carrierSession: s, binding: proof}
		handoff.Lock()
		defer handoff.Unlock()
		if closed || ctx.Err() != nil {
			session.Close()
			return
		}
		select {
		case accepted <- session:
		case <-ctx.Done():
			session.Close()
		default:
			session.Close()
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
	go func() { defer close(done); _ = server.Serve(listener) }()
	var once sync.Once
	closeServer := func() {
		once.Do(func() {
			handoff.Lock()
			closed = true
			handoff.Unlock()
			server.Close()
			listener.Close()
			<-done
			for {
				select {
				case s := <-accepted:
					s.Close()
				default:
					return
				}
			}
		})
	}
	stop := context.AfterFunc(ctx, closeServer)
	return func(c context.Context) (carrierSession, error) {
		select {
		case s := <-accepted:
			return s, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.Done():
			return nil, c.Err()
		case <-done:
			return nil, errors.New("WSS endpoint stopped")
		}
	}, func() { stop(); closeServer() }, nil
}
