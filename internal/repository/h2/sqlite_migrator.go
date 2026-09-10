package h2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

const migrationBatchSize = 250

type legacyTable struct {
	name, ddl string
	columns   []string
	rows      int64
}

var (
	legacyIdentity = regexp.MustCompile(`(?i)\bINTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`)
	legacyAutoInc  = regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`)
	legacyInteger  = regexp.MustCompile(`(?i)\bINTEGER\b`)
	legacyText     = regexp.MustCompile(`(?i)\bTEXT\b`)
	legacyNoCase   = regexp.MustCompile(`(?i)\s+COLLATE\s+NOCASE`)
)

// migrateSQLiteIfNeeded imports <stem>.db into the already-open H2 target.
// SQLite is used only as a migration reader and never as a runtime backend.
func migrateSQLiteIfNeeded(ctx context.Context, target *sql.DB, legacyPath string) (bool, error) {
	info, err := os.Stat(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect legacy SQLite source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("legacy SQLite source is not a regular file: %s", legacyPath)
	}

	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(legacyPath)+"?mode=ro")
	if err != nil {
		return false, fmt.Errorf("open legacy SQLite source: %w", err)
	}
	if err := source.PingContext(ctx); err != nil {
		_ = source.Close()
		return false, fmt.Errorf("read legacy SQLite source: %w", err)
	}
	tables, err := readLegacyTables(ctx, source)
	if err != nil {
		_ = source.Close()
		return false, err
	}

	targetEmpty, err := migrationTargetEmpty(ctx, target, tables)
	if err != nil {
		_ = source.Close()
		return false, err
	}
	if !targetEmpty {
		_ = source.Close()
		return false, fmt.Errorf("legacy SQLite and H2 databases contain conflicting data; refusing to merge %s", legacyPath)
	}

	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		_ = source.Close()
		return false, fmt.Errorf("begin SQLite migration: %w", err)
	}
	rollback := func(cause error) (bool, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			cause = errors.Join(cause, rollbackErr)
		}
		_ = source.Close()
		return false, cause
	}
	if _, err := tx.ExecContext(ctx, "SET REFERENTIAL_INTEGRITY FALSE"); err != nil {
		return rollback(fmt.Errorf("disable H2 referential integrity: %w", err))
	}
	for _, table := range tables {
		exists, err := tableExists(ctx, tx, table.name)
		if err != nil {
			return rollback(err)
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, translateLegacyDDL(table.ddl)); err != nil {
				return rollback(fmt.Errorf("create migrated table %q: %w", table.name, err))
			}
		}
		if err := copyLegacyTable(ctx, source, tx, table); err != nil {
			return rollback(err)
		}
		if err := resetMigratedIdentity(ctx, tx, table); err != nil {
			return rollback(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "SET REFERENTIAL_INTEGRITY TRUE"); err != nil {
		return rollback(fmt.Errorf("enable H2 referential integrity: %w", err))
	}
	if err := createLegacyIndexes(ctx, source, tx); err != nil {
		return rollback(err)
	}
	if err := verifyMigratedCounts(ctx, tx, tables); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		_ = source.Close()
		return false, fmt.Errorf("commit SQLite migration: %w", err)
	}
	if err := source.Close(); err != nil {
		return false, fmt.Errorf("close migrated SQLite source: %w", err)
	}
	if err := archiveLegacySQLite(legacyPath); err != nil {
		return false, err
	}
	return true, nil
}

func readLegacyTables(ctx context.Context, source *sql.DB) ([]legacyTable, error) {
	rows, err := source.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("read legacy SQLite tables: %w", err)
	}
	defer rows.Close()
	var tables []legacyTable
	for rows.Next() {
		var table legacyTable
		if err := rows.Scan(&table.name, &table.ddl); err != nil {
			return nil, err
		}
		columns, err := legacyColumns(ctx, source, table.name)
		if err != nil {
			return nil, err
		}
		table.columns = columns
		if err := source.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table.name)).Scan(&table.rows); err != nil {
			return nil, fmt.Errorf("count legacy table %q: %w", table.name, err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func legacyColumns(ctx context.Context, source *sql.DB, table string) ([]string, error) {
	rows, err := source.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return nil, fmt.Errorf("read legacy columns for %q: %w", table, err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var index, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func migrationTargetEmpty(ctx context.Context, target *sql.DB, tables []legacyTable) (bool, error) {
	for _, table := range tables {
		exists, err := tableExists(ctx, target, table.name)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		var targetRows int64
		if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table.name)).Scan(&targetRows); err != nil {
			return false, fmt.Errorf("count H2 table %q: %w", table.name, err)
		}
		if targetRows != 0 {
			return false, nil
		}
	}
	return true, nil
}

func copyLegacyTable(ctx context.Context, source *sql.DB, target *sql.Tx, table legacyTable) error {
	if table.rows == 0 || len(table.columns) == 0 {
		return nil
	}
	quotedColumns := make([]string, len(table.columns))
	for index, column := range table.columns {
		quotedColumns[index] = quoteIdentifier(column)
	}
	rows, err := source.QueryContext(ctx, "SELECT "+strings.Join(quotedColumns, ",")+" FROM "+quoteIdentifier(table.name))
	if err != nil {
		return fmt.Errorf("read legacy table %q: %w", table.name, err)
	}
	defer rows.Close()
	batch := make([][]any, 0, migrationBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		arguments := make([]any, 0, len(batch)*len(table.columns))
		values := make([]string, len(batch))
		parameter := 1
		for rowIndex, row := range batch {
			placeholders := make([]string, len(row))
			for columnIndex, value := range row {
				placeholders[columnIndex] = fmt.Sprintf("$%d", parameter)
				parameter++
				arguments = append(arguments, value)
			}
			values[rowIndex] = "(" + strings.Join(placeholders, ",") + ")"
		}
		statement := "INSERT INTO " + quoteIdentifier(table.name) + " (" + strings.Join(quotedColumns, ",") + ") VALUES " + strings.Join(values, ",")
		if _, err := target.ExecContext(ctx, statement, arguments...); err != nil {
			return fmt.Errorf("copy legacy table %q: %w", table.name, err)
		}
		batch = batch[:0]
		return nil
	}
	for rows.Next() {
		values := make([]any, len(table.columns))
		destinations := make([]any, len(values))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return fmt.Errorf("scan legacy table %q: %w", table.name, err)
		}
		batch = append(batch, values)
		if len(batch) == migrationBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

func resetMigratedIdentity(ctx context.Context, target *sql.Tx, table legacyTable) error {
	hasID := false
	for _, column := range table.columns {
		hasID = hasID || strings.EqualFold(column, "id")
	}
	if !hasID || table.rows == 0 {
		return nil
	}
	var next int64
	if err := target.QueryRowContext(ctx, "SELECT COALESCE(MAX(\"id\"),0)+1 FROM "+quoteIdentifier(table.name)).Scan(&next); err != nil {
		return fmt.Errorf("read migrated identity for %q: %w", table.name, err)
	}
	if _, err := target.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"id\" RESTART WITH %d", quoteIdentifier(table.name), next)); err != nil {
		return fmt.Errorf("reset migrated identity for %q: %w", table.name, err)
	}
	return nil
}

func createLegacyIndexes(ctx context.Context, source *sql.DB, target *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return fmt.Errorf("read legacy SQLite indexes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			return err
		}
		var count int
		if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.indexes WHERE LOWER(index_name)=LOWER($1)`, name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if _, err := target.ExecContext(ctx, translateLegacyDDL(ddl)); err != nil {
			return fmt.Errorf("create migrated index %q: %w", name, err)
		}
	}
	return rows.Err()
}

func verifyMigratedCounts(ctx context.Context, target *sql.Tx, tables []legacyTable) error {
	for _, table := range tables {
		var count int64
		if err := target.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table.name)).Scan(&count); err != nil {
			return err
		}
		if count != table.rows {
			return fmt.Errorf("row-count verification failed for %q: expected=%d actual=%d", table.name, table.rows, count)
		}
	}
	return nil
}

func translateLegacyDDL(statement string) string {
	statement = legacyIdentity.ReplaceAllString(statement, "BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY")
	statement = legacyAutoInc.ReplaceAllString(statement, "")
	statement = legacyInteger.ReplaceAllString(statement, "BIGINT")
	statement = legacyText.ReplaceAllString(statement, "VARCHAR")
	return legacyNoCase.ReplaceAllString(statement, "")
}

func archiveLegacySQLite(source string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		current := source + suffix
		if _, err := os.Stat(current); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		archive := current + ".bak"
		if _, err := os.Stat(archive); err == nil {
			return fmt.Errorf("legacy SQLite archive already exists: %s", archive)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(current, archive); err != nil {
			return fmt.Errorf("archive legacy SQLite file %s: %w", current, err)
		}
	}
	return nil
}
