package turn

import (
	"testing"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
)

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
