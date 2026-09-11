package sqlite

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"zenbot/internal/repository"
)

// ExecuteAgentSQL executes already validated read-only SQL and enforces every
// server-owned output bound before returning model-visible data.
func (d *Database) ExecuteAgentSQL(ctx context.Context, query string, maxRows, maxColumns, maxCellChars, maxResultChars int) (_ json.RawMessage, resultErr error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("agent SQL database unavailable")
	}
	if maxRows < 1 || maxColumns < 1 || maxCellChars < 1 || maxResultChars < 1 {
		return nil, fmt.Errorf("invalid agent SQL bounds")
	}
	started := time.Now()
	conn, err := d.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		return nil, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, "PRAGMA query_only=OFF"); err != nil {
			// Never return a connection with altered session state to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			resultErr = errors.Join(resultErr, fmt.Errorf("reset agent SQL read-only connection: %w", err))
		}
	}()
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(columns) > maxColumns {
		return nil, fmt.Errorf("agent SQL result has too many columns")
	}
	out := make([][]any, 0)
	truncated := false
	for rows.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		for i, v := range values {
			if s, ok := v.(string); ok && len([]rune(s)) > maxCellChars {
				values[i] = string([]rune(s)[:maxCellChars])
				truncated = true
			}
		}
		out = append(out, values)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]any{"columns": columns, "rows": out, "truncated": truncated, "elapsedMillis": time.Since(started).Milliseconds()})
	if err != nil {
		return nil, err
	}
	if len(data) > maxResultChars {
		return nil, fmt.Errorf("agent SQL result exceeds output bound")
	}
	return data, nil
}

var _ repository.AgentSQLRepository = (*Database)(nil)
