package command

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
)

func TestInfoUserCommandParity(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{
		"merc": {Name: "merc", Trip: "trip-a", Hash: "hash-a"},
	}}
	d, ok := commandDefinitionFor("info")
	if !ok || d.Role != model.USER {
		t.Fatalf("info definition=%+v found=%v", d, ok)
	}
	for _, alias := range []string{"info", "i", "whois", "who"} {
		e.chats = nil
		msg := &model.ChatMessage{Name: "testAuthor", Text: "!" + alias + " @MERC ignored", IsWhisper: true}
		status, err := d.New(e, msg).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("%s status=%v err=%v", alias, status, err)
		}
		want := "testAuthor|\n User trip: trip-a\n User hash: hash-a|true"
		if len(e.chats) != 1 || e.chats[0] != want {
			t.Fatalf("%s chat=%q, want %q", alias, e.chats, want)
		}
	}
}

func TestInfoAliasesDispatchThroughRegisteredChatListener(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{
		"merc":       {Name: "merc", Trip: "trip-a", Hash: "hash-a"},
		"testAuthor": {Name: "testAuthor"},
	}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"info", "i", "whois", "who"} {
		e.chats = nil
		payload, _ := json.Marshal(model.ChatMessage{Name: "testAuthor", Text: "!" + alias + " @MeRc"})
		listener.NewUserChatListener(e).Notify(string(payload))
		if len(e.chats) != 1 || e.chats[0] != "testAuthor|\n User trip: trip-a\n User hash: hash-a|false" {
			t.Fatalf("%s dispatched chat=%q", alias, e.chats)
		}
	}
}

func TestInfoUserCommandMissingAndNotFoundMatchSaturn(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{}}
	d, _ := commandDefinitionFor("info")

	status, err := d.New(e, &model.ChatMessage{Name: "author", Text: "!info", IsWhisper: true}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("missing status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 || e.chats[0] != "author|\\n Example: !info merc|true" {
		t.Fatalf("missing chat=%q", e.chats)
	}

	e.chats = nil
	status, err = d.New(e, &model.ChatMessage{Name: "author", Text: "!info @nobody", IsWhisper: true}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("not-found status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 || e.chats[0] != "author|\\n target with nick:  nobody not found!|true" {
		t.Fatalf("not-found chat=%q", e.chats)
	}
}
