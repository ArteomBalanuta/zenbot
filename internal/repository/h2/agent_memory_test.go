package h2_test

import (
	"context"
	"testing"

	"zenbot/internal/repository/h2"
	"zenbot/internal/testutil/h2fixture"
)

func TestAgentMemoryRepositoryRealH2IsolatesBoundsOrdersAndExpires(t *testing.T) {
	db := h2fixture.Open(t, "agent-memory")
	ctx := context.Background()
	if err := db.AppendAgentMemory(ctx, "room|public", "old user", "old assistant", 10, 100); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendAgentMemory(ctx, "room|public", "new user", "new assistant", 20, 100); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendAgentMemory(ctx, "room|whisper|trip:x", "secret user", "secret assistant", 30, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO agent_memory(identity_key, role, content, created_on, expires_on) VALUES ($1,$2,$3,$4,$5)`, "room|public", "user", "expired", 40, 50); err != nil {
		t.Fatal(err)
	}

	got, err := db.LoadAgentMemory(ctx, "room|public", 50, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Role != "user" || got[0].Content != "new user" || got[1].Role != "assistant" || got[1].Content != "new assistant" {
		t.Fatalf("bounded chronological memory = %#v", got)
	}
	empty, err := db.LoadAgentMemory(ctx, "room|public", 100, 6)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("expiry result = %#v, %v", empty, err)
	}
}

func TestAgentMemoryRepositoryRejectsInvalidAndCancelledRequests(t *testing.T) {
	var nilDB *h2.Database
	if _, err := nilDB.LoadAgentMemory(context.Background(), "key", 0, 1); err == nil {
		t.Fatal("nil db accepted")
	}
	db := h2fixture.Open(t, "agent-memory-invalid")
	for _, key := range []string{"", " "} {
		if _, err := db.LoadAgentMemory(context.Background(), key, 0, 1); err == nil {
			t.Fatal("blank key accepted")
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.AppendAgentMemory(cancelled, "key", "user", "assistant", 1, 2); err == nil {
		t.Fatal("cancelled append accepted")
	}
}
