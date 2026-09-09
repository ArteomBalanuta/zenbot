# Saturn Admin + Moderator Full Migration — Technical Specification and Deterministic Inventory

**Scope:** architecture/inventory only. Saturn `develop` at `src/main/java/org/saturn/app/**` is authoritative. Target observations are the current dirty `migration/saturn-zenbot-parity` worktree. This document does **not** accept, edit, supersede, stage, or clean the current semantic-moderation/reversal/last-online work.

**Evidence convention:** `[O]` observed source; `[T]` source/target test-backed; `[P]` partial/placeholder; `[M]` missing. Source citations use repository-relative paths and symbols rather than volatile line numbers.

## 1. Boundary, invariants, and entry sequence

- Saturn discovery is reflective: `command/factory/CommandFactory.loadCommandCatalog`, accepting `org.saturn.app.command.impl`, annotated `@CommandAliases`, sorting class names, rejecting both normalized duplicate and anagram-equivalent aliases (`validateCommandCatalog`). It constructs `(EngineImpl, ChatMessage, aliases)` commands.
- Saturn parses exactly one prefix-stripped command token plus whitespace-separated arguments in `command/UserCommandBaseImpl`; its special first-argument `\\n` split, `requiredArgument`, `requiredIntArgument`, and `normalizedNickArgument` are shared semantics. `execute` resolves a command, checks authorization, executes, then logs the command/status/channel via `LogRepository`.
- Saturn authorization is OR: configured command trip list (including literal `x`) **or** persisted trip role >= requested role. Failure sends `msg mercury for access.` to the original whisper/public path (`service/impl/AuthorizationServiceImpl.isUserAuthorized`). Do not substitute Zenbot-only authorization language or infer authority from untrusted chat text.
- Zenbot deliberately replaced reflection with the explicit `internal/command.RegisterAll` catalog. It preserves all aliases/roles for this scope but `internal/command/registry.go:saturnCommand.Execute` proves many entries are generic acknowledgement, no-op, or “unavailable,” not parity.
- Current inbound ordering is `ResolveUserMetadata → AuditChatMessage → IgnoreBotMessage → RelayAgentMessage → LogChatMessage → DeliverPendingMail → UpdateAfkState → YoutubePreview → CernEasterEgg → AgentParticipation → DispatchUserCommand` in `internal/listener/message.DefaultChainWithParticipation`. Keep this order unless a source-required operation needs an explicit, reviewed insertion. Dispatch resolves prefix/fields, authorizes the active resolved user, then executes (`DispatchUserCommand.Handle`).

```text
[chat JSON] -> EngineImpl.DispatchMessage("chat") -> UserChatListener
  -> message chain (metadata/audit/self filter/relay/.../agent/command)
  -> common.BuildCommand(alias) -> SecurityService/AuthorizationRepository
  -> concrete command -> typed service/core/snapshot/repository
  -> SendChatMessage or SendRawMessage -> transport / outbound queue
```

`internal/core.EngineImpl.StartContext` already provides parent-context cancellation, one start, transport error reporting once, and `StopContext` cancellation + close wait. `ReplicaManager` is terminal after `StopAll`; snapshot temporary sessions are deliberately not replicas. New command implementations must accept dispatch context end-to-end; never use the `context.Background()` patterns in the generic target placeholder as parity evidence.

## 2. Exact catalog inventory and target disposition

Roles below are source `getAuthorizedRole`; aliases are exact source annotations and exact Zenbot `RegisterAll` entries. “Owner” is the only permitted concrete target ownership; “new” means create that named file/package rather than widening the generic switch.

