package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/llm/openai"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/config"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

// TestProviderToolLoopEvaluation is explicitly opt-in: it sends only synthetic
// conversations to the configured model. The production composition is real,
// but no database, engine, websocket, command handler, or room is constructed.
// Run from the repository root with:
// ZENBOT_PROVIDER_EVAL=1 go test ./cmd/zenbot -run TestProviderToolLoopEvaluation -count=1 -v
// ZENBOT_PROVIDER_EVAL_CONFIG optionally selects a different TOML config file.
// ZENBOT_PROVIDER_EVAL_TEMPERATURE optionally overrides only this evaluation's
// sampling temperature (0 through 2); omission preserves the server default.
func TestProviderToolLoopEvaluation(t *testing.T) {
	if os.Getenv("ZENBOT_PROVIDER_EVAL") != "1" {
		t.Skip("set ZENBOT_PROVIDER_EVAL=1 to contact the configured model using fixture-only tools")
	}
	// Suppress provider logs that might include a private endpoint on transport
	// failure. The explicit evaluation log below never includes config secrets.
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	path := os.Getenv("ZENBOT_PROVIDER_EVAL_CONFIG")
	if path == "" {
		path = "../../config.toml"
	}
	var cfg config.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatal("cannot read provider evaluation config; details withheld to avoid exposing config contents")
	}
	resolved, err := cfg.Agent.Resolve(config.ValueReader{Environment: environmentValues()})
	if err != nil {
		t.Fatal("cannot resolve provider evaluation config; details withheld to avoid exposing config contents")
	}
	// Preserve configured execution limits and provider settings except for an
	// explicit evaluation temperature override below. Use synthetic identity so
	// production account identifiers never enter the prompt.
	resolved.CreatorTrip = "provider-eval-creator"
	var temperature *float64
	temperatureLabel := "server default"
	if raw, present := os.LookupEnv("ZENBOT_PROVIDER_EVAL_TEMPERATURE"); present {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || !(value >= 0 && value <= 2) {
			t.Fatal("ZENBOT_PROVIDER_EVAL_TEMPERATURE must be a finite number from 0 through 2")
		}
		temperature = &value
		temperatureLabel = strconv.FormatFloat(value, 'f', -1, 64)
	}
	t.Logf("provider settings: configured_model=%q thinking=%t temperature=%s max_tokens=%d max_steps=%d max_tools=%d per_tool=%d failure_limit=%d context_tokens=%d reserve_tokens=%d sql=%t endpoint=[redacted] credentials=[redacted]",
		resolved.Model, resolved.ThinkingEnabled, temperatureLabel, resolved.MaxTokens, resolved.MaxSteps, resolved.MaxTools, resolved.MaxCallsPerTool, resolved.MaxToolFailures, resolved.MaxContextTokens, resolved.ContextReserveTokens, resolved.SQL.Enabled)

	tests := []struct {
		name, request string
		recovery      bool
		wantCommands  []string
		wantProduct   bool
	}{
		{"ordinary_answer", "In one short sentence, what is a prime number?", false, nil, false},
		{"exact_kick", "Kick @raider from this room now.", false, []string{"kick @raider"}, false},
		{"remote_counts_then_kick", "Get the current user counts in lounge and programming, multiply the two counts, then kick @raider from this room. Tell me the two counts, their product, and whether the kick succeeded.", false, []string{"list lounge", "list programming", "kick @raider"}, true},
		{"not_found_recovery", "First try to kick @raider. If that exact nickname is not found, inspect the current room users and kick @raider_alt instead if present. Do not kick anyone else.", true, []string{"kick @raider", "kick @raider_alt"}, false},
		{"weather_location", "Show the current weather in Tokyo.", false, []string{"weather Tokyo"}, false},
		{"ping_without_arguments", "Ping hack.chat and show the bot runtime's connection latency.", false, []string{"ping"}, false},
		{"conditional_no_kick", "Get the current user counts in lounge and programming and multiply them. Kick @raider only if the product is greater than 20. Otherwise do not kick anyone. Tell me the product and whether you kicked anyone.", false, []string{"list lounge", "list programming"}, true},
		{"conditional_kick", "Get the current user counts in lounge and programming and multiply them. Kick @raider only if the product is greater than 10. Otherwise do not kick anyone. Tell me the product and whether you kicked anyone.", false, []string{"list lounge", "list programming", "kick @raider"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &providerEvalTransport{base: http.DefaultTransport}
			provider, err := openai.New(openai.Config{
				Endpoint: resolved.Endpoint, Token: resolved.APIKey, Model: resolved.Model,
				MaxTokens: resolved.MaxTokens, ThinkingEnabled: resolved.ThinkingEnabled, Temperature: temperature,
				MaxRetries: resolved.MaxRetries, RetryDelay: time.Duration(resolved.RetryBackoffMillis) * time.Millisecond,
				Timeout: resolved.Timeout,
			}, &http.Client{Transport: transport})
			if err != nil {
				t.Fatal("could not create configured provider client")
			}
			client := &providerEvalClient{client: provider, requestText: test.request}
			catalog, err := prompt.NewCatalog(nil)
			if err != nil {
				t.Fatal(err)
			}
			policy, err := catalog.Text("system/system-policy.txt")
			if err != nil {
				t.Fatal(err)
			}
			assembler, err := assemble.New(assemble.Config{CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker, MaxPromptChars: resolved.MaxPromptChars, MaxContextTokens: resolved.MaxContextTokens, ContextReserveTokens: resolved.ContextReserveTokens}, catalog)
			if err != nil {
				t.Fatal(err)
			}
			fixture := &providerEvalFixtures{recovery: test.recovery}
			loop, err := newAgentToolLoop(resolved, fixture, assembler, catalog, client, fixture, fixture)
			if err != nil {
				t.Fatal(err)
			}
			capabilities := []runtime.Capability{runtime.ModerationCommands, runtime.PermanentBan, runtime.AdminCommands}
			if resolved.SQL.Enabled {
				capabilities = append(capabilities, runtime.DynamicSQL)
			}
			caller := runtime.NewContextWithCapabilities("evaluation", "operator", resolved.CreatorTrip, "synthetic-hash", false, []string{"operator", "raider", "friend"}, capabilities, "")
			invocation := runtime.NewInvocation("provider-eval-"+test.name, caller, test.request, runtime.DIRECT, "", true)
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(resolved.RequestTimeoutMillis)*time.Millisecond)
			defer cancel()
			started := time.Now()
			completion, completeErr := loop.CompleteWithEvidence(ctx, invocation, nil, "")
			finalizer, _ := outputFinalizer(resolved)
			var finalText string
			var shouldReply bool
			var finalErr error
			if !completion.SuppressReply {
				finalText, shouldReply, finalErr = finalizer.FinalizeWithContext(invocation, completion.Response.Content(), live.FinalizationContext{CandidateKind: completion.CandidateKind, ToolAttempted: completion.ToolAttempted})
			}
			commands, directoryCalls, historyCalls := fixture.snapshot()
			summary := map[string]any{
				"case": test.name, "request": test.request, "duration_ms": time.Since(started).Milliseconds(),
				"provider_calls": len(client.rounds), "http_attempts": transport.requests.Load(), "rounds": client.rounds,
				"commands": commands, "room_lookups": directoryCalls, "history_lookups": historyCalls,
				"answer": completion.Response.Content(), "suppressed": completion.SuppressReply,
				"tool_attempted": completion.ToolAttempted, "evidence": completion.Evidence(),
				"policy_sha256":    fmt.Sprintf("%x", sha256.Sum256([]byte(policy))),
				"thinking_enabled": resolved.ThinkingEnabled, "temperature_override": temperature,
				"final_text": finalText, "should_reply": shouldReply, "finalizer_ok": finalErr == nil,
			}
			if completeErr != nil {
				summary["error"] = providerEvalError(completeErr)
			}
			encoded, _ := json.Marshal(summary)
			t.Logf("PROVIDER_EVAL %s", encoded)
			if completeErr != nil {
				t.Fatalf("production loop failed: %s", providerEvalError(completeErr))
			}
			if strings.TrimSpace(completion.Response.Content()) == "" {
				t.Error("production loop returned no final answer")
			}
			if finalErr != nil {
				t.Error("production finalizer rejected the response")
			}
			for _, round := range client.rounds {
				if !round.RequestPresent {
					t.Error("provider round lost the exact original request")
				}
			}
			if len(commands) != len(test.wantCommands) {
				t.Errorf("commands=%q; want exactly %q", commands, test.wantCommands)
			}
			// Independent room reads may be chosen in either order. Kick must follow
			// both observations; exact nickname and no-argument ping are checked at
			// the actual command boundary, after production schema validation.
			for _, expected := range test.wantCommands {
				if !containsEvalString(commands, expected) {
					t.Errorf("missing command %q in %q", expected, commands)
				}
			}
			if test.name == "ordinary_answer" && (len(client.rounds) != 1 || completion.ToolAttempted || directoryCalls != 0 || historyCalls != 0) {
				t.Errorf("ordinary answer required extra calls/tools: provider=%d attempted=%t", len(client.rounds), completion.ToolAttempted)
			}
			if test.name == "remote_counts_then_kick" || test.name == "conditional_kick" {
				if len(commands) != 3 || !sameEvalCommand(commands[2], "kick @raider") || !evalKickAfterRoomObservations(client.rounds) {
					t.Error("kick did not follow both remote-room observations in a later model round")
				}
			}
			if test.recovery && (directoryCalls < 1 || len(commands) != 2 || !sameEvalCommand(commands[0], "kick @raider") || !sameEvalCommand(commands[1], "kick @raider_alt")) {
				t.Error("NOT_FOUND did not lead to current-room inspection and the authorized alternative nickname")
			}
			if test.wantProduct && !strings.Contains(completion.Response.Content(), "12") {
				t.Error("final answer did not include the hand-checked product 3 × 4 = 12")
			}
		})
	}
}

