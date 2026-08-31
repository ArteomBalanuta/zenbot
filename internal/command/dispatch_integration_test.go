package command

import (
	"encoding/json"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
)

func TestRegisterUserUtilitiesDispatchesEveryAliasThroughChatListener(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice", Hash: "hash"},
	}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		alias string
		want  string
	}{
		{alias: "ping", want: "response time: 0 milliseconds"},
		{alias: "p", want: "response time: 0 milliseconds"},
		{alias: "version", want: "1.0.29"},
		{alias: "v", want: "1.0.29"},
		{alias: "ape", want: "⣀"},
		{alias: "harambe", want: "⣀"},
		{alias: "coin", want: ""},
		{alias: "toss", want: ""},
		{alias: "ct", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			engine.chats = nil
			payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!" + tc.alias})
			listener.NewUserChatListener(engine).Notify(string(payload))
			if len(engine.chats) != 1 {
				t.Fatalf("chats=%v", engine.chats)
			}
			if tc.want != "" && !contains(engine.chats[0], tc.want) {
				t.Fatalf("chat=%q, want substring %q", engine.chats[0], tc.want)
			}
			if tc.alias == "coin" || tc.alias == "toss" || tc.alias == "ct" {
				if engine.chats[0] != "alice|head|false" && engine.chats[0] != "alice|tail|false" {
					t.Fatalf("coin chat=%q", engine.chats[0])
				}
			}
		})
	}
}

func TestRegisterUserUtilitiesDispatchesModerationAliasesAndLeavesLegacyCommandsUnknown(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice", Hash: "hash"},
	}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		alias string
		text  string
		want  string
	}{
		{alias: "ban", text: "!ban merc", want: `{"cmd":"ban","nick":"merc"}`},
		{alias: "unban", text: "!unban hash", want: `{"cmd":"unban","hash":"hash"}`},
		{alias: "unbanall", text: "!unbanall", want: `{"cmd":"unbanall"}`},
		{alias: "pardonall", text: "!pardonall", want: `{"cmd":"unbanall"}`},
		{alias: "lock", text: "!lock on", want: `{"cmd":"lockroom"}`},
		{alias: "lockroom", text: "!lockroom off", want: `{"cmd":"unlockroom"}`},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			engine.chats, engine.raws = nil, nil
			if _, ok := (*engine.GetEnabledCommands())[tc.alias]; !ok {
				t.Fatalf("alias %q was not registered", tc.alias)
			}
			payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: tc.text})
			listener.NewUserChatListener(engine).Notify(string(payload))
			if len(engine.raws) != 1 || engine.raws[0] != tc.want {
				t.Fatalf("raw=%v, want [%s]", engine.raws, tc.want)
			}
		})
	}

	for _, alias := range []string{"kick", "unlock"} {
		if _, ok := (*engine.GetEnabledCommands())[alias]; ok {
			t.Fatalf("unexpectedly registered %q", alias)
		}
		engine.chats, engine.raws = nil, nil
		payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!" + alias})
		listener.NewUserChatListener(engine).Notify(string(payload))
		if len(engine.chats) != 0 || len(engine.raws) != 0 {
			t.Fatalf("%q dispatched: chats=%v raws=%v", alias, engine.chats, engine.raws)
		}
	}
}

func TestRegisterUserUtilitiesDoesNotExposeUnknownOrUnimplementedCommands(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice", Hash: "hash"},
	}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"access", "dbz", "search", "scp", "not-a-command"} {
		if _, ok := (*engine.GetEnabledCommands())[alias]; ok {
			t.Fatalf("unexpectedly registered %q", alias)
		}
		engine.chats = nil
		payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!" + alias})
		listener.NewUserChatListener(engine).Notify(string(payload))
		if len(engine.chats) != 0 {
			t.Fatalf("%q dispatched: %v", alias, engine.chats)
		}
	}
}

func TestMailAndNotesAliasesDispatchThroughListener(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}}}
	for _, alias := range []string{"mail", "msg", "send", "note", "save", "notes"} {
		d, ok := commandDefinitionFor(alias)
		if !ok {
			t.Fatalf("missing %s", alias)
		}
		engine.RegisterCommand(&legacyAdapter{engine: engine, def: d})
		engine.chats = nil
		trip := "trip"
		if alias == "notes" {
			trip = ""
		}
		payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Trip: trip, Text: "!" + alias})
		listener.NewUserChatListener(engine).Notify(string(payload))
		if len(engine.chats) != 1 {
			t.Fatalf("%s chats=%v", alias, engine.chats)
		}
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
