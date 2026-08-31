package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type AgentConfig struct {
	Enabled                                bool           `toml:"enabled"`
	Endpoint                               string         `toml:"endpoint"`
	Model                                  string         `toml:"model"`
	APIKeyEnv                              string         `toml:"apiKeyEnv"`
	TimeoutMillis                          int            `toml:"timeoutMillis"`
	MaxTokens                              int            `toml:"maxTokens"`
	MaxSteps                               int            `toml:"maxSteps"`
	MaxTools                               int            `toml:"maxTools"`
	Ambient                                bool           `toml:"ambient"`
	CreatorTrip                            string         `toml:"creatorTrip"`
	AmbientEveryMessages                   int            `toml:"ambientEveryMessages"`
	QuietMinutes                           int            `toml:"quietMinutes"`
	ContextMessageLimit                    int            `toml:"contextMessageLimit"`
	MemoryTurns                            int            `toml:"memoryTurns"`
	MemoryTtlMinutes                       int            `toml:"memoryTtlMinutes"`
	NoReplyMarker                          string         `toml:"noReplyMarker"`
	MaxOutputChars                         int            `toml:"maxOutputChars"`
	MaxConcurrentRequests                  int            `toml:"maxConcurrentRequests"`
	QueueCapacity                          int            `toml:"queueCapacity"`
	SQL                                    AgentSqlConfig `toml:"sql"`
	ModerationEnabled                      bool           `toml:"moderationEnabled"`
	ModerationJoinBurstCount               int            `toml:"moderationJoinBurstCount"`
	ModerationJoinWindowSeconds            int            `toml:"moderationJoinWindowSeconds"`
	ModerationSameHashCount                int            `toml:"moderationSameHashCount"`
	ModerationSameHashWindowSeconds        int            `toml:"moderationSameHashWindowSeconds"`
	ModerationNameClusterCount             int            `toml:"moderationNameClusterCount"`
	ModerationNameClusterWindowSeconds     int            `toml:"moderationNameClusterWindowSeconds"`
	ModerationPostKickWindowSeconds        int            `toml:"moderationPostKickWindowSeconds"`
	ModerationActionCooldownSeconds        int            `toml:"moderationActionCooldownSeconds"`
	ModerationMessageBurstCount            int            `toml:"moderationMessageBurstCount"`
	ModerationMessageBurstWindowSeconds    int            `toml:"moderationMessageBurstWindowSeconds"`
	ModerationRepeatedMessageCount         int            `toml:"moderationRepeatedMessageCount"`
	ModerationRepeatedMessageWindowSeconds int            `toml:"moderationRepeatedMessageWindowSeconds"`
	ModerationSecondBreachWindowSeconds    int            `toml:"moderationSecondBreachWindowSeconds"`
}
type ResolvedAgentConfig struct {
	AgentConfig
	APIKey    string
	Timeout   time.Duration
	MemoryTTL time.Duration
}

