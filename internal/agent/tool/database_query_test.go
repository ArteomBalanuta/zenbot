package tool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

type namedQueryRepositoryStub struct {
	calls int
	name  string
}

func TestDatabaseQueryConditionalBranchesRequireOnlyRelevantSelectors(t *testing.T) {
	repository := &namedQueryRepositoryStub{}
	database := agenttool.DatabaseQuery{Repository: repository}
	descriptor, err := database.Descriptor(historyContext(t, "programming"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "message count", input: `{"query":"message_count"}`, valid: true},
		{name: "message count rejects limit", input: `{"query":"message_count","limit":1}`},
		{name: "registered count rejects room", input: `{"query":"registered_user_count","room":"programming"}`},
		{name: "requester history with limit", input: `{"query":"recent_messages_for_requester","limit":10}`, valid: true},
		{name: "requester history rejects trip override", input: `{"query":"recent_messages_for_requester","trip":"other"}`},
		{name: "room history", input: `{"query":"recent_messages_for_room","room":"lounge","limit":10}`, valid: true},
		{name: "room history requires room", input: `{"query":"recent_messages_for_room","limit":10}`},
		{name: "room history rejects nick", input: `{"query":"recent_messages_for_room","room":"lounge","nick":"alice"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.input)
			schemaErr := contract.ValidateArguments(descriptor.Parameters(), raw)
			if tc.valid {
				if schemaErr != nil {
					t.Fatalf("valid branch rejected: %v", schemaErr)
				}
				result, err := database.Execute(context.Background(), historyContext(t, "programming"), raw)
				if err != nil || result.IsError {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				return
			}
			if schemaErr == nil {
				t.Fatal("irrelevant or missing selector was accepted")
			}
			before := repository.calls
			result, err := database.Execute(context.Background(), historyContext(t, "programming"), raw)
			if err != nil || !result.IsError || result.ErrorCode != "INVALID_ARGUMENTS" || repository.calls != before {
				t.Fatalf("result=%#v err=%v calls=%d/%d", result, err, before, repository.calls)
			}
		})
	}
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
