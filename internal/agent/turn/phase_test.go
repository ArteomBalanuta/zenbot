package turn

import "testing"

func TestPhaseMachineAllowsOnlyClosedTurnTransitions(t *testing.T) {
	machine := NewPhaseMachine()
	for _, next := range []Phase{PhaseModel, PhasePlan, PhaseGate, PhaseExecute, PhaseObserve, PhaseModel, PhaseFinalize, PhaseComplete} {
		if err := machine.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
	}
	if machine.Phase() != PhaseComplete {
		t.Fatalf("phase=%s, want complete", machine.Phase())
	}
	if err := machine.Transition(PhaseModel); err == nil {
		t.Fatal("terminal state accepted another transition")
	}
}

func TestPhaseMachineRejectsSkippedAndRegressiveTransitions(t *testing.T) {
	for _, tc := range []struct {
		name string
		path []Phase
		next Phase
	}{
		{name: "skip planning", next: PhaseExecute},
		{name: "regress from planning", path: []Phase{PhaseModel, PhasePlan}, next: PhaseModel},
		{name: "execute after finalizing", path: []Phase{PhaseModel, PhaseFinalize}, next: PhaseExecute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := NewPhaseMachine()
			for _, phase := range tc.path {
				if err := machine.Transition(phase); err != nil {
					t.Fatalf("setup transition to %s: %v", phase, err)
				}
			}
			if err := machine.Transition(tc.next); err == nil {
				t.Fatalf("invalid transition %s -> %s succeeded", machine.Phase(), tc.next)
			}
		})
	}
}

func TestPhaseMachineSupportsPauseAndResumeAtGate(t *testing.T) {
	machine := NewPhaseMachine()
	for _, next := range []Phase{PhaseModel, PhasePlan, PhaseGate, PhasePaused, PhaseGate, PhaseExecute} {
		if err := machine.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
	}
}