const (
	defaultEndpoint              = "http://localhost:16261"
	defaultTimeoutMillis         = 30000
	defaultMaxTokens             = 1024
	defaultMaxSteps              = 5
	defaultMaxTools              = 4
	defaultCreatorTrip           = "595754"
	defaultAmbientEveryMessages  = 8
	defaultQuietMinutes          = 15
	defaultContextMessageLimit   = 60
	defaultMemoryTurns           = 6
	defaultMemoryTtlMinutes      = 1440
	defaultNoReplyMarker         = "[[SATURN_NO_REPLY]]"
	defaultMaxOutputChars        = 8000
	defaultMaxConcurrentRequests = 1
	DefaultAPIKeyEnv             = "SATURN_AGENT_API_KEY"
	maxConfigLimit               = 1_000_000
	maxMemoryTurns               = 60
	maxMemoryTtlMinutes          = 525600
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
	if c.Enabled {
		if strings.TrimSpace(c.CreatorTrip) == "" {
			return fmt.Errorf("agent.creatorTrip must not be blank")
		}
		if strings.TrimSpace(c.NoReplyMarker) == "" {
			return fmt.Errorf("agent.noReplyMarker must not be blank")
		}
		for name, v := range map[string]int{"ambientEveryMessages": c.AmbientEveryMessages, "quietMinutes": c.QuietMinutes, "contextMessageLimit": c.ContextMessageLimit, "maxConcurrentRequests": c.MaxConcurrentRequests} {
			if v <= 0 {
				return fmt.Errorf("agent.%s must be positive", name)
			}
		}
		if c.QueueCapacity < 0 {
			return fmt.Errorf("agent.queueCapacity must not be negative")
		}
	}
	if c.ModerationEnabled {
		for name, v := range map[string]int{"moderationJoinBurstCount": c.ModerationJoinBurstCount, "moderationJoinWindowSeconds": c.ModerationJoinWindowSeconds, "moderationSameHashCount": c.ModerationSameHashCount, "moderationSameHashWindowSeconds": c.ModerationSameHashWindowSeconds, "moderationNameClusterCount": c.ModerationNameClusterCount, "moderationNameClusterWindowSeconds": c.ModerationNameClusterWindowSeconds, "moderationPostKickWindowSeconds": c.ModerationPostKickWindowSeconds, "moderationActionCooldownSeconds": c.ModerationActionCooldownSeconds, "moderationMessageBurstCount": c.ModerationMessageBurstCount, "moderationMessageBurstWindowSeconds": c.ModerationMessageBurstWindowSeconds, "moderationRepeatedMessageCount": c.ModerationRepeatedMessageCount, "moderationRepeatedMessageWindowSeconds": c.ModerationRepeatedMessageWindowSeconds, "moderationSecondBreachWindowSeconds": c.ModerationSecondBreachWindowSeconds} {
			if v <= 0 {
				return fmt.Errorf("agent.%s must be positive when moderationEnabled", name)
			}
		}
	}
	if c.MemoryTurns < 1 || c.MemoryTurns > maxMemoryTurns {
		return fmt.Errorf("agent.memoryTurns must be between 1 and %d", maxMemoryTurns)
	}
	if c.MemoryTtlMinutes < 1 || c.MemoryTtlMinutes > maxMemoryTtlMinutes {
		return fmt.Errorf("agent.memoryTtlMinutes must be between 1 and %d", maxMemoryTtlMinutes)
	}
	if c.MaxOutputChars < 1 || c.MaxOutputChars > maxConfigLimit {
		return fmt.Errorf("agent.maxOutputChars must be between 1 and %d", maxConfigLimit)
	}
	return nil
}
func (c AgentConfig) Resolve(r ValueReader) (ResolvedAgentConfig, error) {
	v := c
	// Apply defaults before reading runtime values. This preserves an explicit
	// runtime zero/blank value for validation instead of silently defaulting it.
	if v.CreatorTrip == "" {
		v.CreatorTrip = defaultCreatorTrip
	}
	if v.AmbientEveryMessages == 0 {
		v.AmbientEveryMessages = defaultAmbientEveryMessages
	}
	if v.QuietMinutes == 0 {
		v.QuietMinutes = defaultQuietMinutes
	}
	if v.ContextMessageLimit == 0 {
		v.ContextMessageLimit = defaultContextMessageLimit
	}
	if v.MemoryTurns == 0 {
		v.MemoryTurns = defaultMemoryTurns
	}
	if v.MemoryTtlMinutes == 0 {
		v.MemoryTtlMinutes = defaultMemoryTtlMinutes
	}
	if v.NoReplyMarker == "" {
		v.NoReplyMarker = defaultNoReplyMarker
	}
	if v.MaxOutputChars == 0 {
		v.MaxOutputChars = defaultMaxOutputChars
	}
	if v.MaxConcurrentRequests == 0 {
		v.MaxConcurrentRequests = defaultMaxConcurrentRequests
	}
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
	v.CreatorTrip = r.String("creatorTrip", v.CreatorTrip)
	if v.AmbientEveryMessages, err = r.Int("ambientEveryMessages", v.AmbientEveryMessages); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.QuietMinutes, err = r.Int("quietMinutes", v.QuietMinutes); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.ContextMessageLimit, err = r.Int("contextMessageLimit", v.ContextMessageLimit); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MemoryTurns, err = r.Int("memoryTurns", v.MemoryTurns); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MemoryTtlMinutes, err = r.Int("memoryTtlMinutes", v.MemoryTtlMinutes); err != nil {
		return ResolvedAgentConfig{}, err
	}
	v.NoReplyMarker = r.String("noReplyMarker", v.NoReplyMarker)
	if v.MaxOutputChars, err = r.Int("maxOutputChars", v.MaxOutputChars); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxConcurrentRequests, err = r.Int("maxConcurrentRequests", v.MaxConcurrentRequests); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.QueueCapacity, err = r.Int("queueCapacity", v.QueueCapacity); err != nil {
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
	memoryTTL := time.Duration(v.MemoryTtlMinutes) * time.Minute
	if memoryTTL <= 0 {
		return ResolvedAgentConfig{}, fmt.Errorf("agent.memoryTtlMinutes overflows duration")
	}
	return ResolvedAgentConfig{AgentConfig: v, APIKey: r.String(v.APIKeyEnv, ""), Timeout: time.Duration(v.TimeoutMillis) * time.Millisecond, MemoryTTL: memoryTTL}, nil
}
