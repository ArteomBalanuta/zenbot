package command

import (
	"zenbot/internal/common"
	"zenbot/internal/repository"
)

type invocationSurface uint8

const (
	manualInvocation invocationSurface = iota
	agentInvocation
)

// configuredCommandAvailable owns structural dependency checks for registration,
// model visibility and execution. The surface captures declared contract
// differences, such as saturn_list requiring a remote room.
func configuredCommandAvailable(engine common.Engine, canonical string, surface invocationSurface) bool {
	for {
		capture, ok := engine.(*agentCaptureEngine)
		if !ok || capture == nil {
			break
		}
		engine = capture.Engine
	}
	if !configuredDependency(engine) {
		return false
	}
	if owner, ok := engine.(common.CommandAvailability); ok && !owner.CommandAvailable(canonical) {
		return false
	}
	services := bundle(engine)
	switch canonical {
	case "ping":
		return services != nil && services.Ping != nil
	case "weather":
		return services != nil && services.Weather != nil
	case "time":
		return services != nil && services.Time != nil
	case "users":
		return services != nil && services.Users != nil && (configuredDependency(services.Users.GroupB) || configuredDependency(services.Users.Queries))
	case "nicks":
		return services != nil && services.Users != nil && configuredDependency(services.Users.Queries)
	case "lastonline":
		return services != nil && services.Users != nil && (configuredDependency(services.Users.Queries) || configuredDependency(services.Users.LastSeen))
	case "register":
		return services != nil && services.Users != nil && configuredDependency(services.Users.Identity)
	case "messages":
		return services != nil && services.Users != nil && (configuredDependency(services.Users.GroupB) || configuredDependency(services.Users.Identity))
	case "remove":
		if services == nil || services.Users == nil {
			return false
		}
		capability, ok := services.Users.GroupB.(repository.SaturnAuthorizedDeleteRepository)
		return ok && configuredDependency(capability)
	case "access":
		return services != nil && services.Security != nil && configuredDependency(services.Security.Authorization)
	case "mail":
		return services != nil && services.Mail != nil && services.Mail.DB != nil
	case "note", "notes":
		return services != nil && services.Notes != nil && services.Notes.DB != nil
	case "active":
		return services != nil && services.Activity != nil && configuredDependency(services.Activity.Repo)
	case "shadowban", "shadowbanlist", "unshadowban":
		if services == nil || services.ShadowBans == nil || !configuredDependency(services.ShadowBans.Repo) {
			return false
		}
		if canonical != "shadowban" {
			return true
		}
		_, ok := engine.(common.ModerationOperations)
		return ok
	case "captcha", "authorize", "deauthorize", "lock", "overflow", "ban", "kick", "unban", "unbanall", "mute", "unmute", "color", "flair":
		_, ok := engine.(common.ModerationOperations)
		return ok
	case "list", "msgchannel":
		if surface == manualInvocation {
			return true
		}
		_, ok := engine.(common.CredentialedRoomSnapshotSubmitter)
		return ok
	case "nuke":
		_, ok := engine.(common.CredentialedRoomSnapshotSubmitter)
		return ok
	case "resurrect":
		_, mover := engine.(common.LiveRoomMover)
		_, submitter := engine.(common.CredentialedRoomSnapshotSubmitter)
		return mover && submitter
	case "prefix":
		_, ok := engine.(common.PrefixController)
		return ok
	case "replica", "replicaoff", "replicastatus":
		_, ok := engine.(ReplicaController)
		return ok
	case "ws", "wsa":
		_, ok := engine.(common.SupportReplicaRelay)
		return ok
	case "restart", "shutdown":
		return configuredDependency(lifecycleController(engine))
	case "sql":
		return services != nil && configuredDependency(services.SQLCommand)
	default:
		return true
	}
}

func configuredDependency(value any) bool {
	return common.DependencyConfigured(value)
}
