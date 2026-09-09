package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestExampleConfigDocumentsAndResolvesProductionAgentSurface(t *testing.T) {
	var cfg Config
	metadata, err := toml.DecodeFile(filepath.Join("..", "..", "config.example.toml"), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"endpoint", "model", "apiKeyEnv", "timeoutSeconds", "maxCompletionTokens",
		"thinkingEnabled", "maxConcurrentRequests", "maxSteps", "maxToolCallsPerTurn",
		"toolTimeoutMillis", "maxToolCalls", "maxCallsPerTool", "maxToolFailures",
		"maxPromptChars", "maxOutputChars", "memoryTurns", "memoryTtlHours",
		"maxRetries", "retryBackoffMillis", "creatorTrip", "ambientEnabled",
		"ambientEveryMessages", "quietMinutes", "contextMessageLimit", "noReplyMarker",
		"moderationEnabled", "moderationMessageBurstCount", "moderationMessageBurstWindowSeconds",
		"moderationRepeatedMessageCount", "moderationRepeatedMessageWindowSeconds",
		"moderationSecondBreachWindowSeconds", "moderationPostKickWindowSeconds",
		"moderationJoinBurstCount", "moderationJoinBurstWindowSeconds",
		"moderationSameHashJoinCount", "moderationSameHashJoinWindowSeconds",
		"moderationSuspiciousNameJoinCount", "moderationSuspiciousNameJoinWindowSeconds",
		"moderationActionCooldownSeconds", "dynamicSqlEnabled", "dynamicSqlMaxSqlChars",
		"dynamicSqlMaxRows", "dynamicSqlMaxColumns", "dynamicSqlMaxCellChars",
		"dynamicSqlMaxResultChars", "dynamicSqlTimeoutMillis",
	} {
		if !metadata.IsDefined("agent", key) {
			t.Errorf("config.example.toml does not define agent.%s", key)
		}
	}
	for _, key := range []string{"dbPath", "wsUrl", "cmdPrefix", "channel", "nick", "trip", "userTrips", "adminTrips", "autoReconnect", "healthCheckInterval", "autorunCommands"} {
		if !metadata.IsDefined(key) {
			t.Errorf("config.example.toml does not define %s", key)
		}
	}
	cfg.Normalize()
	resolved, err := cfg.Agent.Resolve(ValueReader{})
	if err != nil {
		t.Fatalf("resolve example config: %v", err)
	}
	if cfg.DbPath != "database/database" || resolved.MemoryTurns != 30 || resolved.ContextMessageLimit != 60 || resolved.MaxPromptChars != 8000 || resolved.MaxCallsPerTool != 2 || resolved.MaxToolFailures != 2 {
		t.Fatalf("example runtime values are stale: base=%+v agent=%+v", cfg, resolved.AgentConfig)
	}
}

func TestEnvironmentExampleDocumentsEveryAgentRuntimeOverride(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]bool{}
	for _, line := range strings.Split(string(contents), "\n") {
		name, _, found := strings.Cut(line, "=")
		if found {
			defined[strings.TrimSpace(name)] = true
		}
	}
	for _, name := range []string{
		"SATURN_AGENT_CREATOR_TRIP",
		"SATURN_AGENT_QUEUE_CAPACITY",
		"SATURN_AGENT_AMBIENT_EVERY_MESSAGES",
		"SATURN_AGENT_QUIET_MINUTES",
		"SATURN_AGENT_CONTEXT_MESSAGE_LIMIT",
		"SATURN_AGENT_NO_REPLY_MARKER",
		"SATURN_AGENT_MAX_RETRIES",
		"SATURN_AGENT_RETRY_BACKOFF_MILLIS",
	} {
		if !defined[name] {
			t.Errorf(".env.example does not define %s", name)
		}
	}
}
