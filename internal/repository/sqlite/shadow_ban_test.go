package sqlite_test

import (
	"context"
	"encoding/base64"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/testutil/sqlitefixture"
)

func TestPersistShadowBanStoresTrustedIdentityInRealSQLite(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-ban")
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

func TestRemoveShadowBanBySourceTargetMatchesNameTripAndBase64Name(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-ban-reversal")
	ctx := context.Background()
	name := "author"
	rows := []struct {
		trip string
		name string
		hash string
	}{
		{trip: "other-trip", name: name, hash: "other-hash"},
		{trip: name, name: "other-name", hash: "other-hash"},
		{trip: "other-trip", name: "other-name", hash: base64.StdEncoding.EncodeToString([]byte(name))},
		{trip: "unmatched-trip", name: "unmatched-name", hash: "unmatched-hash"},
	}
	for _, row := range rows {
		if _, err := db.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(?1,?2,?3,?4,?5)`, row.trip, row.name, row.hash, "seed", 1); err != nil {
			t.Fatal(err)
		}
	}
	if changed, err := db.RemoveShadowBanBySourceTarget(ctx, name); err != nil || changed != 3 {
		t.Fatalf("changed=%d err=%v", changed, err)
	}
	var remaining int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM banned_users`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining rows = %d, want 1", remaining)
	}
	if changed, err := db.RemoveShadowBanBySourceTarget(ctx, name); err != nil || changed != 0 {
		t.Fatalf("idempotent zero-row deletion: changed=%d err=%v", changed, err)
	}
}

func TestRemoveShadowBanBySourceTargetNormalizesMentionForNameOnly(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-ban-name-mention")
	ctx := context.Background()
	for _, row := range []struct{ trip, name, hash string }{
		{trip: "other", name: "alice", hash: "other"},
		{trip: "@alice", name: "other-trip", hash: "other"},
		{trip: "other", name: "other-hash", hash: base64.StdEncoding.EncodeToString([]byte("@alice"))},
		{trip: "alice", name: "plain-trip-must-remain", hash: "other"},
		{trip: "other", name: "plain-hash-must-remain", hash: base64.StdEncoding.EncodeToString([]byte("alice"))},
	} {
		if _, err := db.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(?1,?2,?3,'seed',1)`, row.trip, row.name, row.hash); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := db.RemoveShadowBanBySourceTarget(ctx, "@alice")
	if err != nil || changed != 3 {
		t.Fatalf("changed=%d err=%v", changed, err)
	}
	var remaining int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM banned_users`).Scan(&remaining); err != nil || remaining != 2 {
		t.Fatalf("remaining=%d err=%v", remaining, err)
	}
}

func TestRemoveShadowBanBySourceTargetTreatsSQLLookingNameAsValue(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-ban-reversal-injection")
	ctx := context.Background()
	name := "author' OR '1'='1"
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES(?1,?2,?3,?4,?5)`, "other-trip", "unmatched", "unmatched", "seed", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RemoveShadowBanBySourceTarget(ctx, name); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM banned_users`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("SQL-looking target removed unrelated rows: %d", remaining)
	}
}

func TestShadowBanCommandRecordsRoundTripAndDeleteAllInRealSQLite(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-ban-command-records")
	ctx := context.Background()
	if err := db.PersistShadowBanRecord(ctx, repository.ShadowBanRecord{Trip: "trip", Name: "nick", Hash: "raw-hash", Reason: "reason"}); err != nil {
		t.Fatal(err)
	}
	if err := db.PersistShadowBanRecord(ctx, repository.ShadowBanRecord{Name: "offline"}); err != nil {
		t.Fatal(err)
	}
	records, err := db.ListShadowBans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0] != (repository.ShadowBanRecord{Trip: "trip", Name: "nick", Hash: "raw-hash", Reason: "reason"}) || records[1] != (repository.ShadowBanRecord{Name: "offline"}) {
		t.Fatalf("records=%+v", records)
	}
	if changed, err := db.RemoveAllShadowBans(ctx); err != nil || changed != 2 {
		t.Fatalf("changed=%d err=%v", changed, err)
	}
	if records, err = db.ListShadowBans(ctx); err != nil || len(records) != 0 {
		t.Fatalf("after delete-all records=%+v err=%v", records, err)
	}
}
