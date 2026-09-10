package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultProfilingAddress             = "0.0.0.0:6060"
	defaultSlowCommandThresholdMillis   = 250
	defaultSlowStageThresholdMillis     = 25
	defaultSlowTransportThresholdMillis = 25
	defaultBlockProfileRate             = 1_000_000
	defaultMutexProfileFraction         = 5
	maxProfilingThresholdMillis         = 3_600_000
	maxBlockProfileRate                 = 1_000_000_000
	maxMutexProfileFraction             = 1_000_000
)

// ProfilingConfig controls opt-in regular-command tracing and Go runtime profiles.
type ProfilingConfig struct {
	Enabled                      bool   `toml:"enabled"`
	ListenAddress                string `toml:"listenAddress"`
	SlowCommandThresholdMillis   int    `toml:"slowCommandThresholdMillis"`
	SlowStageThresholdMillis     int    `toml:"slowStageThresholdMillis"`
	SlowTransportThresholdMillis int    `toml:"slowTransportThresholdMillis"`
	BlockProfileRate             int    `toml:"blockProfileRate"`
	MutexProfileFraction         int    `toml:"mutexProfileFraction"`
}

// ResolvedProfilingConfig contains validated durations used by runtime composition.
type ResolvedProfilingConfig struct {
	ProfilingConfig
	SlowCommandThreshold   time.Duration
	SlowStageThreshold     time.Duration
	SlowTransportThreshold time.Duration
}

// Resolve applies defaults and environment overrides, then validates all bounds.
func (c ProfilingConfig) Resolve(reader ValueReader) (ResolvedProfilingConfig, error) {
	if strings.TrimSpace(c.ListenAddress) == "" {
		c.ListenAddress = defaultProfilingAddress
	}
	if c.SlowCommandThresholdMillis == 0 {
		c.SlowCommandThresholdMillis = defaultSlowCommandThresholdMillis
	}
	if c.SlowStageThresholdMillis == 0 {
		c.SlowStageThresholdMillis = defaultSlowStageThresholdMillis
	}
	if c.SlowTransportThresholdMillis == 0 {
		c.SlowTransportThresholdMillis = defaultSlowTransportThresholdMillis
	}
	if c.BlockProfileRate == 0 {
		c.BlockProfileRate = defaultBlockProfileRate
	}
	if c.MutexProfileFraction == 0 {
		c.MutexProfileFraction = defaultMutexProfileFraction
	}
	reader = withProfilingEnvironmentAliases(reader)
	var err error
	if c.Enabled, err = reader.Bool("enabled", c.Enabled); err != nil {
		return ResolvedProfilingConfig{}, err
	}
	c.ListenAddress = strings.TrimSpace(reader.String("listenAddress", c.ListenAddress))
	for _, value := range []struct {
		name string
		to   *int
	}{
		{"slowCommandThresholdMillis", &c.SlowCommandThresholdMillis},
		{"slowStageThresholdMillis", &c.SlowStageThresholdMillis},
		{"slowTransportThresholdMillis", &c.SlowTransportThresholdMillis},
		{"blockProfileRate", &c.BlockProfileRate},
		{"mutexProfileFraction", &c.MutexProfileFraction},
	} {
		if *value.to, err = reader.Int(value.name, *value.to); err != nil {
			return ResolvedProfilingConfig{}, err
		}
	}
	if err := c.Validate(); err != nil {
		return ResolvedProfilingConfig{}, err
	}
	return ResolvedProfilingConfig{
		ProfilingConfig:        c,
		SlowCommandThreshold:   time.Duration(c.SlowCommandThresholdMillis) * time.Millisecond,
		SlowStageThreshold:     time.Duration(c.SlowStageThresholdMillis) * time.Millisecond,
		SlowTransportThreshold: time.Duration(c.SlowTransportThresholdMillis) * time.Millisecond,
	}, nil
}

// Validate rejects unusable listener addresses and unsafe profiler bounds.
func (c ProfilingConfig) Validate() error {
	_, portText, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return fmt.Errorf("profiling.listenAddress must be host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("profiling.listenAddress port must be between 1 and 65535")
	}
	for name, value := range map[string]int{
		"slowCommandThresholdMillis":   c.SlowCommandThresholdMillis,
		"slowStageThresholdMillis":     c.SlowStageThresholdMillis,
		"slowTransportThresholdMillis": c.SlowTransportThresholdMillis,
	} {
		if value < 1 || value > maxProfilingThresholdMillis {
			return fmt.Errorf("profiling.%s must be between 1 and %d", name, maxProfilingThresholdMillis)
		}
	}
	if c.BlockProfileRate < 0 || c.BlockProfileRate > maxBlockProfileRate {
		return fmt.Errorf("profiling.blockProfileRate must be between 0 and %d", maxBlockProfileRate)
	}
	if c.MutexProfileFraction < 0 || c.MutexProfileFraction > maxMutexProfileFraction {
		return fmt.Errorf("profiling.mutexProfileFraction must be between 0 and %d", maxMutexProfileFraction)
	}
	return nil
}

func withProfilingEnvironmentAliases(reader ValueReader) ValueReader {
	environment := make(map[string]string, len(reader.Environment)+7)
	for key, value := range reader.Environment {
		environment[key] = value
	}
	aliases := map[string]string{
		"enabled":                      "ZENBOT_PROFILING_ENABLED",
		"listenAddress":                "ZENBOT_PROFILING_LISTEN_ADDRESS",
		"slowCommandThresholdMillis":   "ZENBOT_PROFILING_SLOW_COMMAND_THRESHOLD_MILLIS",
		"slowStageThresholdMillis":     "ZENBOT_PROFILING_SLOW_STAGE_THRESHOLD_MILLIS",
		"slowTransportThresholdMillis": "ZENBOT_PROFILING_SLOW_TRANSPORT_THRESHOLD_MILLIS",
		"blockProfileRate":             "ZENBOT_PROFILING_BLOCK_PROFILE_RATE",
		"mutexProfileFraction":         "ZENBOT_PROFILING_MUTEX_PROFILE_FRACTION",
	}
	for key, environmentKey := range aliases {
		if _, exists := environment[key]; exists {
			continue
		}
		if value, exists := environment[environmentKey]; exists {
			environment[key] = value
		}
	}
	reader.Environment = environment
	return reader
}
