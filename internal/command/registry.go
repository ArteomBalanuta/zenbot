package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type saturnCommand struct {
	engine    common.Engine
	message   *model.ChatMessage
	role      model.Role
	aliases   []string
	canonical string
}

type replicaCommand struct{ commandBase }
type replicaOffCommand struct{ commandBase }
type replicaStatusCommand struct{ commandBase }

func (c *replicaCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	controller, ok := c.engine.(ReplicaController)
	if !ok {
		return model.FAILED, fmt.Errorf("replica controller is not configured")
	}
	channel, err := ParseReplicaChannel(c.message.GetArguments())
	if err != nil {
		if hasBlankReplicaChannel(c.message.Text, c.message.GetArguments()) {
			if err := replyContext(ctx, &c.commandBase, "I'm the host bot serving current channel. Example: "+c.engine.GetPrefix()+"replica lounge"); err != nil {
				return model.FAILED, err
			}
		} else {
			if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"replica lounge"); err != nil {
				return model.FAILED, err
			}
		}
		return model.FAILED, nil
	}
	if channel == c.engine.GetChannel() {
		if err := replyContext(ctx, &c.commandBase, "I'm the host bot serving current channel. Example: "+c.engine.GetPrefix()+"replica lounge"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	for _, existing := range controller.ReplicaChannels() {
		if existing == channel {
			if err := replyContext(ctx, &c.commandBase, "Channel "+channel+" already has a replica running."); err != nil {
				return model.FAILED, err
			}
			return model.FAILED, nil
		}
	}
	if err := controller.AddReplica(ctx, channel); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, fmt.Sprintf("started replica in channel: %s successfully. Number of replicas: %d", channel, len(controller.ReplicaChannels()))); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
func (c *replicaOffCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	controller, ok := c.engine.(ReplicaController)
	if !ok {
		return model.FAILED, fmt.Errorf("replica controller is not configured")
	}
	channel, err := ParseReplicaChannel(c.message.GetArguments())
	if err != nil {
		if hasBlankReplicaChannel(c.message.Text, c.message.GetArguments()) {
			if err := replyContext(ctx, &c.commandBase, "I'm the host bot serving current channel, not a replica."); err != nil {
				return model.FAILED, err
			}
		} else {
			if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"replicaoff lounge"); err != nil {
				return model.FAILED, err
			}
		}
		return model.FAILED, nil
	}
	if channel == c.engine.GetChannel() {
		if err := replyContext(ctx, &c.commandBase, "I'm the host bot serving current channel, not a replica."); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	found := false
	for _, existing := range controller.ReplicaChannels() {
		if existing == channel {
			found = true
			break
		}
	}
	if !found {
		if err := replyContext(ctx, &c.commandBase, "No replica in channel: "+channel); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if err := controller.RemoveReplica(ctx, channel); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, "Successfully shut down replica in channel: "+channel); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
func (c *replicaStatusCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	controller, ok := c.engine.(ReplicaController)
	if !ok {
		return model.FAILED, fmt.Errorf("replica controller is not configured")
	}
	channels := append([]string(nil), controller.ReplicaChannels()...)
	sort.Strings(channels)
	serving := strings.Join(channels, ", ")
	if serving == "" {
		serving = "none"
	}
	text := fmt.Sprintf("Host room:%s, replicas active: %d \\nServing channels: %s", c.engine.GetChannel(), len(channels), serving)
	if err := observeAndReply(ctx, &c.commandBase, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *replicaCommand) Role() model.Role        { return c.role }
func (c *replicaOffCommand) Role() model.Role     { return c.role }
func (c *replicaStatusCommand) Role() model.Role  { return c.role }
func (c *replicaCommand) Aliases() []string       { return append([]string(nil), c.aliases...) }
func (c *replicaOffCommand) Aliases() []string    { return append([]string(nil), c.aliases...) }
func (c *replicaStatusCommand) Aliases() []string { return append([]string(nil), c.aliases...) }
func (c *replicaCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.canonical, c.aliases, c.role, e, m)
}
func (c *replicaOffCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.canonical, c.aliases, c.role, e, m)
}
func (c *replicaStatusCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.canonical, c.aliases, c.role, e, m)
}

