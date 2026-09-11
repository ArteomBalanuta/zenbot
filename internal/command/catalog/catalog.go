// Package catalog owns Saturn command identity and role metadata without
// depending on command handlers or the agent runtime.
package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentcontract "zenbot/internal/agent/tool/contract"
	"zenbot/internal/model"
)

type Entry struct {
	Canonical string
	Aliases   []string
	Role      model.Role
	Agent     AgentToolSpec
}

func entry(canonical string, aliases []string, role model.Role, agent AgentToolSpec) Entry {
	if agent.Actionable && strings.TrimSpace(agent.PrimaryIntent) == "" {
		agent.PrimaryIntent = "saturn_" + canonical
	}
	return Entry{Canonical: canonical, Aliases: aliases, Role: role, Agent: agent}
}

var entries = []Entry{
	entry("access", []string{"grant", "access"}, model.ADMIN, agentTool("Grant access", "Grant one role to one or more trips.", "administration", AgentAdmin, commaListArguments("trips", "Trips receiving access.", "role", "Role to grant.", []string{"ADMIN", "MODERATOR", "TRUSTED", "USER", "REGULAR", "PEST"}), []string{"trips", "role"}, "Use for explicit role grants by a creator.", "Do not use to authorize a trip without assigning a role.", "Grant MODERATOR to trips aaa and bbb", `{"trips":["aaa","bbb"],"role":"MODERATOR"}`)),
	entry("memory", []string{"mem", "memory", "memstats"}, model.ADMIN, agentTool("Memory statistics", "Report the bot process memory statistics.", "runtime", AgentAdmin, emptyArguments(), []string{"process memory"}, "Use when a creator asks about bot memory usage.", "Do not use for conversation memory or user history.", "Show bot memory statistics", `{}`)),
	entry("mine", []string{"mine"}, model.ADMIN, hiddenAgentTool("Interactive mining is not a safe agent action.")),
	entry("prefix", []string{"prefix"}, model.ADMIN, agentTool("Change command prefix", "Replace the bot command prefix for subsequent commands.", "administration", AgentAdmin, positionalArguments(requiredToken("prefix", "New command prefix token.")), []string{"command prefix"}, "Use only when a creator explicitly requests a new prefix.", "Do not infer or change the prefix during ordinary conversation.", "Change the command prefix to $", `{"prefix":"$"}`)),
	entry("replica", []string{"replica", "bot", "agent"}, model.ADMIN, agentTool("Start room replica", "Start a bot replica in another room.", "replicas", AgentAdmin, positionalArguments(requiredToken("room", "Room to serve.")), []string{"room", "replica"}, "Use when a creator asks to start serving another room.", "Do not use for the current host room or for listing replicas.", "Start a replica in lounge", `{"room":"lounge"}`)),
	entry("replicaoff", []string{"replicaoff", "offline", "botoff", "agentoff"}, model.ADMIN, agentTool("Stop room replica", "Stop the bot replica serving a room.", "replicas", AgentAdmin, positionalArguments(requiredToken("room", "Replica room to stop.")), []string{"room", "replica"}, "Use when a creator asks to stop one existing replica.", "Do not use to shut down the host process.", "Stop the lounge replica", `{"room":"lounge"}`)),
	entry("replicastatus", []string{"replicastatus", "status"}, model.ADMIN, agentTool("Replica status", "List the host room and active replica rooms.", "replicas", AgentAdmin, emptyArguments(), []string{"replicas"}, "Use to inspect currently configured room replicas.", "Do not use to create or remove a replica.", "Show replica status", `{}`)),
	entry("restart", []string{"restart", "reload", "re"}, model.ADMIN, agentTool("Restart bot", "Restart the bot runtime.", "runtime", AgentAdmin, emptyArguments(), []string{"bot runtime"}, "Use only for an explicit creator restart request.", "Do not use as error recovery without creator instruction.", "Restart the bot", `{}`)),
	entry("shutdown", []string{"exit", "quit", "shutdown"}, model.ADMIN, agentTool("Shut down bot", "Stop the bot runtime.", "runtime", AgentAdmin, emptyArguments(), []string{"bot runtime"}, "Use only for an explicit creator shutdown request.", "Do not use for stopping a room replica.", "Shut down the bot", `{}`)),
	entry("sql", []string{"sql"}, model.ADMIN, agentTool("Execute SQL", "Execute an explicit SQL statement through Saturn's admin command.", "database", AgentAdmin, positionalArguments(requiredString("query", "Complete SQL statement.")), []string{"database"}, "Use when a creator explicitly asks to run supplied SQL.", "Do not invent SQL when a purpose-built read tool is available.", "Count registered users with SQL", `{"query":"SELECT COUNT(*) FROM users"}`)),
	entry("whiskey", []string{"whiskey"}, model.ADMIN, hiddenAgentTool("The Whiskey proxy is intentionally unavailable to the agent.")),
	entry("dbzstr", []string{"dbzstr", "dstr", "daddstr"}, model.REGULAR, agentTool("Allocate DBZ strength", "Spend a positive number of free DBZ points on strength.", "game", AgentPublic, positionalArguments(requiredInteger("amount", "Strength points to allocate.", 1, 2147483647)), []string{"caller character", "strength"}, "Use when the caller explicitly allocates DBZ strength points.", "Do not use for other DBZ stats or without an amount.", "Add 3 DBZ strength", `{"amount":3}`)),
	entry("dfight", []string{"dfight", "df"}, model.REGULAR, agentTool("Fight DBZ enemy", "Fight the named spawned DBZ enemy.", "game", AgentPublic, positionalArguments(requiredToken("enemy", "Spawned enemy name.")), []string{"DBZ enemy"}, "Use when the caller asks their character to fight an enemy.", "Do not use to fight another chat user outside the DBZ game.", "Fight saibaman", `{"enemy":"saibaman"}`)),
	entry("dbzhelp", []string{"dbzhelp", "dbz", "dhelp"}, model.REGULAR, agentTool("DBZ help", "Display DBZ game commands and mechanics.", "game", AgentPublic, emptyArguments(), []string{"DBZ game"}, "Use for questions about playing the DBZ game.", "Do not use to perform a game action.", "Show DBZ help", `{}`)),
	entry("dbzregister", []string{"dbzregister", "dreg", "dr"}, model.REGULAR, agentTool("Register DBZ character", "Register a DBZ character for the caller.", "game", AgentPublic, emptyArguments(), []string{"caller character"}, "Use when the caller asks to join the DBZ game.", "Do not use to register another user's identity.", "Register my DBZ character", `{}`)),
	entry("dspawn", []string{"dspawn"}, model.REGULAR, agentTool("Spawn DBZ enemy", "Spawn a named DBZ enemy.", "game", AgentPublic, positionalArguments(requiredToken("enemy", "Enemy name.")), []string{"DBZ enemy"}, "Use when the caller explicitly asks to spawn a DBZ enemy.", "Do not use for chat moderation or room users.", "Spawn saibaman", `{"enemy":"saibaman"}`)),
	entry("dbzstats", []string{"dbzstats", "dstats", "dstat", "ds"}, model.REGULAR, agentTool("DBZ statistics", "Display the caller's DBZ character statistics.", "game", AgentPublic, emptyArguments(), []string{"caller character"}, "Use when the caller asks for their DBZ stats.", "Do not use for process or room statistics.", "Show my DBZ stats", `{}`)),
	entry("active", []string{"active", "activity"}, model.MODERATOR, agentTool("User activity", "Display activity statistics for one identity.", "moderation", AgentModerator, positionalArguments(requiredToken("identity", "Trip or identity selector.")), []string{"user activity"}, "Use when a moderator asks for activity statistics.", "Do not use for current room presence.", "Show activity for trip 595754", `{"identity":"595754"}`)),
	entry("authorize", []string{"authorize", "auth"}, model.MODERATOR, agentTool("Authorize trip", "Authorize one trip for room access.", "moderation", AgentModerator, positionalArguments(requiredToken("trip", "Trip to authorize.")), []string{"trip authorization"}, "Use when a moderator explicitly authorizes a trip.", "Do not use to assign a role; use access instead.", "Authorize trip 595754", `{"trip":"595754"}`)),
	entry("automove", []string{"automove"}, model.MODERATOR, agentTool("Configure auto-move", "Enable, disable, or configure source and destination rooms for auto-move.", "moderation", AgentModerator, automoveArguments(), []string{"auto-move", "rooms"}, "Use for explicit auto-move lifecycle requests.", "Do not infer room configuration or use for moving one user.", "Configure auto-move from purgatory to lounge", `{"operation":"configure","source":"purgatory","destination":"lounge"}`)),
	entry("ban", []string{"ban"}, model.MODERATOR, agentTool("Permanently ban user", "Permanently ban one active nickname.", "moderation", AgentPermanentBan, positionalArguments(requiredToken("nick", "Active nickname to ban.")), []string{"user"}, "Use only for an explicit permanent-ban request by a creator.", "Do not use for a temporary removal; use kick instead.", "Ban @raider", `{"nick":"@raider"}`, targetedCommand, runCommandCompatible)),
	entry("captcha", []string{"captcha"}, model.MODERATOR, agentTool("Set captcha", "Enable or disable room captcha protection.", "moderation", AgentModerator, booleanStateArguments("enabled", "Whether captcha must be enabled.", "on", "off"), []string{"room captcha"}, "Use for an explicit captcha state change.", "Do not toggle captcha speculatively.", "Enable captcha", `{"enabled":true}`, runCommandCompatible)),
	entry("color", []string{"color"}, model.MODERATOR, agentTool("Set user color", "Force an active user's display color.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Active nickname."), requiredToken("color", "Color value, such as 00ff00.")), []string{"user", "color"}, "Use when a moderator specifies both user and color.", "Do not use to change the bot theme.", "Set @jill color to 00ff00", `{"nick":"@jill","color":"00ff00"}`, allowsSilentAction)),
	entry("deauthorize", []string{"deauthorize", "deauth"}, model.MODERATOR, agentTool("Deauthorize trip", "Remove room authorization from one trip.", "moderation", AgentModerator, positionalArguments(requiredToken("trip", "Trip to deauthorize.")), []string{"trip authorization"}, "Use when a moderator explicitly revokes a trip's authorization.", "Do not use to remove a registered identity.", "Deauthorize trip 595754", `{"trip":"595754"}`)),
	entry("flair", []string{"flair"}, model.MODERATOR, agentTool("Set user flair", "Force an active user's display flair.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Active nickname."), requiredToken("flair", "Flair value.")), []string{"user", "flair"}, "Use when a moderator specifies both user and flair.", "Do not use to send styled prose.", "Set @jill flair to trusted", `{"nick":"@jill","flair":"trusted"}`)),
	entry("kick", []string{"kick", "k", "out"}, model.MODERATOR, agentTool("Kick room user", "Kick one active user by nickname.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Active nickname to kick.")), []string{"user"}, "Use for an explicit room-removal request.", "Do not use for permanent bans or vague criticism.", "Kick @raider", `{"nick":"@raider"}`, targetedCommand, allowsSilentAction)),
	entry("messages", []string{"messages", "lastmessages"}, model.MODERATOR, agentTool("Recent user messages", "Display up to 30 recent messages associated with a trip.", "moderation", AgentModerator, positionalArguments(requiredToken("trip", "User trip."), requiredInteger("count", "Number of messages from 1 to 30.", 1, 30)), []string{"user messages"}, "Use when a moderator explicitly requests recent messages by trip.", "Do not use for live room membership or more than 30 messages.", "Show 10 messages for trip 595754", `{"trip":"595754","count":10}`, primaryIntent("trip_public_message_history"))),
	entry("lock", []string{"lock", "lockroom"}, model.MODERATOR, agentTool("Set room lock", "Lock or unlock the current room.", "moderation", AgentModerator, booleanStateArguments("locked", "Whether the room must be locked.", "on", "off"), []string{"room lock"}, "Use for an explicit room lock-state request.", "Do not use to mute individual users.", "Lock the room", `{"locked":true}`)),
	entry("mute", []string{"mute", "dumb"}, model.MODERATOR, agentTool("Mute user", "Mute one active user by nickname.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Active nickname to mute.")), []string{"user"}, "Use for an explicit mute request targeting an active user.", "Do not use for kicking or permanent bans.", "Mute @spammer", `{"nick":"@spammer"}`, targetedCommand, runCommandCompatible)),
	entry("nuke", []string{"nuke"}, model.MODERATOR, agentTool("Nuke room", "Permanently ban every active user in a specified room, then lock that room.", "moderation", AgentPermanentBan, positionalArguments(requiredToken("room", "Room to nuke.")), []string{"room"}, "Use only for an explicit permanent-ban request by a creator naming a room.", "Do not use for removing one user.", "Nuke the raid room", `{"room":"raid-room"}`, allowsSilentAction)),
	entry("overflow", []string{"overflow", "shoot", "love", "hug", "kiss"}, model.MODERATOR, agentTool("Overflow user", "Apply the room overflow action to one active nickname.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Active nickname.")), []string{"user"}, "Use when a moderator explicitly requests the overflow action.", "Do not infer this action from ordinary figurative language.", "Overflow @jill", `{"nick":"@jill"}`, allowsSilentAction)),
	entry("register", []string{"reg", "register"}, model.MODERATOR, agentTool("Register identity", "Register or associate a nickname and trip.", "identity", AgentModerator, positionalArguments(requiredToken("nick", "Nickname."), requiredToken("trip", "Trip.")), []string{"nickname", "trip"}, "Use when a moderator explicitly supplies both identity values.", "Do not use for DBZ character registration.", "Register jill with trip abc123", `{"nick":"jill","trip":"abc123"}`)),
	entry("remove", []string{"del", "delete", "remove"}, model.MODERATOR, agentTool("Remove identity", "Delete a registered nickname or trip identity.", "identity", AgentModerator, positionalArguments(requiredToken("identity", "Nickname or trip selector.")), []string{"registered identity"}, "Use for an explicit registered-identity deletion.", "Do not use to kick an active room user.", "Remove registered identity jill", `{"identity":"jill"}`)),
	entry("resurrect", []string{"move", "recover", "heal", "resurrect"}, model.MODERATOR, agentTool("Move user between rooms", "Move a nickname from one serving room to another room.", "moderation", AgentModerator, positionalArguments(requiredToken("nick", "Nickname to move."), requiredToken("source", "Source room."), requiredToken("destination", "Destination room.")), []string{"user", "source room", "destination room"}, "Use when a moderator explicitly specifies user, source, and destination.", "Do not use for configuring automatic movement.", "Move @jill from purgatory to lounge", `{"nick":"@jill","source":"purgatory","destination":"lounge"}`, allowsSilentAction)),
	entry("shadowbanlist", []string{"shadowbanlist", "banlist", "bannedusers"}, model.MODERATOR, agentTool("Shadow-ban list", "Display all currently shadow-banned identities.", "moderation", AgentModerator, emptyArguments(), []string{"shadow bans"}, "Use when a moderator asks to inspect shadow bans.", "Do not use to list permanent server bans.", "Show shadow-banned users", `{}`)),
	entry("shadowban", []string{"shadowban", "sban"}, model.MODERATOR, agentTool("Shadow-ban users", "Shadow-ban one exact nickname or all active names containing a fragment.", "moderation", AgentModerator, shadowBanArguments(), []string{"user"}, "Use for an explicit shadow-ban request.", "Do not use for ordinary kicks or permanent bans.", "Shadow-ban @raider", `{"mode":"exact","target":"@raider"}`, targetedCommand, runCommandCompatible)),
	entry("unbanall", []string{"unbanall", "pardonall"}, model.MODERATOR, agentTool("Remove all permanent bans", "Remove every permanent room ban.", "moderation", AgentPermanentBan, emptyArguments(), []string{"all permanent bans"}, "Use only for an explicit creator request to clear all bans.", "Do not use to remove one ban or shadow ban.", "Unban everyone", `{}`)),
	entry("unban", []string{"unban"}, model.MODERATOR, agentTool("Remove permanent ban", "Remove one permanent ban by hash.", "moderation", AgentPermanentBan, positionalArguments(requiredToken("hash", "Banned user hash.")), []string{"user"}, "Use only when a creator explicitly supplies the ban hash to remove.", "Do not use for shadow bans or mutes.", "Unban hash HjkUEWNlIRH35Xk", `{"hash":"HjkUEWNlIRH35Xk"}`, targetedCommand, runCommandCompatible)),
	entry("unmute", []string{"unmute", "undumb"}, model.MODERATOR, agentTool("Unmute user", "Remove one mute by user hash.", "moderation", AgentModerator, positionalArguments(requiredToken("hash", "Muted user hash.")), []string{"user"}, "Use for an explicit unmute request with a hash.", "Do not use for permanent or shadow bans.", "Unmute hash HjkUEWNlIRH35Xk", `{"hash":"HjkUEWNlIRH35Xk"}`, targetedCommand, runCommandCompatible)),
	entry("unshadowban", []string{"unshadowban", "shadowmercy", "unblock"}, model.MODERATOR, agentTool("Remove shadow ban", "Remove a shadow ban by nickname, trip, hash, or the -all selector.", "moderation", AgentModerator, positionalArguments(requiredToken("identity", "Shadow-ban identity selector or -all.")), []string{"user"}, "Use for an explicit shadow-ban removal request.", "Do not use for permanent bans or mutes.", "Remove jill's shadow ban", `{"identity":"jill"}`, targetedCommand, runCommandCompatible)),
	entry("afk", []string{"afk", "a"}, model.REGULAR, agentTool("Set AFK", "Mark the caller as away, optionally with a reason.", "presence", AgentPublic, positionalArguments(optionalString("reason", "Optional away reason.")), []string{"caller presence"}, "Use when the caller asks to mark themselves away.", "Do not use to change another user's presence.", "Mark me AFK for lunch", `{"reason":"lunch"}`)),
	entry("ape", []string{"ape", "harambe"}, model.REGULAR, agentTool("Send ape", "Send Saturn's ape response to the room.", "fun", AgentPublic, emptyArguments(), []string{"room message"}, "Use when the caller explicitly invokes the ape command.", "Do not use as a general conversational response.", "Send the ape", `{}`)),
	entry("coin", []string{"coin", "toss", "ct"}, model.REGULAR, agentTool("Toss coin", "Toss a coin and report the result.", "utility", AgentPublic, emptyArguments(), []string{"random coin result"}, "Use when the caller asks Saturn to toss a coin.", "Do not use for arbitrary random-number generation.", "Toss a coin", `{}`)),
	entry("help", []string{"help", "h"}, model.REGULAR, agentTool("Command help", "Display Saturn's formatted command help.", "help", AgentPublic, emptyArguments(), []string{"command catalog"}, "Use when the caller asks for the bot command list.", "Do not use instead of answering a question directly.", "Show Saturn help", `{}`, runCommandCompatible)),
	entry("crashcourse", []string{"crashcourse", "howto", "moderationcrashcourse", "hcguide"}, model.REGULAR, agentTool("Moderation crash course", "Display the concise moderation guide.", "help", AgentPublic, emptyArguments(), []string{"moderation guide"}, "Use when the caller asks how Saturn moderation works.", "Do not use to perform moderation actions.", "Show the moderation crash course", `{}`)),
	entry("info", []string{"info", "i", "whois", "who"}, model.REGULAR, agentTool("Current user info", "Display the current trip and hash of an active room user.", "lookup", AgentPublic, positionalArguments(requiredToken("nick", "Active nickname.")), []string{"user"}, "Use for current identity facts about an active user.", "Do not use for historical messages or last-seen time.", "Who is @jill?", `{"nick":"@jill"}`, runCommandCompatible)),
	entry("l", []string{"l"}, model.REGULAR, hiddenAgentTool("Recursive invocation of the agent command is forbidden.")),
	entry("lastonline", []string{"lastonline", "seen", "last", "online", "lastseen"}, model.REGULAR, agentTool("Last online", "Display when a registered nickname was last observed.", "lookup", AgentPublic, positionalArguments(requiredToken("nick", "Registered nickname.")), []string{"user presence history"}, "Use when the caller asks when a user was last online.", "Do not use for current room membership.", "When was jill last online?", `{"nick":"jill"}`, runCommandCompatible)),
	entry("nicks", []string{"nicks", "t2n"}, model.REGULAR, agentTool("Nicknames by trip", "List registered nicknames associated with one trip.", "lookup", AgentPublic, positionalArguments(requiredToken("trip", "Trip to resolve.")), []string{"trip", "nicknames"}, "Use when the caller asks which names belong to a trip.", "Do not use to inspect active room users.", "List names for trip 595754", `{"trip":"595754"}`, primaryIntent("trip_nicknames"))),
	entry("list", []string{"list"}, model.REGULAR, agentTool("List users in another room", "List users currently present in a named room through Saturn's temporary room snapshot workflow.", "lookup", AgentPublic, positionalArguments(requiredToken("room", "Other room whose current users are needed.")), []string{"other room users"}, "Use whenever the caller asks who is currently in a room other than the caller's current room.", "Do not use for the caller's current room or infer presence from database history.", "Who is currently in lounge?", `{"room":"lounge"}`, runCommandCompatible, primaryIntent("remote_room_presence"))),
	entry("mail", []string{"mail", "msg", "send"}, model.REGULAR, agentTool("Queue user mail", "Queue a private message for a registered recipient.", "messaging", AgentPublic, positionalArguments(requiredToken("recipient", "Registered recipient name."), requiredString("message", "Message to deliver.")), []string{"recipient", "queued message"}, "Use when the caller explicitly asks to mail a registered user.", "Do not use for immediate public room messages.", "Mail jill hello there", `{"recipient":"jill","message":"hello there"}`)),
	entry("msgchannel", []string{"msgchannel", "msgroom"}, model.REGULAR, agentTool("Message room", "Send an anonymous relay message to a named room.", "messaging", AgentPublic, positionalArguments(requiredToken("room", "Destination room."), requiredString("message", "Message body.")), []string{"room", "message"}, "Use when the caller explicitly asks to message another room.", "Do not use for private user mail.", "Tell lounge hello", `{"room":"lounge","message":"hello"}`)),
	entry("note", []string{"note", "save"}, model.REGULAR, agentTool("Save note", "Save one private note for the caller's trip.", "notes", AgentPublic, positionalArguments(requiredString("text", "Note text.")), []string{"caller notes"}, "Use when the caller asks Saturn to remember a private note.", "Do not use as conversational memory or for another user.", "Save note buy tea", `{"text":"buy tea"}`)),
	entry("notes", []string{"notes"}, model.REGULAR, agentTool("Manage notes", "List or purge private notes belonging to the caller's trip.", "notes", AgentPublic, notesArguments(), []string{"caller notes"}, "Use to list or explicitly purge the caller's saved notes.", "Do not purge notes unless the caller explicitly requests deletion.", "List my notes", `{"operation":"list"}`)),
	entry("ping", []string{"ping", "p"}, model.REGULAR, agentTool("Ping hack.chat", "Measure and report this bot runtime's TCP connection latency to hack.chat:80.", "utility", AgentPublic, emptyArguments(), []string{"hack.chat connection", "bot runtime"}, "Use when the caller asks to ping hack.chat, Saturn, or measure the bot runtime's connection latency.", "The target is fixed; do not use to test arbitrary network hosts or invent a host argument.", "Ping hack.chat", `{}`, runCommandCompatible)),
	entry("users", []string{"users", "whitelist", "blacklist", "offenders", "knownoffenders"}, model.REGULAR, agentTool("Registered users", "Display Saturn's registered user directory.", "lookup", AgentPublic, emptyArguments(), []string{"registered users"}, "Use when the caller asks for Saturn's registered users.", "Do not use for current room membership.", "Show registered users", `{}`, runCommandCompatible)),
	entry("say", []string{"say", "echo"}, model.REGULAR, agentTool("Say message", "Send supplied text as a bot room message.", "messaging", AgentPublic, positionalArguments(requiredString("message", "Text Saturn should say.")), []string{"room message"}, "Use when the caller explicitly asks Saturn to say exact content.", "Do not use for ordinary conversational answers.", "Say hello room", `{"message":"hello room"}`)),
	entry("sub", []string{"sub", "subscribe"}, model.REGULAR, agentTool("Subscribe", "Subscribe the caller's trip to Saturn notifications.", "subscriptions", AgentPublic, emptyArguments(), []string{"caller subscription"}, "Use when the caller explicitly asks to subscribe.", "Do not use to subscribe another user.", "Subscribe me", `{}`)),
	entry("time", []string{"time", "t"}, model.REGULAR, agentTool("Local time", "Display the current local time for a location.", "lookup", AgentPublic, positionalArguments(requiredString("location", "City or location.")), []string{"location time"}, "Use when the caller asks for current time in a location.", "Do not guess current time from conversation history.", "What time is it in Tokyo?", `{"location":"Tokyo"}`, runCommandCompatible)),
	entry("unsub", []string{"unsub", "unsubscribe"}, model.REGULAR, agentTool("Unsubscribe", "Remove the caller's Saturn notification subscription.", "subscriptions", AgentPublic, emptyArguments(), []string{"caller subscription"}, "Use when the caller explicitly asks to unsubscribe.", "Do not use to remove another user's subscription.", "Unsubscribe me", `{}`)),
	entry("version", []string{"version", "v"}, model.REGULAR, agentTool("Bot version", "Display Saturn's running version.", "utility", AgentPublic, emptyArguments(), []string{"bot version"}, "Use when the caller asks which Saturn version is running.", "Do not use for dependency or model versions.", "Show the bot version", `{}`, runCommandCompatible)),
	entry("weather", []string{"weather", "w", "today"}, model.REGULAR, agentTool("Current weather", "Fetch and display current weather for a location.", "lookup", AgentPublic, positionalArguments(requiredString("location", "City or location.")), []string{"location weather"}, "Use when current weather is requested for a location.", "Do not answer live weather from memory or historical messages.", "Show weather in Chisinau", `{"location":"Chisinau"}`, runCommandCompatible)),
	entry("wsa", []string{"wsa", "wsayanon", "anonsay"}, model.USER, hiddenAgentTool("Anonymous support relay is not exposed to the agent.")),
	entry("ws", []string{"ws", "wsay"}, model.USER, hiddenAgentTool("Support relay is not exposed to the agent.")),
}

