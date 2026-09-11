package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx executes fn atomically. Callback errors and panics roll back; any
// error returned by Commit has an unknown outcome, including cancellation.
func WithTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
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
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%w: %w", ErrCommitOutcomeUnknown, err)
	}
	return nil
}
