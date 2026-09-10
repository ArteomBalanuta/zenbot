// Package catalog owns Saturn command identity and role metadata without
// depending on command handlers or the agent runtime.
package catalog

import (
	"sort"
	"strings"

	"zenbot/internal/model"
)

type Entry struct {
	Canonical string
	Aliases   []string
	Role      model.Role
}

func entry(canonical string, aliases []string, role model.Role) Entry {
	return Entry{Canonical: canonical, Aliases: aliases, Role: role}
}

var entries = []Entry{
	entry("access", []string{"grant", "access"}, model.ADMIN),
	entry("memory", []string{"mem", "memory", "memstats"}, model.ADMIN),
	entry("mine", []string{"mine"}, model.ADMIN),
	entry("prefix", []string{"prefix"}, model.ADMIN),
	entry("replica", []string{"replica", "bot", "agent"}, model.ADMIN),
	entry("replicaoff", []string{"replicaoff", "offline", "botoff", "agentoff"}, model.ADMIN),
	entry("replicastatus", []string{"replicastatus", "status"}, model.ADMIN),
	entry("restart", []string{"restart", "reload", "re"}, model.ADMIN),
	entry("shutdown", []string{"exit", "quit", "shutdown"}, model.ADMIN),
	entry("sql", []string{"sql"}, model.ADMIN),
	entry("whiskey", []string{"whiskey"}, model.ADMIN),
	entry("dbzstr", []string{"dbzstr", "dstr", "daddstr"}, model.REGULAR),
	entry("dfight", []string{"dfight", "df"}, model.REGULAR),
	entry("dbzhelp", []string{"dbzhelp", "dbz", "dhelp"}, model.REGULAR),
	entry("dbzregister", []string{"dbzregister", "dreg", "dr"}, model.REGULAR),
	entry("dspawn", []string{"dspawn"}, model.REGULAR),
	entry("dbzstats", []string{"dbzstats", "dstats", "dstat", "ds"}, model.REGULAR),
	entry("active", []string{"active", "activity"}, model.MODERATOR),
	entry("authorize", []string{"authorize", "auth"}, model.MODERATOR),
	entry("automove", []string{"automove"}, model.MODERATOR),
	entry("ban", []string{"ban"}, model.MODERATOR),
	entry("captcha", []string{"captcha"}, model.MODERATOR),
	entry("color", []string{"color"}, model.MODERATOR),
	entry("deauthorize", []string{"deauthorize", "deauth"}, model.MODERATOR),
	entry("flair", []string{"flair"}, model.MODERATOR),
	entry("kick", []string{"kick", "k", "out"}, model.MODERATOR),
	entry("messages", []string{"messages", "lastmessages"}, model.MODERATOR),
	entry("lock", []string{"lock", "lockroom"}, model.MODERATOR),
	entry("mute", []string{"mute", "dumb"}, model.MODERATOR),
	entry("nuke", []string{"nuke"}, model.MODERATOR),
	entry("overflow", []string{"overflow", "shoot", "love", "hug", "kiss"}, model.MODERATOR),
	entry("register", []string{"reg", "register"}, model.MODERATOR),
	entry("remove", []string{"del", "delete", "remove"}, model.MODERATOR),
	entry("resurrect", []string{"move", "recover", "heal", "resurrect"}, model.MODERATOR),
	entry("shadowbanlist", []string{"shadowbanlist", "banlist", "bannedusers"}, model.MODERATOR),
	entry("shadowban", []string{"shadowban", "sban"}, model.MODERATOR),
	entry("unbanall", []string{"unbanall", "pardonall"}, model.MODERATOR),
	entry("unban", []string{"unban"}, model.MODERATOR),
	entry("unmute", []string{"unmute", "undumb"}, model.MODERATOR),
	entry("unshadowban", []string{"unshadowban", "shadowmercy", "unblock"}, model.MODERATOR),
	entry("afk", []string{"afk", "a"}, model.REGULAR),
	entry("ape", []string{"ape", "harambe"}, model.REGULAR),
	entry("coin", []string{"coin", "toss", "ct"}, model.REGULAR),
	entry("help", []string{"help", "h"}, model.REGULAR),
	entry("crashcourse", []string{"crashcourse", "howto", "moderationcrashcourse", "hcguide"}, model.REGULAR),
	entry("info", []string{"info", "i", "whois", "who"}, model.REGULAR),
	entry("l", []string{"l"}, model.REGULAR),
	entry("lastonline", []string{"lastonline", "seen", "last", "online", "lastseen"}, model.REGULAR),
	entry("nicks", []string{"nicks", "t2n"}, model.REGULAR),
	entry("list", []string{"list"}, model.REGULAR),
	entry("mail", []string{"mail", "msg", "send"}, model.REGULAR),
	entry("msgchannel", []string{"msgchannel", "msgroom"}, model.REGULAR),
	entry("note", []string{"note", "save"}, model.REGULAR),
	entry("notes", []string{"notes"}, model.REGULAR),
	entry("ping", []string{"ping", "p"}, model.REGULAR),
	entry("users", []string{"users", "whitelist", "blacklist", "offenders", "knownoffenders"}, model.REGULAR),
	entry("say", []string{"say", "echo"}, model.REGULAR),
	entry("sub", []string{"sub", "subscribe"}, model.REGULAR),
	entry("time", []string{"time", "t"}, model.REGULAR),
	entry("unsub", []string{"unsub", "unsubscribe"}, model.REGULAR),
	entry("version", []string{"version", "v"}, model.REGULAR),
	entry("weather", []string{"weather", "w", "today"}, model.REGULAR),
	entry("wsa", []string{"wsa", "wsayanon", "anonsay"}, model.USER),
	entry("ws", []string{"ws", "wsay"}, model.USER),
}

