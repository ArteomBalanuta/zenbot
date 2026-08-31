package live

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/runtime"
	"zenbot/internal/repository"
)

type contextRepositoryStub struct {
	rows  []repository.PublicRoomMessage
	err   error
	calls int
	room  string
	limit int
}

func (s *contextRepositoryStub) RecentPublicRoomMessages(_ context.Context, room string, limit int) ([]repository.PublicRoomMessage, error) {
	s.calls++
	s.room, s.limit = room, limit
	return s.rows, s.err
}

func TestRepositoryConversationContextProviderSerializesAndRemovesNewestCurrentDuplicate(t *testing.T) {
	repo := &contextRepositoryStub{rows: []repository.PublicRoomMessage{
		{Name: "alice", Message: "same", Channel: "room", CreatedOnMillis: 1},
		{Name: `ali"ce`, Trip: "trip", Hash: "hash", Message: `quoted " \ ☃`, Channel: "room", CreatedOnMillis: 2},
		{Name: "alice", Message: "same", Channel: "room", CreatedOnMillis: 3},
	}}
	provider, err := NewRepositoryConversationContextProvider(repo, 7)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("request", runtime.NewContext("Room", "alice", "", "", false, nil), "prompt", runtime.MENTION, "same", false)
	contextJSON, err := provider.Load(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 || repo.room != "Room" || repo.limit != 7 {
		t.Fatalf("repository call = %#v", repo)
	}
	if strings.Contains(contextJSON, `"createdOn":3`) || !strings.Contains(contextJSON, `"createdOn":1`) || !strings.Contains(contextJSON, `quoted \" \\ ☃`) {
		t.Fatalf("context JSON = %s", contextJSON)
	}
	if !strings.HasPrefix(contextJSON, `{"rows":[`) || !strings.Contains(contextJSON, `"name":"ali\"ce"`) {
		t.Fatalf("context JSON not escaped envelope: %s", contextJSON)
	}
}

func TestRepositoryConversationContextProviderEmptyWhisperValidationAndErrors(t *testing.T) {
	if _, err := NewRepositoryConversationContextProvider(nil, 1); err == nil {
		t.Fatal("nil repository accepted")
	}
	repo := &contextRepositoryStub{}
	if _, err := NewRepositoryConversationContextProvider(repo, 0); err == nil {
		t.Fatal("zero limit accepted")
	}
	provider, err := NewRepositoryConversationContextProvider(repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	public := runtime.NewInvocation("request", runtime.NewContext("room", "alice", "", "", false, nil), "prompt", runtime.MENTION, "new", false)
	value, err := provider.Load(context.Background(), public)
	if err != nil || value != `{"rows":[]}` {
		t.Fatalf("empty context = %q, %v", value, err)
	}
	whisper := runtime.NewInvocation("request", runtime.NewContext("room", "alice", "", "", true, nil), "prompt", runtime.DIRECT, "new", true)
	value, err = provider.Load(context.Background(), whisper)
	if err != nil || value != "" || repo.calls != 1 {
		t.Fatalf("whisper context = %q, %v calls=%d", value, err, repo.calls)
	}
	repo.err = errors.New("database unavailable")
	if _, err := provider.Load(context.Background(), public); !errors.Is(err, repo.err) {
		t.Fatalf("repository error = %v", err)
	}
}

func TestLoadRecentContextDegradesErrorsAndPreservesCancellation(t *testing.T) {
	repo := &contextRepositoryStub{err: errors.New("unavailable")}
	provider, err := NewRepositoryConversationContextProvider(repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("request-42", runtime.NewContext("room", "alice", "", "", false, nil), "prompt", runtime.MENTION, "new", false)
	value, err := loadRecentContext(context.Background(), provider, inv)
	if err != nil || value != "" {
		t.Fatalf("fallback = %q, %v", value, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	value, err = loadRecentContext(cancelled, provider, inv)
	if value != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %q, %v", value, err)
	}
	repo.err = context.DeadlineExceeded
	value, err = loadRecentContext(context.Background(), provider, inv)
	if value != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider cancellation was degraded = %q, %v", value, err)
	}
}
