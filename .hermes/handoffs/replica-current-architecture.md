# Replica lifecycle parity handoff (bounded post-mine slice)

**Scope.** This is a source-grounded handoff for only `replica` / `replicaoff` / `replicastatus`. Mine is deliberately deferred. Observations are from the dirty Zenbot checkout and the Saturn authority at `/Users/ab/workspace/projects/saturn`; target citations below are repository-relative to Zenbot and Saturn citations are relative to that Saturn source root.

## Decision and bounded outcome

[OBSERVED] Zenbot already has a managed replica lifecycle vertical: construction, WebSocket start, manager ownership, runtime-failure removal, command catalog entries, capability-gated registration, and a real-WebSocket add-path test. It does **not** have Saturn reply/error parity, and the three routes remain generic `saturnCommand` cases (`internal/command/handlers.go:newCommand`; guard allowlist in `internal/command/admin_moderator_catalog_guard_test.go:allowedScopedGenericFallbacks`).

[RECOMMENDED] The next slice should replace only the three generic routes with concrete replica lifecycle command handlers over the existing `command.ReplicaController` / `core.ManagedReplicaController` boundary. Keep the current composition root and lifecycle manager; do not fold in mine, agents, proxies, whiskey, restart/shutdown/sql, credential sessions, or automove semantics.

## Authoritative Saturn contract

### Catalog, role, aliases, arguments, replies, errors

All three Saturn classes inherit `UserCommandBaseImpl`, whose `getAuthorizedRole()` returns `ADMIN`; dispatch authorizes before `execute()` (`src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:83-103,133-135`). Constructors pass `getAdminAndUserTrips(engine)`—the configured admin plus user trip strings—to the base (`.../Util.java:43-48`). This is source behavior, not a recommendation to weaken Zenbot's admin gate.

| Canonical command | Saturn aliases | role | accepted arguments | successful reply | source failure reply / result |
|---|---|---|---|---|---|
| `replica` | `replica`, `bot`, `agent` | ADMIN | first whitespace-delimited argument after command is the channel; trailing arguments are ignored | `started replica in channel: <channel> successfully. Number of replicas: <count>` | no argument: `Example: <prefix>replica lounge`; blank or host channel: `I'm the host bot serving current channel. Example: <prefix>replica lounge`; duplicate: `Channel <channel> already has a replica running.`; each returns FAILED |
| `replicaoff` | `replicaoff`, `offline`, `botoff`, `agentoff` | ADMIN | first post-command argument is channel; trailing arguments ignored | `Successfully shut down replica in channel: <channel>` | no argument: `Example: <prefix>replicaoff lounge`; blank or host channel: `I'm the host bot serving current channel, not a replica.`; absent: `No replica in channel: <channel>`; each returns FAILED |
| `replicastatus` | `replicastatus`, `status` | ADMIN | no required argument; supplied arguments are ignored | `Host room:<host>, replicas active: <count> \nServing channels: <joined channels or none>` | no explicit error path; returns SUCCESSFUL |

Citations: `src/main/java/org/saturn/app/command/impl/admin/ReplicaCommandImpl.java:18-73`; `ReplicaOffCommandImpl.java:15-49`; `ReplicaStatusCommandImpl.java:16-32`.

[OBSERVED] Saturn trims the selected channel. It rejects exactly blank and host-channel input, checks its concurrent map for duplicate/absence, creates a fresh `EngineImpl` with a new H2 connection and `EngineType.REPLICA`, sets channel/nick/password, **adds it to the map before calling `start()`**, and reports success through the host out service (`ReplicaCommandImpl.java:53-73`). Off calls `replica.stop()` then removes its map entry (`ReplicaOffCommandImpl.java:37-47`). Status directly iterates concurrent-map keys; no ordering is imposed.

## Current Zenbot architecture

### Observed ownership and sequence

```text
[OBSERVED inbound command path]
chat payload
  -> listener/message.DispatchUserCommand.Handle
     -> common.BuildCommand(alias, engine, message)
     -> EngineImpl.EnabledCommands alias metadata
     -> IsUserAuthorized(author, ADMIN)
     -> legacyAdapter.Execute (Background context)
     -> Saturn-command switch
        replica       -> ReplicaController.AddReplica
        replicaoff    -> ReplicaController.RemoveReplica
        replicastatus -> ReplicaController.ReplicaChannels

[OBSERVED production composition]
cmd/zenbot/main.go
  -> ReplicaManager(host channel)
  -> ReplicaFactory(config, repository, replica options)
  -> ManagedReplicaController(manager, factory constructor)
  -> master.SetReplicaController(controller)
  -> RegisterUserUtilitiesWithDirectAgent(master, ...)
```

