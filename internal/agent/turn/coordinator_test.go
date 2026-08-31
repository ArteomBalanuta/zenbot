package turn

import (
	"context"
	"testing"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

type scriptedLLM struct {
	responses []llm.LlmResponse
	calls     int
}

func (s *scriptedLLM) Complete(context.Context, llm.LlmRequest) (llm.LlmResponse, error) {
	s.calls++
	if len(s.responses) == 0 {
		return llm.NewLlmResponse("", nil, "stop"), nil
	}
	r := s.responses[0]
	s.responses = s.responses[1:]
	return r, nil
}

type countingExecutor struct {
	calls  int
	result contract.Result
}

func (e *countingExecutor) Execute(context.Context, api.Context, execution.Call) contract.Result {
	e.calls++
	return e.result
}
func TestFreshCoordinatorReservesBeforeExecuteAndValidatesTarget(t *testing.T) {
	ctx, _ := api.NewContext("room", "caller", "", "", false, []string{})
	ex := &countingExecutor{result: contract.SuccessResult("id", UserMessageHistory, "history")}
	client := &scriptedLLM{responses: []llm.LlmResponse{llm.NewLlmResponse("fresh", nil, "stop")}}
	s := NewState(ExecutionLimits{MaxToolCalls: 0})
	c := FreshDataCoordinator{Client: client, Executor: ex}
	call := llm.NewLlmToolCall("id", UserMessageHistory, map[string]any{"nick": "other"})
	_, err := c.Process(context.Background(), FreshProcessInput{Response: llm.NewLlmResponse("candidate", []llm.LlmToolCall{call}, "tool"), RequiredTool: UserMessageHistory, RequiredNick: "wanted", Context: ctx, State: s})
	if err == nil || ex.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, ex.calls)
	}
}
func TestFreshValidatorRejectsIncompleteSynthesis(t *testing.T) {
	s := NewState(ExecutionLimits{})
	if err := (FinalValidator{}).Validate(llm.NewLlmResponse("", nil, "stop"), s, UserMessageHistory); err == nil {
		t.Fatal("missing evidence")
	}
	s.RecordSuccessfulTool(UserMessageHistory)
	if err := (FinalValidator{}).Validate(llm.NewLlmResponse("", nil, "stop"), s, UserMessageHistory); err == nil {
		t.Fatal("empty synthesis")
	}
}
