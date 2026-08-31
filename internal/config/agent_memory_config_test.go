package config

import "testing"

func TestAgentMemoryConfigurationResolvesBoundedDefaultsAndOverrides(t *testing.T) {
	defaults, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.MemoryTurns != 6 || defaults.MemoryTtlMinutes != 1440 {
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

func TestAgentMemoryConfigurationRejectsOutOfRangeValues(t *testing.T) {
	for _, values := range []map[string]string{
		{"memoryTurns": "-1"}, {"memoryTurns": "61"},
		{"memoryTtlMinutes": "-1"}, {"memoryTtlMinutes": "525601"},
	} {
		if _, err := (AgentConfig{}).Resolve(ValueReader{Runtime: values}); err == nil {
			t.Fatalf("expected error for %#v", values)
		}
	}
}