Citations: `internal/listener/message/handlers.go:167-189`; `internal/common/command.go:21-31`; `internal/core/engine_impl.go:RegisterCommand, IsUserAuthorized`; `internal/command/dispatch_adapter.go:40-104`; `cmd/zenbot/main.go:280-336`.

### Lifecycle ownership, concurrency, and errors

[OBSERVED]

* `core.ReplicaManager` owns the authoritative managed set. It protects the map and stopped bit with `sync.RWMutex`; `Channels()` returns a sorted snapshot. `Add` trims, rejects blank/nil, host channel, stopped manager, and duplicates. `Replicas()` / `ManagedEngines()` return copies. (`internal/core/replica_manager.go`.)
* `ManagedReplicaController.AddReplica` validates controller and trimmed channel, constructs first, installs an `EngineImpl` runtime-failure callback, starts the engine, then registers it with the manager. If manager registration fails after start, it calls `StopContext`; construction/start/registration failures are optionally sent to the non-blocking error sink with contextual wrapping. (`internal/core/replica_controller.go:32-60`.)
* Runtime transport failure is reported only once by `EngineImpl.reportTransportError`; the controller callback reports `replica <channel> runtime: ...` then asks the manager to remove it using `context.Background()`. Manager `Remove` deletes under lock **before** calling `Replica.Stop` outside its lock. Thus an unsuccessful stop still leaves the replica absent from the manager. (`internal/core/engine_impl.go:153-169`; `internal/core/replica_controller.go:45-49`; `internal/core/replica_manager.go:41-54`.)
* `StopAll` sets `stopped=true`, snapshots and clears the manager map under lock, then stops outside the lock and returns only the first stop error. It permanently rejects future `Add` calls. (`internal/core/replica_manager.go:56-71`.)
* `EngineImpl.StartContext` serializes per-engine start through `runtimeMu`, starts transport, sends join once, and launches the receive loop. `StopContext` clears the runtime handle, cancels, closes transport, and waits only until the supplied context expires. (`internal/core/engine_impl.go:171-247`.)
* There is no controller-wide add/remove serialization. Concurrent creates for one channel may both construct/start; manager accepts one and the loser is stopped after duplicate registration failure. A remove racing a create before registration sees not found; a status snapshot can observe either side of the mutation. These are observed implications of the separate controller and manager critical sections, not promised UX semantics.
* Normal dispatch authorization is before command execution. `SecurityService` checks the configured/H2 authorization source and denies on lookup error. (`internal/service/security_service.go:53-80`.)

### Current capability gate

[OBSERVED] `RegisterUserUtilitiesWithDirectAgent` adds all three lifecycle canonicals only when `e` implements `command.ReplicaController`; otherwise none of their aliases enter `EnabledCommands`. This is the correct fail-closed exposure boundary. `*core.EngineImpl` satisfies it by delegating to its installed `*ManagedReplicaController`; if the controller has not been installed, direct execution returns `replica controller is not configured`. (`internal/command/dispatch_adapter.go:67-72`; `internal/core/engine_impl.go:260-279`.)

[RECOMMENDED] Preserve the gate exactly. A concrete handler must not be registered merely because the static catalog contains the aliases; capability absence must remain unknown/unexposed at inbound dispatch. Production must continue to install the controller before utility registration (`cmd/zenbot/main.go:294-310`).

## Source-to-target differences requiring this slice

| Area | Saturn source | Zenbot current | parity decision |
|---|---|---|---|
| handler form | dedicated admin classes | generic switch cases | replace only these cases with concrete handlers / factories |
| add ordering | map add, then non-blocking start | construct, start, then manager add | retain Zenbot safer ownership invariant; do not regress to a visible-but-not-started replica |
| remove ordering | stop, then map remove | remove, then stop | retain Zenbot lock-safe delete-before-stop behavior; specify failure as lifecycle error, not Saturn map retention |
| reply text | exact source strings in table | add: ` replicas: N`; off: no success reply; status: ` channel: host replicas: a,z` | implement exact Saturn text, including `none` and `\\n` representation used by `SendChatMessage` |
| errors | source replies and FAILED status | helpers return bare errors; legacy adapter only logs errors | concrete commands should send the exact source failure reply and return FAILED without relying on logged errors as user feedback |
| status order | concurrent-map iteration, unspecified | lexically sorted | preserve deterministic target sort unless literal source order is explicitly required; tests should assert sorted target output |
| context | synchronous Java calls | adapter discards execution context for `context.Background()` | do not broaden this slice; concrete command can accept passed context internally only if legacy adapter contract is separately changed |
| credentials / DB | fresh connection and copies password | shared repository object and copied configuration | do not add per-replica credential/DB behavior in this slice |

