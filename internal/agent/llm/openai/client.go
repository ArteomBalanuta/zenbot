package openai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
)

// Config controls an OpenAI-compatible endpoint. HTTP and timing hooks are injectable for tests.
type Config struct {
	Endpoint        string
	Token           string
	Model           string
	MaxTokens       int
	ThinkingEnabled bool
	Temperature     *float64
	Options         map[string]any
	HTTP            *http.Client
	Retries         int
	MaxRetries      int
	Timeout         time.Duration
	RetryDelay      time.Duration
	Sleep           func(context.Context, time.Duration) error
	Now             func() time.Time
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
func (c *Client) Complete(ctx context.Context, in llm.LlmRequest) (response llm.LlmResponse, err error) {
	started := time.Now()
	attempts := 0
	payloadBytes := 0
	observability.Info(ctx, "agent.llm.request.started",
		"context_items", len(in.Messages()),
		"tool_definition_count", len(in.Tools()),
	)
	defer func() {
		attributes := []any{"duration_ms", time.Since(started).Milliseconds(), "attempt_count", attempts}
		if err != nil {
			var providerErr *llm.LlmError
			if errors.As(err, &providerErr) {
				attributes = append(attributes, "error_code", providerErr.Code, "http_status", providerErr.Status, "provider_code", providerErr.ProviderCode)
			}
			observability.Error(ctx, "agent.llm.request.failed", err, attributes...)
			return
		}
		usage := response.Usage()
		diagnostics := response.ProviderDiagnostics()
		observability.Info(ctx, "agent.llm.request.completed",
			"duration_ms", time.Since(started).Milliseconds(),
			"attempt_count", attempts,
			"finish_reason", response.FinishReason(),
			"tool_call_count", len(response.ToolCalls()),
			"output_chars", len([]rune(response.Content())),
			"reasoning_chars", diagnosticInt(diagnostics, "reasoning_chars"),
			"prompt_tokens", usage["prompt_tokens"],
			"completion_tokens", usage["completion_tokens"],
			"total_tokens", usage["total_tokens"],
			"payload_bytes", payloadBytes,
		)
	}()
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
	payloadBytes = len(body)
	maxAttempts := cfg.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	opCtx, cancelOperation := context.WithTimeout(ctx, cfg.Timeout)
	defer cancelOperation()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attempts = attempt + 1
		observability.Debug(ctx, "agent.llm.attempt.started", "attempt", attempts, "max_attempts", maxAttempts, "payload_bytes", len(body))
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
			observability.Info(ctx, "agent.llm.attempt.retrying", "attempt", attempts, "http_status", resp.StatusCode)
			if err := cfg.Sleep(opCtx, retryAfter(resp.Header.Get("Retry-After"), cfg.RetryDelay, attempt, cfg.Now)); err != nil {
				return llm.LlmResponse{}, contextError(err)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return llm.LlmResponse{}, httpError(resp.StatusCode, data)
		}
		decoded, decodeErr := decodeResponse(data)
		if decodeErr != nil {
			logMalformedResponse(ctx, data, resp.Header.Get("Content-Type"), decodeErr)
		}
		return decoded, decodeErr
	}
	return llm.LlmResponse{}, &llm.LlmError{Code: "transport", Err: errors.New("request attempts exhausted")}
}

func requestPayload(c Config, r llm.LlmRequest) (map[string]any, error) {
	p := cloneMap(c.Options)
	if p == nil {
		p = map[string]any{}
	}
	// These fields belong to this model round, never to reusable provider options.
	for _, field := range []string{"tools", "tool_choice", "parallel_tool_calls", "response_format"} {
		delete(p, field)
	}
	p["messages"] = messageJSON(r.Messages())
	p["stream"] = false
	if isOpenRouter(c.Endpoint) {
		// Local inference extensions are not part of OpenRouter's API.
		delete(p, "chat_template_kwargs")
		delete(p, "bypass_prompt_cache")
		reasoning := map[string]any{}
		if existing, ok := p["reasoning"].(map[string]any); ok {
			reasoning = cloneMap(existing)
		}
		reasoning["enabled"] = c.ThinkingEnabled
		p["reasoning"] = reasoning
	} else {
		thinkingOptions := map[string]any{}
		if existing, ok := p["chat_template_kwargs"].(map[string]any); ok {
			thinkingOptions = cloneMap(existing)
		}
		thinkingOptions["enable_thinking"] = c.ThinkingEnabled
		p["chat_template_kwargs"] = thinkingOptions
	}
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
		p["tool_choice"] = string(r.ToolChoice())
	}
	if v := r.ResponseFormat(); v != nil {
		p["response_format"] = v
	}
	if r.BypassPromptCache() && !isOpenRouter(c.Endpoint) {
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
				calls[j] = map[string]any{"id": t.ID(), "type": "function", "function": map[string]any{"name": t.Name(), "arguments": t.RawArguments()}}
			}
			x["tool_calls"] = calls
		}
		out[i] = x
	}
	return out
}

