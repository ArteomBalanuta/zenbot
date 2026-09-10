package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AgentConfig struct {
	Enabled                                   bool           `toml:"enabled"`
	Endpoint                                  string         `toml:"endpoint"`
	Model                                     string         `toml:"model"`
	APIKeyEnv                                 string         `toml:"apiKeyEnv"`
	TimeoutMillis                             int            `toml:"timeoutMillis"`
	RequestTimeoutMillis                      int            `toml:"requestTimeoutMillis"`
	MaxTokens                                 int            `toml:"maxTokens"`
	ThinkingEnabled                           bool           `toml:"thinkingEnabled"`
	MaxSteps                                  int            `toml:"maxSteps"`
	MaxTools                                  int            `toml:"maxTools"`
	MaxToolCalls                              int            `toml:"maxToolCalls"`
	MaxCallsPerTool                           int            `toml:"maxCallsPerTool"`
	MaxToolFailures                           int            `toml:"maxToolFailures"`
	MaxPromptChars                            int            `toml:"maxPromptChars"`
	MaxContextTokens                          int            `toml:"maxContextTokens"`
	ContextReserveTokens                      int            `toml:"contextReserveTokens"`
	MaxRetries                                int            `toml:"maxRetries"`
	RetryBackoffMillis                        int            `toml:"retryBackoffMillis"`
	Ambient                                   bool           `toml:"ambient"`
	CreatorTrip                               string         `toml:"creatorTrip"`
	AmbientEveryMessages                      int            `toml:"ambientEveryMessages"`
	QuietMinutes                              int            `toml:"quietMinutes"`
	ContextMessageLimit                       int            `toml:"contextMessageLimit"`
	MemoryTurns                               int            `toml:"memoryTurns"`
	MemoryRawTurns                            int            `toml:"memoryRawTurns"`
	MemorySummaryMaxChars                     int            `toml:"memorySummaryMaxChars"`
	MemoryTtlMinutes                          int            `toml:"memoryTtlMinutes"`
	NoReplyMarker                             string         `toml:"noReplyMarker"`
	MaxOutputChars                            int            `toml:"maxOutputChars"`
	MaxConcurrentRequests                     int            `toml:"maxConcurrentRequests"`
	QueueCapacity                             int            `toml:"queueCapacity"`
	SQL                                       AgentSqlConfig `toml:"sql"`
	ModerationEnabled                         bool           `toml:"moderationEnabled"`
	ModerationJoinBurstCount                  int            `toml:"moderationJoinBurstCount"`
	ModerationJoinWindowSeconds               int            `toml:"moderationJoinWindowSeconds"`
	ModerationJoinBurstWindowSeconds          int            `toml:"moderationJoinBurstWindowSeconds"`
	ModerationSameHashCount                   int            `toml:"moderationSameHashCount"`
	ModerationSameHashWindowSeconds           int            `toml:"moderationSameHashWindowSeconds"`
	ModerationSameHashJoinCount               int            `toml:"moderationSameHashJoinCount"`
	ModerationSameHashJoinWindowSeconds       int            `toml:"moderationSameHashJoinWindowSeconds"`
	ModerationNameClusterCount                int            `toml:"moderationNameClusterCount"`
	ModerationNameClusterWindowSeconds        int            `toml:"moderationNameClusterWindowSeconds"`
	ModerationSuspiciousNameJoinCount         int            `toml:"moderationSuspiciousNameJoinCount"`
	ModerationSuspiciousNameJoinWindowSeconds int            `toml:"moderationSuspiciousNameJoinWindowSeconds"`
	ModerationPostKickWindowSeconds           int            `toml:"moderationPostKickWindowSeconds"`
	ModerationActionCooldownSeconds           int            `toml:"moderationActionCooldownSeconds"`
	ModerationMessageBurstCount               int            `toml:"moderationMessageBurstCount"`
	ModerationMessageBurstWindowSeconds       int            `toml:"moderationMessageBurstWindowSeconds"`
	ModerationRepeatedMessageCount            int            `toml:"moderationRepeatedMessageCount"`
	ModerationRepeatedMessageWindowSeconds    int            `toml:"moderationRepeatedMessageWindowSeconds"`
	ModerationSecondBreachWindowSeconds       int            `toml:"moderationSecondBreachWindowSeconds"`
	TimeoutSeconds                            int            `toml:"timeoutSeconds"`
	MaxCompletionTokens                       int            `toml:"maxCompletionTokens"`
	MaxToolCallsPerTurn                       int            `toml:"maxToolCallsPerTurn"`
	ToolTimeoutMillis                         int            `toml:"toolTimeoutMillis"`
	MemoryTtlHours                            int            `toml:"memoryTtlHours"`
	AmbientEnabled                            bool           `toml:"ambientEnabled"`
	DynamicSQLEnabled                         bool           `toml:"dynamicSqlEnabled"`
	DynamicSQLMaxSQLChars                     int            `toml:"dynamicSqlMaxSqlChars"`
	DynamicSQLMaxRows                         int            `toml:"dynamicSqlMaxRows"`
	DynamicSQLMaxColumns                      int            `toml:"dynamicSqlMaxColumns"`
	DynamicSQLMaxCellChars                    int            `toml:"dynamicSqlMaxCellChars"`
	DynamicSQLMaxResultChars                  int            `toml:"dynamicSqlMaxResultChars"`
	DynamicSQLTimeoutMillis                   int            `toml:"dynamicSqlTimeoutMillis"`
}
type ResolvedAgentConfig struct {
	AgentConfig
	APIKey    string
	Timeout   time.Duration
	MemoryTTL time.Duration
}