| Saturn class/package | role; aliases | Saturn behavioral contract/dependencies | exact Zenbot owner | disposition |
|---|---|---|---|---|
| `impl/admin/AccessUserCommandImpl` | ADMIN; `grant`, `access` | exactly two args `<trip> <Role>`; validates enum; `AuthorizationService.grant`; grants configured admin trips USER; sends grant/error acknowledgements | `internal/command/identity_commands.go`; `internal/service/services.go:UserService`; `repository.AuthorizationRepository.GrantTrip` | [T] accepted identity slice; retain exact semantics regression |
| `MemoryCommandImpl` | ADMIN; `mem`, `memory`, `memstats` | JVM free/total/max/used formatted by `OutService`; original whisper mode | `internal/command/memory.go` | [T] accepted rapid slice; target Go runtime stats are an intentional foundation mismatch requiring decision D1 below |
| `MineTripCommandImpl` | ADMIN; `mine` | `<room> <start|stop>`; proxy config; temporary `LIST_CMD` snapshot; `MineTripOperation`; reply workflow output | new `internal/command/mine.go`; existing `listener/snapshot` coordinator/session factory; new `core/trip_miner.go` | [M] catalog no-op |
| `PrefixCommandImpl` | ADMIN; `prefix` | one argument; changes engine prefix and replies old/new | new `internal/command/prefix.go`; new synchronized `core.EngineImpl.SetPrefix` | [M] catalog no-op; current `Prefix` is unsynchronized field |
| `ReplicaCommandImpl` | ADMIN; `replica`, `bot`, `agent` | channel argument, rejects duplicate, constructs REPLICA engine/database, starts and reports count | `internal/command/replica.go`, `core/replica_controller.go`, `factory/replica_factory.go` | [P] concrete add exists; exact Saturn acknowledgement/composition and live failure lifecycle remain incomplete |
| `ReplicaOffCommandImpl` | ADMIN; `replicaoff`, `offline`, `botoff`, `agentoff` | usage; rejects host current channel; finds/stops channel replica; reply | `internal/command/replica.go`, `core/replica_controller.go` | [P] concrete stop exists; Saturn responses/host guard must be reconciled |
| `ReplicaStatusCommandImpl` | ADMIN; `replicastatus`, `status` | replies current host channel + replica list | `internal/command/replica.go` | [P] concrete status exists; output parity unproven |
| `RestartCommandImpl` | ADMIN; `restart`, `reload`, `re` | `ApplicationLifecycle.restartHost()` | new `internal/command/lifecycle.go`; `cmd/zenbot/main.go` lifecycle supervisor | [M] no-op; requires controlled process lifecycle decision |
| `ShutdownCommandImpl` | ADMIN; `exit`, `quit`, `shutdown` | `ApplicationLifecycle.shutdown()` | new `internal/command/lifecycle.go`; `cmd/zenbot/main.go` supervisor | [M] no-op; requires controlled process lifecycle decision |
| `SqlUserCommandImpl` | ADMIN; `sql` | joins args, `SQLService.executeSql(cmd,true)`, sends result | new `internal/command/sql.go`, `internal/service/sql.go`, `repository/h2/sql_admin.go` | [M] no-op; do not activate generic agent SQL policy as substitute |
| `WhiskeyReplicaCommandImpl` | ADMIN; `whiskey` | `<channel> <name>`; parses configured proxies, tests candidates, starts AGENT/Whiskey replica, reports success/failure to author | new `internal/command/whiskey.go`, `core/whiskey_controller.go`, `factory/replica_factory.go`, config extension | [M] explicit unavailable error; high-risk remote/proxy lifecycle |
| `impl/moderator/ActivityCommandImpl` | MODERATOR; `active`, `activity` | required target; `SQLService.executeActivityStats`; `Stats: \\n...` | new `internal/command/activity.go`, `service/activity.go`, `repository/h2/activity.go` | [M] generic “accepted” |
| `AuthorizeTripCommandImpl` | MODERATOR; `authorize`, `auth` | one trip; emits `authtrip`; acknowledgement | new `internal/command/moderation_identity.go`; typed raw operation in `core/moderation.go` | [P] identity registration exists, but command is generic acknowledgement; protocol operation missing |
| `AutoMoveUserCommandImpl` | MODERATOR; `automove` | `on|off`; toggles engine auto-move state, source-shaped usage/status replies | new `internal/command/automove.go`; new `core/auto_move.go`; listener join hook | [M] generic acknowledgement |
| `BanUserCommandImpl` | MODERATOR; `ban` | normalized nick required; raw `{"cmd":"ban","nick":...}`; acknowledgement | `internal/command/ban.go`, `core.EngineImpl.Ban` | [P] concrete target exists but source usage, identity normalization, payload/response need reconciliation |
| `CaptchaCommandImpl` | MODERATOR; `captcha` | default/on enables `enablecaptcha`; off disables `disablecaptcha`; usage otherwise | new `internal/command/captcha.go`; typed `core` raw operation | [P] join semantic foundation emits enable only; public command is generic acknowledgement |
| `ColorCommandImpl` | MODERATOR; `color` | `<nick> <hex>`; target must be active; `forcecolor` nick/color payload | new `internal/command/color_flair.go`; typed core operation | [M] generic acknowledgement |
| `DeAuthorizeTripCommandImpl` | MODERATOR; `deauthorize`, `deauth` | one trip; `deauthtrip`; acknowledgement | `internal/command/moderation_identity.go`; typed core raw operation | [M] generic acknowledgement |
| `FlairCommandImpl` | MODERATOR; `flair` | `<nick> <flair>`; target active; `forceflair` nick/flair payload | new `internal/command/color_flair.go`; typed core operation | [M] generic acknowledgement |
| `KickUserCommandImpl` | MODERATOR; `kick`, `k`, `out` | normalized/sanitized nick; local kick or remote snapshot/move behavior; `kick` raw payload | `internal/command/kick.go`; `core.EngineImpl.Kick`; `listener/snapshot` | [P] local concrete handler exists; exact missing/remote/last-kicked behavior unproven |
| `LastMessagesCommandImpl` | MODERATOR; `messages`, `lastmessages` | parses selector/count (max 30), identity history query, Java-escaped reply | `internal/command/identity_commands.go`; `service.UserService.SaturnLastMessages`; `repository.SqlUtilGroupBRepository` | [T] accepted identity/history slice; preserve tests |
| `LockRoomUserCommandImpl` | MODERATOR; `lock`, `lockroom` | `on|off`; raw `lockroom`/`unlockroom`; exact usage/replies | `internal/command/lock.go`; `core.EngineImpl.Lock/Unlock` | [P] concrete behavior exists but catalog switch bypasses it for registration paths and response parity must be closed |
| `MuteUserCommandImpl` | MODERATOR; `mute`, `dumb` | normalized active target required; needs hash; raw `mute` nick; reply includes target/hash | new `internal/command/mute.go`; typed `core` moderation operation | [M] generic acknowledgement; semantic scaffolding is not user command parity |
| `NukeCommandImpl` | MODERATOR; `nuke` | remote room snapshot then `NukeRoomOperation`; temporary session cleanup/reply | new `internal/command/nuke.go`; `listener/snapshot` operation | [M] generic acknowledgement |
| `OverflowCommandImpl` | MODERATOR; `overflow`, `shoot`, `love`, `hug`, `kiss` | target argument; raw `overflow` nick; source output | new `internal/command/overflow.go`; typed core operation | [M] generic acknowledgement |
| `RegisterUserCommandImpl` | MODERATOR; `reg`, `register` | name/trip two-way registration, conditional existing identity paths and messages | `internal/command/identity_commands.go`; UserService identity methods | [T] accepted identity slice |
| `RemoveUserCommandImpl` | MODERATOR; `del`, `delete`, `remove` | removes identity by name/trip; success/error reply | `internal/command/identity_commands.go`; `UserService.DeleteIdentity` | [T] accepted Group-B target behavior |
| `ResurrectUserCommandImpl` | MODERATOR; `move`, `recover`, `heal`, `resurrect` | `<nick> <from> <to>`; local/remote list snapshot and raw kick `to`; static last-kicked state support | new `internal/command/resurrect.go`; `listener/snapshot/KickOrResurrectOperation`; new `core/move_state.go` | [M] generic acknowledgement |
| `ShadowBanList` | MODERATOR; `shadowbanlist`, `banlist`, `bannedusers` | reads bans, decodes base64 hash, format/no-ban response | new `internal/command/shadow_ban.go`; `repository.ShadowBanRepository` list extension | [M] generic acknowledgement; persistence only partial |
| `ShadowBanUserCommandImpl` | MODERATOR; `shadowban`, `sban` | single active/offline target or `-c <contains>` active scan; persist trip/name/base64(hash), active targets also kick | new `internal/command/shadow_ban.go`; `core` typed shadow action; `h2/shadow_ban.go` | [P] uncommitted semantic foundation persists trusted join/message actions; public command, contains mode, responses missing—preserve exactly |
| `UnBanAllUserCommandImpl` | MODERATOR; `unbanall`, `pardonall` | raw `unbanall`; `mercy.` reply | `internal/command/unbanAll.go`; `core.EngineImpl.UnbanAll` | [P] concrete core exists; exact dispatch/output not proven |
| `UnBanUserCommandImpl` | MODERATOR; `unban` | hash required; raw `unban` hash; exact usage/reply | `internal/command/unban.go`; `core.EngineImpl.Unban` | [P] concrete target exists; source usage/hashing output unproven |
| `UnMuteUserCommandImpl` | MODERATOR; `unmute`, `undumb` | hash required; raw `unmute` hash; source exact replies | new `internal/command/mute.go`; typed core operation | [M] generic acknowledgement; reversal scaffolding only records unmute contract, not public command |
| `UnShadowBanUserCommandImpl` | MODERATOR; `unshadowban`, `shadowmercy`, `unblock` | blank => list/delete-all with output; target => delete matching name/trip/base64 hash, no raw op | new `internal/command/shadow_ban.go`; `ShadowBanReversalRepository`; new list/delete-all repository methods | [P] uncommitted reversal typed `RemoveShadowBanBySourceTarget` exists; public command and all-mode missing—preserve exactly |

