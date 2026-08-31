package live

import (
	"context"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/turn"
	"zenbot/internal/repository"
)

type memoryRepositoryStub struct {
	rows                 []repository.AgentMemoryMessage
	appended             int
	key, user, assistant string
}

func (s *memoryRepositoryStub) LoadAgentMemory(context.Context, string, int64, int) ([]repository.AgentMemoryMessage, error) {
	return append([]repository.AgentMemoryMessage(nil), s.rows...), nil
}
func (s *memoryRepositoryStub) AppendAgentMemory(_ context.Context, key, user, assistant string, _, _ int64) error {
	s.appended++
	s.key, s.user, s.assistant = key, user, assistant
	return nil
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