const (
	defaultEndpoint                            = "http://localhost:16261"
	defaultTimeoutMillis                       = 30000
	defaultRequestTimeoutMillis                = 180000
	defaultMaxTokens                           = 1024
	defaultMaxSteps                            = 5
	defaultMaxTools                            = 4
	defaultMaxCallsPerTool                     = 2
	defaultMaxToolFailures                     = 2
	defaultMaxPromptChars                      = 8000
	defaultMaxContextTokens                    = 16000
	defaultContextReserveTokens                = 2048
	defaultMaxRetries                          = 2
	defaultRetryBackoffMillis                  = 250
	defaultToolTimeoutMillis                   = 10000
	defaultAmbientEveryMessages                = 8
	defaultQuietMinutes                        = 15
	defaultContextMessageLimit                 = 60
	defaultMemoryTurns                         = 30
	defaultMemoryRawTurns                      = 20
	defaultMemorySummaryMaxChars               = 12000
	defaultMemoryTtlMinutes                    = 10080
	defaultNoReplyMarker                       = "[[SATURN_NO_REPLY]]"
	defaultMaxOutputChars                      = 8000
	defaultMaxConcurrentRequests               = 2
	defaultModerationMessageBurstCount         = 6
	defaultModerationMessageBurstWindowSeconds = 5
	defaultModerationRepeatedMessageCount      = 4
	defaultModerationRepeatedWindowSeconds     = 10
	defaultModerationSecondBreachSeconds       = 30
	defaultModerationPostKickSeconds           = 600
	defaultModerationJoinBurstCount            = 8
	defaultModerationJoinWindowSeconds         = 10
	defaultModerationSameHashCount             = 5
	defaultModerationSameHashWindowSeconds     = 20
	defaultModerationNameClusterCount          = 5
	defaultModerationNameClusterWindowSeconds  = 20
	defaultModerationActionCooldownSeconds     = 30
	DefaultAPIKeyEnv                           = "SATURN_AGENT_API_KEY"
	maxConfigLimit                             = 1_000_000
	maxMemoryTurns                             = 60
	maxMemoryTtlMinutes                        = 525600
)

