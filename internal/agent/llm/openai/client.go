package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zenbot/internal/agent/llm"
)

// Config controls an OpenAI-compatible endpoint. HTTP and timing hooks are injectable for tests.
type Config struct {
	Endpoint    string
	Token       string
	Model       string
	MaxTokens   int
	Temperature *float64
	Options     map[string]any
	HTTP        *http.Client
	Retries     int
	MaxRetries  int
	Timeout     time.Duration
	RetryDelay  time.Duration
	Sleep       func(context.Context, time.Duration) error
	Now         func() time.Time
}

type Client struct {
	Config
	// Legacy field names remain available for the original private scaffold.
	BaseURL string
	Token   string
	HTTP    *http.Client
	Retries int
}

func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, &llm.LlmError{Code: "validation", Err: errors.New("endpoint is required")}
	}
	u, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, &llm.LlmError{Code: "validation", Err: errors.New("endpoint must be an absolute HTTP(S) URL")}
	}
	if cfg.MaxRetries < 0 || cfg.Retries < 0 || cfg.Timeout < 0 || cfg.RetryDelay < 0 || cfg.MaxTokens < 0 {
		return nil, &llm.LlmError{Code: "validation", Err: errors.New("retry, timeout, delay, and token limits must not be negative")}
	}
	if cfg.Temperature != nil && (*cfg.Temperature < 0 || *cfg.Temperature > 2) {
		return nil, &llm.LlmError{Code: "validation", Err: errors.New("temperature must be between 0 and 2")}
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	cfg.HTTP = httpClient
	return NewClient(cfg), nil
}

