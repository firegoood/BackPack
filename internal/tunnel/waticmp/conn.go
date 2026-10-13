// Package waticmp adapts the attributed WatTunnel Session/Stream engine.
// The native enrollment envelope is intentionally not the standalone wattun wire.
package waticmp

import (
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	wat "github.com/backpack/backpack/internal/tunnel/waticmp/engine"
	"golang.org/x/time/rate"
)

const (
	headerSize      = 23
	helloSize       = headerSize + 32 + 32 + 32 + 32
	protocolVersion = 1
	msgHello        = 1
	msgAck          = 2
	msgData         = 3
	msgMTUProbe     = 4
	msgMTUAck       = 5
)

// Settings is explicitly opt-in. PeerIP is a route hint; identity is proved
// by the unique enrollment key and TunnelID, not by the source address.
type Settings struct {
	Enabled          bool
	PeerIP           string
	TunnelID         string
	Direction        string // normal: Iran requests; reverse: Kharej requests
	Token            string
	Paths            int
	MaxStreams       int
	MaxPayload       int
	MaxBps           int
	HandshakeTimeout time.Duration
}

func (s Settings) Validate() error {
	if !s.Enabled || os.Getenv("BACKPACK_DISABLE_WATICMP") == "1" {
		return errors.New("Wat ICMP is disabled; explicit per-tunnel enable is required")
	}
	ip := net.ParseIP(s.PeerIP)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return errors.New("Wat ICMP needs an explicit unicast IPv4 peer address")
	}
	if len(s.Token) < 32 || len(s.TunnelID) < 16 || len(s.TunnelID) > 64 {
		return errors.New("Wat ICMP requires a unique tunnel ID and an enrollment secret of at least 32 bytes")
	}
	if s.Direction != "normal" && s.Direction != "reverse" {
		return errors.New("Wat ICMP direction must be normal (Iran requests) or reverse (Kharej requests)")
	}
	if s.Paths < 0 || s.Paths > 8 || s.MaxStreams < 0 || s.MaxStreams > 256 {
		return errors.New("Wat ICMP supports 1-8 lanes and at most 256 streams")
	}
	if s.MaxPayload != 0 && (s.MaxPayload < 256 || s.MaxPayload > 1400) {
		return errors.New("Wat ICMP payload must be 256-1400 bytes; increases require authenticated PMTU confirmation")
	}
	if s.MaxBps < 0 || s.MaxBps > 4_000_000 {
		return errors.New("Wat ICMP rate must be 0-4000000 bytes/s")
	}
	if s.HandshakeTimeout < 0 || s.HandshakeTimeout > time.Minute {
		return errors.New("Wat ICMP handshake deadline must not exceed one minute")
	}
	return nil
}

type packet struct {
	data    [1600]byte
	n, lane int
	src     net.Addr
}
type handshake struct{ data [helloSize]byte }
type confirmation struct {
	nonce, pub, peerNonce, peerPub [32]byte
	until                          time.Time
}

type packetIO struct {
	raw                     *wat.ICMPIO
	peer                    net.IP
	role, direction         byte
	header                  [headerSize]byte
	key                     [32]byte
	stop                    chan struct{}
	once                    sync.Once
	wg                      sync.WaitGroup
	hello                   chan handshake
	data                    chan *packet
	free                    chan *packet
	previous                *packet
	dataBudget, helloBudget *rate.Limiter
	ackBudget               *rate.Limiter
	confirmation            atomic.Pointer[confirmation]
	mtuKey                  atomic.Pointer[[32]byte]
	mtuAcks                 chan mtuAck
	mtuBudget               *rate.Limiter
	receiveLimit            int
	sendLimit               atomic.Int64
}

