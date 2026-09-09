# Admin `mine`: source-grounded architecture handoff

## Verdict and bounded delivery

**Implement `mine` as the next admin parity slice only after adding a process-owned mining scheduler capability and a credential-bearing temporary snapshot session.** It is not a repository/service/H2 slice: Saturn persists successful mined credentials to a local `trips.txt` file, not a database. The present Zenbot `mine` catalog row is a deliberately allowed generic no-op, so it must remain unregistered until its concrete capability exists.

This handoff is source-grounded in Saturn `MineTripCommandImpl` and every directly invoked snapshot/model/base-command dependency. Claims below are tagged **[OBSERVED]**, **[TEST-BACKED]**, **[LIMITATION]**, or **[RECOMMENDED]**.

## Authority and observed Saturn contract

### Catalog, authorization, parsing, and statuses

- **[OBSERVED]** Saturn declares exactly `@CommandAliases({"mine"})`. It inherits `UserCommandBaseImpl`, whose default authorized role is `ADMIN`; the `MineTripCommandImpl` constructor passes `getAdminTrips(engine)` as its configured authorization list.  
  **Citations:** Saturn `src/main/java/org/saturn/app/command/impl/admin/MineTripCommandImpl.java` (`@CommandAliases`, constructor); `src/main/java/org/saturn/app/command/UserCommandBaseImpl.java` (`getAuthorizedRole`); `src/main/java/org/saturn/app/util/Util.java` (`getAdminTrips`).
- **[OBSERVED]** Arguments are whitespace-separated after the command alias. `execute()` requires at least two arguments: `<room> <start|stop>`. Missing input replies to the author with the exact leading-space text ` Example: <prefix>mine <room> <start|stop>` and returns `FAILED`.  
  **Citations:** Saturn `MineTripCommandImpl.execute`; Saturn `UserCommandBaseImpl.getArguments`, `fail`, `replyToAuthor`.
- **[OBSERVED]** A target equal to the serving engine channel returns `FAILED` silently. A blank/null second argument returns `FAILED` silently. Any subcommand other than exact lowercase `count`, `start`, or `stop` returns `FAILED` silently. There is no trim, lowercasing, room normalization, or extra-argument validation in `MineTripCommandImpl`.  
  **Citation:** Saturn `MineTripCommandImpl.execute`, `handleCommand`.
- **[OBSERVED]** `count` replies exactly ` TaskCount: <taskCount>, Completed: <completedTaskCount>, Active: <activeCount>` through the invoking author and original whisper mode, then returns `SUCCESSFUL`. `taskCount`, `completedTaskCount`, and `activeCount` are the executor's global metrics, not room-specific counts.  
  **Citations:** Saturn `MineTripCommandImpl.handleCommand`, `printTaskCount`; Saturn `UserCommandBaseImpl.replyToAuthor`.
- **[OBSERVED]** `start` accepts an optional third delay argument. Absent/null/blank defaults to `35` seconds; otherwise it calls Java `Long.parseLong` directly, so malformed or overflow input throws rather than producing a command failure reply. It schedules after an initial five-second delay, returns `SUCCESSFUL` after scheduling the checker, and emits no immediate chat acknowledgement.  
  **Citation:** Saturn `MineTripCommandImpl.handleCommand`, `parseDelay`, `startMining`, `execute`.
- **[OBSERVED]** `stop` calls `shutdownNow()` on the static shared miner executor, awaits termination for up to ten seconds, returns `SUCCESSFUL`, and emits no chat reply. This is global shutdown: the room parameter is logged but does not select tasks. The static executor is never recreated, so later `start` behavior after a stop is an unhandled/rejected-execution source defect rather than a lifecycle to normalize away.  
  **Citation:** Saturn `MineTripCommandImpl.stopMining`, static executor field.

### Mining work, remote delivery, and persistence

