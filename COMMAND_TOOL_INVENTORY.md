# Saturn Command And Agent Tool Inventory

## Scope

Zenbot registers all 64 Saturn commands and exposes 59 of them as agent tools.
`l`, `mine`, `whiskey`, `ws`, and `wsa` carry explicit non-actionable metadata.
`l` remains the direct agent entry point and cannot be exposed back to the
model because that would permit recursive agent invocation.

Command identity is defined once in `internal/command/catalog/catalog.go`.
Each `catalog.Entry` owns its aliases, role, actionability, routing semantics,
access, targets, examples, and argument grammar. Chat registration, agent tool
registration, compatibility aliases, and moderation-target policy all derive
from those entries; there is no second command-name map.

## Contract

Every `saturn_<canonical>` tool has a command-specific JSON object schema. The
model supplies typed fields such as `{"location":"Chisinau"}` or
`{"mode":"exact","targets":["@raider"]}`; it never supplies a raw command
line or generic `arguments` field. An immutable `AgentArgumentContract`
validates and encodes those fields into the legacy command tail only inside the
trusted adapter. Schema or cross-field failures return `INVALID_ARGUMENTS`
before the command gateway is invoked. Encoded tails are capped at 4,000 bytes.

Every command tool remains an action, is non-idempotent, and executes
sequentially. Its result reports captured room messages and the number of
messages delivered. The normal command handler remains authoritative for role
checks, persistence, side effects, and user-visible formatting.

Access values below are capability gates applied before normal command
authorization:

- `PUBLIC`: available to any direct or addressed agent invocation.
- `MODERATOR`: requires the trusted caller to have moderation capability.
- `PERMANENT_BAN`: requires the creator-only permanent-ban capability.
- `ADMIN`: requires the creator/admin command capability.

### Typed Argument Families

| Shape | Commands | Provider fields |
|---|---|---|
| Empty object | `memory`, `replicastatus`, `restart`, `shutdown`, `dbzhelp`, `dbzregister`, `dbzstats`, `shadowbanlist`, `unbanall`, `ape`, `coin`, `help`, `crashcourse`, `ping`, `users`, `sub`, `unsub`, `version` | `{}` only |
| One required string | `prefix(prefix)`, `replica(room)`, `replicaoff(room)`, `sql(query)`, `dfight(enemy)`, `dspawn(enemy)`, `active(identity)`, `authorize(trip)`, `ban(nick)`, `deauthorize(trip)`, `mute(nick)`, `nuke(room)`, `overflow(nick)`, `remove(identity)`, `unban(hash)`, `unmute(hash)`, `unshadowban(identity)`, `info(nick)`, `lastonline(nick)`, `nicks(trip)`, `list(room)`, `note(text)`, `say(message)`, `time(location)`, `weather(location)` | Named nonblank string; token fields reject embedded whitespace |
| One optional string | `afk(reason)` | `reason` may be omitted |
| Ordered fields | `color(nick,color)`, `flair(nick,flair)`, `register(nick,trip)`, `mail(recipient,message)`, `msgchannel(room,message)`, `messages(trip,count)`, `resurrect(nick,source,destination)` | Strict named object; `messages.count` is `1..30` |
| Boolean state | `captcha(enabled)`, `lock(locked)` | Boolean encoded internally as `on` or `off` |
| Access grant | `access(trips,role)` | Nonempty trip array and `ADMIN|MODERATOR|TRUSTED|USER|REGULAR|PEST` |
| Kick modes | `kick(mode,targets)` | `mode` is `exact|multiple|contains`; cardinality is checked per mode |
| Shadow-ban modes | `shadowban(mode,target)` | `mode` is `exact|contains` |
| Auto-move operations | `automove(operation,source?,destination?)` | `enable|disable|configure`; configure requires both rooms |
| Notes operations | `notes(operation)` | `list|purge` |
| Positive integer | `dbzstr(amount)` | Signed 32-bit positive integer |

## Complete Matrix

