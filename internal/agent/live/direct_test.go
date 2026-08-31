package live

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/turn"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestDirectInvokerKeepsOrdinaryCommandResponseOutsideQuoteOnlyPolicy(t *testing.T) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("ordinary command answer", nil, "stop")}}
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client, Finalizer: OutputFinalizer{Catalog: &catalog}}
	completion, err := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller", Text: "l hello?"}, "hello?")
	if err != nil || completion.Text() != "ordinary command answer" {
		t.Fatalf("completion=%#v err=%v", completion, err)
	}
}

func TestDirectInvokerSuppressesOrdinaryReplyForCorrectedCommandDelivery(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("corrected", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
	}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ToolLoop: loop}
	completion, err := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller", Text: "l weather?"}, "weather?")
	if err != nil || completion.Text() != "" || len(completion.DurableEvidence()) != 0 || gateway.calls != 1 {
		t.Fatalf("completion=%#v err=%v gateway=%d", completion, err, gateway.calls)
	}
}

func TestDirectInvokerPassesPublicConversationContextAndSuppressesWhispers(t *testing.T) {
	provider, err := NewRepositoryConversationContextProvider(&contextRepositoryStub{rows: []repository.PublicRoomMessage{{Name: "public", Message: "direct room evidence", Channel: "room", CreatedOnMillis: 1}}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	client := &captureLiveClient{}
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client, ConversationContext: provider}
	if _, err := invoker.Invoke(context.Background(), &model.ChatMessage{Channel: "room", Name: "nick", Text: "current"}, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || !strings.Contains(client.requests[0].Messages()[0].Content(), "direct room evidence") {
		t.Fatalf("direct public request did not receive context: %#v", client.requests)
	}
	if _, err := invoker.Invoke(context.Background(), &model.ChatMessage{Channel: "room", Name: "nick", Text: "current", Whisper: true}, "prompt"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(client.requests[1].Messages()[0].Content(), "direct room evidence") {
		t.Fatalf("direct whisper leaked public context: %s", client.requests[1].Messages()[0].Content())
	}
}

func TestDirectInvokerPersistDeliveryAppendsCandidateOnlyForPublicVisibleArtifact(t *testing.T) {
	store := turn.NewMemoryStore()
	memory, err := turn.NewTurnMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	invoker := DirectInvoker{Memory: &memory}
	message := &model.ChatMessage{Channel: "room", Name: "nick", Text: "!l prompt"}
	completion := runtime.NewDirectCompletion("visible", []turn.PersistableEvidence{{Tool: "room_users", Content: `{"room":"room","users":[],"count":0,"returnedCount":0,"truncated":false}`}})
	if err := invoker.PersistDelivery(context.Background(), message, "prompt", completion); err != nil {
		t.Fatal(err)
	}
	ctx, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if got := store.EvidenceFor(ctx); len(got) != 1 || got[0].Tool != "room_users" {
		t.Fatalf("persisted evidence = %#v", got)
	}
	whisper := &model.ChatMessage{Channel: "room", Name: "nick", Text: "!l prompt", Whisper: true}
	if err := invoker.PersistDelivery(context.Background(), whisper, "prompt", completion); err != nil {
		t.Fatal(err)
	}
	whisperCtx, _ := api.NewContext("room", "nick", "", "", true, []string{})
	if got := store.EvidenceFor(whisperCtx); len(got) != 0 {
		t.Fatalf("whisper evidence = %#v", got)
	}
}

func TestDirectInvokerFailsClosedForRequiredFreshHistoryWithoutBoundedLoop(t *testing.T) {
	client := &captureLiveClient{}
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client}
	message := &model.ChatMessage{Channel: "room", Name: "caller", Text: "l tell me about alice"}
	if _, err := invoker.Invoke(context.Background(), message, "tell me about alice"); err == nil {
		t.Fatal("required fresh history fell back to a provider-only direct response")
	}
	if len(client.requests) != 0 {
		t.Fatalf("required fresh history reached provider without bounded loop: %#v", client.requests)
	}
}

func TestDirectInvokerRejectsInternalEvidenceWithoutDeliveryArtifact(t *testing.T) {
	finalizer, err := NewOutputFinalizer("none", 8000)
	if err != nil {
		t.Fatal(err)
	}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("[Internal tool evidence from room_users] secret", nil, "stop")}}
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client, Finalizer: finalizer}
	completion, got := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller", Text: "l hello?"}, "hello?")
	if got == nil || completion.Text() != "" || got.Error() != "agent response exposed internal tool evidence" || strings.Contains(got.Error(), "secret") || len(client.requests) != 1 {
		t.Fatalf("completion=%#v err=%v requests=%d", completion, got, len(client.requests))
	}
}