**Mechanical count:** 11 admin + 23 moderator = **34 source command classes**, 34 target catalog definitions, and 72 scoped aliases (admin 24; moderator 48). A verifier must derive this directly from source annotations and `RegisterAll`, then compare exact ordered alias sets and source role; do not trust this table alone.

## 3. Required dependency inventory

| Saturn dependency/source symbol | required behavior | target state / owner |
|---|---|---|
| `service/impl/ModServiceImpl` | all raw commands serialized through `JsonPayloads.command`: `ban(nick)`, `unban(hash)`, `unbanall`, `lockroom`, `unlockroom`, `enablecaptcha`, `disablecaptcha`, `authtrip(trip)`, `deauthtrip(trip)`, `mute(nick)`, `unmute(hash)`, `forceflair(nick,flair)`, `forcecolor(nick,color)`, `kick(nick[,to])`, `overflow(nick)` | [P] `core.EngineImpl` has legacy Ban/Kick/Lock methods; create a typed `internal/core/moderation_operations.go` implementing all operations via `util.JsonPayloads`, returning errors/context and avoiding untyped maps |
| `ModServiceImpl.shadowBan/getBannedUsers/unshadowBan/unshadowbanAll/isShadowBanned` | `banned_users`: nullable trip/name/hash/reason, created_on; hash stored Base64 UTF-8; match trip OR exact name OR decoded hash | [P] `repository/h2/shadow_ban.go` has persist + target delete only. Add typed `ListShadowBans`, `RemoveAllShadowBans`; do not alter accepted reversal semantics |
| `AuthorizationServiceImpl` | app authorized trips OR `trips.type` hierarchy; grant upsert/update; unknown/blank resolves REGULAR | [T/P] `SecurityService` + `AuthorizationRepository` and H2 auth present; access accepted. Verify config-OR wording and role ordering before shared edits |
| `DataBaseServiceImpl`, `SQLServiceImpl`, `SqlUtil` | activity, admin SQL, identity/history selectors; prepared bindings; H2 connection | [P/M] Group B identity/history accepted, H2 is PostgreSQL-wire. Add bounded named queries; admin arbitrary SQL is decision D2 and cannot use `agent/sql.Policy` placeholder |
| `listener/snapshot/DefaultRoomSnapshotCoordinator`, `MineTripOperation`, `NukeRoomOperation`, `KickOrResurrectOperation` | temporary websocket session owns workflow id, parser, result callback, cleanup; remote room operations are not permanent replicas | [P] target `listener/snapshot` coordinator/session factory exists with tests. Mine/nuke/resurrect operation adapters absent |
| `facade/impl/EngineImpl`, `ApplicationLifecycle` | host/replica engine maps, shared command output/raw queues, lifecycle restart/shutdown | [P] target managed transport context lifecycle + `ReplicaManager` exist; lifecycle command supervisor and mutable prefix/automove/Whiskey contracts absent |
| `listener/impl/UserMessageListenerImpl` and Saturn message handler chain | metadata/audit/bot filtering then command dispatch; commands audit after execution | [P] target chain ordered above; audit happens before dispatch and command audit depends on generic common path. Add command audit result tests per slice; do not reorder relay/agent semantic work |
| `model/dto/User`, `IdentityUtil` | case/trim normalization only at explicitly used nick target boundaries; active-user requirements for mute/color/flair/shadow active behavior | [P] `model.User`, `util/identity.go`, and active lookup exist; enforce per-command source differences rather than a global “protected user” bypass |