| Canonical | Aliases | Chat status | Agent surface | Access |
|---|---|---|---|---|
| `access` | `grant`, `access` | Concrete | `saturn_access` | ADMIN |
| `memory` | `mem`, `memory`, `memstats` | Concrete | `saturn_memory` | ADMIN |
| `mine` | `mine` | Excluded | None | - |
| `prefix` | `prefix` | Concrete | `saturn_prefix` | ADMIN |
| `replica` | `replica`, `bot`, `agent` | Concrete | `saturn_replica` | ADMIN |
| `replicaoff` | `replicaoff`, `offline`, `botoff`, `agentoff` | Concrete | `saturn_replicaoff` | ADMIN |
| `replicastatus` | `replicastatus`, `status` | Concrete | `saturn_replicastatus` | ADMIN |
| `restart` | `restart`, `reload`, `re` | Concrete | `saturn_restart` | ADMIN |
| `shutdown` | `exit`, `quit`, `shutdown` | Concrete | `saturn_shutdown` | ADMIN |
| `sql` | `sql` | Concrete | `saturn_sql` | ADMIN |
| `whiskey` | `whiskey` | Excluded | None | - |
| `dbzstr` | `dbzstr`, `dstr`, `daddstr` | Concrete | `saturn_dbzstr` | PUBLIC |
| `dfight` | `dfight`, `df` | Concrete | `saturn_dfight` | PUBLIC |
| `dbzhelp` | `dbzhelp`, `dbz`, `dhelp` | Concrete | `saturn_dbzhelp` | PUBLIC |
| `dbzregister` | `dbzregister`, `dreg`, `dr` | Concrete | `saturn_dbzregister` | PUBLIC |
| `dspawn` | `dspawn` | Concrete | `saturn_dspawn` | PUBLIC |
| `dbzstats` | `dbzstats`, `dstats`, `dstat`, `ds` | Concrete | `saturn_dbzstats` | PUBLIC |
| `active` | `active`, `activity` | Concrete | `saturn_active` | MODERATOR |
| `authorize` | `authorize`, `auth` | Concrete | `saturn_authorize` | MODERATOR |
| `automove` | `automove` | Concrete | `saturn_automove` | MODERATOR |
| `ban` | `ban` | Concrete | `saturn_ban` | PERMANENT_BAN |
| `captcha` | `captcha` | Concrete | `saturn_captcha` | MODERATOR |
| `color` | `color` | Concrete | `saturn_color` | MODERATOR |
| `deauthorize` | `deauthorize`, `deauth` | Concrete | `saturn_deauthorize` | MODERATOR |
| `flair` | `flair` | Concrete | `saturn_flair` | MODERATOR |
| `kick` | `kick`, `k`, `out` | Concrete | `saturn_kick` | MODERATOR |
| `messages` | `messages`, `lastmessages` | Concrete | `saturn_messages` | MODERATOR |
| `lock` | `lock`, `lockroom` | Concrete | `saturn_lock` | MODERATOR |
| `mute` | `mute`, `dumb` | Concrete | `saturn_mute` | MODERATOR |
| `nuke` | `nuke` | Concrete | `saturn_nuke` | MODERATOR |
| `overflow` | `overflow`, `shoot`, `love`, `hug`, `kiss` | Concrete | `saturn_overflow` | MODERATOR |
| `register` | `reg`, `register` | Concrete | `saturn_register` | MODERATOR |
| `remove` | `del`, `delete`, `remove` | Concrete | `saturn_remove` | MODERATOR |
| `resurrect` | `move`, `recover`, `heal`, `resurrect` | Concrete | `saturn_resurrect` | MODERATOR |
| `shadowbanlist` | `shadowbanlist`, `banlist`, `bannedusers` | Concrete | `saturn_shadowbanlist` | MODERATOR |
| `shadowban` | `shadowban`, `sban` | Concrete | `saturn_shadowban` | MODERATOR |
| `unbanall` | `unbanall`, `pardonall` | Concrete | `saturn_unbanall` | PERMANENT_BAN |
| `unban` | `unban` | Concrete | `saturn_unban` | PERMANENT_BAN |
| `unmute` | `unmute`, `undumb` | Concrete | `saturn_unmute` | MODERATOR |
| `unshadowban` | `unshadowban`, `shadowmercy`, `unblock` | Concrete | `saturn_unshadowban` | MODERATOR |
| `afk` | `afk`, `a` | Concrete | `saturn_afk` | PUBLIC |
| `ape` | `ape`, `harambe` | Concrete | `saturn_ape` | PUBLIC |
| `coin` | `coin`, `toss`, `ct` | Concrete | `saturn_coin` | PUBLIC |
| `help` | `help`, `h` | Concrete | `saturn_help` | PUBLIC |
| `crashcourse` | `crashcourse`, `howto`, `moderationcrashcourse`, `hcguide` | Concrete | `saturn_crashcourse` | PUBLIC |
| `info` | `info`, `i`, `whois`, `who` | Concrete | `saturn_info` | PUBLIC |
| `l` | `l` | Concrete direct route | None (recursion guard) | PUBLIC |
| `lastonline` | `lastonline`, `seen`, `last`, `online`, `lastseen` | Concrete | `saturn_lastonline` | PUBLIC |
| `nicks` | `nicks`, `t2n` | Concrete | `saturn_nicks` | PUBLIC |
| `list` | `list` | Concrete | `saturn_list` | PUBLIC |
| `mail` | `mail`, `msg`, `send` | Concrete | `saturn_mail` | PUBLIC |
| `msgchannel` | `msgchannel`, `msgroom` | Concrete | `saturn_msgchannel` | PUBLIC |
| `note` | `note`, `save` | Concrete | `saturn_note` | PUBLIC |
| `notes` | `notes` | Concrete | `saturn_notes` | PUBLIC |
| `ping` | `ping`, `p` | Concrete | `saturn_ping` | PUBLIC |
| `users` | `users`, `whitelist`, `blacklist`, `offenders`, `knownoffenders` | Concrete | `saturn_users` | PUBLIC |
| `say` | `say`, `echo` | Concrete | `saturn_say` | PUBLIC |
| `sub` | `sub`, `subscribe` | Concrete | `saturn_sub` | PUBLIC |
| `time` | `time`, `t` | Concrete | `saturn_time` | PUBLIC |
| `unsub` | `unsub`, `unsubscribe` | Concrete | `saturn_unsub` | PUBLIC |
| `version` | `version`, `v` | Concrete | `saturn_version` | PUBLIC |
| `weather` | `weather`, `w`, `today` | Concrete | `saturn_weather` | PUBLIC |
| `wsa` | `wsa`, `wsayanon`, `anonsay` | Excluded | None | - |
| `ws` | `ws`, `wsay` | Excluded | None | - |

