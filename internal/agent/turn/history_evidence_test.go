package turn_test

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/turn"
	"zenbot/internal/repository"
)

type historyEvidenceRepository struct{}

func (historyEvidenceRepository) RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]repository.PublicRoomMessage, error) {
	return nil, nil
}

func TestRealHistoryResultCanBeRetainedAsEvidence(t *testing.T) {
	history := tool.UserMessageHistory{Repository: historyEvidenceRepository{}, Limit: 10}
	descriptor, err := history.Descriptor(api.Context{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := history.Execute(context.Background(), api.Context{}, json.RawMessage(`{"nick":"alice"}`))
	if err != nil || result.IsError {
		t.Fatalf("history result=%#v err=%v", result, err)
	}
	evidence, err := turn.NewPersistableEvidence(descriptor, result)
	if err != nil {
		t.Fatalf("real history result rejected for persistence: %v (%s)", err, result.Content)
	}
	if evidence.Content != result.Content {
		t.Fatalf("evidence changed the original result: %s", evidence.Content)
	}
}
