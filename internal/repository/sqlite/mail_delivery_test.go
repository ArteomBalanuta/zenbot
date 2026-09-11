package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/service"
)

type mailDeliverer interface {
	DeliverPending(context.Context, string, string, func(context.Context, model.Mail) error) error
}

func deliveryService(t *testing.T, db *sql.DB) mailDeliverer {
	t.Helper()
	s, ok := any(&service.MailService{DB: db}).(mailDeliverer)
	if !ok {
		t.Fatal("mail service has no durable recipient delivery operation")
	}
	return s
}

func seedDeliveryMail(t *testing.T, db *sql.DB, recipients, status string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender',?1,'body',?2,1,'true')`, recipients, status); err != nil {
		t.Fatal(err)
	}
}

func assertAttemptState(t *testing.T, db *sql.DB, trip, want string) {
	t.Helper()
	var state, attempt string
	if err := db.QueryRow(`SELECT state,attempt_id FROM mail_delivery WHERE recipient_trip=?1`, trip).Scan(&state, &attempt); err != nil {
		t.Fatal(err)
	}
	if state != want || attempt == "" {
		t.Fatalf("recipient state=%q attempt=%q, want %s and durable identity", state, attempt, want)
	}
}

func TestMailDeliveryConcurrentInstancesClaimOnce(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	a, b := deliveryService(t, d.DB), deliveryService(t, d.DB)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var sends atomic.Int32
	start, results := make(chan struct{}), make(chan error, 2)
	for _, s := range []mailDeliverer{a, b} {
		go func(s mailDeliverer) {
			<-start
			results <- s.DeliverPending(ctx, "alice", "trip-a", func(context.Context, model.Mail) error { sends.Add(1); return nil })
		}(s)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if sends.Load() != 1 {
		t.Fatalf("transport sends=%d, want 1", sends.Load())
	}
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
}

func TestMailDeliveryFailedFinalizationRetainsUnknown(t *testing.T) {
	d := openTestDB(t)
	s := deliveryService(t, d.DB)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	if _, err := d.DB.Exec(`CREATE TRIGGER reject_accept BEFORE UPDATE ON mail_delivery WHEN NEW.state='ACCEPTED' BEGIN SELECT RAISE(ABORT, 'reject acceptance'); END`); err != nil {
		t.Fatal(err)
	}
	sends := 0
	err := s.DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	if err == nil {
		t.Fatal("successful transport followed by failed persistence was hidden")
	}
	var uncertain *service.MailDeliveryUncertainError
	if !errors.Is(err, service.ErrMailDeliveryUnknown) || !errors.As(err, &uncertain) || uncertain.MailID == 0 || uncertain.AttemptID == "" || errors.Unwrap(err) == nil {
		t.Fatalf("missing typed attempt uncertainty and underlying error: %v", err)
	}
	assertAttemptState(t, d.DB, "trip-a", "UNKNOWN")
	if err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("transport sends=%d, want 1", sends)
	}
}

func TestMailDeliveryInterruptedAttemptIsNotReclaimed(t *testing.T) {
	d := openTestDB(t)
	s := deliveryService(t, d.DB)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	if _, err := d.DB.Exec(`INSERT INTO mail_delivery(mail_id,recipient_trip,attempt_id,state,updated_on) SELECT id,'trip-a','interrupted-attempt','ATTEMPTING',1 FROM mail`); err != nil {
		t.Fatal(err)
	}
	sends := 0
	if err := s.DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatalf("interrupted attempt replayed: sends=%d", sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "ATTEMPTING")
}

