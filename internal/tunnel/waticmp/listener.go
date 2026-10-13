package waticmp

import (
	"context"
	"net"
	"sync"
)

// Listener accepts fresh enrolled peer incarnations on raw ICMP, never TCP/UDP.
// The existing reverse lifecycle owns it and all of its accepted connections.
type Listener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	settings Settings
	once     sync.Once
}

func Listen(ctx context.Context, s Settings) (*Listener, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Listener{ctx: ctx, cancel: cancel, settings: s}, nil
}
func (l *Listener) Accept(ctx context.Context) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, l.cancel)
	defer stop()
	return Connect(l.ctx, true, l.settings)
}
func (l *Listener) Close() error   { l.once.Do(l.cancel); return nil }
func (l *Listener) Addr() net.Addr { return &net.IPAddr{IP: net.IPv4zero} }
