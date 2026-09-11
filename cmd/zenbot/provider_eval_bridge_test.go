package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm/openai"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/config"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type capabilityLimits struct {
	MaxSteps         int `json:"max_steps"`
	MaxTools         int `json:"max_tools"`
	MaxCallsPerTool  int `json:"max_calls_per_tool"`
	MaxToolFailures  int `json:"max_tool_failures"`
	MaxContextTokens int `json:"max_context_tokens"`
}

type capabilityOutcome struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type capabilityFixtureInput struct {
	CurrentUsers []string                     `json:"current_users"`
	Rooms        map[string][]string          `json:"rooms"`
	Weather      map[string]capabilityOutcome `json:"weather"`
	Ping         capabilityOutcome            `json:"ping"`
	History      map[string][]string          `json:"history"`
	HistoryError []string                     `json:"history_error"`
	Kick         map[string][]string          `json:"kick"`
}

type capabilityCaseInput struct {
	Name           string                 `json:"name"`
	Request        string                 `json:"request"`
	Endpoint       string                 `json:"endpoint"`
	Fixture        capabilityFixtureInput `json:"fixture"`
	RequiredLimits capabilityLimits       `json:"required_limits"`
}

type capabilityHistoryCall struct {
	Room  string `json:"room"`
	Nick  string `json:"nick"`
	Limit int    `json:"limit"`
}

type capabilityFixtures struct {
	mu           sync.Mutex
	input        capabilityFixtureInput
	commands     []string
	directory    int
	historyCalls []capabilityHistoryCall
	kickCalls    map[string]int
}

