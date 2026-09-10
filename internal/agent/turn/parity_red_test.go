package turn

import (
	"context"
	"errors"
	"testing"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
)

type parityGuard struct{ ok bool }

func (g parityGuard) FindCommand(string) (string, bool) { return "do", g.ok }

type parityCorrector struct{ calls int }

func (c *parityCorrector) CorrectUnverifiedAction(context.Context, llm.LlmResponse, []llm.LlmMessage, []contract.Definition, string) (llm.LlmResponse, []llm.LlmMessage, error) {
	c.calls++
	return llm.NewLlmResponse("corrected", nil, "stop"), []llm.LlmMessage{llm.NewLlmMessage("user", "correction", nil, "")}, nil
}

type parityPolicy struct{ seen PolicyInput }

func (p *parityPolicy) Apply(_ context.Context, i PolicyInput) (PolicyResult, error) {
	p.seen = i
	return Continue(llm.NewLlmResponse("next", nil, "stop"), false), nil
}

func TestParityPolicyInputSnapshotsAndChainCarriesAllFields(t *testing.T) {
	st := NewState(ExecutionLimits{})
	defs := []contract.Definition{{Name: "x", Description: "x", Parameters: []byte(`{"type":"object"}`)}}
	in, err := NewPolicyInput(llm.NewLlmResponse("start", nil, "stop"), []llm.LlmMessage{llm.NewLlmMessage("user", "p", nil, "")}, defs, parityGuard{}, st, "prompt", "corr")
	if err != nil {
		t.Fatal(err)
	}
	defs[0].Parameters[0] = 'X'
	p := &parityPolicy{}
	chain := NewPolicyChain([]Policy{p})
	got, err := chain.Apply(context.Background(), in)
	if err != nil || got.Response.Content() != "next" {
		t.Fatalf("%+v %v", got, err)
	}
	if p.seen.State != st || p.seen.Prompt != "prompt" || p.seen.CorrelationID != "corr" || p.seen.CommandProseGuard == nil || p.seen.Definitions[0].Parameters[0] != '{' {
		t.Fatal("fields not preserved")
	}
}

func TestParityUnverifiedActionCorrectsOnceAndReset(t *testing.T) {
	st := NewState(ExecutionLimits{})
	corr := &parityCorrector{}
	p, err := NewUnverifiedActionPolicy(parityGuard{ok: false}, corr)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := NewPolicyInput(llm.NewLlmResponse("I did it", nil, "stop"), []llm.LlmMessage{}, []contract.Definition{}, parityGuard{}, st, "p", "c")
	r, err := p.Apply(context.Background(), in)
	if err != nil || r.Response.Content() != "corrected" || corr.calls != 1 || !st.UnverifiedActionChecked() {
		t.Fatalf("%+v %v", r, err)
	}
	_, _ = p.Apply(context.Background(), in)
	if corr.calls != 1 {
		t.Fatal("uncapped correction")
	}
	st.ResetUnverifiedActionCheck()
	_, _ = p.Apply(context.Background(), in)
	if corr.calls != 2 {
		t.Fatal("reset did not permit correction")
	}
}

func TestParityMemoryEvidenceUsesContextBucket(t *testing.T) {
	m := NewMemoryStore()
	a, _ := api.NewContext("room-a", "u", "", "", false, []string{})
	b, _ := api.NewContext("room-b", "u", "", "", false, []string{})
	if err := m.AppendEvidenceFor(a, []EvidenceEntry{{Tool: "a", Content: "one"}}); err != nil {
		t.Fatal(err)
	}
	if len(m.EvidenceFor(a)) != 1 || len(m.EvidenceFor(b)) != 0 {
		t.Fatal("cross-key evidence")
	}
}

func TestParityTurnMemoryRemovesPrecedingUserForLegacyAssistant(t *testing.T) {
	store := &memoryFixture{history: []llm.LlmMessage{
		llm.NewLlmMessage("user", "tell me about jill", nil, ""),
		llm.NewLlmMessage("assistant", "*[sips tea]* The archives reveal a user.", nil, ""),
		llm.NewLlmMessage("user", "ordinary", nil, ""),
	}}
	memory, err := NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	got, err := memory.Load(api.Context{}, "corr")
	if err != nil || len(got) != 1 || got[0].Content() != "ordinary" {
		t.Fatalf("legacy pair not removed: %#v err=%v", got, err)
	}
}

type memoryFixture struct{ history []llm.LlmMessage }

func (m *memoryFixture) Load(api.Context) ([]llm.LlmMessage, error)           { return m.history, nil }
func (m *memoryFixture) Append(api.Context, string, string) error             { return nil }
func (m *memoryFixture) AppendToolEvidence(api.Context, string, string) error { return nil }

var _ = errors.Is
