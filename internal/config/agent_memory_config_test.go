package config

import "testing"

func TestAgentMemoryConfigurationResolvesBoundedDefaultsAndOverrides(t *testing.T) {
	defaults, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.MemoryTurns != 30 || defaults.MemoryRawTurns != 20 || defaults.MemorySummaryMaxChars != 12000 || defaults.MemoryTtlMinutes != 10080 {
		t.Fatalf("defaults: %#v", defaults.AgentConfig)
	}

	explicit, err := (AgentConfig{MemoryTurns: 2, MemoryTtlMinutes: 3}).Resolve(ValueReader{Runtime: map[string]string{"memoryTurns": "4", "memoryTtlMinutes": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.MemoryTurns != 4 || explicit.MemoryTtlMinutes != 5 {
		t.Fatalf("runtime overrides: %#v", explicit.AgentConfig)
	}
}

func TestAgentMemoryCompactionConfigurationUsesEnvironmentAliases(t *testing.T) {
	resolved, err := (AgentConfig{}).Resolve(ValueReader{Environment: map[string]string{
		"SATURN_AGENT_MEMORY_RAW_TURNS":         "12",
		"SATURN_AGENT_MEMORY_SUMMARY_MAX_CHARS": "9000",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.MemoryRawTurns != 12 || resolved.MemorySummaryMaxChars != 9000 {
		t.Fatalf("resolved=%#v", resolved.AgentConfig)
	}
}

func TestAgentMemoryConfigurationRejectsOutOfRangeValues(t *testing.T) {
	for _, values := range []map[string]string{
		{"memoryTurns": "-1"}, {"memoryTurns": "61"},
		{"memoryTtlMinutes": "-1"}, {"memoryTtlMinutes": "525601"},
		{"memoryRawTurns": "0"}, {"memoryRawTurns": "31"},
		{"memorySummaryMaxChars": "0"}, {"memorySummaryMaxChars": "1000001"},
	} {
		if _, err := (AgentConfig{}).Resolve(ValueReader{Runtime: values}); err == nil {
			t.Fatalf("expected error for %#v", values)
		}
	}
}
