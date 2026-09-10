package h2_test

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/repository/h2"
	"zenbot/internal/testutil/h2fixture"
)

func TestRecentPublicRoomMessagesFiltersBoundsAndOrdersRealH2(t *testing.T) {
	db := h2fixture.Open(t, "agent-context")
	insert := func(name, trip, hash, message string, created int64, room, visibility string) {
		t.Helper()
		if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, trip, hash, message, created_on, channel, visibility) VALUES ($1,$2,$3,$4,$5,$6,$7)`, name, trip, hash, message, created, room, visibility); err != nil {
			t.Fatal(err)
		}
	}
	insert("old", "trip-old", "hash-old", "old", 10, "Lounge", "PUBLIC")
	if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, trip, hash, message, created_on, channel, visibility) VALUES ($1,NULL,NULL,NULL,$2,$3,'PUBLIC')`, "null-fields", 15, "lounge"); err != nil {
		t.Fatal(err)
	}
	insert("first-tie", "trip-a", "hash-a", `quoted " \ unicode ☃`, 20, "lOuNgE", "PUBLIC")
	insert("second-tie", "trip-b", "hash-b", "newest", 20, "LOUNGE", "PUBLIC")
	insert("whisper", "secret-trip", "secret-hash", "secret", 30, "lounge", "WHISPER")
	insert("other", "other-trip", "other-hash", "other room", 40, "elsewhere", "PUBLIC")
	if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, message, created_on, channel, visibility) VALUES ($1,$2,$3,$4,NULL)`, "legacy", "legacy secret", 50, "lounge"); err != nil {
		t.Fatal(err)
	}

	rows, err := db.RecentPublicRoomMessages(context.Background(), "LOUNGE", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3: %#v", len(rows), rows)
	}
	if rows[0].Name != "null-fields" || rows[1].Name != "first-tie" || rows[2].Name != "second-tie" {
		t.Fatalf("chronological tied rows = %#v", rows)
	}
	if rows[0].Trip != "" || rows[0].Hash != "" || rows[0].Message != "" || rows[0].Channel != "lounge" {
		t.Fatalf("nullable fields were not safely mapped: %#v", rows[0])
	}
	if rows[1].Trip != "trip-a" || rows[1].Hash != "hash-a" || rows[1].Channel != "lOuNgE" || rows[1].Message != `quoted " \ unicode ☃` || rows[1].CreatedOnMillis != 20 {
		t.Fatalf("special/public fields = %#v", rows[1])
	}
	for _, row := range rows {
		if strings.Contains(row.Message, "secret") || row.Name == "other" {
			t.Fatalf("leaked non-public or cross-room row: %#v", row)
		}
	}
}

func TestRecentPublicRoomMessagesRejectsInvalidArguments(t *testing.T) {
	var nilDB *h2.Database
	if _, err := nilDB.RecentPublicRoomMessages(context.Background(), "room", 1); err == nil {
		t.Fatal("nil database accepted")
	}
	db := h2fixture.Open(t, "agent-context-invalid")
	for _, test := range []struct {
		room  string
		limit int
	}{
		{" ", 1},
		{"room", 0},
		{"room", -1},
	} {
		if _, err := db.RecentPublicRoomMessages(context.Background(), test.room, test.limit); err == nil {
			t.Fatalf("invalid arguments accepted: %#v", test)
		}
	}
}

func TestRecentPublicRoomMessagesUsesExactParameterizedRoom(t *testing.T) {
	db := h2fixture.Open(t, "agent-context-room-scope")
	if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, message, created_on, channel, visibility) VALUES ($1,$2,$3,$4,'PUBLIC')`, "alice", "lounge evidence", 1, "lounge"); err != nil {
		t.Fatal(err)
	}

	for _, room := range []string{" lounge ", `lounge' OR '1'='1`} {
		rows, err := db.RecentPublicRoomMessages(context.Background(), room, 10)
		if err != nil {
			t.Fatalf("room %q query failed: %v", room, err)
		}
		if len(rows) != 0 {
			t.Fatalf("room %q leaked exact-room evidence: %#v", room, rows)
		}
	}
}