func mac(key []byte, parts ...[]byte) []byte {
	h := hmac.New(sha256.New, key)
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func openIO(server bool, s Settings) (*packetIO, error) {
	role := byte(2)
	if server {
		role = 1
	}
	direction := byte(0)
	if s.Direction == "reverse" {
		direction = 1
	}
	key := mac([]byte(s.Token), []byte("github.com/backpack/backpack/waticmp/v1/enrollment:"), []byte(s.TunnelID))
	// Wat session server is Iran. Wat's raw reverse flag therefore equals
	// the user-facing NORMAL direction, not its reverse direction.
	raw, err := wat.NewICMPIOReverse(server, s.Direction == "normal", s.PeerIP, key, "echo")
	if err != nil {
		return nil, fmt.Errorf("Wat ICMP raw socket (Linux CAP_NET_RAW or root required): %w", err)
	}
	i := &packetIO{raw: raw, peer: net.ParseIP(s.PeerIP).To4(), role: role, direction: direction,
		stop: make(chan struct{}), hello: make(chan handshake, 8), data: make(chan *packet, 128), free: make(chan *packet, 130),
		dataBudget: rate.NewLimiter(12_000, 256), helloBudget: rate.NewLimiter(8, 8), ackBudget: rate.NewLimiter(2, 1)}
	i.mtuAcks, i.mtuBudget = make(chan mtuAck, 16), rate.NewLimiter(16, 8)
	i.receiveLimit = s.MaxPayload
	if i.receiveLimit == 0 {
		i.receiveLimit = 512
	}
	i.sendLimit.Store(256)
	copy(i.key[:], key)
	copy(i.header[:4], []byte("FWIC"))
	copy(i.header[4:20], mac(key, []byte("demux:"))[:16])
	i.header[20], i.header[21], i.header[22] = protocolVersion, role, direction
	for n := 0; n < cap(i.free); n++ {
		i.free <- new(packet)
	}
	i.wg.Add(1)
	go func() { defer i.wg.Done(); i.read() }()
	return i, nil
}

func (i *packetIO) read() {
	defer i.once.Do(func() { close(i.stop); i.raw.Close() })
	for {
		p, lane, src, err := i.raw.Recv()
		if err != nil {
			return
		}
		ip, ok := src.(*net.IPAddr)
		if !ok || !ip.IP.Equal(i.peer) || len(p) < headerSize+1 || len(p) > 1600 {
			continue
		}
		if !hmac.Equal(p[:20], i.header[:20]) || p[20] != protocolVersion || p[21] == i.role || p[21] != 3-i.role || p[22] != i.direction {
			continue
		}
		body := p[headerSize:]
		switch body[0] {
		case msgHello, msgAck:
			if len(p) != helloSize+1 || !i.helloBudget.Allow() || !hmac.Equal(mac(i.key[:], p[:len(p)-32]), p[len(p)-32:]) {
				continue
			}
			var h handshake
			// Remove the message discriminator; the fixed struct keeps the
			// sender header and complete authenticated challenge material.
			copy(h.data[:headerSize], p[:headerSize])
			copy(h.data[headerSize:], p[headerSize+1:])
			if c := i.confirmation.Load(); c != nil && time.Now().Before(c.until) &&
				hmac.Equal(h.data[headerSize:headerSize+32], c.peerNonce[:]) &&
				hmac.Equal(h.data[headerSize+32:headerSize+64], c.peerPub[:]) && i.ackBudget.Allow() {
				_ = i.sendChallenge(msgAck, c.nonce[:], c.pub[:], c.peerNonce[:])
			}
			select {
			case i.hello <- h:
			default:
			}
		case msgData:
			if !i.dataBudget.Allow() || len(body)-1 > i.receiveLimit {
				continue
			}
			var b *packet
			select {
			case b = <-i.free:
			default:
				continue
			}
			b.n = copy(b.data[:], body[1:])
			b.lane = lane
			b.src = src
			select {
			case i.data <- b:
			default:
				i.free <- b
			}
		case msgMTUProbe, msgMTUAck:
			i.receiveMTU(p)
		}
	}
}

func (i *packetIO) sendChallenge(kind byte, nonce, pub, echo []byte) error {
	p := make([]byte, 0, helloSize+1)
	p = append(p, i.header[:]...)
	p = append(p, kind)
	p = append(p, nonce...)
	p = append(p, pub...)
	p = append(p, echo...)
	p = append(p, mac(i.key[:], p)...)
	return i.raw.Send(p, 0)
}

func (i *packetIO) Send(p []byte, lane int) error {
	if len(p) == 0 || int64(len(p)) > i.sendLimit.Load() {
		return errors.New("Wat ICMP datagram outside the approved size budget")
	}
	b := make([]byte, 0, headerSize+1+len(p))
	b = append(b, i.header[:]...)
	b = append(b, msgData)
	b = append(b, p...)
	return i.raw.Send(b, lane)
}
func (i *packetIO) Recv() ([]byte, int, net.Addr, error) {
	if i.previous != nil {
		i.free <- i.previous
		i.previous = nil
	}
	select {
	case <-i.stop:
		return nil, 0, nil, net.ErrClosed
	case b := <-i.data:
		i.previous = b
		return b.data[:b.n], b.lane, b.src, nil
	}
}
func (i *packetIO) SetPeer(net.Addr)   {} // enrollment fixes the allowed destination
func (i *packetIO) CopiesOnSend() bool { return true }
func (i *packetIO) Close() error {
	i.once.Do(func() { close(i.stop); i.raw.Close() })
	i.wg.Wait()
	return nil
}

// Conn carries one enrolled peer's reliable Wat streams. New cryptographic
// key material is created on every Connect, never loaded from disk snapshots.
type Conn struct {
	session    *wat.Session
	io         *packetIO
	binding    [32]byte
	once       sync.Once
	stopCancel func() bool
}

func Connect(parent context.Context, server bool, s Settings) (*Conn, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Paths == 0 {
		s.Paths = 4
	}
	if s.MaxPayload == 0 {
		s.MaxPayload = 512
	}
	if s.MaxBps == 0 {
		s.MaxBps = 1_000_000
	}
	if s.HandshakeTimeout == 0 {
		s.HandshakeTimeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, s.HandshakeTimeout)
	defer cancel()
	i, err := openIO(server, s)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			i.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { i.Close() })
	defer stop()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	pub := priv.PublicKey().Bytes()
	tk := time.NewTicker(500 * time.Millisecond)
	defer tk.Stop()
	var peerNonce, peerPub [32]byte
	havePeer := false
	send := func() {
		kind := byte(msgHello)
		var echo [32]byte
		if havePeer {
			kind = msgAck
			echo = peerNonce
		}
		_ = i.sendChallenge(kind, nonce[:], pub, echo[:])
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Wat ICMP peer authentication timed out or cancelled (both peers need native protocol v1 and matching identity/direction): %w", ctx.Err())
		case <-tk.C:
			send()
		case h := <-i.hello:
			body := h.data[headerSize:]
			// Reject a challenge to a previous local incarnation. A HELLO
			// can only elicit an ACK; it cannot admit a production session.
			var offered [32]byte
			copy(offered[:], body[:32])
			if offered == nonce {
				continue
			}
			copy(peerNonce[:], body[:32])
			copy(peerPub[:], body[32:64])
			havePeer = true
			send()
			if !hmac.Equal(body[64:96], nonce[:]) {
				continue
			}
			pk, err := ecdh.X25519().NewPublicKey(peerPub[:])
			if err != nil {
				return nil, err
			}
			shared, err := priv.ECDH(pk)
			if err != nil {
				return nil, errors.New("Wat ICMP rejected an invalid peer key")
			}
			first, second := nonce[:], peerNonce[:]
			p1, p2 := pub, peerPub[:]
			if !server {
				first, second = second, first
				p1, p2 = p2, p1
			}
			binding := mac(i.key[:], []byte("session:"), first, second, p1, p2, shared)
			sid := binary.BigEndian.Uint64(binding[:8]) | 1
			confirmation := &confirmation{nonce: nonce, peerNonce: peerNonce, peerPub: peerPub, until: time.Now().Add(s.HandshakeTimeout)}
			copy(confirmation.pub[:], pub)
			i.confirmation.Store(confirmation)
			var mtuKey [32]byte
			copy(mtuKey[:], mac(binding, []byte("pmtu:")))
			i.mtuKey.Store(&mtuKey)
			s.MaxPayload, err = i.probeMTU(ctx, s.MaxPayload)
			if err != nil {
				return nil, err
			}
			i.sendLimit.Store(int64(s.MaxPayload))
			if !stop() && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			engine, err := wat.NewSession(wat.Config{Server: server, Key: binding, SID: sid, Paths: s.Paths,
				MaxPayload: s.MaxPayload, MaxStreams: s.MaxStreams, InitBps: 200_000, MinBps: 32_000, MaxBps: float64(s.MaxBps)}, i)
			if err != nil {
				return nil, err
			}
			c := &Conn{session: engine, io: i}
			copy(c.binding[:], binding)
			c.stopCancel = context.AfterFunc(parent, engine.Close)
			ok = true
			return c, nil
		}
	}
}

