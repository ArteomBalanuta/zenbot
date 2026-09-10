package h2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"zenbot/internal/model"
)

const (
	messagesTable         = "messages"
	presenceTable         = "user_presence_log"
	executedCommandsTable = "executed_commands"
)

func (d *Database) SQLDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.DB
}

func (d *Database) Close() error {
	if d == nil {
		return nil
	}
	var first error
	if d.DB != nil {
		first = d.DB.Close()
	}
	if d.Server != nil {
		if err := d.Server.Stop(context.Background()); first == nil {
			first = err
		}
	}
	return first
}

func (d *Database) MessageAudit(ctx context.Context, r model.MessageRecord) (int64, error) {
	if r.Visibility == "" {
		r.Visibility = "PUBLIC"
	}
	if r.Visibility != "PUBLIC" && r.Visibility != "WHISPER" {
		return 0, fmt.Errorf("invalid message visibility %q", r.Visibility)
	}
	return insertReturning(ctx, d.DB, messagesTable, `INSERT INTO messages("trip","name","hash","message","created_on","channel","visibility") VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, r.Trip, r.Name, r.Hash, r.Message, r.CreatedOnMillis, r.Channel, r.Visibility)
}
func (d *Database) PresenceAudit(ctx context.Context, r model.PresenceRecord) (int64, error) {
	return insertReturning(ctx, d.DB, presenceTable, `INSERT INTO user_presence_log(trip,name,hash,event_type,created_on,channel) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, r.Trip, r.Name, r.Hash, r.EventType, r.CreatedOnMillis, r.Channel)
}
func (d *Database) CommandAudit(ctx context.Context, r model.CommandAuditRecord) (int64, error) {
	return insertReturning(ctx, d.DB, executedCommandsTable, `INSERT INTO executed_commands(trip,command_name,arguments,status,created_on,channel) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, r.Trip, r.CommandName, r.Arguments, r.Status, r.CreatedOnMillis, r.Channel)
}
func insertReturning(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table, query string, args ...any) (int64, error) {
	if table != messagesTable && table != presenceTable && table != executedCommandsTable {
		return 0, fmt.Errorf("invalid audit table %q", table)
	}
	query = strings.TrimSpace(strings.TrimSuffix(query, "RETURNING id"))
	if _, err := q.ExecContext(ctx, query, args...); err != nil {
		return 0, err
	}
	var id int64
	if err := q.QueryRowContext(ctx, "SELECT MAX(id) FROM "+table).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
func (d *Database) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	return d.MessageAudit(context.Background(), model.MessageRecord{Trip: trip, Name: name, Hash: hash, Message: message, Channel: channel, CreatedOnMillis: time.Now().UnixMilli(), Visibility: "PUBLIC"})
}
func (d *Database) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	return d.PresenceAudit(context.Background(), model.PresenceRecord{Trip: trip, Name: name, Hash: hash, EventType: eventType, Channel: channel, CreatedOnMillis: time.Now().UnixMilli()})
}
func (d *Database) LogCommand(ctx context.Context, r model.CommandAuditRecord) (int64, error) {
	return d.CommandAudit(ctx, r)
}

// WithTx executes fn atomically. Any error, panic, or context cancellation rolls back.
func (d *Database) WithTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
