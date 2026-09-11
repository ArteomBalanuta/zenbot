package turn

import (
	"errors"
	"testing"
)

func TestStateBoundsAndEvidence(t *testing.T) {
	s := NewState(ExecutionLimits{MaxSteps: 1, MaxToolCalls: 2})
	if !s.AdvanceStep() || s.AdvanceStep() {
		t.Fatal("step budget")
	}
	if !s.ReserveToolCalls(2) || s.ReserveToolCalls(1) || s.ReserveToolCalls(-1) {
		t.Fatal("tool budget")
	}
	s.MarkToolAttempted(2)
	if err := s.RecordToolSuccess(); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolFailure(); err != nil {
		t.Fatal(err)
	}
	if s.Evidence() != (Evidence{Attempted: true, AttemptedCount: 2, SuccessfulCount: 1, FailedCount: 1}) {
		t.Fatal(s.Evidence())
	}
}
func TestEvidenceRejectsInvalidAndOverrecord(t *testing.T) {
	if _, err := NewEvidence(true, 1, 1, 1); err == nil {
		t.Fatal("invalid")
	}
	s := NewState(ExecutionLimits{})
	s.MarkToolAttempted(1)
	_ = s.RecordToolSuccess()
	if s.RecordToolFailure() == nil {
		t.Fatal("over")
	}
}
func TestMemoryPrevalidatesAndRedacts(t *testing.T) {
	m := NewMemoryStore()
	if err := m.AppendEvidence([]EvidenceEntry{{Tool: "a", Content: "1"}, {Tool: "", Content: "2"}}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal(err)
	}
	if len(m.Evidence()) != 0 {
		t.Fatal("partial")
	}
}
