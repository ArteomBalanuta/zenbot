package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type lastOnlineCommandQueriesStub struct {
	record repository.LastOnlineRecord
	err    error
	calls  int
	target string
}

func (s *lastOnlineCommandQueriesStub) RegisteredUsers(context.Context) ([]repository.RegisteredUser, error) {
	return nil, nil
}
func (s *lastOnlineCommandQueriesStub) NicksByTrip(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *lastOnlineCommandQueriesStub) BasicUserData(context.Context, string, string) (string, error) {
	return "", nil
}
func (s *lastOnlineCommandQueriesStub) LastOnline(_ context.Context, target string) (repository.LastOnlineRecord, error) {
	s.calls++
	s.target = target
	return s.record, s.err
}

func TestLastOnlineCommandUsesFirstNormalizedArgumentAndWhisper(t *testing.T) {
	queries := &lastOnlineCommandQueriesStub{record: repository.LastOnlineRecord{
		Found:          true,
		LastMessage:    sql.NullString{String: "hello", Valid: true},
		LastSeenMillis: sql.NullInt64{Int64: 0, Valid: true},
	}}
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}, bundle: &service.Bundle{Users: &service.UserService{Queries: queries, Now: func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }}}}
	definition, ok := commandDefinitionFor("lastseen")
	if !ok || definition.Role != model.REGULAR {
		t.Fatalf("definition=%+v present=%v", definition, ok)
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!lastseen  @merc ignored", IsWhisper: true}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if queries.calls != 1 || queries.target != "merc" {
		t.Fatalf("calls=%d target=%q", queries.calls, queries.target)
	}
	if len(engine.chats) != 1 || engine.chats[0] != "alice|\\n Nick|Trip: merc\\n Joined:  - \\n Last seen: Thu, 1 Jan 1970 00:00:00 GMT\\n Seen active: 1 days, 0 hours, 0 minutes, 0 seconds ago.\\n Session duration:  -  \\n Last message: hello\\n|true" {
		t.Fatalf("chats=%v", engine.chats)
	}
}

func TestLastOnlineCommandMissingTargetRepliesWithUsageWithoutQuery(t *testing.T) {
	queries := &lastOnlineCommandQueriesStub{}
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}, bundle: &service.Bundle{Users: &service.UserService{Queries: queries}}}
	definition, _ := commandDefinitionFor("lastonline")
	for _, text := range []string{"!lastonline", "!lastonline @"} {
		engine.chats = nil
		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: text}).Execute(context.Background())
		if err != nil || status != model.FAILED {
			t.Fatalf("%q status=%v err=%v", text, status, err)
		}
		if len(engine.chats) != 1 || engine.chats[0] != "alice|\\n Example: !lastseen merc|false" {
			t.Fatalf("%q chats=%v", text, engine.chats)
		}
	}
	if queries.calls != 0 {
		t.Fatalf("unexpected queries=%d", queries.calls)
	}
}

func TestLastOnlineCommandReturnsServiceErrorWithoutReply(t *testing.T) {
	wantErr := errors.New("database unavailable")
	queries := &lastOnlineCommandQueriesStub{err: wantErr}
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}, bundle: &service.Bundle{Users: &service.UserService{Queries: queries}}}
	definition, _ := commandDefinitionFor("lastonline")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!lastonline merc"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, wantErr) || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}
}

func TestLastOnlineAliasesDispatchAgainstRealH2AndRequireQueries(t *testing.T) {
	withoutQueries := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	if err := RegisterUserUtilities(withoutQueries); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"lastonline", "seen", "last", "online", "lastseen"} {
		if _, ok := (*withoutQueries.GetEnabledCommands())[alias]; ok {
			t.Fatalf("registered without queries: %q", alias)
		}
	}

	db := h2fixture.Open(t, "last-online-dispatch")
	for _, statement := range []string{
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','JOINED',1000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','hello',2000,'PUBLIC')",
	} {
		if _, err := db.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}, bundle: &service.Bundle{Users: &service.UserService{Queries: db}}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"lastonline", "seen", "last", "online", "lastseen"} {
		if _, ok := (*engine.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("missing alias %q", alias)
		}
	}
	for _, alias := range []string{"lastseen", "seen"} {
		engine.chats = nil
		payload, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "trip-a", Text: "!" + alias + " merc"})
		if err != nil {
			t.Fatal(err)
		}
		listener.NewUserChatListener(engine).Notify(string(payload))
		if len(engine.chats) != 1 || !contains(engine.chats[0], "\\n Nick|Trip: merc\\n") || !contains(engine.chats[0], "Last message: hello") {
			t.Fatalf("%s chats=%v", alias, engine.chats)
		}
	}
}
