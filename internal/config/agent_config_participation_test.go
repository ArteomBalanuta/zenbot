package config

import (
	"strings"
	"testing"
)

func TestAgentConfigParticipationDefaults(t *testing.T) {
	got, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatorTrip != "595754" || got.AmbientEveryMessages != 8 || got.QuietMinutes != 15 || got.ContextMessageLimit != 60 || got.NoReplyMarker != "[[SATURN_NO_REPLY]]" || got.MaxOutputChars != 8000 || got.MaxConcurrentRequests != 1 || got.QueueCapacity != 0 {
		t.Fatalf("participation defaults: %#v", got)
	}
}

func TestAgentConfigOutputBoundRuntimeOverrideAndValidation(t *testing.T) {
	got, err := (AgentConfig{MaxOutputChars: 4}).Resolve(ValueReader{Runtime: map[string]string{"maxOutputChars": "9"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxOutputChars != 9 {
		t.Fatalf("maxOutputChars = %d, want 9", got.MaxOutputChars)
	}
	for _, value := range []string{"0", "-1", "1000001"} {
		t.Run(value, func(t *testing.T) {
			_, err := (AgentConfig{}).Resolve(ValueReader{Runtime: map[string]string{"maxOutputChars": value}})
			if err == nil || !strings.Contains(err.Error(), "maxOutputChars") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAgentConfigParticipationValidation(t *testing.T) {
	for name, c := range map[string]AgentConfig{
		"creatorTrip":           {Enabled: true, Model: "m", CreatorTrip: " "},
		"noReplyMarker":         {Enabled: true, Model: "m", NoReplyMarker: " "},
		"ambientEveryMessages":  {Enabled: true, Model: "m", AmbientEveryMessages: -1},
		"quietMinutes":          {Enabled: true, Model: "m", QuietMinutes: -1},
		"contextMessageLimit":   {Enabled: true, Model: "m", ContextMessageLimit: -1},
		"maxConcurrentRequests": {Enabled: true, Model: "m", MaxConcurrentRequests: -1},
		"queueCapacity":         {Enabled: true, Model: "m", QueueCapacity: -1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Resolve(ValueReader{})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAgentConfigParticipationRuntimeInvalidValuesAreNotDefaulted(t *testing.T) {
	for name, value := range map[string]string{
		"creatorTrip":           " ",
		"noReplyMarker":         " ",
		"ambientEveryMessages":  "0",
		"quietMinutes":          "0",
		"contextMessageLimit":   "0",
		"maxConcurrentRequests": "0",
		"queueCapacity":         "-1",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (AgentConfig{Enabled: true, Model: "m"}).Resolve(ValueReader{Runtime: map[string]string{name: value}})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAgentConfigParticipationRuntimeOverrides(t *testing.T) {
	got, err := (AgentConfig{CreatorTrip: "file", AmbientEveryMessages: 1, QuietMinutes: 1, ContextMessageLimit: 1, NoReplyMarker: "file", MaxConcurrentRequests: 2, QueueCapacity: 3}).Resolve(ValueReader{Runtime: map[string]string{
		"creatorTrip": "runtime", "ambientEveryMessages": "9", "quietMinutes": "10", "contextMessageLimit": "11", "noReplyMarker": "runtime-marker", "maxConcurrentRequests": "12", "queueCapacity": "13",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatorTrip != "runtime" || got.AmbientEveryMessages != 9 || got.QuietMinutes != 10 || got.ContextMessageLimit != 11 || got.NoReplyMarker != "runtime-marker" || got.MaxConcurrentRequests != 12 || got.QueueCapacity != 13 {
		t.Fatalf("resolved=%#v", got)
	}
}
