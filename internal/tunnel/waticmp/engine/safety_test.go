package wat

import (
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"
)

func TestUnauthenticatedOrReplayedPacketCannotAllocateStreams(t *testing.T) {
	s := memorySession()
	s.sid, s.replayWin = 17, replayWin
	s.rxRing = make([]uint64, replayWin)
	var err error
	s.aead, err = sessionAEAD(make([]byte, 32), s.sid)
	if err != nil {
		t.Fatal(err)
	}
	packet := func(pn uint64) []byte {
		header := make([]byte, hdrLen)
		binary.BigEndian.PutUint64(header, s.sid)
		binary.BigEndian.PutUint64(header[8:], pn)
		var nonce [12]byte
		binary.BigEndian.PutUint64(nonce[4:], pn)
		return s.aead.Seal(header, nonce[:], []byte{frPing}, header[:8])
	}
	good := packet(dirServerBit + 1)
	s.handle(good, 0, nil)
	s.handle(good, 0, nil)
	bad := packet(dirServerBit + 2)
	bad[len(bad)-1] ^= 1
	s.handle(bad, 0, nil)
	s.handle(packet(3), 0, nil) // reflected local direction
	if s.stats.PktsIn != 1 || s.stats.ReplayDropped != 1 || s.stats.AuthDropped != 1 || len(s.streams) != 0 {
		t.Fatalf("packet isolation violated: %+v streams=%d", s.stats, len(s.streams))
	}
}

func TestPumpSnapshotBeforeReceiveDoesNotOverflowAckDelay(t *testing.T) {
	s := memorySession()
	s.sid, s.nextPN, s.maxPlain = 1, 1, 480
	s.lanes, s.ackMax = []int{0}, 64
	s.rxRanges, s.ackPend = rangeSet{{0, 1}}, 6
	s.rxAt = s.now.Add(time.Microsecond)
	var err error
	s.aead, err = sessionAEAD(make([]byte, 32), s.sid)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.build(s.now)
	if !ok {
		t.Fatal("no ACK emitted")
	}
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[4:], 1)
	plain, err := s.aead.Open(nil, nonce[:], p.p[hdrLen:], p.p[:8])
	if err != nil {
		t.Fatal(err)
	}
	r := &reader{b: plain}
	if r.byte() != frAck {
		t.Fatal("expected ACK")
	}
	_, delay, _ := parseAck(r)
	if r.err != nil || delay != 0 {
		t.Fatalf("clock snapshot generated invalid ACK delay: %d %v", delay, r.err)
	}
}

func memorySession() *Session {
	return &Session{cfg: Config{MaxStreams: 2}, streams: map[uint64]*Stream{}, tomb: map[uint64]time.Time{},
		now: time.Now(), wake: make(chan struct{}, 1)}
}

func TestFirstDelayedAckCanTeachTheSessionItsRTT(t *testing.T) {
	s := memorySession()
	s.cc = newCC(200000, 32000, 256000, 8*time.Millisecond)
	s.lastSend = s.now
	s.sent = []*sentPkt{{pn: 1, at: s.now, size: 100}}
	s.inflight = 100
	arrival := s.now.Add(360 * time.Millisecond)
	s.timers(arrival)
	if s.stats.Retrans != 0 || len(s.sent) != 1 || s.sent[0].done {
		t.Fatal("first packet was retired before a delayed initial ACK could arrive")
	}
	s.onAck(1, 0, rangeSet{{1, 2}}, arrival)
	if s.cc.srtt != 360*time.Millisecond || s.inflight != 0 {
		t.Fatalf("initial RTT was not learned: rtt=%s inflight=%d", s.cc.srtt, s.inflight)
	}
	if s.cc.lossTime() <= s.cc.srtt || s.cc.pto() <= s.cc.srtt {
		t.Fatal("learned RTT does not bound loss and tail-repair timers")
	}
}

