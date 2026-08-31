package config

import "testing"

func TestAgentConfigStrictScalarErrorsAndDefaultAPIKey(t *testing.T) {
	for _, values := range []map[string]string{{"enabled": "sometimes"}, {"timeoutMillis": "12x"}, {"maxTokens": "999999999999999999999"}} {
		if _, err := (AgentConfig{}).Resolve(ValueReader{Runtime: values}); err == nil {
			t.Fatalf("expected parse error for %#v", values)
		}
	}
	c, err := (AgentConfig{Enabled: true, Model: "m"}).Resolve(ValueReader{Environment: map[string]string{DefaultAPIKeyEnv: "default-secret"}})
	if err != nil || c.APIKey != "default-secret" || c.APIKeyEnv != DefaultAPIKeyEnv {
		t.Fatalf("default key result=%#v err=%v", c, err)
	}
	c, err = (AgentConfig{APIKeyEnv: "CUSTOM", Enabled: true, Model: "m"}).Resolve(ValueReader{Environment: map[string]string{"CUSTOM": "custom-secret", DefaultAPIKeyEnv: "default-secret"}})
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
	c, err := (AgentConfig{APIKeyEnv: "KEY", Endpoint: "http://localhost", Model: "m", Enabled: true}).Resolve(ValueReader{Environment: map[string]string{"KEY": "secret"}})
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
