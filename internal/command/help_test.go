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

func TestRegisteredHelpDescribesOnlySupportedTruthfulRoutes(t *testing.T) {
	payload := executeRegisteredHelp(t)
	logical := strings.ReplaceAll(payload, "\u2009", "")

	for _, unsupported := range []string{"mine <room>", "whiskey <channel>"} {
		if strings.Contains(logical, unsupported) {
			t.Errorf("help advertises unsupported route %q", unsupported)
		}
	}
	for _, truthful := range []string{
		"mem,memory,memstats- shows Go runtime memory values in MiB",
		"restart,reload,re- submits a request to restart this host only; completion is unconfirmed",
		"shutdown,exit,quit- submits a request to shut down this host only; completion is unconfirmed",
		"move,recover,heal,resurrect <nick> <source> <destination>- requests moving a user between rooms",
		"nuke <room>- requests permanent bans for every active user in a room, then locks it",
	} {
		if !strings.Contains(logical, truthful) {
			t.Errorf("help is missing truthful route %q", truthful)
		}
	}
	if strings.Contains(logical, "last kicked user") {
		t.Error("help advertises the removed no-argument resurrect shortcut")
	}
}

func TestRegisteredHelpListsPublicMsgChannelOnceInUserCommands(t *testing.T) {
	payload := executeRegisteredHelp(t)
	logical := strings.ReplaceAll(payload, "\u2009", "")
	want := "msgchannel,msgroom <room> <text>- requests an anonymous message relay to another room"
	if count := strings.Count(logical, want); count != 1 {
		t.Fatalf("public msgchannel row count=%d, want 1 in payload %q", count, payload)
	}
	adminEnd := strings.Index(logical, "Moderator commands:")
	userStart := strings.Index(logical, "User commands:")
	if adminEnd < 0 || userStart < 0 || adminEnd >= userStart {
		t.Fatalf("help section boundaries adminEnd=%d userStart=%d", adminEnd, userStart)
	}
	if strings.Contains(logical[:adminEnd], "msgchannel") || !strings.Contains(logical[userStart:], want) {
		t.Fatalf("msgchannel is not classified solely as a public user command: %q", payload)
	}
}

func TestAlignHelpProcessesActualNewlinesAndPreservesNonCommandRows(t *testing.T) {
	input := "\u9577\u547d\u4ee4 - first\nx - second\n\nExample: !x\n"
	want := "\u9577\u547d\u4ee4 - first\\nx \u2009\u2009- second\\n\\nExample: !x\\n"
	if got := alignHelp(input); got != want {
		t.Fatalf("alignHelp() = %q, want %q", got, want)
	}
}

func executeRegisteredHelp(t *testing.T) string {
	t.Helper()
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	metadata, ok := (*engine.GetEnabledCommands())["help"]
	if !ok {
		t.Fatal("registered help route is missing")
	}
	metadata.Command(&model.ChatMessage{Name: "alice", Text: "!help"}).Execute()
	if len(engine.chats) != 1 {
		t.Fatalf("registered help deliveries=%q", engine.chats)
	}
	return strings.TrimSuffix(strings.TrimPrefix(engine.chats[0], "alice|"), "|true")
}
