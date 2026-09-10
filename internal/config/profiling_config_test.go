package config

import (
	"testing"
	"time"
)

func TestProfilingConfigResolvesSafeDefaultsAndEnvironmentOverrides(t *testing.T) {
	defaults, err := (ProfilingConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Enabled || defaults.ListenAddress != "0.0.0.0:6060" || defaults.SlowCommandThreshold != 250*time.Millisecond || defaults.SlowStageThreshold != 25*time.Millisecond || defaults.SlowTransportThreshold != 25*time.Millisecond || defaults.BlockProfileRate != 1_000_000 || defaults.MutexProfileFraction != 5 {
		t.Fatalf("profiling defaults=%+v", defaults)
	}

	overridden, err := (ProfilingConfig{}).Resolve(ValueReader{Environment: map[string]string{
		"ZENBOT_PROFILING_ENABLED":                         "true",
		"ZENBOT_PROFILING_LISTEN_ADDRESS":                  "127.0.0.1:7070",
		"ZENBOT_PROFILING_SLOW_COMMAND_THRESHOLD_MILLIS":   "400",
		"ZENBOT_PROFILING_SLOW_STAGE_THRESHOLD_MILLIS":     "40",
		"ZENBOT_PROFILING_SLOW_TRANSPORT_THRESHOLD_MILLIS": "30",
		"ZENBOT_PROFILING_BLOCK_PROFILE_RATE":              "2000000",
		"ZENBOT_PROFILING_MUTEX_PROFILE_FRACTION":          "7",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !overridden.Enabled || overridden.ListenAddress != "127.0.0.1:7070" || overridden.SlowCommandThreshold != 400*time.Millisecond || overridden.SlowStageThreshold != 40*time.Millisecond || overridden.SlowTransportThreshold != 30*time.Millisecond || overridden.BlockProfileRate != 2_000_000 || overridden.MutexProfileFraction != 7 {
		t.Fatalf("profiling overrides=%+v", overridden)
	}
}

func TestProfilingConfigRejectsInvalidRuntimeBounds(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"listen address":      {"listenAddress": "not-an-address"},
		"command threshold":   {"slowCommandThresholdMillis": "0"},
		"stage threshold":     {"slowStageThresholdMillis": "-1"},
		"transport threshold": {"slowTransportThresholdMillis": "0"},
		"block rate":          {"blockProfileRate": "-1"},
		"mutex fraction":      {"mutexProfileFraction": "-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (ProfilingConfig{}).Resolve(ValueReader{Runtime: values}); err == nil {
				t.Fatalf("Resolve(%v) succeeded", values)
			}
		})
	}
}
