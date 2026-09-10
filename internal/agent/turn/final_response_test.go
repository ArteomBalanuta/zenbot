package turn

import (
	"testing"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
)

func TestFinalResponseValidatorRejectsInvalidTerminalResponses(t *testing.T) {
	validator := FinalResponseValidator{}
	tests := []struct {
		name     string
		response llm.LlmResponse
		input    FinalResponseInput
	}{
		{name: "empty", response: llm.NewLlmResponse(" ", nil, "stop")},
		{name: "truncated", response: llm.NewLlmResponse("partial", nil, "length")},
		{name: "still calling tools", response: llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("1", "lookup", map[string]any{})}, "tool_calls")},
		{name: "textual native tool call", response: llm.NewLlmResponse(`<|tool_call>call:run_command{arguments:<|\"|>lounge<|\"|>,command:<|\"|>room_users<|\"|>}<tool_call|>`, nil, "stop")},
		{name: "repeats prior answer", response: llm.NewLlmResponse("same", nil, "stop"), input: FinalResponseInput{PriorAssistant: "same"}},
		{name: "missing required evidence", response: llm.NewLlmResponse("answer", nil, "stop"), input: FinalResponseInput{RequiredTool: "history"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.Response = tc.response
			if err := validator.Validate(tc.input); err == nil {
				t.Fatal("invalid final response was accepted")
			}
		})
	}
	if err := validator.Validate(FinalResponseInput{
		Response:     llm.NewLlmResponse("fresh answer", nil, "stop"),
		RequiredTool: "history",
		Results:      []contract.Result{contract.SuccessResult("1", "history", map[string]any{"count": 1})},
	}); err != nil {
		t.Fatalf("valid final response rejected: %v", err)
	}
}
