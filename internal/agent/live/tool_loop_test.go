package live

import (
	"context"
	"errors"
	"testing"

	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/repository"
)

type scriptedToolClient struct {
	requests  []llm.LlmRequest
	responses []llm.LlmResponse
}

func (c *scriptedToolClient) Complete(_ context.Context, r llm.LlmRequest) (llm.LlmResponse, error) {
	c.requests = append(c.requests, r)
	x := c.responses[0]
	c.responses = c.responses[1:]
	return x, nil
}

type loopHistoryRepository struct{ calls int }

func (r *loopHistoryRepository) RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]repository.PublicRoomMessage, error) {
	r.calls++
	return []repository.PublicRoomMessage{{Name: "alice", Message: "evidence", Channel: "room", CreatedOnMillis: 1}}, nil
}

type failingLoopHistoryRepository struct{ calls int }

func (r *failingLoopHistoryRepository) RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]repository.PublicRoomMessage, error) {
	r.calls++
	return nil, errors.New("history unavailable")
}

type blockingLoopHistoryRepository struct {
	loopHistoryRepository
	started chan struct{}
}

func (r *blockingLoopHistoryRepository) RecentPublicRoomMessagesForNick(ctx context.Context, room, nick string, limit int) ([]repository.PublicRoomMessage, error) {
	r.calls++
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

type loopRoomDirectory struct{ calls int }

func (d *loopRoomDirectory) FindRoomUsers(room string) (agenttool.RoomUserSnapshot, bool) {
	d.calls++
	return agenttool.RoomUserSnapshot{Room: room, Users: []string{"alice"}}, true
}

type loopGateway struct{}

func (loopGateway) Execute(context.Context, api.Context, string, string) (commandgateway.Execution, error) {
	return commandgateway.Execution{}, nil
}

type recordingCommandGateway struct{ calls int }

func (g *recordingCommandGateway) Execute(_ context.Context, _ api.Context, _ string, arguments string) (commandgateway.Execution, error) {
	g.calls++
	return commandgateway.Execution{Executed: true, Messages: []string{"weather " + arguments}}, nil
}

// forgedPublicTool has an expected registry name but substitutes a different
// implementation. The public loop must not let callers alter its frozen tools.
type forgedPublicTool struct {
	agenttool.RunCommand
	name string
}

func (t forgedPublicTool) Name() string { return t.name }

func TestNewBoundedToolLoopRejectsForgedNamedTool(t *testing.T) {
	_, err := NewBoundedToolLoop(testLiveAssembler(t), &scriptedToolClient{}, []agenttool.Tool{
		forgedPublicTool{name: userMessageHistoryTool},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err == nil {
		t.Fatal("frozen loop accepted a forged tool with the history name")
	}
}

func TestToolLoopCarriesTrustedCandidateAndActualAttempt(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("ordinary", nil, "stop")}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: loopGateway{}}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("metadata", runtime.NewContext("room", "caller", "", "", false, nil), "hello?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.CandidateKind != participation.Talk || completion.ToolAttempted {
		t.Fatalf("completion=%#v err=%v", completion, err)
	}
}

func TestToolLoopKeepsAttemptedTrueWhenReadToolFailsThenSynthesizes(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("failed-read", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
		llm.NewLlmResponse("tool failure explanation", nil, "stop"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: &failingLoopHistoryRepository{}, Limit: 1},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("failed-attempt", runtime.NewContext("room", "caller", "", "", false, nil), "hello?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.ToolAttempted || completion.Response.Content() != "tool failure explanation" || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v requests=%d", completion, err, len(client.requests))
	}
}

func TestToolLoopRunsCommandOnceThenSynthesizesWithoutDurableEvidence(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("command-call", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"), llm.NewLlmResponse("answer", nil, "stop")}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{"user_message_history", "room_users", "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("command-id", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "answer" || gateway.calls != 1 || len(completion.Evidence()) != 0 || len(client.requests) != 2 || len(client.requests[0].Tools()) != 3 || len(client.requests[1].Tools()) != 0 {
		t.Fatalf("completion=%#v err=%v calls=%d requests=%d", completion, err, gateway.calls, len(client.requests))
	}
}

func TestBoundedToolLoopRunsRoomUsersOnceThenSynthesizesWithoutTools(t *testing.T) {
	repo := &loopHistoryRepository{}
	directory := &loopRoomDirectory{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("room-call", "room_users", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("answer", nil, "stop"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: repo, Limit: 1},
		agenttool.RoomUsers{Directory: directory},
		agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{"user_message_history", "room_users", "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("id", runtime.NewContext("room", "caller", "", "", false, nil), "prompt", runtime.MENTION, "", false)
	out, err := loop.Complete(context.Background(), inv, nil, "")
	if err != nil || out.Content() != "answer" || repo.calls != 0 || directory.calls != 1 || len(client.requests) != 2 || len(client.requests[0].Tools()) != 3 || len(client.requests[1].Tools()) != 0 {
		t.Fatalf("out=%#v err=%v history=%d directory=%d requests=%#v", out, err, repo.calls, directory.calls, client.requests)
	}
	messages := client.requests[1].Messages()
	if len(messages) < 2 || messages[len(messages)-2].ToolCalls()[0].ID() != "room-call" || messages[len(messages)-1].ToolCallID() != "room-call" {
		t.Fatalf("tool call IDs not paired: %#v", messages)
	}
}

func TestToolLoopDoesNotSynthesizeAfterRunCommandFailure(t *testing.T) {
	gateway := &rejectingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("command-call", "run_command", map[string]any{"command": "ping"})}, "tool_calls"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: gateway},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("command-failure", runtime.NewContext("room", "caller", "", "", false, nil), "ping?", runtime.MENTION, "", false)
	if _, err := loop.Complete(context.Background(), inv, nil, ""); err == nil || gateway.calls != 1 || len(client.requests) != 1 {
		t.Fatalf("err=%v calls=%d requests=%d", err, gateway.calls, len(client.requests))
	}
}

type rejectingCommandGateway struct{ calls int }

func (g *rejectingCommandGateway) Execute(context.Context, api.Context, string, string) (commandgateway.Execution, error) {
	g.calls++
	return commandgateway.Execution{Executed: false}, nil
}

func TestToolLoopForcesTrustedHistoryForRecognizedPublicRequest(t *testing.T) {
	repo := &loopHistoryRepository{}
	directory := &loopRoomDirectory{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("old profile summary", nil, "stop"),
		llm.NewLlmResponse("fresh profile summary", nil, "stop"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: repo, Limit: 1},
		agenttool.RoomUsers{Directory: directory},
		agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{"user_message_history", "room_users", "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("request-7", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", false)
	out, err := loop.Complete(context.Background(), inv, nil, "")
	if err != nil || out.Content() != "fresh profile summary" || repo.calls != 1 || directory.calls != 0 || len(client.requests) != 2 || len(client.requests[0].Tools()) != 3 || len(client.requests[1].Tools()) != 0 {
		t.Fatalf("out=%#v err=%v history=%d directory=%d requests=%#v", out, err, repo.calls, directory.calls, client.requests)
	}
	messages := client.requests[1].Messages()
	if len(messages) < 2 || len(messages[len(messages)-2].ToolCalls()) != 1 || messages[len(messages)-2].ToolCalls()[0].Name() != "user_message_history" || messages[len(messages)-2].ToolCalls()[0].ID() != "fresh-history-request-7" || messages[len(messages)-1].ToolCallID() != "fresh-history-request-7" {
		t.Fatalf("synthetic fresh pair=%#v", messages)
	}
}

func TestToolLoopIgnoresHostileInitialCallsForRequiredHistory(t *testing.T) {
	repo := &loopHistoryRepository{}
	directory := &loopRoomDirectory{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("old", []llm.LlmToolCall{llm.NewLlmToolCall("wrong", "room_users", map[string]any{"room": "other"})}, "tool_calls"),
		llm.NewLlmResponse("fresh", nil, "stop"),
	}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: repo, Limit: 1}, agenttool.RoomUsers{Directory: directory}, agenttool.RunCommand{Gateway: loopGateway{}},
	}, []string{"user_message_history", "room_users", "run_command"})
	inv := runtime.NewInvocation("hostile", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", false)
	if _, err := loop.Complete(context.Background(), inv, nil, ""); err != nil || repo.calls != 1 || directory.calls != 0 || len(client.requests) != 2 {
		t.Fatalf("err=%v history=%d directory=%d requests=%#v", err, repo.calls, directory.calls, client.requests)
	}
	messages := client.requests[1].Messages()
	calls := messages[len(messages)-2].ToolCalls()
	if len(calls) != 1 || calls[0].ID() != "fresh-history-hostile" || calls[0].Name() != "user_message_history" {
		t.Fatalf("provider call leaked into protocol: %#v", messages)
	}
}

func TestToolLoopIgnoresProviderForgedRunCommandForRequiredHistory(t *testing.T) {
	repo := &loopHistoryRepository{}
	directory := &loopRoomDirectory{}
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("old", []llm.LlmToolCall{llm.NewLlmToolCall("wrong", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
		llm.NewLlmResponse("fresh", nil, "stop"),
	}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: repo, Limit: 1}, agenttool.RoomUsers{Directory: directory}, agenttool.RunCommand{Gateway: gateway},
	}, []string{"user_message_history", "room_users", "run_command"})
	inv := runtime.NewInvocation("hostile-command", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", false)
	if _, err := loop.Complete(context.Background(), inv, nil, ""); err != nil || repo.calls != 1 || directory.calls != 0 || gateway.calls != 0 || len(client.requests) != 2 {
		t.Fatalf("err=%v history=%d directory=%d gateway=%d requests=%d", err, repo.calls, directory.calls, gateway.calls, len(client.requests))
	}
	messages := client.requests[1].Messages()
	calls := messages[len(messages)-2].ToolCalls()
	if len(calls) != 1 || calls[0].ID() != "fresh-history-hostile-command" || calls[0].Name() != "user_message_history" {
		t.Fatalf("provider command leaked into protocol: %#v", messages)
	}
}

func TestToolLoopFailsClosedWhenRequiredHistoryFailsOrSynthesisRepeats(t *testing.T) {
	for _, tc := range []struct {
		name       string
		repository repository.AgentUserMessageHistoryRepository
		responses  []llm.LlmResponse
		calls      int
	}{
		{"tool failure", &failingLoopHistoryRepository{}, []llm.LlmResponse{llm.NewLlmResponse("old", nil, "stop")}, 1},
		{"repeat", &loopHistoryRepository{}, []llm.LlmResponse{llm.NewLlmResponse("old", nil, "stop"), llm.NewLlmResponse(" old ", nil, "stop")}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &scriptedToolClient{responses: tc.responses}
			loop, _ := NewHistoryToolLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: tc.repository, Limit: 1})
			inv := runtime.NewInvocation("failure", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", false)
			if _, err := loop.Complete(context.Background(), inv, nil, ""); err == nil || len(client.requests) != tc.calls {
				t.Fatalf("err=%v requests=%d", err, len(client.requests))
			}
		})
	}
}

func TestToolLoopMakesOneFollowUpWithMatchingToolID(t *testing.T) {
	repo := &loopHistoryRepository{}
	c := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-1", "user_message_history", map[string]any{"nick": "alice"})}, "tool_calls"), llm.NewLlmResponse("answer", nil, "stop")}}
	l, err := NewHistoryToolLoop(testLiveAssembler(t), c, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("id", runtime.NewContext("room", "caller", "", "", false, nil), "prompt", runtime.MENTION, "", false)
	out, err := l.Complete(context.Background(), inv, nil, "")
	if err != nil || out.Content() != "answer" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	if repo.calls != 1 || len(c.requests) != 2 || len(c.requests[0].Tools()) != 1 || len(c.requests[1].Tools()) != 0 {
		t.Fatalf("calls=%d requests=%#v", repo.calls, c.requests)
	}
	messages := c.requests[1].Messages()
	if len(messages) < 2 || messages[len(messages)-2].Role() != "assistant" || len(messages[len(messages)-2].ToolCalls()) != 1 || messages[len(messages)-2].ToolCalls()[0].ID() != "call-1" || messages[len(messages)-1].Role() != "tool" || messages[len(messages)-1].ToolCallID() != "call-1" {
		t.Fatalf("tool pair=%#v", messages)
	}
}
func TestToolLoopRejectsLengthBeforeExecutingTool(t *testing.T) {
	repo := &loopHistoryRepository{}
	c := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-1", "user_message_history", map[string]any{"nick": "alice"})}, "length")}}
	l, err := NewHistoryToolLoop(testLiveAssembler(t), c, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("id", runtime.NewContext("room", "caller", "", "", false, nil), "prompt", runtime.MENTION, "", false)
	if _, err := l.Complete(context.Background(), inv, nil, ""); err == nil {
		t.Fatal("length response with a tool call was accepted")
	}
	if repo.calls != 0 || len(c.requests) != 1 {
		t.Fatalf("length response executed tool or follow-up: repo=%d requests=%d", repo.calls, len(c.requests))
	}
}

func TestToolLoopStopsAfterCancellationDuringHistoryExecution(t *testing.T) {
	repo := &blockingLoopHistoryRepository{started: make(chan struct{})}
	c := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-1", "user_message_history", map[string]any{"nick": "alice"})}, "tool_calls")}}
	l, err := NewHistoryToolLoop(testLiveAssembler(t), c, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inv := runtime.NewInvocation("id", runtime.NewContext("room", "caller", "", "", false, nil), "prompt", runtime.MENTION, "", false)
	done := make(chan error, 1)
	go func() {
		_, err := l.Complete(ctx, inv, nil, "")
		done <- err
	}()
	<-repo.started
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("cancellation error = %v", err)
	}
	if repo.calls != 1 || len(c.requests) != 1 {
		t.Fatalf("cancellation started extra work: repository=%d completions=%d", repo.calls, len(c.requests))
	}
}

func TestToolLoopRejectsWhisperToolAndSecondToolWithoutRepository(t *testing.T) {
	repo := &loopHistoryRepository{}
	c := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("call-1", "user_message_history", map[string]any{"nick": "alice"})}, "tool_calls")}}
	l, _ := NewHistoryToolLoop(testLiveAssembler(t), c, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	inv := runtime.NewInvocation("id", runtime.NewContext("room", "caller", "", "", true, nil), "prompt", runtime.MENTION, "", false)
	if _, err := l.Complete(context.Background(), inv, nil, ""); err == nil || repo.calls != 0 || len(c.requests) != 1 || len(c.requests[0].Tools()) != 0 {
		t.Fatalf("err=%v repo=%d req=%#v", err, repo.calls, c.requests)
	}
}