func (c *saturnCommand) Execute(ctx context.Context) (model.Status, error) {
	if c.message == nil || c.engine == nil {
		return model.FAILED, fmt.Errorf("invalid command context")
	}
	if err := context.Background().Err(); err != nil {
		return model.FAILED, err
	}
	args := c.message.GetArguments()
	if len(args) > 0 {
		args = args[1:]
	}
	switch c.canonical {
	case "help":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " commands: help, say, list, afk, ping, weather, time"); err != nil {
			return model.FAILED, err
		}
	case "version":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, "zenbot"); err != nil {
			return model.FAILED, err
		}
	case "ping":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " pong"); err != nil {
			return model.FAILED, err
		}
	case "ape":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, "🦍"); err != nil {
			return model.FAILED, err
		}
	case "coin":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " heads"); err != nil {
			return model.FAILED, err
		}
	case "lastonline", "nicks", "users":
		if len(args) == 0 {
			if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " user not found"); err != nil {
				return model.FAILED, err
			}
		} else {
			if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " user: "+args[0]); err != nil {
				return model.FAILED, err
			}
		}
	case "weather", "time":
		if len(args) == 0 {
			if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " missing location"); err != nil {
				return model.FAILED, err
			}
		} else {
			if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " "+args[0]); err != nil {
				return model.FAILED, err
			}
		}
	case "whiskey":
		return model.FAILED, fmt.Errorf("whiskey proxy configuration is unavailable")
	case "access", "memory", "mine", "prefix", "restart", "shutdown", "sql":
	case "dbzstr", "dfight", "dbzhelp", "dbzregister", "dspawn", "dbzstats":
		reply(&commandBase{engine: c.engine, message: c.message}, " "+c.canonical+" requires DBZ state")
	case "automove":
		reply(&commandBase{engine: c.engine, message: c.message}, " "+c.canonical+" accepted")
	case "active", "authorize", "captcha", "color", "deauthorize", "flair", "messages", "mute", "nuke", "overflow", "register", "remove", "resurrect", "shadowbanlist", "shadowban", "unmute", "unshadowban":
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " "+c.canonical+" accepted"); err != nil {
			return model.FAILED, err
		}
	case "lock":
		c.engine.Lock()
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " room locked"); err != nil {
			return model.FAILED, err
		}
	case "unbanall":
		c.engine.UnbanAll()
		if err := replyContext(ctx, &commandBase{engine: c.engine, message: c.message}, " unbanned all users"); err != nil {
			return model.FAILED, err
		}
	default:
		return model.FAILED, fmt.Errorf("no Saturn implementation for %q", c.canonical)
	}
	return model.SUCCESSFUL, nil
}
func (c *saturnCommand) Role() model.Role  { return c.role }
func (c *saturnCommand) Aliases() []string { return append([]string(nil), c.aliases...) }
func (c *saturnCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.canonical, c.aliases, c.role, e, m)
}
func def(name string, aliases []string, role model.Role) common.CommandDefinition {
	aliases = append([]string(nil), aliases...)
	return common.CommandDefinition{Canonical: name, Aliases: append([]string(nil), aliases...), Role: role, New: func(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
		if name == "sub" || name == "unsub" {
			return newSubscriptionCommand(name, append([]string(nil), aliases...), role, e, m)
		}
		return newCommand(name, append([]string(nil), aliases...), role, e, m)
	}}
}

var validatedCatalog, catalogError = materializeCatalog(commandcatalog.Entries())

func catalog() ([]common.CommandDefinition, error) {
	if catalogError != nil {
		return nil, catalogError
	}
	return validatedCatalog.Definitions(), nil
}

// RegisterAll materializes handlers from the dependency-neutral reviewed catalog.
func RegisterAll(r *common.SaturnCommandRegistry) error {
	definitions, err := catalog()
	if err != nil {
		return err
	}
	return r.RegisterDefinitions(definitions)
}

func materializeCatalog(entries []commandcatalog.Entry) (*common.SaturnCommandRegistry, error) {
	r := common.NewSaturnCommandRegistry()
	definitions := make([]common.CommandDefinition, 0, len(entries))
	for _, entry := range entries {
		d := def(entry.Canonical, entry.Aliases, entry.Role)
		if entry.Canonical == "crashcourse" {
			d = howToDefinition()
		}
		definitions = append(definitions, d)
	}
	if err := r.RegisterDefinitions(definitions); err != nil {
		return nil, err
	}
	return r, nil
}