type providerEvalTransport struct {
	base     http.RoundTripper
	requests atomic.Int32
}

func (p *providerEvalTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.requests.Add(1)
	return p.base.RoundTrip(r)
}

type providerEvalCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type providerEvalToolResult struct {
	CallID  string          `json:"call_id"`
	Content json.RawMessage `json:"content"`
}

type providerEvalRound struct {
	ToolsOffered   int                      `json:"tools_offered"`
	ToolBytes      int                      `json:"tool_schema_bytes"`
	ToolChoice     llm.ToolChoice           `json:"tool_choice"`
	Model          string                   `json:"model,omitempty"`
	Finish         string                   `json:"finish"`
	Calls          []providerEvalCall       `json:"calls,omitempty"`
	Answer         string                   `json:"answer,omitempty"`
	Usage          map[string]int           `json:"usage,omitempty"`
	Error          string                   `json:"error,omitempty"`
	RequestPresent bool                     `json:"original_request_present"`
	ToolResults    []providerEvalToolResult `json:"tool_results,omitempty"`
	ReasoningChars any                      `json:"reasoning_chars,omitempty"`
}

type providerEvalClient struct {
	client      llm.LlmClient
	rounds      []providerEvalRound
	requestText string
}

func (c *providerEvalClient) Complete(ctx context.Context, request llm.LlmRequest) (llm.LlmResponse, error) {
	response, err := c.client.Complete(ctx, request)
	manifest, _ := json.Marshal(request.Tools())
	round := providerEvalRound{ToolsOffered: len(request.Tools()), ToolBytes: len(manifest), ToolChoice: request.ToolChoice(), Finish: response.FinishReason(), Answer: response.Content(), Usage: response.Usage()}
	round.Model, _ = response.ProviderDiagnostics()["model"].(string)
	round.ReasoningChars = response.ProviderDiagnostics()["reasoning_chars"]
	for _, message := range request.Messages() {
		if message.Role() == "user" && strings.Contains(message.Content(), c.requestText) {
			round.RequestPresent = true
		}
		if message.Role() == "tool" {
			raw := json.RawMessage(message.Content())
			if !json.Valid(raw) {
				raw, _ = json.Marshal(message.Content())
			}
			round.ToolResults = append(round.ToolResults, providerEvalToolResult{CallID: message.ToolCallID(), Content: raw})
		}
	}
	for _, call := range response.ToolCalls() {
		raw := json.RawMessage(call.RawArguments())
		if !json.Valid(raw) {
			raw, _ = json.Marshal(call.RawArguments())
		}
		round.Calls = append(round.Calls, providerEvalCall{ID: call.ID(), Name: call.Name(), Arguments: raw})
	}
	if err != nil {
		round.Error = providerEvalError(err)
	}
	c.rounds = append(c.rounds, round)
	return response, err
}