func TestAgentCapabilityBridge(t *testing.T) {
	raw := os.Getenv("ZENBOT_CAPABILITY_CASE_JSON")
	if raw == "" {
		t.Skip("test-only bridge; invoke through tests/agent_capabilities")
	}
	var testCase capabilityCaseInput
	if err := json.Unmarshal([]byte(raw), &testCase); err != nil || strings.TrimSpace(testCase.Name) == "" || strings.TrimSpace(testCase.Request) == "" {
		emitCapabilityResult(map[string]any{"configuration_error": "invalid capability case JSON"})
		return
	}

	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	resolved, configError := capabilityConfig(testCase)
	if configError != "" {
		emitCapabilityResult(map[string]any{"case": testCase.Name, "configuration_error": configError})
		return
	}
	if shortfall := capabilityLimitShortfall(resolved, testCase.RequiredLimits); shortfall != "" {
		emitCapabilityResult(map[string]any{"case": testCase.Name, "configuration_error": shortfall})
		return
	}

	provider, err := openai.New(openai.Config{
		Endpoint: resolved.Endpoint, Token: resolved.APIKey, Model: resolved.Model,
		MaxTokens: resolved.MaxTokens, ThinkingEnabled: resolved.ThinkingEnabled,
		MaxRetries: resolved.MaxRetries, RetryDelay: time.Duration(resolved.RetryBackoffMillis) * time.Millisecond,
		Timeout: resolved.Timeout,
	}, nil)
	if err != nil {
		emitCapabilityResult(map[string]any{"case": testCase.Name, "configuration_error": "provider client configuration is invalid"})
		return
	}
	client := &providerEvalClient{client: provider, requestText: testCase.Request}
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := catalog.Text("system/system-policy.txt")
	if err != nil {
		t.Fatal(err)
	}
	assembler, err := assemble.New(assemble.Config{
		CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker,
		MaxPromptChars: resolved.MaxPromptChars, MaxContextTokens: resolved.MaxContextTokens,
		ContextReserveTokens: resolved.ContextReserveTokens,
	}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &capabilityFixtures{input: testCase.Fixture, kickCalls: map[string]int{}}
	loop, err := newAgentToolLoop(resolved, fixture, assembler, catalog, client, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	caller := runtime.NewContextWithCapabilities("evaluation", "operator", resolved.CreatorTrip, "synthetic-hash", false, testCase.Fixture.CurrentUsers,
		[]runtime.Capability{runtime.ModerationCommands, runtime.PermanentBan, runtime.AdminCommands}, "")
	invocation := runtime.NewInvocation("capability-"+testCase.Name, caller, testCase.Request, runtime.DIRECT, "", true)
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
	result := map[string]any{
		"case": testCase.Name, "duration_ms": time.Since(started).Milliseconds(),
		"provider_calls": len(client.rounds), "rounds": client.rounds,
		"commands": commands, "room_lookups": directoryCalls, "history_lookups": historyCalls,
		"answer": completion.Response.Content(), "suppressed": completion.SuppressReply,
		"tool_attempted": completion.ToolAttempted, "evidence": completion.Evidence(),
		"final_text": finalText, "should_reply": shouldReply, "finalizer_ok": finalErr == nil,
		"model": resolved.Model, "thinking_enabled": resolved.ThinkingEnabled,
		"policy_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(policy))),
		"limits":        map[string]int{"max_steps": resolved.MaxSteps, "max_tools": resolved.MaxTools, "max_calls_per_tool": resolved.MaxCallsPerTool, "max_tool_failures": resolved.MaxToolFailures, "max_context_tokens": resolved.MaxContextTokens},
	}
	if completeErr != nil {
		result["error"] = providerEvalError(completeErr)
	}
	emitCapabilityResult(result)
}

func capabilityConfig(testCase capabilityCaseInput) (config.ResolvedAgentConfig, string) {
	if path := os.Getenv("ZENBOT_CAPABILITY_PROVIDER_CONFIG"); path != "" {
		var cfg config.Config
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return config.ResolvedAgentConfig{}, "cannot read explicit provider config; details withheld"
		}
		resolved, err := cfg.Agent.Resolve(config.ValueReader{Environment: environmentValues()})
		if err != nil {
			return config.ResolvedAgentConfig{}, "cannot resolve explicit provider config; details withheld"
		}
		resolved.CreatorTrip = "capability-eval-creator"
		return resolved, ""
	}
	if strings.TrimSpace(testCase.Endpoint) == "" {
		return config.ResolvedAgentConfig{}, "offline case endpoint is required"
	}
	resolved, err := (config.AgentConfig{
		Enabled: true, Endpoint: testCase.Endpoint, Model: "scripted-capability-model", CreatorTrip: "capability-eval-creator",
		MaxSteps: 20, MaxTools: 20, MaxCallsPerTool: 20, MaxToolFailures: 8,
		MaxContextTokens: 64000, ContextReserveTokens: 2048,
		TimeoutMillis: 5000, RequestTimeoutMillis: 20000, MaxRetries: 0,
	}).Resolve(config.ValueReader{Environment: map[string]string{}})
	if err != nil {
		return config.ResolvedAgentConfig{}, "offline capability configuration is invalid"
	}
	return resolved, ""
}

func capabilityLimitShortfall(resolved config.ResolvedAgentConfig, required capabilityLimits) string {
	checks := []struct {
		name     string
		actual   int
		required int
	}{
		{"maxSteps", resolved.MaxSteps, required.MaxSteps},
		{"maxTools", resolved.MaxTools, required.MaxTools},
		{"maxCallsPerTool", resolved.MaxCallsPerTool, required.MaxCallsPerTool},
		{"maxToolFailures", resolved.MaxToolFailures, required.MaxToolFailures},
		{"maxContextTokens", resolved.MaxContextTokens, required.MaxContextTokens},
	}
	for _, check := range checks {
		if check.required > 0 && check.actual < check.required {
			return fmt.Sprintf("configured %s=%d is below case requirement %d", check.name, check.actual, check.required)
		}
	}
	return ""
}

func emitCapabilityResult(value map[string]any) {
	encoded, _ := json.Marshal(value)
	fmt.Printf("CAPABILITY_RESULT %s\n", encoded)
}

func (f *capabilityFixtures) Execute(_ context.Context, _ api.Context, command, arguments string) (commandgateway.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	arguments = strings.TrimSpace(arguments)
	f.commands = append(f.commands, strings.TrimSpace(command+" "+arguments))
	switch command {
	case "list":
		names, found := f.input.Rooms[arguments]
		if !found {
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
		}
		users := make([]*model.User, len(names))
		for index, name := range names {
			users[index] = &model.User{Name: name, Hash: fmt.Sprintf("%06d", index+1)}
		}
		result, err := snapshot.NewListRoomOperation().Apply(snapshot.RoomSnapshotContext{TargetChannel: arguments}, snapshot.Snapshot{Users: users})
		if err != nil {
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, err
		}
		return capabilityReadExecution(result.Reply, result.Data), nil
	case "weather":
		outcome, found := f.input.Weather[arguments]
		if !found || strings.EqualFold(outcome.Status, "error") {
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
		}
		return capabilityReadExecution(outcome.Message, nil), nil
	case "ping":
		if arguments != "" || strings.EqualFold(f.input.Ping.Status, "error") {
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
		}
		message := f.input.Ping.Message
		if message == "" {
			message = "Ping to hack.chat:80: 37 ms."
		}
		return capabilityReadExecution(message, nil), nil
	case "kick":
		nick := strings.TrimPrefix(arguments, "@")
		index := f.kickCalls[nick]
		f.kickCalls[nick] = index + 1
		outcomes, configured := f.input.Kick[nick]
		status := "not_found"
		if index < len(outcomes) {
			status = strings.ToLower(outcomes[index])
		} else if configured || containsCapabilityUser(f.input.CurrentUsers, nick) {
			status = "succeeded"
		}
		switch status {
		case "not_found":
			return commandgateway.Execution{Status: commandgateway.OutcomeNotFound}, nil
		case "rejected":
			return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
		case "unknown":
			return commandgateway.Execution{Status: commandgateway.OutcomeUnknown}, nil
		default:
			return commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 1}}, nil
		}
	default:
		return commandgateway.Execution{Status: commandgateway.OutcomeRejected}, nil
	}
}

