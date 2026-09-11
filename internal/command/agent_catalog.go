package command

import (
	"fmt"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
)

// AgentCommandDefinitions returns the command registry entries that may be
// exposed as agent tools. Command names and aliases remain owned by RegisterAll.
func AgentCommandDefinitions() ([]common.CommandDefinition, error) {
	if catalogError != nil {
		return nil, catalogError
	}
	definitions := make([]common.CommandDefinition, 0, len(commandcatalog.AgentEntries()))
	for _, entry := range commandcatalog.AgentEntries() {
		definition, ok := commandDefinitionFor(entry.Canonical)
		if !ok {
			return nil, fmt.Errorf("missing agent command definition %q", entry.Canonical)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
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
	return commandgateway.RequiredCapability(entry)
}

func AgentCommandAuthorized(caller api.Context, definition common.CommandDefinition) bool {
	entry, ok := commandcatalog.AgentEntry(definition.Canonical)
	return ok && commandgateway.Authorized(caller, entry)
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
