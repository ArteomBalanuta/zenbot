package h2

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"zenbot/internal/model"
)

func openTestDB(t *testing.T) *Database {
	t.Helper()
	jar := "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"
	dir := t.TempDir()
	d, err := Open(context.Background(), Config{BaseDir: dir, DatabaseStem: filepath.Join(dir, "db"), H2Jar: jar, Port: 55436, StartupTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
func TestAuditRecordsAndVisibility(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.MessageAudit(ctx, model.MessageRecord{Trip: "trip", Name: "alice", Hash: "h", Message: "public", CreatedOnMillis: 1, Visibility: "PUBLIC"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.MessageAudit(ctx, model.MessageRecord{Trip: "trip", Name: "alice", Message: "secret", CreatedOnMillis: 2, Visibility: "WHISPER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PresenceAudit(ctx, model.PresenceRecord{Trip: "trip", Name: "alice", EventType: "join", CreatedOnMillis: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CommandAudit(ctx, model.CommandAuditRecord{Trip: "trip", CommandName: "list", Status: "SUCCESSFUL", CreatedOnMillis: 4}); err != nil {
		t.Fatal(err)
	}
	var public, whisper, presence, commands int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE visibility='PUBLIC'").Scan(&public); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE visibility='WHISPER'").Scan(&whisper); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM user_presence_log").Scan(&presence); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM executed_commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if public != 1 || whisper != 1 || presence != 1 || commands != 1 {
		t.Fatalf("counts public=%d whisper=%d presence=%d commands=%d", public, whisper, presence, commands)
	}
}

func TestInsertReturningUsesExplicitTableForQuotedColumns(t *testing.T) {
	d := openTestDB(t)

	id, err := insertReturning(
		context.Background(),
		d.DB,
		"messages",
		`INSERT INTO messages("trip","name","message","created_on","visibility") VALUES($1,$2,$3,$4,$5)`,
		"quoted-trip", "quoted-name", "quoted-message", int64(10), "PUBLIC",
	)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("expected positive inserted id, got %d", id)
	}
}

func TestWithTxRollsBackAllWrites(t *testing.T) {
	d := openTestDB(t)
	err := d.WithTx(context.Background(), func(tx *sql.Tx) error {
		if _, e := tx.Exec("INSERT INTO messages(name,created_on,visibility) VALUES($1,$2,$3)", "rollback", 1, "PUBLIC"); e != nil {
			return e
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	var count int
	if err = d.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE name='rollback'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d rows", count)
	}
}

func TestAuthorizationTripsPersistAndResolveRoles(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.GrantTrip(ctx, "trusted-trip", model.TRUSTED); err != nil {
		t.Fatal(err)
	}
	role, err := d.ResolveRole(ctx, "trusted-trip")
	if err != nil {
		t.Fatal(err)
	}
	if role != model.TRUSTED {
		t.Fatalf("resolved role = %v, want TRUSTED", role)
	}
	if err := d.GrantTrip(ctx, "trusted-trip", model.ADMIN); err != nil {
		t.Fatal(err)
	}
	role, err = d.ResolveRole(ctx, "trusted-trip")
	if err != nil {
		t.Fatal(err)
	}
	if role != model.ADMIN {
		t.Fatalf("updated role = %v, want ADMIN", role)
	}

	allowed, err := d.IsTripAuthorized(ctx, "trusted-trip", model.MODERATOR, nil)
	if err != nil || !allowed {
		t.Fatalf("db authorization = %v, %v; want true, nil", allowed, err)
	}
	allowed, err = d.IsTripAuthorized(ctx, "unknown-trip", model.ADMIN, []string{"x"})
	if err != nil || !allowed {
		t.Fatalf("wildcard authorization = %v, %v; want true, nil", allowed, err)
	}
	allowed, err = d.IsTripAuthorized(ctx, "unknown-trip", model.USER, nil)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("unknown trip unexpectedly authorized")
	}
}
