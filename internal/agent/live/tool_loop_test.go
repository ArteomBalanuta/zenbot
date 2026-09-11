package live

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/repository"
)

type scriptedToolClient struct {
	requests  []llm.LlmRequest
	responses []llm.LlmResponse
}

func (c *scriptedToolClient) Complete(ctx context.Context, r llm.LlmRequest) (llm.LlmResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.LlmResponse{}, err
	}
	c.requests = append(c.requests, r)
	if len(c.responses) == 0 {
		return llm.LlmResponse{}, errors.New("unexpected model round")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
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

func verifiedGatewayExecution(messages ...string) commandgateway.Execution {
	return commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true,
		Action: &commandgateway.ActionReceipt{Count: 1}, Messages: append([]string{}, messages...), Delivery: &commandgateway.DeliveryReceipt{Count: len(messages)}}
}
func (g *recordingCommandGateway) Execute(_ context.Context, _ api.Context, _ string, arguments string) (commandgateway.Execution, error) {
	g.calls++
	return verifiedGatewayExecution("weather " + arguments), nil
}

type rejectingCommandGateway struct{ calls int }

func (g *rejectingCommandGateway) Execute(context.Context, api.Context, string, string) (commandgateway.Execution, error) {
	g.calls++
	return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
}
func testBoundedLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []agenttool.Tool, allowed []string) (*ToolLoop, error) {
	return NewRegistryToolLoop(assembler, client, tools, allowed, ToolLoopLimits())
}
func testHistoryLoop(assembler *assemble.Assembler, client llm.LlmClient, history agenttool.UserMessageHistory) (*ToolLoop, error) {
	return testBoundedLoop(assembler, client, []agenttool.Tool{history}, []string{history.Name()})
}
func TestToolLoopOrdinaryAnswerDoesNotExecuteOrAddJudgeRound(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("ordinary", nil, "stop")}}
	loop, err := testHistoryLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: &loopHistoryRepository{}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("plain", runtime.NewContext("room", "caller", "", "", false, nil), "hello?", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.ToolAttempted || completion.Response.Content() != "ordinary" || len(client.requests) != 1 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
}
func TestToolLoopReadFailureBecomesModelFeedback(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("failed-read", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
		llm.NewLlmResponse("The history lookup failed.", nil, "stop"),
	}}
	loop, err := testHistoryLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: &failingLoopHistoryRepository{}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("failed", runtime.NewContext("room", "caller", "", "", false, nil), "check alice", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.ToolAttempted || completion.Response.Content() != "The history lookup failed." || len(client.requests) != 2 {
		t.Fatalf("completion=%#v err=%v", completion, err)
	}
	if !messagesContain(client.requests[1].Messages(), "error") {
		t.Fatal("failure not visible")
	}
}
func TestToolLoopCommandFailureStillProducesFinalExplanation(t *testing.T) {
	gateway := &rejectingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("cmd", "run_command", map[string]any{"command": "ping"})}, "tool_calls"),
		llm.NewLlmResponse("The ping command failed.", nil, "stop"),
	}}
	loop, err := testBoundedLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.RunCommand{Gateway: gateway}}, []string{"run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("failed-cmd", runtime.NewContext("room", "caller", "", "", false, nil), "ping", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || gateway.calls != 1 || completion.Response.Content() != "The ping command failed." {
		t.Fatalf("completion=%#v err=%v", completion, err)
	}
}
func TestToolLoopPreservesToolIDAndActualHistoryEvidence(t *testing.T) {
	repo := &loopHistoryRepository{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("history-1", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
		llm.NewLlmResponse("Alice wrote evidence.", nil, "stop"),
	}}
	loop, err := testHistoryLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("history", runtime.NewContext("room", "caller", "", "", false, nil), "check alice", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || repo.calls != 1 || len(client.requests) != 2 || len(completion.Evidence()) != 1 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
	if !requestContainsToolCallID(client.requests[1], "history-1") {
		t.Fatal("original tool id lost")
	}
	assertAtomicToolProtocol(t, 1, client.requests[1].Messages())
	if !messagesContain(client.requests[1].Messages(), "oldestCreatedOn") {
		t.Fatal("real history result metadata missing")
	}
}
func TestToolLoopStopsAfterCancellationDuringHistoryExecution(t *testing.T) {
	repo := &blockingLoopHistoryRepository{started: make(chan struct{})}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("history", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
	}}
	loop, err := testHistoryLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inv := runtime.NewInvocation("cancel", runtime.NewContext("room", "caller", "", "", false, nil), "check alice", runtime.MENTION, "", false)
	done := make(chan error, 1)
	go func() { _, err := loop.Complete(ctx, inv, nil, ""); done <- err }()
	<-repo.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if repo.calls != 1 || len(client.requests) != 1 {
		t.Fatalf("extra work after cancellation: repo=%d requests=%d", repo.calls, len(client.requests))
	}
}
func TestToolLoopDoesNotExecuteUnexposedWhisperTool(t *testing.T) {
	repo := &loopHistoryRepository{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("history", userMessageHistoryTool, map[string]any{"nick": "alice"})}, "tool_calls"),
		llm.NewLlmResponse("Tools are unavailable for this invocation.", nil, "stop"),
	}}
	loop, err := testHistoryLoop(testLiveAssembler(t), client, agenttool.UserMessageHistory{Repository: repo, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("whisper", runtime.NewContext("room", "caller", "", "", true, nil), "check alice", runtime.MENTION, "", false)
	_, err = loop.Complete(context.Background(), inv, nil, "")
	if err != nil || repo.calls != 0 || len(client.requests[0].Tools()) != 0 || !messagesContain(client.requests[1].Messages(), "TOOL_NOT_ALLOWED") {
		t.Fatalf("repo=%d requests=%#v err=%v", repo.calls, client.requests, err)
	}
}
func TestToolLoopDoesNotInterpretAnswerProseAsAnExecutableCommand(t *testing.T) {
	gateway := &recordingCommandGateway{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("The weather command accepts a location.", nil, "stop")}}
	loop, err := testBoundedLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.RunCommand{Gateway: gateway}}, []string{"run_command"})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("docs", runtime.NewContext("room", "caller", "", "", false, nil), "Explain the weather command.", runtime.DIRECT, "", false)
	_, err = loop.Complete(context.Background(), inv, nil, "")
	if err != nil || gateway.calls != 0 || len(client.requests) != 1 {
		t.Fatalf("calls=%d requests=%d err=%v", gateway.calls, len(client.requests), err)
	}
}
