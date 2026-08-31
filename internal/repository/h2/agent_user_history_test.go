package h2_test

import (
	"context"
	"testing"

	"zenbot/internal/repository/h2"
	"zenbot/internal/testutil/h2fixture"
)

func TestRecentPublicRoomMessagesForNickFiltersBoundsAndOrdersRealH2(t *testing.T) {
	db := h2fixture.Open(t, "agent-user-history")
	insert := func(name, message string, created int64, room, visibility string) {
		t.Helper()
		if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, trip, hash, message, created_on, channel, visibility) VALUES ($1,'trip','hash',$2,$3,$4,$5)`, name, message, created, room, visibility); err != nil {
			t.Fatal(err)
		}
	}
	insert("Alice", "old", 10, "Lounge", "PUBLIC")
	insert("alice", "new", 20, "lounge", "PUBLIC")
	insert("alice", "secret", 30, "lounge", "WHISPER")
	insert("alice", "elsewhere", 40, "other", "PUBLIC")
	if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name, message, created_on, channel, visibility) VALUES ('alice','legacy',50,'lounge',NULL)`); err != nil {
		t.Fatal(err)
	}

	rows, err := db.RecentPublicRoomMessagesForNick(context.Background(), "LOUNGE", "ALICE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "alice" || rows[0].Message != "new" || rows[0].Channel != "lounge" {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].Trip != "trip" || rows[0].Hash != "hash" {
		t.Fatalf("fixture unexpectedly changed: %#v", rows[0])
	}
}

func TestRecentPublicRoomMessagesForNickRejectsInvalidAndCannotBroadenRoom(t *testing.T) {
	var nilDB *h2.Database
	if _, err := nilDB.RecentPublicRoomMessagesForNick(context.Background(), "room", "nick", 1); err == nil {
		t.Fatal("nil database accepted")
	}
	db := h2fixture.Open(t, "agent-user-history-invalid")
	if _, err := db.DB.ExecContext(context.Background(), `INSERT INTO messages(name,message,created_on,channel,visibility) VALUES ('alice','evidence',1,'lounge','PUBLIC')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		room, nick string
		limit      int
	}{{" ", "alice", 1}, {"lounge", " ", 1}, {"lounge", "alice", 0}, {" lounge ", "alice", 1}, {`lounge' OR '1'='1`, "alice", 1}} {
		rows, err := db.RecentPublicRoomMessagesForNick(context.Background(), tc.room, tc.nick, tc.limit)
		if tc.room == " " || tc.nick == " " || tc.limit == 0 {
			if err == nil {
				t.Fatalf("accepted %#v", tc)
			}
		} else if err != nil || len(rows) != 0 {
			t.Fatalf("scope broadened %#v: rows=%#v err=%v", tc, rows, err)
		}
	}
}
