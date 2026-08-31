package h2_test

import (
	"context"
	"encoding/base64"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/testutil/h2fixture"
)

func TestPersistShadowBanStoresTrustedIdentityInRealH2(t *testing.T) {
	db := h2fixture.Open(t, "shadow-ban")
	user := model.User{Name: "raider", Trip: "trip", Hash: "raw-hash"}
	if err := db.PersistShadowBan(context.Background(), user, "repeated same-hash raid"); err != nil {
		t.Fatal(err)
	}
	var trip, name, hash, reason string
	if err := db.DB.QueryRowContext(context.Background(), `SELECT trip,name,hash,reason FROM banned_users`).Scan(&trip, &name, &hash, &reason); err != nil {
		t.Fatal(err)
	}
	if trip != user.Trip || name != user.Name || hash != base64.StdEncoding.EncodeToString([]byte(user.Hash)) || reason != "repeated same-hash raid" {
		t.Fatalf("persisted shadow-ban = trip=%q name=%q hash=%q reason=%q", trip, name, hash, reason)
	}
}
