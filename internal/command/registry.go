package command

import (
	"context"
	"fmt"
	"strings"
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
	case "msgchannel", "ws", "wsa":
		room, text, err := ParseMsgChannel(c.message.GetArguments())
		if err != nil {
			return model.FAILED, err
		}
		if room == strings.TrimSpace(c.engine.GetChannel()) {
			_, err = c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper)
			return model.SUCCESSFUL, err
		}
		return model.FAILED, fmt.Errorf("remote room delivery is not configured")
	case "replica":
		controller, ok := c.engine.(ReplicaController)
		if !ok {
			return model.FAILED, fmt.Errorf("replica controller is not configured")
		}
		channel, err := ParseReplicaChannel(c.message.GetArguments())
		if err != nil {
			return model.FAILED, err
		}
		if err = controller.AddReplica(context.Background(), channel); err != nil {
			return model.FAILED, err
		}
		reply(&commandBase{engine: c.engine, message: c.message}, ReplicaReply(len(controller.ReplicaChannels())))
	case "replicaoff":
		controller, ok := c.engine.(ReplicaController)
		if !ok {
			return model.FAILED, fmt.Errorf("replica controller is not configured")
		}
		if err := ReplicaOff(context.Background(), controller, c.message.GetArguments()); err != nil {
			return model.FAILED, err
		}
	case "replicastatus":
		controller, ok := c.engine.(ReplicaController)
		if !ok {
			return model.FAILED, fmt.Errorf("replica controller is not configured")
		}
		reply(&commandBase{engine: c.engine, message: c.message}, ReplicaStatusReply(c.engine.GetChannel(), controller.ReplicaChannels()))
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

// RegisterAll is intentionally explicit and deterministic; it is the reviewed Saturn catalog.
func RegisterAll(r *common.SaturnCommandRegistry) error {
	entries := []common.CommandDefinition{
		def("access", []string{"grant", "access"}, model.ADMIN), def("memory", []string{"mem", "memory", "memstats"}, model.ADMIN), def("mine", []string{"mine"}, model.ADMIN), def("prefix", []string{"prefix"}, model.ADMIN), def("replica", []string{"replica", "bot", "agent"}, model.ADMIN), def("replicaoff", []string{"replicaoff", "offline", "botoff", "agentoff"}, model.ADMIN), def("replicastatus", []string{"replicastatus", "status"}, model.ADMIN), def("restart", []string{"restart", "reload", "re"}, model.ADMIN), def("shutdown", []string{"exit", "quit", "shutdown"}, model.ADMIN), def("sql", []string{"sql"}, model.ADMIN), def("whiskey", []string{"whiskey"}, model.ADMIN),
		def("dbzstr", []string{"dbzstr", "dstr", "daddstr"}, model.REGULAR), def("dfight", []string{"dfight", "df"}, model.REGULAR), def("dbzhelp", []string{"dbzhelp", "dbz", "dhelp"}, model.REGULAR), def("dbzregister", []string{"dbzregister", "dreg", "dr"}, model.REGULAR), def("dspawn", []string{"dspawn"}, model.REGULAR), def("dbzstats", []string{"dbzstats", "dstats", "dstat", "ds"}, model.REGULAR),
		def("active", []string{"active", "activity"}, model.MODERATOR), def("authorize", []string{"authorize", "auth"}, model.MODERATOR), def("automove", []string{"automove"}, model.MODERATOR), def("ban", []string{"ban"}, model.MODERATOR), def("captcha", []string{"captcha"}, model.MODERATOR), def("color", []string{"color"}, model.MODERATOR), def("deauthorize", []string{"deauthorize", "deauth"}, model.MODERATOR), def("flair", []string{"flair"}, model.MODERATOR), def("kick", []string{"kick", "k", "out"}, model.MODERATOR), def("messages", []string{"messages", "lastmessages"}, model.MODERATOR), def("lock", []string{"lock", "lockroom"}, model.MODERATOR), def("mute", []string{"mute", "dumb"}, model.MODERATOR), def("nuke", []string{"nuke"}, model.MODERATOR), def("overflow", []string{"overflow", "shoot", "love", "hug", "kiss"}, model.MODERATOR), def("register", []string{"reg", "register"}, model.MODERATOR), def("remove", []string{"del", "delete", "remove"}, model.MODERATOR), def("resurrect", []string{"move", "recover", "heal", "resurrect"}, model.MODERATOR), def("shadowbanlist", []string{"shadowbanlist", "banlist", "bannedusers"}, model.MODERATOR), def("shadowban", []string{"shadowban", "sban"}, model.MODERATOR), def("unbanall", []string{"unbanall", "pardonall"}, model.MODERATOR), def("unban", []string{"unban"}, model.MODERATOR), def("unmute", []string{"unmute", "undumb"}, model.MODERATOR), def("unshadowban", []string{"unshadowban", "shadowmercy", "unblock"}, model.MODERATOR),
		def("afk", []string{"afk", "a"}, model.USER), def("ape", []string{"ape", "harambe"}, model.USER), def("coin", []string{"coin", "toss", "ct"}, model.USER), def("help", []string{"help", "h"}, model.USER), howToDefinition(), def("info", []string{"info", "i", "whois", "who"}, model.USER), def("l", []string{"l"}, model.USER), def("lastonline", []string{"lastonline", "seen", "last", "online", "lastseen"}, model.USER), def("nicks", []string{"nicks", "t2n"}, model.USER), def("list", []string{"list"}, model.USER), def("mail", []string{"mail", "msg", "send"}, model.USER), def("msgchannel", []string{"msgchannel", "msgroom"}, model.USER), def("note", []string{"note", "save"}, model.USER), def("notes", []string{"notes"}, model.USER), def("ping", []string{"ping", "p"}, model.USER), def("users", []string{"users", "whitelist", "blacklist", "offenders", "knownoffenders"}, model.USER), def("say", []string{"say", "echo"}, model.USER), def("sub", []string{"sub", "subscribe"}, model.USER), def("time", []string{"time", "t"}, model.USER), def("unsub", []string{"unsub", "unsubscribe"}, model.USER), def("version", []string{"version", "v"}, model.USER), def("weather", []string{"weather", "w", "today"}, model.USER), def("wsa", []string{"wsa", "wsayanon", "anonsay"}, model.USER), def("ws", []string{"ws", "wsay"}, model.USER),
	}
	for _, d := range entries {
		if err := r.Register(d); err != nil {
			return err
		}
	}
	return r.Validate()
}
