package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"zenbot/internal/model"
)

// Keep the actual SQLite writes/queries, injecting a second writer immediately
// after the primary INSERT. This deterministically exposes SELECT MAX(id)
// attribution without relying on a scheduler race or mocked database rows.
type interleavedAuditWriter struct {
	db     *sql.DB
	insert func()
}

func (w interleavedAuditWriter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := w.db.ExecContext(ctx, query, args...)
	if err == nil && strings.Contains(strings.ToUpper(query), "INSERT") {
		w.insert()
	}
	return result, err
}

func (w interleavedAuditWriter) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	row := w.db.QueryRowContext(ctx, query, args...)
	if strings.Contains(strings.ToUpper(query), "INSERT") {
		w.insert()
	}
	return row
}

func TestAuditIdentityReturnedIDBelongsToItsOwnInsert(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	for _, tc := range []struct{ table, column string }{
		{"messages", "name"},
		{"user_presence_log", "name"},
		{"executed_commands", "command_name"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			// Table/column names are code-owned fixture literals, never inputs.
			query := "INSERT INTO " + tc.table + "(" + tc.column + ",created_on) VALUES(?1,?2)"
			otherInserts := 0
			writer := interleavedAuditWriter{db: database.DB, insert: func() {
				if _, err := database.DB.ExecContext(ctx, query, "second-writer", 2); err != nil {
					t.Fatal(err)
				}
				otherInserts++
			}}
			id, err := insertReturning(ctx, writer, tc.table, query, "first-writer", 1)
			if err != nil {
				t.Fatal(err)
			}
			if otherInserts != 1 {
				t.Fatalf("interleaved real insert count=%d, want 1", otherInserts)
			}
			var owner string
			if err := database.DB.QueryRowContext(ctx, "SELECT "+tc.column+" FROM "+tc.table+" WHERE id=?1", id).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			if owner != "first-writer" {
				t.Fatalf("returned audit id %d belongs to %q instead of this invocation", id, owner)
			}
		})
	}
}

func TestAuditIdentityReturnedIDMatchesConcurrentPublicAPICaller(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		ownerQuery string
		write      func(string, int64) (int64, error)
	}{
		{
			name:       "messages",
			ownerQuery: "SELECT name FROM messages WHERE id=?1",
			write: func(owner string, createdOn int64) (int64, error) {
				return database.MessageAudit(ctx, model.MessageRecord{Name: owner, CreatedOnMillis: createdOn})
			},
		},
		{
			name:       "user_presence_log",
			ownerQuery: "SELECT name FROM user_presence_log WHERE id=?1",
			write: func(owner string, createdOn int64) (int64, error) {
				return database.PresenceAudit(ctx, model.PresenceRecord{Name: owner, CreatedOnMillis: createdOn})
			},
		},
		{
			name:       "executed_commands",
			ownerQuery: "SELECT command_name FROM executed_commands WHERE id=?1",
			write: func(owner string, createdOn int64) (int64, error) {
				return database.CommandAudit(ctx, model.CommandAuditRecord{CommandName: owner, CreatedOnMillis: createdOn})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const writers = 32
			type result struct {
				owner string
				id    int64
				err   error
			}
			start := make(chan struct{})
			results := make(chan result, writers)
			var ready sync.WaitGroup
			ready.Add(writers)
			for i := 0; i < writers; i++ {
				go func(i int) {
					owner := fmt.Sprintf("writer-%02d", i)
					ready.Done()
					<-start
					id, err := tc.write(owner, int64(i+1))
					results <- result{owner: owner, id: id, err: err}
				}(i)
			}
			ready.Wait()
			close(start)
			for i := 0; i < writers; i++ {
				got := <-results
				if got.err != nil {
					t.Fatal(got.err)
				}
				var owner string
				if err := database.DB.QueryRowContext(ctx, tc.ownerQuery, got.id).Scan(&owner); err != nil {
					t.Fatal(err)
				}
				if owner != got.owner {
					t.Fatalf("returned audit id %d belongs to %q, caller supplied %q", got.id, owner, got.owner)
				}
			}
		})
	}
}

type countingAuditWriter struct {
	db         *sql.DB
	statements int
}

func (w *countingAuditWriter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	w.statements++
	return w.db.ExecContext(ctx, query, args...)
}

func (w *countingAuditWriter) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	w.statements++
	return w.db.QueryRowContext(ctx, query, args...)
}

func TestAuditIdentityFailedOrCanceledInsertReturnsNoIDWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  func() context.Context
		args []any
	}{
		{
			name: "storage rejection",
			ctx:  context.Background,
			args: []any{nil, int64(1), "PUBLIC"},
		},
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			args: []any{"canceled-writer", int64(2), "PUBLIC"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := openTestDB(t)
			writer := &countingAuditWriter{db: database.DB}
			id, err := insertReturning(
				tc.ctx(), writer, messagesTable,
				`INSERT INTO messages("name","created_on","visibility") VALUES(?1,?2,?3) RETURNING id`,
				tc.args...,
			)
			if err == nil {
				t.Fatal("expected insert error")
			}
			if id != 0 {
				t.Fatalf("failed insert returned id %d, want 0", id)
			}
			if writer.statements != 1 {
				t.Fatalf("database statements=%d, want exactly one insert attempt", writer.statements)
			}
			var count int
			if err := database.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("failed insert created %d rows, want 0", count)
			}
		})
	}
}