func (c *Conn) RemoteAddr() net.Addr { return &net.IPAddr{IP: c.io.peer} }
func (c *Conn) LocalAddr() net.Addr  { return &net.IPAddr{IP: net.IPv4zero} }
func (c *Conn) CloseWithError(_ uint64, _ string) error {
	c.once.Do(func() {
		if c.stopCancel != nil {
			c.stopCancel()
		}
		c.session.Close()
	})
	return nil
}
func (c *Conn) OpenStreamSync(ctx context.Context) (*Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &Stream{Stream: st, conn: c}, nil
}
func (c *Conn) AcceptStream(ctx context.Context) (*Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { c.CloseWithError(0, "accept cancelled") })
	defer stop()
	st, err := c.session.Accept()
	if err != nil {
		return nil, err
	}
	return &Stream{Stream: st, conn: c}, nil
}
func (c *Conn) Proof(role string) string {
	return base64.RawURLEncoding.EncodeToString(mac(c.binding[:], []byte("control-proof:"), []byte(role)))
}
func (c *Conn) Stats() wat.Stats { return c.session.Stats() }

// Diagnostic contains counters and the last authenticated packet-size proof,
// never keys, enrollment secrets, handshake nonces or control tickets.
func (c *Conn) Diagnostic(direction string) any {
	return struct {
		Direction string    `json:"direction"`
		Payload   int64     `json:"authenticated_payload"`
		Engine    wat.Stats `json:"engine"`
	}{direction, c.io.sendLimit.Load(), c.Stats()}
}

type Stream struct {
	*wat.Stream
	conn *Conn
}

func (s *Stream) Close() error         { return s.Stream.Close() }
func (s *Stream) LocalAddr() net.Addr  { return s.conn.LocalAddr() }
func (s *Stream) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }
func (s *Stream) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}
	return s.SetWriteDeadline(t)
}

var _ net.Conn = (*Stream)(nil)