- **[OBSERVED]** Construction builds a static `ip -> Proxy` map from `engine.proxies` by splitting each configured string at `:` and retaining only the first two segments. A nonempty map produces one fixed-delay task per map entry; an empty map produces one task with `null` proxy. The proxy is only logged: `joinChannel(channel, proxy)` does not pass it into the temporary session or transport. Duplicate IPs overwrite earlier entries, and malformed proxy strings can throw during construction.  
  **Citations:** Saturn `MineTripCommandImpl` constructor, `startMining`, `scheduleMiningTask`, `joinChannel`; Saturn `src/main/java/org/saturn/app/model/dto/Proxy.java`.
- **[OBSERVED]** Each scheduled invocation generates a UUID workflow ID, random eight-character alphanumeric nick, and random 128-character alphanumeric password. It creates a one-use `LIST_CMD` temporary engine/session for the target room, then submits an online-set workflow. The request author is the generated nick; source channel is the serving engine channel; target channel is the requested room; destination and reply message are `null`.  
  **Citation:** Saturn `MineTripCommandImpl.joinChannel`; Saturn `src/main/java/org/saturn/app/listener/snapshot/EngineSnapshotSession.java`; Saturn `src/main/java/org/saturn/app/listener/snapshot/RoomSnapshotRequest.java`.
- **[OBSERVED]** The online-set parser requires JSON object `cmd == "onlineSet"` and the `users` array for `LIST_CMD`; malformed payloads fail the workflow. The temporary session registers only a snapshot listener, starts the temporary engine, flushes queued raw messages on success, and stops the temporary engine on close.  
  **Citations:** Saturn `src/main/java/org/saturn/app/listener/snapshot/GsonOnlineSetPayloadParser.java`; `EngineSnapshotSession.java`; `DefaultRoomSnapshotCoordinator.java`.
- **[OBSERVED]** `MineTripOperation` finds the generated nick with `IdentityUtil.sameNick`. If absent, it returns failed result text exactly `Didn't join`. If present, it appends exactly `Password: <password> trip: <trip> \r\n` to relative `trips.txt`, creating the file if necessary. An `IOException` produces failed result text exactly `Failed to save mined trip`. It sends no raw action and no success reply.  
  **Citations:** Saturn `src/main/java/org/saturn/app/listener/snapshot/MineTripOperation.java`; `src/main/java/org/saturn/app/util/IdentityUtil.java` (`sameNick`).
- **[OBSERVED]** The coordinator reports operation reply only when non-null. Its failure fallback only delivers `Unable to complete room operation.` when `replyMessage` is non-null; mine supplies `null`, and its coordinator reply sink merely logs warnings. Therefore mine failures are log-only, not author-facing, except pre-schedule usage/count responses.  
  **Citations:** Saturn `DefaultRoomSnapshotCoordinator.receive`, `publishFailure`; Saturn `MineTripCommandImpl.joinChannel`.
- **[LIMITATION]** Saturn declares `tasks` and periodically checks it, but `scheduleMiningTask` does not append the returned scheduled future. Consequently checker executions see no tasks; there is no user-visible status/error delivery from the checker. Preserve this as a source defect in the parity slice—do not invent successful-mining notices, retries, or task-detail output.  
  **Citation:** Saturn `MineTripCommandImpl` static `tasks`, `execute`, `scheduleMiningTask`, `check`.

### Observed end-to-end sequence

```text
[OBSERVED — Saturn]
admin !mine room start [delay]
  -> ADMIN authorization (configured admin trips)
  -> static miner executor: one periodic task per proxy-map entry, or one nil-proxy task
  -> after 5s, each run generates workflow ID + nick + password
  -> temporary LIST_CMD session joins target room as nick#password
  -> correlated onlineSet/users payload
  -> MineTripOperation finds generated nick
     -> append "Password: ... trip: ... \r\n" to trips.txt
  -> temporary session flushes/closes

admin !mine room count -> author reply with global executor metrics
admin !mine room stop  -> shutdownNow global executor; no author reply
```

## Current Zenbot mapping and gaps

