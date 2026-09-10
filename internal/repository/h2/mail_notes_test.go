package h2

import (
	"context"
	"testing"
	"zenbot/internal/service"
)

func TestMailAndNotesPersistenceParity(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-a',1),('USER','trip-b',2)"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO names(name,created_on) VALUES('merc',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-a' AND n.name='merc'"); err != nil {
		t.Fatal(err)
	}
	m := &service.MailService{DB: d.DB}
	if err := m.Queue("hello", "alice#src", "@merc", true); err != nil {
		t.Fatal(err)
	}
	var receiver, message, status, whisper string
	if err := d.DB.QueryRow("SELECT receiver,message,status,is_whisper FROM mail").Scan(&receiver, &message, &status, &whisper); err != nil {
		t.Fatal(err)
	}
	if receiver != "trip-a" || message != "hello " || status != "PENDING" || whisper != "true" {
		t.Fatalf("mail=%q %q %q %q", receiver, message, status, whisper)
	}
	n := &service.NoteService{DB: d.DB}
	if err := n.Save("trip-a", `quote "x"`); err != nil {
		t.Fatal(err)
	}
	got, err := n.List("trip-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != `quote \"x\"` {
		t.Fatalf("notes=%v", got)
	}
	if err := n.Clear("trip-a"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE trip='trip-a'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal(count)
	}
}