func providerEvalError(err error) string {
	var providerErr *llm.LlmError
	if errors.As(err, &providerErr) {
		return fmt.Sprintf("provider error code=%s http_status=%d", providerErr.Code, providerErr.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "evaluation request deadline exceeded"
	}
	return "non-provider loop error (details withheld; inspect locally)"
}

type providerEvalFixtures struct {
	mu             sync.Mutex
	recovery       bool
	kickNotFound   bool
	commands       []string
	directoryCalls int
	historyCalls   int
}

func (f *providerEvalFixtures) Execute(_ context.Context, _ api.Context, command, arguments string) (commandgateway.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, strings.TrimSpace(command+" "+arguments))
	if command == "kick" {
		nick := strings.TrimPrefix(arguments, "@")
		if nick == "raider" && f.recovery {
			f.kickNotFound = true
			return commandgateway.Execution{Status: commandgateway.OutcomeNotFound}, nil
		}
		if nick != "raider" && !(f.recovery && nick == "raider_alt") {
			return commandgateway.Execution{Status: commandgateway.OutcomeNotFound}, nil
		}
		return commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 1}}, nil
	}
	var message string
	var names []string
	var data json.RawMessage
	switch {
	case command == "list" && arguments == "lounge":
		names = []string{"Iris", "Moss", "River"}
	case command == "list" && arguments == "programming":
		names = []string{"Ada", "Lin", "Sam", "Terry"}
	case command == "weather" && arguments == "Tokyo":
		message = "Tokyo: clear, 24 °C, humidity 65%, wind 8 km/h."
	case command == "ping" && arguments == "":
		message = "Ping to hack.chat:80: 37 ms."
	default:
		return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
	}
	if command == "list" {
		users := make([]*model.User, len(names))
		for index, name := range names {
			users[index] = &model.User{Name: name, Hash: fmt.Sprintf("%06d", index+1)}
		}
		// Exercise the actual source formatter/data contract. Only the remote
		// snapshot and delivery are fixtures; no live room/session is created.
		result, err := snapshot.NewListRoomOperation().Apply(snapshot.RoomSnapshotContext{TargetChannel: arguments}, snapshot.Snapshot{Users: users})
		if err != nil {
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, err
		}
		message, data = result.Reply, result.Data
	}
	return commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 1}, Delivery: &commandgateway.DeliveryReceipt{Count: 1}, Messages: []string{message}, Data: data}, nil
}

