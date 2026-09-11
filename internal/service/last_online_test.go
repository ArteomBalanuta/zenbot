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
	want := `\n Nick|Trip: merc\n Last observed: Thu, 1 Jan 1970 12:00:00 GMT\n Last presence event: LEFT at Thu, 1 Jan 1970 12:00:00 GMT\n Last public message: Thu, 1 Jan 1970 00:00:00 GMT — quote \" slash \\ newline\n<>&\u0001\n`
	for _, service := range []UserService{{Queries: queries}, {LastSeen: queries}} {
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
	want := `\n Nick|Trip: join-only\n Last observed: Thu, 1 Jan 1970 00:00:00 GMT\n Last presence event: JOINED at Thu, 1 Jan 1970 00:00:00 GMT\n Last public message:  - \n`
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}
