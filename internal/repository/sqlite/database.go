// Package sqlite provides Zenbot's embedded SQLite persistence.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Config selects the SQLite database file. Relative paths resolve from the working directory.
type Config struct{ Path string }
type Database struct{ DB *sql.DB }

const currentSchemaVersion = 5

// Open opens a local database and atomically installs or upgrades its schema.
func Open(ctx context.Context, c Config) (*Database, error) {
	if strings.TrimSpace(c.Path) == "" {
		return nil, errors.New("SQLite database path is required")
	}
	path, err := filepath.Abs(c.Path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	params := url.Values{}
	// These pragmas run for every newly opened connection, not just the first.
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "journal_mode(WAL)")
	// Writers acquire their lock before reading, avoiding deferred transaction
	// lock upgrades that cannot honor SQLite's busy timeout.
	params.Set("_txlock", "immediate")
	uri.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	if err = db.PingContext(ctx); err == nil {
		err = bootstrap(ctx, db)
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	return &Database{DB: db}, nil
}

// bootstrap owns a connection so foreign key enforcement can be suspended only
// for SQLite's transactional table-rebuild procedure. Integrity is verified
// before commit; all other connections continue enforcing foreign keys.
func bootstrap(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() {
		_, restoreErr := conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
		err = errors.Join(err, restoreErr)
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&version); err != nil {
		return err
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("SQLite schema version %d is newer than supported %d", version, currentSchemaVersion)
	}
	if err = ensureColumn(ctx, tx, "messages", "visibility", `TEXT DEFAULT 'PUBLIC' CHECK (visibility IN ('PUBLIC','WHISPER'))`); err != nil {
		return err
	}
	if err = ensureColumn(ctx, tx, "mail", "text_encoding", `TEXT NOT NULL DEFAULT 'JSON_STRING'`); err != nil {
		return err
	}
	for _, stmt := range strings.Split(schemaSQL, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("SQLite schema bootstrap: %w", err)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE messages SET visibility='PUBLIC' WHERE visibility IS NULL`); err != nil {
		return err
	}
	if err = upgradeTripRoleConstraint(ctx, tx); err != nil {
		return err
	}
	if err = upgradeSummaryIdentity(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	rowsErr := rows.Err()
	closeErr := rows.Close()
	if err = errors.Join(rowsErr, closeErr); err != nil {
		return err
	}
	if invalid {
		return errors.New("SQLite schema upgrade rejected: foreign key violations")
	}
	if version < currentSchemaVersion {
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_version(version) VALUES(?)`, currentSchemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureColumn(ctx context.Context, tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+quoteIdentifier(table)+`)`)
	if err != nil {
		return err
	}
	exists, found := false, false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var value any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &value, &pk); err != nil {
			rows.Close()
			return err
		}
		exists = true
		found = found || strings.EqualFold(name, column)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	if !exists || found {
		return nil
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE `+quoteIdentifier(table)+` ADD COLUMN `+quoteIdentifier(column)+` `+definition)
	return err
}

func upgradeTripRoleConstraint(ctx context.Context, tx *sql.Tx) error {
	var ddl string
	if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='trips'`).Scan(&ddl); err != nil {
		return err
	}
	if strings.Contains(strings.ToUpper(ddl), "'PEST'") {
		return nil
	}
	columns, err := tx.QueryContext(ctx, `PRAGMA table_xinfo(trips)`)
	if err != nil {
		return err
	}
	expected := map[string]bool{"id": true, "type": true, "trip": true, "created_on": true}
	for columns.Next() {
		var cid, notNull, pk, hidden int
		var name, kind string
		var defaultValue any
		if err := columns.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk, &hidden); err != nil {
			columns.Close()
			return err
		}
		if !expected[strings.ToLower(name)] {
			columns.Close()
			return fmt.Errorf("SQLite trips upgrade cannot preserve additional column %q", name)
		}
		delete(expected, strings.ToLower(name))
	}
	if err := errors.Join(columns.Err(), columns.Close()); err != nil {
		return err
	}
	if len(expected) != 0 {
		return errors.New("SQLite trips upgrade requires the standard trip columns")
	}
	// AUTOINCREMENT remembers deleted IDs too. Retain its high-water mark when
	// rebuilding, so an old identity can never be assigned to a new trip.
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM sqlite_sequence WHERE name='trips'`).Scan(&sequence); err != nil {
		return err
	}
	// Recreate user indexes and triggers after replacing the constrained table.
	rows, err := tx.QueryContext(ctx, `SELECT sql FROM sqlite_schema WHERE tbl_name='trips' AND type IN ('index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	var extra []string
	for rows.Next() {
		var statement string
		if err = rows.Scan(&statement); err != nil {
			rows.Close()
			return err
		}
		extra = append(extra, statement)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE trips_upgrade (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL CHECK(type IN ('ADMIN','MODERATOR','TRUSTED','USER','REGULAR','PEST')), trip TEXT UNIQUE, created_on INTEGER NOT NULL)`,
		`INSERT INTO trips_upgrade(id,type,trip,created_on) SELECT id,type,trip,created_on FROM trips`,
		`DROP TABLE trips`,
		`ALTER TABLE trips_upgrade RENAME TO trips`,
	}
	for _, statement := range append(statements, extra...) {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("SQLite trip role upgrade: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sqlite_sequence SET seq=MAX(seq,?) WHERE name='trips'`, sequence); err != nil {
		return err
	}
	return nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func upgradeSummaryIdentity(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_xinfo(agent_memory_summary)`)
	if err != nil {
		return err
	}
	expected := map[string]bool{"identity_key": true, "content": true, "covered_through_id": true, "fingerprint": true, "created_on": true, "expires_on": true}
	needsUpgrade := false
	standard := true
	for rows.Next() {
		var cid, notNull, pk, hidden int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk, &hidden); err != nil {
			rows.Close()
			return err
		}
		key := strings.ToLower(name)
		if !expected[key] || hidden != 0 {
			standard = false
		}
		delete(expected, key)
		if key == "identity_key" {
			needsUpgrade = notNull == 0
			if pk != 1 {
				standard = false
			}
		} else if pk != 0 {
			standard = false
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if !needsUpgrade {
		return nil
	}
	if !standard || len(expected) != 0 {
		return errors.New("SQLite summary upgrade cannot preserve nonstandard columns")
	}
	var nulls int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_memory_summary WHERE identity_key IS NULL`).Scan(&nulls); err != nil {
		return err
	}
	if nulls > 0 {
		return errors.New("SQLite summary upgrade rejected: NULL identity keys")
	}
	rows, err = tx.QueryContext(ctx, `SELECT sql FROM sqlite_schema WHERE tbl_name='agent_memory_summary' AND type IN ('index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	var extra []string
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			rows.Close()
			return err
		}
		extra = append(extra, statement)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE summary_upgrade(identity_key TEXT PRIMARY KEY NOT NULL,content TEXT NOT NULL,covered_through_id INTEGER NOT NULL,fingerprint TEXT NOT NULL,created_on INTEGER NOT NULL,expires_on INTEGER NOT NULL)`,
		`INSERT INTO summary_upgrade SELECT identity_key,content,covered_through_id,fingerprint,created_on,expires_on FROM agent_memory_summary`,
		`DROP TABLE agent_memory_summary`,
		`ALTER TABLE summary_upgrade RENAME TO agent_memory_summary`,
	}
	for _, statement := range append(statements, extra...) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("SQLite summary identity upgrade: %w", err)
		}
	}
	return nil
}

//go:embed schema.sql
var schemaSQL string