| Source concern | Existing Zenbot seam | Assessment / required direction |
|---|---|---|
| Catalog metadata | `internal/command/registry.go` (`RegisterAll`) has `mine` / sole alias / `ADMIN`; `internal/command/admin_moderator_catalog_guard_test.go` guards this inventory. | **[TEST-BACKED]** Metadata is present. **[LIMITATION]** `handlers.go:newCommand` falls through to `saturnCommand`, and `registry.go:saturnCommand.Execute` handles `mine` as a successful no-op. Replace only this canonical with a concrete command and remove only `mine` from `allowedScopedGenericFallbacks`. |
| Inbound dispatch | `internal/command/dispatch_adapter.go:legacyAdapter.Execute` invokes a definition with `context.Background`; `RegisterUserUtilitiesWithDirectAgent` exposes only named complete canonicals behind runtime type assertions. | **[LIMITATION]** Mine is not registered today, which is correct. **[RECOMMENDED]** append `mine` only when the engine satisfies the new narrow mining capability; retain legacy dispatch's background-context limitation rather than claiming inbound cancellation. |
| Snapshot workflow | `internal/common/room_snapshot.go:RoomSnapshotSubmitter`; `internal/core/room_snapshot.go:SubmitRoomSnapshot`; `internal/listener/snapshot/coordinator.go`. | **[TEST-BACKED]** Command-owned submission, correlated payload handling, terminal close, timeout/cancel/error cleanup, and late-event rejection already exist. Reuse the coordinator; do not create sessions from command code. |
| Temporary session credentials | `internal/listener/snapshot/RoomSnapshotRequest` currently has no nick/password/proxy fields. `session_factory.go:transportSession.Start` joins only `{"cmd":"join","channel":...}`. `cmd/zenbot/main.go:newRoomSnapshotEngineOptions` builds a generic websocket temporary transport. | **[HIGH RISK / LIMITATION]** Existing snapshot sessions cannot join as Saturn's generated `nick#password`; without a request/session credential extension they cannot realize mine. Add only an explicit mining session-auth field/capability (not a broad engine mutation), and prove normal nuke/resurrect requests remain credential-free and behaviorally unchanged. |
| Scheduler / metrics / global stop | No `mine` scheduler, registry, task metrics, or stop lifecycle was found in `internal/command`, `internal/core`, `internal/service`, or `internal/repository/h2`. | **[HIGH RISK / GAP]** Introduce a process-owned scheduler/controller behind a narrow `common` interface. It must serialize lifecycle/state safely and expose global queued/completed/active counts. Do not make a command instance own goroutines or a new executor. |
| Credential persistence | No mine repository, service, H2 schema, or H2 test exists. | **[OBSERVED GAP]** Saturn writes a relative local file. Use a narrow append-only credential sink injected at composition (default path decision must be explicit) rather than H2/repository/service. Do not add a schema/table/migration. |
| Proxies | `internal/config/config.go:Config` and `internal/transport.Config` do not expose Saturn-style proxy strings; `factory.NewCoordinatedSessionFactory` always constructs the configured direct transport. | **[HIGH RISK / BLOCKING DECISION]** Saturn's proxy map is source behavior but does not actually influence its temporary session. Do not add proxy routing speculatively. Either (A) initially support the direct/no-proxy branch only and fail closed when proxy configuration is requested, or (B) obtain an explicit transport/proxy contract before claiming parity. The chosen disposition must be documented and tested. |
| Accepted remote snapshots | `nuke`/`resurrect` use the same `RoomSnapshotSubmitter`; their source and QA handoffs define accepted behavior. | **[STANDARD RISK]** Mine must reuse this substrate without changing request semantics, coordinator cleanup, nuke/resurrect command registration, raw protocols, or their tests. |

## Proposed target architecture and precise file map

### Interfaces and ownership

1. **[RECOMMENDED]** Add `internal/common/mine.go` with a narrow command-facing capability, for example:

   ```go
   type MineController interface {
       StartMine(context.Context, MineStartRequest) error
       StopMine(context.Context, channel string) error
       MineTaskCounts() MineTaskCounts
   }
   type MineStartRequest struct { Channel string; Delay time.Duration }
   type MineTaskCounts struct { TaskCount, Completed, Active int64 }
   ```

   The interface belongs in `common` because command registration needs a capability assertion without importing `core`. `StopMine` retains the source channel argument for logging/audit parity even though stop is process-global. Decide and document concrete Go error treatment for an exhausted/stopped scheduler before implementation; do not silently turn it into a restartable service.

