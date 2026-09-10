package live

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
	commandcatalog "zenbot/internal/command/catalog"
)

type parallelReadTool struct {
	name    string
	started *atomic.Int32
	ready   chan struct{}
}

type correctingReadTool struct{ calls atomic.Int32 }

type failingReadTool struct{ calls atomic.Int32 }

type invalidDescriptorTool struct{}

type roomDeliveryTool struct {
	name   string
	result contract.Result
}

type typedCommandGateway struct {
	calls     int
	command   string
	arguments string
}

func (gateway *typedCommandGateway) Execute(_ context.Context, _ api.Context, command, arguments string) (commandgateway.Execution, error) {
	gateway.calls++
	gateway.command = command
	gateway.arguments = arguments
	return commandgateway.Execution{Executed: true, Messages: []string{"delivered"}}, nil
}

func (t roomDeliveryTool) Name() string { return t.name }
func (t roomDeliveryTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.name, t.name, "Deliver one command result directly to the room.", "test", contract.AccessUser, contract.Action, contract.RoomDelivery, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), nil, nil, false, time.Second, json.RawMessage(`{"type":"object"}`), nil, []string{"room_delivery"}, []string{"Do not use for model-only data."})
}
func (t roomDeliveryTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	return t.result, nil
}

func (invalidDescriptorTool) Name() string { return "invalid_descriptor" }
func (invalidDescriptorTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.Descriptor{}, errors.New("broken descriptor")
}
func (invalidDescriptorTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	return contract.SuccessResult("", "invalid_descriptor", map[string]any{}), nil
}

func (t *correctingReadTool) Name() string { return "correcting_read" }
func (t *correctingReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Correcting read", "Read one validated value after correcting invalid arguments.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","minLength":1}},"required":["value"]}`), nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not call with a blank value."})
}
func (t *correctingReadTool) Execute(_ context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	return contract.SuccessResult("", t.Name(), map[string]any{"value": "observed"}), nil
}

