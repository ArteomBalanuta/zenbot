package command

import (
	"zenbot/internal/agent/api"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
)

// AgentCommandDefinitions returns the command registry entries that may be
// exposed as agent tools. Command names and aliases remain owned by RegisterAll.
func AgentCommandDefinitions() []common.CommandDefinition {
	definitions := make([]common.CommandDefinition, 0, len(commandcatalog.AgentEntries()))
	for _, entry := range commandcatalog.AgentEntries() {
		definition, ok := commandDefinitionFor(entry.Canonical)
		if ok {
			definition.Aliases = append([]string(nil), definition.Aliases...)
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

// AgentCommandDefinition resolves one canonical command from the reviewed
// agent catalog. Aliases are deliberately not accepted at this contract seam.
func AgentCommandDefinition(canonical string) (common.CommandDefinition, bool) {
	entry, ok := commandcatalog.AgentEntry(canonical)
	if !ok {
		return common.CommandDefinition{}, false
	}
	return commandDefinitionFor(entry.Canonical)
}

// AgentCommandCapability returns the capability required in addition to the
// trusted invocation context. Public command tools have no extra capability.
func AgentCommandCapability(definition common.CommandDefinition) (api.Capability, bool) {
	entry, ok := commandcatalog.AgentEntry(definition.Canonical)
	if !ok {
		return "", true
	}
	switch commandcatalog.Access(entry) {
	case commandcatalog.AgentPermanentBan:
		return api.PermanentBan, true
	case commandcatalog.AgentAdmin:
		return api.AdminCommands, true
	case commandcatalog.AgentModerator:
		return api.ModerationCommands, true
	default:
		return "", false
	}
}

func AgentCommandAuthorized(caller api.Context, definition common.CommandDefinition) bool {
	if caller.ModerationTarget() != nil && !commandcatalog.ModerationReviewAllows(definition.Canonical) {
		return false
	}
	required, restricted := AgentCommandCapability(definition)
	return !restricted || caller.HasCapability(required)
}

// AgentRunCommandAliases derives the compact compatibility tool's enum from
// the same command catalog and the caller's trusted capabilities.
func AgentRunCommandAliases(caller api.Context) []string {
	if caller.ModerationTarget() != nil {
		return commandcatalog.ModerationReviewRunCommandAliases()
	}
	return commandcatalog.RunCommandAliases(caller.HasCapability(api.ModerationCommands), caller.HasCapability(api.PermanentBan))
}

func AgentCommandTargetsUser(canonical string) bool {
	return commandcatalog.TargetsUser(canonical)
}