## Recommended implementation seam

1. Add a focused concrete command type (one type may dispatch the three canonicals, but separate types are clearer) in `internal/command/replica.go` or a new `replica_commands.go`. Keep the small `ReplicaController` interface as its only lifecycle dependency.
2. Route `newCommand` for the three canonicals to those concrete types. Remove their generic-switch cases and remove exactly those three names from `allowedScopedGenericFallbacks` once their dedicated tests pass.
3. Preserve aliases and `model.ADMIN` catalog metadata in `RegisterAll`; do not add routes outside the existing `ReplicaController` capability gate.
4. Translate only known controller errors into the Saturn-visible validation replies when the condition is distinguishable at command level. Prefer preflight validation for blank/host/duplicate/not-found using parsed channel, `engine.GetChannel()`, and a channel snapshot; still propagate lifecycle construction/start/stop errors rather than inventing a Saturn reply that source never specified.
5. On add success reply with the manager snapshot count after success. On off success reply only after `RemoveReplica` succeeds. Status must snapshot once, render `none` for zero, and use a stable sorted list.

### Proposed sequence

```text
[PROPOSED concrete replica command]
admin chat !replica lounge
  -> DispatchUserCommand authorization (ADMIN)
  -> capability-gated adapter / concrete replica command
  -> parse first channel; reject missing/blank/host/duplicate with Saturn reply
  -> ManagedReplicaController.AddReplica(ctx, lounge)
       -> ReplicaFactory.NewReplica (REPLICA config copy)
       -> EngineImpl.StartContext + join
       -> ReplicaManager.Add(lounge, managed engine)
  -> snapshot ReplicaChannels
  -> reply: started replica in channel: lounge successfully. Number of replicas: 1

admin chat !replicaoff lounge
  -> same authorization and parse/preflight
  -> ManagedReplicaController.RemoveReplica
       -> ReplicaManager deletes entry under lock
       -> StopContext outside lock
  -> reply only when stop returned nil

admin chat !replicastatus
  -> authorization -> snapshot sorted ReplicaChannels
  -> reply host/count/list-or-none
```

## High / Standard risk assessment

**High risk**

* Changing manager ordering or holding its mutex while transport stop/start runs can deadlock or make status/other lifecycle operations block. Keep lifecycle calls outside `ReplicaManager.mu`.
* Weakening/removing the `ReplicaController` registration gate exposes privileged aliases on incompletely composed engines.
* Treating a successful `StartContext` as registered ownership before `ReplicaManager.Add` succeeds creates leaked connections on duplicate races; preserve the current stop-on-registration-failure compensation.
* Changing runtime-failure callback behavior can cause duplicate reporting, stale manager entries, or recursion through `Remove`/`Stop`.

**Standard risk**

* Exact reply punctuation, prefix interpolation, whisper preservation, `none`, and literal newline encoding are compatibility-sensitive.
* Preflight duplicate/not-found checks race with lifecycle mutations; controller errors remain authoritative after preflight.
* `ReplicaFactory.NewReplica` assumes non-nil `Config`; this production assumption should be explicit in unit tests rather than silently broadened here.
* Existing status sorting differs from Saturn's unspecified concurrent-map iteration but is a desirable deterministic target contract.

## Precise file map