### Persistence/H2 target contract

Saturn `schema-h2.sql` owns the needed existing tables: `trips`, `names`, `trip_names`, `messages`, `executed_commands`, and `banned_users`; relevant indexes include `idx_messages_{trip,name,hash}_created_on`, `idx_banned_users_{name,trip,hash}`, and command/channel index. Zenbot `repository/h2/database.go:bootstrap` starts H2 2.3.232 over PostgreSQL wire and has its own embedded schema plus additive `user_presence_log` and `banned_users` creation. Each implementation slice must use that bootstrap boundary, parameterized `$n` SQL, and real-H2 test fixture.

Required new SQL (names are contracts, not permission to change schema now):

```sql
-- shadow ban list/all delete
SELECT trip, name, hash, reason FROM banned_users;
DELETE FROM banned_users;
-- activity: exact source query/format must be ported from SQLServiceImpl, not invented
-- admin SQL: only after D2, explicit read/write mode and result renderer
```

No new table is justified for `prefix`, `automove`, or command lifecycle unless the user selects durable cross-process state (D3). Existing `banned_users` preservation must remain compatible with the accepted target’s `DELETE ... name=$1 OR trip=$2 OR hash=$3` source target binding/base64 contract.

## 4. Authorization, protection, and error/output policy

1. **Public command authorization:** preserve source role minimum and config-OR-DB authority. Registration must remain conditional only on available required dependencies; aliases must not silently resolve to generic fallback.
2. **Protected users:** Saturn command classes do not uniformly protect admin/bot/self targets; they normally act on active identity or raw target. The current uncommitted semantic monitors protect bot/self/configured admin + creator trips and are fail-closed. Keep those protections for *automated semantic moderation only*. A user decision is required before applying them to manual Saturn command behavior (D4).
3. **Parsing:** use source-specific missing-argument usage, first argument only where source does, target normalization only where `IdentityUtil.normalizeNickTarget`/`normalizedNickArgument` is used, literal `-c` behavior for shadowban, integer cap 30 for messages, and `on|off` case behavior as tested in Saturn. No generic `strings.Join` parser can claim parity.
4. **Output/error:** send to issuer with inbound whisper status. Preserve source `\\n` output content through target’s established normalization path; raw protocol values must be JSON escaped via `util.JsonPayloads`/`encoding/json`, never format strings. A service/repository error must produce `FAILED` and no success acknowledgement. Canceled contexts must produce no outbound payload/reply.
5. **Lifecycle/retry:** Saturn command code creates snapshot work and replica threads but no universal command retry policy. Zenbot snapshot/transport/retry/cancellation behavior must be reused, not augmented. Replicas use managed Start/Stop and must be removed after start/runtime failure exactly once; temporary remote operations must unregister/close on success, parse failure, timeout, caller cancellation, and transport failure.