type responseEnvelope struct {
	Error   json.RawMessage `json:"error"`
	Choices []struct {
		Message *struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
}

func decodeResponse(data []byte) (llm.LlmResponse, error) {
	var e responseEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return llm.LlmResponse{}, &llm.LlmError{Code: "malformed_response", Err: fmt.Errorf("invalid JSON response: %w", err)}
	}
	if len(e.Error) > 0 && !bytes.Equal(bytes.TrimSpace(e.Error), []byte("null")) {
		// An error envelope is not a completion, even when HTTP succeeded.
		err := httpError(http.StatusOK, data)
		err.Code = "provider"
		err.Err = errors.New("upstream returned an error envelope")
		return llm.LlmResponse{}, err
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
		argumentData := string(t.Function.Arguments)
		var encoded string
		if json.Unmarshal(t.Function.Arguments, &encoded) == nil {
			argumentData = encoded
		}
		// The executor validates each call independently and returns useful
		// INVALID_ARGUMENTS feedback. Preserve malformed strings and integer
		// precision so correction sees exactly what the provider produced.
		calls[i] = llm.NewLlmToolCall(t.ID, t.Function.Name, argumentData)
	}
	diagnostics := map[string]any{}
	if err := json.Unmarshal(data, &diagnostics); err == nil {
		delete(diagnostics, "choices")
		delete(diagnostics, "usage")
	}
	if ch.Message.ReasoningContent != nil {
		diagnostics["reasoning_chars"] = len([]rune(*ch.Message.ReasoningContent))
	}
	return llm.NewLlmResponseWithMetadata(content, calls, reason, decodeUsage(e.Usage), diagnostics), nil
}

func diagnosticInt(diagnostics map[string]any, name string) int {
	switch value := diagnostics[name].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

// decodeUsage treats provider accounting as optional metadata. Providers may
// add nested detail objects without invalidating an otherwise usable response.
func decodeUsage(raw json.RawMessage) map[string]int {
	usage := map[string]int{}
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return usage
	}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var count int
		if json.Unmarshal(value, &count) == nil {
			usage[name] = count
		}
	}
	return usage
}

func logMalformedResponse(ctx context.Context, data []byte, contentType string, err error) {
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		return
	}
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	if parseErr != nil {
		mediaType = "invalid"
	}
	fingerprint := sha256.Sum256(data)
	observability.Error(ctx, "agent.llm.response.malformed", err,
		"response_bytes", len(data),
		"media_type", mediaType,
		"json_offset", syntaxErr.Offset,
		"body_sha256", fmt.Sprintf("%x", fingerprint),
	)
}
func httpError(status int, data []byte) *llm.LlmError {
	var payload struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &payload)
	var providerCode string
	if json.Unmarshal(payload.Error.Code, &providerCode) != nil {
		var numeric json.Number
		if json.Unmarshal(payload.Error.Code, &numeric) == nil {
			providerCode = numeric.String()
		}
	}
	snippet := strings.TrimSpace(string(data))
	if len(snippet) > 256 {
		snippet = snippet[:256]
	}
	return &llm.LlmError{Code: "http", Status: status, ProviderCode: providerCode, ProviderMessage: payload.Error.Message, Snippet: snippet, Err: fmt.Errorf("upstream status %d", status)}
}
func endpointURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return s // Request construction reports invalid URLs.
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
	case strings.HasSuffix(path, "/v1"):
		path += "/chat/completions"
	default:
		path += "/v1/chat/completions"
	}
	u.Path, u.RawPath = path, ""
	return u.String()
}

func isOpenRouter(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	return err == nil && strings.EqualFold(u.Hostname(), "openrouter.ai")
}
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
