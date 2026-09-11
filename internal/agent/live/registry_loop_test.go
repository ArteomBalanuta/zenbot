package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
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

type routeCommandGateway struct {
	commands []string
}

type tinyBudgetCatalog struct{}

func (tinyBudgetCatalog) Text(string) (string, error) { return "policy", nil }
func (tinyBudgetCatalog) Formatted(path string, values ...any) (string, error) {
	if path == "input/router-contextualized-prompt.txt" && len(values) == 4 {
		return fmt.Sprint(values[3]), nil
	}
	return "policy", nil
}

type largeBudgetTool struct{ name string }

func (tool largeBudgetTool) Name() string { return tool.name }
func (tool largeBudgetTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(
		tool.name, tool.name, "Return a large bounded test observation.", "test",
		contract.AccessUser, contract.ReadOnly, contract.ModelData,
		json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`),
		[]string{tool.name}, nil, []string{"Do not use outside context-budget tests."},
		contract.WithMaxModelResultBytes(700),
	)
}
func (tool largeBudgetTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	rows := make([]map[string]any, 24)
	for index := range rows {
		rows[index] = map[string]any{"index": index, "value": strings.Repeat(tool.name, 12)}
	}
	return contract.SuccessResult("", tool.name, map[string]any{"rows": rows}), nil
}

func (gateway *typedCommandGateway) Execute(_ context.Context, _ api.Context, command, arguments string) (commandgateway.Execution, error) {
	gateway.calls++
	gateway.command = command
	gateway.arguments = arguments
	return verifiedGatewayExecution("delivered"), nil
}

func (gateway *routeCommandGateway) Execute(_ context.Context, _ api.Context, command, arguments string) (commandgateway.Execution, error) {
	gateway.commands = append(gateway.commands, strings.TrimSpace(command+" "+arguments))
	return verifiedGatewayExecution(command + " result"), nil
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
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.RoomUsers{Directory: directory}}, []string{roomUsersTool}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("registry-evidence", runtime.NewContext("room", "caller", "", "", false, nil), "hello there", runtime.AMBIENT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "answer from evidence" || !completion.ToolAttempted || directory.calls != 1 || len(completion.Evidence()) != 1 || completion.Evidence()[0].Tool != roomUsersTool || len(client.requests) != 2 || !requestContainsToolCallID(client.requests[1], "room-call") {
		t.Fatalf("completion=%#v roomCalls=%d", completion, directory.calls)
	}
}

func TestRegistryToolLoopKeepsNewestRequestWithoutPlannerTaskState(t *testing.T) {
	const objective = "answer the newest request directly"
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("direct answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(
		testLiveAssembler(t), client,
		[]agenttool.Tool{read}, []string{read.Name()},
		turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("no-planner-state", runtime.NewContext("room", "caller", "", "", false, nil), objective, runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "direct answer" || len(client.requests) != 1 {
		t.Fatalf("completion=%#v requests=%d", completion, len(client.requests))
	}
	if !messagesContain(client.requests[0].Messages(), objective) {
		t.Fatalf("execution request lost newest request: %#v", client.requests[0].Messages())
	}
	if messagesContain(client.requests[0].Messages(), "TASK_STATE_JSON=") {
		t.Fatalf("execution request contains planner-derived task state: %#v", client.requests[0].Messages())
	}
}

func TestRegistryToolLoopLetsExecutionModelRouteCurrentAndRemoteRooms(t *testing.T) {
	objective := "count users in lounge, count users in programming, multiply the counts, ping hack.chat, and summarize once"
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("lounge-call", "saturn_list", map[string]any{"room": "lounge"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("programming-call", "room_users", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("ping-call", "saturn_ping", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("Lounge 1 × programming 1 = 1; hack.chat ping completed.", nil, "stop"),
	}}
	gateway := &routeCommandGateway{}
	directory := &loopRoomDirectory{}
	unrelated := &correctingReadTool{}
	listDefinition, ok := commandcatalog.AgentEntry("list")
	if !ok {
		t.Fatal("saturn list definition is unavailable")
	}
	pingDefinition, ok := commandcatalog.AgentEntry("ping")
	if !ok {
		t.Fatal("saturn ping definition is unavailable")
	}
	tools := []agenttool.Tool{
		agenttool.SaturnCommand{Definition: listDefinition, Gateway: gateway},
		agenttool.RoomUsers{Directory: directory},
		agenttool.SaturnCommand{Definition: pingDefinition, Gateway: gateway},
		unrelated,
	}
	allowed := []string{"saturn_list", "room_users", "saturn_ping", unrelated.Name()}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, tools, allowed,
		turn.ExecutionLimits{MaxSteps: 5, MaxToolCalls: 4, MaxCallsPerTool: 2, MaxToolFailures: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("contextual-room-route", runtime.NewContext("programming", "caller", "", "", false, nil), objective, runtime.DIRECT, "", false)

	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "Lounge 1 × programming 1 = 1; hack.chat ping completed." {
		t.Fatalf("response=%q", completion.Response.Content())
	}
	if directory.calls != 1 || len(gateway.commands) != 2 || gateway.commands[0] != "list lounge" || gateway.commands[1] != "ping" {
		t.Fatalf("directory calls=%d commands=%v", directory.calls, gateway.commands)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests=%d, want four execution requests", len(client.requests))
	}
	if !providerRequestHasTool(client.requests[0], unrelated.Name()) {
		t.Fatalf("initial execution request omitted caller-visible tool %q", unrelated.Name())
	}
	if messagesContain(client.requests[3].Messages(), "TASK_OBLIGATION") {
		t.Fatalf("planner-derived feedback leaked into direct execution: %#v", client.requests[3].Messages())
	}
}

func TestRegistryToolLoopExecutesCompoundCountsThenKickWithoutSemanticPlanner(t *testing.T) {
	objective := "count users in lounge, count users in programming multiply these counts after execute kick tajweed after that summarize everything in one message for me"
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{
			llm.NewLlmToolCall("lounge-call", "saturn_list", map[string]any{"room": "lounge"}),
			llm.NewLlmToolCall("programming-call", "room_users", map[string]any{}),
		}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{
			llm.NewLlmToolCall("kick-call", "saturn_kick", map[string]any{"nick": "tajweed"}),
		}, "tool_calls"),
		llm.NewLlmResponse("Lounge 1 x programming 1 = 1; tajweed was kicked.", nil, "stop"),
	}}
	gateway := &routeCommandGateway{}
	directory := &loopRoomDirectory{}
	listDefinition, ok := commandcatalog.AgentEntry("list")
	if !ok {
		t.Fatal("saturn list definition is unavailable")
	}
	kickDefinition, ok := commandcatalog.AgentEntry("kick")
	if !ok {
		t.Fatal("saturn kick definition is unavailable")
	}
	tools := []agenttool.Tool{
		agenttool.SaturnCommand{Definition: listDefinition, Gateway: gateway},
		agenttool.RoomUsers{Directory: directory},
		agenttool.SaturnCommand{Definition: kickDefinition, Gateway: gateway},
	}
	allowed := []string{"saturn_list", "room_users", "saturn_kick"}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, tools, allowed,
		turn.ExecutionLimits{MaxSteps: 5, MaxToolCalls: 4, MaxCallsPerTool: 2, MaxToolFailures: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("ordered-kick", runtime.NewContextWithCapabilities(
		"programming", "merc", "595754", "", false, []string{"merc", "tajweed"}, []runtime.Capability{runtime.ModerationCommands}, "",
	), objective, runtime.DIRECT, "", true)

	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "Lounge 1 x programming 1 = 1; tajweed was kicked." {
		t.Fatalf("response=%q", completion.Response.Content())
	}
	if directory.calls != 1 || len(gateway.commands) != 2 || gateway.commands[0] != "list lounge" || gateway.commands[1] != "kick tajweed" {
		t.Fatalf("directory calls=%d commands=%v", directory.calls, gateway.commands)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests=%d, want three execution requests and no planning request", len(client.requests))
	}
	if !requestContainsToolCallID(client.requests[2], "kick-call") || !messagesContain(client.requests[2].Messages(), "tajweed") {
		t.Fatal("final synthesis did not receive the executed kick and its target")
	}
	for _, name := range allowed {
		if !providerRequestHasTool(client.requests[0], name) {
			t.Fatalf("initial execution request omitted caller-visible tool %q", name)
		}
	}
	for _, request := range client.requests {
		if messagesContain(request.Messages(), "TASK_OBLIGATION_MISMATCH") || messagesContain(request.Messages(), "TASK_DEPENDENCY_PENDING") {
			t.Fatalf("planner-derived rejection leaked into execution: %#v", request.Messages())
		}
	}
}

func requestContainsToolCallID(request llm.LlmRequest, id string) bool {
	for _, message := range request.Messages() {
		for _, call := range message.ToolCalls() {
			if call.ID() == id {
				return true
			}
		}
	}
	return false
}

func TestRegistryToolLoopAcceptsPlainFinalAnswerWithAutoToolChoice(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("ordinary answer", nil, "stop"),
		llm.NewLlmResponse("unexpected retry", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
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

func TestRegistryToolLoopExecutesDirectToolCallWithAutoChoice(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("read", read.Name(), map[string]any{"value": "requested"})}, "tool_calls"),
		llm.NewLlmResponse("observed value", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2})
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

func TestRegistryToolLoopRepairsTruncationInCanonicalContext(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("unfinished answer", nil, "length"),
		llm.NewLlmResponse("recovered answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	memory := []llm.LlmMessage{
		llm.NewLlmMessage("user", "previous request", nil, ""),
		llm.NewLlmMessage("assistant", "previous assistant offer", nil, ""),
	}
	inv := runtime.NewInvocation("truncated-decision", runtime.NewContext("room", "caller", "", "", false, nil), "who are you?", runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, memory, "")
	if err != nil || completion.Response.Content() != "recovered answer" || read.calls.Load() != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	retry := client.requests[1]
	if retry.ToolChoice() != llm.ToolChoiceAuto || !providerRequestHasTool(retry, read.Name()) {
		t.Fatalf("repair lost native available tools: %#v", retry)
	}
	for _, required := range []string{"previous request", "previous assistant offer", "who are you?", "unfinished answer", "TOOL_LOOP_FEEDBACK="} {
		if !messagesContain(retry.Messages(), required) {
			t.Fatalf("repair dropped %q: %#v", required, retry.Messages())
		}
	}
}

func TestRegistryToolLoopFailsAfterOneTruncationRepair(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("unfinished", nil, "length"),
		llm.NewLlmResponse("still unfinished", nil, "length"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("truncated-twice", runtime.NewContext("room", "caller", "", "", false, nil), "who are you?", runtime.DIRECT, "", false)
	_, err = loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err == nil || !strings.Contains(err.Error(), "invalid final response after structural correction") || read.calls.Load() != 0 || len(client.requests) != 2 {
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
	foundMute, foundKick := false, false
	for _, value := range values {
		foundMute = foundMute || value == "mute"
		foundKick = foundKick || value == "kick"
	}
	if !foundMute || foundKick {
		t.Fatalf("moderator run_command schema has overlapping command routes: %#v", values)
	}
}

func TestRegistryToolLoopHonorsModelReplyChoiceAfterSuccessfulDelivery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		response     string
		wantSuppress bool
	}{
		{name: "already delivered", response: "NO_REPLY", wantSuppress: true},
		{name: "derived synthesis requested", response: "The requested derived answer.", wantSuppress: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delivery := roomDeliveryTool{name: "delivered_command", result: contract.ActionSuccessResult("", "delivered_command", map[string]any{"messages": []string{"already visible"}, "deliveredCount": 1}, 1)}
			client := &scriptedToolClient{responses: []llm.LlmResponse{
				llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("delivery-call", delivery.Name(), map[string]any{})}, "tool_calls"),
				llm.NewLlmResponse(tc.response, nil, "stop"),
			}}
			loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{delivery}, []string{delivery.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
			if err != nil {
				t.Fatal(err)
			}
			inv := runtime.NewInvocation("registry-delivery", runtime.NewContext("room", "caller", "", "", false, nil), "run it", runtime.MENTION, "", false)
			completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
			if err != nil || completion.SuppressReply != tc.wantSuppress || len(client.requests) != 2 {
				t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
			}
			if tc.wantSuppress {
				if !strings.Contains(completion.Response.Content(), "already visible") {
					t.Fatal("suppressed reply lost the delivered answer needed by conversation memory")
				}
			} else if completion.Response.Content() != tc.response {
				t.Fatalf("model synthesis changed: %q", completion.Response.Content())
			}
		})
	}
}

func TestNoReplyRequiresVerifiedRoomDeliveryEvidence(t *testing.T) {
	agent, _ := api.NewContext("room", "caller", "", "", false, nil)
	tool := roomDeliveryTool{name: "delivered_command"}
	registry := agenttool.NewRegistry([]agenttool.Tool{tool}, []string{tool.Name()})
	call := execution.Call{ID: "delivery-call", Name: tool.Name(), Arguments: json.RawMessage(`{}`)}

	for _, result := range []contract.Result{
		contract.SuccessResult("delivery-call", tool.Name(), map[string]any{"deliveredCount": 1}),
		contract.ActionSuccessResult("delivery-call", tool.Name(), map[string]any{"deliveredCount": 0}, 0),
		contract.ErrorResult("delivery-call", tool.Name(), "COMMAND_REJECTED", "rejected"),
	} {
		if canSuppressCompletedReply(registry, agent, []toolBatchResult{{Call: call, Result: result}}) {
			t.Fatalf("unverified delivery suppressed reply: %#v", result)
		}
	}
	verified := contract.ActionSuccessResult("delivery-call", tool.Name(), map[string]any{"deliveredCount": 1}, 1)
	if !canSuppressCompletedReply(registry, agent, []toolBatchResult{{Call: call, Result: verified}}) {
		t.Fatal("verified room delivery did not suppress duplicate reply")
	}
	second := toolBatchResult{
		Call:   execution.Call{ID: "delivery-call-2", Name: tool.Name(), Arguments: json.RawMessage(`{}`)},
		Result: contract.ActionSuccessResult("delivery-call-2", tool.Name(), map[string]any{"deliveredCount": 1}, 1),
	}
	if !canSuppressCompletedReply(registry, agent, []toolBatchResult{{Call: call, Result: verified}, second}) {
		t.Fatal("verified multi-step deliveries were not eligible for no-reply suppression")
	}
	kickDefinition, ok := commandcatalog.AgentEntry("kick")
	if !ok {
		t.Fatal("kick definition is unavailable")
	}
	kick := agenttool.SaturnCommand{Definition: kickDefinition}
	kickRegistry := agenttool.NewRegistry([]agenttool.Tool{kick}, []string{kick.Name()})
	kickResult := contract.ActionSuccessResult("kick-call", kick.Name(), map[string]any{"actionCount": 1, "deliveredCount": 0, "messages": []string{}}, 0)
	if canSuppressCompletedReply(kickRegistry, agent, []toolBatchResult{{Call: execution.Call{ID: "kick-call", Name: kick.Name(), Arguments: json.RawMessage(`{"nick":"tajweed"}`)}, Result: kickResult}}) {
		t.Fatal("silent action was allowed to suppress its required confirmation")
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
				roomDeliveryTool{name: "delivered_command", result: contract.ActionSuccessResult("", "delivered_command", map[string]any{"deliveredCount": 1}, 1)},
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
			loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, tc.tools, allowed, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
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
	_, err := NewRegistryToolLoop(testLiveAssembler(t), &scriptedToolClient{}, []agenttool.Tool{invalidDescriptorTool{}}, []string{"invalid_descriptor"}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
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
	t.Logf("creator provider manifest: %d bytes across %d tools", len(encoded), len(definitions))
	var providerDefinitions []struct {
		Function struct {
			Name       string `json:"name"`
			Strict     bool   `json:"strict"`
			Parameters struct {
				Type string `json:"type"`
			} `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(encoded, &providerDefinitions); err != nil {
		t.Fatal(err)
	}
	optionalParameters := map[string]bool{"user_message_history": true, "database_query": true, "saturn_afk": true, "saturn_automove": true, "database_schema": true}
	for _, definition := range providerDefinitions {
		if definition.Function.Parameters.Type != "object" {
			t.Errorf("%s does not expose a root parameter object", definition.Function.Name)
		}
		if definition.Function.Strict == optionalParameters[definition.Function.Name] {
			t.Errorf("%s strict=%t does not match required/optional parameter contract", definition.Function.Name, definition.Function.Strict)
		}
	}
}

func TestRegistryExecutionFansOutIndependentReadOnlyCalls(t *testing.T) {
	var started atomic.Int32
	ready := make(chan struct{})
	tools := []agenttool.Tool{
		parallelReadTool{name: "read_one", started: &started, ready: ready},
		parallelReadTool{name: "read_two", started: &started, ready: ready},
	}
	registry := agenttool.NewRegistry(tools, []string{"read_one", "read_two"})
	executor := &execution.Executor{Registry: registry, Ledger: execution.NewLedger(map[string]int{"read_one": 1, "read_two": 1}, 2)}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	batch := execution.ExecuteAll(context.Background(), executor, agent, []execution.Call{
		{ID: "one", Name: "read_one", Arguments: json.RawMessage(`{}`)},
		{ID: "two", Name: "read_two", Arguments: json.RawMessage(`{}`)},
	})
	if len(batch) != 2 || batch[0].IsError || batch[1].IsError || started.Load() != 2 {
		t.Fatalf("batch=%#v started=%d", batch, started.Load())
	}
}

func TestRegistryExecutionRejectsDuplicateCallIDsBeforeExecution(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	executor := &execution.Executor{Registry: registry}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	batch := execution.ExecuteAll(context.Background(), executor, agent, []execution.Call{
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"one"}`)},
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"two"}`)},
	})
	if len(batch) != 2 || batch[0].ErrorCode != "INVALID_TOOL_PROTOCOL" || batch[1].ErrorCode != "INVALID_TOOL_PROTOCOL" {
		t.Fatal("duplicate call IDs were accepted")
	}
	if read.calls.Load() != 0 {
		t.Fatalf("tool executed %d times before identity validation", read.calls.Load())
	}
}

