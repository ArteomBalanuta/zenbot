package command

import (
	"slices"
	"testing"

	"zenbot/internal/agent/api"
)

func TestAgentCommandDefinitionsCoverCatalogWithoutRecursiveOrExcludedCommands(t *testing.T) {
	definitions := AgentCommandDefinitions()
	got := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if got[definition.Canonical] {
			t.Fatalf("duplicate agent command %q", definition.Canonical)
		}
		got[definition.Canonical] = true
	}
	for _, definition := range catalog() {
		_, excluded := map[string]bool{"l": true, "mine": true, "whiskey": true, "ws": true, "wsa": true}[definition.Canonical]
		if got[definition.Canonical] == excluded {
			t.Fatalf("agent catalog inclusion for %q = %v, excluded=%v", definition.Canonical, got[definition.Canonical], excluded)
		}
	}
}

func TestAgentRunCommandAliasesExpandFromCapabilities(t *testing.T) {
	public, _ := api.NewContext("room", "user", "", "", false, []string{})
	if got := AgentRunCommandAliases(public); !slices.Contains(got, "weather") || slices.Contains(got, "mute") || slices.Contains(got, "ban") {
		t.Fatalf("public aliases = %#v", got)
	}
	moderator, _ := api.NewContextWithCapabilities("room", "mod", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if got := AgentRunCommandAliases(moderator); !slices.Contains(got, "kick") || slices.Contains(got, "ban") {
		t.Fatalf("moderator aliases = %#v", got)
	}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands})
	if got := AgentRunCommandAliases(creator); !slices.Contains(got, "kick") || !slices.Contains(got, "ban") || slices.Contains(got, "prefix") {
		t.Fatalf("creator aliases = %#v", got)
	}
}
