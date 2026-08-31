package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/repository"
)

type historyRepositoryStub struct {
	rows       []repository.PublicRoomMessage
	err        error
	room, nick string
	limit      int
	calls      int
}

func (s *historyRepositoryStub) RecentPublicRoomMessagesForNick(_ context.Context, room, nick string, limit int) ([]repository.PublicRoomMessage, error) {
	s.calls++
	s.room, s.nick, s.limit = room, nick, limit
	return s.rows, s.err
}

func historyContext(t *testing.T, room string) api.Context {
	t.Helper()
	c, err := api.NewContext(room, "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUserMessageHistoryDescriptorAndRestrictedJSONResult(t *testing.T) {
	repo := &historyRepositoryStub{rows: []repository.PublicRoomMessage{{Name: "Alice", Trip: "secret-trip", Hash: "secret-hash", Message: `quoted " data`, CreatedOnMillis: 7, Channel: "Lounge"}}}
	tool := agenttool.UserMessageHistory{Repository: repo, Limit: 3}
	d, err := tool.Descriptor(historyContext(t, "Lounge"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "user_message_history" || !d.IsReadOnly() || d.Timeout() <= 0 {
		t.Fatalf("descriptor = %#v", d)
	}
	var schema struct {
		Additional bool                      `json:"additionalProperties"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(d.Parameters(), &schema); err != nil {
		t.Fatalf("closed descriptor schema = %s err=%v", d.Parameters(), err)
	}
	nickSchema := schema.Properties["nick"]
	if schema.Additional || len(schema.Required) != 1 || schema.Required[0] != "nick" || nickSchema["type"] != "string" || nickSchema["minLength"] != float64(1) || nickSchema["maxLength"] != float64(100) {
		t.Fatalf("closed descriptor schema = %s", d.Parameters())
	}
	r, err := tool.Execute(context.Background(), historyContext(t, "Lounge"), json.RawMessage(`{"nick":" @ALICE "}`))
	if err != nil {
		t.Fatal(err)
	}
	if repo.room != "Lounge" || repo.nick != "ALICE" || repo.limit != 3 {
		t.Fatalf("trusted scope = %#v", repo)
	}
	if strings.Contains(r.Content, "secret-trip") || strings.Contains(r.Content, "secret-hash") {
		t.Fatalf("restricted JSON leaked identifiers: %s", r.Content)
	}
	if r.IsError || !strings.Contains(r.Content, `"returnedCount":1`) {
		t.Fatalf("result=%#v", r)
	}
}

func TestUserMessageHistoryRejectsBlankAndHidesRepositoryError(t *testing.T) {
	repo := &historyRepositoryStub{err: errors.New("driver password secret")}
	tool := agenttool.UserMessageHistory{Repository: repo, Limit: 1}
	if _, err := tool.Execute(context.Background(), historyContext(t, "room"), json.RawMessage(`{"nick":" @ "}`)); err == nil || repo.calls != 0 {
		t.Fatalf("blank call=%d err=%v", repo.calls, err)
	}
	r, err := tool.Execute(context.Background(), historyContext(t, "room"), json.RawMessage(`{"nick":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || r.ErrorCode != "TOOL_EXECUTION_FAILED" || strings.Contains(r.Content, "driver") {
		t.Fatalf("repository error leaked: %#v", r)
	}
}
