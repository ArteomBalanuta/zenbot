package service_test

import (
	"database/sql"
	"testing"

	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func openMailGroupCDB(t *testing.T) *sql.DB {
	t.Helper()
	db := h2fixture.Open(t, "mail-group-c")
	return db.DB
}

func seedMailRecipient(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-a',1),('USER','trip-b',2)",
		"INSERT INTO names(name,created_on) VALUES('Merc',1)",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-a' AND n.name='Merc'",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-b' AND n.name='Merc'",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMailGroupCQueueSerializesResolvedTripsAndEscapedPayload(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)

	if err := (&service.MailService{DB: db}).Queue("quote \"x\"\nline", "alice#origin", " @mErC ", true); err != nil {
		t.Fatal(err)
	}

	var owner, receiver, message, status, whisper string
	var createdOn int64
	if err := db.QueryRow("SELECT owner,receiver,message,status,is_whisper,created_on FROM mail").Scan(&owner, &receiver, &message, &status, &whisper, &createdOn); err != nil {
		t.Fatal(err)
	}
	if owner != "alice#origin" || receiver != "trip-a,trip-b" || message != `quote \"x\"\nline ` || status != "PENDING" || whisper != "true" || createdOn <= 0 {
		t.Fatalf("mail owner=%q receiver=%q message=%q status=%q whisper=%q createdOn=%d", owner, receiver, message, status, whisper, createdOn)
	}
}

func TestMailGroupCQueueIgnoresFailedWriteLikeSaturn(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)
	if _, err := db.Exec("DROP TABLE mail"); err != nil {
		t.Fatal(err)
	}

	if err := (&service.MailService{DB: db}).Queue("still acknowledged", "alice#origin", "merc", true); err != nil {
		t.Fatalf("Queue returned write error, want Saturn-compatible acknowledgement: %v", err)
	}
}

func TestMailGroupCQueuedMailIsPendingUntilDelivered(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)
	mail := &service.MailService{DB: db}

	if err := mail.Queue("status update", "alice#origin", "@merc", true); err != nil {
		t.Fatal(err)
	}

	pending, err := mail.Pending("MERC", "trip-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending mail count = %d, want 1", len(pending))
	}
	got := pending[0]
	if got.Owner != "alice#origin" || got.Receiver != "trip-a,trip-b" || got.Message != "status update " || got.Status != "PENDING" || !got.IsWhisper || got.CreatedOn <= 0 {
		t.Fatalf("pending mail = %+v", got)
	}

	if err := mail.MarkDelivered(got.ID); err != nil {
		t.Fatal(err)
	}

	pending, err = mail.Pending("merc", "trip-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending mail count after delivery = %d, want 0", len(pending))
	}

	var status string
	if err := db.QueryRow("SELECT status FROM mail WHERE id = ?", got.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "DELIVERED" {
		t.Fatalf("mail status = %q, want DELIVERED", status)
	}
}
