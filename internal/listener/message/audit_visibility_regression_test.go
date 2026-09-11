package message

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type auditVisibilityEngine struct {
	common.Engine
	legacyCalls int
	ctx         context.Context
	records     []model.MessageRecord
	failure     error
}

func (*auditVisibilityEngine) GetChannel() string { return "programming" }
func (e *auditVisibilityEngine) LogMessage(string, string, string, string, string) (int64, error) {
	e.legacyCalls++
	return 1, nil
}
func (e *auditVisibilityEngine) LogMessageRecord(ctx context.Context, record model.MessageRecord) (int64, error) {
	e.ctx = ctx
	e.records = append(e.records, record)
	return 1, e.failure
}

func TestSharedMessageAuditCancellationBetweenHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	later := false
	chain := NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { cancel(); return true, nil }), HandlerFunc(func(context.Context, *Context) (bool, error) { later = true; return true, nil }))
	if err := chain.Process(ctx, &model.ChatMessage{}, nil); err != context.Canceled || later {
		t.Fatalf("err=%v later=%t", err, later)
	}
}

type legacyVisibilityEngine struct {
	common.Engine
	bodies []string
}

func (*legacyVisibilityEngine) GetChannel() string { return "trusted" }
func (e *legacyVisibilityEngine) LogMessage(_, _, _, body, _ string) (int64, error) {
	e.bodies = append(e.bodies, body)
	return 1, nil
}

func TestSharedMessageAuditPrivateBodyNeverUsesPublicLogger(t *testing.T) {
	for _, m := range []*model.ChatMessage{{IsWhisper: true}, {Whisper: true}, {Type: "whisper"}} {
		e := &legacyVisibilityEngine{}
		m.Text = "private token abc"
		_, err := (AuditChatMessage{}).Handle(context.Background(), &Context{Engine: e, Message: m})
		if err == nil || strings.Contains(err.Error(), m.Text) || len(e.bodies) != 0 {
			t.Fatalf("private audit leaked: err=%v bodies=%v", err, e.bodies)
		}
	}
	e := &legacyVisibilityEngine{}
	_, err := (AuditChatMessage{}).Handle(context.Background(), &Context{Engine: e, Message: &model.ChatMessage{Text: "public"}})
	if err != nil || len(e.bodies) != 1 || e.bodies[0] != "public" {
		t.Fatalf("public fallback: err=%v bodies=%v", err, e.bodies)
	}
}

func TestSharedMessageAuditStoreErrorStopsChain(t *testing.T) {
	e := &auditVisibilityEngine{failure: errors.New("typed store unavailable")}
	later := false
	chain := NewChain(AuditChatMessage{}, HandlerFunc(func(context.Context, *Context) (bool, error) { later = true; return true, nil }))
	err := chain.Process(context.Background(), &model.ChatMessage{IsWhisper: true, Text: "private"}, e)
	if !errors.Is(err, e.failure) || later || e.legacyCalls != 0 {
		t.Fatalf("err=%v later=%t legacy=%d", err, later, e.legacyCalls)
	}
}

func TestSharedMessageAuditPrecancelStopsLegacyDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executed := false
	cmd := &dispatchTestCommand{executed: &executed}
	e := &dispatchTestEngine{allowed: true, commands: map[string]common.CommandMetadata{"auth": {Alias: "auth", Command: func(*model.ChatMessage) common.Command { return cmd }}}}
	_, err := (DispatchUserCommand{}).Handle(ctx, &Context{Engine: e, Message: &model.ChatMessage{Name: "alice", Text: "!auth"}, Author: &model.User{Name: "alice"}})
	if err != context.Canceled || executed || e.chats != 0 {
		t.Fatalf("err=%v executed=%t sends=%d", err, executed, e.chats)
	}
}

func TestSharedMessageAuditUsesExplicitVisibilityAndCallerContext(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		engine := &auditVisibilityEngine{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		message := &model.ChatMessage{Name: "alice", Trip: "Trip", Text: "sensitive command", IsWhisper: whisper}
		_, err := (AuditChatMessage{}).Handle(ctx, &Context{Engine: engine, Message: message})
		want := "PUBLIC"
		if whisper {
			want = "WHISPER"
		}
		if err != nil || engine.legacyCalls != 0 || len(engine.records) != 1 {
			t.Errorf("typed audit bypassed: whisper=%t legacy=%d records=%v err=%v", whisper, engine.legacyCalls, engine.records, err)
			continue
		}
		if engine.ctx != ctx || engine.records[0].Visibility != want || engine.records[0].Channel != "programming" || engine.records[0].Message != message.Text {
			t.Errorf("audit boundary lost context/privacy: context=%v record=%+v", engine.ctx, engine.records[0])
		}
	}
}
