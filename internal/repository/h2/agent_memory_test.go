package h2_test

import (
	"context"
	"testing"

	"zenbot/internal/repository"
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

func TestAppendAgentTurnRollsBackConversationWhenEvidenceInsertFails(t *testing.T) {
	db := h2fixture.Open(t, "agent-turn-rollback")
	ctx := context.Background()
	if _, err := db.DB.ExecContext(ctx, `ALTER TABLE agent_tool_memory ADD CONSTRAINT reject_forced_evidence CHECK (content <> '"force-failure"')`); err != nil {
		t.Fatal(err)
	}
	record := repository.AgentTurnRecord{
		IdentityKey:     "room|public",
		User:            "question",
		Assistant:       "answer",
		Evidence:        []repository.AgentTurnEvidence{{ToolName: "room_users", Content: `"force-failure"`}},
		CreatedOnMillis: 10,
		ExpiresOnMillis: 20,
	}
	if err := db.AppendAgentTurn(ctx, record); err == nil {
		t.Fatal("forced evidence failure committed")
	}
	var memoryCount, evidenceCount int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_memory WHERE identity_key = $1`, record.IdentityKey).Scan(&memoryCount); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_tool_memory WHERE identity_key = $1`, record.IdentityKey).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	if memoryCount != 0 || evidenceCount != 0 {
		t.Fatalf("partial turn persisted: memory=%d evidence=%d", memoryCount, evidenceCount)
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

func TestAgentMemorySummaryRepositoryRoundTripsLatestUnexpiredSummary(t *testing.T) {
	db := h2fixture.Open(t, "agent-memory-summary")
	ctx := context.Background()
	want := repository.AgentMemorySummary{
		IdentityKey:      "room|public",
		Content:          "durable summary",
		CoveredThroughID: 42,
		Fingerprint:      "fingerprint",
		CreatedOnMillis:  100,
		ExpiresOnMillis:  200,
	}
	if err := db.UpsertAgentMemorySummary(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := db.LoadAgentMemorySummary(ctx, want.IdentityKey, 150)
	if err != nil || got == nil || *got != want {
		t.Fatalf("summary=%#v err=%v", got, err)
	}
	expired, err := db.LoadAgentMemorySummary(ctx, want.IdentityKey, 200)
	if err != nil || expired != nil {
		t.Fatalf("expired summary=%#v err=%v", expired, err)
	}
}
