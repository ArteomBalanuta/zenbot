package h2

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"zenbot/internal/repository"
)

// ExecuteAgentSQL executes already validated read-only SQL and enforces every
// server-owned output bound before returning model-visible data.
func (d *Database) ExecuteAgentSQL(ctx context.Context, query string, maxRows, maxColumns, maxCellChars, maxResultChars int) (json.RawMessage, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("agent SQL database unavailable")
	}
	if maxRows < 1 || maxColumns < 1 || maxCellChars < 1 || maxResultChars < 1 {
		return nil, fmt.Errorf("invalid agent SQL bounds")
	}
	started := time.Now()
	rows, err := d.DB.QueryContext(ctx, query)
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