2. **[RECOMMENDED]** Add a process-owned implementation in `internal/core/mine.go`, installed once on the master during composition. It owns the scheduler, first-run five-second delay, repeating delay, global stop, counts, RNG injection, and the callback that submits a mine snapshot. It must not live in `service` or `repository`, and it must not be reconstructed by each command invocation.

3. **[RECOMMENDED]** Add a mining-specific snapshot operation in `internal/listener/snapshot/mine_operation.go`. It receives generated nick/password and an injected append-only sink; it parses the existing `Snapshot`, finds the generated nick with Saturn `IdentityUtil.sameNick` semantics (trim, remove a leading `@`, then locale-root case-insensitive equality), and appends Saturn's exact credential record. It returns `Failed("Didn't join")` or `Failed("Failed to save mined trip")` without direct chat delivery.

4. **[RECOMMENDED]** Extend only `internal/listener/snapshot/RoomSnapshotRequest` and `session_factory.go` with explicit optional temporary join credentials needed by mine, such as a typed `JoinIdentity{Nick, Password}`. `transportSession.Start` must preserve the current credential-less join for nuke/resurrect and emit `nick#password` only when present. Never put mine credentials in logs, replies, H2, request outcome text, or generic command audit fields.

5. **[RECOMMENDED]** Add `internal/command/mine.go`, wire `handlers.go:newCommand` case `"mine"`, delete mine from `registry.go:saturnCommand.Execute` generic no-op grouping, and remove only `"mine"` from `allowedScopedGenericFallbacks`. Keep parsing/status/replies in the command; delegate scheduling and persistence/session mechanics.

6. **[RECOMMENDED]** In `internal/command/dispatch_adapter.go`, append `mine` only under `e.(common.MineController)`. `cmd/zenbot/main.go` must build the coordinator/session factory, append-only sink, and one shared mine controller before `RegisterUserUtilitiesWithDirectAgent`. Factory wiring may be factored into `internal/factory` only if it preserves current snapshot composition and avoids a circular import.

7. **[RECOMMENDED]** Do not add `internal/repository/**`, `internal/repository/h2/**`, `internal/service/**`, SQL, schema, or migration files for mine. The required H2 package test in the baseline is a non-regression check, not an implementation destination.

### Proposed Zenbot sequence

```text
[RECOMMENDED — target]
admin !mine room start [delay]
  -> concrete ADMIN mineCommand; capability + context gate
  -> shared core MineController.StartMine(room, delay)
  -> after 5s, generates credential pair and submits RoomSnapshotRequest
     (target room + typed temporary join credentials + MineTripOperation)
  -> existing coordinator/session lifecycle parses onlineSet, applies operation,
     flushes/closes/unregisters temporary session
  -> operation appends credential record through injected local sink

admin !mine room count -> same command replies with controller global counts
admin !mine room stop  -> controller global shutdown; no success reply
```

## Delivery, errors, and capability gates

- **[RECOMMENDED]** Registration gate: require `common.MineController` only after its composition dependencies are internally complete. If absent, register no `mine` alias. A command constructed directly without the capability returns `FAILED` plus a non-nil configuration error and no chat/raw/output side effect, matching the existing nuke/resurrect capability pattern.
- **[RECOMMENDED]** Preserve Saturn author/whisper behavior for the two direct replies: missing-arity usage and `count`. Do not acknowledge `start`/`stop`; delayed mine-operation errors must remain non-author-facing because the mine request has no reply message.
- **[RECOMMENDED]** Check a direct `Execute(ctx)` context before parsing/submission/scheduling; a pre-cancelled direct invocation must return `FAILED`, `context.Canceled`, no controller call, and no output. Record explicitly that inbound legacy dispatch still uses `context.Background()`.
- **[RECOMMENDED]** Preserve explicit errors from controller/snapshot submission rather than sending a false success reply. Do not call the generic `reply` helper for task failures, because it discards delivery errors and would change the source's log-only mine-failure surface.
- **[STANDARD RISK]** The local credential file contains secrets. Tests must use a temp path and assert record bytes without writing repository-root `trips.txt`. Production path ownership, permissions, rotation, and secret-at-rest policy are an explicit configuration/owner decision—not an H2 migration decision.

