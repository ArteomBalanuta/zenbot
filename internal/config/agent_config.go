package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type AgentConfig struct {
	Enabled       bool           `toml:"enabled"`
	Endpoint      string         `toml:"endpoint"`
	Model         string         `toml:"model"`
	APIKeyEnv     string         `toml:"apiKeyEnv"`
	TimeoutMillis int            `toml:"timeoutMillis"`
	MaxTokens     int            `toml:"maxTokens"`
	MaxSteps      int            `toml:"maxSteps"`
	MaxTools      int            `toml:"maxTools"`
	Ambient       bool           `toml:"ambient"`
	SQL           AgentSqlConfig `toml:"sql"`
}
type ResolvedAgentConfig struct {
	AgentConfig
	APIKey  string
	Timeout time.Duration
}

const (
	defaultEndpoint      = "http://localhost:16261"
	defaultTimeoutMillis = 30000
	defaultMaxTokens     = 1024
	defaultMaxSteps      = 5
	defaultMaxTools      = 4
	DefaultAPIKeyEnv     = "SATURN_AGENT_API_KEY"
	maxConfigLimit       = 1_000_000
)

func (c AgentConfig) Validate() error {
	if c.Endpoint != "" {
		u, e := url.Parse(strings.TrimRight(c.Endpoint, "/"))
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("agent.endpoint must be an absolute HTTP(S) URL")
		}
	}
	if c.TimeoutMillis < 0 {
		return fmt.Errorf("agent.timeoutMillis must not be negative")
	}
	for name, v := range map[string]int{"maxTokens": c.MaxTokens, "maxSteps": c.MaxSteps, "maxTools": c.MaxTools} {
		if v < 0 {
			return fmt.Errorf("agent.%s must not be negative", name)
		}
		if v > maxConfigLimit {
			return fmt.Errorf("agent.%s exceeds maximum %d", name, maxConfigLimit)
		}
	}
	if c.Enabled && strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("agent.model is required when agent is enabled")
	}
	if c.Enabled && strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("agent.endpoint is required when agent is enabled")
	}
	return nil
}
func (c AgentConfig) Resolve(r ValueReader) (ResolvedAgentConfig, error) {
	v := c
	var err error
	if v.Enabled, err = r.Bool("enabled", v.Enabled); err != nil {
		return ResolvedAgentConfig{}, err
	}
	v.Endpoint = r.String("endpoint", v.Endpoint)
	v.Model = r.String("model", v.Model)
	v.APIKeyEnv = r.String("apiKeyEnv", v.APIKeyEnv)
	if v.TimeoutMillis, err = r.Int("timeoutMillis", v.TimeoutMillis); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxTokens, err = r.Int("maxTokens", v.MaxTokens); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxSteps, err = r.Int("maxSteps", v.MaxSteps); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxTools, err = r.Int("maxTools", v.MaxTools); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.Ambient, err = r.Bool("ambient", v.Ambient); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.Endpoint == "" {
		v.Endpoint = defaultEndpoint
	}
	v.Endpoint = strings.TrimRight(strings.TrimSpace(v.Endpoint), "/")
	if v.TimeoutMillis == 0 {
		v.TimeoutMillis = defaultTimeoutMillis
	}
	if v.MaxTokens == 0 {
		v.MaxTokens = defaultMaxTokens
	}
	if v.MaxSteps == 0 {
		v.MaxSteps = defaultMaxSteps
	}
	if v.MaxTools == 0 {
		v.MaxTools = defaultMaxTools
	}
	if err := v.Validate(); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.TimeoutMillis <= 0 {
		return ResolvedAgentConfig{}, fmt.Errorf("agent.timeoutMillis must be positive")
	}
	if v.APIKeyEnv == "" {
		v.APIKeyEnv = DefaultAPIKeyEnv
	}
	return ResolvedAgentConfig{AgentConfig: v, APIKey: r.String(v.APIKeyEnv, ""), Timeout: time.Duration(v.TimeoutMillis) * time.Millisecond}, nil
}
