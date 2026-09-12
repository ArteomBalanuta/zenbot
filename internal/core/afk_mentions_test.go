package core

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	messagehandler "zenbot/internal/listener/message"
	"zenbot/internal/model"
)

func afkWhispers(t *testing.T, output *stateOutputTransport) []string {
	t.Helper()
	output.mu.Lock()
	defer output.mu.Unlock()
	var whispers []string
	for _, payload := range output.payloads {
		var message struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(message.Text, "/whisper @") {
			whispers = append(whispers, message.Text)
		}
	}
	return whispers
}

func TestAfkLatestMentionDeliveredPrivatelyOnce(t *testing.T) {
	output := &stateOutputTransport{}
	e := &EngineImpl{Transport: output}
	u := &model.User{Name: "alice", Trip: "Trip"}
	e.AddAfkUser(u, "lunch")
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "@alice first"})
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "carol", Text: "@alice latest\nsecond line"})
	if got := afkWhispers(t, output); len(got) != 0 {
		t.Fatalf("delivered before return: %v", got)
	}
	_, err := (messagehandler.UpdateAfkState{}).Handle(context.Background(), &messagehandler.Context{Engine: e, Author: u, Message: &model.ChatMessage{Name: "alice", Trip: "Trip", Text: "back"}})
	if err != nil {
		t.Fatal(err)
	}
	e.RemoveIfAfk(u)
	got := afkWhispers(t, output)
	if len(got) != 1 || !strings.HasPrefix(got[0], "/whisper @alice ") || !strings.Contains(got[0], "carol") || !strings.Contains(got[0], "@alice latest\nsecond line") || strings.Contains(got[0], "first") {
		t.Fatalf("latest mention not delivered exactly once: %v", got)
	}
}

func TestAfkMentionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"@ann, hello", true}, {"ANN!", true}, {"(AbC123)", true},
		{"joanna", false}, {"@ann_other", false}, {"abc123", false},
		{"xAbC123", false}, {"AbC123/", false}, {"", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			output := &stateOutputTransport{}
			e := &EngineImpl{Transport: output}
			e.AddAfkUser(&model.User{Name: "ann", Trip: "AbC123"}, "away")
			e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: tc.text})
			if (output.count() == 1) != tc.want {
				t.Fatalf("notifications=%d want match=%v", output.count(), tc.want)
			}
		})
	}
}

func TestAfkMentionIdentityRenameAndRoomIsolation(t *testing.T) {
	output := &stateOutputTransport{}
	e := &EngineImpl{Transport: output}
	e.AddAfkUser(&model.User{Name: "alice", Trip: "Shared"}, "away")
	e.AddAfkUser(&model.User{Name: "alias", Trip: "Shared"}, "away")
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "@alice private reminder"})
	e.RemoveIfAfk(&model.User{Name: "alice", Trip: "Wrong"})
	e.RemoveIfAfk(&model.User{Name: "alias", Trip: "Shared"})
	other := &EngineImpl{Transport: output}
	other.AddAfkUser(&model.User{Name: "alice", Trip: "Shared"}, "away")
	other.RemoveIfAfk(&model.User{Name: "alice", Trip: "Shared"})
	if got := afkWhispers(t, output); len(got) != 0 {
		t.Fatalf("wrong identity/room received mention: %v", got)
	}
	e.RenameAfkUser("alice", "renamed")
	e.RemoveIfAfk(&model.User{Name: "renamed", Trip: "Shared"})
	got := afkWhispers(t, output)
	if len(got) != 1 || !strings.HasPrefix(got[0], "/whisper @renamed ") {
		t.Fatalf("rename lost mention: %v", got)
	}
}

func TestAfkNewSessionAndWhispersDoNotLeakMentions(t *testing.T) {
	output := &stateOutputTransport{}
	e := &EngineImpl{Transport: output}
	u := &model.User{Name: "alice", Trip: "Trip"}
	e.AddAfkUser(u, "away")
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "@alice old session"})
	e.AddAfkUser(u, "new session")
	for _, message := range []*model.ChatMessage{
		{Name: "bob", Text: "@alice secret", IsWhisper: true},
		{Name: "bob", Text: "@alice secret", Whisper: true},
		{Name: "bob", Text: "@alice secret", Type: "whisper"},
	} {
		e.NotifyAfkIfMentioned(message)
	}
	e.RemoveIfAfk(u)
	if got := afkWhispers(t, output); len(got) != 0 {
		t.Fatalf("stale/private mention leaked: %v", got)
	}
}

func TestAfkConcurrentReturnsDeliverMentionOnce(t *testing.T) {
	output := &stateOutputTransport{}
	e := &EngineImpl{Transport: output}
	u := &model.User{Name: "alice", Trip: "Trip"}
	e.AddAfkUser(u, "away")
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "@alice hello"})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { e.RemoveIfAfk(u) })
	}
	wg.Wait()
	if got := afkWhispers(t, output); len(got) != 1 {
		t.Fatalf("concurrent deliveries: %v", got)
	}
}

func TestAfkReminderReleasesLockBeforeSending(t *testing.T) {
	e := &EngineImpl{Transport: &stateOutputTransport{}}
	u := &model.User{Name: "alice", Trip: "Trip"}
	e.AddAfkUser(u, "away")
	e.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "@alice hello"})
	output := newBlockingStateOutputTransport()
	t.Cleanup(output.unblock)
	e.Transport = output
	done := make(chan struct{})
	go func() { e.RemoveIfAfk(u); close(done) }()
	waitForSignal(t, output.started, "private reminder send")
	added := make(chan struct{})
	go func() { e.AddAfkUser(u, "next session"); close(added) }()
	waitForSignal(t, added, "AFK add during blocked reminder")
	output.unblock()
	waitForSignal(t, done, "return notification")
	e.RemoveIfAfk(u)
	if got := afkWhispers(t, output); len(got) != 1 {
		t.Fatalf("stale reminder in new session: %v", got)
	}
}
