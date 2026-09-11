# Commands

Use the configured command prefix before a command or alias; examples below use
`*`, the example configuration's default. Commands also accept whispers. `<value>` is required, `[value]` is optional,
and names, trips, hashes, and room selectors are single tokens unless the command
explicitly accepts text. Run `*help` for the in-chat reference.

The [command catalog](../internal/command/catalog/catalog.go) is the source of
truth for names, aliases, declared roles, agent access, and argument contracts.
It currently contains 64 entries, of which 59 can be exposed as `saturn_<name>`
agent tools. Actual availability depends on the caller and configured services;
the catalog count is not a promise that every deployment exposes every command.
`l` is the direct agent entry point, `ws` and `wsa` are manual support relays,
and `mine` and `whiskey` are unavailable legacy entries.

## Access and authorization

The tables group commands by their declared chat role: `REGULAR`, `USER`,
`MODERATOR`, or `ADMIN`. Stronger roles satisfy weaker thresholds. Configured
administrator trips and persisted role grants participate in authorization.
The existing per-command `userTrips` whitelist also permits selected commands;
it is not a global role bypass and does not authorize raw SQL. See the
[security service](../internal/service/security_service.go) and
[chat adapter](../internal/command/dispatch_adapter.go) for those exceptions.

Agent access is a separate gate derived from trusted caller identity. Public
tools need no extra capability; moderation tools require moderation capability;
admin tools require creator/admin capability. `ban`, `unban`, `unbanall`, and
`nuke` require creator-only permanent-ban capability through the agent, even
though their declared chat role is `MODERATOR`. A natural-language assertion of
authority does not grant access. Automated moderation review is narrower still:
it may only mute the reviewed author, not run arbitrary moderator commands.
The [agent gateway](../internal/command/agent_gateway.go) checks access and
targets before execution, and command handlers retain their authorization rules.

## User commands (`REGULAR`)

Aliases below are alternatives to the canonical name in the first column.

| Command and syntax | Other aliases | Behavior |
|---|---|---|
| `help` | `h` | Whisper the command reference. |
| `crashcourse` | `howto`, `moderationcrashcourse`, `hcguide` | Show the moderation guide. |
| `l <prompt>` | — | Ask the configured room agent. Never exposed back to the agent as a tool. |
| `afk [reason]` | `a` | Mark yourself away. |
| `ape` | `harambe` | Print an ape. |
| `coin` | `toss`, `ct` | Toss a coin. |
| `info <nick>` | `i`, `whois`, `who` | Show an active room user's current trip and hash. |
| `lastonline <nick-or-trip>` | `seen`, `last`, `online`, `lastseen` | Show the latest persisted public message and join/leave observations across stored rooms. Ambiguous nick/trip matches are rejected. |
| `nicks <trip>` | `t2n` | List historical publicly observed nicknames for an exact trip. |
| `list <room>` | — | Show a live room snapshot; other rooms use a temporary room connection. |
| `users` | `whitelist`, `blacklist`, `offenders`, `knownoffenders` | Show the registered user directory; these aliases do not select different live rosters. |
| `mail <recipient> <text>` | `msg`, `send` | Queue private mail to a registered recipient. Queuing does not prove the recipient has read it. |
| `msgchannel <room> <text>` | `msgroom` | Request an anonymous message relay to a room. |
| `note <text>` | `save` | Save a private note for your trip. |
| `notes [purge]` | — | List your private notes, or delete all of them with `purge`. |
| `ping` | `p` | Measure the bot runtime's TCP connection latency to `hack.chat:80`. No arbitrary target argument. |
| `say <text>` | `echo` | Send supplied text as a bot room message. |
| `sub` | `subscribe` | Subscribe your trip to join notifications. |
| `unsub` | `unsubscribe` | Cancel your subscription. |
| `time <location>` | `t` | Fetch current local time for a city or location. |
| `weather <location>` | `w`, `today` | Fetch current weather. |
| `version` | `v` | Show the running bot version. |