## 5. Dependency-ordered bounded implementation slices

Each slice owns only listed paths; tests are minimum parity evidence plus `go test ./...` at batch completion. “High Risk” routes to `@senior-developer`.

| order / slice | prerequisites and contract | target paths | risk / route | focused test targets |
|---|---|---|---|---|
| S0 catalog guard | freeze exact 36/86 scoped source catalog, roles, aliases and reject generic fallback for any concrete slice | `internal/command/catalog_test.go` (test only) | Standard / @developer | source-derived table test; all aliases resolve expected role/canonical |
| S1 typed raw moderation operations | one context-aware typed operation per ModService payload; exact JSON and no output on canceled/error | new `internal/core/moderation_operations.go`, `*_test.go`; minimal common interface extension | High / @senior-developer | every payload including special nick/hash fields; cancellation/no queue; escaping |
| S2 manual simple moderation | captcha, auth/deauth, lock, overflow, ban/unban/unbanall concrete commands using S1; no generic switch | new `internal/command/{captcha,moderation_identity,overflow}.go`, reconcile `ban.go`,`unban.go`,`unbanAll.go`,`lock.go`, tests | Standard / @developer | aliases/roles, usage, precise payload/output/error/whisper dispatch |
| S3 active-target moderation | mute/unmute, color/flair; active-user resolve case behavior and hash inclusion | new `internal/command/{mute,color_flair}.go`, tests | High / @senior-developer | absent target no payload; case target; raw payload; exact mute reply/hash; protected-policy separation |
| S4 shadow-ban command vertical | list, single/offline, contains mode, all deletion; retain current uncommitted semantic/reversal source exactly | new `internal/command/shadow_ban.go`; extend `repository` + `h2/shadow_ban.go`; tests | High / @senior-developer | real-H2 base64 round trip/list; `-c`; active kick; blank all; repository failure/cancel; no semantic-monitor regression |
| S5 identity/activity/history closure | preserve accepted register/authorize/access/messages/remove, add activity only; no rewrites of accepted files except reviewed integration | new `command/activity.go`, `service/activity.go`, `repository/h2/activity.go`, tests | Standard / @developer | source activity formatting/query and existing accepted identity regressions |
| S6 prefix + automove | decide volatile/durable state then atomic prefix update and source listener join move behavior | new `command/{prefix,automove}.go`, new `core/{prefix_state,auto_move}.go`, listener integration/test | High / @senior-developer | dispatch after prefix mutation; on/off; concurrent read; join order; no agent-chain reorder |
| S7 remote snapshot operations | port MineTrip/Nuke/KickOrResurrect operation adapters on existing coordinator; no replica registration | new `command/{mine,nuke,resurrect}.go`, new snapshot operation files/tests | High / @senior-developer | real websocket temporary session, timeout/cancel/parse cleanup, raw target operations, no host active-user mutation |
| S8 replica + Whiskey completion | exact replica command responses, start failure removal; proxy candidate sequencing + AGENT relay/proxy config | reconcile `command/replica.go`; new `command/whiskey.go`, `core/whiskey_controller.go`, factory/config tests | High / @senior-developer | command through inbound websocket, runtime failure removal once, proxy failover/cancel, no leaked replica |
| S9 mine/lifecycle/admin SQL | Mine depends S7; restart/shutdown requires D1; SQL requires D2 and bounded H2 result renderer | `command/mine.go`, new `command/lifecycle.go`, `command/sql.go`, `service/sql.go`, `h2/sql_admin.go` | High / @senior-developer | operation contract, lifecycle supervisor fake, SQL policy/error/render real-H2 |
| S10 final shared registration/audit | replace only scoped catalog generic switch cases, composition dependencies, command audit after execution, all aliases live | `handlers.go`, `dispatch_adapter.go`, factory/main only as required, integration tests | High / @senior-developer | inbound authorized/unauthorized public+whisper, audit status, preexisting agent/reversal/lastonline full regression |