## Production Selection

Production exposes exactly one tool contract per available command through
`saturn_<command>`. The older `run_command` adapter remains in the codebase for
bounded compatibility tests, but is not registered in the production agent
inventory because it duplicates the concrete tools and makes model selection
ambiguous. Capability filtering still removes commands the current caller is
not authorized to execute.

`Registry.Manifest` computes a deterministic, caller-filtered, versioned
manifest and fails closed if any allowed descriptor is invalid. The local
manifest retains labels, descriptions, categories, aliases, targets,
positive/negative routing guidance, examples, schemas, access, effects, result
modes, capabilities, idempotency, timeouts, prerequisites, and resources. A
compact projection is sent as OpenAI-compatible function definitions without
`strict: true`; the creator-visible provider payload is regression-tested at
32 KiB or less. No keyword router, discovery call, or preliminary LLM request
selects commands.

## Verification

- `internal/command/production_registry_parity_test.go` proves every in-scope
  catalog entry builds a concrete command handler.
- `internal/command/agent_catalog_test.go` proves catalog-to-agent derivation.
- `internal/command/catalog/agent_contract_test.go` proves all 59 actionable
  contracts, the exact five hidden commands, immutable schemas, examples, and
  every argument-encoding strategy.
- `internal/agent/tool/saturn_command_test.go` proves schema, authorization,
  execution, and error-envelope behavior.
- `cmd/zenbot/live_agent_test.go` proves contextual production registration.
