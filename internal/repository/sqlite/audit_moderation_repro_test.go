package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"zenbot/internal/repository"
)

func TestAuditModerationDeleteAliasPreservesOtherRegisteredIdentity(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "shared-trip")
	if err := d.RegisterNameByTrip(context.Background(), "bob", "shared-trip"); err != nil {
		t.Fatal(err)
	}
	result, err := d.DeleteIdentityAuthorized(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	var links int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trip_names tn JOIN names n ON n.id=tn.name_id JOIN trips t ON t.id=tn.trip_id WHERE n.name='bob' AND t.trip='shared-trip'").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("bob registration links=%d deletion=%+v", links, result)
	}
	if result != (repository.DeleteResult{TripNamesRows: 1, NameRows: 1}) {
		t.Fatalf("deletion=%+v", result)
	}
	assertCounts(t, d, 1, 1, 1)
}

func TestAuditModerationDeleteTripPreservesOtherRegisteredIdentity(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "shared-name", "trip-a")
	if err := d.RegisterTripByName(context.Background(), "shared-name", "trip-b"); err != nil {
		t.Fatal(err)
	}
	result, err := d.DeleteIdentityAuthorized(context.Background(), "trip-a")
	if err != nil {
		t.Fatal(err)
	}
	assertIdentityLink(t, d, "shared-name", "trip-b")
	if result != (repository.DeleteResult{TripNamesRows: 1, TripRows: 1}) {
		t.Fatalf("deletion=%+v", result)
	}
	assertCounts(t, d, 1, 1, 1)
}

func TestAuditModerationDeleteTripUsesExactIdentity(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "owner", "Trip-A")
	if result, err := d.DeleteIdentityAuthorized(context.Background(), "trip-a"); err == nil || result != (repository.DeleteResult{}) {
		t.Fatalf("case-variant deletion=%+v err=%v", result, err)
	}
	assertIdentityLink(t, d, "owner", "Trip-A")
	result, err := d.DeleteIdentityAuthorized(context.Background(), "Trip-A")
	if err != nil || result != (repository.DeleteResult{TripNamesRows: 1, TripRows: 1, NameRows: 1}) {
		t.Fatalf("exact deletion=%+v err=%v", result, err)
	}
	assertCounts(t, d, 0, 0, 0)
}

func TestAuditModerationDeleteRejectsAmbiguousTripAndNameCollision(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "Trip-A")
	seedIdentity(t, d, "Trip-A", "trip-b")
	if err := d.RegisterNameByTrip(context.Background(), "bob", "Trip-A"); err != nil {
		t.Fatal(err)
	}
	if result, err := d.DeleteIdentityAuthorized(context.Background(), "Trip-A"); err == nil || result != (repository.DeleteResult{}) {
		t.Fatalf("ambiguous deletion=%+v err=%v", result, err)
	}
	assertIdentityLink(t, d, "alice", "Trip-A")
	assertIdentityLink(t, d, "bob", "Trip-A")
	assertIdentityLink(t, d, "Trip-A", "trip-b")
	assertCounts(t, d, 3, 2, 3)
}

func TestAuditModerationDeleteCanceledContextPreservesIdentity(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "trip-a")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := d.DeleteIdentityAuthorized(ctx, "alice")
	if !errors.Is(err, context.Canceled) || result != (repository.DeleteResult{}) {
		t.Fatalf("canceled deletion=%+v err=%v", result, err)
	}
	assertIdentityLink(t, d, "alice", "trip-a")
	assertCounts(t, d, 1, 1, 1)
}

func TestAuditModerationDeleteCancellationRollsBackRelationship(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "trip-a")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec := func(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
		result, err := tx.ExecContext(ctx, query, args...)
		cancel()
		return result, err
	}
	result, err := d.deleteIdentity(withSaturnAuthorization(ctx), "alice", "trip-a", exec)
	if err == nil || result != (repository.DeleteResult{}) {
		t.Fatalf("canceled deletion=%+v err=%v", result, err)
	}
	assertIdentityLink(t, d, "alice", "trip-a")
	assertCounts(t, d, 1, 1, 1)
}

func TestAuditModerationDeleteParentConstraintFailureRollsBackEverything(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "trip-a")
	for _, query := range []string{
		`CREATE TABLE protected_identity(name_id BIGINT REFERENCES names(id))`,
		`INSERT INTO protected_identity(name_id) SELECT id FROM names WHERE name='alice'`,
	} {
		if _, err := d.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	result, err := d.DeleteIdentityAuthorized(context.Background(), "alice")
	if err == nil || result != (repository.DeleteResult{}) {
		t.Fatalf("failed deletion=%+v err=%v", result, err)
	}
	assertIdentityLink(t, d, "alice", "trip-a")
	assertCounts(t, d, 1, 1, 1)
}

func assertIdentityLink(t *testing.T, d *Database, name, trip string) {
	t.Helper()
	var links int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trip_names tn JOIN names n ON n.id=tn.name_id JOIN trips t ON t.id=tn.trip_id WHERE n.name=?1 AND t.trip=?2`, name, trip).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("unrelated registration removed: %s/%s links=%d", name, trip, links)
	}
}
