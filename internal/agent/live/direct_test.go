package live

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestDirectInvokerPreservesCommandOriginatedRegularAnswer(t *testing.T) {
	want := "Love is both an emotion and a practice.\nIt grows through attention and trust."
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(want, nil, "stop")}}
	invoker := DirectInvoker{Assembler: testLiveAssembler(t), Client: client, Finalizer: OutputFinalizer{}}

	completion, err := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller", Text: "l what is love?"}, "what is love?")

	if err != nil || completion.Text() != want {
		t.Fatalf("completion=%#v err=%v, want %q", completion, err, want)
	}
}

func TestDirectInvokerHonorsModelSilenceAfterVerifiedCommandDelivery(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("corrected", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
		llm.NewLlmResponse("NO_REPLY", nil, "stop"),
	}}
	loop, _ := testBoundedLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
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
	if len(client.requests) != 1 || !messagesContain(client.requests[0].Messages(), "direct room evidence") || strings.Contains(client.requests[0].Messages()[0].Content(), "direct room evidence") {
		t.Fatalf("direct public request did not receive context: %#v", client.requests)
	}
	if _, err := invoker.Invoke(context.Background(), &model.ChatMessage{Channel: "room", Name: "nick", Text: "current", Whisper: true}, "prompt"); err != nil {
		t.Fatal(err)
	}
	if messagesContain(client.requests[1].Messages(), "direct room evidence") {
		t.Fatalf("direct whisper leaked public context: %#v", client.requests[1].Messages())
	}
}

func messagesContain(messages []llm.LlmMessage, text string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content(), text) {
			return true
		}
	}
	return false
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

func TestDirectInvokerRetainsExecutionWhenContinuationFails(t *testing.T) {
	for _, fixture := range []struct {
		name, code string
		state      contract.EffectState
	}{
		{"committed", "", contract.EffectCommitted},
		{"unknown", "ACTION_OUTCOME_UNKNOWN", contract.EffectUnknown},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			action := &engineActionTool{errorCode: fixture.code}
			client := &scriptedToolClient{responses: []llm.LlmResponse{
				llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("performed", action.Name(), map[string]any{})}, "tool_calls"),
			}}
			assembler := testLiveAssembler(t)
			loop, err := NewRegistryToolLoop(assembler, client, []agenttool.Tool{action}, []string{action.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
			if err != nil {
				t.Fatal(err)
			}
			memory, err := turn.NewTurnMemory(turn.NewMemoryStore())
			if err != nil {
				t.Fatal(err)
			}
			invoker := DirectInvoker{Assembler: assembler, Client: client, ToolLoop: loop, Memory: &memory}
			completion, err := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller"}, "Perform the action and report.")
			if err == nil || completion.Text() != "" || action.calls.Load() != 1 {
				t.Fatalf("interrupted direct completion invented an answer or repeated an action: completion=%#v err=%v calls=%d", completion, err, action.calls.Load())
			}
			var interrupted *IncompleteTurnError
			if !errors.As(err, &interrupted) {
				t.Fatalf("direct error discarded request-local execution: %v", err)
			}
			retained, found := interrupted.Completion.Observations.Full("performed")
			if !found || retained.EffectState != fixture.state {
				t.Fatalf("direct error lost actual execution state: %#v", retained)
			}
			ctx, err := api.NewContext("room", "caller", "", "", false, []string{})
			if err != nil {
				t.Fatal(err)
			}
			loaded, loadErr := memory.LoadContext(context.Background(), ctx, "later")
			if loadErr != nil || len(loaded) != 2 || loaded[1].Role() != "user" || !strings.HasPrefix(loaded[1].Content(), turn.InterruptedToolTurnPrefix) || !strings.Contains(loaded[1].Content(), string(fixture.state)) {
				t.Fatalf("direct interrupted receipt was lost or rendered as assistant success: messages=%#v err=%v", loaded, loadErr)
			}
		})
	}
}

func TestDirectInvokerRetainsActionWhenFinalizerRejectsResponse(t *testing.T) {
	action := &engineActionTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("performed", action.Name(), map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("[Internal tool evidence from engine_action] secret", nil, "stop"),
	}}
	assembler := testLiveAssembler(t)
	loop, err := NewRegistryToolLoop(assembler, client, []agenttool.Tool{action}, []string{action.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	invoker := DirectInvoker{Assembler: assembler, Client: client, ToolLoop: loop, Finalizer: OutputFinalizer{}}
	completion, err := invoker.InvokeCompletion(context.Background(), &model.ChatMessage{Channel: "room", Name: "caller"}, "Perform the action and report.")
	var interrupted *IncompleteTurnError
	if completion.Text() != "" || !errors.As(err, &interrupted) || strings.Contains(err.Error(), "secret") || action.calls.Load() != 1 {
		t.Fatalf("finalizer lost executed action or leaked rejected response: completion=%#v err=%v calls=%d", completion, err, action.calls.Load())
	}
	retained, found := interrupted.Completion.Observations.Full("performed")
	if !found || retained.EffectState != contract.EffectCommitted {
		t.Fatalf("finalizer rejection lost committed receipt: %#v", retained)
	}
}