func Entries() []Entry {
	out := make([]Entry, len(entries))
	for index, definition := range entries {
		definition.Aliases = append([]string(nil), definition.Aliases...)
		definition.Agent = cloneAgentToolSpec(definition.Agent)
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

func AgentEntries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, definition := range Entries() {
		if definition.Agent.Actionable {
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
	return definition.Agent.Access
}

func RunCommandAliases(allowModeration, allowPermanentBan bool) []string {
	aliases := make(map[string]struct{})
	for _, definition := range AgentEntries() {
		if !definition.Agent.RunCommandCompatible {
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

// ModerationReviewRunCommandAliases returns legacy aliases whose commands are
// admitted for an autonomous reviewed-author invocation.
func ModerationReviewRunCommandAliases() []string {
	unique := make(map[string]struct{})
	for _, definition := range AgentEntries() {
		if !definition.Agent.RunCommandCompatible || !ModerationReviewAllows(definition.Canonical) {
			continue
		}
		for _, alias := range append([]string{definition.Canonical}, definition.Aliases...) {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias != "" {
				unique[alias] = struct{}{}
			}
		}
	}
	aliases := make([]string, 0, len(unique))
	for alias := range unique {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func TargetsUser(command string) bool {
	definition, found := AgentEntryByAlias(command)
	return found && definition.Agent.TargetsUser
}

// ModerationReviewAllows is the fixed command authority granted to an
// autonomous reviewed-author invocation. It does not decide whether the
// reviewed message warrants that action.
func ModerationReviewAllows(command string) bool {
	definition, found := AgentEntryByAlias(command)
	return found && definition.Canonical == "mute"
}

func ValidateAgentContracts() error {
	seenCanonicals := make(map[string]struct{}, len(entries))
	seenAliases := make(map[string]string)
	for _, definition := range entries {
		canonical := strings.ToLower(strings.TrimSpace(definition.Canonical))
		if canonical == "" || canonical != definition.Canonical {
			return fmt.Errorf("invalid canonical command %q", definition.Canonical)
		}
		if _, duplicate := seenCanonicals[canonical]; duplicate {
			return fmt.Errorf("duplicate canonical command %q", canonical)
		}
		seenCanonicals[canonical] = struct{}{}
		for _, alias := range append([]string{canonical}, definition.Aliases...) {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return fmt.Errorf("command %q has a blank alias", canonical)
			}
			if owner, duplicate := seenAliases[alias]; duplicate && owner != canonical {
				return fmt.Errorf("alias %q belongs to both %q and %q", alias, owner, canonical)
			}
			seenAliases[alias] = canonical
		}
		if err := validateAgentToolSpec(canonical, definition.Agent); err != nil {
			return err
		}
	}
	return nil
}

func validateAgentToolSpec(canonical string, spec AgentToolSpec) error {
	hiddenReason := strings.TrimSpace(spec.HiddenReason)
	if !spec.Actionable {
		if hiddenReason == "" {
			return fmt.Errorf("command %q has no explicit agent actionability decision", canonical)
		}
		if spec.Arguments != nil {
			return fmt.Errorf("hidden command %q declares arguments", canonical)
		}
		return nil
	}
	if hiddenReason != "" {
		return fmt.Errorf("actionable command %q also declares a hidden reason", canonical)
	}
	if strings.TrimSpace(spec.PrimaryIntent) == "" {
		return fmt.Errorf("actionable command %q has no primary intent", canonical)
	}
	if strings.TrimSpace(spec.Label) == "" || strings.TrimSpace(spec.Description) == "" || strings.TrimSpace(spec.Category) == "" {
		return fmt.Errorf("actionable command %q has incomplete descriptive metadata", canonical)
	}
	if !validAgentAccess(spec.Access) {
		return fmt.Errorf("actionable command %q has invalid access %q", canonical, spec.Access)
	}
	for label, values := range map[string][]string{
		"target": spec.Targets, "use-when": spec.UseWhen, "when-not-use": spec.WhenNotUse,
	} {
		if len(values) == 0 {
			return fmt.Errorf("actionable command %q has no %s metadata", canonical, label)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("actionable command %q has blank %s metadata", canonical, label)
			}
		}
	}
	if spec.Arguments == nil {
		return fmt.Errorf("actionable command %q has no argument contract", canonical)
	}
	schema := spec.Arguments.Schema()
	if err := agentcontract.ValidateSchema(schema, true); err != nil {
		return fmt.Errorf("actionable command %q has an invalid parameter schema: %w", canonical, err)
	}
	if len(spec.Examples) == 0 {
		return fmt.Errorf("actionable command %q has no examples", canonical)
	}
	for _, example := range spec.Examples {
		if strings.TrimSpace(example.Prompt) == "" || !json.Valid(example.Arguments) {
			return fmt.Errorf("actionable command %q has a malformed example", canonical)
		}
		if err := agentcontract.ValidateArguments(schema, example.Arguments); err != nil {
			return fmt.Errorf("actionable command %q schema rejects example arguments: %w", canonical, err)
		}
		tail, err := spec.Arguments.Encode(example.Arguments)
		if err != nil {
			return fmt.Errorf("actionable command %q has invalid example arguments: %w", canonical, err)
		}
		if len(tail) > maxAgentCommandTailBytes {
			return fmt.Errorf("actionable command %q example exceeds %d encoded bytes", canonical, maxAgentCommandTailBytes)
		}
	}
	return nil
}

func validAgentAccess(access AgentAccess) bool {
	switch access {
	case AgentPublic, AgentModerator, AgentPermanentBan, AgentAdmin:
		return true
	default:
		return false
	}
}
