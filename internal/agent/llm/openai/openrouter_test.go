package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

func TestCompletionEndpointForms(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://openrouter.ai/api", "https://openrouter.ai/api/v1/chat/completions"},
		{"https://openrouter.ai/api/v1/", "https://openrouter.ai/api/v1/chat/completions"},
		{"https://openrouter.ai/api/v1/chat/completions", "https://openrouter.ai/api/v1/chat/completions"},
		{"http://localhost:16261", "http://localhost:16261/v1/chat/completions"},
		{"https://example.test/v1?region=eu", "https://example.test/v1/chat/completions?region=eu"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := endpointURL(tc.input); got != tc.want {
				t.Fatalf("got %s; want %s", got, tc.want)
			}
		})
	}
}

func TestOpenRouterWireContract(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://openrouter.ai/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("incorrect URL or authorization")
			}
			var p map[string]any
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"chat_template_kwargs", "bypass_prompt_cache"} {
				if _, exists := p[field]; exists {
					t.Errorf("unsupported field %s", field)
				}
			}
			reasoning, ok := p["reasoning"].(map[string]any)
			if !ok || reasoning["enabled"] != thinking {
				t.Errorf("reasoning=%v", p["reasoning"])
			}
			if p["model"] != "openai/gpt-4o" || p["stream"] != false || p["tool_choice"] != "auto" || len(p["tools"].([]any)) != 1 {
				t.Error("model/tool contract lost")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"saturn_ping","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))}, nil
		})
		client := NewClient(Config{Endpoint: "https://openrouter.ai/api/v1", Token: "test-key", Model: "openai/gpt-4o", ThinkingEnabled: thinking, HTTP: &http.Client{Transport: transport}})
		request := llm.NewLlmRequest(testRequest().Messages(), []any{map[string]any{"type": "function", "function": map[string]any{"name": "saturn_ping", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}, true, nil, nil)
		response, err := client.Complete(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		calls := response.ToolCalls()
		if len(calls) != 1 || calls[0].Name() != "saturn_ping" || calls[0].ID() != "call-1" || calls[0].RawArguments() != "{}" {
			t.Fatalf("lost tool call: %v", calls)
		}
	}
}

func TestProviderNumericErrorEnvelopes(t *testing.T) {
	for _, status := range []int{200, 402, 429} {
		client := NewClient(Config{Endpoint: "https://openrouter.ai/api/v1", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":402,"message":"Insufficient credits","metadata":{"raw":"private upstream payload"}}}`))}, nil
		})}})
		_, err := client.Complete(context.Background(), testRequest())
		var providerErr *llm.LlmError
		if !errors.As(err, &providerErr) || providerErr.ProviderCode != "402" || providerErr.ProviderMessage != "Insufficient credits" || providerErr.Status != status {
			t.Fatalf("status %d: error %#v", status, err)
		}
		if strings.Contains(err.Error(), "private upstream payload") {
			t.Fatal("raw metadata leaked")
		}
	}
}
