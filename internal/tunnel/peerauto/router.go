package peerauto

import (
	"context"
	"crypto/hmac"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/backpack/backpack/internal/metrics"
	"github.com/backpack/backpack/internal/utils/handlers"
	"github.com/backpack/backpack/internal/utils/network"
)

const requestSize = 8 + 2 + 1 + 32

func requestHeader(r *route, slot int, udp bool) []byte {
	b := make([]byte, requestSize)
	binary.BigEndian.PutUint64(b, r.generation)
	binary.BigEndian.PutUint16(b[8:], uint16(slot))
	if udp {
		b[10] = 1
	}
	copy(b[11:], keyed(r.endpoint.w.key, "github.com/backpack/backpack/peer-auto/v1/data:", r.ticket, string(b[:11])))
	return b
}
func (m *Manager) takeSlot(ctx context.Context) bool {
	select {
	case m.connections <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	default:
		return false
	}
}
func (m *Manager) freeSlot() { <-m.connections }

func (m *Manager) trackFlow(r *route) func() {
	m.flowAdmitted.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			if r.ctx.Err() != nil {
				m.flowInterrupted.Add(1)
			} else {
				m.flowCompleted.Add(1)
				if r.retired.Load() {
					m.flowDrained.Add(1)
				}
			}
		})
	}
}

func (m *Manager) acceptData(e *candidate) {
	for e.ctx.Err() == nil {
		c, err := e.session.Accept(e.ctx)
		if err != nil {
			e.cancel()
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var kind [1]byte
		_, err = io.ReadFull(c, kind[:])
		_ = c.SetReadDeadline(time.Time{})
		if err != nil || kind[0] != 1 {
			c.Close()
			continue
		}
		select {
		case e.data <- c:
		case <-e.ctx.Done():
			c.Close()
			return
		}
	}
}
func (m *Manager) openData(e *candidate) {
	var wg sync.WaitGroup
	defer wg.Wait()
	consumed := make(chan struct{}, m.policy.MaxConnections+4)
	permits := make(chan struct{}, m.policy.MaxConnections+4)
	idle := 0
	for e.ctx.Err() == nil {
		if idle >= 4 {
			select {
			case <-e.ctx.Done():
				return
			case <-consumed:
				idle--
			}
			continue
		}
		select {
		case permits <- struct{}{}:
		case <-e.ctx.Done():
			return
		}
		c, err := e.session.Open(e.ctx)
		if err != nil {
			<-permits
			if !pause(e.ctx, 250*time.Millisecond) {
				return
			}
			continue
		}
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = c.Write([]byte{1})
		_ = c.SetWriteDeadline(time.Time{})
		if err != nil {
			c.Close()
			<-permits
			e.cancel()
			return
		}
		idle++
		wg.Add(1)
		go func() { defer wg.Done(); defer func() { <-permits }(); defer c.Close(); m.serveData(e, c, consumed) }()
	}
}
func (m *Manager) serveData(e *candidate, c net.Conn, consumed chan<- struct{}) {
	var b [requestSize]byte
	_, err := io.ReadFull(c, b[:])
	select {
	case consumed <- struct{}{}:
	case <-e.ctx.Done():
		return
	}
	if err != nil {
		return
	}
	m.mu.Lock()
	r := m.active
	m.mu.Unlock()
	slot := int(binary.BigEndian.Uint16(b[8:10]))
	udp := b[10] == 1
	if r == nil || r.endpoint != e || r.generation != binary.BigEndian.Uint64(b[:8]) || slot >= len(m.mappings) || b[10] > 1 || udp && !m.policy.AcceptUDP || !hmac.Equal(b[:], requestHeader(r, slot, udp)) {
		m.flowRejected.Add(1)
		return
	}
	if !m.takeSlot(e.ctx) {
		m.flowRejected.Add(1)
		return
	}
	defer m.freeSlot()
	proto := "tcp"
	if udp {
		proto = "udp"
	}
	var backend net.Conn
	lastTarget := ""
	for _, target := range m.mappings[slot].Targets {
		lastTarget = target
		backend, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(e.ctx, proto, target)
		if err == nil {
			break
		}
	}
	if err != nil || backend == nil {
		// Local application failure is distinct from carrier/control health.
		_, _ = c.Write([]byte{0})
		why := "unreachable"
		if errors.Is(err, syscall.ECONNREFUSED) {
			why = "refused"
		} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			why = "timeout"
		}
		metrics.ReportLocalDialFailure(lastTarget, why)
		return
	}
	metrics.ReportLocalDialSuccess()
	defer backend.Close()
	if r.ctx.Err() != nil {
		return
	}
	if _, err = c.Write([]byte{1}); err != nil {
		return
	}
	defer m.trackFlow(r)()
	if udp {
		relayDatagrams(r.ctx, c, backend)
	} else {
		handlers.TCPConnectionHandler(r.ctx, false, metrics.CountedConn(c), backend, m.log, nil, 0, false)
	}
}
func relayDatagrams(ctx context.Context, stream, backend net.Conn) {
	stop := context.AfterFunc(ctx, func() { stream.Close(); backend.Close() })
	defer stop()
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, network.MaxDatagram)
		for {
			_ = stream.SetReadDeadline(time.Now().Add(60 * time.Second))
			n, e := network.ReadDatagram(stream, buf)
			if e != nil {
				return
			}
			metrics.AddBytes(uint64(n), 0)
			_ = backend.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, e = backend.Write(buf[:n]); e != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, network.MaxDatagram)
		for {
			_ = backend.SetReadDeadline(time.Now().Add(60 * time.Second))
			n, e := backend.Read(buf)
			if e != nil {
				return
			}
			_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if e = network.WriteDatagram(stream, buf[:n]); e != nil {
				return
			}
			metrics.AddBytes(0, uint64(n))
		}
	}()
	<-done
	stream.Close()
	backend.Close()
	<-done
}

