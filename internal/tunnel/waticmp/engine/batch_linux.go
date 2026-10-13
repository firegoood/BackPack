// Copyright (c) 2026 Amirreza. SPDX-License-Identifier: MIT
// Adapted for FullPack; see LICENSE and ORIGIN.json in this package.

//go:build linux

package wat

import (
	"net"
	"os"
	"sync"

	"golang.org/x/net/ipv4"
)

// ICMP recvmmsg / sendmmsg through x/net/ipv4.

const (
	rxBatch = 32   // datagrams per recvmmsg
	rxBufSz = 2048 // fits the largest datagram of the raw-socket and UDP carriers (1400 + headers)
)

// Retain the reference's optional ICMP batching switch; no unrelated UDP carrier.
func icmpBatchDisabled() bool { return os.Getenv("WAT_BATCH") == "0" }

// mmsgRx reads up to rxBatch datagrams per system call and hands them out one by one.
type mmsgRx struct {
	pc   *ipv4.PacketConn
	msgs []ipv4.Message
	n, i int
}

func newMmsgRx(pc *ipv4.PacketConn) *mmsgRx {
	r := &mmsgRx{pc: pc, msgs: make([]ipv4.Message, rxBatch)}
	for i := range r.msgs {
		r.msgs[i].Buffers = [][]byte{make([]byte, rxBufSz)}
	}
	return r
}

// next returns the next datagram; it is valid until the next call that refills the batch.
func (r *mmsgRx) next() ([]byte, net.Addr, error) {
	for r.i >= r.n {
		n, err := r.pc.ReadBatch(r.msgs, 0)
		if err != nil {
			return nil, nil, err
		}
		r.n, r.i = n, 0
	}
	m := &r.msgs[r.i]
	r.i++
	return m.Buffers[0][:m.N], m.Addr, nil
}

// mmsgTx sends a burst with sendmmsg. Shared by every session of a server, hence the lock.
type mmsgTx struct {
	pc   *ipv4.PacketConn
	mu   sync.Mutex
	msgs []ipv4.Message
}

// write sends the messages built by fill (called under the lock with the reusable message slice).
func (t *mmsgTx) write(n int, dst net.Addr, set func(i int, m *ipv4.Message)) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for len(t.msgs) < n {
		t.msgs = append(t.msgs, ipv4.Message{Buffers: make([][]byte, 1)})
	}
	ms := t.msgs[:n]
	for i := range ms {
		ms[i].Addr = dst
		set(i, &ms[i])
	}
	var err error
	for len(ms) > 0 {
		var k int
		if k, err = t.pc.WriteBatch(ms, 0); err != nil || k <= 0 {
			break
		}
		ms = ms[k:]
	}
	for i := 0; i < n; i++ {
		t.msgs[i].Buffers[0] = nil
	}
	return err
}
