package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

func TestCompletePreservesNullContentAndUsage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		messages := body["messages"].([]any)
		if messages[0].(map[string]any)["content"] != nil {
			t.Errorf("content was not JSON null: %#v", messages[0])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":null},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`))
	}))
	defer s.Close()
	got, err := NewClient(Config{Endpoint: s.URL}).Complete(context.Background(), llm.NewLlmRequest([]llm.LlmMessage{llm.NewLlmMessage("assistant", nil, nil, "")}, nil, false, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentNullable() != nil || got.Usage()["prompt_tokens"] != 7 || got.Usage()["completion_tokens"] != 3 {
		t.Fatalf("response metadata lost: %#v", got.Usage())
	}
}

func TestNewRejectsInvalidProviderConfiguration(t *testing.T) {
	for _, cfg := range []Config{{Endpoint: "relative", Retries: -1}, {Endpoint: "http://example.test", MaxTokens: -1}, {Endpoint: "http://example.test", Temperature: floatPtr(3)}} {
		if _, err := New(cfg, nil); err == nil {
			t.Fatalf("accepted invalid config: %#v", cfg)
		}
	}
}

func TestProviderErrorDoesNotLeakToken(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("secret-token")) }))
	defer s.Close()
	_, err := NewClient(Config{Endpoint: s.URL, Token: "secret-token"}).Complete(context.Background(), testRequest())
	var le *llm.LlmError
	if !errors.As(err, &le) || strings.Contains(err.Error(), "secret-token") || le.Snippet == "" {
		t.Fatalf("unsafe provider error: %v", err)
	}
}

func floatPtr(v float64) *float64 { return &v }
