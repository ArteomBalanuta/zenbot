package config

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestConfigDecodesSaturnScalarListsAndBotTrip(t *testing.T) {
	var cfg Config
	_, err := toml.Decode(`
wsUrl = "wss://hack.chat/chat-ws"
nick = "korin"
trip = "bot-secret"
adminTrips = "admin-a, admin-b"
userTrips = "user-a"
autorunCommands = "replica lounge, say hello lads!!"
`, &cfg)
	if err != nil {
		t.Fatalf("decode Saturn config: %v", err)
	}
	cfg.Normalize()

	if cfg.Password != "bot-secret" {
		t.Fatalf("password=%q, want Saturn trip", cfg.Password)
	}
	assertStrings(t, "admin trips", cfg.AdminTrips, []string{"admin-a", "admin-b"})
	assertStrings(t, "user trips", cfg.UserTrips, []string{"user-a"})
	assertStrings(t, "autorun", cfg.AutorunCommands, []string{"replica lounge", "say hello lads!!"})
}

func TestConfigUsesTokenWhenFileDoesNotContainBotTrip(t *testing.T) {
	t.Setenv("TOKEN", "environment-secret")
	cfg := Config{}
	cfg.Normalize()
	if cfg.Password != "environment-secret" {
		t.Fatalf("password=%q", cfg.Password)
	}
}

func TestAgentConfigAcceptsSaturnNamesAndEnvironmentOverrides(t *testing.T) {
	var cfg Config
	_, err := toml.Decode(`
[agent]
enabled = true
endpoint = "http://file.invalid:16261"
model = ""
creatorTrip = "creator"
timeoutSeconds = 12
thinkingEnabled = true
maxCompletionTokens = 2048
maxToolCalls = 9
maxToolCallsPerTurn = 7
maxCallsPerTool = 3
maxToolFailures = 4
maxPromptChars = 9000
toolTimeoutMillis = 3456
memoryTurns = 30
memoryTtlHours = 168
ambientEnabled = true
dynamicSqlEnabled = true
dynamicSqlMaxRows = 75
moderationEnabled = true
moderationMessageBurstCount = 6
moderationMessageBurstWindowSeconds = 5
moderationRepeatedMessageCount = 4
moderationRepeatedMessageWindowSeconds = 10
moderationSecondBreachWindowSeconds = 30
moderationPostKickWindowSeconds = 600
moderationJoinBurstCount = 8
moderationJoinBurstWindowSeconds = 11
moderationSameHashJoinCount = 5
moderationSameHashJoinWindowSeconds = 21
moderationSuspiciousNameJoinCount = 6
moderationSuspiciousNameJoinWindowSeconds = 22
moderationActionCooldownSeconds = 30
`, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := cfg.Agent.Resolve(ValueReader{Environment: map[string]string{
		"SATURN_AGENT_ENDPOINT":              "http://env.invalid:16261",
		"SATURN_AGENT_MAX_COMPLETION_TOKENS": "4096",
	}})
	if err != nil {
		t.Fatalf("resolve source-compatible agent config: %v", err)
	}
	if resolved.Endpoint != "http://env.invalid:16261" || resolved.Model != "" {
		t.Fatalf("provider=%q model=%q", resolved.Endpoint, resolved.Model)
	}
	if resolved.Timeout != 12_000_000_000 || resolved.MaxTokens != 4096 || resolved.MaxTools != 7 {
		t.Fatalf("execution config=%+v", resolved.AgentConfig)
	}
	encoded, err := json.Marshal(resolved.AgentConfig)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]any{
		"ThinkingEnabled": true,
		"MaxToolCalls":    float64(9),
		"MaxCallsPerTool": float64(3),
		"MaxToolFailures": float64(4),
		"MaxPromptChars":  float64(9000),
	} {
		if got := fields[name]; got != want {
			t.Fatalf("%s=%v, want %v in %+v", name, got, want, fields)
		}
	}
	if resolved.ToolTimeoutMillis != 3456 || resolved.MemoryTTL.Hours() != 168 || !resolved.Ambient {
		t.Fatalf("memory/participation config=%+v", resolved.AgentConfig)
	}
	if !resolved.SQL.Enabled || resolved.SQL.MaxRows != 75 {
		t.Fatalf("SQL config=%+v", resolved.SQL)
	}
	if resolved.ModerationJoinWindowSeconds != 11 || resolved.ModerationSameHashCount != 5 || resolved.ModerationSameHashWindowSeconds != 21 || resolved.ModerationNameClusterCount != 6 || resolved.ModerationNameClusterWindowSeconds != 22 {
		t.Fatalf("moderation compatibility=%+v", resolved.AgentConfig)
	}
}

func TestAgentConfigAcceptsCompleteSaturnEnvironmentSurface(t *testing.T) {
	resolved, err := (AgentConfig{}).Resolve(ValueReader{Environment: map[string]string{
		"SATURN_AGENT_THINKING_ENABLED":                               "true",
		"SATURN_AGENT_MAX_TOOL_CALLS":                                 "10",
		"SATURN_AGENT_MAX_CALLS_PER_TOOL":                             "3",
		"SATURN_AGENT_MAX_TOOL_FAILURES":                              "4",
		"SATURN_AGENT_MAX_PROMPT_CHARS":                               "9001",
		"SATURN_AGENT_NO_REPLY_MARKER":                                "[[CUSTOM_SILENCE]]",
		"SATURN_AGENT_TOOL_TIMEOUT_MILLIS":                            "4321",
		"SATURN_AGENT_MEMORY_TTL_HOURS":                               "24",
		"SATURN_AGENT_DYNAMIC_SQL_ENABLED":                            "true",
		"SATURN_AGENT_DYNAMIC_SQL_MAX_ROWS":                           "77",
		"SATURN_AGENT_MODERATION_JOIN_BURST_WINDOW_SECONDS":           "12",
		"SATURN_AGENT_MODERATION_SAME_HASH_JOIN_COUNT":                "6",
		"SATURN_AGENT_MODERATION_SAME_HASH_JOIN_WINDOW_SECONDS":       "23",
		"SATURN_AGENT_MODERATION_SUSPICIOUS_NAME_JOIN_COUNT":          "7",
		"SATURN_AGENT_MODERATION_SUSPICIOUS_NAME_JOIN_WINDOW_SECONDS": "24",
	}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(resolved.AgentConfig)
	var fields map[string]any
	_ = json.Unmarshal(encoded, &fields)
	if fields["ThinkingEnabled"] != true || fields["MaxToolCalls"] != float64(10) || fields["MaxCallsPerTool"] != float64(3) || fields["MaxToolFailures"] != float64(4) || fields["MaxPromptChars"] != float64(9001) {
		t.Fatalf("execution environment aliases=%+v", fields)
	}
	if resolved.ToolTimeoutMillis != 4321 || resolved.MemoryTTL.Hours() != 24 || !resolved.SQL.Enabled || resolved.SQL.MaxRows != 77 {
		t.Fatalf("environment config=%+v", resolved)
	}
	if resolved.NoReplyMarker != "[[CUSTOM_SILENCE]]" {
		t.Fatalf("no-reply marker=%q", resolved.NoReplyMarker)
	}
	if resolved.ModerationJoinWindowSeconds != 12 || resolved.ModerationSameHashCount != 6 || resolved.ModerationSameHashWindowSeconds != 23 || resolved.ModerationNameClusterCount != 7 || resolved.ModerationNameClusterWindowSeconds != 24 {
		t.Fatalf("moderation environment aliases=%+v", resolved.AgentConfig)
	}
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s=%q, want %q", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s=%q, want %q", label, got, want)
		}
	}
}
