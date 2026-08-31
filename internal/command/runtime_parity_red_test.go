package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type runtimeParityGroupB struct {
	lastName *string
	lastTrip string
	lastN    int
	messages []repository.SaturnLastMessage
	users    []repository.SaturnRegisteredUser
	delete   []string
	err      error
}

func (r *runtimeParityGroupB) DeleteIdentity(context.Context, string, string) (repository.DeleteResult, error) {
	return repository.DeleteResult{}, errors.New("legacy delete must not be called")
}
func (r *runtimeParityGroupB) SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error) {
	return r.users, r.err
}
func (r *runtimeParityGroupB) SaturnLastMessages(_ context.Context, name *string, trip string, n int) ([]repository.SaturnLastMessage, error) {
	r.lastName, r.lastTrip, r.lastN = name, trip, n
	return r.messages, r.err
}
func (r *runtimeParityGroupB) DeleteIdentityAuthorized(_ context.Context, selector string) (repository.DeleteResult, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" || selector == "missing" || selector == "ambiguous" {
		return repository.DeleteResult{}, errors.New("identity selector must match exactly one registered user")
	}
	r.delete = append(r.delete, selector)
	return repository.DeleteResult{TripRows: 1}, r.err
}

func runtimeParityEngine(g *runtimeParityGroupB) *commandEngineStub {
	return &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{GroupB: g}, Mail: &service.MailService{GroupB: g}}, users: map[string]*model.User{}}
}

func TestRuntimeParityRemoveAliasesAreRegistered(t *testing.T) {
	e := runtimeParityEngine(&runtimeParityGroupB{})
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"remove", "del", "delete"} {
		if _, ok := (*e.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("alias %q not registered", alias)
		}
	}
}

func TestRuntimeParityRemoveAliasesExecuteRealDelete(t *testing.T) {
	g := &runtimeParityGroupB{}
	e := runtimeParityEngine(g)
	for _, alias := range []string{"remove", "del", "delete"} {
		e.chats = nil
		def, ok := commandDefinitionFor(alias)
		if !ok {
			t.Fatalf("missing alias %q", alias)
		}
		status, err := def.New(e, &model.ChatMessage{Text: "!" + alias + "  Merc ", Name: "mod"}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("%s: status=%v err=%v", alias, status, err)
		}
		if len(g.delete) != 1 || g.delete[0] != "Merc" {
			t.Fatalf("%s delete calls=%v", alias, g.delete)
		}
		if len(e.chats) != 1 || e.chats[0] != "mod|User has been removed successfully|false" {
			t.Fatalf("%s output=%v", alias, e.chats)
		}
		g.delete = nil
	}
}

func TestRuntimeParityRemoveInvalidSelectorsDoNotDelete(t *testing.T) {
	for _, selector := range []string{"", "missing", "ambiguous"} {
		g := &runtimeParityGroupB{}
		e := runtimeParityEngine(g)
		status, err := newCommand("remove", []string{"del", "delete", "remove"}, model.MODERATOR, e, &model.ChatMessage{Text: "!remove " + selector}).Execute(context.Background())
		if status != model.FAILED || err == nil {
			t.Fatalf("selector %q: status=%v err=%v", selector, status, err)
		}
		if len(g.delete) != 0 {
			t.Fatalf("selector %q deleted: %v", selector, g.delete)
		}
	}
}

func TestRuntimeParityMessagesUsesGroupBAndAdaptsTrip(t *testing.T) {
	long := "<" + string(make([]byte, 201))
	g := &runtimeParityGroupB{messages: []repository.SaturnLastMessage{{Name: "Merc", Message: long}}}
	e := runtimeParityEngine(g)
	status, err := newCommand("messages", []string{"messages", "lastmessages"}, model.MODERATOR, e, &model.ChatMessage{Text: "!messages trip 0"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if g.lastName != nil || g.lastTrip != "trip" || g.lastN != 0 {
		t.Fatalf("Group B args=(%v,%q,%d)", g.lastName, g.lastTrip, g.lastN)
	}
	if len(e.chats) != 1 || len(e.chats[0]) == 0 {
		t.Fatalf("missing output: %v", e.chats)
	}
}

func TestRuntimeParityUsersUsesGroupB(t *testing.T) {
	g := &runtimeParityGroupB{users: []repository.SaturnRegisteredUser{{Name: "Merc", Trip: "trip"}}}
	e := runtimeParityEngine(g)
	status, err := newCommand("users", []string{"users"}, model.USER, e, &model.ChatMessage{Text: "!users"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL || len(e.chats) != 1 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
	if e.chats[0] == "" || !runtimeContains(e.chats[0], "Merc") {
		t.Fatalf("output=%q", e.chats[0])
	}
}

func TestRuntimeParityMailDirectoryPreservesSaturnEscapedNewlines(t *testing.T) {
	got := formatSaturnRegisteredUsers([]repository.SaturnRegisteredUser{{Name: "Merc", Trip: "trip"}})
	if got != "Merc trip\\n" {
		t.Fatalf("directory=%q, want Saturn escaped newline", got)
	}
}

func runtimeContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || runtimeIndex(s, sub) >= 0)
}
func runtimeIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
