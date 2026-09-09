package h2

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

// PersistShadowBan mirrors Saturn's persistent shadow-ban contract: trusted
// join identity is stored locally; no hack.chat Ban command is emitted.
func (d *Database) PersistShadowBan(ctx context.Context, user model.User, reason string) error {
	return d.PersistShadowBanRecord(ctx, repository.ShadowBanRecord{Trip: user.Trip, Name: user.Name, Hash: user.Hash, Reason: reason})
}

// PersistShadowBanRecord inserts one source-compatible shadow-ban record.
// Blank fields are stored as NULL to preserve Saturn's offline-target shape.
func (d *Database) PersistShadowBanRecord(ctx context.Context, record repository.ShadowBanRecord) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	hash := record.Hash
	if hash != "" {
		hash = base64.StdEncoding.EncodeToString([]byte(hash))
	}
	value := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	_, err := d.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES($1,$2,$3,$4,$5)`, value(record.Trip), value(record.Name), value(hash), value(record.Reason), time.Now().UnixMilli())
	return err
}

// ListShadowBans reads Saturn's banned_users rows and decodes the stored
// base64 hash at the repository boundary.
func (d *Database) ListShadowBans(ctx context.Context) ([]repository.ShadowBanRecord, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT trip,name,hash,reason FROM banned_users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []repository.ShadowBanRecord
	for rows.Next() {
		var trip, name, hash, reason *string
		if err := rows.Scan(&trip, &name, &hash, &reason); err != nil {
			return nil, err
		}
		record := repository.ShadowBanRecord{}
		if trip != nil {
			record.Trip = *trip
		}
		if name != nil {
			record.Name = *name
		}
		if reason != nil {
			record.Reason = *reason
		}
		if hash != nil {
			decoded, err := base64.StdEncoding.DecodeString(*hash)
			if err != nil {
				return nil, fmt.Errorf("decode stored shadow-ban hash: %w", err)
			}
			record.Hash = string(decoded)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// RemoveShadowBanBySourceTarget mirrors Saturn's local unshadowban persistence
// semantics. The supplied target is bound exactly as name and trip, while the
// hash predicate receives its standard UTF-8 base64 encoding.
func (d *Database) RemoveShadowBanBySourceTarget(ctx context.Context, target string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users WHERE name=$1 OR trip=$2 OR hash=$3`, target, target, base64.StdEncoding.EncodeToString([]byte(target)))
	return err
}

// RemoveAllShadowBans deletes every local shadow-ban identity record.
func (d *Database) RemoveAllShadowBans(ctx context.Context) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users`)
	return err
}
