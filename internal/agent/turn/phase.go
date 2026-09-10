package turn

import (
	"fmt"
	"sync"
)

type Phase string

const (
	PhaseAssemble Phase = "ASSEMBLE"
	PhaseModel    Phase = "MODEL"
	PhasePlan     Phase = "PLAN"
	PhaseGate     Phase = "GATE"
	PhaseExecute  Phase = "EXECUTE"
	PhaseObserve  Phase = "OBSERVE"
	PhaseReflect  Phase = "REFLECT"
	PhaseFinalize Phase = "FINALIZE"
	PhasePaused   Phase = "PAUSED"
	PhaseComplete Phase = "COMPLETE"
	PhaseFailed   Phase = "FAILED"
)

var phaseTransitions = map[Phase]map[Phase]struct{}{
	PhaseAssemble: {PhaseModel: {}, PhaseFailed: {}},
	PhaseModel:    {PhasePlan: {}, PhaseReflect: {}, PhaseFinalize: {}, PhaseFailed: {}},
	PhasePlan:     {PhaseGate: {}, PhaseFailed: {}},
	PhaseGate:     {PhaseExecute: {}, PhaseObserve: {}, PhasePaused: {}, PhaseFailed: {}},
	PhaseExecute:  {PhaseObserve: {}, PhaseFailed: {}},
	PhaseObserve:  {PhaseModel: {}, PhaseReflect: {}, PhaseFinalize: {}, PhaseFailed: {}},
	PhaseReflect:  {PhaseModel: {}, PhaseFinalize: {}, PhaseFailed: {}},
	PhaseFinalize: {PhaseComplete: {}, PhaseReflect: {}, PhaseFailed: {}},
	PhasePaused:   {PhaseGate: {}, PhaseFailed: {}},
}

type PhaseMachine struct {
	mu    sync.RWMutex
	phase Phase
}

func NewPhaseMachine() *PhaseMachine { return &PhaseMachine{phase: PhaseAssemble} }

func (m *PhaseMachine) Phase() Phase {
	if m == nil {
		return PhaseFailed
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase
}

func (m *PhaseMachine) Transition(next Phase) error {
	if m == nil {
		return fmt.Errorf("phase machine is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, allowed := phaseTransitions[m.phase][next]; !allowed {
		return fmt.Errorf("invalid turn transition %s -> %s", m.phase, next)
	}
	m.phase = next
	return nil
}
