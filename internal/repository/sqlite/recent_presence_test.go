package sqlite

import (
	"context"
	"testing"
	"time"

	"zenbot/internal/model"
)

func TestRecentPresenceNamesReadsPresenceAuditAndLegacyMessageRows(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UnixMilli()
	if _, err := db.PresenceAudit(context.Background(), model.PresenceRecord{Name: "new-name", Trip: "trip", Hash: "hash", EventType: "joined", CreatedOnMillis: now, Channel: "programming"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MessageAudit(context.Background(), model.MessageRecord{Name: "old-name", Trip: "trip", Hash: "hash", Message: "LEFT", CreatedOnMillis: now - 1, Channel: "programming", Visibility: "PUBLIC"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PresenceAudit(context.Background(), model.PresenceRecord{Name: "expired", Trip: "trip", Hash: "hash", EventType: "left", CreatedOnMillis: now - int64(time.Hour/time.Millisecond), Channel: "programming"}); err != nil {
		t.Fatal(err)
	}

	names, err := db.RecentPresenceNames(context.Background(), "hash", "trip", now-int64(15*time.Minute/time.Millisecond), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "new-name" || names[1] != "old-name" {
		t.Fatalf("names=%v", names)
	}
}
