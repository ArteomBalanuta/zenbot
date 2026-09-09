package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestConfigAcceptsSaturnWsUrlAndNickKeys(t *testing.T) {
	var c Config
	if _, err := toml.Decode(`wsUrl = "wss://hack.chat/chat-ws"
nick = "alphaBot"
channel = "programming"
cmdPrefix = "*"`, &c); err != nil {
		t.Fatal(err)
	}
	c.Normalize()
	if c.WebsocketUrl != "wss://hack.chat/chat-ws" || c.Name != "alphaBot" {
		t.Fatalf("config=%+v", c)
	}
}

func TestAgentSQLConfigurationPreservesSaturnBounds(t *testing.T) {
	resolved, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SQL.MaxSQLChars != 4000 || resolved.SQL.MaxRows != 50 || resolved.SQL.MaxColumns != 32 || resolved.SQL.MaxCellChars != 2000 || resolved.SQL.MaxResultChars != 32000 || resolved.SQL.TimeoutMillis != 1000 {
		t.Fatalf("sql=%+v", resolved.SQL)
	}
}

func TestAgentSQLConfigurationRejectsExplicitZeroBounds(t *testing.T) {
	for _, name := range []string{
		"dynamicSqlMaxSqlChars",
		"dynamicSqlMaxRows",
		"dynamicSqlMaxColumns",
		"dynamicSqlMaxCellChars",
		"dynamicSqlMaxResultChars",
		"dynamicSqlTimeoutMillis",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (AgentConfig{}).Resolve(ValueReader{Runtime: map[string]string{name: "0"}})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("explicit zero %s error=%v", name, err)
			}
		})
	}
}

func TestAgentConfigStrictScalarErrorsAndDefaultAPIKey(t *testing.T) {
	for _, values := range []map[string]string{{"enabled": "sometimes"}, {"timeoutMillis": "12x"}, {"maxTokens": "999999999999999999999"}} {
		if _, err := (AgentConfig{}).Resolve(ValueReader{Runtime: values}); err == nil {
			t.Fatalf("expected parse error for %#v", values)
		}
	}
	c, err := (AgentConfig{Enabled: true, Model: "m", CreatorTrip: "creator"}).Resolve(ValueReader{Environment: map[string]string{DefaultAPIKeyEnv: "default-secret"}})
	if err != nil || c.APIKey != "default-secret" || c.APIKeyEnv != DefaultAPIKeyEnv {
		t.Fatalf("default key result=%#v err=%v", c, err)
	}
	c, err = (AgentConfig{APIKeyEnv: "CUSTOM", Enabled: true, Model: "m", CreatorTrip: "creator"}).Resolve(ValueReader{Environment: map[string]string{"CUSTOM": "custom-secret", DefaultAPIKeyEnv: "default-secret"}})
	if err != nil || c.APIKey != "custom-secret" {
		t.Fatalf("override result=%#v err=%v", c, err)
	}
}

func TestValueReaderPrecedenceAndExplicitSecretLookup(t *testing.T) {
	r := ValueReader{Runtime: map[string]string{"endpoint": "runtime"}, Environment: map[string]string{"endpoint": "environment"}, File: map[string]string{"endpoint": "file"}}
	if got := r.String("endpoint", "default"); got != "runtime" {
		t.Fatal(got)
	}
	if got := (ValueReader{Environment: map[string]string{"endpoint": "environment"}, File: map[string]string{"endpoint": "file"}}).String("endpoint", "default"); got != "environment" {
		t.Fatal(got)
	}
	c, err := (AgentConfig{APIKeyEnv: "KEY", Endpoint: "http://localhost", Model: "m", Enabled: true, CreatorTrip: "creator"}).Resolve(ValueReader{Environment: map[string]string{"KEY": "secret"}})
	if err != nil || c.APIKey != "secret" {
		t.Fatalf("resolved=%#v err=%v", c, err)
	}
}

func TestAgentConfigDefaultsAndValidation(t *testing.T) {
	c, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Endpoint != "http://localhost:16261" || c.TimeoutMillis != 30000 || c.MaxTokens != 1024 || c.MaxSteps != 5 || c.MaxTools != 4 {
		t.Fatalf("defaults=%#v", c)
	}
	for _, bad := range []AgentConfig{{Endpoint: "file:///tmp/x"}, {TimeoutMillis: -1}, {MaxTokens: -1}, {MaxSteps: -1}, {MaxTools: -1}} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("expected invalid config: %#v", bad)
		}
	}
}

func TestValueReaderRejectsMalformedScalarsAtEverySource(t *testing.T) {
	for _, source := range []struct {
		name string
		make func(string) ValueReader
	}{
		{"runtime", func(v string) ValueReader {
			return ValueReader{Runtime: map[string]string{"enabled": v, "maxTokens": v}}
		}},
		{"environment", func(v string) ValueReader {
			return ValueReader{Environment: map[string]string{"enabled": v, "maxTokens": v}}
		}},
		{"file", func(v string) ValueReader { return ValueReader{File: map[string]string{"enabled": v, "maxTokens": v}} }},
	} {
		t.Run(source.name, func(t *testing.T) {
			r := source.make("not-a-scalar")
			if _, err := r.Bool("enabled", false); err == nil {
				t.Fatal("malformed bool was accepted")
			}
			if _, err := r.Int("maxTokens", 1); err == nil {
				t.Fatal("malformed int was accepted")
			}
		})
	}
}

func TestAgentConfigRejectsExplicitZeroSafetyBounds(t *testing.T) {
	for _, name := range []string{
		"timeoutMillis",
		"maxTokens",
		"maxSteps",
		"maxTools",
		"maxToolCalls",
		"maxCallsPerTool",
		"maxToolFailures",
		"maxPromptChars",
		"maxConcurrentRequests",
		"toolTimeoutMillis",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (AgentConfig{Enabled: true, Model: "test", CreatorTrip: "creator"}).Resolve(ValueReader{Runtime: map[string]string{name: "0"}})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("explicit zero %s error=%v", name, err)
			}
		})
	}
}

func TestAgentConfigAllowsZeroRetryCountAndBackoff(t *testing.T) {
	resolved, err := (AgentConfig{}).Resolve(ValueReader{Runtime: map[string]string{
		"maxRetries":         "0",
		"retryBackoffMillis": "0",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.MaxRetries != 0 || resolved.RetryBackoffMillis != 0 {
		t.Fatalf("retry settings=%+v", resolved.AgentConfig)
	}
}
