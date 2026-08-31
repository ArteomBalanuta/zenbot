package command

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
)

func TestSubscriptionCatalogHasSaturnAliasesAndRoles(t *testing.T) {
	for _, tc := range []struct {
		alias string
		role  model.Role
	}{
		{"sub", model.USER}, {"subscribe", model.USER}, {"unsub", model.USER}, {"unsubscribe", model.USER},
	} {
		d, ok := commandDefinitionFor(tc.alias)
		if !ok || d.Role != tc.role {
			t.Fatalf("alias=%q definition=%v ok=%v, want role %v", tc.alias, d, ok, tc.role)
		}
	}
}

func TestSubscriptionsMatchSaturnFailuresIdempotenceAndAcknowledgementVisibility(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}

	// Missing trip fails and does not mutate subscriptions.
	for _, alias := range []string{"sub", "subscribe"} {
		e.chats = nil
		p, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!" + alias})
		listener.NewUserChatListener(e).Notify(string(p))
		if len(e.chats) != 1 || e.chats[0] != "alice|you have to set your trip to use this command.|false" {
			t.Fatalf("%s: %v", alias, e.chats)
		}
	}
	if len(e.subs) != 0 {
		t.Fatalf("missing-trip subscription mutated: %v", e.subs)
	}

	// Public and whisper acknowledgements preserve inbound visibility.
	for _, whisper := range []bool{false, true} {
		e.chats = nil
		p, _ := json.Marshal(model.ChatMessage{Name: "alice", Trip: "Trip-A", Text: "!subscribe", Type: whisperType(whisper)})
		listener.NewUserChatListener(e).Notify(string(p))
		if len(e.chats) != 1 || e.chats[0] != "alice|your trip will be whispered hashes and nicks for each new joining user. |"+boolString(whisper) {
			t.Fatalf("subscribe whisper=%v: %v", whisper, e.chats)
		}
	}
	if len(e.subs) != 1 {
		t.Fatalf("subscribe must be idempotent: %v", e.subs)
	}

	// Unsubscribe succeeds once, then exactly follows Saturn's absent-trip failure.
	e.chats = nil
	p, _ := json.Marshal(model.ChatMessage{Name: "alice", Trip: "Trip-A", Text: "!unsub", Type: "whisper"})
	listener.NewUserChatListener(e).Notify(string(p))
	if len(e.chats) != 1 || e.chats[0] != "alice|your trip will no longer receive hashes and nicks for each new joining user. |true" {
		t.Fatalf("unsubscribe: %v", e.chats)
	}
	if len(e.subs) != 0 {
		t.Fatalf("unsubscribe did not remove: %v", e.subs)
	}
	d, ok := commandDefinitionFor("unsubscribe")
	if !ok {
		t.Fatal("missing unsubscribe definition")
	}
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "Trip-A", Text: "!unsubscribe"}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("absent unsubscribe status=%v err=%v chats=%v", status, err, e.chats)
	}
}
