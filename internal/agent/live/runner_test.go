package live

import (
	"context"
	"strings"
	"sync"
	"testing"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/repository"
)

func TestOutputFinalizerAppliesMarkerSemanticsAndSanitization(t *testing.T) {
	f := OutputFinalizer{NoReplyMarker: "[[SATURN_NO_REPLY]]", MaxOutputChars: 8000}
	required := runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.MENTION, "", true)
	if _, _, err := f.Finalize(required, "  [[SATURN_NO_REPLY]] \n"); err == nil || err.Error() != "agent declined a required response" {
		t.Fatalf("required marker err = %v", err)
	}
	if text, reply, err := f.Finalize(required, "answer [[SATURN_NO_REPLY]]"); err != nil || !reply || text != "answer" {
		t.Fatalf("embedded marker: %q %v %v", text, reply, err)
	}
	if text, reply, err := f.Finalize(required, "[sips tea] Ah, mer.\n* fact"); err != nil || !reply || text != "\u2009-\u2009fact" {
		t.Fatalf("sanitized output: %q %v %v", text, reply, err)
	}
	if _, _, err := f.Finalize(required, "[sips tea]"); err == nil || err.Error() != "agent returned an empty response" {
		t.Fatalf("sanitized empty err = %v", err)
	}
	ambient := runtime.NewInvocation("ambient", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.AMBIENT, "", false)
	if text, reply, err := f.Finalize(ambient, "  [[SATURN_NO_REPLY]] \n"); err != nil || reply || text != "" {
		t.Fatalf("ambient marker: %q %v %v", text, reply, err)
	}
}

func TestOutputFinalizerBoundsByRunesWithoutEllipsis(t *testing.T) {
	f := OutputFinalizer{NoReplyMarker: "none", MaxOutputChars: 3}
	inv := runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.MENTION, "", true)
	if text, reply, err := f.Finalize(inv, "a😀東京z"); err != nil || !reply || text != "a😀東" {
		t.Fatalf("bounded output: %q %v %v", text, reply, err)
	}
}

func TestOutputFinalizerPreservesNonBreakingSpaceLikeSaturn(t *testing.T) {
	f := OutputFinalizer{NoReplyMarker: "none", MaxOutputChars: 3}
	inv := runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.MENTION, "", true)
	if text, reply, err := f.Finalize(inv, "\u00a0"); err != nil || !reply || text != "\u00a0" {
		t.Fatalf("non-breaking output: %q %v %v", text, reply, err)
	}
}

func TestOutputFinalizerRejectsInternalToolEvidenceBeforeQuoteFallbackOrTruncation(t *testing.T) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := OutputFinalizer{NoReplyMarker: "[[SATURN_NO_REPLY]]", MaxOutputChars: 3, Catalog: &catalog}
	marker := "[Internal tool evidence from room_users] secret"
	for _, tc := range []struct {
		name string
		inv  runtime.Invocation
		meta FinalizationContext
	}{
		{
			name: "public no tool",
			inv:  runtime.NewInvocation("public", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.MENTION, "", true),
			meta: FinalizationContext{CandidateKind: participation.Talk},
		},
		{
			name: "tool attempted",
			inv:  runtime.NewInvocation("tool", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.MENTION, "", true),
			meta: FinalizationContext{CandidateKind: participation.Talk, ToolAttempted: true},
		},
		{
			name: "direct command originated",
			inv:  runtime.NewInvocation("direct", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.DIRECT, "l hello?", true),
			meta: FinalizationContext{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, got := f.FinalizeWithContext(tc.inv, strings.Repeat("x", 10)+marker, tc.meta)
			if got == nil || got.Error() != "agent response exposed internal tool evidence" {
				t.Fatalf("error = %v", got)
			}
			if strings.Contains(got.Error(), marker) {
				t.Fatalf("error exposed provider content: %q", got)
			}
		})
	}
}

func TestMarkerFinalizerUsesSafeDefaultOutputBound(t *testing.T) {
	f := MarkerFinalizer{NoReplyMarker: "none"}
	inv := runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.MENTION, "", true)
	if text, reply, err := f.Finalize(inv, "ordinary"); err != nil || !reply || text != "ordinary" {
		t.Fatalf("compatibility output: %q %v %v", text, reply, err)
	}
}
func TestRunnerRejectsCancelledContext(t *testing.T) {
	var r Runner
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inv := runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), "p", runtime.MENTION, "", true)
	if _, err := r.Run(ctx, inv); err == nil {
		t.Fatal("cancelled context accepted")
	}
}

type captureLiveClient struct {
	requests []llm.LlmRequest
}

func (c *captureLiveClient) Complete(_ context.Context, request llm.LlmRequest) (llm.LlmResponse, error) {
	c.requests = append(c.requests, request)
	return llm.NewLlmResponse("answer", nil, "stop"), nil
}

