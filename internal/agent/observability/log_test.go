package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestEventCarriesRequestIdentityWithoutSensitivePayloads(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := WithRequest(context.Background(), Request{
		ID: "request-42", Mode: "DIRECT", Room: "programming", Nick: "alice",
	})
	Info(ctx, "agent.test", "tool", "room_users", "api_key", "must-not-appear")

	logged := output.String()
	for _, expected := range []string{"agent.test", "request_id=request-42", "mode=DIRECT", "room=programming", "nick=alice", "tool=room_users"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log %q does not contain %q", logged, expected)
		}
	}
	if strings.Contains(logged, "must-not-appear") || strings.Contains(logged, "api_key") {
		t.Fatalf("sensitive attribute escaped redaction: %q", logged)
	}
}

func TestErrorRecordsStageAndCause(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := WithStage(WithRequest(context.Background(), Request{ID: "request-7"}), "llm.initial")
	Error(ctx, "agent.llm.failed", context.DeadlineExceeded)

	logged := output.String()
	for _, expected := range []string{"level=ERROR", "agent.llm.failed", "request_id=request-7", "stage=llm.initial", "error=\"context deadline exceeded\""} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log %q does not contain %q", logged, expected)
		}
	}
}
