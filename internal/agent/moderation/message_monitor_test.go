package moderation

import (
	"sync"
	"testing"
	"time"

	"zenbot/internal/model"
)

func TestMessageMonitorNormalizesAndExcludesBeforeState(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMessageMonitor(MessageConfig{Enabled: true, BurstCount: 2, BurstWindow: time.Second, RepeatedCount: 2, RepeatedWindow: time.Second, SecondBreachWindow: time.Second, PostKickWindow: time.Second, ActionCooldown: time.Millisecond}, func() time.Time { return now }, func(x model.ChatMessage) bool { return x.Trip == "admin" })
	for _, msg := range []model.ChatMessage{{Name: "a", Hash: "h", Text: "x", Whisper: true}, {Name: "a", Hash: "h", Text: "x", Trip: "admin"}, {Name: "a", Hash: "h", Text: " \\n\t "}, {Text: "x"}} {
		if got := m.OnMessage(msg); len(got) != 0 {
			t.Fatalf("excluded message decisions=%#v", got)
		}
	}
	if len(m.messages) != 0 || len(m.offences) != 0 || len(m.actions) != 0 {
		t.Fatalf("excluded input retained monitor state: %#v", m)
	}
	if got := m.OnMessage(model.ChatMessage{Name: "a", Hash: "h", Text: " A\\n\tB "}); len(got) != 0 {
		t.Fatalf("first message decisions=%#v", got)
	}
	if got := m.OnMessage(model.ChatMessage{Name: "a", Hash: "h", Text: "a b"}); len(got) != 1 || got[0].Action != Mute {
		t.Fatalf("normalized repeated threshold decisions=%#v", got)
	}
}

func TestMessageMonitorThresholdWindowsEscalationCooldownAndPruning(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMessageMonitor(MessageConfig{Enabled: true, BurstCount: 2, BurstWindow: time.Second, RepeatedCount: 2, RepeatedWindow: time.Second, SecondBreachWindow: 2 * time.Second, PostKickWindow: 3 * time.Second, ActionCooldown: time.Millisecond}, func() time.Time { return now }, nil)
	msg := func(text string) model.ChatMessage {
		return model.ChatMessage{Name: "spammer", Trip: "t", Hash: "h", Text: text}
	}
	var got []Action
	for _, wave := range []string{"one", "two", "three", "four"} {
		m.OnMessage(msg(wave + "-1"))
		if ds := m.OnMessage(msg(wave + "-2")); len(ds) == 1 {
			got = append(got, ds[0].Action)
		}
		now = now.Add(time.Millisecond)
	}
	want := []Action{Warn, Mute, Kick, ShadowBan}
	if len(got) != len(want) {
		t.Fatalf("actions=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions=%v want=%v", got, want)
		}
	}
	m.OnMessage(msg("after-1"))
	if ds := m.OnMessage(msg("after-2")); len(ds) != 0 {
		t.Fatalf("shadow-banned user acted: %#v", ds)
	}

	now = now.Add(4 * time.Second)
	m.OnMessage(msg("fresh-1"))
	if ds := m.OnMessage(msg("fresh-2")); len(ds) != 1 || ds[0].Action != Warn {
		t.Fatalf("expired offence did not restart warning: %#v", ds)
	}
	other := model.ChatMessage{Name: "other", Hash: "other", Text: "old"}
	m.OnMessage(other)
	now = now.Add(2 * time.Second)
	m.OnMessage(msg("prune"))
	if _, ok := m.messages["hash:other"]; ok {
		t.Fatal("expired distinct identity was retained")
	}
}

func TestMessageMonitorUsesInclusiveWindowsAndIsRaceSafe(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMessageMonitor(MessageConfig{Enabled: true, BurstCount: 2, BurstWindow: time.Second, RepeatedCount: 9, RepeatedWindow: time.Second, SecondBreachWindow: time.Second, PostKickWindow: time.Second, ActionCooldown: time.Millisecond}, func() time.Time { return now }, nil)
	msg := model.ChatMessage{Name: "a", Hash: "h", Text: "x"}
	m.OnMessage(msg)
	now = now.Add(time.Second)
	if ds := m.OnMessage(msg); len(ds) != 1 || ds[0].Action != Warn {
		t.Fatalf("inclusive burst window decisions=%#v", ds)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.OnMessage(msg) }()
	}
	wg.Wait()
}
