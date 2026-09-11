package sqlite

import (
	"context"
	"testing"
)

func TestAgentSQLReadOnlyGuardAndConnectionReuse(t *testing.T) {
	d := openTestDB(t)
	d.DB.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := d.DB.Exec(`INSERT INTO messages(name,message,created_on) VALUES('alice','original',1)`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE messages SET message='changed' RETURNING id`,
		`DELETE FROM messages RETURNING id`,
		`INSERT INTO messages(name,created_on) VALUES('injected',1) RETURNING id`,
	} {
		if _, err := d.ExecuteAgentSQL(ctx, query, 10, 10, 100, 1000); err == nil {
			t.Fatalf("write accepted: %s", query)
		}
	}
	if _, err := d.ExecuteAgentSQL(ctx, `SELECT message FROM messages`, 10, 10, 100, 1000); err != nil {
		t.Fatal(err)
	}
	var count int
	var message string
	if err := d.DB.QueryRow(`SELECT COUNT(*), MIN(message) FROM messages`).Scan(&count, &message); err != nil || count != 1 || message != "original" {
		t.Fatalf("count=%d message=%q err=%v", count, message, err)
	}
	if _, err := d.DB.Exec(`UPDATE messages SET message='ordinary write'`); err != nil {
		t.Fatalf("read-only state leaked into pool: %v", err)
	}
}
