package turn

import (
	"errors"
	"zenbot/internal/agent/tool/contract"
)

type ExecutionLimits struct{ MaxSteps, MaxToolCalls int }
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
	limits                                                                  ExecutionLimits
	steps, reserved, attempted, succeeded, failed                           int
	tools                                                                   bool
	commandCorrection, freshnessCorrection, synthesisCorrection, unverified bool
	successfulCommands, failedCommands, successfulTools                     map[string]struct{}
	results                                                                 []contract.Result
}

func NewState(l ExecutionLimits) *State {
	return &State{limits: l, tools: true, successfulCommands: map[string]struct{}{}, failedCommands: map[string]struct{}{}, successfulTools: map[string]struct{}{}}
}
func (s *State) AdvanceStep() bool {
	if s.steps >= s.limits.MaxSteps {
		return false
	}
	s.steps++
	return true
}
func (s *State) ReserveToolCalls(n int) bool {
	if n < 0 || s.reserved+n > s.limits.MaxToolCalls {
		return false
	}
	s.reserved += n
	return true
}
func (s *State) DisableTools()      { s.tools = false }
func (s *State) ToolsEnabled() bool { return s.tools }
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
func (s *State) RecordSuccessfulCommand(v string) bool {
	_, ok := s.successfulCommands[v]
	s.successfulCommands[v] = struct{}{}
	return !ok
}
func (s *State) RecordFailedCommand(v string) bool {
	_, ok := s.failedCommands[v]
	s.failedCommands[v] = struct{}{}
	return !ok
}
func (s *State) RecordSuccessfulTool(v string) bool {
	_, ok := s.successfulTools[v]
	s.successfulTools[v] = struct{}{}
	return !ok
}
func (s *State) HasSuccessfulTool(v string) bool    { _, ok := s.successfulTools[v]; return ok }
func (s *State) HasSuccessfulCommand(v string) bool { _, ok := s.successfulCommands[v]; return ok }
func (s *State) HasAnySuccessfulCommand() bool      { return len(s.successfulCommands) > 0 }
func (s *State) SuccessfulCommands() []string       { return keys(s.successfulCommands) }
func (s *State) FailedCommands() []string           { return keys(s.failedCommands) }
func (s *State) SuccessfulTools() []string          { return keys(s.successfulTools) }
func keys(m map[string]struct{}) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}
func (s *State) CommandCorrectionUsed() bool        { return s.commandCorrection }
func (s *State) MarkCommandCorrectionUsed()         { s.commandCorrection = true }
func (s *State) ClearCommandCorrection()            { s.commandCorrection = false }
func (s *State) FreshnessCorrectionUsed() bool      { return s.freshnessCorrection }
func (s *State) MarkFreshnessCorrectionUsed()       { s.freshnessCorrection = true }
func (s *State) FreshSynthesisCorrectionUsed() bool { return s.synthesisCorrection }
func (s *State) MarkFreshSynthesisCorrectionUsed()  { s.synthesisCorrection = true }
func (s *State) UnverifiedActionChecked() bool      { return s.unverified }
func (s *State) MarkUnverifiedActionChecked()       { s.unverified = true }
func (s *State) ResetUnverifiedActionCheck()        { s.unverified = false }
