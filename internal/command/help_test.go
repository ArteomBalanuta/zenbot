package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
)

func TestHelpUsesExactSaturnPayloadAndForcedWhisper(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	msg := &model.ChatMessage{Name: "alice", Text: "!help", IsWhisper: false}
	d, ok := commandDefinitionFor("help")
	if !ok {
		t.Fatal("missing help definition")
	}
	status, err := d.New(e, msg).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 {
		t.Fatalf("chats=%v", e.chats)
	}
	want := strings.Join([]string{
		fmtHelp(helpHeader, "!"), alignHelp(adminCommands),
		`         \n Moderator commands:\n`, alignHelp(moderatorCommands),
		`         \n User commands:\n`, alignHelp(userCommands),
		fmtHelp(helpExamples, "!", "!", "!", "!", "!", "!"),
	}, "")
	want = strings.ReplaceAll(want, `\\n`, `\n`)
	if got := e.chats[0]; got != "alice|"+want+"|true" {
		t.Fatalf("help payload mismatch: got %q, want %q", got, "alice|"+want+"|true")
	}
	if strings.Contains("/whisper @alice "+strings.ReplaceAll(want, `\\n`, "\\n"), ".\\n") {
		t.Fatal("forced whisper contains legacy dot separator")
	}
}

func TestHelpAliasesPrefixExpansionAndDispatch(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"help", "h"} {
		e.chats = nil
		payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!" + alias, IsWhisper: true})
		listener.NewUserChatListener(e).Notify(string(payload))
		if len(e.chats) != 1 || !strings.HasSuffix(e.chats[0], "|true") {
			t.Fatalf("%s dispatch=%q", alias, e.chats)
		}
	}
	for _, alias := range []string{"crashcourse", "howto", "moderationcrashcourse", "hcguide"} {
		if _, ok := (*e.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("alias %q was not registered", alias)
		}
	}
}
