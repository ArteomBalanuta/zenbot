package config

import (
	"strings"
	"testing"
)

func TestAgentModerationRequiresMessageDetectionSettings(t *testing.T) {
	resolved, err := (AgentConfig{}).Resolve(ValueReader{})
	if err != nil {
		t.Fatal(err)
	}
	c := resolved.AgentConfig
	c.ModerationEnabled = true
	c.ModerationMessageBurstCount = 0
	c.ModerationMessageBurstWindowSeconds = 0
	c.ModerationRepeatedMessageCount = 0
	c.ModerationRepeatedMessageWindowSeconds = 0
	c.ModerationSecondBreachWindowSeconds = 0
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "moderation") {
		t.Fatalf("missing message settings error=%v", err)
	}
	c.ModerationMessageBurstCount, c.ModerationMessageBurstWindowSeconds, c.ModerationRepeatedMessageCount, c.ModerationRepeatedMessageWindowSeconds, c.ModerationSecondBreachWindowSeconds = 1, 1, 1, 1, 1
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
