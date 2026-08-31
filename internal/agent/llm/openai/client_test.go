package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/agent/llm"
)

func testRequest() llm.LlmRequest {
	return llm.NewLlmRequest([]llm.LlmMessage{llm.NewLlmMessage("user", "hello", nil, "")}, nil, false, nil, nil)
}
func noSleep(context.Context, time.Duration) error { return nil }

func TestCompleteSuccessAndRequestOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("bad request metadata")
		}
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		for _, want := range []string{`"model":"model-x"`, `"max_tokens":42`, `"stream":false`, `"bypass_prompt_cache":true`} {
			if !strings.Contains(body, want) {
				t.Errorf("request missing %s: %s", want, body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	c := NewClient(Config{Endpoint: server.URL, Token: "secret-token", Model: "model-x", MaxTokens: 42, Retries: 0})
	r := llm.NewLlmRequest(testRequest().Messages(), nil, true, nil, nil)
	got, err := c.Complete(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content() != "answer" || got.FinishReason() != "stop" {
		t.Fatalf("response = %#v", got)
	}
}

func TestNewClientClonesNestedOptions(t *testing.T) {
	nested := map[string]string{"region": "west"}
	options := map[string]any{"provider": nested}
	c := NewClient(Config{Endpoint: "http://example.invalid", Options: options})
	nested["region"] = "east"
	if c.Options["provider"].(map[string]string)["region"] != "west" {
		t.Fatal("client retained mutable options input")
	}
	c.Options["provider"].(map[string]string)["region"] = "mutated"
	if c.Options["provider"].(map[string]string)["region"] != "mutated" {
		t.Fatal("client options were unexpectedly immutable internally")
	}
}

func TestCompleteMapsToolCallsAndToolResults(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`)
	}))
	defer s.Close()
	msg := llm.NewLlmMessage("tool", "result", nil, "c1")
	got, err := NewClient(Config{Endpoint: s.URL}).Complete(context.Background(), llm.NewLlmRequest([]llm.LlmMessage{msg}, []any{map[string]any{"type": "function"}}, false, map[string]any{"type": "json_object"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.Content() != "" || got.FinishReason() != "" || len(got.ToolCalls()) != 1 || got.ToolCalls()[0].Arguments()["q"] != "x" {
		t.Fatalf("bad tool response: %#v", got)
	}
	for _, want := range []string{`"tool_call_id":"c1"`, `"tool_choice":"auto"`, `"tools"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request missing %s", want)
		}
	}
}

func TestCompleteMalformedSuccessfulResponse(t *testing.T) {
	for _, body := range []string{`not-json`, `{"choices":[]}`, `{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"["}}]}}]}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		_, err := NewClient(Config{Endpoint: s.URL}).Complete(context.Background(), testRequest())
		s.Close()
		var le *llm.LlmError
		if !errors.As(err, &le) || le.Code != "malformed_response" {
			t.Errorf("body %s error=%v", body, err)
		}
	}
}

func TestCompleteRetriesTransientButNotClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		var n atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			if n.Load() == 1 {
				w.WriteHeader(status)
				return
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		}))
		_, err := NewClient(Config{Endpoint: s.URL, Retries: 1, Sleep: noSleep}).Complete(context.Background(), testRequest())
		s.Close()
		if err != nil || n.Load() != 2 {
			t.Fatalf("status %d err=%v attempts=%d", status, err, n.Load())
		}
	}
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `secret-token`)
	}))
	defer s.Close()
	_, err := NewClient(Config{Endpoint: s.URL, Retries: 3, Sleep: noSleep, Token: "secret-token"}).Complete(context.Background(), testRequest())
	var le *llm.LlmError
	if !errors.As(err, &le) || n.Load() != 1 || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("4xx/secret error=%v attempts=%d", err, n.Load())
	}
}

func TestCompleteTransportErrorCancellationAndTimeout(t *testing.T) {
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("dial secret-token") })
	_, err := NewClient(Config{Endpoint: "http://example.invalid", HTTP: &http.Client{Transport: tr}, Retries: 0, Token: "secret-token"}).Complete(context.Background(), testRequest())
	if !strings.Contains(err.Error(), "transport") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewClient(Config{Endpoint: "http://example.invalid"}).Complete(ctx, testRequest())
	var le *llm.LlmError
	if !errors.As(err, &le) || le.Code != "cancelled" {
		t.Fatalf("cancel error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCompletePreservesProviderDiagnostics(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":"req-1","system_fingerprint":"fp","provider_meta":{"region":"west"},"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer s.Close()
	got, err := NewClient(Config{Endpoint: s.URL}).Complete(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	d := got.ProviderDiagnostics()
	if d["id"] != "req-1" || d["system_fingerprint"] != "fp" || d["provider_meta"].(map[string]any)["region"] != "west" {
		t.Fatalf("diagnostics=%#v", d)
	}
	d["provider_meta"].(map[string]any)["region"] = "mutated"
	if got.ProviderDiagnostics()["provider_meta"].(map[string]any)["region"] != "west" {
		t.Fatal("diagnostics accessor was mutable")
	}
}

func TestCompleteCancellationDuringRequest(t *testing.T) {
	var attempts atomic.Int32
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewClient(Config{Endpoint: "http://example.invalid", HTTP: &http.Client{Transport: tr}}).Complete(ctx, testRequest())
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		var le *llm.LlmError
		if !errors.As(err, &le) || le.Code != "cancelled" || !errors.Is(err, context.Canceled) || attempts.Load() != 1 {
			t.Fatalf("err=%v attempts=%d", err, attempts.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation was not prompt")
	}
}

func TestCompleteCancellationDuringBackoff(t *testing.T) {
	var attempts atomic.Int32
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("busy")), Request: r}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewClient(Config{Endpoint: "http://example.invalid", HTTP: &http.Client{Transport: tr}, Retries: 1, RetryDelay: time.Hour}).Complete(ctx, testRequest())
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		var le *llm.LlmError
		if !errors.As(err, &le) || le.Code != "cancelled" || !errors.Is(err, context.Canceled) || attempts.Load() != 1 {
			t.Fatalf("err=%v attempts=%d", err, attempts.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("backoff cancellation was not prompt")
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	date := now.Add(3 * time.Second).UTC().Format(http.TimeFormat)
	var delays []time.Duration
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", date)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer s.Close()
	cfg := Config{Endpoint: s.URL, Retries: 1, Sleep: func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}, Now: func() time.Time { return now }}
	got, err := NewClient(cfg).Complete(context.Background(), testRequest())
	if err != nil || got.Content() != "ok" || attempts.Load() != 2 || len(delays) != 1 || delays[0] != 3*time.Second {
		t.Fatalf("date retry got=%q err=%v attempts=%d delays=%v", got.Content(), err, attempts.Load(), delays)
	}
	if got := retryAfter(date, time.Second, 0, func() time.Time { return now }); got != 3*time.Second {
		t.Fatalf("delay=%v", got)
	}
	for _, value := range []string{"not-a-date", "-1", "9223372036854775807", now.Add(-time.Second).UTC().Format(http.TimeFormat)} {
		if got := retryAfter(value, time.Second, 0, func() time.Time { return now }); got != time.Second {
			t.Fatalf("fallback for %q = %v", value, got)
		}
	}
	if got := retryAfter("0", time.Second, 0, func() time.Time { return now }); got != 0 {
		t.Fatalf("zero=%v", got)
	}
}