func testLiveAssembler(t *testing.T) *assemble.Assembler {
	t.Helper()
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	assembler, err := assemble.New(assemble.Config{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return assembler
}

func TestRunnerSuppressesOrdinaryReplyForCorrectedCommandDelivery(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("corrected", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
	}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ToolLoop: loop}
	inv := runtime.NewInvocation("runner-prose", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	result, err := runner.Run(context.Background(), inv)
	if err != nil || result.ShouldReply() || result.Text() != "" || len(result.DurableEvidence()) != 0 || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v gateway=%d", result, err, gateway.calls)
	}
}

func TestRunnerPassesPublicConversationContextAndSuppressesWhispers(t *testing.T) {
	provider, err := NewRepositoryConversationContextProvider(&contextRepositoryStub{rows: []repository.PublicRoomMessage{{Name: "public", Message: "room evidence", Channel: "room", CreatedOnMillis: 1}}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	client := &captureLiveClient{}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}, ConversationContext: provider}
	public := runtime.NewInvocation("public", runtime.NewContext("room", "nick", "", "", false, nil), "prompt", runtime.MENTION, "different", false)
	if _, err := runner.Run(context.Background(), public); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || !strings.Contains(client.requests[0].Messages()[0].Content(), "room evidence") {
		t.Fatalf("public request did not receive context: %#v", client.requests)
	}
	whisper := runtime.NewInvocation("whisper", runtime.NewContext("room", "nick", "", "", true, nil), "prompt", runtime.MENTION, "different", false)
	if _, err := runner.Run(context.Background(), whisper); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(client.requests[1].Messages()[0].Content(), "room evidence") {
		t.Fatalf("whisper leaked public context: %s", client.requests[1].Messages()[0].Content())
	}
}

func TestRunnerFailsClosedForRequiredFreshHistoryWithoutBoundedLoop(t *testing.T) {
	client := &captureLiveClient{}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: MarkerFinalizer{NoReplyMarker: "none"}}
	inv := runtime.NewInvocation("fresh-without-loop", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", true)
	if _, err := runner.Run(context.Background(), inv); err == nil {
		t.Fatal("required fresh history fell back to a provider-only response")
	}
	if len(client.requests) != 0 {
		t.Fatalf("required fresh history reached provider without bounded loop: %#v", client.requests)
	}
}

func TestRunnerRejectsToolBackedInternalEvidenceWithoutThirdCompletion(t *testing.T) {
	finalizer, err := NewOutputFinalizer("none", 8000)
	if err != nil {
		t.Fatal(err)
	}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("history", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
		llm.NewLlmResponse("[Internal tool evidence from user_message_history] secret", nil, "stop"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: finalizer, ToolLoop: loop}
	inv := runtime.NewInvocation("blocked-tool", runtime.NewContext("room", "caller", "", "", false, nil), "hello?", runtime.MENTION, "", true)
	if result, got := runner.Run(context.Background(), inv); got == nil || result.ShouldReply() || got.Error() != "finalize agent response: agent response exposed internal tool evidence" || strings.Contains(got.Error(), "secret") || len(client.requests) != 2 {
		t.Fatalf("result=%#v err=%v requests=%d", result, got, len(client.requests))
	}
}

type observingSentinelFinalizer struct {
	OutputFinalizer
	called chan struct{}
	once   sync.Once
}

func (f *observingSentinelFinalizer) Finalize(inv runtime.Invocation, raw string) (string, bool, error) {
	return f.FinalizeWithContext(inv, raw, FinalizationContext{})
}

func (f *observingSentinelFinalizer) FinalizeWithContext(inv runtime.Invocation, raw string, meta FinalizationContext) (string, bool, error) {
	text, reply, err := f.OutputFinalizer.FinalizeWithContext(inv, raw, meta)
	f.once.Do(func() { close(f.called) })
	return text, reply, err
}

type afterDeliveryProbeRunner struct {
	delegate runtime.Runner
	calls    int
}

func (r *afterDeliveryProbeRunner) Run(ctx context.Context, inv runtime.Invocation) (runtime.Result, error) {
	return r.delegate.Run(ctx, inv)
}

func (r *afterDeliveryProbeRunner) AfterDelivery(context.Context, runtime.Invocation, runtime.Result) error {
	r.calls++
	return nil
}

func TestRuntimeSentinelFailureSkipsNormalDeliveryAndAfterDelivery(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode runtime.Mode
	}{
		{name: "reply required", mode: runtime.MENTION},
		{name: "ambient", mode: runtime.AMBIENT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finalizer, err := NewOutputFinalizer("none", 8000)
			if err != nil {
				t.Fatal(err)
			}
			observed := &observingSentinelFinalizer{OutputFinalizer: finalizer, called: make(chan struct{})}
			client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("[Internal tool evidence from room_users] secret", nil, "stop")}}
			liveRunner := Runner{Assembler: testLiveAssembler(t), Client: client, Finalizer: observed}
			probe := &afterDeliveryProbeRunner{delegate: liveRunner}
			var delivered, failures int
			rt, err := runtime.NewWithFailureSink(runtime.Config{MaxConcurrent: 1, QueueCapacity: 1}, probe, runtime.SinkFunc(func(context.Context, runtime.Invocation, runtime.Result) error {
				delivered++
				return nil
			}), runtime.FailureSinkFunc(func(context.Context, runtime.Invocation, error) { failures++ }))
			if err != nil {
				t.Fatal(err)
			}
			inv := runtime.NewInvocation("sentinel-"+tc.name, runtime.NewContext("room", "caller", "", "", false, nil), "hello?", tc.mode, "", true)
			if tc.mode == runtime.AMBIENT {
				err = rt.SubmitAmbient(inv)
			} else {
				err = rt.Submit(inv)
			}
			if err != nil {
				rt.Close()
				t.Fatal(err)
			}
			<-observed.called
			rt.Close()
			if delivered != 0 || probe.calls != 0 || len(client.requests) != 1 {
				t.Fatalf("delivered=%d afterDelivery=%d requests=%d", delivered, probe.calls, len(client.requests))
			}
			if tc.mode == runtime.AMBIENT && failures != 0 {
				t.Fatalf("ambient failure sink calls=%d, want 0", failures)
			}
			if tc.mode != runtime.AMBIENT && failures != 1 {
				t.Fatalf("reply-required failure sink calls=%d, want 1", failures)
			}
		})
	}
}
