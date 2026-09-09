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
			reply(&c.commandBase, "I'm the host bot serving current channel. Example: "+c.engine.GetPrefix()+"replica lounge")
		} else {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"replica lounge")
		}
		return model.FAILED, nil
	}
	if channel == c.engine.GetChannel() {
		reply(&c.commandBase, "I'm the host bot serving current channel. Example: "+c.engine.GetPrefix()+"replica lounge")
		return model.FAILED, nil
	}
	for _, existing := range controller.ReplicaChannels() {
		if existing == channel {
			reply(&c.commandBase, "Channel "+channel+" already has a replica running.")
			return model.FAILED, nil
		}
	}
	if err := controller.AddReplica(ctx, channel); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, fmt.Sprintf("started replica in channel: %s successfully. Number of replicas: %d", channel, len(controller.ReplicaChannels())))
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
			reply(&c.commandBase, "I'm the host bot serving current channel, not a replica.")
		} else {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"replicaoff lounge")
		}
		return model.FAILED, nil
	}
	if channel == c.engine.GetChannel() {
		reply(&c.commandBase, "I'm the host bot serving current channel, not a replica.")
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
		reply(&c.commandBase, "No replica in channel: "+channel)
		return model.FAILED, nil
	}
	if err := controller.RemoveReplica(ctx, channel); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, "Successfully shut down replica in channel: "+channel)
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
	reply(&c.commandBase, fmt.Sprintf("Host room:%s, replicas active: %d \\nServing channels: %s", c.engine.GetChannel(), len(channels), serving))
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

func (c *saturnCommand) Execute(context.Context) (model.Status, error) {
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
		reply(&commandBase{engine: c.engine, message: c.message}, " commands: help, say, list, afk, ping, weather, time")
	case "version":
		reply(&commandBase{engine: c.engine, message: c.message}, "zenbot")
	case "ping":
		reply(&commandBase{engine: c.engine, message: c.message}, " pong")
	case "ape":
		reply(&commandBase{engine: c.engine, message: c.message}, "🦍")
	case "coin":
		reply(&commandBase{engine: c.engine, message: c.message}, " heads")
	case "lastonline", "nicks", "users":
		if len(args) == 0 {
			reply(&commandBase{engine: c.engine, message: c.message}, " user not found")
		} else {
			reply(&commandBase{engine: c.engine, message: c.message}, " user: "+args[0])
		}
	case "weather", "time":
		if len(args) == 0 {
			reply(&commandBase{engine: c.engine, message: c.message}, " missing location")
		} else {
			reply(&commandBase{engine: c.engine, message: c.message}, " "+args[0])
		}
	case "whiskey":
		return model.FAILED, fmt.Errorf("whiskey proxy configuration is unavailable")
	case "access", "memory", "mine", "prefix", "restart", "shutdown", "sql":
	case "dbzstr", "dfight", "dbzhelp", "dbzregister", "dspawn", "dbzstats":
		reply(&commandBase{engine: c.engine, message: c.message}, " "+c.canonical+" requires DBZ state")
	case "active", "authorize", "automove", "captcha", "color", "deauthorize", "flair", "messages", "mute", "nuke", "overflow", "register", "remove", "resurrect", "shadowbanlist", "shadowban", "unmute", "unshadowban":
		reply(&commandBase{engine: c.engine, message: c.message}, " "+c.canonical+" accepted")
	case "lock":
		c.engine.Lock()
		reply(&commandBase{engine: c.engine, message: c.message}, " room locked")
	case "unbanall":
		c.engine.UnbanAll()
		reply(&commandBase{engine: c.engine, message: c.message}, " unbanned all users")
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
	return common.CommandDefinition{Canonical: name, Aliases: aliases, Role: role, New: func(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
		if name == "sub" || name == "unsub" {
			return newSubscriptionCommand(name, aliases, role, e, m)
		}
		return newCommand(name, aliases, role, e, m)
	}}
}

func catalog() []common.CommandDefinition {
	r := common.NewSaturnCommandRegistry()
	_ = RegisterAll(r)
	return r.Definitions()
}

// RegisterAll materializes handlers from the dependency-neutral reviewed catalog.
func RegisterAll(r *common.SaturnCommandRegistry) error {
	for _, entry := range commandcatalog.Entries() {
		d := def(entry.Canonical, entry.Aliases, entry.Role)
		if entry.Canonical == "crashcourse" {
			d = howToDefinition()
		}
		if err := r.Register(d); err != nil {
			return err
		}
	}
	return r.Validate()
}
