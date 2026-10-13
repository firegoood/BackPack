package metrics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOldDiagnosticOwnerCannotClearNewGeneration(t *testing.T) {
	clearOld := SetTransportDiagnostics(func() any { return map[string]string{"active": "old"} })
	clearNew := SetTransportDiagnostics(func() any { return map[string]string{"active": "new"} })
	defer clearNew()
	clearOld()
	var got map[string]string
	if json.Unmarshal(diagnosticSnapshot(), &got) != nil || got["active"] != "new" {
		t.Fatal("old generation cleared current diagnostics")
	}
	clearNew()
	if diagnosticSnapshot() != nil {
		t.Fatal("owner callback outlived cleanup")
	}
	clearLarge := SetTransportDiagnostics(func() any { return strings.Repeat("x", 65537) })
	defer clearLarge()
	if diagnosticSnapshot() != nil {
		t.Fatal("unbounded diagnostic entered the snapshot")
	}
}