## TDD: first RED and test map

Run each named RED in isolation before production code, then implement the smallest Green path. Existing dirty worktree work must not be edited or rebased.

| RED test first | Intended location | Required proof |
|---|---|---|
| `TestMineIsNotRegisteredWithoutMineController` | `internal/command/mine_test.go` | Existing snapshot capability alone does not expose `mine`; a controller exposes exactly alias `mine` and ADMIN role. |
| `TestMineDefaultUsageIsConcreteAdminCommand` | `internal/command/mine_test.go` | `newCommand("mine", ...)` is not `*saturnCommand`; catalog generic-fallback guard no longer lists mine. |
| `TestMineRejectsMissingArgumentsWithExactUsageAndOriginalWhisper` | `internal/command/mine_test.go` | Exact leading-space usage, `FAILED`, no controller call. |
| `TestMineRejectsServingChannelAndUnknownOrBlankModeSilently` | `internal/command/mine_test.go` | Source's silent `FAILED` branches; no scheduling/reply. |
| `TestMineCountRepliesWithGlobalControllerCounts` | `internal/command/mine_test.go` | Exact ` TaskCount: ..., Completed: ..., Active: ...`, source whisper route, `SUCCESSFUL`. |
| `TestMineStartUsesDefaultOrLiteralDelayWithoutImmediateReply` | `internal/command/mine_test.go` | Default 35 seconds, explicit delay forwarded, start successful/no chat. Add malformed/overflow policy test documenting the approved Go adaptation before Green. |
| `TestMineStopDelegatesGlobalShutdownWithoutReply` | `internal/command/mine_test.go` | Channel argument forwarded, `SUCCESSFUL`, no chat. |
| `TestMineDirectExecutionHonorsPreCancelledContextAndMissingCapability` | `internal/command/mine_test.go` | No task/session/output on cancellation or absent controller. |
| `TestMineControllerSchedulesAfterFiveSecondsAndReportsGlobalCounts` | `internal/core/mine_test.go` | First-run delay, repeating schedule, global counts, start callback request content, and no command-owned worker. Use injectable clock/scheduler/RNG; no real 5/35-second waits. |
| `TestMineControllerStopIsGlobalAndPreservesChosenPostStopPolicy` | `internal/core/mine_test.go` | Stop cancels all scheduled workers, preserves a documented non-restart/rejection source parity disposition, and does not leak goroutines. |
| `TestMineOperationWritesExactCredentialLineWhenGeneratedNickIsPresent` | `internal/listener/snapshot/mine_operation_test.go` | Exact `Password: <password> trip: <trip> \r\n` append to temp sink, no raw action/reply. |
| `TestMineOperationReturnsSourceFailureForAbsentNickOrSinkError` | `internal/listener/snapshot/mine_operation_test.go` | Exact `Didn't join` and `Failed to save mined trip`; no chat side effect. |
| `TestMineSnapshotUsesCredentialedJoinAndKeepsExistingJoinsCredentialFree` | `internal/listener/snapshot/session_factory_test.go` | Mine join has target channel and generated `nick#password`; existing nuke/resurrect temporary requests preserve current credential-free join behavior. |
| `TestMineWorkflowCleansTemporarySessionOnSuccessFailureTimeoutAndLatePayload` | `internal/listener/snapshot/coordinator_test.go` | Reuse/extend existing lifecycle assertions; operation errors and all terminal paths close/unregister exactly once. |
| `TestMainCompositionInstallsOneMineControllerAndRegistersMine` | `cmd/zenbot/*_test.go` | Master uses the shared controller with its snapshot coordinator; no replica or temporary session gets its own controller. |

