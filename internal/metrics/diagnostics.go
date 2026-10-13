package metrics

import (
	"encoding/json"
	"sync/atomic"
)

// The owner supplies bounded, secret-free transport diagnostics. The callback
// avoids a dependency from metrics back into a tunnel/controller package.
type diagnosticSource struct{ read func() any }

var transportDiagnostics atomic.Pointer[diagnosticSource]

func SetTransportDiagnostics(read func() any) (clear func()) {
	source := &diagnosticSource{read: read}
	transportDiagnostics.Store(source)
	return func() { transportDiagnostics.CompareAndSwap(source, nil) }
}
func diagnosticSnapshot() json.RawMessage {
	if source := transportDiagnostics.Load(); source != nil {
		b, err := json.Marshal(source.read())
		if err == nil && len(b) <= 64<<10 {
			return b
		}
	}
	return nil
}