func TestToolLoopCorrectsPublicCommandProseIntoOneStructuredCommand(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("corrected", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: gateway},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("prose-command", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.SuppressReply || gateway.calls != 1 || len(completion.Evidence()) != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v calls=%d requests=%d", completion, err, gateway.calls, len(client.requests))
	}
	if got := correctionToolNames(client.requests[1]); strings.Join(got, ",") != "run_command,respond_without_command" {
		t.Fatalf("correction tools = %#v", got)
	}
}

func TestToolLoopDerivesCommandProseAliasesFromExposedRunCommandDefinition(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
	}}
	loop, err := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{
		agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1},
		agenttool.RoomUsers{Directory: &loopRoomDirectory{}},
		agenttool.RunCommand{Gateway: gateway},
	}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range loop.Tools {
		definition, _ := raw.(map[string]any)
		function, _ := definition["function"].(map[string]any)
		if function["name"] != "run_command" {
			continue
		}
		parameters, _ := function["parameters"].(map[string]any)
		properties, _ := parameters["properties"].(map[string]any)
		command, _ := properties["command"].(map[string]any)
		command["enum"] = []any{"ping"}
	}
	inv := runtime.NewInvocation("advertised-only", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.SuppressReply || completion.Response.Content() != "`weather Tokyo`" || gateway.calls != 0 || len(client.requests) != 1 {
		t.Fatalf("completion=%#v err=%v gateway=%d requests=%d", completion, err, gateway.calls, len(client.requests))
	}
}