History and registration are different from presence: `lastonline`, `nicks`, and
`users` do not prove who is online now. In agent requests, `room_users` supplies
the caller's current room; `saturn_list` requires another room. Its result can
include `room`, `users`, `count`, `returnedCount`, and `truncated`, all derived
from the same deduplicated snapshot as the displayed list. An empty snapshot has
an empty user list and count zero. In manual chat, `list <current-room>` is also
supported. Bare `list` displays the local roster and a usage example but returns
an unsuccessful command status; supply the room for a normal invocation.

Last-seen and mail timestamps use a single compact representation: `now`,
`5m ago`, or `3h ago` for events less than a day old; otherwise a UTC date such
as `9 Sep 22:08 UTC`, with the year included when it differs from the current
UTC year. Future timestamps use an absolute date. Last-seen combines the latest
event with its timestamp and keeps an older independent presence/message fact
without duplicating the latest timestamp. Missing history, nicknames, registered
users, and notes receive explicit empty-result replies. `n2t` is not an alias;
`nicks`/`t2n` performs trip-to-nickname lookup.

## DBZ game (`REGULAR`)

These commands operate on the existing DBZ game and require its configured
service. Use `dbzhelp` for the game rules and available enemies.

| Command and syntax | Other aliases | Behavior |
|---|---|---|
| `dbzhelp` | `dbz`, `dhelp` | Show game help. |
| `dbzregister` | `dreg`, `dr` | Register your character. |
| `dbzstats` | `dstats`, `dstat`, `ds` | Show your character statistics. |
| `dbzstr <amount>` | `dstr`, `daddstr` | Spend available points on strength; supply a positive integer. |
| `dspawn <enemy>` | — | Spawn a named enemy. |
| `dfight <enemy>` | `df` | Fight a spawned enemy. |

## Moderation and identity (`MODERATOR`)

| Command and syntax | Other aliases | Behavior |
|---|---|---|
| `active <trip>` | `activity` | Show activity statistics for an exact trip, not a nickname. |
| `authorize <trip>` | `auth` | Request room authorization for a trip; does not assign a bot role. |
| `deauthorize <trip>` | `deauth` | Request removal of room authorization. |
| `automove on`, `automove off`, `automove <source> <destination>` | — | Enable, disable, or configure the existing automatic room movement service. Requires an auto-move controller. |
| `ban <nick>` | — | Request a permanent ban for an active user. Creator-only through the agent. |
| `captcha <on-or-off>` | — | Request a captcha state change. |
| `color <nick> <color>` | — | Request a display color for an active user, such as `00ff00`. |
| `flair <nick> <flair>` | — | Request a display flair for an active user. |
| `kick <nick>` | `k`, `out` | Request removal of an active user. |
| `messages <trip> <count>` | `lastmessages` | Show recent messages for a trip; request 1–30. |
| `lock <on-or-off>` | `lockroom` | Request a current-room lock or unlock. |
| `mute <nick>` | `dumb` | Request a mute for an active user. |
| `nuke <room>` | — | Snapshot a room, request permanent bans for every observed user, then request a lock. Creator-only through the agent. |
| `overflow <nick>` | `shoot`, `love`, `hug`, `kiss` | Send the overflow action for an active user. |
| `register <nick> <trip>` | `reg` | Create an identity or attach one new value to an existing identity. Rejects requests where both values already exist. |
| `remove <nick-or-trip>` | `del`, `delete` | Delete a registered identity; does not kick an online user. |
| `resurrect <nick> <source> <destination>` | `move`, `recover`, `heal` | Find the user in the source room and request a kick to the destination. |
| `shadowban <nick>` or `shadowban -c <fragment>` | `sban` | Save local shadow-ban records and request kicks for active matches. |
| `shadowbanlist` | `banlist`, `bannedusers` | List local shadow bans, not permanent server bans. |
| `unshadowban <nick-or-trip-or-hash>` or `unshadowban -all` | `shadowmercy`, `unblock` | Remove matching local shadow bans or all local shadow bans. |
| `unban <hash>` | — | Request removal of a permanent ban by hash. Creator-only through the agent. |
| `unbanall` | `pardonall` | Request clearing every permanent room ban. Creator-only through the agent. |
| `unmute <hash>` | `undumb` | Request removal of a mute by hash. |

