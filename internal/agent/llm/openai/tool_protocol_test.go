package openai

import (
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

func TestMalformedArgumentsRemainToolObservationsInsteadOfRejectingResponse(t *testing.T) {
	response, err := decodeResponse([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"bad","function":{"name":"lookup","arguments":"{broken"}},{"id":"good","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},"finish_reason":"tool_calls"}]}`))
	if err != nil {
		t.Fatalf("one invalid call discarded the complete response: %v", err)
	}
	calls := response.ToolCalls()
	if len(calls) != 2 || calls[0].RawArguments() != "{broken" || !strings.Contains(calls[1].RawArguments(), "9007199254740993") {
		t.Fatalf("raw calls were changed: %#v", calls)
	}
	encoded := messageJSON([]llm.LlmMessage{llm.NewLlmMessage("assistant", nil, calls, "")})
	wireCalls := encoded[0]["tool_calls"].([]map[string]any)
	if got := wireCalls[0]["function"].(map[string]any)["arguments"]; got != "{broken" {
		t.Fatalf("correction transcript changed invalid arguments to %v", got)
	}
	if got := wireCalls[1]["function"].(map[string]any)["arguments"]; got != `{"id":9007199254740993}` {
		t.Fatalf("integer precision changed in transcript: %v", got)
	}
}

func TestToolFreeRequestOverridesStaleProviderOptions(t *testing.T) {
	payload, err := requestPayload(Config{Options: map[string]any{
		"tools": []any{"stale tool"}, "tool_choice": "required", "parallel_tool_calls": true,
		"response_format": map[string]any{"type": "json_object"},
	}}, llm.NewLlmRequest(nil, nil, false, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tools", "tool_choice", "parallel_tool_calls", "response_format"} {
		if _, present := payload[name]; present {
			t.Errorf("request-controlled field %s leaked from provider options", name)
		}
	}
}
