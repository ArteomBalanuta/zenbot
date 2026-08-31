package config

import (
	"strings"
	"testing"
)

func TestAgentModerationRequiresMessageDetectionSettings(t *testing.T) {
	c := AgentConfig{MemoryTurns: 1, MemoryTtlMinutes: 1, MaxOutputChars: 1, ModerationEnabled: true, ModerationJoinBurstCount: 1, ModerationJoinWindowSeconds: 1, ModerationSameHashCount: 1, ModerationSameHashWindowSeconds: 1, ModerationNameClusterCount: 1, ModerationNameClusterWindowSeconds: 1, ModerationPostKickWindowSeconds: 1, ModerationActionCooldownSeconds: 1}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "moderation") {
		t.Fatalf("missing message settings error=%v", err)
	}
	c.ModerationMessageBurstCount, c.ModerationMessageBurstWindowSeconds, c.ModerationRepeatedMessageCount, c.ModerationRepeatedMessageWindowSeconds, c.ModerationSecondBreachWindowSeconds = 1, 1, 1, 1, 1
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