func (t *failingReadTool) Name() string { return "failing_read" }
func (t *failingReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Failing read", "Fail predictably for request-local failure-budget verification.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}},"required":["value"]}`), nil, nil, true, 0, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not use outside tests."})
}
func (t *failingReadTool) Execute(_ context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	return contract.Result{}, errors.New("expected tool failure")
}

func (t parallelReadTool) Name() string { return t.name }
func (t parallelReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.name, t.name, "Independent read for parallel registry-loop verification.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), nil, nil, true, 250*time.Millisecond, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not use for writes."})
}
func (t parallelReadTool) Execute(ctx context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	if t.started.Add(1) == 2 {
		close(t.ready)
	}
	select {
	case <-t.ready:
		return contract.SuccessResult("", t.name, map[string]any{"value": "ok"}), nil
	case <-ctx.Done():
		return contract.Result{}, ctx.Err()
	}
}

func TestRegistryToolLoopRecordsReadEvidenceAndAttempt(t *testing.T) {
	directory := &loopRoomDirectory{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("room-call", roomUsersTool, map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("answer from evidence", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{agenttool.RoomUsers{Directory: directory}}, []string{roomUsersTool}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("registry-evidence", runtime.NewContext("room", "caller", "", "", false, nil), "hello there", runtime.AMBIENT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "answer from evidence" || !completion.ToolAttempted || directory.calls != 1 || len(completion.Evidence()) != 1 || completion.Evidence()[0].Tool != roomUsersTool || len(client.requests) != 2 || client.requests[1].Messages()[len(client.requests[1].Messages())-2].ToolCalls()[0].ID() != "room-call" {
		t.Fatalf("completion=%#v roomCalls=%d", completion, directory.calls)
	}
}

func TestRegistryToolLoopAcceptsPlainFinalAnswerWithAutoToolChoice(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("ordinary answer", nil, "stop"),
		llm.NewLlmResponse("unexpected retry", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("structured-answer", runtime.NewContext("room", "caller", "", "", false, nil), "what is love?", runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "ordinary answer" || read.calls.Load() != 0 || len(client.requests) != 1 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if client.requests[0].ToolChoice() != llm.ToolChoiceAuto || providerRequestHasTool(client.requests[0], "respond_to_user") {
		t.Fatalf("initial request did not use the standard auto tool protocol: %#v", client.requests[0])
	}
}

func TestRegistryToolLoopUsesSemanticGateBeforeAcceptingFinalAnswer(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("I will fetch the current value now.", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("gate-1", completionAssessmentTool, map[string]any{
			"decision": "CONTINUE",
			"feedback": "Call correcting_read and use its observation.",
		})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("read-1", read.Name(), map[string]any{"value": "current"})}, "tool_calls"),
		llm.NewLlmResponse("The observed value is current.", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("gate-2", completionAssessmentTool, map[string]any{
			"decision": "FINAL",
			"feedback": "The answer reports the successful current observation.",
		})}, "tool_calls"),
	}}
	gate, err := NewSemanticCompletionGate(client, "Judge whether the answer satisfies the newest request and uses required tool observations.")
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, gate, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 1, MaxCallsPerTool: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("semantic-gate", runtime.NewContext("room", "caller", "", "", false, nil), "get the current value", runtime.DIRECT, "", false)

	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "The observed value is current." || read.calls.Load() != 1 || len(client.requests) != 5 {
		t.Fatalf("completion=%#v calls=%d requests=%d", completion, read.calls.Load(), len(client.requests))
	}
	if client.requests[1].ToolChoice() != llm.ToolChoiceRequired || client.requests[4].ToolChoice() != llm.ToolChoiceRequired {
		t.Fatalf("semantic decisions were not constrained: first=%q final=%q", client.requests[1].ToolChoice(), client.requests[4].ToolChoice())
	}
	if !strings.Contains(client.requests[4].Messages()[1].Content(), `"toolObservations":[{"tool":"correcting_read","status":"success"`) {
		t.Fatalf("final gate did not receive tool observation: %#v", client.requests[4].Messages())
	}
	if strings.Contains(client.requests[4].Messages()[1].Content(), "SEMANTIC_COMPLETION_FEEDBACK") {
		t.Fatalf("internal evaluator feedback leaked into conversational context: %#v", client.requests[4].Messages())
	}
}

func TestRegistryToolLoopExecutesDirectToolCallWithAutoChoice(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("read", read.Name(), map[string]any{"value": "requested"})}, "tool_calls"),
		llm.NewLlmResponse("observed value", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("narrated-action", runtime.NewContext("room", "caller", "", "", false, nil), "please perform the available read", runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "observed value" || read.calls.Load() != 1 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if client.requests[0].ToolChoice() != llm.ToolChoiceAuto || client.requests[1].ToolChoice() != llm.ToolChoiceAuto {
		t.Fatalf("tool choices=%q/%q", client.requests[0].ToolChoice(), client.requests[1].ToolChoice())
	}
}

func TestRegistryToolLoopRetriesTruncatedInitialResponseWithCompactContext(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("unfinished planning prose", nil, "length"),
		llm.NewLlmResponse("recovered answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	memory := []llm.LlmMessage{
		llm.NewLlmMessage("user", "old question", nil, ""),
		llm.NewLlmMessage("assistant", "old answer", nil, ""),
		llm.NewLlmMessage("user", "relevant question", nil, ""),
		llm.NewLlmMessage("assistant", "relevant answer", nil, ""),
		llm.NewLlmMessage("user", "previous request", nil, ""),
		llm.NewLlmMessage("assistant", "previous assistant offer", nil, ""),
	}
	inv := runtime.NewInvocation("truncated-decision", runtime.NewContext("room", "caller", "", "", false, nil), "who are you?", runtime.DIRECT, "", false)
	historical := []turn.HistoricalEvidence{{Tool: roomUsersTool, Content: `{"count":1}`, ObservedAtMillis: 1}}
	completion, err := loop.CompleteWithEvidenceAndHistorical(context.Background(), inv, memory, strings.Repeat("room noise ", 500), historical)
	if err != nil || completion.Response.Content() != "recovered answer" || read.calls.Load() != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	initialMessages := client.requests[0].Messages()
	retryMessages := client.requests[1].Messages()
	if len(retryMessages) >= len(initialMessages) || client.requests[1].ToolChoice() != llm.ToolChoiceAuto {
		t.Fatalf("retry was not compact and automatic: initial=%d retry=%d choice=%q", len(initialMessages), len(retryMessages), client.requests[1].ToolChoice())
	}
	retryText := ""
	for _, message := range retryMessages {
		retryText += message.Content() + "\n"
	}
	for _, forbidden := range []string{"unfinished planning prose", "RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA", "HISTORICAL_TOOL_EVIDENCE_UNTRUSTED_DATA", "room noise"} {
		if strings.Contains(retryText, forbidden) {
			t.Fatalf("compact retry retained %q: %s", forbidden, retryText)
		}
	}
	for _, required := range []string{"previous assistant offer", "who are you?"} {
		if !strings.Contains(retryText, required) {
			t.Fatalf("compact retry dropped %q: %s", required, retryText)
		}
	}
}

func TestRegistryToolLoopFailsAfterOneCompactTruncationRetry(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("unfinished", nil, "length"),
		llm.NewLlmResponse("still unfinished", nil, "length"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("truncated-twice", runtime.NewContext("room", "caller", "", "", false, nil), "who are you?", runtime.DIRECT, "", false)
	_, err = loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err == nil || !strings.Contains(err.Error(), "remained truncated after compact retry") || read.calls.Load() != 0 || len(client.requests) != 2 {
		t.Fatalf("error=%v calls=%d requests=%d", err, read.calls.Load(), len(client.requests))
	}
}

func providerRequestHasTool(request llm.LlmRequest, name string) bool {
	for _, raw := range request.Tools() {
		definition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := definition["function"].(map[string]any)
		if ok && function["name"] == name {
			return true
		}
	}
	return false
}

func TestProviderToolDefinitionsFilterModeratorCommandsWithoutRepeatedAuthorizationProse(t *testing.T) {
	moderator, err := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}
	runCommand := agenttool.RunCommand{}
	registry := agenttool.NewRegistry([]agenttool.Tool{runCommand}, []string{runCommand.Name()})
	definitions, err := providerToolDefinitions(registry, moderator)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("definitions=%d, want 1", len(definitions))
	}
	function := definitions[0].(map[string]any)["function"].(map[string]any)
	description, _ := function["description"].(string)
	if strings.Contains(description, "Availability: authorized for the current caller.") {
		t.Fatalf("description repeats registry authorization state: %q", description)
	}
	parameters := function["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	command := properties["command"].(map[string]any)
	values := command["enum"].([]any)
	foundKick := false
	for _, value := range values {
		foundKick = foundKick || value == "kick"
	}
	if !foundKick {
		t.Fatalf("moderator run_command schema does not expose kick: %#v", values)
	}
}

func TestRegistryToolLoopSuppressesFinalReplyAfterSuccessfulRoomDelivery(t *testing.T) {
	delivery := roomDeliveryTool{name: "delivered_command", result: contract.SuccessResult("", "delivered_command", map[string]any{"messages": []string{"already visible"}, "deliveredCount": 1})}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("delivery-call", delivery.Name(), map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("already visible", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{delivery}, []string{delivery.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("registry-delivery", runtime.NewContext("room", "caller", "", "", false, nil), "run it", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.SuppressReply || len(client.requests) != 2 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
}

func TestRegistryToolLoopKeepsSynthesisForMixedOrFailedDeliveryResults(t *testing.T) {
	tests := []struct {
		name  string
		tools []agenttool.Tool
		calls []llm.LlmToolCall
	}{
		{
			name: "mixed model data",
			tools: []agenttool.Tool{
				roomDeliveryTool{name: "delivered_command", result: contract.SuccessResult("", "delivered_command", map[string]any{"deliveredCount": 1})},
				&correctingReadTool{},
			},
			calls: []llm.LlmToolCall{
				llm.NewLlmToolCall("delivery-call", "delivered_command", map[string]any{}),
				llm.NewLlmToolCall("read-call", "correcting_read", map[string]any{"value": "yes"}),
			},
		},
		{
			name: "failed delivery",
			tools: []agenttool.Tool{
				roomDeliveryTool{name: "delivered_command", result: contract.ErrorResult("", "delivered_command", "COMMAND_REJECTED", "not delivered")},
			},
			calls: []llm.LlmToolCall{llm.NewLlmToolCall("delivery-call", "delivered_command", map[string]any{})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed := make([]string, 0, len(tc.tools))
			for _, registered := range tc.tools {
				allowed = append(allowed, registered.Name())
			}
			client := &scriptedToolClient{responses: []llm.LlmResponse{
				llm.NewLlmResponse(nil, tc.calls, "tool_calls"),
				llm.NewLlmResponse("synthesis required", nil, "stop"),
			}}
			loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, tc.tools, allowed, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
			if err != nil {
				t.Fatal(err)
			}
			inv := runtime.NewInvocation("registry-delivery-control", runtime.NewContext("room", "caller", "", "", false, nil), "run it", runtime.MENTION, "", false)
			completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
			if err != nil || completion.SuppressReply || completion.Response.Content() != "synthesis required" {
				t.Fatalf("completion=%#v err=%v", completion, err)
			}
		})
	}
}

func TestRegistryToolLoopRejectsInvalidRestrictedDescriptorAtComposition(t *testing.T) {
	_, err := NewRegistryToolLoop(testLiveAssembler(t), &scriptedToolClient{}, acceptingCompletionGate{}, []agenttool.Tool{invalidDescriptorTool{}}, []string{"invalid_descriptor"}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("error=%v", err)
	}
}

func TestProviderToolDefinitionsFailsClosedOnDescriptorError(t *testing.T) {
	registry := agenttool.NewRegistry([]agenttool.Tool{invalidDescriptorTool{}}, []string{"invalid_descriptor"})
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	definitions, err := providerToolDefinitions(registry, caller)
	if err == nil || !strings.Contains(err.Error(), "broken descriptor") || len(definitions) != 0 {
		t.Fatalf("definitions=%#v error=%v", definitions, err)
	}
}

func TestCreatorProviderManifestFitsConfiguredContextBudget(t *testing.T) {
	tools := []agenttool.Tool{
		agenttool.UserMessageHistory{},
		agenttool.RoomUsers{},
		agenttool.DatabaseQuery{},
		agenttool.DatabaseSchema{Enabled: true},
		agenttool.DatabaseSQL{},
	}
	allowed := []string{"user_message_history", "room_users", "database_query", "database_schema", "database_sql"}
	for _, definition := range commandcatalog.AgentEntries() {
		command := agenttool.SaturnCommand{Definition: definition}
		tools = append(tools, command)
		allowed = append(allowed, command.Name())
	}
	registry := agenttool.NewRegistry(tools, allowed)
	creator, _ := api.NewContextWithCapabilities(
		"programming", "creator", "creator-trip", "hash", false, []string{},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands, api.DynamicSQL},
	)
	definitions, err := providerToolDefinitions(registry, creator)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 32*1024 {
		t.Fatalf("creator provider manifest = %d bytes, want at most %d", len(encoded), 32*1024)
	}
	if strings.Contains(string(encoded), `"strict"`) {
		t.Fatal("provider manifest unexpectedly enabled strict mode")
	}
}

func TestExecuteRegistryBatchFansOutIndependentReadOnlyCalls(t *testing.T) {
	var started atomic.Int32
	ready := make(chan struct{})
	tools := []agenttool.Tool{
		parallelReadTool{name: "read_one", started: &started, ready: ready},
		parallelReadTool{name: "read_two", started: &started, ready: ready},
	}
	registry := agenttool.NewRegistry(tools, []string{"read_one", "read_two"})
	executor := &execution.Executor{Registry: registry, Ledger: execution.NewLedger(map[string]int{"read_one": 1, "read_two": 1}, 2)}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	_, batch, err := executeRegistryBatch(context.Background(), executor, agent, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}, state, []execution.Call{
		{ID: "one", Name: "read_one", Arguments: json.RawMessage(`{}`)},
		{ID: "two", Name: "read_two", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil || len(batch) != 2 || batch[0].Result.IsError || batch[1].Result.IsError || started.Load() != 2 {
		t.Fatalf("batch=%#v started=%d err=%v", batch, started.Load(), err)
	}
}

func TestExecuteRegistryBatchRejectsDuplicateCallIDsBeforeExecution(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	executor := &execution.Executor{Registry: registry}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	_, _, err := executeRegistryBatch(context.Background(), executor, agent, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}, state, []execution.Call{
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"one"}`)},
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"two"}`)},
	})
	if err == nil {
		t.Fatal("duplicate call IDs were accepted")
	}
	if read.calls.Load() != 0 {
		t.Fatalf("tool executed %d times before identity validation", read.calls.Load())
	}
}

func TestRegistryToolLoopKeepsToolsAvailableForArgumentSelfCorrection(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("bad", read.Name(), map[string]any{"value": 7})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fixed", read.Name(), map[string]any{"value": "yes"})}, "tool_calls"),
		llm.NewLlmResponse("corrected answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("self-correction", runtime.NewContext("room", "caller", "", "", false, nil), "read it", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "corrected answer" || read.calls.Load() != 1 || len(client.requests) != 3 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if len(client.requests[1].Tools()) != 1 || len(client.requests[2].Tools()) != 1 {
		t.Fatalf("tool manifest disappeared during correction: second=%d third=%d", len(client.requests[1].Tools()), len(client.requests[2].Tools()))
	}
	secondMessages := client.requests[1].Messages()
	if secondMessages[len(secondMessages)-1].Role() != "tool" || !strings.Contains(secondMessages[len(secondMessages)-1].Content(), "INVALID_ARGUMENTS") {
		t.Fatalf("invalid-argument observation missing: %#v", secondMessages)
	}
}

func TestRegistryToolLoopCorrectsTypedSaturnArgumentsWithoutInventingCommandText(t *testing.T) {
	definition, ok := commandcatalog.AgentEntry("list")
	if !ok {
		t.Fatal("list command is not agent actionable")
	}
	gateway := &typedCommandGateway{}
	command := agenttool.SaturnCommand{Definition: definition, Gateway: gateway}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("bad", command.Name(), map[string]any{"arguments": "lounge"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fixed", command.Name(), map[string]any{"room": "lounge"})}, "tool_calls"),
		llm.NewLlmResponse("completed", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{command}, []string{command.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2, MaxCallsPerTool: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("typed-correction", runtime.NewContext("room", "moderator", "trip", "", false, nil), "list users in lounge", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil || completion.Response.Content() != "completed" || gateway.calls != 1 || gateway.command != "list" || gateway.arguments != "lounge" {
		t.Fatalf("completion=%#v gateway=%#v err=%v", completion, gateway, err)
	}
	if len(client.requests) != 3 || len(client.requests[1].Tools()) != 1 || len(client.requests[2].Tools()) != 1 {
		t.Fatalf("typed manifest was not retained across correction: requests=%d", len(client.requests))
	}
	observation := client.requests[1].Messages()[len(client.requests[1].Messages())-1]
	if observation.Role() != "tool" || !strings.Contains(observation.Content(), "INVALID_ARGUMENTS") {
		t.Fatalf("correctable observation missing: %#v", observation)
	}
}

func TestRegistryToolLoopHonorsPerToolCallBudget(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("bounded answer", nil, "stop"),
	}}
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 3, MaxCallsPerTool: 1}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, limits)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("per-tool-budget", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err != nil {
		t.Fatal(err)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	messages := client.requests[2].Messages()
	if !strings.Contains(messages[len(messages)-1].Content(), "TOOL_CALL_LIMIT_REACHED") {
		t.Fatalf("limit observation missing: %#v", messages[len(messages)-1])
	}
}

func TestRegistryToolLoopHonorsConfiguredFailureBudget(t *testing.T) {
	read := &failingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("degraded answer", nil, "stop"),
	}}
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 3, MaxToolFailures: 1}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, limits)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("failure-budget", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err != nil {
		t.Fatal(err)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	messages := client.requests[2].Messages()
	if !strings.Contains(messages[len(messages)-1].Content(), "TOOL_DISABLED") {
		t.Fatalf("disabled observation missing: %#v", messages[len(messages)-1])
	}
	if len(client.requests[2].Tools()) != 0 {
		t.Fatalf("disabled tool remained available during degradation: %#v", client.requests[2].Tools())
	}
}

func TestRegistryToolLoopReservesTerminalSynthesisAfterLastToolRound(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("answer from both observations", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 2, MaxCallsPerTool: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("terminal-reserve", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatalf("last-round observations were discarded: %v", err)
	}
	if completion.Response.Content() != "answer from both observations" || read.calls.Load() != 2 || len(client.requests) != 3 {
		t.Fatalf("completion=%#v tool_calls=%d provider_calls=%d", completion, read.calls.Load(), len(client.requests))
	}
	if len(client.requests[2].Tools()) != 0 {
		t.Fatalf("terminal synthesis retained tools: %#v", client.requests[2].Tools())
	}
}

func TestRegistryToolLoopReflectsInvalidPlainFinalResponseWithoutTools(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(" ", nil, "stop"),
		llm.NewLlmResponse("recovered final answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, acceptingCompletionGate{}, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("final-reflection", runtime.NewContext("room", "caller", "", "", false, nil), "answer me", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "recovered final answer" || len(client.requests) != 2 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
	if len(client.requests[1].Tools()) != 0 || client.requests[1].ToolChoice() != llm.ToolChoiceAuto {
		t.Fatalf("final reflection retained execution tools: %#v", client.requests[1])
	}
}
