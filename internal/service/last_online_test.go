package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"zenbot/internal/repository"
)

type lastOnlineQueriesStub struct {
	record repository.LastOnlineRecord
	err    error
	target string
	calls  int
}

func (s *lastOnlineQueriesStub) RegisteredUsers(context.Context) ([]repository.RegisteredUser, error) {
	return nil, nil
}
func (s *lastOnlineQueriesStub) NicksByTrip(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *lastOnlineQueriesStub) BasicUserData(context.Context, string, string) (string, error) {
	return "", nil
}
func (s *lastOnlineQueriesStub) LastOnline(_ context.Context, target string) (repository.LastOnlineRecord, error) {
	s.calls++
	s.target = target
	return s.record, s.err
}

func (s *lastOnlineQueriesStub) LastSeen(ctx context.Context, target string) (repository.LastOnlineRecord, error) {
	return s.LastOnline(ctx, target)
}

func TestUserServiceLastOnlineRendersIndependentPublicFacts(t *testing.T) {
	queries := &lastOnlineQueriesStub{record: repository.LastOnlineRecord{
		Found:              true,
		LastMessage:        sql.NullString{String: "quote \" slash \\ newline\n<>&\x01", Valid: true},
		LastMessageMillis:  sql.NullInt64{Int64: 0, Valid: true},
		LastPresenceMillis: sql.NullInt64{Int64: 12 * 60 * 60 * 1000, Valid: true},
		LastPresenceEvent:  sql.NullString{String: "LEFT", Valid: true},
	}}
	want := "merc — last seen leaving · 12h ago\nLast message · 1 Jan 00:00 UTC: quote \" slash \\ newline\n<>&\u0001"
	for _, service := range []UserService{{Queries: queries}, {LastSeen: queries}} {
		service.Now = func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }
		got, err := service.LastOnline(context.Background(), "merc")
		if err != nil || got != want {
			t.Fatalf("payload=%q want=%q err=%v", got, want, err)
		}
	}
	if queries.calls != 2 || queries.target != "merc" {
		t.Fatalf("queries calls=%d target=%q", queries.calls, queries.target)
	}
}

func TestUserServiceLastOnlineReturnsNotFoundForMissingRows(t *testing.T) {
	service := UserService{Queries: &lastOnlineQueriesStub{}, Now: func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }}

	got, err := service.LastOnline(context.Background(), "absent")
	if got != "" || !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("payload=%q err=%v, want repository.ErrNotFound", got, err)
	}
}

func TestUserServiceLastOnlineRendersPresenceWithoutMessage(t *testing.T) {
	service := UserService{Queries: &lastOnlineQueriesStub{record: repository.LastOnlineRecord{
		Found:              true,
		LastPresenceMillis: sql.NullInt64{Int64: 0, Valid: true},
		LastPresenceEvent:  sql.NullString{String: "JOINED", Valid: true},
	}}, Now: func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }}

	got, err := service.LastOnline(context.Background(), "join-only")
	if err != nil {
		t.Fatal(err)
	}
	want := "join-only — last seen joining · 1 Jan 00:00 UTC\nNo public messages found."
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}