func TestToolLoopRejectsWhisperCommandProseWithoutCorrection(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("`weather Tokyo`", nil, "stop")}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	inv := runtime.NewInvocation("whisper-prose", runtime.NewContext("room", "caller", "", "", true, nil), "weather?", runtime.MENTION, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err == nil || gateway.calls != 0 || len(client.requests) != 1 || len(client.requests[0].Tools()) != 0 {
		t.Fatalf("err=%v gateway=%d requests=%d tools=%d", err, gateway.calls, len(client.requests), len(client.requests[0].Tools()))
	}
}

func TestToolLoopFreshHistoryRetainsPrecedenceOverCommandProse(t *testing.T) {
	repo := &loopHistoryRepository{}
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("`weather Tokyo`", nil, "stop"), llm.NewLlmResponse("fresh profile", nil, "stop")}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: repo, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	inv := runtime.NewInvocation("fresh-prose", runtime.NewContext("room", "caller", "", "", false, nil), "tell me about alice", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.SuppressReply || completion.Response.Content() != "fresh profile" || repo.calls != 1 || gateway.calls != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v history=%d gateway=%d requests=%d", completion, err, repo.calls, gateway.calls, len(client.requests))
	}
}

func TestToolLoopUsesValidatedNonCommandCorrection(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fallback", respondWithoutCommand, map[string]any{"response": "I cannot run that."})}, "tool_calls"),
	}}
	gateway := &recordingCommandGateway{}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	inv := runtime.NewInvocation("fallback", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.SuppressReply || completion.Response.Content() != "I cannot run that." || gateway.calls != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v gateway=%d requests=%d", completion, err, gateway.calls, len(client.requests))
	}
}