## 6. Parallel execution schedule

```text
Wave A (parallel): S0 | S1 | S5
Wave B (after S1, parallel): S2 | S3 | S4
Wave C (parallel, no dependency on B except shared catalog lock): S6 | S7
Wave D: S8 after existing replica foundation + S7 transport findings; S9 lifecycle/SQL after decisions
Wave E: S10 only after all selected concrete commands pass focused tests
```

Rules for parallelism: S3/S4 must not edit current semantic monitor/reversal files; S5 must not alter last-online accepted files; S6 owns listener insertion only after an explicit ordering review; S8 owns replica composition seams; S10 is the sole shared registry/dispatch integration owner. Every worker starts with `git status --short`, records preexisting dirty paths, and only stages nothing/commits nothing. No reset, clean, checkout, or application-code modification belongs to this architecture task.

## 7. Genuine decisions / incompatible foundations (must be user-selected)

- **D1 lifecycle semantics (blocking S9):** Saturn directly calls a singleton process restart/shutdown. Zenbot has a context-managed `main` lifecycle but no restart supervisor. Choose: (a) controlled in-process supervisor/re-exec contract, (b) shutdown-only mapped response plus external orchestrator restart, or (c) explicitly prohibit lifecycle commands. Do not make these no-op “successes.”
- **D2 admin `sql` authority (blocking S9):** Saturn invokes arbitrary `executeSql(cmd,true)` under ADMIN. Zenbot’s `agent/sql.Policy` is an unrelated partial PostgreSQL parser guard and is not an admin console. Choose Saturn-compatible arbitrary H2 SQL with auditable admin-only execution, or a strict read-only/allowlisted incompatible target. The latter must be documented as an approved deviation, not called parity.
- **D3 prefix/automove durability (blocking S6):** Saturn mutates runtime engine state. Zenbot replica configuration/restarts can make memory-only propagation ambiguous. Choose volatile per-engine state (closest source behavior), persisted host-only state, or synchronized host/replica propagation; schema/config changes require explicit approval.
- **D4 manual protected-user rule (blocking exact moderator parity):** source manual commands do not share current automatic semantic protection gates. Choose strict source manual behavior or apply protection to manual commands as a knowingly incompatible safety rule. Never silently use the semantic monitor’s rules.
- **D5 Whiskey endpoint/proxy foundation (blocking S8):** Saturn proxy parsing/tests and AGENT replica mode have no equivalent Zenbot configuration contract. User must supply/approve target proxy config keys, timeout/attempt order, and whether AGENT means host-relayed child; do not reuse unrelated websocket config invisibly.

