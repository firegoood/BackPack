package peerauto

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

var errProbePressure = errors.New("SKIPPED: local CPU/memory/application pressure")
var errProbeBusy = errors.New("SKIPPED: global probe concurrency budget")

// Two authority probes per process, one per pair (the decision owner is
// serial). Their 1 s spacing bounds aggregate echo payload to <256 KiB/s;
// each transport session is separately limited and lives at most 15 s.
var probeBudget = struct {
	sync.Mutex
	slots chan struct{}
	next  time.Time
}{slots: make(chan struct{}, 2)}

func (m *Manager) acquireProbe() bool {
	select {
	case probeBudget.slots <- struct{}{}:
	default:
		return false
	}
	probeBudget.Lock()
	wait := time.Until(probeBudget.next)
	probeBudget.next = time.Now().Add(max(wait, 0) + time.Second)
	probeBudget.Unlock()
	if wait > 0 && !pause(m.ctx, wait) {
		releaseProbe()
		return false
	}
	return true
}
func releaseProbe() { <-probeBudget.slots }

var resources = struct {
	sync.Mutex
	proc *process.Process
	at   time.Time
	cpu  float64
	rss  uint64
}{}

func (m *Manager) pressure() bool {
	// Reserving user capacity comes before starting a benchmark. A skipped
	// probe is never a failed carrier measurement.
	if len(m.connections) >= max(1, m.policy.MaxConnections/2) {
		return true
	}
	resources.Lock()
	defer resources.Unlock()
	if resources.proc == nil {
		resources.proc, _ = process.NewProcess(int32(os.Getpid()))
	}
	if resources.proc != nil && time.Since(resources.at) > time.Second {
		if mem, e := resources.proc.MemoryInfo(); e == nil {
			resources.rss = mem.RSS
		}
		if cpu, e := resources.proc.Percent(0); e == nil {
			resources.cpu = cpu / float64(max(1, runtime.GOMAXPROCS(0)))
		}
		resources.at = time.Now()
	}
	return resources.rss > uint64(m.policy.MaxMemoryMB)*1024*1024 || resources.cpu >= 85
}

type Event struct {
	At        time.Time `json:"at"`
	Candidate string    `json:"candidate,omitempty"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason"`
}

func (m *Manager) event(id, result, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.History = append(m.status.History, Event{time.Now(), id, result, reason})
	if len(m.status.History) > 64 {
		m.status.History = m.status.History[len(m.status.History)-64:]
	}
}
func recordCommit(st durableState, id string, now time.Time) durableState {
	recent := make([]int64, 0, 12)
	for _, t := range st.Switches {
		if now.Unix()-t < 3600 {
			recent = append(recent, t)
		}
	}
	if st.LastGood != "" && st.LastGood != id {
		recent = append(recent, now.Unix())
		st.LastChange = now.Unix()
	}
	if len(recent) > 12 {
		recent = recent[len(recent)-12:]
	}
	st.Switches = recent
	st.Active, st.LastGood, st.Intent = id, id, ""
	return st
}
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	x := append([]float64(nil), values...)
	slices.Sort(x)
	return x[len(x)/2]
}
func (m *Manager) eligible(e *candidate, now time.Time) bool {
	return e.ctx.Err() == nil && !m.runtimeDisabled[e.spec.ID] && (m.runtimeLock == "" || m.runtimeLock == e.spec.ID) && !now.Before(e.quarantineUntil)
}
func (m *Manager) nextSwitch(st durableState, now time.Time, degradation bool) time.Time {
	var next time.Time
	if st.LastChange != 0 {
		next = time.Unix(st.LastChange, 0).Add(time.Duration(m.policy.Cooldown) * time.Second)
	}
	count := 0
	oldest := now
	for _, t := range st.Switches {
		at := time.Unix(t, 0)
		if now.Sub(at) < time.Hour {
			count++
			if at.Before(oldest) {
				oldest = at
			}
		}
	}
	if count >= m.policy.MaxSwitchesPerHour {
		next = maxTime(next, oldest.Add(time.Hour))
	}
	if degradation {
		next = maxTime(next, m.Snapshot().LastSwitch.Add(time.Duration(m.policy.MinimumDwell)*time.Second))
	}
	return next
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func (m *Manager) probeFailure(e *candidate, err error) {
	result := "FAILED"
	if errors.Is(err, errProbePressure) || errors.Is(err, errProbeBusy) {
		result = "SKIPPED"
	} else {
		e.failedProbes++
		e.quarantineUntil = time.Now().Add(min(time.Minute, time.Duration(5*(1<<min(e.failedProbes-1, 3)))*time.Second))
	}
	m.candidateStatus(e.spec.ID, func(s *CandidateStatus) {
		s.Reason = result + ": " + err.Error()
		s.QuarantineUntil = e.quarantineUntil
	})
	m.event(e.spec.ID, result, err.Error())
}

// One owner serializes actions, failure debounce and durable switch decisions.
// Standbys use only manifest-authorized endpoints and are kept authenticated
// before the active data and control path fails.
func (m *Manager) leaderLoop() error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return nil
		case a := <-m.actions:
			m.handleAction(a)
			continue
		case <-ticker.C:
		}
		m.mu.Lock()
		active := m.active
		eps := make([]*candidate, 0, len(m.endpoints))
		for _, p := range m.policy.Candidates {
			if e := m.endpoints[p.ID]; e != nil {
				eps = append(eps, e)
			}
		}
		m.mu.Unlock()
		// Independent streams can be pinged concurrently. This is liveness only;
		// no goroutine may propose/commit a transport change.
		var wg sync.WaitGroup
		for _, e := range eps {
			e := e
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.mu.Lock()
				r, err := e.w.ask(message{Op: "STATUS"}, "STATUS")
				e.mu.Unlock()
				if err != nil {
					e.cancel()
					return
				}
				e.lastSeen.Store(time.Now().UnixNano())
				if active != nil && active.endpoint == e && (r.Active != e.spec.ID || r.Generation != active.generation) {
					e.cancel()
				}
			}()
		}
		wg.Wait()
		m.mu.Lock()
		active = m.active
		m.mu.Unlock()
		if active != nil && active.endpoint.ctx.Err() == nil {
			m.unavailableSince = time.Time{}
			if time.Since(m.lastHealth) >= 10*time.Second {
				m.lastHealth = time.Now()
				if err := m.exchangeProbe(active.endpoint, 512); err != nil {
					m.probeFailure(active.endpoint, err)
					if !errors.Is(err, errProbePressure) && !errors.Is(err, errProbeBusy) {
						m.dataFailures++
					}
				} else {
					m.dataFailures = 0
				}
			}
			if m.dataFailures >= m.policy.FailureThreshold {
				m.mu.Lock()
				m.fenceLocked()
				m.mu.Unlock()
				m.event(active.endpoint.spec.ID, "FAILED", "three or more isolated end-to-end data probes failed; backend outages excluded")
				active = nil
			} else {
				m.periodicProbe(eps, active)
				continue
			}
		}
		if m.unavailableSince.IsZero() {
			m.unavailableSince = time.Now()
		}
		st := m.store.snapshot()
		if st.LastGood != "" && time.Since(m.unavailableSince) < time.Duration(m.policy.FailureThreshold)*2*time.Second {
			m.setState("RECOVERING", "debouncing missing active peer; awaiting same-candidate reconnection")
			continue
		}
		// Same-candidate recovery does not consume a cross-carrier switch budget.
		// Prefer last good only while reachable; a dead path cannot veto recovery.
		slices.SortStableFunc(eps, func(a, b *candidate) int {
			if a.spec.ID == st.LastGood {
				return -1
			}
			if b.spec.ID == st.LastGood {
				return 1
			}
			return 0
		})
		recovered := false
		for _, e := range eps {
			if !m.eligible(e, time.Now()) {
				continue
			}
			if m.policy.ObserveOnly && st.LastGood == "" {
				primary := ""
				for _, p := range m.policy.Candidates {
					if !p.Disabled {
						primary = p.ID
						break
					}
				}
				if e.spec.ID != primary {
					continue
				}
			}
			if st.LastGood != "" && e.spec.ID != st.LastGood {
				if m.policy.ObserveOnly {
					continue
				}
				next := m.nextSwitch(st, time.Now(), false)
				m.mu.Lock()
				m.status.NextSwitch = next
				m.mu.Unlock()
				if time.Now().Before(next) {
					continue
				}
			}
			if err := m.activate(e, "confirmed failure/bootstrap recovery through enrolled candidate"); err != nil {
				m.probeFailure(e, err)
				continue
			}
			m.unavailableSince = time.Time{}
			m.dataFailures = 0
			m.lastHealth = time.Now()
			recovered = true
			break
		}
		if !recovered {
			m.setState("NO_ROUTE", "no eligible mutually passing candidate; inspect lock, budget, privilege and enrollment status")
		}
	}
}

