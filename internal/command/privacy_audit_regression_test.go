package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	messagehandler "zenbot/internal/listener/message"
	"zenbot/internal/model"
)

type privateNotesSendErrorEngine struct {
	*commandEngineStub
	err error
}

func (e *privateNotesSendErrorEngine) SendChatMessage(string, string, bool) (string, error) {
	return "", e.err
}

func TestUtilityAuditNotesArePrivate(t *testing.T) {
	e := openNotesParityEngine(t)
	if err := e.bundle.Notes.Save(context.Background(), "trip", "private secret"); err != nil {
		t.Fatal(err)
	}
	d, _ := commandDefinitionFor("notes")
	for _, whisper := range []bool{false, true} {
		e.chats = nil
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "trip", Text: "!notes", IsWhisper: whisper}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(e.chats) != 1 || !strings.Contains(e.chats[0], "private secret") || !strings.HasSuffix(e.chats[0], "|true") {
			t.Errorf("notes delivery: %q", e.chats)
		}
	}
}

func TestPrivateNotesDeliveryFailureReturnsError(t *testing.T) {
	base := openNotesParityEngine(t)
	if err := base.bundle.Notes.Save(context.Background(), "trip", "private secret"); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("whisper failed")
	e := &privateNotesSendErrorEngine{commandEngineStub: base, err: wantErr}
	d, _ := commandDefinitionFor("notes")
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "trip", Text: "!notes"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, wantErr) {
		t.Fatalf("status=%v err=%v", status, err)
	}
}

func TestPrivateMailCommandQueuesWhispersFromPublicRequests(t *testing.T) {
	e, db := openMailGroupCParityEngine(t)
	seedMailGroupCCommandRecipient(t, db)
	for _, alias := range []string{"mail", "msg", "send"} {
		d, _ := commandDefinitionFor(alias)
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "origin", Text: "!" + alias + " merc private secret"}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("status=%v err=%v", status, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mail WHERE is_whisper='true'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("private mail count=%d want 3", count)
	}
	for _, chat := range e.chats {
		if strings.Contains(chat, "private secret") {
			t.Errorf("mail acknowledgment exposed content: %q", chat)
		}
	}
	e.chats = nil
	for _, trip := range []string{"", "TRIP-A", "attacker"} {
		_, err := (messagehandler.DeliverPendingMail{}).Handle(context.Background(), &messagehandler.Context{Engine: e, Message: &model.ChatMessage{Name: "trip-a", Trip: trip}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(e.chats) != 0 {
		t.Fatalf("mail sent to attacker: %q", e.chats)
	}
	_, err := (messagehandler.DeliverPendingMail{}).Handle(context.Background(), &messagehandler.Context{Engine: e, Message: &model.ChatMessage{Name: "recipient", Trip: "trip-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.chats) != 3 {
		t.Fatalf("private delivery: %q", e.chats)
	}
	for _, chat := range e.chats {
		if !strings.HasPrefix(chat, "recipient|") || !strings.Contains(chat, "private secret") || !strings.HasSuffix(chat, "|true") {
			t.Fatalf("private delivery exposed recipient or payload: %q", chat)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM mail WHERE status='DELIVERED'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("delivered mail count=%d want 3", count)
	}
}