func NewClient(cfg Config) *Client {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries != 0 {
		cfg.Retries = cfg.MaxRetries
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 100 * time.Millisecond
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Options = cloneMap(cfg.Options)
	return &Client{Config: cfg}
}

// Complete sends one non-streaming chat completion, retrying only transient failures.
func (c *Client) Complete(ctx context.Context, in llm.LlmRequest) (llm.LlmResponse, error) {
	cfg := c.Config
	if c.BaseURL != "" {
		cfg.Endpoint = c.BaseURL
	}
	if c.Token != "" {
		cfg.Token = c.Token
	}
	if c.HTTP != nil {
		cfg.HTTP = c.HTTP
	}
	if c.Retries != 0 {
		cfg.Retries = c.Retries
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries != 0 {
		cfg.Retries = cfg.MaxRetries
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 100 * time.Millisecond
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if cfg.Endpoint == "" {
		return llm.LlmResponse{}, &llm.LlmError{Code: "validation", Err: errors.New("endpoint is required")}
	}
	payload, err := requestPayload(cfg, in)
	if err != nil {
		return llm.LlmResponse{}, &llm.LlmError{Code: "validation", Err: err}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.LlmResponse{}, &llm.LlmError{Code: "validation", Err: err}
	}
	maxAttempts := cfg.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	opCtx, cancelOperation := context.WithTimeout(ctx, cfg.Timeout)
	defer cancelOperation()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := opCtx.Err(); err != nil {
			return llm.LlmResponse{}, contextError(err)
		}
		reqCtx, cancel := context.WithCancel(opCtx)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpointURL(cfg.Endpoint), bytes.NewReader(body))
		if err != nil {
			cancel()
			return llm.LlmResponse{}, &llm.LlmError{Code: "request", Err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		resp, doErr := cfg.HTTP.Do(req)
		if doErr != nil {
			cancel()
			if opCtx.Err() != nil {
				return llm.LlmResponse{}, contextError(opCtx.Err())
			}
			if attempt+1 < maxAttempts {
				if err := cfg.Sleep(opCtx, retryAfter("", cfg.RetryDelay, attempt, cfg.Now)); err != nil {
					return llm.LlmResponse{}, contextError(err)
				}
				continue
			}
			return llm.LlmResponse{}, &llm.LlmError{Code: "transport", Err: safeTransportError(doErr)}
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if readErr != nil {
			if attempt+1 < maxAttempts {
				if err := cfg.Sleep(opCtx, retryAfter("", cfg.RetryDelay, attempt, cfg.Now)); err != nil {
					return llm.LlmResponse{}, contextError(err)
				}
				continue
			}
			return llm.LlmResponse{}, &llm.LlmError{Code: "transport", Err: readErr}
		}
		if retryable(resp.StatusCode) && attempt+1 < maxAttempts {
			if err := cfg.Sleep(opCtx, retryAfter(resp.Header.Get("Retry-After"), cfg.RetryDelay, attempt, cfg.Now)); err != nil {
				return llm.LlmResponse{}, contextError(err)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return llm.LlmResponse{}, httpError(resp.StatusCode, data)
		}
		return decodeResponse(data)
	}
	return llm.LlmResponse{}, &llm.LlmError{Code: "transport", Err: errors.New("request attempts exhausted")}
}

func requestPayload(c Config, r llm.LlmRequest) (map[string]any, error) {
	p := cloneMap(c.Options)
	if p == nil {
		p = map[string]any{}
	}
	p["messages"] = messageJSON(r.Messages())
	p["stream"] = false
	if c.Model != "" {
		p["model"] = c.Model
	}
	if c.MaxTokens > 0 {
		p["max_tokens"] = c.MaxTokens
	}
	if c.Temperature != nil {
		p["temperature"] = *c.Temperature
	}
	if tools := r.Tools(); len(tools) > 0 {
		p["tools"] = tools
		p["tool_choice"] = "auto"
	}
	if v := r.ResponseFormat(); v != nil {
		p["response_format"] = v
	}
	if v := r.Projection(); v != nil {
		p["projection"] = v
	}
	if r.BypassPromptCache() {
		p["bypass_prompt_cache"] = true
	}
	return p, nil
}
func messageJSON(ms []llm.LlmMessage) []map[string]any {
	out := make([]map[string]any, len(ms))
	for i, m := range ms {
		x := map[string]any{"role": m.Role()}
		x["content"] = m.ContentNullable()
		if m.ToolCallID() != "" {
			x["tool_call_id"] = m.ToolCallID()
		}
		if ts := m.ToolCalls(); len(ts) > 0 {
			calls := make([]map[string]any, len(ts))
			for j, t := range ts {
				ab, _ := json.Marshal(t.Arguments())
				calls[j] = map[string]any{"id": t.ID(), "type": "function", "function": map[string]any{"name": t.Name(), "arguments": string(ab)}}
			}
			x["tool_calls"] = calls
		}
		out[i] = x
	}
	return out
}

type responseEnvelope struct {
	Choices []struct {
		Message *struct {
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]int `json:"usage"`
}

func decodeResponse(data []byte) (llm.LlmResponse, error) {
	var e responseEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return llm.LlmResponse{}, &llm.LlmError{Code: "malformed_response", Err: errors.New("invalid JSON response")}
	}
	if len(e.Choices) == 0 {
		return llm.LlmResponse{}, &llm.LlmError{Code: "malformed_response", Err: errors.New("response has no choices")}
	}
	ch := e.Choices[0]
	if ch.Message == nil {
		return llm.LlmResponse{}, &llm.LlmError{Code: "malformed_response", Err: errors.New("choice has no message")}
	}
	var content any
	if ch.Message.Content != nil {
		content = *ch.Message.Content
	}
	reason := ""
	if ch.FinishReason != nil {
		reason = *ch.FinishReason
	}
	calls := make([]llm.LlmToolCall, len(ch.Message.ToolCalls))
	for i, t := range ch.Message.ToolCalls {
		var args map[string]any
		if len(t.Function.Arguments) > 0 {
			argumentData := t.Function.Arguments
			var encoded string
			if json.Unmarshal(argumentData, &encoded) == nil {
				argumentData = json.RawMessage(encoded)
			}
			if json.Unmarshal(argumentData, &args) != nil {
				return llm.LlmResponse{}, &llm.LlmError{Code: "malformed_response", Err: errors.New("invalid tool arguments")}
			}
		}
		calls[i] = llm.NewLlmToolCall(t.ID, t.Function.Name, args)
	}
	diagnostics := map[string]any{}
	if err := json.Unmarshal(data, &diagnostics); err == nil {
		delete(diagnostics, "choices")
		delete(diagnostics, "usage")
	}
	return llm.NewLlmResponseWithMetadata(content, calls, reason, e.Usage, diagnostics), nil
}
func httpError(status int, data []byte) *llm.LlmError {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &payload)
	snippet := strings.TrimSpace(string(data))
	if len(snippet) > 256 {
		snippet = snippet[:256]
	}
	return &llm.LlmError{Code: "http", Status: status, ProviderCode: payload.Error.Code, ProviderMessage: payload.Error.Message, Snippet: snippet, Err: fmt.Errorf("upstream status %d", status)}
}
func endpointURL(s string) string { return strings.TrimRight(s, "/") + "/v1/chat/completions" }
func retryable(s int) bool {
	return s == http.StatusRequestTimeout || s == http.StatusTooManyRequests || (s >= 500 && s <= 599)
}
func retryAfter(value string, fallback time.Duration, attempt int, now func() time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds <= int64((time.Duration(1<<63-1))/time.Second) {
			return time.Duration(seconds) * time.Second
		}
	}
	if date, err := http.ParseTime(value); err == nil {
		if now == nil {
			now = time.Now
		}
		if delay := date.Sub(now()); delay > 0 {
			return delay
		}
	}
	if fallback <= 0 {
		fallback = 100 * time.Millisecond
	}
	shift := min(attempt, 20)
	if fallback > time.Duration(1<<63-1)/time.Duration(1<<shift) {
		return fallback
	}
	return fallback * time.Duration(1<<shift)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func backoff(attempt int) time.Duration {
	return retryAfter("", 100*time.Millisecond, attempt, time.Now)
}
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func contextError(err error) *llm.LlmError {
	code := "cancelled"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
	}
	return &llm.LlmError{Code: code, Err: err}
}
func safeTransportError(err error) error { return errors.New("upstream transport failure") }
func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = llm.CloneJSONValue(v)
	}
	return out
}
