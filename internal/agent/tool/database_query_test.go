package tool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agenttool "zenbot/internal/agent/tool"
)

type namedQueryRepositoryStub struct {
	calls int
	name  string
}

func (r *namedQueryRepositoryStub) ExecuteAgentQuery(_ context.Context, name string, _ json.RawMessage, _, _ string) (json.RawMessage, error) {
	r.calls++
	r.name = name
	return json.RawMessage(`{"count":1}`), nil
}

func TestDatabaseQueryRemovesOverlappingHistoryAndNicknameRoutes(t *testing.T) {
	repository := &namedQueryRepositoryStub{}
	database := agenttool.DatabaseQuery{Repository: repository}
	descriptor, err := database.Descriptor(historyContext(t, "programming"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"recent_messages_for_user", "known_nicks_for_trip"} {
		if strings.Contains(string(descriptor.Parameters()), forbidden) {
			t.Fatalf("overlapping query %q remains model-visible: %s", forbidden, descriptor.Parameters())
		}
		result, err := database.Execute(context.Background(), historyContext(t, "programming"), json.RawMessage(`{"query":"`+forbidden+`","nick":"alice","trip":"trip"}`))
		if err != nil || !result.IsError || result.ErrorCode != "INVALID_ARGUMENTS" {
			t.Fatalf("query=%q result=%#v err=%v", forbidden, result, err)
		}
	}
	if repository.calls != 0 {
		t.Fatalf("removed routes reached repository %d times", repository.calls)
	}

	result, err := database.Execute(context.Background(), historyContext(t, "programming"), json.RawMessage(`{"query":"message_count"}`))
	if err != nil || result.IsError || repository.calls != 1 || repository.name != "message_count" {
		t.Fatalf("allowed result=%#v err=%v repository=%#v", result, err, repository)
	}
}