| File | role in slice | expected change |
|---|---|---|
| `internal/command/replica.go` | controller interface, parse/render/off helpers | likely extend or place concrete handlers beside it |
| `internal/command/handlers.go` | `newCommand` factory and generic fallback | route the three commands to concrete types; remove generic cases |
| `internal/command/registry.go` | Saturn catalog, aliases, roles, generic switch | preserve catalog; remove three generic execute cases if concrete handlers own execution |
| `internal/command/dispatch_adapter.go` | capability-gated runtime registration | preserve gate; add focused registration test only if necessary |
| `internal/command/replica_test.go` | current helper-only test | replace/expand into exact reply, failure, and status tests |
| `internal/command/admin_moderator_catalog_guard_test.go` | source alias/role and generic fallback guard | remove only the three names from fallback allowlist after conversion |
| `internal/command/real_websocket_replica_test.go` | inbound add vertical | retain; extend only if a real response assertion is practical |
| `internal/command/real_websocket_replica_failure_test.go` | runtime cleanup/error sink | retain unchanged as lifecycle regression coverage |
| `internal/core/replica_controller.go` | construct/start/register/remove/error sink | no planned change; behavior is implementation constraint |
| `internal/core/replica_manager.go` | ownership, locking, status snapshots | no planned change; behavior is implementation constraint |
| `internal/core/engine_impl.go` | controller delegation, managed lifecycle | no planned change |
| `internal/factory/replica_factory.go` | replica engine configuration copy | no planned change |
| `cmd/zenbot/main.go` | production controller composition and shutdown | no planned change |

## First RED tracer/test matrix

Write these tests before production changes. Use direct concrete-command tests for reply/status/error coverage and retain the real-WebSocket tests for actual transport wiring.

| RED test | setup / action | required assertion |
|---|---|---|
| `TestReplicaRegistersOnlyWithReplicaController` | run `RegisterUserUtilities` with and without a `ReplicaController` fake | all aliases for all three commands absent without capability and present with it; role is ADMIN |
| `TestReplicaAliasesAndFirstArgumentUseSaturnContract` | execute `replica`, `bot`, `agent` with `!<alias> lounge ignored` | fake receives only `lounge`; each success reply exactly `started replica in channel: lounge successfully. Number of replicas: 1` |
| `TestReplicaSaturnFailuresReplyAndFail` | missing, whitespace, host, duplicate variants | exact four source-facing replies from the contract table; FAILED status; no add call after preflight failure |
| `TestReplicaOffAliasesRepliesAndFailures` | each off alias; missing, blank, host, absent, successful stop | exact source messages; success reply only after successful remove |
| `TestReplicaStatusExactReplyForNoneAndSortedChannels` | fake returns zero, then unsorted `z,a` | `Host room:host, replicas active: 0 \\nServing channels: none`; then count 2 and deterministic `a, z` (record the deliberate target-order choice) |
| `TestConcreteReplicaCommandsAreNotGenericFallbacks` | instantiate definitions from `commandDefinitionFor` | each is not `*saturnCommand`; update bounded fallback allowlist accordingly |
| `TestReplicaLifecycleFailureDoesNotClaimSuccess` | controller `AddReplica` / `RemoveReplica` returns construction/start/stop error | FAILED, no success chat; error returned for adapter logging/error ownership |
| retain `TestRealInboundWebSocketReplicaCommandReachesManager` | real inbound `!replica requested-room` | manager contains room and second WebSocket opens |
| retain `TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry` | close replica transport | manager cleanup and exactly one contextual runtime error |

## Explicit exclusions

Do **not** modify or take requirements from: `mine`; credential sessions; proxy selection/failover; `whiskey`; `restart`; `shutdown`; `sql`; agents or live-agent composition; automove command semantics or state transitions. Automove's existing use of the managed replica controller is evidence only that the lifecycle seam exists, not proof of command parity.

## Evidence limitations and verification record

[TEST-BACKED] Existing relevant tests cover manager ownership/snapshots and stopped state (`internal/core/replica_manager_test.go`), helper parsing/sorted status (`internal/command/replica_test.go`), real inbound creation (`internal/command/real_websocket_replica_test.go`), runtime-failure cleanup/report-once (`internal/command/real_websocket_replica_failure_test.go`), factory independent WebSockets (`internal/factory/engine_factory_test.go`), and source-shaped aliases/roles (`internal/command/admin_moderator_catalog_guard_test.go`). They do not currently assert Saturn-exact replica replies or all error branches; that is why the RED matrix is first.

[OBSERVED] The worktree was already dirty before this handoff. This document is the only intended new artifact; no application or test source is changed by this architecture task.

Verification executed from `/Users/ab/workspace/go-projects/zenbot` after the handoff was written:

```text
$ go test ./internal/command ./internal/core ./internal/factory -count=1 && git diff --check
ok  \tzenbot/internal/command\t9.550s
ok  \tzenbot/internal/core\t1.274s
ok  \tzenbot/internal/factory\t0.472s
```

`git diff --check` emitted no output and returned success. Citation QA resolved the cited Zenbot target paths and inspected the three named Saturn authority files; required handoff headings were present.
