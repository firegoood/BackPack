package waticmp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

func testSettings(id, direction string) Settings {
	return Settings{Enabled: true, PeerIP: "127.0.0.1", TunnelID: id, Direction: direction,
		Token: "0123456789abcdef0123456789abcdef", MaxStreams: 128}
}
func pair(t *testing.T, id, direction string) (*Conn, *Conn) {
	t.Helper()
	if os.Getenv("WATICMP_LIVE") != "1" {
		t.Skip("set WATICMP_LIVE=1 in an isolated Linux environment with CAP_NET_RAW")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	type result struct {
		c   *Conn
		err error
	}
	ch := make(chan result, 1)
	s := testSettings(id, direction)
	go func() { c, e := Connect(ctx, true, s); ch <- result{c, e} }()
	c, e := Connect(ctx, false, s)
	if e != nil {
		t.Fatal(e)
	}
	r := <-ch
	if r.err != nil {
		c.CloseWithError(0, "")
		t.Fatal(r.err)
	}
	t.Cleanup(func() { c.CloseWithError(0, ""); r.c.CloseWithError(0, "") })
	return c, r.c
}

func TestNativeRawDirectionsAndHalfClose(t *testing.T) {
	for _, direction := range []string{"normal", "reverse"} {
		t.Run(direction, func(t *testing.T) {
			client, server := pair(t, "raw-directions-"+direction, direction)
			if client.Proof("client") != server.Proof("client") || client.Proof("server") == client.Proof("client") {
				t.Fatal("session binding/role separation failed")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			errch := make(chan error, 1)
			payload := bytes.Repeat([]byte("reliable application data/"), 2048)
			go func() {
				st, e := server.AcceptStream(ctx)
				if e != nil {
					errch <- e
					return
				}
				defer st.Close()
				_ = st.SetDeadline(time.Now().Add(8 * time.Second))
				got, e := io.ReadAll(st)
				if e != nil {
					errch <- e
					return
				}
				if !bytes.Equal(got, payload) {
					errch <- errors.New("request corrupt")
					return
				}
				_, e = st.Write(got)
				if e == nil {
					e = st.CloseWrite()
				}
				errch <- e
			}()
			st, e := client.OpenStreamSync(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer st.Close()
			_ = st.SetDeadline(time.Now().Add(8 * time.Second))
			if _, e = st.Write(payload); e != nil {
				t.Fatal(e)
			}
			if e = st.CloseWrite(); e != nil {
				t.Fatal(e)
			}
			got, e := io.ReadAll(st)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, payload) {
				t.Fatal("response corrupt")
			}
			if e = <-errch; e != nil {
				t.Fatal(e)
			}
			t.Logf("%s: request/reply %d/%d bytes verified through native raw ICMP", direction, len(payload), len(got))
		})
	}
}

func TestNativeConcurrentStreamsAndIdentityIsolation(t *testing.T) {
	client, server := pair(t, "concurrent-stream-identity-A", "reverse")
	other, remote := pair(t, "concurrent-stream-identity-B", "normal")
	if client.Proof("client") == other.Proof("client") {
		t.Fatal("cross-identity binding collision")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const streams = 100
	var handlers sync.WaitGroup
	errch := make(chan error, streams+1)
	go func() {
		for n := 0; n < streams; n++ {
			st, e := server.AcceptStream(ctx)
			if e != nil {
				errch <- e
				return
			}
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer st.Close()
				_ = st.SetDeadline(time.Now().Add(20 * time.Second))
				_, e := io.Copy(st, st)
				if e == nil {
					e = st.CloseWrite()
				}
				if e != nil {
					errch <- e
				}
			}()
		}
	}()
	var clients sync.WaitGroup
	for n := 0; n < streams; n++ {
		clients.Add(1)
		go func(n int) {
			defer clients.Done()
			st, e := client.OpenStreamSync(ctx)
			if e != nil {
				errch <- e
				return
			}
			defer st.Close()
			_ = st.SetDeadline(time.Now().Add(20 * time.Second))
			p := bytes.Repeat([]byte{byte(n)}, 1024)
			if _, e = st.Write(p); e == nil {
				e = st.CloseWrite()
			}
			if e != nil {
				errch <- e
				return
			}
			got, e := io.ReadAll(st)
			if e != nil {
				errch <- e
				return
			}
			if !bytes.Equal(p, got) {
				errch <- errors.New("concurrent stream integrity failure")
			}
		}(n)
	}
	clients.Wait()
	handlers.Wait()
	// The second identity must still exchange actual bytes independently.
	go func() {
		st, e := remote.AcceptStream(ctx)
		if e != nil {
			errch <- e
			return
		}
		defer st.Close()
		_ = st.SetDeadline(time.Now().Add(5 * time.Second))
		_, e = io.Copy(st, st)
		if e == nil {
			e = st.CloseWrite()
		}
		if e != nil {
			errch <- e
		}
	}()
	st, e := other.OpenStreamSync(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_ = st.SetDeadline(time.Now().Add(5 * time.Second))
	_, e = st.Write([]byte("isolated"))
	if e == nil {
		e = st.CloseWrite()
	}
	if e != nil {
		t.Fatal(e)
	}
	p, e := io.ReadAll(st)
	st.Close()
	if e != nil || string(p) != "isolated" {
		t.Fatalf("second identity: %q %v", p, e)
	}
	select {
	case e := <-errch:
		t.Fatalf("%v; client=%+v; server=%+v", e, client.Stats(), server.Stats())
	default:
	}
	t.Log("100 concurrent streams and a separate identity exchanged verified data")
}

func TestNativeBadEnrollmentAndCancellation(t *testing.T) {
	s := testSettings("enrollment-negative", "reverse")
	for _, mutate := range []func(*Settings){func(s *Settings) { s.Enabled = false }, func(s *Settings) { s.Token = "short" }, func(s *Settings) { s.PeerIP = "::1" }, func(s *Settings) { s.MaxPayload = 1401 }, func(s *Settings) { s.MaxStreams = 257 }, func(s *Settings) { s.Direction = "auto" }} {
		bad := s
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatalf("unsafe settings accepted: %+v", bad)
		}
	}
	if os.Getenv("WATICMP_LIVE") != "1" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c, e := Connect(ctx, true, s); e == nil {
		c.CloseWithError(0, "")
		t.Fatal("cancelled pairing accepted")
	}
}