func (c AgentConfig) Validate() error {
	if c.Endpoint != "" {
		u, e := url.Parse(strings.TrimRight(c.Endpoint, "/"))
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("agent.endpoint must be an absolute HTTP(S) URL")
		}
	}
	if c.TimeoutMillis <= 0 {
		return fmt.Errorf("agent.timeoutMillis must be positive")
	}
	for name, v := range map[string]int{"maxTokens": c.MaxTokens, "maxSteps": c.MaxSteps, "maxTools": c.MaxTools, "maxToolCalls": c.MaxToolCalls, "maxCallsPerTool": c.MaxCallsPerTool, "maxToolFailures": c.MaxToolFailures, "maxPromptChars": c.MaxPromptChars, "maxContextTokens": c.MaxContextTokens, "contextReserveTokens": c.ContextReserveTokens, "maxConcurrentRequests": c.MaxConcurrentRequests, "toolTimeoutMillis": c.ToolTimeoutMillis, "requestTimeoutMillis": c.RequestTimeoutMillis} {
		if v <= 0 {
			return fmt.Errorf("agent.%s must be positive", name)
		}
		if v > maxConfigLimit {
			return fmt.Errorf("agent.%s exceeds maximum %d", name, maxConfigLimit)
		}
	}
	if c.ContextReserveTokens >= c.MaxContextTokens {
		return fmt.Errorf("agent.contextReserveTokens must be smaller than maxContextTokens")
	}
	for name, v := range map[string]int{"maxRetries": c.MaxRetries, "retryBackoffMillis": c.RetryBackoffMillis, "queueCapacity": c.QueueCapacity} {
		if v < 0 {
			return fmt.Errorf("agent.%s must not be negative", name)
		}
		if v > maxConfigLimit {
			return fmt.Errorf("agent.%s exceeds maximum %d", name, maxConfigLimit)
		}
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
		for name, v := range map[string]int{"ambientEveryMessages": c.AmbientEveryMessages, "quietMinutes": c.QuietMinutes, "contextMessageLimit": c.ContextMessageLimit} {
			if v <= 0 {
				return fmt.Errorf("agent.%s must be positive", name)
			}
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
	if c.MemoryRawTurns < 1 || c.MemoryRawTurns > c.MemoryTurns {
		return fmt.Errorf("agent.memoryRawTurns must be between 1 and memoryTurns")
	}
	if c.MemorySummaryMaxChars < 1 || c.MemorySummaryMaxChars > maxConfigLimit {
		return fmt.Errorf("agent.memorySummaryMaxChars must be between 1 and %d", maxConfigLimit)
	}
	if c.MemoryTtlMinutes < 1 || c.MemoryTtlMinutes > maxMemoryTtlMinutes {
		return fmt.Errorf("agent.memoryTtlMinutes must be between 1 and %d", maxMemoryTtlMinutes)
	}
	if c.MaxOutputChars < 1 || c.MaxOutputChars > maxConfigLimit {
		return fmt.Errorf("agent.maxOutputChars must be between 1 and %d", maxConfigLimit)
	}
	if c.MaxPromptChars < 1 || c.MaxPromptChars > maxConfigLimit {
		return fmt.Errorf("agent.maxPromptChars must be between 1 and %d", maxConfigLimit)
	}
	if c.ToolTimeoutMillis <= 0 || c.ToolTimeoutMillis > maxConfigLimit {
		return fmt.Errorf("agent.toolTimeoutMillis must be between 1 and %d", maxConfigLimit)
	}
	if err := c.SQL.Validate(); err != nil {
		return err
	}
	return nil
}
func (c AgentConfig) Resolve(r ValueReader) (ResolvedAgentConfig, error) {
	v := c
	r = withAgentEnvironmentAliases(r)
	if v.TimeoutMillis == 0 && v.TimeoutSeconds > 0 {
		v.TimeoutMillis = v.TimeoutSeconds * 1000
	}
	if v.MaxTokens == 0 && v.MaxCompletionTokens > 0 {
		v.MaxTokens = v.MaxCompletionTokens
	}
	if v.MaxTools == 0 && v.MaxToolCallsPerTurn > 0 {
		v.MaxTools = v.MaxToolCallsPerTurn
	}
	if v.MemoryTtlMinutes == 0 && v.MemoryTtlHours > 0 {
		v.MemoryTtlMinutes = v.MemoryTtlHours * 60
	}
	if v.AmbientEnabled {
		v.Ambient = true
	}
	v.applyModerationCompatibility()
	v.applyFlatSQLCompatibility()
	// Apply defaults before reading runtime values. This preserves an explicit
	// runtime zero/blank value for validation instead of silently defaulting it.
	v.applyExecutionDefaults()
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
	if v.MaxPromptChars == 0 {
		v.MaxPromptChars = defaultMaxPromptChars
	}
	if v.MaxContextTokens == 0 {
		v.MaxContextTokens = defaultMaxContextTokens
	}
	if v.ContextReserveTokens == 0 {
		v.ContextReserveTokens = defaultContextReserveTokens
	}
	if v.MaxToolCalls == 0 {
		v.MaxToolCalls = defaultMaxTools
	}
	if v.MaxCallsPerTool == 0 {
		v.MaxCallsPerTool = defaultMaxCallsPerTool
	}
	if v.MaxToolFailures == 0 {
		v.MaxToolFailures = defaultMaxToolFailures
	}
	if v.MaxConcurrentRequests == 0 {
		v.MaxConcurrentRequests = defaultMaxConcurrentRequests
	}
	if v.ToolTimeoutMillis == 0 {
		v.ToolTimeoutMillis = defaultToolTimeoutMillis
	}
	v.applySQLDefaults()
	v.applyModerationDefaults()
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
	if v.RequestTimeoutMillis, err = r.Int("requestTimeoutMillis", v.RequestTimeoutMillis); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxTokens, err = r.Int("maxTokens", v.MaxTokens); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.ThinkingEnabled, err = r.Bool("thinkingEnabled", v.ThinkingEnabled); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxSteps, err = r.Int("maxSteps", v.MaxSteps); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxTools, err = r.Int("maxTools", v.MaxTools); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxToolCalls, err = r.Int("maxToolCalls", v.MaxToolCalls); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxCallsPerTool, err = r.Int("maxCallsPerTool", v.MaxCallsPerTool); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxToolFailures, err = r.Int("maxToolFailures", v.MaxToolFailures); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxPromptChars, err = r.Int("maxPromptChars", v.MaxPromptChars); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxContextTokens, err = r.Int("maxContextTokens", v.MaxContextTokens); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.ContextReserveTokens, err = r.Int("contextReserveTokens", v.ContextReserveTokens); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MaxRetries, err = r.Int("maxRetries", v.MaxRetries); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.RetryBackoffMillis, err = r.Int("retryBackoffMillis", v.RetryBackoffMillis); err != nil {
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
	if v.MemoryRawTurns == 0 {
		v.MemoryRawTurns = min(defaultMemoryRawTurns, v.MemoryTurns)
	}
	if v.MemoryRawTurns, err = r.Int("memoryRawTurns", v.MemoryRawTurns); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if v.MemorySummaryMaxChars == 0 {
		v.MemorySummaryMaxChars = defaultMemorySummaryMaxChars
	}
	if v.MemorySummaryMaxChars, err = r.Int("memorySummaryMaxChars", v.MemorySummaryMaxChars); err != nil {
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
	if v.ToolTimeoutMillis, err = r.Int("toolTimeoutMillis", v.ToolTimeoutMillis); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if err = v.readModerationOverrides(r); err != nil {
		return ResolvedAgentConfig{}, err
	}
	if err = v.readSQLOverrides(r); err != nil {
		return ResolvedAgentConfig{}, err
	}
	v.Endpoint = strings.TrimRight(strings.TrimSpace(v.Endpoint), "/")
	if err := v.Validate(); err != nil {
		return ResolvedAgentConfig{}, err
	}
	memoryTTL := time.Duration(v.MemoryTtlMinutes) * time.Minute
	if memoryTTL <= 0 {
		return ResolvedAgentConfig{}, fmt.Errorf("agent.memoryTtlMinutes overflows duration")
	}
	apiKey := ""
	if v.APIKeyEnv != "" {
		apiKey = r.String(v.APIKeyEnv, "")
	}
	return ResolvedAgentConfig{AgentConfig: v, APIKey: apiKey, Timeout: time.Duration(v.TimeoutMillis) * time.Millisecond, MemoryTTL: memoryTTL}, nil
}

func (c *AgentConfig) applyExecutionDefaults() {
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	if c.APIKeyEnv == "" {
		c.APIKeyEnv = DefaultAPIKeyEnv
	}
	defaults := map[*int]int{
		&c.TimeoutMillis:        defaultTimeoutMillis,
		&c.RequestTimeoutMillis: defaultRequestTimeoutMillis,
		&c.MaxTokens:            defaultMaxTokens,
		&c.MaxSteps:             defaultMaxSteps,
		&c.MaxTools:             defaultMaxTools,
		&c.MaxRetries:           defaultMaxRetries,
		&c.RetryBackoffMillis:   defaultRetryBackoffMillis,
		&c.MaxContextTokens:     defaultMaxContextTokens,
		&c.ContextReserveTokens: defaultContextReserveTokens,
	}
	for destination, fallback := range defaults {
		if *destination == 0 {
			*destination = fallback
		}
	}
}

func (c *AgentConfig) applySQLDefaults() {
	defaults := map[*int]int{
		&c.SQL.MaxSQLChars:    4000,
		&c.SQL.MaxRows:        50,
		&c.SQL.MaxColumns:     32,
		&c.SQL.MaxCellChars:   2000,
		&c.SQL.MaxResultChars: 32000,
		&c.SQL.TimeoutMillis:  1000,
	}
	for destination, fallback := range defaults {
		if *destination == 0 {
			*destination = fallback
		}
	}
}

func (c *AgentConfig) applyFlatSQLCompatibility() {
	if c.DynamicSQLEnabled {
		c.SQL.Enabled = true
	}
	if c.DynamicSQLMaxSQLChars > 0 {
		c.SQL.MaxSQLChars = c.DynamicSQLMaxSQLChars
	}
	if c.DynamicSQLMaxRows > 0 {
		c.SQL.MaxRows = c.DynamicSQLMaxRows
	}
	if c.DynamicSQLMaxColumns > 0 {
		c.SQL.MaxColumns = c.DynamicSQLMaxColumns
	}
	if c.DynamicSQLMaxCellChars > 0 {
		c.SQL.MaxCellChars = c.DynamicSQLMaxCellChars
	}
	if c.DynamicSQLMaxResultChars > 0 {
		c.SQL.MaxResultChars = c.DynamicSQLMaxResultChars
	}
	if c.DynamicSQLTimeoutMillis > 0 {
		c.SQL.TimeoutMillis = c.DynamicSQLTimeoutMillis
	}
}

func (c *AgentConfig) applyModerationCompatibility() {
	if c.ModerationJoinWindowSeconds == 0 {
		c.ModerationJoinWindowSeconds = c.ModerationJoinBurstWindowSeconds
	}
	if c.ModerationSameHashCount == 0 {
		c.ModerationSameHashCount = c.ModerationSameHashJoinCount
	}
	if c.ModerationSameHashWindowSeconds == 0 {
		c.ModerationSameHashWindowSeconds = c.ModerationSameHashJoinWindowSeconds
	}
	if c.ModerationNameClusterCount == 0 {
		c.ModerationNameClusterCount = c.ModerationSuspiciousNameJoinCount
	}
	if c.ModerationNameClusterWindowSeconds == 0 {
		c.ModerationNameClusterWindowSeconds = c.ModerationSuspiciousNameJoinWindowSeconds
	}
}

func (c *AgentConfig) applyModerationDefaults() {
	defaults := map[*int]int{
		&c.ModerationMessageBurstCount:            defaultModerationMessageBurstCount,
		&c.ModerationMessageBurstWindowSeconds:    defaultModerationMessageBurstWindowSeconds,
		&c.ModerationRepeatedMessageCount:         defaultModerationRepeatedMessageCount,
		&c.ModerationRepeatedMessageWindowSeconds: defaultModerationRepeatedWindowSeconds,
		&c.ModerationSecondBreachWindowSeconds:    defaultModerationSecondBreachSeconds,
		&c.ModerationPostKickWindowSeconds:        defaultModerationPostKickSeconds,
		&c.ModerationJoinBurstCount:               defaultModerationJoinBurstCount,
		&c.ModerationJoinWindowSeconds:            defaultModerationJoinWindowSeconds,
		&c.ModerationSameHashCount:                defaultModerationSameHashCount,
		&c.ModerationSameHashWindowSeconds:        defaultModerationSameHashWindowSeconds,
		&c.ModerationNameClusterCount:             defaultModerationNameClusterCount,
		&c.ModerationNameClusterWindowSeconds:     defaultModerationNameClusterWindowSeconds,
		&c.ModerationActionCooldownSeconds:        defaultModerationActionCooldownSeconds,
	}
	for destination, fallback := range defaults {
		if *destination == 0 {
			*destination = fallback
		}
	}
}

func (c *AgentConfig) readModerationOverrides(r ValueReader) error {
	var err error
	if c.ModerationEnabled, err = r.Bool("moderationEnabled", c.ModerationEnabled); err != nil {
		return err
	}
	values := []struct {
		name string
		to   *int
	}{
		{"moderationMessageBurstCount", &c.ModerationMessageBurstCount},
		{"moderationMessageBurstWindowSeconds", &c.ModerationMessageBurstWindowSeconds},
		{"moderationRepeatedMessageCount", &c.ModerationRepeatedMessageCount},
		{"moderationRepeatedMessageWindowSeconds", &c.ModerationRepeatedMessageWindowSeconds},
		{"moderationSecondBreachWindowSeconds", &c.ModerationSecondBreachWindowSeconds},
		{"moderationPostKickWindowSeconds", &c.ModerationPostKickWindowSeconds},
		{"moderationJoinBurstCount", &c.ModerationJoinBurstCount},
		{"moderationJoinWindowSeconds", &c.ModerationJoinWindowSeconds},
		{"moderationSameHashCount", &c.ModerationSameHashCount},
		{"moderationSameHashWindowSeconds", &c.ModerationSameHashWindowSeconds},
		{"moderationNameClusterCount", &c.ModerationNameClusterCount},
		{"moderationNameClusterWindowSeconds", &c.ModerationNameClusterWindowSeconds},
		{"moderationActionCooldownSeconds", &c.ModerationActionCooldownSeconds},
	}
	for _, value := range values {
		if *value.to, err = r.Int(value.name, *value.to); err != nil {
			return err
		}
	}
	return nil
}

func (c *AgentConfig) readSQLOverrides(r ValueReader) error {
	var err error
	if c.SQL.Enabled, err = r.Bool("dynamicSqlEnabled", c.SQL.Enabled); err != nil {
		return err
	}
	values := []struct {
		name string
		to   *int
	}{
		{"dynamicSqlMaxSqlChars", &c.SQL.MaxSQLChars},
		{"dynamicSqlMaxRows", &c.SQL.MaxRows},
		{"dynamicSqlMaxColumns", &c.SQL.MaxColumns},
		{"dynamicSqlMaxCellChars", &c.SQL.MaxCellChars},
		{"dynamicSqlMaxResultChars", &c.SQL.MaxResultChars},
		{"dynamicSqlTimeoutMillis", &c.SQL.TimeoutMillis},
	}
	for _, value := range values {
		if *value.to, err = r.Int(value.name, *value.to); err != nil {
			return err
		}
	}
	return nil
}

func withAgentEnvironmentAliases(reader ValueReader) ValueReader {
	environment := make(map[string]string, len(reader.Environment)+64)
	for key, value := range reader.Environment {
		environment[key] = value
	}
	aliases := map[string]string{
		"enabled": "SATURN_AGENT_ENABLED", "endpoint": "SATURN_AGENT_ENDPOINT",
		"model": "SATURN_AGENT_MODEL", "apiKeyEnv": "SATURN_AGENT_API_KEY_ENV",
		"requestTimeoutMillis": "SATURN_AGENT_REQUEST_TIMEOUT_MILLIS",
		"thinkingEnabled":      "SATURN_AGENT_THINKING_ENABLED", "maxTokens": "SATURN_AGENT_MAX_COMPLETION_TOKENS", "maxSteps": "SATURN_AGENT_MAX_STEPS",
		"maxTools": "SATURN_AGENT_MAX_TOOL_CALLS_PER_TURN", "maxRetries": "SATURN_AGENT_MAX_RETRIES",
		"maxToolCalls": "SATURN_AGENT_MAX_TOOL_CALLS", "maxCallsPerTool": "SATURN_AGENT_MAX_CALLS_PER_TOOL",
		"maxToolFailures": "SATURN_AGENT_MAX_TOOL_FAILURES", "maxPromptChars": "SATURN_AGENT_MAX_PROMPT_CHARS",
		"maxContextTokens": "SATURN_AGENT_MAX_CONTEXT_TOKENS", "contextReserveTokens": "SATURN_AGENT_CONTEXT_RESERVE_TOKENS",
		"retryBackoffMillis": "SATURN_AGENT_RETRY_BACKOFF_MILLIS", "ambient": "SATURN_AGENT_AMBIENT_ENABLED",
		"creatorTrip": "SATURN_AGENT_CREATOR_TRIP", "ambientEveryMessages": "SATURN_AGENT_AMBIENT_EVERY_MESSAGES",
		"quietMinutes": "SATURN_AGENT_QUIET_MINUTES", "contextMessageLimit": "SATURN_AGENT_CONTEXT_MESSAGE_LIMIT",
		"memoryTurns": "SATURN_AGENT_MEMORY_TURNS", "memoryRawTurns": "SATURN_AGENT_MEMORY_RAW_TURNS",
		"memorySummaryMaxChars": "SATURN_AGENT_MEMORY_SUMMARY_MAX_CHARS", "noReplyMarker": "SATURN_AGENT_NO_REPLY_MARKER",
		"maxOutputChars":        "SATURN_AGENT_MAX_OUTPUT_CHARS",
		"maxConcurrentRequests": "SATURN_AGENT_MAX_CONCURRENT_REQUESTS", "queueCapacity": "SATURN_AGENT_QUEUE_CAPACITY",
		"toolTimeoutMillis": "SATURN_AGENT_TOOL_TIMEOUT_MILLIS", "moderationEnabled": "SATURN_AGENT_MODERATION_ENABLED",
		"moderationMessageBurstCount":            "SATURN_AGENT_MODERATION_MESSAGE_BURST_COUNT",
		"moderationMessageBurstWindowSeconds":    "SATURN_AGENT_MODERATION_MESSAGE_BURST_WINDOW_SECONDS",
		"moderationRepeatedMessageCount":         "SATURN_AGENT_MODERATION_REPEATED_MESSAGE_COUNT",
		"moderationRepeatedMessageWindowSeconds": "SATURN_AGENT_MODERATION_REPEATED_MESSAGE_WINDOW_SECONDS",
		"moderationSecondBreachWindowSeconds":    "SATURN_AGENT_MODERATION_SECOND_BREACH_WINDOW_SECONDS",
		"moderationPostKickWindowSeconds":        "SATURN_AGENT_MODERATION_POST_KICK_WINDOW_SECONDS",
		"moderationJoinBurstCount":               "SATURN_AGENT_MODERATION_JOIN_BURST_COUNT",
		"moderationJoinWindowSeconds":            "SATURN_AGENT_MODERATION_JOIN_BURST_WINDOW_SECONDS",
		"moderationSameHashCount":                "SATURN_AGENT_MODERATION_SAME_HASH_JOIN_COUNT",
		"moderationSameHashWindowSeconds":        "SATURN_AGENT_MODERATION_SAME_HASH_JOIN_WINDOW_SECONDS",
		"moderationNameClusterCount":             "SATURN_AGENT_MODERATION_SUSPICIOUS_NAME_JOIN_COUNT",
		"moderationNameClusterWindowSeconds":     "SATURN_AGENT_MODERATION_SUSPICIOUS_NAME_JOIN_WINDOW_SECONDS",
		"moderationActionCooldownSeconds":        "SATURN_AGENT_MODERATION_ACTION_COOLDOWN_SECONDS",
		"dynamicSqlEnabled":                      "SATURN_AGENT_DYNAMIC_SQL_ENABLED", "dynamicSqlMaxSqlChars": "SATURN_AGENT_DYNAMIC_SQL_MAX_SQL_CHARS",
		"dynamicSqlMaxRows": "SATURN_AGENT_DYNAMIC_SQL_MAX_ROWS", "dynamicSqlMaxColumns": "SATURN_AGENT_DYNAMIC_SQL_MAX_COLUMNS",
		"dynamicSqlMaxCellChars": "SATURN_AGENT_DYNAMIC_SQL_MAX_CELL_CHARS", "dynamicSqlMaxResultChars": "SATURN_AGENT_DYNAMIC_SQL_MAX_RESULT_CHARS",
		"dynamicSqlTimeoutMillis": "SATURN_AGENT_DYNAMIC_SQL_TIMEOUT_MILLIS",
	}
	for key, envKey := range aliases {
		if _, exists := environment[key]; exists {
			continue
		}
		if value, exists := environment[envKey]; exists {
			environment[key] = value
		}
	}
	if _, exists := environment["timeoutMillis"]; !exists {
		if value, exists := environment["SATURN_AGENT_TIMEOUT_MILLIS"]; exists {
			environment["timeoutMillis"] = value
		} else if value, exists := environment["SATURN_AGENT_TIMEOUT_SECONDS"]; exists {
			if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				environment["timeoutMillis"] = strconv.Itoa(seconds * 1000)
			} else {
				environment["timeoutMillis"] = value
			}
		}
	}
	if _, exists := environment["memoryTtlMinutes"]; !exists {
		if value, exists := environment["SATURN_AGENT_MEMORY_TTL_HOURS"]; exists {
			if hours, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				environment["memoryTtlMinutes"] = strconv.Itoa(hours * 60)
			} else {
				environment["memoryTtlMinutes"] = value
			}
		}
	}
	reader.Environment = environment
	return reader
}
