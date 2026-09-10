package live

import (
	"context"
	"strings"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/turn"
	"zenbot/internal/repository"
)

type memoryRepositoryStub struct {
	rows                 []repository.AgentMemoryMessage
	appended             int
	key, user, assistant string
	summary              *repository.AgentMemorySummary
	upserted             *repository.AgentMemorySummary
	upsertCount          int
}

func (s *memoryRepositoryStub) LoadAgentMemory(context.Context, string, int64, int) ([]repository.AgentMemoryMessage, error) {
	return append([]repository.AgentMemoryMessage(nil), s.rows...), nil
}
func (s *memoryRepositoryStub) AppendAgentMemory(_ context.Context, key, user, assistant string, _, _ int64) error {
	s.appended++
	s.key, s.user, s.assistant = key, user, assistant
	return nil
}
func (s *memoryRepositoryStub) LoadAgentMemorySummary(context.Context, string, int64) (*repository.AgentMemorySummary, error) {
	if s.summary == nil {
		return nil, nil
	}
	copy := *s.summary
	return &copy, nil
}
func (s *memoryRepositoryStub) UpsertAgentMemorySummary(_ context.Context, summary repository.AgentMemorySummary) error {
	s.upserted = &summary
	s.upsertCount++
	return nil
}

type liveMemorySummarizer struct{}

func (liveMemorySummarizer) Summarize(context.Context, []llm.LlmMessage) (string, error) {
	return "facts from older turns", nil
}

func TestPersistentMemoryStoreLoadsChronologicallyAndAppendsAtomically(t *testing.T) {
	repo := &memoryRepositoryStub{rows: []repository.AgentMemoryMessage{{Role: "user", Content: "old"}, {Role: "assistant", Content: "answer"}}}
	store := PersistentMemoryStore{Repository: repo, Turns: 6, TTL: time.Hour, Clock: func() time.Time { return time.Unix(10, 0) }}
	memory, err := turn.NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := memory.Load(ctx, "id")
	if err != nil || len(got) != 2 || got[0].Content() != "old" || got[1].Content() != "answer" {
		t.Fatalf("memory=%#v err=%v", got, err)
	}
	if err := memory.Append(ctx, "prompt", "visible", "id"); err != nil {
		t.Fatal(err)
	}
	if repo.appended != 1 || repo.key != ctx.MemoryKey() || repo.user != "prompt" || repo.assistant != "visible" {
		t.Fatalf("append=%#v", repo)
	}
}

func TestPersistentMemoryStoreDoesNotAppendAfterCancellation(t *testing.T) {
	repo := &memoryRepositoryStub{}
	store := PersistentMemoryStore{Repository: repo, Turns: 1, TTL: time.Hour}
	memory, err := turn.NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := memory.AppendContext(cancelled, agent, "prompt", "visible", "id"); err == nil {
		t.Fatal("cancelled append succeeded")
	}
	if repo.appended != 0 {
		t.Fatalf("cancelled append persisted %d exchanges", repo.appended)
	}
}

func TestPersistentMemoryStoreCompactsAndPersistsOldestTurns(t *testing.T) {
	repo := &memoryRepositoryStub{rows: []repository.AgentMemoryMessage{
		{ID: 1, Role: "user", Content: "u1", CreatedOnMillis: 1},
		{ID: 2, Role: "assistant", Content: "a1", CreatedOnMillis: 1},
		{ID: 3, Role: "user", Content: "u2", CreatedOnMillis: 2},
		{ID: 4, Role: "assistant", Content: "a2", CreatedOnMillis: 2},
		{ID: 5, Role: "user", Content: "u3", CreatedOnMillis: 3},
		{ID: 6, Role: "assistant", Content: "a3", CreatedOnMillis: 3},
	}}
	compactor := turn.MemoryCompactor{Summarizer: liveMemorySummarizer{}}
	store := PersistentMemoryStore{Repository: repo, Turns: 6, TTL: time.Hour, Clock: func() time.Time { return time.UnixMilli(10) }, Compactor: &compactor, RawTurnLimit: 1}
	agent, _ := api.NewContext("room", "nick", "", "", false, []string{})

	got, err := store.LoadContext(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !strings.Contains(got[0].Content(), "facts from older turns") || got[1].Content() != "u3" || got[2].Content() != "a3" {
		t.Fatalf("compacted memory=%#v", got)
	}
	if repo.upserted == nil || repo.upserted.CoveredThroughID != 4 || repo.upserted.IdentityKey != agent.MemoryKey() || repo.upserted.Fingerprint == "" {
		t.Fatalf("persisted summary=%#v", repo.upserted)
	}
	repo.summary = repo.upserted
	again, err := store.LoadContext(context.Background(), agent)
	if err != nil || len(again) != 3 || repo.upsertCount != 1 {
		t.Fatalf("second projection=%#v upserts=%d err=%v", again, repo.upsertCount, err)
	}
}
