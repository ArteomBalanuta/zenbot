package message

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type mailDeliveryAuditEngine struct {
	common.Engine
	mail           *service.MailService
	whispers       int
	publicAttempts int
	whisperError   error
	bodies         []string
}

func (e *mailDeliveryAuditEngine) ServiceBundle() *service.Bundle {
	return &service.Bundle{Mail: e.mail}
}

func (e *mailDeliveryAuditEngine) SendChatMessage(_, body string, whisper bool) (string, error) {
	e.bodies = append(e.bodies, body)
	if whisper {
		e.whispers++
		return body, e.whisperError
	}
	e.publicAttempts++
	if e.publicAttempts == 1 {
		return "", errors.New("public send failed")
	}
	return body, nil
}

func TestMailDeliveryCompactFormatPreservesBodyAndPrivacy(t *testing.T) {
	database := h2fixture.Open(t, "mail-compact-format")
	body := "first\nsecond \\n <>&"
	if _, err := database.DB.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper,text_encoding) VALUES('sender','trip-a',$1,'PENDING',1,'true','PLAIN')`, body); err != nil {
		t.Fatal(err)
	}
	engine := &mailDeliveryAuditEngine{mail: &service.MailService{DB: database.DB}}
	_, err := (DeliverPendingMail{}).Handle(context.Background(), &Context{Engine: engine, Message: &model.ChatMessage{Name: "alice", Trip: "trip-a"}})
	want := "Mail from sender · 1 Jan 1970 00:00 UTC\n" + body
	if err != nil || len(engine.bodies) != 1 || engine.bodies[0] != want || engine.whispers != 1 || engine.publicAttempts != 0 {
		t.Fatalf("bodies=%q whispers=%d public=%d err=%v", engine.bodies, engine.whispers, engine.publicAttempts, err)
	}
}

// A failure in a later public batch must not replay a previously delivered
// private batch. The database and listener are real; only transport is scripted.
func TestMailDeliveryAuditDoesNotReplaySuccessfulPrivateBatch(t *testing.T) {
	database := h2fixture.Open(t, "mail-batch-replay-audit")
	db := database.SQLDB()
	for _, whisper := range []string{"true", "false"} {
		if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip-a','body','PENDING',1,$1)`, whisper); err != nil {
			t.Fatal(err)
		}
	}
	engine := &mailDeliveryAuditEngine{mail: &service.MailService{DB: db}}
	state := &Context{Engine: engine, Message: &model.ChatMessage{Name: "alice", Trip: "trip-a"}}
	if _, err := (DeliverPendingMail{}).Handle(context.Background(), state); err == nil {
		t.Fatal("scripted public delivery failure was hidden")
	}
	_, _ = (DeliverPendingMail{}).Handle(context.Background(), state)
	if engine.whispers != 1 {
		t.Fatalf("already delivered private mail replayed: successful whispers=%d", engine.whispers)
	}
}

func TestMailDeliveryAuditCancellationStopsDelivery(t *testing.T) {
	database := h2fixture.Open(t, "mail-cancel-audit")
	db := database.SQLDB()
	if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip-a','body','PENDING',1,'true')`); err != nil {
		t.Fatal(err)
	}
	engine := &mailDeliveryAuditEngine{mail: &service.MailService{DB: db}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (DeliverPendingMail{}).Handle(ctx, &Context{Engine: engine, Message: &model.ChatMessage{Name: "alice", Trip: "trip-a"}})
	if !errors.Is(err, context.Canceled) || engine.whispers != 0 || engine.publicAttempts != 0 {
		t.Fatalf("canceled listener delivered mail: err=%v whispers=%d public=%d", err, engine.whispers, engine.publicAttempts)
	}
}

func TestMailDeliveryAuditRecipientsHaveIndependentDeliveryState(t *testing.T) {
	database := h2fixture.Open(t, "mail-recipient-state-audit")
	db := database.SQLDB()
	if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip-a,trip-b','body','PENDING',1,'true')`); err != nil {
		t.Fatal(err)
	}
	engine := &mailDeliveryAuditEngine{mail: &service.MailService{DB: db}}
	for _, trip := range []string{"trip-a", "trip-b", "trip-a", "trip-b"} {
		_, err := (DeliverPendingMail{}).Handle(context.Background(), &Context{Engine: engine, Message: &model.ChatMessage{Name: "shared-name", Trip: trip}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if engine.whispers != 2 {
		t.Fatalf("independent recipients must each get one mail: sends=%d, want 2", engine.whispers)
	}
}

func TestMailDeliveryAuditAmbiguousSendIsNotAutomaticallyReplayed(t *testing.T) {
	database := h2fixture.Open(t, "mail-unknown-send-audit")
	db := database.SQLDB()
	if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip-a','body','PENDING',1,'true')`); err != nil {
		t.Fatal(err)
	}
	engine := &mailDeliveryAuditEngine{mail: &service.MailService{DB: db}, whisperError: errors.New("connection lost after write")}
	state := &Context{Engine: engine, Message: &model.ChatMessage{Name: "alice", Trip: "trip-a"}}
	if _, err := (DeliverPendingMail{}).Handle(context.Background(), state); err == nil {
		t.Fatal("ambiguous send was hidden")
	}
	// A fresh service instance must honor persisted attempt state. An in-memory
	// dedup map would still permit replay after a host replacement or restart.
	engine.mail = &service.MailService{DB: db}
	engine.whisperError = nil
	_, _ = (DeliverPendingMail{}).Handle(context.Background(), state)
	if engine.whispers != 1 {
		t.Fatalf("ambiguous persisted delivery was automatically replayed: sends=%d", engine.whispers)
	}
}
