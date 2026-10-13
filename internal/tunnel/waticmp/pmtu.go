package waticmp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"
)

type mtuAck struct {
	nonce [16]byte
	size  int
}

// DF is set on the raw socket. A padded authenticated request has exactly the
// same outer size as a data packet at this limit; an ACK proves peer receipt.
// Keys are bound to this fresh session. Echoing by the host kernel cannot pass.
func (i *packetIO) receiveMTU(p []byte) {
	k := i.mtuKey.Load()
	if k == nil || len(p) < headerSize+1+18+32 || !i.mtuBudget.Allow() ||
		!hmac.Equal(mac(k[:], p[:len(p)-32]), p[len(p)-32:]) {
		return
	}
	b := p[headerSize+1:]
	a := mtuAck{size: int(binary.BigEndian.Uint16(b[:2]))}
	copy(a.nonce[:], b[2:18])
	if a.size < 256 || a.size > i.receiveLimit {
		return
	}
	if p[headerSize] == msgMTUProbe {
		if len(p) != headerSize+1+a.size {
			return
		}
		// No amplification: the authenticated response is always 74 bytes.
		r := append([]byte(nil), i.header[:]...)
		r = append(r, msgMTUAck)
		r = append(r, b[:18]...)
		r = append(r, mac(k[:], r)...)
		_ = i.raw.Send(r, 0)
	} else if len(p) == headerSize+1+18+32 {
		select {
		case i.mtuAcks <- a:
		default:
		}
	}
}

func (i *packetIO) probeMTU(ctx context.Context, ceiling int) (int, error) {
	approved := 0
	steps := []int{256, 512, 588, 648, 1024, 1400}
	for _, size := range steps {
		if size > ceiling {
			break
		}
		var n [16]byte
		if _, err := rand.Read(n[:]); err != nil {
			return 0, err
		}
		p := make([]byte, headerSize+1+size)
		copy(p, i.header[:])
		p[headerSize] = msgMTUProbe
		binary.BigEndian.PutUint16(p[headerSize+1:], uint16(size))
		copy(p[headerSize+3:], n[:])
		copy(p[len(p)-32:], mac(i.mtuKey.Load()[:], p[:len(p)-32]))
		passed := false
		for attempt := 0; attempt < 4 && !passed; attempt++ {
			// EMSGSIZE is a local PMTU failure; never enable IP fragmentation.
			if err := i.raw.Send(p, 0); err != nil {
				break
			}
			// Restrictive ICMP paths can also have a long RTT. Keep the
			// nonce/MAC proof and four-attempt budget, but allow a real
			// round trip instead of declaring a delayed 256-byte ACK absent.
			// Connect's context still bounds the whole handshake/probe.
			t := time.NewTimer(500 * time.Millisecond)
		wait:
			for {
				select {
				case <-ctx.Done():
					t.Stop()
					return 0, ctx.Err()
				case <-i.stop:
					t.Stop()
					return 0, errors.New("Wat ICMP socket closed during PMTU test")
				case <-t.C:
					break wait
				case a := <-i.mtuAcks:
					if a.size == size && a.nonce == n {
						passed = true
						t.Stop()
						break wait
					}
				}
			}
		}
		if !passed {
			break
		}
		approved = size
	}
	if approved == 0 {
		return 0, errors.New("Wat ICMP authenticated PMTU probe did not pass at 256 bytes")
	}
	return approved, nil
}
