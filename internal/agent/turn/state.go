package turn

import (
	"errors"
	"time"
)

type ExecutionLimits struct {
	MaxSteps, MaxToolCalls, MaxCallsPerTool, MaxToolFailures int
	ToolTimeout                                              time.Duration
}
type Evidence struct {
	Attempted                                    bool
	AttemptedCount, SuccessfulCount, FailedCount int
}

func NewEvidence(attempted bool, attemptedCount, successfulCount, failedCount int) (Evidence, error) {
	if attemptedCount < 0 || successfulCount < 0 || failedCount < 0 || successfulCount+failedCount != attemptedCount || attempted != (attemptedCount > 0) {
		return Evidence{}, errors.New("inconsistent tool evidence")
	}
	return Evidence{attempted, attemptedCount, successfulCount, failedCount}, nil
}

type State struct {
	limits                                        ExecutionLimits
	steps, reserved, attempted, succeeded, failed int
}

func NewState(l ExecutionLimits) *State {
	return &State{limits: l}
}
func (s *State) AdvanceStep() bool {
	if s.steps >= s.limits.MaxSteps {
		return false
	}
	s.steps++
	return true
}
func (s *State) RemainingSteps() int {
	remaining := s.limits.MaxSteps - s.steps
	if remaining < 0 {
		return 0
	}
	return remaining
}
func (s *State) ReserveToolCalls(n int) bool {
	if n < 0 || s.reserved+n > s.limits.MaxToolCalls {
		return false
	}
	s.reserved += n
	return true
}
func (s *State) RemainingToolCalls() int { return s.limits.MaxToolCalls - s.reserved }
func (s *State) MarkToolAttempted(n int) error {
	if n < 0 {
		return errors.New("tool attempt count must not be negative")
	}
	s.attempted += n
	return nil
}
func (s *State) RecordToolSuccess() error {
	if s.succeeded+s.failed >= s.attempted {
		return errors.New("tool result exceeds attempted tool count")
	}
	s.succeeded++
	return nil
}
func (s *State) RecordToolFailure() error {
	if s.succeeded+s.failed >= s.attempted {
		return errors.New("tool result exceeds attempted tool count")
	}
	s.failed++
	return nil
}
func (s *State) Evidence() Evidence {
	return Evidence{s.attempted > 0, s.attempted, s.succeeded, s.failed}
}