## 8. Completeness ledger and mechanical acceptance

A slice may move from `[M]/[P]` to `[T]` only when all ledger columns are true. This ledger is intentionally mechanical.

| ledger key | source unit set | target proof required |
|---|---|---|
| A01–A11 | all 11 admin classes named in §2 | source annotation alias/role extraction == target registry; concrete `newCommand`/definition has no generic route; focused contract test; integration dispatch test |
| M01–M23 | all 23 moderator classes named in §2 | same four checks; include raw payload assertion where applicable |
| D01 | `ModServiceImpl` payload methods | exact cmd/key/value JSON fixture set, including escaping/cancel/no-send |
| D02 | `AuthorizationServiceImpl` | config OR persisted role matrix; denial path; role ordering; grant upsert real-H2 |
| D03 | ban persistence methods | H2 schema exists, base64 write/read/delete name/trip/hash, list/all-delete, failure propagation |
| D04 | snapshot operations | temporary ownership registry zero after every terminal result and caller cancellation |
| D05 | Engine/lifecycle/replica | start/stop/retry/cancellation/error once, replica manager visibility/removal, no temp-as-replica |
| D06 | listener behavior | exact order vector, authorization before command side effects, command audit result after execution |

**Verifier recipe (read-only):**
1. Enumerate Saturn `command/impl/admin/*.java` and `moderator/*.java`, parse each `@CommandAliases` and `getAuthorizedRole`; assert 11/23/34 and alias equality against `internal/command/registry.go:RegisterAll`.
2. Search every scoped canonical in `newCommand`/registered definitions; reject `saturnCommand.Execute` generic cases, `accepted`, `requires`, `unavailable`, blank cases, and no-op fallthrough for a claimed row.
3. Resolve every cited target file and run named focused tests plus `go test ./...`; run real-H2 tests for D02/D03 and websocket tests for D04/D05.
4. Run `git diff --check` and compare status against the recorded pre-task dirty set. Only this handoff may be new/modified by this architecture work.

## 9. Evidence limitations

- Accepted handoffs establish only their bounded target slices: identity/history, memory, replica lifecycle portions, last-online, and current uncommitted semantic moderation/reversals. They do not establish public moderator command parity.
- The source has sparse tests for several listed commands. Implementers must read each corresponding Saturn class and source test where present before reproducing strings/edge cases; this specification does not authorize output “improvements.”
- No application code, test, config, migration-plan, frozen audit, Saturn source, existing handoff, staging, commit, or push was performed by this work.
