package turn

import (
	"context"
	"errors"
	"testing"
	"zenbot/internal/agent/llm"
)

func TestStateBoundsFlagsSetsSnapshotsAndEvidence(t *testing.T) {
	s := NewState(ExecutionLimits{MaxSteps: 1, MaxToolCalls: 2})
	if !s.AdvanceStep() || s.AdvanceStep() {
		t.Fatal("step budget")
	}
	if !s.ReserveToolCalls(2) || s.ReserveToolCalls(1) || s.ReserveToolCalls(-1) {
		t.Fatal("tool budget")
	}
	s.DisableTools()
	s.DisableTools()
	if s.ToolsEnabled() {
		t.Fatal("disable")
	}
	s.MarkCommandCorrectionUsed()
	if !s.CommandCorrectionUsed() {
		t.Fatal("flags")
	}
	if !s.RecordSuccessfulCommand("x") || s.RecordSuccessfulCommand("x") || !s.RecordFailedCommand("y") || s.RecordFailedCommand("y") {
		t.Fatal("sets")
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
	got := s.SuccessfulCommands()
	s.RecordSuccessfulCommand("z")
	if len(got) != 1 {
		t.Fatal("snapshot")
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
func TestPolicyChainCarriesResponseAndStops(t *testing.T) {
	p := NewPolicyChain([]Policy{PolicyFunc(func(context.Context, PolicyInput) (PolicyResult, error) {
		return Continue(llm.NewLlmResponse("one", nil, "stop"), false), nil
	}), PolicyFunc(func(context.Context, PolicyInput) (PolicyResult, error) {
		return Stop(llm.NewLlmResponse("two", nil, "stop")), nil
	}), PolicyFunc(func(context.Context, PolicyInput) (PolicyResult, error) {
		t.Fatal("ran")
		return Continue(llm.NewLlmResponse("bad", nil, "stop"), false), nil
	})})
	r, err := p.Apply(context.Background(), PolicyInput{Response: llm.NewLlmResponse("start", nil, "stop")})
	if err != nil || r.Response.Content() != "two" || r.Continue {
		t.Fatalf("%+v %v", r, err)
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
