// Package enginectl is the engine's local control socket.
//
// # The rung that was missing
//
// The watchdog's only lever on a tunnel was `systemctl restart`, and
// internal/manage/throughputhealth.go says so where the graduated response is
// described: "an engine runs as its own process with no control socket, so
// 're-dial', 'rebuild the pool' and 'restart the transport' are not available to
// ask for without building an IPC channel first. Pretending otherwise would
// mean a ladder whose rungs are all the same rung."
//
// This is that channel, and it makes one rung real: the engine can restart its
// own transport in place. That is a smaller act than restarting the process and
// a better one — the process keeps its metrics history, its uptime, its
// accumulated counters and its log continuity, and nothing else on the machine
// has to be told. A stall that clears on a transport restart never reaches the
// `systemctl` rung at all.
//
// # Why a unix socket and not a port
//
// It is reachable only by something already on the machine and running as root.
// The socket is 0600 under /run/fullpack, which is 0700, so the attack surface
// added is exactly the attack surface of already being root — which is to say
// none. A TCP port, however tightly bound, is one firewall mistake away from
// being reachable, and this is a channel that restarts tunnels.
//
// # Why it speaks the shape internal/node already defines
//
// A request is an op name and a raw body; a response is ok, an error string and
// a raw body. That is the same restricted shape the panel uses to talk to a
// managed server, and it is restricted for the same reason: the set of things
// that can be asked for is a closed list in one place, not whatever the caller
// can construct. Two RPC shapes in one product is how a gap opens between them.
//
// # Why the unit does not declare a RuntimeDirectory
//
// `RuntimeDirectory=fullpack` is the systemd idiom and would have systemd make
// the directory with the right mode and remove it on stop. It is not used, and
// the reason is not style: the tunnel units are compared against one template
// and rewritten when they differ, so adding a line to that template restarts
// every tunnel on the machine at the next update. /run is a tmpfs and is empty
// at boot, the engine makes the directory itself, and Serve removes its own
// socket — so the line would buy nothing and cost every operator an outage.
//
// # What it deliberately does not offer
//
// "Rebuild the pool" is not here, and neither is anything else the engine cannot
// genuinely do today. An op that reports success and changes nothing is worse
// than an op that does not exist, because a ladder with a rung that does nothing
// is a ladder whose failures are invisible.
package enginectl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// OpPing answers that the engine is alive and answering, which a process
	// table cannot say: a wedged engine is a running process.
	OpPing = "ping"

	// OpStatus is what the engine knows about itself right now, as opposed to
	// what it last wrote to its metrics snapshot up to thirty seconds ago.
	OpStatus = "status"

	// OpRestartTransport tears the current transport down and starts it again
	// inside the same process. The rung below `systemctl restart`.
	OpRestartTransport = "restart-transport"
	OpAutoStatus       = "auto-status"
	OpAutoAction       = "auto-action"
)

// Dir is where the sockets live. One per tunnel, named after it.
var Dir = "/run/fullpack"

// SocketPath is the socket for one tunnel.
func SocketPath(name string) string { return filepath.Join(Dir, name+".sock") }

// Request is one operation, in the shape internal/node/protocol.go defines.
type Request struct {
	Op   string          `json:"op"`
	Body json.RawMessage `json:"body,omitempty"`
}

