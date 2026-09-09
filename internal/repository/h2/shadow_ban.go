package h2

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"zenbot/internal/model"
)

func (d *Database) PersistShadowBanSelector(ctx context.Context, name, reason string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("shadow-ban target is required")
	}
	_, err := d.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(NULL,$1,NULL,$2,$3)`, name, reason, time.Now().UnixMilli())
	return err
}

func (d *Database) ListShadowBans(ctx context.Context) ([]model.BanRecord, error) {
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
	_, err := d.DB.ExecContext(ctx, `DELETE FROM banned_users WHERE name=$1 OR trip=$2 OR hash=$3`, target, target, encoded)
	return err
}

// PersistShadowBan mirrors Saturn's persistent shadow-ban contract: trusted
// join identity is stored locally; no hack.chat Ban command is emitted.
func (d *Database) PersistShadowBan(ctx context.Context, user model.User, reason string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("shadow-ban database is unavailable")
	}
	hash := user.Hash
	if hash != "" {
		hash = base64.StdEncoding.EncodeToString([]byte(hash))
	}
	_, err := d.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES($1,$2,$3,$4,$5)`, user.Trip, user.Name, hash, reason, time.Now().UnixMilli())
	return err
}