func TestRegistryToolLoopRepairsReusedCallIDWithoutExecutingRejectedCall(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("reused", read.Name(), map[string]any{"value": "one"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("reused", read.Name(), map[string]any{"value": "two"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fresh", read.Name(), map[string]any{"value": "two"})}, "tool_calls"),
		llm.NewLlmResponse("read both values", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2, MaxCallsPerTool: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("reused-call-id", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil || completion.Response.Content() != "read both values" || read.calls.Load() != 2 || len(client.requests) != 4 {
		t.Fatalf("completion=%#v calls=%d requests=%d error=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if !messagesContain(client.requests[2].Messages(), "fresh unique IDs") || !providerRequestHasTool(client.requests[2], read.Name()) {
		t.Fatalf("repair lost identity feedback or tools: %#v", client.requests[2])
	}
	for index, request := range client.requests {
		assertAtomicToolProtocol(t, index, request.Messages())
	}
}

func TestRegistryToolLoopKeepsToolsAvailableForArgumentSelfCorrection(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("bad", read.Name(), map[string]any{"value": 7})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fixed", read.Name(), map[string]any{"value": "yes"})}, "tool_calls"),
		llm.NewLlmResponse("corrected answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("self-correction", runtime.NewContext("room", "caller", "", "", false, nil), "read it", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "corrected answer" || read.calls.Load() != 1 || len(client.requests) != 3 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if !providerRequestHasTool(client.requests[1], read.Name()) || len(client.requests[2].Tools()) != 0 {
		t.Fatalf("tool availability after correction: second=%d third=%d", len(client.requests[1].Tools()), len(client.requests[2].Tools()))
	}
	secondMessages := client.requests[1].Messages()
	if !messagesContain(secondMessages, "INVALID_ARGUMENTS") {
		t.Fatalf("invalid-argument observation missing: %#v", secondMessages)
	}
}

func TestRegistryToolLoopCorrectsTypedSaturnArgumentsWithoutInventingCommandText(t *testing.T) {
	definition, ok := commandcatalog.AgentEntry("weather")
	if !ok {
		t.Fatal("weather command is not agent actionable")
	}
	gateway := &typedCommandGateway{}
	command := agenttool.SaturnCommand{Definition: definition, Gateway: gateway}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("bad", command.Name(), map[string]any{"arguments": "Tokyo"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fixed", command.Name(), map[string]any{"location": "Tokyo"})}, "tool_calls"),
		llm.NewLlmResponse("completed", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{command}, []string{command.Name()}, turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 2, MaxCallsPerTool: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("typed-correction", runtime.NewContext("room", "moderator", "trip", "", false, nil), "show weather in Tokyo", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, "")
	if err != nil || completion.Response.Content() != "completed" || gateway.calls != 1 || gateway.command != "weather" || gateway.arguments != "Tokyo" {
		t.Fatalf("completion=%#v gateway=%#v err=%v", completion, gateway, err)
	}
	if len(client.requests) != 3 || !providerRequestHasTool(client.requests[1], command.Name()) || len(client.requests[2].Tools()) != 0 {
		t.Fatalf("typed tool availability after correction: requests=%d second=%d third=%d", len(client.requests), len(client.requests[1].Tools()), len(client.requests[2].Tools()))
	}
	if !messagesContain(client.requests[1].Messages(), "INVALID_ARGUMENTS") {
		t.Fatalf("correctable observation missing: %#v", client.requests[1].Messages())
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
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, limits)
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
	if !messagesContain(messages, "TOOL_CALL_LIMIT_REACHED") {
		t.Fatalf("limit observation missing: %#v", messages)
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
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, limits)
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
	if !messagesContain(messages, "TOOL_DISABLED") {
		t.Fatalf("disabled observation missing: %#v", messages)
	}
	if providerRequestHasTool(client.requests[2], read.Name()) {
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
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 2, MaxCallsPerTool: 2})
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

func TestRegistryToolLoopRepairsInvalidPlainFinalResponseWithAvailableTools(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(" ", nil, "stop"),
		llm.NewLlmResponse("recovered final answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("final-reflection", runtime.NewContext("room", "caller", "", "", false, nil), "answer me", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "recovered final answer" || len(client.requests) != 2 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
	if !providerRequestHasTool(client.requests[1], read.Name()) || client.requests[1].ToolChoice() != llm.ToolChoiceAuto {
		t.Fatalf("structural repair lost available tools: %#v", client.requests[1])
	}
}

func TestEveryProviderRequestFitsMultiRoundBudgetAndKeepsProtocolAtomic(t *testing.T) {
	const maxTokens = 800
	const reserveTokens = 100
	assembler, err := assemble.New(assemble.Config{
		MaxPromptChars: 200, MaxContextTokens: maxTokens, ContextReserveTokens: reserveTokens,
	}, tinyBudgetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	tools := []agenttool.Tool{
		largeBudgetTool{name: "large_one"},
		largeBudgetTool{name: "large_two"},
		largeBudgetTool{name: "large_three"},
	}
	allowed := []string{"large_one", "large_two", "large_three"}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-one", "large_one", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-two", "large_two", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-three", "large_three", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("unfinished terminal answer", nil, "length"),
		llm.NewLlmResponse("final answer from bounded observations", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(
		assembler, client, tools, allowed,
		turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 3, MaxCallsPerTool: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	objective := "Use all three sources and report a final answer."
	inv := runtime.NewInvocation("bounded-multi-round", runtime.NewContext("room", "caller", "", "", false, nil), objective, runtime.DIRECT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "final answer from bounded observations" || len(client.requests) != 5 {
		t.Fatalf("completion=%#v requests=%d", completion, len(client.requests))
	}
	pruned := false
	for index, request := range client.requests {
		assertRequestWithinBudget(t, index, request, maxTokens, reserveTokens)
		assertAtomicToolProtocol(t, index, request.Messages())
		if !messagesContain(request.Messages(), objective) {
			t.Fatalf("request %d lost immutable newest request: %#v", index, request.Messages())
		}
		for _, message := range request.Messages() {
			if message.Role() == "tool" && !json.Valid([]byte(message.Content())) {
				t.Fatalf("request %d contains invalid observation JSON: %q", index, message.Content())
			}
		}
		if projection, ok := request.Projection().(assemble.Projection); ok && projection.Pruned {
			pruned = true
		}
	}
	if !pruned {
		t.Fatal("tiny multi-round budget never pruned an optional context unit")
	}
}

func TestToolLoopEnforcesMaxPromptCharsBeforeProviderCall(t *testing.T) {
	assembler, err := assemble.New(assemble.Config{MaxPromptChars: 3, MaxContextTokens: 500}, tinyBudgetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	read := &correctingReadTool{}
	client := &scriptedToolClient{}
	loop, err := NewRegistryToolLoop(assembler, client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	invocation := runtime.NewInvocation("oversized", runtime.NewContext("room", "caller", "", "", false, nil), "😀😀😀😀", runtime.DIRECT, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), invocation, nil, ""); err == nil || !strings.Contains(err.Error(), "prompt character limit") {
		t.Fatalf("oversized prompt error=%v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider was called %d times for an oversized prompt", len(client.requests))
	}
}

func assertRequestWithinBudget(t *testing.T, index int, request llm.LlmRequest, maxTokens, reserveTokens int) {
	t.Helper()
	messages := make([]map[string]any, 0, len(request.Messages()))
	for _, message := range request.Messages() {
		item := map[string]any{"role": message.Role(), "content": message.ContentNullable()}
		if message.ToolCallID() != "" {
			item["tool_call_id"] = message.ToolCallID()
		}
		if calls := message.ToolCalls(); len(calls) > 0 {
			encodedCalls := make([]map[string]any, len(calls))
			for callIndex, call := range calls {
				encodedCalls[callIndex] = map[string]any{"id": call.ID(), "type": "function", "function": map[string]any{"name": call.Name(), "arguments": call.RawArguments()}}
			}
			item["tool_calls"] = encodedCalls
		}
		messages = append(messages, item)
	}
	messageJSON, _ := json.Marshal(messages)
	toolJSON, _ := json.Marshal(request.Tools())
	estimatedTokens := (len(messageJSON) + len(toolJSON) + 64 + 3) / 4
	if estimatedTokens+reserveTokens > maxTokens {
		t.Fatalf("request %d estimated tokens=%d + reserve=%d exceeds %d", index, estimatedTokens, reserveTokens, maxTokens)
	}
}

func assertAtomicToolProtocol(t *testing.T, requestIndex int, messages []llm.LlmMessage) {
	t.Helper()
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		calls := message.ToolCalls()
		if len(calls) == 0 {
			if message.Role() == "tool" {
				t.Fatalf("request %d has orphan tool message at %d", requestIndex, index)
			}
			continue
		}
		if message.Role() != "assistant" || index+len(calls) >= len(messages) {
			t.Fatalf("request %d has incomplete assistant tool-call unit at %d", requestIndex, index)
		}
		for offset, call := range calls {
			observation := messages[index+offset+1]
			if observation.Role() != "tool" || observation.ToolCallID() != call.ID() {
				t.Fatalf("request %d split tool call %q at %d", requestIndex, call.ID(), index)
			}
		}
		index += len(calls)
	}
}
