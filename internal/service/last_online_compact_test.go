package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"zenbot/internal/repository"
)

func TestLastOnlineCompactFacts(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	stamp := func(value time.Time) sql.NullInt64 { return sql.NullInt64{Int64: value.UnixMilli(), Valid: true} }
	for _, tc := range []struct {
		name   string
		record repository.LastOnlineRecord
		want   string
	}{
		{"sample", repository.LastOnlineRecord{Found: true, LastPresenceEvent: sql.NullString{String: "JOINED", Valid: true}, LastPresenceMillis: stamp(time.Date(2026, 9, 9, 22, 8, 7, 0, time.UTC)), LastMessageMillis: stamp(time.Date(2026, 8, 24, 15, 28, 4, 0, time.UTC)), LastMessage: sql.NullString{String: "gn", Valid: true}}, "nex — last seen joining · 9 Sep 22:08 UTC\nLast message · 24 Aug 15:28 UTC: gn"},
		{"message-newer", repository.LastOnlineRecord{Found: true, LastPresenceEvent: sql.NullString{String: "LEFT", Valid: true}, LastPresenceMillis: stamp(now.Add(-2 * time.Hour)), LastMessageMillis: stamp(now.Add(-5 * time.Minute)), LastMessage: sql.NullString{String: "hello\nworld \\n", Valid: true}}, "nex — last seen messaging · 5m ago\nLast presence: left · 2h ago\nLast message: hello\nworld \\n"},
		{"presence-only", repository.LastOnlineRecord{Found: true, LastPresenceEvent: sql.NullString{String: "LEFT", Valid: true}, LastPresenceMillis: stamp(now.Add(-30 * time.Second))}, "nex — last seen leaving · now\nNo public messages found."},
		{"no-timestamps", repository.LastOnlineRecord{Found: true}, "nex — last seen: unknown\nNo public messages found."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := UserService{Queries: &lastOnlineQueriesStub{record: tc.record}, Now: func() time.Time { return now }}
			got, err := s.LastOnline(context.Background(), "nex")
			if err != nil || got != tc.want {
				t.Fatalf("got=%q want=%q err=%v", got, tc.want, err)
			}
		})
	}
}
