package health

import (
	"github.com/backpack/backpack/internal/manage/core"
	"testing"
	"time"
)

func TestAutoRecoveryIsNeverDrivenByLegacyWatchdog(t *testing.T) {
	tunnel := core.Tunnel{Name: "auto-test", Transport: "auto"}
	if !coordinatorOwnsRecovery(tunnel) {
		t.Fatal("Auto has competing recovery owners")
	}
	if checkThroughput(tunnel, newFlowWatch(), map[string]time.Time{}) {
		t.Fatal("legacy throughput recovery operated Auto")
	}
	if coordinatorOwnsRecovery(core.Tunnel{Transport: "stealth"}) {
		t.Fatal("manual watchdog behavior changed")
	}
}
