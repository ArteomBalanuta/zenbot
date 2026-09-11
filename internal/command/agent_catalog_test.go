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
	if got := AgentRunCommandAliases(moderator); !slices.Contains(got, "mute") || slices.Contains(got, "kick") || slices.Contains(got, "ban") {
		t.Fatalf("moderator aliases = %#v", got)
	}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands})
	if got := AgentRunCommandAliases(creator); slices.Contains(got, "kick") || !slices.Contains(got, "ban") || slices.Contains(got, "prefix") {
		t.Fatalf("creator aliases = %#v", got)
	}
}

func TestAgentModerationReviewPolicyAllowsOnlyMute(t *testing.T) {
	caller, err := api.NewContextWithModerationTarget(
		"room", "bot", "creator-trip", "", false, []string{"reviewed"},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands}, "reviewed",
	)
	if err != nil {
		t.Fatal(err)
	}
	if aliases := AgentRunCommandAliases(caller); !slices.Equal(aliases, []string{"dumb", "mute"}) {
		t.Fatalf("review legacy aliases=%#v", aliases)
	}
	for _, canonical := range []string{"nuke", "lock", "notes", "register", "mail", "shadowban", "kick"} {
		definition, ok := AgentCommandDefinition(canonical)
		if !ok {
			t.Fatalf("missing definition %q", canonical)
		}
		if AgentCommandAuthorized(caller, definition) {
			t.Errorf("review authorized %q", canonical)
		}
	}
	mute, ok := AgentCommandDefinition("mute")
	if !ok || !AgentCommandAuthorized(caller, mute) {
		t.Fatal("review did not authorize mute")
	}
}
