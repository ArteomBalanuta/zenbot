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

func TestUserServiceLastOnlineRendersSaturnPayload(t *testing.T) {
	queries := &lastOnlineQueriesStub{record: repository.LastOnlineRecord{
		Found:          true,
		LastMessage:    sql.NullString{String: "quote \" slash \\ newline\n<>&\x01", Valid: true},
		LastSeenMillis: sql.NullInt64{Int64: 0, Valid: true},
		JoinedMillis:   sql.NullInt64{Int64: 12 * 60 * 60 * 1000, Valid: true},
	}}
	service := UserService{Queries: queries, Now: func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }}

	got, err := service.LastOnline(context.Background(), "merc")
	if err != nil {
		t.Fatal(err)
	}
	want := `\n Nick|Trip: merc\n Joined: Thu, 1 Jan 1970 12:00:00 GMT\n Last seen: Thu, 1 Jan 1970 00:00:00 GMT\n Seen active: 1 days, 0 hours, 0 minutes, 0 seconds ago.\n Session duration: 0 days, 12 hours, 0 minutes, 0 seconds \n Last message: quote \" slash \\ newline\n<>&\u0001\n`
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
	if queries.calls != 1 || queries.target != "merc" {
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

func TestUserServiceLastOnlineLeavesSessionFieldsDefaultWithoutLastMessage(t *testing.T) {
	service := UserService{Queries: &lastOnlineQueriesStub{record: repository.LastOnlineRecord{
		Found:        true,
		JoinedMillis: sql.NullInt64{Int64: 0, Valid: true},
	}}, Now: func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) }}

	got, err := service.LastOnline(context.Background(), "join-only")
	if err != nil {
		t.Fatal(err)
	}
	want := `\n Nick|Trip: join-only\n Joined:  - \n Last seen:  - \n Seen active:  -  ago.\n Session duration:  -  \n Last message:  - \n`
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}
