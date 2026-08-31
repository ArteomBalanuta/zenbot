package command

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/model"
)

func TestUserUtilityClusterMetadataAndAliases(t *testing.T) {
	want := map[string][]string{
		"ping":        {"ping", "p"},
		"version":     {"version", "v"},
		"ape":         {"ape", "harambe"},
		"coin":        {"coin", "toss", "ct"},
		"crashcourse": {"crashcourse", "howto", "moderationcrashcourse", "hcguide"},
	}
	for canonical, aliases := range want {
		d, ok := commandDefinitionFor(canonical)
		if !ok || d.Role != model.USER {
			t.Fatalf("%s definition=%+v found=%v", canonical, d, ok)
		}
		if strings.Join(d.Aliases, ",") != strings.Join(aliases, ",") {
			t.Fatalf("%s aliases=%v want %v", canonical, d.Aliases, aliases)
		}
	}
}

func TestHowToUsesExactSaturnAddressedOutput(t *testing.T) {
	wantPayload := "hack.chat moderation guide \n In case spammer or a ~~valid~~ nasty user joined: \n https://youtu.be/E_Yl9ul3Ulw"
	for _, tc := range []struct {
		alias, want string
		whisper     bool
	}{
		{"crashcourse", "@alice " + wantPayload, false},
		{"howto", "/whisper @alice " + wantPayload, true},
		{"moderationcrashcourse", "@alice " + wantPayload, false},
		{"hcguide", "/whisper @alice " + wantPayload, true},
	} {
		e := &commandEngineStub{}
		d, ok := commandDefinitionFor(tc.alias)
		if !ok || d.Role != model.USER {
			t.Fatalf("bad definition for %s: %+v", tc.alias, d)
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + tc.alias, IsWhisper: tc.whisper}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("%s status=%v err=%v", tc.alias, status, err)
		}
		if len(e.chats) != 1 || e.chats[0] != tc.want {
			t.Fatalf("%s output=%q, want %q", tc.alias, e.chats, tc.want)
		}
	}
}

func TestPingVersionApeAndCoinParity(t *testing.T) {
	cases := []struct {
		alias string
		text  string
		check func(t *testing.T, chats []string)
	}{
		{"p", "!p ignored arguments", func(t *testing.T, chats []string) {
			if len(chats) != 1 || chats[0] != "alice|response time: 0 milliseconds|false" {
				t.Fatalf("ping output=%v", chats)
			}
		}},
		{"v", "!v extra", func(t *testing.T, chats []string) {
			if len(chats) != 1 || chats[0] != "alice|1.0.29|true" {
				t.Fatalf("version output=%v", chats)
			}
		}},
		{"harambe", "!harambe extra", func(t *testing.T, chats []string) {
			if len(chats) != 1 || !strings.HasPrefix(chats[0], "alice| ") || !strings.Contains(chats[0], "⣀") || !strings.HasSuffix(chats[0], "|true") {
				t.Fatalf("ape output=%v", chats)
			}
		}},
		{"ct", "!ct extra", func(t *testing.T, chats []string) {
			if len(chats) != 1 || (chats[0] != "alice|head|false" && chats[0] != "alice|tail|false") {
				t.Fatalf("coin output=%v", chats)
			}
		}},
	}
	for _, tc := range cases {
		e := &commandEngineStub{}
		d, ok := commandDefinitionFor(tc.alias)
		if !ok {
			t.Fatalf("missing definition for %s", tc.alias)
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: tc.text, IsWhisper: true}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("%s status=%v err=%v", tc.alias, status, err)
		}
		tc.check(t, e.chats)
	}
}

func TestUserUtilityClusterIsSilentOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, alias := range []string{"ping", "version", "ape", "coin"} {
		e := &commandEngineStub{}
		d, _ := commandDefinitionFor(alias)
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias}).Execute(ctx)
		if status != model.FAILED || err == nil || len(e.chats) != 0 {
			t.Fatalf("%s status=%v err=%v chats=%v", alias, status, err, e.chats)
		}
	}
}
