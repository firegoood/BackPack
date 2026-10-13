// Package probe contains the actual byte-integrity exchange shared by the
// interactive Connection Test and enrolled unattended peer probes.
package probe

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"time"
)

func Echo(c net.Conn, udp bool) error {
	// Preserve the legacy Connection Test's immediate short-write rejection.
	p := make([]byte, 512)
	if _, e := rand.Read(p); e != nil {
		return e
	}
	_ = c.SetDeadline(time.Now().Add(4 * time.Second))
	defer c.SetDeadline(time.Time{})
	if n, e := c.Write(p); e != nil {
		return e
	} else if n != len(p) {
		return io.ErrShortWrite
	}
	got := make([]byte, len(p))
	var e error
	if udp {
		var n int
		n, e = c.Read(got)
		got = got[:n]
	} else {
		_, e = io.ReadFull(c, got)
	}
	if e != nil {
		return e
	}
	if !bytes.Equal(got, p) {
		return errors.New("the echo came back different")
	}
	return nil
}

// Exchange succeeds only after the complete random payload returns unchanged.
// Memory/traffic is bounded and cancellation joins the writer before return.
func Exchange(ctx context.Context, c net.Conn, size int, udp bool) (time.Duration, string, error) {
	return exchange(ctx, c, size, udp, 0)
}

// ExchangePaced uses the same byte-integrity exchange at a modest application
// rate for all candidate transports. Results at this cap are lower bounds,
// not an estimate of their unrestricted throughput.
func ExchangePaced(ctx context.Context, c net.Conn, size int) (time.Duration, string, error) {
	return exchange(ctx, c, size, false, 128000)
}
func exchange(ctx context.Context, c net.Conn, size int, udp bool, bps int) (time.Duration, string, error) {
	if size < 1 || size > 32768 {
		return 0, "", errors.New("probe size outside budget")
	}
	p := make([]byte, size)
	if _, err := rand.Read(p); err != nil {
		return 0, "", err
	}
	_ = c.SetDeadline(time.Now().Add(exchangeTimeout(size, bps)))
	defer c.SetDeadline(time.Time{})
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	start := time.Now()
	we := make(chan error, 1)
	go func() {
		n, e := writePayload(ctx, c, p, bps)
		if e == nil && n != len(p) {
			e = io.ErrShortWrite
		}
		we <- e
	}()
	got := make([]byte, size)
	var err error
	if udp {
		var n int
		n, err = c.Read(got)
		got = got[:n]
	} else {
		_, err = io.ReadFull(c, got)
	}
	if err != nil {
		c.Close()
	}
	werr := <-we
	if err == nil {
		err = werr
	}
	if err == nil && !bytes.Equal(got, p) {
		err = errors.New("probe echo integrity mismatch")
	}
	h := sha256.Sum256(p)
	return time.Since(start), hex.EncodeToString(h[:]), err
}

func Respond(c net.Conn, size int) (string, error) {
	return respond(c, size, 0)
}
func RespondPaced(c net.Conn, size int) (string, error) { return respond(c, size, 128000) }
func respond(c net.Conn, size int, bps int) (string, error) {
	if size < 1 || size > 32768 {
		return "", errors.New("probe size outside budget")
	}
	_ = c.SetDeadline(time.Now().Add(exchangeTimeout(size, bps)))
	defer c.SetDeadline(time.Time{})
	b := make([]byte, size)
	if _, e := io.ReadFull(c, b); e != nil {
		return "", e
	}
	if _, e := writePayload(context.Background(), c, b, bps); e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func exchangeTimeout(size, bps int) time.Duration {
	// A paced bulk measurement must admit a slow but passing path. Preserve
	// the existing Connection Test/interactive deadline; Auto's entire fresh
	// session still has a separate 15 s lifetime and a 32 KiB byte ceiling.
	if bps != 0 && size > 512 {
		return 8 * time.Second
	}
	return 4 * time.Second
}

func writePayload(ctx context.Context, c net.Conn, b []byte, bps int) (int, error) {
	if bps == 0 || len(b) <= 512 {
		return c.Write(b)
	}
	start := time.Now()
	sent := 0
	for sent < len(b) {
		// Keep each burst below 4 KiB, and account for the first burst too.
		n := min(4096, len(b)-sent)
		at := start.Add(time.Duration(int64(sent+n) * int64(time.Second) / int64(bps)))
		if wait := time.Until(at); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return sent, ctx.Err()
			case <-timer.C:
			}
		}
		written, e := c.Write(b[sent : sent+n])
		sent += written
		if e != nil {
			return sent, e
		}
		if written != n {
			return sent, io.ErrShortWrite
		}
	}
	return sent, nil
}
