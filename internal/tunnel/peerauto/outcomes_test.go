package peerauto

import (
	"context"
	"testing"
	"time"
)

func TestFlowOutcomesDistinguishCompletedDrainAndFence(t *testing.T) {
	m, _, _ := followerForState(t)
	for _, outcome := range []string{"completed", "drained", "interrupted"} {
		ctx, cancel := context.WithCancel(context.Background())
		r := &route{ctx: ctx, cancel: cancel}
		finish := m.trackFlow(r)
		if outcome == "drained" {
			r.retired.Store(true)
		}
		if outcome == "interrupted" {
			cancel()
		}
		finish()
		finish() // repeated socket close must not double-count
		cancel()
	}
	s := m.Snapshot().Flows
	if s.Admitted != 3 || s.Completed != 2 || s.Drained != 1 || s.Interrupted != 1 {
		t.Fatalf("incorrect close outcomes: %+v", s)
	}
}

func TestStatusRetainsCooldownAndHourlyEligibilityAfterCommit(t *testing.T) {
	m, _, _ := followerForState(t)
	now := time.Now().Truncate(time.Second)
	st := m.store.snapshot()
	st.LastChange, st.Switches = now.Unix(), []int64{now.Add(-10 * time.Minute).Unix(), now.Unix()}
	if err := m.store.save(st); err != nil {
		t.Fatal(err)
	}
	m.status.NextSwitch = time.Time{}
	want := now.Add(50 * time.Minute)
	if got := m.Snapshot().NextSwitch; !got.Equal(want) {
		t.Fatalf("committed route hid hourly switch budget: got %v want %v", got, want)
	}
}