func Entries() []Entry {
	out := make([]Entry, len(entries))
	for index, definition := range entries {
		definition.Aliases = append([]string(nil), definition.Aliases...)
		out[index] = definition
	}
	return out
}

type AgentAccess string

const (
	AgentPublic       AgentAccess = "PUBLIC"
	AgentModerator    AgentAccess = "MODERATOR"
	AgentPermanentBan AgentAccess = "PERMANENT_BAN"
	AgentAdmin        AgentAccess = "ADMIN"
)

var agentExclusions = map[string]struct{}{
	"l": {}, "mine": {}, "whiskey": {}, "ws": {}, "wsa": {},
}

var runCommandCanonicals = map[string]struct{}{
	"help": {}, "list": {}, "users": {}, "info": {}, "lastonline": {},
	"ping": {}, "weather": {}, "time": {}, "version": {},
	"captcha": {}, "mute": {}, "unmute": {}, "kick": {}, "shadowban": {}, "unshadowban": {},
	"ban": {}, "unban": {},
}

var permanentBanCommands = map[string]struct{}{
	"ban": {}, "unban": {}, "unbanall": {},
}

var targetedModerationCommands = map[string]struct{}{
	"ban": {}, "unban": {}, "mute": {}, "unmute": {}, "kick": {}, "shadowban": {}, "unshadowban": {},
}

func AgentEntries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, definition := range Entries() {
		if _, excluded := agentExclusions[definition.Canonical]; !excluded {
			out = append(out, definition)
		}
	}
	return out
}

func AgentEntry(canonical string) (Entry, bool) {
	canonical = strings.ToLower(strings.TrimSpace(canonical))
	for _, definition := range AgentEntries() {
		if definition.Canonical == canonical {
			return definition, true
		}
	}
	return Entry{}, false
}

func AgentEntryByAlias(alias string) (Entry, bool) {
	alias = strings.ToLower(strings.TrimSpace(alias))
	for _, definition := range AgentEntries() {
		for _, candidate := range append([]string{definition.Canonical}, definition.Aliases...) {
			if strings.ToLower(strings.TrimSpace(candidate)) == alias {
				return definition, true
			}
		}
	}
	return Entry{}, false
}

func Access(definition Entry) AgentAccess {
	if _, permanent := permanentBanCommands[definition.Canonical]; permanent {
		return AgentPermanentBan
	}
	switch definition.Role {
	case model.ADMIN:
		return AgentAdmin
	case model.MODERATOR:
		return AgentModerator
	default:
		return AgentPublic
	}
}

func RunCommandAliases(allowModeration, allowPermanentBan bool) []string {
	aliases := make(map[string]struct{})
	for _, definition := range AgentEntries() {
		if _, included := runCommandCanonicals[definition.Canonical]; !included {
			continue
		}
		access := Access(definition)
		if access == AgentModerator && !allowModeration || access == AgentPermanentBan && !allowPermanentBan {
			continue
		}
		for _, alias := range append([]string{definition.Canonical}, definition.Aliases...) {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias != "" {
				aliases[alias] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(aliases))
	for alias := range aliases {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

func TargetsUser(command string) bool {
	definition, found := AgentEntryByAlias(command)
	if !found {
		return false
	}
	_, ok := targetedModerationCommands[definition.Canonical]
	return ok
}