// Response is the answer.
type Response struct {
	OK   bool            `json:"ok"`
	Err  string          `json:"err,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

// Status is what the engine says about itself.
type Status struct {
	Name      string `json:"name"`
	Role      string `json:"role,omitempty"`
	Transport string `json:"transport,omitempty"`
	// Connected is the engine's own answer about its control channel, which is
	// a different claim from "the process is running".
	Connected bool   `json:"connected"`
	Peer      string `json:"peer,omitempty"`
	BytesIn   uint64 `json:"bytesIn"`
	BytesOut  uint64 `json:"bytesOut"`
	// Generation counts how many times the transport has been started in this
	// process, so a caller can tell a restart that happened from one that was
	// asked for and did not.
	Generation  int             `json:"generation"`
	Diagnostics json.RawMessage `json:"diagnostics,omitempty"`
}

// Handler is what the engine supplies: the two questions it can answer and the
// one thing it can be asked to do.
//
// It is an interface rather than three function fields because an engine that
// can answer one of these can answer all three, and a half-implemented control
// socket is the thing the package comment refuses to build.
type Handler interface {
	Status() Status
	// RestartTransport asks for the running transport to be torn down and
	// started again. It returns when the request has been accepted, not when
	// the new one is carrying — the caller polls Status for that, because
	// "accepted" and "working" are different answers and a socket that blocks
	// until the second one is a socket that hangs on the failure it exists for.
	RestartTransport() error
}

// Optional extension: legacy engines keep the original closed operation set.
type AutoHandler interface {
	AutoStatus() (any, error)
	AutoAction(context.Context, json.RawMessage) error
}

// ErrNotRunning is what Dial reports when no engine is listening for this
// tunnel: the socket is absent, or it is there and nothing is behind it.
var ErrNotRunning = errors.New("no engine is listening for this tunnel")

// Serve answers on the tunnel's socket until ctx ends.
//
// A socket that cannot be created is not fatal to the engine and must not be:
// the control socket is a convenience for whatever is watching, and a tunnel
// that refused to carry traffic because /run was not writable would be trading
// the product for the diagnostic.
func Serve(ctx context.Context, name string, h Handler) error {
	if err := os.MkdirAll(Dir, 0o700); err != nil {
		return fmt.Errorf("enginectl: %w", err)
	}
	// MkdirAll leaves an existing directory's mode alone, and a directory this
	// product made on an earlier version — or one somebody created by hand —
	// may be readable by anyone. The socket's own 0600 is what actually
	// protects it; this is the second lock on the same door, and it is cheap.
	_ = os.Chmod(Dir, 0o700)
	path := SocketPath(name)
	release, e := claimSocket(path)
	if e != nil {
		return e
	}
	defer release()
	// A socket left by a process that did not exit cleanly would refuse the
	// bind. The exclusive socket claim is held until listener cleanup finishes,
	// so a duplicate process cannot unlink a live engine's control endpoint.
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("enginectl: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return fmt.Errorf("enginectl: %w", err)
	}

	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	defer func() { ln.Close(); _ = os.Remove(path) }()
	serveCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	permits := make(chan struct{}, 32)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case permits <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-permits }()
			stopConn := context.AfterFunc(serveCtx, func() { conn.Close() })
			defer stopConn()
			serveConn(serveCtx, conn, h)
		}()
	}
}

// serveConn answers requests on one connection until it closes.
//
// The deadline is per request rather than per connection: a caller that holds
// the socket open between questions is doing the right thing, and one that
// opens it and says nothing must not hold a goroutine for ever.
func serveConn(ctx context.Context, conn net.Conn, h Handler) {
	defer conn.Close()
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		res := answerContext(ctx, req, h)
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := enc.Encode(res); err != nil {
			return
		}
	}
}

func answerContext(parent context.Context, req Request, h Handler) Response {
	switch req.Op {
	case OpPing:
		return Response{OK: true}

	case OpStatus:
		body, err := json.Marshal(h.Status())
		if err != nil {
			return Response{Err: err.Error()}
		}
		return Response{OK: true, Body: body}

	case OpRestartTransport:
		if err := h.RestartTransport(); err != nil {
			return Response{Err: err.Error()}
		}
		return Response{OK: true}
	case OpAutoStatus:
		if a, ok := h.(AutoHandler); ok {
			s, e := a.AutoStatus()
			if e != nil {
				return Response{Err: e.Error()}
			}
			b, e := json.Marshal(s)
			if e != nil {
				return Response{Err: e.Error()}
			}
			return Response{OK: true, Body: b}
		}
	case OpAutoAction:
		if a, ok := h.(AutoHandler); ok {
			if len(req.Body) > 1024 {
				return Response{Err: "Auto action exceeds size limit"}
			}
			ctx, cancel := context.WithTimeout(parent, 90*time.Second)
			defer cancel()
			if e := a.AutoAction(ctx, req.Body); e != nil {
				return Response{Err: e.Error()}
			}
			return Response{OK: true}
		}
	}
	// The closed list is the point. An unknown op is refused by name so that a
	// caller from a newer build learns that this engine is older, rather than
	// reading silence as success.
	return Response{Err: "this engine does not do " + req.Op}
}

// Client talks to one engine.
type Client struct{ conn net.Conn }

// Dial opens the tunnel's socket.
func Dial(name string) (*Client, error) {
	conn, err := net.DialTimeout("unix", SocketPath(name), 2*time.Second)
	if err != nil {
		return nil, ErrNotRunning
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Call sends one request and decodes the answer into out, which may be nil.
func (c *Client) Call(op string, out any) error {
	return c.CallBody(op, nil, out)
}
func (c *Client) CallBody(op string, body any, out any) error {
	timeout := 10 * time.Second
	if op == OpAutoAction {
		timeout = 100 * time.Second
	}
	_ = c.conn.SetDeadline(time.Now().Add(timeout))
	b, e := json.Marshal(body)
	if e != nil {
		return e
	}
	if err := json.NewEncoder(c.conn).Encode(Request{Op: op, Body: b}); err != nil {
		return err
	}
	var res Response
	if err := json.NewDecoder(c.conn).Decode(&res); err != nil {
		return err
	}
	if !res.OK {
		return errors.New(res.Err)
	}
	if out != nil && len(res.Body) > 0 {
		return json.Unmarshal(res.Body, out)
	}
	return nil
}

func AskBody(name, op string, body, out any) error {
	c, e := Dial(name)
	if e != nil {
		return e
	}
	defer c.Close()
	return c.CallBody(op, body, out)
}

// Ask is the one-shot form: dial, ask, close. It is what a watchdog wants,
// which asks one question every few minutes and holds nothing in between.
func Ask(name, op string, out any) error {
	c, err := Dial(name)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Call(op, out)
}
