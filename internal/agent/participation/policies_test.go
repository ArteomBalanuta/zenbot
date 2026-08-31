package participation

import (
	"testing"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/model"
)

func TestMentionParserBoundariesAndCleanup(t *testing.T) {
	p := MentionParser{}
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{{"@Bot: hello, world!", "hello, world!", true}, {"x@Botish hi", "", false}, {"@Bot", "", false}, {"hello @bot, please help?", "hello, please help?", true}} {
		got, ok := p.Parse(tc.in, "Bot")
		if ok != tc.ok || got != tc.want {
			t.Errorf("Parse(%q)=(%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
func TestQuietRegistryUsesIdentityAndExpiresAtBoundary(t *testing.T) {
	now := time.Unix(100, 0)
	q := NewQuietRegistryAt(time.Second, func() time.Time { return now })
	c, _ := api.NewContext("Room", "N", "Trip", "", false, []string{})
	q.Silence(c)
	if !q.IsQuiet(c) {
		t.Fatal("not quiet")
	}
	now = now.Add(time.Second)
	if q.IsQuiet(c) {
		t.Fatal("quiet entry did not expire")
	}
	if !q.IsPoliteQuietRequest("please be quiet", "bot") {
		t.Fatal("quiet request not recognized")
	}
}
func TestClassifierDeterministicPrecedence(t *testing.T) {
	c := Classifier{}
	for _, s := range []string{"hello", "How are you?", "run command", "{}", "123"} {
		_ = c.Classify(s)
	}
	if c.Classify("hello") != Talk || c.Classify("run command") != Unclassified || c.Finalize(Talk, ToolEvidence{Attempted: true}) != ToolCall {
		t.Fatal("classifier policy mismatch")
	}
}
func TestFactoryUsesTrustedCapabilitiesAndPreservesMessage(t *testing.T) {
	f := InvocationFactory{}
	m := model.ChatMessage{Name: "alice", Trip: "creator", Text: "@bot hi", Whisper: true}
	inv, e := f.Create(TrustedSnapshot{Room: "room", Users: []string{"alice"}, CreatorTrip: "creator"}, m, "hi", api.DIRECT, false)
	if e != nil {
		t.Fatal(e)
	}
	c := inv.Context()
	if !c.HasCapability(api.AdminCommands) || !c.HasCapability(api.PermanentBan) || c.MemoryKey() == "room" {
		t.Fatalf("trusted context mismatch: %v", c)
	}
	if inv.CurrentMessageText() == nil || *inv.CurrentMessageText() != m.Text {
		t.Fatal("inbound text not preserved")
	}
}
func TestPipelinePrecedenceAndUnsupportedFailure(t *testing.T) {
	q := NewQuietRegistry(time.Minute)
	f := &InvocationFactory{}
	p := Pipeline{Factory: f, Quiet: q}
	e := Event{Message: model.ChatMessage{Name: "u", Text: "@bot hi"}, Snapshot: TrustedSnapshot{Room: "r", Users: []string{}}, BotNick: "bot"}
	out := p.Handle(e)
	if out.Decision != Claimed || out.Err == nil {
		t.Fatalf("mention should claim and fail closed: %+v", out)
	}
}

func TestPipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors(t *testing.T) {
	p := Pipeline{}
	for _, name := range []string{"BoT", "helper-bot", "bot_2"} {
		out := p.Handle(Event{Message: model.ChatMessage{Name: name, Text: "@bot hi"}, Snapshot: TrustedSnapshot{Room: "r", Users: []string{}}, BotNick: "bot"})
		if out.Decision != Pass || out.Submitted || out.Err != nil {
			t.Fatalf("author %q should be ineligible: %+v", name, out)
		}
	}
}

func TestMentionParserSaturnParity(t *testing.T) {
	p := MentionParser{}
	for _, tc := range []struct {
		name, in, nick, want string
		ok                   bool
	}{
		{"requires literal at", "Bot, explain this?", "Bot", "", false},
		{"unicode preceding letter is not a boundary", "猫@korin explain this", "korin", "", false},
		{"unicode preceding number is not a boundary", "²@korin explain this", "korin", "", false},
		{"unicode following letter is not a boundary", "@korin猫 explain this", "korin", "", false},
		{"unicode following number is not a boundary", "@korin² explain this", "korin", "", false},
		{"case insensitive exact mention", "@KoRiN, can you explain this?", "korin", "can you explain this?", true},
		{"mention after prose", "what do you think, @KORIN?", "korin", "what do you think?", true},
		{"removes all mentions", "@korin hello @KORIN", "korin", "hello", true},
		{"trailing comma question cleanup", "@korin hello,?", "korin", "hello?", true},
		{"trailing comma exclamation cleanup", "@korin hello,!", "korin", "hello!", true},
		{"literal special characters in nick", "@bot+1: explain", "bot+1", "explain", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Parse(tc.in, tc.nick)
			if ok != tc.ok || got != tc.want {
				t.Errorf("Parse(%q, %q)=(%q,%v), want (%q,%v)", tc.in, tc.nick, got, ok, tc.want, tc.ok)
			}
		})
	}
}