func containsCapabilityUser(users []string, nick string) bool {
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user), nick) {
			return true
		}
	}
	return false
}

func capabilityReadExecution(message string, data json.RawMessage) commandgateway.Execution {
	return commandgateway.Execution{
		Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true,
		Action: &commandgateway.ActionReceipt{Count: 1}, Delivery: &commandgateway.DeliveryReceipt{Count: 1},
		Messages: []string{message}, Data: data, DataObserved: len(data) > 0,
	}
}

func (f *capabilityFixtures) FindRoomUsers(room string) (tool.RoomUserSnapshot, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.directory++
	if room != "evaluation" {
		return tool.RoomUserSnapshot{}, false
	}
	return tool.RoomUserSnapshot{Room: room, Users: append([]string(nil), f.input.CurrentUsers...)}, true
}

func (f *capabilityFixtures) RecentPublicRoomMessagesForNick(_ context.Context, room, nick string, limit int) ([]repository.PublicRoomMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyCalls = append(f.historyCalls, capabilityHistoryCall{Room: room, Nick: nick, Limit: limit})
	for _, failedNick := range f.input.HistoryError {
		if strings.EqualFold(failedNick, nick) {
			return nil, fmt.Errorf("fixture history failure")
		}
	}
	messages := f.input.History[nick]
	rows := make([]repository.PublicRoomMessage, 0, len(messages))
	for index, message := range messages {
		rows = append(rows, repository.PublicRoomMessage{Name: nick, Message: message, Channel: "evaluation", CreatedOnMillis: int64(index + 1)})
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (*capabilityFixtures) ExecuteAgentQuery(context.Context, string, json.RawMessage, string, string) (json.RawMessage, error) {
	return json.RawMessage(`{"count":0,"rows":[]}`), nil
}
func (*capabilityFixtures) DescribeAgentSchema(context.Context) (repository.AgentDatabaseSchema, error) {
	return repository.AgentDatabaseSchema{Tables: []repository.AgentDatabaseTable{}}, nil
}
func (*capabilityFixtures) ExecuteAgentSQL(context.Context, string, int, int, int, int) (json.RawMessage, error) {
	return json.RawMessage(`{"columns":[],"rows":[],"rowCount":0}`), nil
}

func (f *capabilityFixtures) snapshot() ([]string, int, []capabilityHistoryCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...), f.directory, append([]capabilityHistoryCall(nil), f.historyCalls...)
}