func (f *providerEvalFixtures) FindRoomUsers(room string) (tool.RoomUserSnapshot, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.directoryCalls++
	users := []string{"operator", "raider", "friend"}
	// A preflight snapshot can still contain a user who leaves before a kick.
	// The first failed kick changes only this fixture's next authoritative read.
	if f.recovery && f.kickNotFound {
		users = []string{"operator", "raider_alt", "friend"}
	}
	return tool.RoomUserSnapshot{Room: room, Users: users}, room == "evaluation"
}

func (f *providerEvalFixtures) RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]repository.PublicRoomMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyCalls++
	return []repository.PublicRoomMessage{}, nil
}

func (*providerEvalFixtures) ExecuteAgentQuery(context.Context, string, json.RawMessage, string, string) (json.RawMessage, error) {
	return json.RawMessage(`{"count":0,"rows":[]}`), nil
}
func (*providerEvalFixtures) DescribeAgentSchema(context.Context) (repository.AgentDatabaseSchema, error) {
	return repository.AgentDatabaseSchema{Tables: []repository.AgentDatabaseTable{}}, nil
}
func (*providerEvalFixtures) ExecuteAgentSQL(context.Context, string, int, int, int, int) (json.RawMessage, error) {
	return json.RawMessage(`{"columns":[],"rows":[],"rowCount":0}`), nil
}
func (f *providerEvalFixtures) snapshot() ([]string, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...), f.directoryCalls, f.historyCalls
}

func containsEvalString(values []string, expected string) bool {
	for _, value := range values {
		if sameEvalCommand(value, expected) {
			return true
		}
	}
	return false
}

func sameEvalCommand(actual, expected string) bool {
	// Production Kick strips an optional @ before resolving the active nickname.
	if strings.HasPrefix(actual, "kick ") && strings.HasPrefix(expected, "kick ") {
		return strings.TrimPrefix(strings.TrimPrefix(actual, "kick "), "@") == strings.TrimPrefix(strings.TrimPrefix(expected, "kick "), "@")
	}
	return actual == expected
}

func evalKickAfterRoomObservations(rounds []providerEvalRound) bool {
	seenRooms := map[string]bool{}
	for _, round := range rounds {
		for _, call := range round.Calls {
			if call.Name == "saturn_kick" && len(seenRooms) != 2 {
				return false
			}
		}
		for _, call := range round.Calls {
			if call.Name == "saturn_list" {
				var args struct{ Room string }
				_ = json.Unmarshal(call.Arguments, &args)
				if args.Room == "lounge" || args.Room == "programming" {
					seenRooms[args.Room] = true
				}
			}
		}
	}
	return true
}