Focused commands after each RED/Green unit:

```sh
go test ./internal/command -run 'Test.*Mine' -count=1
go test ./internal/core -run 'Test.*Mine' -count=1
go test ./internal/listener/snapshot -run 'Test.*Mine|Test.*Coordinator|Test.*Session' -count=1
go test ./cmd/zenbot -run 'Test.*Mine|Test.*RoomSnapshot' -count=1
```

Then run the package and whole-repository acceptance gates selected by the implementing owner. The required pre-implementation baseline command is recorded below.

## Risks and explicit decisions

### High risk

1. **Credentialed join gap:** current temporary transport joins only a channel. Adding mine without controlled credentials would produce a non-source join and make the operation's generated-nick lookup fail. Extend the request/session seam narrowly and test nuke/resurrect regression.
2. **Global scheduler lifetime:** Saturn uses static process-wide executors and global stop. A per-command goroutine leaks work and changes `count`/`stop`; a restartable controller conceals the source's shutdown defect. Define ownership and post-stop policy before implementation.
3. **Secret persistence:** mined passwords must not surface through logs, replies, H2, or test fixtures outside temporary directories. File path/permissions are a required production decision.
4. **Proxy ambiguity:** Saturn models multiple proxies yet never injects one into `EngineSnapshotSession`. Zenbot lacks proxy configuration and routing. Do not claim proxy parity without an approved transport contract.

### Standard risk

1. Integer conversion differs between Java `Long.parseLong` and Go duration conversion. Preserve direct errors (no success reply) and make overflow/malformed behavior an explicit source-owner-approved adaptation test.
2. Saturn's `portMappedByIp` and scheduler checker are static and defective. Do not “fix” duplicates, tracker population, completion announcements, or per-room stop semantics within this slice.
3. Snapshot coordinator/session code is shared with accepted nuke/resurrect. Any extension must keep their request shapes, terminal cleanup, and registration gates unchanged.
4. `legacyAdapter` discards inbound cancellation by design. Only direct concrete-command cancellation may be asserted until dispatch architecture changes separately.

## Explicit no-scope list

- No changes to accepted `nuke` or `resurrect` behavior, aliases, raw payloads, registration gates, tests, or temporary-session lifecycle semantics.
- No generic Saturn catalog activation and no behavior work for `replica`, `replicaoff`, `replicastatus`, `restart`, `shutdown`, `sql`, or `whiskey`.
- No H2 table, repository interface, service, schema, migration, SQL, or audit-log persistence for mined credentials.
- No proxy configuration/routing, SOCKS/HTTP dialer work, proxy retries, or multi-proxy semantics without an explicit approved decision.
- No completion chat, mining success/failure chat, retry policy, task history, per-room task inventory, scheduler restart, or repair of Saturn's unused `tasks` tracker.
- No refactor of the shared command dispatcher, snapshot coordinator, session registry, transport protocol, or core engine beyond the narrow credential/session and mine-controller seams identified above.
- No changes outside this handoff during architecture work: do not alter application/test code, reset, clean, checkout, stage, or commit the dirty worktree.

## Baseline and path verification record

**[OBSERVED]** Before this handoff, the worktree was already dirty with concurrent migration changes and untracked handoffs. This architecture task changes only `.hermes/handoffs/mine-current-architecture.md`.

**[OBSERVED]** Every repository-relative Zenbot citation in this document was read from `/Users/ab/workspace/go-projects/zenbot`; every Saturn citation was read from `/Users/ab/workspace/projects/saturn`. The relevant existing target paths are present, including command registry/dispatch, common snapshot seam, core submitter, snapshot coordinator/session factory, factory/main composition, config, and nuke/resurrect regression tests.

Required read-only baseline (run after this document was written):

```text
$ go test ./internal/command ./internal/service ./internal/repository/h2 -count=1
ok  	zenbot/internal/command	9.345s
ok  	zenbot/internal/service	2.954s
ok  	zenbot/internal/repository/h2	37.193s

$ git diff --check
PASS (exit 0; no output)
```