Manual kick also supports `kick -m <nick> <nick> ...` for explicit multiple
targets and `kick -c <fragment>` for matching active names. These bulk modes are
not arguments to `saturn_kick`, whose contract accepts one nickname. The
`saturn_shadowban` tool expresses its exact/contains modes as typed fields.

Successful outbound moderation confirms a submitted request, not that the chat
server applied the ban, mute, kick, color, authorization, captcha, or lock.
Remote `nuke` and `resurrect` completion confirms the workflow's writes and flush,
not the resulting membership or moderation state. Shadow bans have two distinct
effects: local persistence can be confirmed while kick application remains
unconfirmed. Bulk operations can partially succeed before a later error.

## Administration (`ADMIN`)

| Command and syntax | Other aliases | Behavior |
|---|---|---|
| `access <trip[,trip...]> <role>` | `grant` | Persist a role grant: `ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, or `PEST`. |
| `memory` | `mem`, `memstats` | Show Go runtime memory values in MiB. |
| `prefix <token>` | — | Change the live command prefix. |
| `replica <room>` | `bot`, `agent` | Start a bot replica for a room. |
| `replicaoff <room>` | `offline`, `botoff`, `agentoff` | Stop a running room replica. |
| `replicastatus` | `status` | Show host and replica status. |
| `restart` | `reload`, `re` | Submit a restart request for the current host. |
| `shutdown` | `exit`, `quit` | Submit a shutdown request for the current host. |
| `sql <SQL>` | — | Submit raw SQL through the bot's database connection and render its result. |

Restart and shutdown acknowledgments report request admission, including a
request coalesced with an existing one. They do not promise completed retirement
unless the result explicitly reports completion. Shutdown does not stop the
whole process, replicas, or the agent runtime. Use `replicaoff` for a replica.

The raw `sql` command and its `saturn_sql` agent wrapper share the admin command
path. They do **not** use the bounded read-only SQL policy. The separate
`database_sql` agent tool requires `DYNAMIC_SQL` capability, schema inspection,
and validated read-only SQL with configured limits. Treat these as distinct
interfaces: permission to use a read tool is not permission to execute raw SQL.

## Support relays (`USER`)

| Command and syntax | Other aliases | Behavior |
|---|---|---|
| `ws <text>` | `wsay` | Forward text through the configured support relay. |
| `wsa <text>` | `wsayanon`, `anonsay` | Forward anonymous support relay text. |

These commands require the support relay capability and are not agent tools.
The `ADMIN` catalog entries `mine` and `whiskey` have no supported production
handler and are not included as usable commands here.

## Agent command results

Each exposed `saturn_<canonical>` command has a typed JSON argument object,
derived from the [argument contracts](../internal/command/catalog/agent_contract.go).
For example, weather takes `{"location":"Chisinau"}`, kick takes
`{"nick":"@raider"}`, and lock takes `{"locked":true}`. Invalid fields are
rejected before dispatch. Most commands send their output directly to the room;
kick is silent and supplies an action receipt for the agent's confirmation.

Command tools execute sequentially and are treated as non-idempotent actions,
including commands that display information. Results retain delivery and action
receipts even when a later step fails. An error does not imply rollback, and an
unknown outcome must not be treated as proof that nothing happened. Agent
results distinguish actions not started, not committed, committed, partially
committed, and unknown. Verify available evidence before repeating an action.

For implementation details, see the [availability checks](../internal/command/availability.go),
[production registration](../internal/command/dispatch_adapter.go), and
[command tool adapter](../internal/agent/tool/saturn_command.go).
