package service_test

import (
	"context"
	"testing"
	"zenbot/internal/service"
)

func TestMailAndNotesRoundTripPlainText(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)
	ctx := context.Background()
	text := "quote \" actual\n literal \\n path C:\\new 🌦️\t\r"
	mail := &service.MailService{DB: db}
	if err := mail.Queue(ctx, text, "sender", "@merc", true); err != nil {
		t.Fatal(err)
	}
	pending, err := mail.Pending(ctx, "merc", "trip-a")
	if err != nil || len(pending) != 1 || pending[0].Message != text+" " {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	var stored string
	if err := db.QueryRow("SELECT message FROM mail").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != text+" " {
		t.Fatalf("new mail was pre-escaped: %q", stored)
	}
	notes := &service.NoteService{DB: db}
	if err := notes.Save(ctx, "trip-a", text); err != nil {
		t.Fatal(err)
	}
	got, err := notes.List(ctx, "trip-a")
	if err != nil || len(got) != 1 || got[0] != text {
		t.Fatalf("notes=%q err=%v", got, err)
	}
}

func TestLegacyMailJSONStringsDecodeOnceOnRead(t *testing.T) {
	db := openMailGroupCDB(t)
	// Both the previous Go writer and Saturn's escapeJson writer stored JSON
	// string contents without the enclosing quotes. The schema default identifies
	// these legacy writers explicitly; never infer encoding from the text.
	_, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip-a',?,'PENDING',1,'true')`, `actual\n literal \\n quote \" slash \\`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&service.MailService{DB: db}).Pending(context.Background(), "merc", "trip-a")
	if want := "actual\n literal \\n quote \" slash \\"; err != nil || len(got) != 1 || got[0].Message != want {
		t.Fatalf("got=%+v err=%v want=%q", got, err, want)
	}
}

func TestMailRawReceiverPreservesExactTripBeforeNicknameLookup(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)
	if _, err := db.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('USER','@merc',1)`); err != nil {
		t.Fatal(err)
	}
	got, err := (&service.MailService{DB: db}).QueueResolved(context.Background(), "text", "sender", "@merc", true)
	if err != nil || got != "@merc" {
		t.Fatalf("receiver=%q err=%v", got, err)
	}
}

func TestMailRejectsInvalidTaggedEncodingWithoutGuessing(t *testing.T) {
	for _, tc := range []struct{ encoding, text string }{{"JSON_STRING", "bad\\q"}, {"UNKNOWN", "valid text"}} {
		t.Run(tc.encoding, func(t *testing.T) {
			db := openMailGroupCDB(t)
			if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper,text_encoding) VALUES('sender','trip',?,'PENDING',1,'true',?)`, tc.text, tc.encoding); err != nil {
				t.Fatal(err)
			}
			got, err := (&service.MailService{DB: db}).Pending(context.Background(), "merc", "trip")
			if err == nil || len(got) != 0 {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
