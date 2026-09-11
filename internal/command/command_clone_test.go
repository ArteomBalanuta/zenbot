package command

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/model"
)

func TestCommandCloneRetainsCanonicalBehaviorAndNewEngine(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		old, next := &commandEngineStub{}, &commandEngineStub{}
		d, _ := commandDefinitionFor("memory")
		clone := d.New(old, &model.ChatMessage{Name: "old", Text: "!memory"}).NewInstance(next, &model.ChatMessage{Name: "new", Text: "!mem"})
		status, err := clone.Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL || len(old.chats) != 0 || len(next.chats) != 1 || !strings.HasPrefix(next.chats[0], "new|") || !strings.Contains(next.chats[0], "MB") {
			t.Fatalf("status=%s err=%v old=%v next=%v", status, err, old.chats, next.chats)
		}
	})
	t.Run("access", func(t *testing.T) {
		oldAuth, nextAuth := &authFake{}, &authFake{}
		old, next := newIdentityEngine(&identityFake{}, oldAuth), newIdentityEngine(&identityFake{}, nextAuth)
		d, _ := commandDefinitionFor("access")
		clone := d.New(old, &model.ChatMessage{Name: "old", Trip: "old", Text: "!access old USER"}).NewInstance(next, &model.ChatMessage{Name: "new", Trip: "admin", Text: "!grant new-trip ADMIN"})
		status, err := clone.Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL || len(oldAuth.granted) != 0 || len(nextAuth.granted) != 1 || nextAuth.granted[0] != "new-trip:Admin" {
			t.Fatalf("status=%s err=%v old=%v next=%v", status, err, oldAuth.granted, nextAuth.granted)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		oldControl, nextControl := &recordingLifecycleController{}, &recordingLifecycleController{}
		old, next := &lifecycleCommandEngineStub{commandEngineStub: &commandEngineStub{}, controller: oldControl}, &lifecycleCommandEngineStub{commandEngineStub: &commandEngineStub{}, controller: nextControl}
		d, _ := commandDefinitionFor("shutdown")
		clone := d.New(old, &model.ChatMessage{Text: "!shutdown"}).NewInstance(next, &model.ChatMessage{Text: "!quit"})
		status, err := clone.Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL || oldControl.shutdownCalls != 0 || nextControl.shutdownCalls != 1 {
			t.Fatalf("status=%s err=%v old=%d next=%d", status, err, oldControl.shutdownCalls, nextControl.shutdownCalls)
		}
	})
	t.Run("direct agent", func(t *testing.T) {
		old, next := &commandEngineStub{}, &commandEngineStub{}
		submitter := &recordingDirectAgentSubmitter{}
		d, _ := directLDefinition(submitter)
		message := &model.ChatMessage{Name: "new", Text: "!L line one\n\tline  two: café \\n"}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		clone := d.New(old, &model.ChatMessage{Name: "old", Text: "!l old"}).NewInstance(next, message)
		status, err := clone.Execute(ctx)
		if err != nil || status != model.SUCCESSFUL || submitter.calls != 1 || submitter.ctx != ctx || submitter.message != message || submitter.prompt != "line one\n\tline  two: café \\n" || len(old.chats) != 0 || len(next.chats) != 0 {
			t.Fatalf("status=%s err=%v submission=%+v", status, err, submitter)
		}
	})
	t.Run("subscriptions", func(t *testing.T) {
		old, next := &commandEngineStub{}, &commandEngineStub{}
		for _, canonical := range []string{"sub", "unsub"} {
			d, _ := commandDefinitionFor(canonical)
			clone := d.New(old, &model.ChatMessage{Name: "old", Trip: "old", Text: "!" + canonical}).NewInstance(next, &model.ChatMessage{Name: "new", Trip: "new", Text: "!" + canonical})
			status, err := clone.Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL || old.IsSubscribedTrip("old") || next.IsSubscribedTrip("new") != (canonical == "sub") {
				t.Fatalf("canonical=%s status=%s err=%v old=%v next=%v", canonical, status, err, old.subs, next.subs)
			}
		}
	})
}

func TestCommandCloneUnknownCanonicalFailsExplicitly(t *testing.T) {
	engine := &commandEngineStub{}
	clone := newCommand("unknown-canonical", []string{"memory"}, model.REGULAR, engine, &model.ChatMessage{Text: "!memory"}).NewInstance(engine, &model.ChatMessage{Text: "!memory"})
	status, err := clone.Execute(context.Background())
	if status != model.FAILED || err == nil || len(engine.chats) != 0 {
		t.Fatalf("status=%s err=%v chats=%v", status, err, engine.chats)
	}
}
