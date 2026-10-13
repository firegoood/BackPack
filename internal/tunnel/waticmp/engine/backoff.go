// Copyright (c) 2026 Amirreza. SPDX-License-Identifier: MIT
// Adapted receive backoff from wat/server.go; see ORIGIN.json.
package wat

import "time"

// Bounded receive-error backoff; successful traffic immediately resets it.
type recvBackoff struct {
	d      time.Duration
	failed bool
}

func (b *recvBackoff) fail(err error, logf func(string, ...any), what string) {
	if !b.failed {
		b.failed = true
		if logf != nil {
			logf("%s error: %v", what, err)
		}
	}
	if b.d == 0 {
		b.d = time.Millisecond
	}
	time.Sleep(b.d)
	b.d = min(2*b.d, 200*time.Millisecond)
}
func (b *recvBackoff) ok() { b.d = 0; b.failed = false }
