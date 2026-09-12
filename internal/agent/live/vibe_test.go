package live

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/turn"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func vibeInvocation() runtime.Invocation {
	return runtime.NewInvocation("vibe-test", runtime.NewContext("room", "merc", "", "", false, nil), "Summarize this room's vibe.", runtime.Mode("VIBE"), "*vibe", true)
}

func TestVibeRunnerUsesRoomEvidenceAndDedicatedPrompt(t *testing.T) {
	repo := &contextRepositoryStub{rows: []repository.PublicRoomMessage{{Name: "merc", Message: "Let's compare these two models", Channel: "room", CreatedOnMillis: 1}}}
	provider, _ := NewRepositoryConversationContextProvider(repo, 60)
	client := &captureLiveClient{}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ConversationContext: provider}
	result, err := runner.Run(context.Background(), vibeInvocation())
	if err != nil || !result.ShouldReply() {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d", len(client.requests))
	}
	var content string
	for _, m := range client.requests[0].Messages() {
		content += m.Content()
	}
	if !strings.Contains(content, "Let's compare") || !strings.Contains(content, "participant") {
		t.Fatalf("missing evidence or analysis guidance: %s", content)
	}
}

func TestVibeHistoryFailureDoesNotBecomeEmptyConversation(t *testing.T) {
	repo := &contextRepositoryStub{err: errors.New("history offline")}
	provider, _ := NewRepositoryConversationContextProvider(repo, 60)
	client := &captureLiveClient{}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ConversationContext: provider}
	_, err := runner.Run(context.Background(), vibeInvocation())
	if !errors.Is(err, repo.err) || len(client.requests) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(client.requests))
	}
}

func TestVibeContextFiltersBoundsAndNormalizes(t *testing.T) {
	repo := &contextRepositoryStub{rows: []repository.PublicRoomMessage{
		{Name: "merc", Message: "first", Channel: "room", CreatedOnMillis: 1},
		{Name: "bot", Message: "automated", Channel: "room", CreatedOnMillis: 2},
		{Name: "outsider", Message: "private-other-room", Channel: "other", CreatedOnMillis: 3},
		{Name: "merc", Message: "*vibe", Channel: "room", CreatedOnMillis: 4},
		{Name: "john", Message: "*weather city", Channel: "room", CreatedOnMillis: 5},
		{Name: " @merc ", Trip: "not-needed", Hash: "not-needed", Message: "second", Channel: "room", CreatedOnMillis: 6},
		{Name: "alex", Message: strings.Repeat("界", 1200), Channel: "room", CreatedOnMillis: 7},
	}}
	p, _ := NewRepositoryConversationContextProvider(repo, 100)
	p.BotNames = func() []string { return []string{"@BOT"} }
	p.CommandPrefix = func() string { return "." }
	p.CommandPrefixes = func() []string { return []string{".", "*"} }
	got, err := p.Load(context.Background(), vibeInvocation())
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Rows []struct {
			Name, Message string
			Truncated     bool
		}
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatal(err)
	}
	if repo.limit != 60 || len(payload.Rows) != 3 || payload.Rows[0].Message != "first" || payload.Rows[1].Name != "merc" || payload.Rows[1].Message != "second" || len([]rune(payload.Rows[2].Message)) != 1000 || !payload.Rows[2].Truncated || strings.Contains(got, "not-needed") {
		t.Fatalf("bad context: %s", got)
	}
}

func TestVibeSubmissionThroughRuntimeDeliversWithoutToolsOrMemory(t *testing.T) {
	repo := &contextRepositoryStub{}
	p, _ := NewRepositoryConversationContextProvider(repo, 60)
	client := &captureLiveClient{}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ConversationContext: p, ToolLoop: &ToolLoop{}, Memory: &turn.TurnMemory{}}
	delivered := make(chan runtime.Result, 1)
	rt, err := runtime.New(runtime.Config{MaxConcurrent: 1}, runner, runtime.SinkFunc(func(_ context.Context, inv runtime.Invocation, result runtime.Result) error {
		if inv.Mode() != runtime.VIBE || inv.Context().Room() != "room" {
			return errors.New("wrong invocation")
		}
		delivered <- result
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	a := DirectSubmissionAdapter{Service: RuntimeService{Runtime: rt}, Factory: participation.NewInvocationFactory(nil), Snapshot: func() participation.TrustedSnapshot { return participation.TrustedSnapshot{Room: "room"} }}
	if err := a.SubmitVibe(context.Background(), &model.ChatMessage{Name: "merc", Text: "*vibe"}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-delivered:
		if !result.ShouldReply() || result.Text() != "answer" || len(client.requests) != 1 || len(client.requests[0].Tools()) != 0 {
			t.Fatalf("result=%v calls=%d", result, len(client.requests))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no runtime delivery")
	}
}

func TestVibeWhisperAndMissingHistoryDoNotCallModel(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		client := &captureLiveClient{}
		runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}}
		inv := runtime.NewInvocation("id", runtime.NewContext("room", "merc", "", "", whisper, nil), "vibe", runtime.VIBE, "*vibe", true)
		result, err := runner.Run(context.Background(), inv)
		if len(client.requests) != 0 || (whisper && (err != nil || !result.ShouldReply())) || (!whisper && err == nil) {
			t.Fatalf("whisper=%v result=%v err=%v", whisper, result, err)
		}
	}
}

func TestVibeRejectsUnexpectedProviderToolCalls(t *testing.T) {
	p, _ := NewRepositoryConversationContextProvider(&contextRepositoryStub{}, 60)
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("summary", []llm.LlmToolCall{llm.NewLlmToolCall("call", "saturn_kick", map[string]any{"nick": "merc"})}, "tool_calls")}}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, ConversationContext: p, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}}
	if result, err := runner.Run(context.Background(), vibeInvocation()); err == nil || result.ShouldReply() {
		t.Fatalf("tool call accepted: %v %v", result, err)
	}
}

func TestVibeFinalizerPreservesNaturalMarkdown(t *testing.T) {
	raw := "**Room vibe:** Relaxed.\n\n**merc** — asking thoughtful questions."
	got, reply, err := (OutputFinalizer{NoReplyMarker: "NO_REPLY", MaxOutputChars: 8000}).Finalize(vibeInvocation(), raw)
	if err != nil || !reply || got != raw {
		t.Fatalf("formatting corrupted: %q %v", got, err)
	}
}

func TestVibeContextKeepsNewestBoundedSample(t *testing.T) {
	repo := &contextRepositoryStub{}
	for i := int64(1); i <= 60; i++ {
		repo.rows = append(repo.rows, repository.PublicRoomMessage{Name: "merc", Channel: "room", Message: strings.Repeat("x", 1000), CreatedOnMillis: i})
	}
	p, _ := NewRepositoryConversationContextProvider(repo, 60)
	got, err := p.Load(context.Background(), vibeInvocation())
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Rows []struct {
			CreatedOn int64  `json:"createdOn"`
			Message   string `json:"message"`
		}
	}
	if err := json.Unmarshal([]byte(got), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Rows) != 16 || envelope.Rows[0].CreatedOn != 45 || envelope.Rows[15].CreatedOn != 60 {
		t.Fatalf("wrong retained sample: %+v", envelope)
	}
	for _, row := range envelope.Rows {
		if len(row.Message) != 1000 {
			t.Fatal("message unexpectedly sliced")
		}
	}
}