func (m *Manager) periodicProbe(eps []*candidate, active *route) {
	now := time.Now()
	if m.pressure() {
		return
	}
	for _, e := range eps {
		if !m.eligible(e, now) {
			continue
		}
		if e.nextProbe.IsZero() {
			e.nextProbe = now.Add(time.Duration(m.policy.ProbeInterval) * time.Second)
		}
		if now.Before(e.nextProbe) {
			continue
		}
		// ±10% jitter avoids synchronizing independent pairs on the same VPS.
		interval := time.Duration(m.policy.ProbeInterval) * time.Second
		e.nextProbe = now.Add(interval + time.Duration(rand.Int64N(max(1, int64(interval/5)))) - interval/10)
		if err := m.probe(e); err != nil {
			m.probeFailure(e, err)
			return
		}
		e.failedProbes = 0
		if e.baseline == 0 && len(e.samples) >= 3 {
			e.baseline = median(e.samples)
		}
		if e != active.endpoint {
			return
		}
		severe := len(e.samples) >= 3 && e.baseline > 0 && median(e.samples) < e.baseline*0.4
		if !severe {
			m.degradedSamples = 0
			m.setState("HEALTHY", "current carrier passes; small benchmark advantages do not trigger a switch")
			return
		}
		m.degradedSamples++
		m.setState("DEGRADED", "sustained useful-throughput/latency score below 40% of three-sample initial baseline")
		if !m.policy.AllowDegradationSwitch || m.policy.ObserveOnly || m.degradedSamples < 3 {
			return
		}
		next := m.nextSwitch(m.store.snapshot(), time.Now(), true)
		m.mu.Lock()
		m.status.NextSwitch = next
		m.mu.Unlock()
		if time.Now().Before(next) {
			return
		}
		for _, other := range eps {
			if other == e || !m.eligible(other, time.Now()) || time.Since(other.probeAt) > time.Duration(m.policy.ProbeInterval)*2*time.Second || len(other.samples) < 3 {
				continue
			}
			if median(other.samples) < median(e.samples)*1.5 {
				continue
			}
			if err := m.activate(other, fmt.Sprintf("sustained severe degradation; alternative median score exceeds current by 50%%")); err != nil {
				m.probeFailure(other, err)
			} else {
				m.degradedSamples = 0
			}
			break
		}
		return // at most one periodic bulk measurement per tick
	}
}
