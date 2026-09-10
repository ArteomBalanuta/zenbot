package h2_test

import (
	"context"
	"testing"

	"zenbot/internal/repository"
	"zenbot/internal/repository/h2"
	"zenbot/internal/testutil/h2fixture"
)

func TestAgentToolEvidenceRepositoryRealH2IsolatesBoundsOrdersAndExpires(t *testing.T) {
	db := h2fixture.Open(t, "agent-tool-memory")
	ctx := context.Background()
	for _, row := range []struct {
		key, tool, content string
		created, expires   int64
	}{
		{"room|public", "room_users", `{"users":["old"]}`, 10, 100},
		{"room|public", "user_message_history", `{"rows":[]}`, 20, 100},
		{"room|whisper|trip:x", "room_users", `{"users":["secret"]}`, 30, 100},
	} {
		if err := db.AppendAgentToolEvidence(ctx, row.key, row.tool, row.content, row.created, row.expires); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO agent_tool_memory(identity_key, tool_name, content, created_on, expires_on) VALUES ($1,$2,$3,$4,$5)`, "room|public", "room_users", `bad`, 15, 100); err != nil {
		t.Fatal(err)
	}
	got, err := db.LoadAgentToolEvidence(ctx, "room|public", 50, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ToolName != "user_message_history" || got[0].Content != `{"rows":[]}` || got[0].CreatedOnMillis != 20 {
		t.Fatalf("bounded chronological evidence = %#v", got)
	}
	if _, err := db.LoadAgentToolEvidence(ctx, "room|public", 50, 3); err == nil {
		t.Fatal("malformed stored JSON accepted")
	}
	if err := db.AppendAgentToolEvidence(ctx, "room|public", "room_users", `{"users":[]}`, 100, 101); err != nil {
		t.Fatal(err)
	}
	got, err = db.LoadAgentToolEvidence(ctx, "room|public", 100, 4)
	if err != nil || len(got) != 1 || got[0].CreatedOnMillis != 100 {
		t.Fatalf("expiry/cleanup = %#v, %v", got, err)
	}
}

func TestAgentToolEvidenceRepositoryRejectsInvalidAndCancelled(t *testing.T) {
	var nilDB *h2.Database
	if _, err := nilDB.LoadAgentToolEvidence(context.Background(), "key", 0, 1); err == nil {
		t.Fatal("nil db accepted")
	}
	db := h2fixture.Open(t, "agent-tool-memory-invalid")
	for _, args := range [][]string{{"", "room_users", `{}`}, {"key", "", `{}`}, {"key", "room_users", " "}} {
		if err := db.AppendAgentToolEvidence(context.Background(), args[0], args[1], args[2], 1, 2); err == nil {
			t.Fatalf("invalid append accepted: %#v", args)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.AppendAgentToolEvidence(cancelled, "key", "room_users", `{}`, 1, 2); err == nil {
		t.Fatal("cancelled append accepted")
	}
	var _ repository.AgentToolEvidenceRepository = db
}
