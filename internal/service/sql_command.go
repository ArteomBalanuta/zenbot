package service

import (
	"context"
	"database/sql"
	"fmt"
)

// RawSQLQuery is the narrow raw-query dependency used by the human SQL command.
// It intentionally exposes no statement policy or database handle.
type RawSQLQuery interface {
	Query(context.Context, string) (SQLTable, error)
}

// SQLTable is the unformatted result of a raw SQL query.
type SQLTable struct {
	Columns []string
	Rows    [][]string
}

// RawSQLService executes raw queries through the application's existing H2 DB.
// Command parsing, rendering, and replies are intentionally deferred to tracer 8.
type RawSQLService struct {
	DB *sql.DB
}

func (s *RawSQLService) Query(ctx context.Context, rawSQL string) (_ SQLTable, err error) {
	rows, err := s.DB.QueryContext(ctx, rawSQL)
	if err != nil {
		return SQLTable{}, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	columns, err := rows.Columns()
	if err != nil {
		return SQLTable{}, err
	}
	table := SQLTable{Columns: append([]string(nil), columns...)}
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for i := range values {
		destinations[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return SQLTable{}, err
		}
		row := make([]string, len(values))
		for i, value := range values {
			switch value := value.(type) {
			case nil:
				row[i] = "null"
			case []byte:
				row[i] = string(value)
			default:
				row[i] = fmt.Sprint(value)
			}
		}
		table.Rows = append(table.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return SQLTable{}, err
	}
	return table, nil
}
