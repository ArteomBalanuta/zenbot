package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestIdentityAmbiguousNicknameRejectsWithoutOrphan(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.DB.Exec(`INSERT INTO names(name,created_on) VALUES('Alice',1),('alice',1)`); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterTripByName(context.Background(), "ALICE", "new-trip"); err == nil {
		t.Error("ambiguous nickname accepted")
	}
	var trips, links int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trips`).Scan(&trips); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trip_names`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if trips != 0 || links != 0 {
		t.Fatalf("orphan writes: trips=%d links=%d", trips, links)
	}
}

func TestIdentityNicknameLookupPreservesMissingAndUnambiguousCases(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.RegisterTripByName(ctx, "missing", "Trip"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing name err=%v", err)
	}
	if err := d.Register(ctx, "Alice", "Trip", model.USER); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterTripByName(ctx, "ALICE", "Second"); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := d.DB.QueryRow(`SELECT n.name FROM names n JOIN trip_names tn ON tn.name_id=n.id JOIN trips t ON t.id=tn.trip_id WHERE t.trip='Second'`).Scan(&name); err != nil || name != "Alice" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestIdentityContextCancelsQueriesAndTransactionsThroughService(t *testing.T) {
	d := openTestDB(t)
	s := &service.UserService{Identity: d}
	d.DB.SetMaxOpenConns(1)
	for _, tc := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"name query", func(ctx context.Context) error { _, err := s.IsNameRegistered(ctx, "Alice"); return err }},
		{"trip query", func(ctx context.Context) error { _, err := s.IsTripRegistered(ctx, "Trip"); return err }},
		{"history query", func(ctx context.Context) error { _, err := s.LastMessages(ctx, "Alice", "Trip", 2); return err }},
		{"register transaction", func(ctx context.Context) error { return s.Register(ctx, "Alice", "Trip", model.USER) }},
		{"name transaction", func(ctx context.Context) error { return s.RegisterNameByTrip(ctx, "Alias", "Trip") }},
		{"trip transaction", func(ctx context.Context) error { return s.RegisterTripByName(ctx, "Alice", "Trip2") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Hold the only connection so the real query/transaction must honor
			// cancellation while waiting, rather than merely checking at entry.
			held, err := d.DB.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tc.run(ctx) }()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("canceled operation err=%v", err)
				}
			case <-time.After(time.Second):
				t.Error("operation ignored cancellation")
				_ = held.Close()
				<-done
			}
		})
	}
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM names`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("writes after cancellation=%d err=%v", count, err)
	}
}

func TestGrantTripsValidatesAllTargetsAndCancelsWithoutWrites(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.GrantTrip(ctx, "first", model.USER); err != nil {
		t.Fatal(err)
	}
	for _, targets := range [][]string{nil, {}, {"first", ""}, {"first", "  "}, {"first", "second,third"}} {
		if err := d.GrantTrips(ctx, targets, model.ADMIN); err == nil {
			t.Errorf("accepted targets %q", targets)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := d.GrantTrips(canceled, []string{"first", "new"}, model.ADMIN); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
	role, err := d.ResolveRole(ctx, "first")
	if err != nil || role != model.USER {
		t.Fatalf("partial grant role=%v err=%v", role, err)
	}
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trips`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("trips=%d err=%v", count, err)
	}
}

func TestGrantTripsPreservesExactCredentialsAndBindsQuotedValues(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.GrantTrips(ctx, []string{"AbC123", "abc123", "AbC123", "'quoted"}, model.ADMIN); err != nil {
		t.Fatal(err)
	}
	if err := d.GrantTrips(ctx, []string{"abc123"}, model.USER); err != nil {
		t.Fatal(err)
	}
	for trip, want := range map[string]model.Role{"AbC123": model.ADMIN, "abc123": model.USER, "'quoted": model.ADMIN} {
		role, err := d.ResolveRole(ctx, trip)
		if err != nil || role != want {
			t.Errorf("%s role=%v want=%v err=%v", trip, role, want, err)
		}
	}
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trips`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("trips=%d err=%v", count, err)
	}
}

func TestIdentityCaseVariantCredentialsRemainIndependent(t *testing.T) {
	d := openTestDB(t)
	if err := d.Register(context.Background(), "Alice", "AbC123", model.ADMIN); err != nil {
		t.Fatal(err)
	}
	if err := d.Register(context.Background(), "Bob", "abc123", model.USER); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterNameByTrip(context.Background(), "LowerAlias", "abc123"); err != nil {
		t.Fatal(err)
	}
	var trip string
	if err := d.DB.QueryRow(`SELECT t.trip FROM trips t JOIN trip_names tn ON tn.trip_id=t.id JOIN names n ON n.id=tn.name_id WHERE n.name='LowerAlias'`).Scan(&trip); err != nil {
		t.Fatal(err)
	}
	if trip != "abc123" {
		t.Fatalf("alias attached to %q", trip)
	}
}
