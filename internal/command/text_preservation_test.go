package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/listener"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type textDispatchEngine struct {
	*commandEngineStub
	allow    bool
	requests []snapshot.RoomSnapshotRequest
}

func (e *textDispatchEngine) RegisterCommand(c common.Command) {
	if e.commands == nil {
		e.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range c.GetAliases() {
		e.commands[alias] = common.CommandMetadata{Alias: alias, Command: func(m *model.ChatMessage) common.Command { return c.NewInstance(e, m) }}
	}
}
func (e *textDispatchEngine) IsUserAuthorized(_ *model.User, role *model.Role) bool {
	e.authorizationCalls++
	return e.allow && *role != model.ADMIN
}
func (e *textDispatchEngine) SubmitCredentialedRoomSnapshot(req snapshot.RoomSnapshotRequest) error {
	e.requests = append(e.requests, req)
	return nil
}

func dispatchExactText(t *testing.T, engine *textDispatchEngine, alias, tail string) {
	t.Helper()
	definition, ok := commandDefinitionFor(alias)
	if !ok {
		t.Fatal(alias)
	}
	engine.RegisterCommand(&legacyAdapter{engine: engine, def: definition})
	message := model.ChatMessage{Name: "alice", Trip: "trip", Channel: "programming", Text: "!" + alias + " " + tail}
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	listener.NewUserChatListener(engine).NotifyContext(context.Background(), string(raw))
}

func TestTextPreservationDispatchNoteMailAndRemoteMessage(t *testing.T) {
	const want = "line one\n\tline  two: café \\n"
	t.Run("note", func(t *testing.T) {
		base := openNotesParityEngine(t)
		engine := &textDispatchEngine{commandEngineStub: base, allow: true}
		dispatchExactText(t, engine, "save", want)
		var stored string
		if err := base.bundle.Notes.DB.QueryRow("SELECT note FROM notes").Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != want {
			t.Fatalf("stored note=%q want=%q", stored, want)
		}
	})
	t.Run("mail", func(t *testing.T) {
		base, db := openMailGroupCParityEngine(t)
		seedMailGroupCCommandRecipient(t, db)
		base.users["alice"].Trip = "trip"
		engine := &textDispatchEngine{commandEngineStub: base, allow: true}
		dispatchExactText(t, engine, "send", "@Merc   "+want)
		var encoded, decoded string
		if err := db.QueryRow("SELECT message FROM mail").Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(`"`+encoded+`"`), &decoded); err != nil {
			t.Fatal(err)
		}
		// Mail storage adds its existing trailing separator and escapes JSON once.
		if decoded != want+" " {
			t.Fatalf("decoded mail=%q want=%q", decoded, want+" ")
		}
	})
	t.Run("remote message", func(t *testing.T) {
		engine := &textDispatchEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}}}, allow: true}
		dispatchExactText(t, engine, "msgroom", "lounge\t"+want)
		if len(engine.requests) != 1 || engine.requests[0].RemoteMessage != "anonymous mail from: ?programming message: "+want {
			t.Fatalf("requests=%+v", engine.requests)
		}
	})
}

func TestTextPreservationDispatchSayAndAFK(t *testing.T) {
	const want = "line one\n\tline  two: café \\n"
	for _, alias := range []string{"echo", "a"} {
		t.Run(alias, func(t *testing.T) {
			user := &model.User{Name: "alice", Trip: "trip"}
			engine := &textDispatchEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": user}}, allow: true}
			dispatchExactText(t, engine, alias, want)
			if alias == "echo" {
				if len(engine.chats) != 1 || engine.chats[0] != "|"+want+"|false" {
					t.Fatalf("say output=%q", engine.chats)
				}
			} else if engine.afkUsers[user] != want {
				t.Fatalf("afk reason=%q want=%q", engine.afkUsers[user], want)
			}
		})
	}
	t.Run("unauthorized say", func(t *testing.T) {
		engine := &textDispatchEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}}}}
		dispatchExactText(t, engine, "say", want)
		if len(engine.chats) != 1 || !strings.Contains(engine.chats[0], "not authorized") || strings.Contains(engine.chats[0], want) {
			t.Fatalf("unauthorized output=%q", engine.chats)
		}
	})
}
