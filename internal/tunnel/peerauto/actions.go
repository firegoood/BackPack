package peerauto

import (
	"context"
	"errors"
	"time"
)

type Action struct {
	Operation string `json:"operation"`
	Candidate string `json:"candidate,omitempty"`
}
type actionRequest struct {
	action Action
	result chan error
	ctx    context.Context
}

func (m *Manager) Action(ctx context.Context, a Action) error {
	if !m.iran {
		return errors.New("ask the enrolled Iran switching authority")
	}
	r := actionRequest{action: a, result: make(chan error, 1), ctx: ctx}
	select {
	case m.actions <- r:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
	select {
	case e := <-r.result:
		return e
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}
func (m *Manager) handleAction(a actionRequest) {
	if a.ctx != nil && a.ctx.Err() != nil {
		a.result <- a.ctx.Err()
		return
	}
	if a.action.Operation == "unlock" {
		m.runtimeLock = m.policy.ManualLock
		m.mu.Lock()
		m.status.ManualLock = m.runtimeLock
		m.mu.Unlock()
		a.result <- nil
		return
	}
	if a.action.Operation == "observe" || a.action.Operation == "resume" {
		m.policy.ObserveOnly = a.action.Operation == "observe"
		m.mu.Lock()
		m.status.ObserveOnly = m.policy.ObserveOnly
		m.mu.Unlock()
		a.result <- nil
		return
	}
	m.mu.Lock()
	e := m.endpoints[a.action.Candidate]
	m.mu.Unlock()
	if e == nil {
		a.result <- errors.New("candidate is disabled or not authenticated/reachable")
		return
	}
	var err error
	switch a.action.Operation {
	case "test-now":
		err = m.probe(e)
		if err != nil {
			m.probeFailure(e, err)
		} else {
			m.event(e.spec.ID, "PASS", "isolated bidirectional integrity measurement; production route unchanged")
		}
	case "lock":
		if m.policy.ManualLock != "" && m.policy.ManualLock != e.spec.ID {
			err = errors.New("enrolled manual lock forbids this candidate")
		} else {
			m.runtimeLock = e.spec.ID
			m.mu.Lock()
			m.status.ManualLock = e.spec.ID
			m.mu.Unlock()
		}
	case "disable-carrier":
		m.mu.Lock()
		active := m.active
		m.mu.Unlock()
		if active != nil && active.endpoint == e {
			err = errors.New("select and confirm another carrier before disabling the active one")
		} else {
			m.mu.Lock()
			m.runtimeDisabled[e.spec.ID] = true
			if cancel := m.endpointCancel[e.spec.ID]; cancel != nil {
				cancel()
			}
			m.mu.Unlock()
			e.cancel()
			m.candidateStatus(e.spec.ID, func(s *CandidateStatus) { s.Ready = false; s.Reason = "disabled by local operator until restart" })
		}
	case "switch":
		if !m.eligible(e, time.Now()) {
			err = errors.New("enrolled manual lock forbids this candidate")
		} else {
			err = m.activate(e, "manual peer-coordinated carrier selection")
		}
	default:
		err = errors.New("unknown peer Auto action")
	}
	a.result <- err
}
