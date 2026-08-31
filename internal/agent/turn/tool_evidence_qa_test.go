package turn

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/repository"
)

type qaEvidenceStore struct {
	*InMemoryStore
	rows []repository.AgentToolEvidence
}

func (s qaEvidenceStore) LoadToolEvidenceContext(context.Context, api.Context) ([]repository.AgentToolEvidence, error) {
	return append([]repository.AgentToolEvidence(nil), s.rows...), nil
}

func TestAppendToolEvidenceContextRejectsUntrustedOrMalformedCandidate(t *testing.T) {
	store := NewMemoryStore()
	memory, err := NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []PersistableEvidence{
		{Tool: "unknown_tool", Content: `{}`},
		{Tool: "room_users", Content: `{}`},
		{Tool: "user_message_history", Content: `{"rows":[]}`},
	} {
		if err := memory.AppendToolEvidenceContext(context.Background(), ctx, []PersistableEvidence{candidate}); !errors.Is(err, ErrInvalidEvidence) {
			t.Fatalf("candidate %#v error = %v, want ErrInvalidEvidence", candidate, err)
		}
	}
}

func TestLoadHistoricalEvidenceContextSkipsInvalidStoredCandidate(t *testing.T) {
	store := qaEvidenceStore{InMemoryStore: NewMemoryStore(), rows: []repository.AgentToolEvidence{
		{ToolName: "unknown_tool", Content: `{}`, CreatedOnMillis: 1},
		{ToolName: "room_users", Content: `{}`, CreatedOnMillis: 2},
		{ToolName: "room_users", Content: `{"room":"room","users":[],"count":0,"returnedCount":0,"truncated":false}`, CreatedOnMillis: 3},
	}}
	memory, err := NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := memory.LoadHistoricalEvidenceContext(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tool != "room_users" || got[0].ObservedAtMillis != 3 {
		t.Fatalf("loaded historical evidence = %#v", got)
	}
}
