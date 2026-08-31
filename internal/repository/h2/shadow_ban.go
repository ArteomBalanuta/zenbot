package h2

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"
	"zenbot/internal/model"
)

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
