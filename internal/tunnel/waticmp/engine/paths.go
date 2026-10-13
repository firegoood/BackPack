// Copyright (c) 2026 Amirreza. SPDX-License-Identifier: MIT
// Adapted for FullPack; see LICENSE and ORIGIN.json in this package.

package wat

import (
	"time"
)

// Multipath (bonding). One session can run over several carriers at once; a "path" is one carrier. A PacketIO
// opts in by implementing the interfaces below, so every single-carrier setup (icmp, udp, tcp, ...) keeps the exact
// behaviour it had before: the session only looks at these when its PacketIO provides them.

// PathFeedback is told by the session how each path performs. OnAck is called for every newly acknowledged
// packet (rtt is a round-trip sample, or 0 when this packet is not the freshest acked packet of its path in this
// ACK, so it would be inflated by waiting); OnLoss when a packet sent on the path is declared lost. Both run
// under the session lock: implementations must be quick and must not call back into the session.
type PathFeedback interface {
	OnAck(path int, rtt time.Duration, bytes int)
	OnLoss(path int)
}

// PathChooser lets the PacketIO decide the path of every packet (called under the session lock, so cheap). The
// returned value is passed to Send as the `path` argument and reported back through PathFeedback. interactive is
// true for packets that carry young streams, datagrams, acks or are small: latency matters more than throughput.
type PathChooser interface {
	NumPaths() int
	PathProfile(path int) Profile
	ChoosePath(size int, interactive bool) int
}

// PathProber is implemented by a PacketIO that wants the session to emit a tiny ack-eliciting packet (PING) on a
// given path from time to time: how a dead path is found alive again. The function may be called from any goroutine.
type PathProber interface{ SetProber(func(path int)) }

// PathStatser exposes per-carrier numbers for /stats.
type PathStatser interface{ PathStats() []CarrierStat }

// CarrierStat is one carrier of a bond, as seen from this end.
type CarrierStat struct {
	Name    string  `json:"name"`
	Alive   bool    `json:"alive"`
	SRTTms  float64 `json:"srtt_ms"`
	Share   float64 `json:"share"`
	Sent    uint64  `json:"sent"`
	Acked   uint64  `json:"acked"`
	Lost    uint64  `json:"lost"`
	Present bool    `json:"-"`
}

// PathTimings tunes health and probing; the zero value is the production setting.
type PathTimings struct {
	Silence    time.Duration // floor of "no acks while sending" before a path is suspect (default 1 s)
	ProbeFirst time.Duration // first probe of a dead path (default 2 s); the gap doubles ...
	ProbeMax   time.Duration // ... up to this (default 30 s)
	Ramp       time.Duration // a revived path goes from a 10 % to a full share over this (default 3 s)
	IdleProbe  time.Duration // a live path with nothing sent for this long gets a PING (default 5 s)
}
