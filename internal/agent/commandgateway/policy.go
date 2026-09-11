package commandgateway

import (
	"strings"

	"zenbot/internal/agent/api"
	commandcatalog "zenbot/internal/command/catalog"
)

// RequiredCapability is shared by command admission and tool descriptors.
func RequiredCapability(definition commandcatalog.Entry) (api.Capability, bool) {
	switch commandcatalog.Access(definition) {
	case commandcatalog.AgentAdmin:
		return api.AdminCommands, true
	case commandcatalog.AgentModerator:
		return api.ModerationCommands, true
	case commandcatalog.AgentPermanentBan:
		return api.PermanentBan, true
	default:
		return "", false
	}
}

func Authorized(caller api.Context, definition commandcatalog.Entry) bool {
	if caller.ModerationTarget() != nil && !commandcatalog.ModerationReviewAllows(definition.Canonical) {
		return false
	}
	required, restricted := RequiredCapability(definition)
	return !restricted || caller.HasCapability(required)
}

// TargetAllowed enforces the reviewed-author boundary without selecting intent.
func TargetAllowed(caller api.Context, command, arguments string) bool {
	target := caller.ModerationTarget()
	if target == nil || !commandcatalog.TargetsUser(command) {
		return true
	}
	fields := strings.Fields(arguments)
	if len(fields) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimPrefix(fields[0], "@"), *target)
}