func TestCallerPacingCeilingBoundsStartupAndFloor(t *testing.T) {
	c := newCC(200000, 32000, 8000, 8*time.Millisecond)
	if c.rate > 8000 || c.minRate > 8000 || c.floor() > 8000 {
		t.Fatalf("low explicit ceiling was exceeded: %+v", c)
	}
	c.refill(time.Second)
	if c.tokens > 8000*0.012+4000 {
		t.Fatal("pacer exceeded its bounded initial token burst")
	}
	for n := 0; n < 20; n++ {
		at := time.Now().Add(time.Duration(n) * time.Second)
		c.sample(360*time.Millisecond, at)
		c.limited = true
		c.tick(at)
		if c.rate > 8000 {
			t.Fatalf("growth exceeded explicit ceiling: %f", c.rate)
		}
	}
}

func TestLateAckLearnsAChangedRTTAfterPacketRetirement(t *testing.T) {
	s := memorySession()
	s.cc = newCC(200000, 32000, 256000, 8*time.Millisecond)
	s.cc.sample(20*time.Millisecond, s.now)
	s.lastSend = s.now
	s.sent = []*sentPkt{{pn: 17, at: s.now, size: 100}}
	s.inflight = 100
	s.timers(s.now.Add(130 * time.Millisecond))
	if len(s.sent)-s.sentHead != 0 || s.inflight != 0 {
		t.Fatal("test packet was not retired by the original short RTT timer")
	}
	s.onAck(17, 0, rangeSet{{17, 18}}, s.now.Add(360*time.Millisecond))
	if s.cc.latest != 360*time.Millisecond || s.cc.pto() <= 360*time.Millisecond {
		t.Fatalf("late authenticated ACK did not adapt timers: latest=%s pto=%s", s.cc.latest, s.cc.pto())
	}
	before := s.cc.srtt
	s.onAck(17, 0, rangeSet{{17, 18}}, s.now.Add(time.Second))
	if s.cc.srtt != before {
		t.Fatal("repeated ACK reused consumed timing metadata")
	}
}
func TestWindowRetirementMustNotUnderflow(t *testing.T) {
	s := memorySession()
	s.winExtra = 4096
	st := s.newStream(1)
	st.win = streamWindow + 4096
	s.killStream(st, ErrReset)
	if s.winExtra != 0 {
		t.Fatalf("budget after retirement: %d", s.winExtra)
	}
	s.killStream(st, ErrReset)
	if s.winExtra != 0 {
		t.Fatal("duplicate retirement changed budget")
	}
}
func TestReadAndBlockedWriteDeadlines(t *testing.T) {
	s := memorySession()
	st := s.newStream(1)
	st.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, e := st.Read(make([]byte, 1)); !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatalf("read deadline: %v", e)
	}
	st.SetReadDeadline(time.Time{})
	st.sbuf = make([]byte, st.writeCap())
	st.SetWriteDeadline(time.Now().Add(20 * time.Millisecond))
	if _, e := st.Write([]byte{1}); !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatalf("write deadline: %v", e)
	}
	st.Reset()
}
func TestMalformedOffsetsAndAckOverflow(t *testing.T) {
	s := memorySession()
	s.onStream(1, ^uint64(0), []byte{1}, true)
	if len(s.streams) != 0 {
		t.Fatal("overflowing offset allocated a stream")
	}
	b := appendUv(nil, ^uint64(0))
	b = appendUv(b, 0)
	b = appendUv(b, 1)
	b = appendUv(b, 1)
	r := &reader{b: b}
	parseAck(r)
	if r.err == nil {
		t.Fatal("overflowing largest ACK accepted")
	}
}
func FuzzAckDecoder(f *testing.F) {
	f.Add([]byte{0, 0, 1, 1})
	f.Add([]byte{255, 255, 255})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2048 {
			return
		}
		r := &reader{b: b}
		_, _, rs := parseAck(r)
		if len(rs) > 64 {
			t.Fatal("unbounded ACK ranges")
		}
	})
}