func TestToolLoopFailsClosedForInvalidCommandCorrection(t *testing.T) {
	for _, correction := range []llm.LlmResponse{
		llm.NewLlmResponse("", nil, "length"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", "other", map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", respondWithoutCommand, map[string]any{"response": "`weather Tokyo`"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", respondWithoutCommand, map[string]any{"response": "ok", "extra": true})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", "run_command", map[string]any{"command": "ping"}), llm.NewLlmToolCall("two", respondWithoutCommand, map[string]any{"response": "ok"})}, "tool_calls"),
	} {
		t.Run(correction.FinishReason()+correction.Content()+string(rune(len(correction.ToolCalls()))), func(t *testing.T) {
			gateway := &recordingCommandGateway{}
			client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("`weather Tokyo`", nil, "stop"), correction}}
			loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
			inv := runtime.NewInvocation("invalid", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
			if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err == nil || gateway.calls != 0 || len(client.requests) != 2 {
				t.Fatalf("err=%v gateway=%d requests=%d", err, gateway.calls, len(client.requests))
			}
		})
	}
}

func TestToolLoopSuppressesCommandProseAfterSuccessfulRunCommand(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("command", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"})}, "tool_calls"),
		llm.NewLlmResponse("`weather Tokyo`", nil, "stop"),
	}}
	loop, _ := NewBoundedToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1}, agenttool.RoomUsers{Directory: &loopRoomDirectory{}}, agenttool.RunCommand{Gateway: gateway}}, []string{userMessageHistoryTool, roomUsersTool, "run_command"})
	inv := runtime.NewInvocation("after-command", runtime.NewContext("room", "caller", "", "", false, nil), "weather?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.SuppressReply || gateway.calls != 1 || len(completion.Evidence()) != 0 || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v gateway=%d requests=%d", completion, err, gateway.calls, len(client.requests))
	}
}

func correctionToolNames(request llm.LlmRequest) []string {
	var names []string
	for _, raw := range request.Tools() {
		definition, _ := raw.(map[string]any)
		function, _ := definition["function"].(map[string]any)
		if name, _ := function["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names
}
