# Saturn Command And Agent Tool Inventory

## Scope

Zenbot migrates every Saturn command except the explicitly excluded `mine`,
`whiskey`, `ws`, and `wsa` commands. `l` is implemented as the direct agent
entry point and is intentionally not exposed back to the model as a tool,
which prevents recursive agent invocation.

Command identity is defined once in `internal/command/catalog/catalog.go`.
Both chat registration and agent tool registration derive from that catalog,
so aliases are not duplicated in separate registries.

## Contract

Every `saturn_<canonical>` tool accepts the same strict input object:

```json
{
  "arguments": "exact text after the command alias"
}
```

`arguments` is optional, is limited to 4,000 characters, and never includes
the command prefix. Every command tool is an action, is non-idempotent, and is
executed sequentially. Its result reports captured room messages and the
number of messages delivered. The normal command handler remains responsible
for validation, authorization, persistence, side effects, and user-visible
formatting.

Access values below are capability gates applied before normal command
authorization:

- `PUBLIC`: available to any direct or addressed agent invocation.
- `MODERATOR`: requires the trusted caller to have moderation capability.
- `PERMANENT_BAN`: requires the creator-only permanent-ban capability.
- `ADMIN`: requires the creator/admin command capability.

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

## Compatibility Tool

`run_command` remains as a compact compatibility surface for common lookups
and moderation actions. Its `command` enum is generated from the same catalog
and filtered by trusted capabilities. It does not replace the complete
per-command tools and cannot expose excluded commands.

Public commands in this compatibility surface are `help`, `list`, `users`,
`info`, `lastonline`, `ping`, `weather`, `time`, and `version`. Moderation adds
`captcha`, `mute`, `unmute`, `kick`, `shadowban`, and `unshadowban`.
Creator-only permanent-ban access adds `ban` and `unban`.

## Verification

- `internal/command/production_registry_parity_test.go` proves every in-scope
  catalog entry builds a concrete command handler.
- `internal/command/agent_catalog_test.go` proves catalog-to-agent derivation.
- `internal/agent/tool/saturn_command_test.go` proves schema, authorization,
  execution, and error-envelope behavior.
- `cmd/zenbot/live_agent_test.go` proves contextual production registration.