func TestMailDeliveryCancellationAfterSendFinalizesAndStops(t *testing.T) {
	d := openTestDB(t)
	s := deliveryService(t, d.DB)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sends := 0
	err := s.DeliverPending(ctx, "alice", "trip-a", func(context.Context, model.Mail) error { sends++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || sends != 1 {
		t.Fatalf("err=%v sends=%d, want canceled and one", err, sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
	var pending int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM mail WHERE status='PENDING'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending mail=%d, want unattempted second mail", pending)
	}
}

func TestMailDeliveryBootstrapPreservesLegacyRecipientState(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a,trip-b", "PENDING")
	seedDeliveryMail(t, d.DB, "trip-a", "DELIVERED")
	// Simulate a pre-delivery-table database, then run normal bootstrap twice.
	if _, err := d.DB.Exec(`DROP TABLE IF EXISTS mail_delivery`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := bootstrap(context.Background(), d.DB); err != nil {
			t.Fatal(err)
		}
	}
	s := deliveryService(t, d.DB)
	sends := 0
	for _, trip := range []string{"trip-a", "trip-b", "trip-a", "trip-b"} {
		if err := s.DeliverPending(context.Background(), "shared", trip, func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if sends != 2 {
		t.Fatalf("legacy transport sends=%d, want 2", sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
	assertAttemptState(t, d.DB, "trip-b", "ACCEPTED")
	var delivered int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM mail WHERE status='DELIVERED'`).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if delivered != 2 {
		t.Fatalf("delivered source rows=%d, want 2", delivered)
	}
}

func TestMailDeliveryClaimFailureSendsNothing(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	if _, err := d.DB.Exec(`CREATE TRIGGER reject_claim BEFORE INSERT ON mail_delivery WHEN NEW.state='ATTEMPTING' BEGIN SELECT RAISE(ABORT, 'reject claim'); END`); err != nil {
		t.Fatal(err)
	}
	sends := 0
	err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	if err == nil || sends != 0 {
		t.Fatalf("err=%v sends=%d, want failed claim and no send", err, sends)
	}
	var attempts int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM mail_delivery`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("failed claim persisted %d attempts", attempts)
	}
}

func TestMailDeliveryStaleFinalizationCannotOverwriteAnotherAttempt(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	sends := 0
	err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error {
		sends++
		// Model an investigation changing the durable ownership while the old
		// transport is in progress. Its late receipt must not overwrite this row.
		_, err := d.DB.Exec(`UPDATE mail_delivery SET attempt_id='new-owner',state='UNKNOWN'`)
		return err
	})
	if !errors.Is(err, service.ErrMailDeliveryUnknown) || sends != 1 {
		t.Fatalf("err=%v sends=%d", err, sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "UNKNOWN")
	var attempt string
	if err := d.DB.QueryRow(`SELECT attempt_id FROM mail_delivery`).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if attempt != "new-owner" {
		t.Fatalf("stale receipt overwrote attempt %q", attempt)
	}
}

func TestMailDeliveryCanceledBeforeSelectionLeavesMailPending(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sends := 0
	err := deliveryService(t, d.DB).DeliverPending(ctx, "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	if !errors.Is(err, context.Canceled) || sends != 0 {
		t.Fatalf("err=%v sends=%d", err, sends)
	}
	var attempts, pending int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM mail_delivery`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM mail WHERE status='PENDING'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || pending != 1 {
		t.Fatalf("attempts=%d pending=%d", attempts, pending)
	}
}

func TestMailNotesCanceledCallsPreserveStoredRows(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	notes := &service.NoteService{DB: d.DB}
	if err := notes.Save(context.Background(), "trip-a", "keep"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := notes.Save(ctx, "trip-a", "new"); !errors.Is(err, context.Canceled) {
		t.Fatalf("save err=%v", err)
	}
	if err := notes.Clear(ctx, "trip-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("clear err=%v", err)
	}
	if _, err := notes.List(ctx, "trip-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("list err=%v", err)
	}
	if err := (&service.MailService{DB: d.DB}).Queue(ctx, "new", "sender", "trip-a", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("queue err=%v", err)
	}
	got, err := notes.List(context.Background(), "trip-a")
	if err != nil || len(got) != 1 || got[0] != "keep" {
		t.Fatalf("notes=%v err=%v", got, err)
	}
}

func TestMailDeliverySenderPreflightCancellationRemainsPending(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := deliveryService(t, d.DB).DeliverPending(ctx, "alice", "trip-a", func(context.Context, model.Mail) error {
		cancel()
		return errors.Join(service.ErrMailSendNotStarted, ctx.Err())
	})
	if !errors.Is(err, context.Canceled) || errors.Is(err, service.ErrMailDeliveryUnknown) {
		t.Fatalf("definite preflight cancellation misclassified: %v", err)
	}
	assertAttemptState(t, d.DB, "trip-a", "PENDING")
	sends := 0
	if err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("unsent pending mail did not deliver once: sends=%d", sends)
	}
}