func (m *Manager) claim(ctx context.Context, slot int, udp bool) (net.Conn, error) {
	m.mu.Lock()
	r := m.active
	m.mu.Unlock()
	if r == nil {
		return nil, errors.New("NO_ROUTE: no committed peer route")
	}
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	var c net.Conn
	select {
	case c = <-r.endpoint.data:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.endpoint.ctx.Done():
		return nil, errors.New("peer session unavailable")
	case <-t.C:
		return nil, errors.New("peer stream pool exhausted")
	}
	m.mu.Lock()
	same := m.active == r
	m.mu.Unlock()
	if !same {
		c.Close()
		return nil, errors.New("route changed before stream admission")
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	defer c.SetDeadline(time.Time{})
	if _, err := io.Copy(c, bytesReader(requestHeader(r, slot, udp))); err != nil {
		c.Close()
		return nil, err
	}
	var ack [1]byte
	if _, err := io.ReadFull(c, ack[:]); err != nil {
		c.Close()
		return nil, err
	}
	if ack[0] != 1 {
		c.Close()
		return nil, errors.New("peer backend unavailable (carrier remains healthy)")
	}
	stop := context.AfterFunc(r.ctx, func() { c.Close() })
	b := &boundConn{Conn: c, stop: stop, finish: m.trackFlow(r)}
	if h, ok := c.(interface{ CloseWrite() error }); ok {
		return &halfBoundConn{boundConn: b, half: h}, nil
	}
	return b, nil
}

type boundConn struct {
	net.Conn
	stop   func() bool
	finish func()
}

func (c *boundConn) Close() error { c.stop(); err := c.Conn.Close(); c.finish(); return err }

type halfBoundConn struct {
	*boundConn
	half interface{ CloseWrite() error }
}

func (c *halfBoundConn) CloseWrite() error { return c.half.CloseWrite() }

// Avoid a wrapper around the actual user stream; CloseWrite remains visible.
type headerReader struct{ b []byte }

func bytesReader(b []byte) *headerReader { return &headerReader{b} }
func (r *headerReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

// The Iran owner binds user ports once. Probe/candidate endpoints never bind
// them and selection changes only which fresh streams receive new users.
func (m *Manager) bindUsers() error {
	var listeners []io.Closer
	fail := func(e error) error {
		for _, l := range listeners {
			l.Close()
		}
		return e
	}
	for slot, mapping := range m.mappings {
		l, e := net.Listen("tcp", mapping.Listen)
		if e != nil {
			return fail(e)
		}
		listeners = append(listeners, l)
		slot := slot
		m.launch(func() {
			stop := context.AfterFunc(m.ctx, func() { l.Close() })
			defer stop()
			defer l.Close()
			for {
				c, e := l.Accept()
				if e != nil {
					return
				}
				if !m.takeSlot(m.ctx) {
					m.flowRejected.Add(1)
					c.Close()
					continue
				}
				m.launch(func() {
					defer m.freeSlot()
					defer c.Close()
					s, e := m.claim(m.ctx, slot, false)
					if e != nil {
						m.flowRejected.Add(1)
						return
					}
					defer s.Close()
					handlers.TCPConnectionHandler(m.ctx, false, c, metrics.CountedConn(s), m.log, nil, 0, false)
				})
			}
		})
		if m.policy.AcceptUDP {
			pc, e := net.ListenPacket("udp", mapping.Listen)
			if e != nil {
				return fail(e)
			}
			listeners = append(listeners, pc)
			m.launch(func() { m.serveUDP(pc, slot) })
		}
	}
	return nil
}

type udpSource struct {
	q    chan []byte
	done chan struct{}
}

func (m *Manager) serveUDP(pc net.PacketConn, slot int) {
	stop := context.AfterFunc(m.ctx, func() { pc.Close() })
	defer stop()
	defer pc.Close()
	var mu sync.Mutex
	sources := map[string]*udpSource{}
	buf := make([]byte, network.MaxDatagram)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		key := addr.String()
		mu.Lock()
		f := sources[key]
		if f == nil && m.takeSlot(m.ctx) {
			f = &udpSource{q: make(chan []byte, 4), done: make(chan struct{})}
			sources[key] = f
			flow := f
			peer := addr
			m.launch(func() {
				defer m.freeSlot()
				defer close(flow.done)
				defer func() { mu.Lock(); delete(sources, key); mu.Unlock() }()
				m.udpSource(pc, peer, slot, flow)
			})
		}
		enqueued := false
		if f != nil && len(f.q) < cap(f.q) {
			select {
			case <-f.done:
			case f.q <- append([]byte(nil), buf[:n]...):
				enqueued = true
			default:
			}
		}
		if !enqueued {
			m.udpQueueDrops.Add(1)
		}
		mu.Unlock()
	}
}
func (m *Manager) udpSource(pc net.PacketConn, peer net.Addr, slot int, f *udpSource) {
	c, err := m.claim(m.ctx, slot, true)
	if err != nil {
		return
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buf := make([]byte, network.MaxDatagram)
		for {
			_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
			n, e := network.ReadDatagram(c, buf)
			if e != nil {
				return
			}
			metrics.AddBytes(uint64(n), 0)
			if _, e = pc.WriteTo(buf[:n], peer); e != nil {
				return
			}
		}
	}()
	defer func() { cancel(); c.Close(); <-done }()
	t := time.NewTimer(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			return
		case b := <-f.q:
			_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if network.WriteDatagram(c, b) != nil {
				return
			}
			metrics.AddBytes(0, uint64(len(b)))
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(60 * time.Second)
		}
	}
}
