package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/util"
)

func (d *Database) PersistShadowBanSelector(ctx context.Context, name, reason string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("shadow-ban target is required")
	}
	_, err := d.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(NULL,?1,NULL,?2,?3)`, name, reason, time.Now().UnixMilli())
	return err
}

// ListLegacyShadowBans retains the original rich-row reader for compatibility.
// New command code uses ListShadowBans and the typed ShadowBanRecord boundary.
func (d *Database) ListLegacyShadowBans(ctx context.Context) ([]model.BanRecord, error) {
	rows, err := d.DB.QueryContext(ctx, `SELECT id,trip,name,hash,reason,created_on FROM banned_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BanRecord
	for rows.Next() {
		var r model.BanRecord
		var trip, name, hash, reason sql.NullString
		if err := rows.Scan(&r.ID, &trip, &name, &hash, &reason, &r.CreatedOn); err != nil {
			return nil, err
		}
		r.Trip, r.Name, r.Hash, r.Reason = trip.String, name.String, hash.String, reason.String
		if r.Hash != "" {
			if raw, e := base64.StdEncoding.DecodeString(r.Hash); e == nil {
				r.Hash = string(raw)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *Database) RemoveShadowBan(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	encoded := base64.StdEncoding.EncodeToString([]byte(target))
	_, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users WHERE name=?1 OR trip=?2 OR hash=?3`, target, target, encoded)
	return err
}

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
	_, err := d.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(?1,?2,?3,?4,?5)`, value(record.Trip), value(record.Name), value(hash), value(record.Reason), time.Now().UnixMilli())
	return err
}

// HasShadowBanMatch tests exact identities without decoding stored hashes.
// Blank inputs bind NULL so they cannot match empty or missing identity fields.
func (d *Database) HasShadowBanMatch(ctx context.Context, trip, name, hash string) (bool, error) {
	if d == nil || d.DB == nil {
		return false, fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	nonblank := func(value string) any {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return value
	}
	var encodedHash any
	if strings.TrimSpace(hash) != "" {
		encodedHash = base64.StdEncoding.EncodeToString([]byte(hash))
	}
	var matched bool
	err := d.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM banned_users WHERE trip=?1 OR name=?2 OR hash=?3)`, nonblank(trip), nonblank(name), encodedHash).Scan(&matched)
	return matched, err
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
// semantics. One optional mention marker is removed for the name predicate;
// trip and hash predicates retain the exact supplied credential.
func (d *Database) RemoveShadowBanBySourceTarget(ctx context.Context, target string) (int64, error) {
	if d == nil || d.DB == nil {
		return 0, fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	nameTarget, err := util.NormalizeNickTarget(&target)
	if err != nil {
		return 0, err
	}
	result, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users WHERE name=?1 OR trip=?2 OR hash=?3`, nameTarget, target, base64.StdEncoding.EncodeToString([]byte(target)))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// RemoveAllShadowBans deletes every local shadow-ban identity record.
func (d *Database) RemoveAllShadowBans(ctx context.Context) (int64, error) {
	if d == nil || d.DB == nil {
		return 0, fmt.Errorf("shadow-ban database is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
