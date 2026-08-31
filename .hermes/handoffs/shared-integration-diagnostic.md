# Shared Integration Forensic Diagnostic

## Scope and evidence basis

This report is the root-cause analysis of the five blocking gaps in `shared-integration-qa.md`. It is grounded in the current Zenbot checkout and read-only Saturn `develop` source. It does not modify application code or Saturn.

Evidence labels:

- **[OBSERVED]** current implementation behavior in source.
- **[SATURN-REFERENCE]** behavior in Saturn used as the integration reference.
- **[IMPACT]** deterministic consequence of the observed call path.
- **[RECOMMENDED]** minimum repair boundary, not an assertion that it already exists.

The relevant implementation handoff is `.hermes/handoffs/shared-integration-implementation.md`; the architecture target is `.hermes/handoffs/shared-integration-architecture.md`.

## Executive finding

The accepted transport, managed engine, replica-manager, command-handler, and lifecycle primitives are individually present, but the integration boundaries are incomplete. Specifically:

1. The snapshot factory abstraction stops at an injected callback, before transport-engine construction and event routing.
2. The temporary profile changes only four listeners to dummies; `onlineSet` still uses the host-state listener.
3. The live main registration list does not register `replica`; tests exercise command definitions directly rather than the inbound WebSocket chat path.
4. Transport errors terminate at a log statement in the engine loop; the configured `LifecycleErrors` sink is never assigned or consumed by the engine.
5. Replica start/stop is owned by `ManagedReplicaController`/`ReplicaManager`, outside the only `Lifecycle` created by `main`, so replica failures cannot appear in `Lifecycle.Errors()`.

These are implementation-boundary failures, not merely absent assertions.

## Gap 1 — Coordinated temporary session is not a real transport-backed session

### Root cause

`CoordinatedSessionFactory` is only a registry wrapper around an injected constructor. In `internal/listener/snapshot/session_factory.go:63-85`, the factory requires `New func(context.Context, RoomSnapshotRequest, SnapshotSink) (Session, error)` and calls it with a registry context. It does not own or receive transport configuration, an engine factory, coordinator callbacks, or a session-ID-aware event adapter. The returned `Session` is wrapped only for idempotent `Close` and registry cleanup (`:88-104`).

The advertised runtime dependencies are also missing at the factory seam: `internal/factory/engine_factory.go:19-24` defines `EngineOptions` with `Transport`, `ListenerProfile`, `ReplicaManager`, and `LifecycleErrors`, but no `SnapshotCoordinator` or `SessionRegistry`. A repository-wide search finds no construction of `RoomSnapshotCoordinator` or `CoordinatedSessionFactory` from `main`, the command adapter, or the factory.

### Data-flow propagation

Observed path:

```text
RoomSnapshotCoordinator.Submit (coordinator.go:135-165)
  -> factory.Create(request, sink) (coordinator.go:151)
  -> CoordinatedSessionFactory.Create (session_factory.go:68-85)
  -> registry.Open(context.Background())
  -> injected f.New(ctx, req, sink)
  -> opaque Session only
```

The `sink` reaches the callback, but there is no enforced binding between the transport's inbound `onlineSet` event and that sink. There is also no engine event loop in this path. `RoomSnapshotCoordinator.OnSnapshot`, `OnTransportError`, and `OnClosed` exist (`coordinator.go:167-177`) but nothing in `CoordinatedSessionFactory` invokes them. `find` must scan active workflows by `session.ID()` (`coordinator.go:195-203`), yet the registry ID generated at `session_factory.go:20-42` is not passed into `New` or required to equal `Session.ID()`.

The current focused test (`internal/listener/snapshot/coordinated_factory_test.go:19-42`) proves only registry cleanup and exactly-once underlying `Session.Close`; its fake session has no transport or event source.

### Saturn reference boundary

Saturn's `EngineSnapshotSession.create` constructs a real `EngineImpl` with `EngineType.LIST_CMD` and `ListenerProfile.TEMPORARY_ONLINE_SET`, installs a private `onlineSet` sink, and exposes only start/close/flush/sendRaw (`saturn/src/main/java/org/saturn/app/listener/snapshot/EngineSnapshotSession.java:10-78`). `EngineImpl.registerDefaultPayloadListeners` registers `onlineSet` and returns early for the temporary profile (`saturn/src/main/java/org/saturn/app/facade/impl/EngineImpl.java:102-111`). Zenbot has no equivalent construction or private sink in its coordinated factory.

### Minimal repair plan

**[RECOMMENDED]** Extend the snapshot/factory seam so `Create` receives a coordinator-bound session identity and can construct a temporary `EngineImpl` using the shared `transport.Connection` (`internal/transport/connection.go:40-188`). Add the architecture-specified snapshot coordinator/registry dependencies to `EngineOptions`, or provide an equivalent explicit snapshot factory dependency. The constructed session must:

- allocate a collision-resistant ID and use that exact ID for `Session.ID()` and coordinator routing;
- configure `core.TemporaryOnlineSet`;
- route only `onlineSet` to the supplied `SnapshotSink`;
- translate transport `Errors()` and terminal close into `OnTransportError(id, err)` / `OnClosed(id, ...)` exactly once;
- implement bounded `Start`, `Flush`, `SendRaw`, and idempotent `Close`;
- clean the temporary registry on every success, failure, timeout, and cancellation;
- never call `ReplicaManager.Add`.

### Compatibility constraints

Preserve `Session`, `SessionFactory`, `RoomSnapshotCoordinator`, exact raw payload bytes, and existing registry cleanup semantics where possible. Do not put temporary sessions into the host replica map. Keep `transport.Connection` channel ownership rules: channels are intentionally not closed by `Close`; event loops stop via context/connection state. Preserve DBZ, identity, repository, and permanent-engine behavior.

### Tests required to prove repair

1. A real `httptest` WebSocket server creates a temporary session through the coordinator-bound factory, sends one `onlineSet`, and verifies the operation receives it and emits only its expected raw/chat payload.
2. Assert the host `ActiveUsers`, subscriptions, DBZ state, command registry, and `ReplicaManager.Channels()` are unchanged.
3. Inject a transport read error and assert exactly one coordinator failure with the session ID; repeat close/error ordering and assert no duplicate terminal transition.
4. Exercise success, factory-start failure, timeout, cancellation, and explicit close; assert registry length returns to zero and `Session.Close` is exactly once.
5. Force ID collision (or deterministic ID source) and assert retry/unique session identity.

## Gap 2 — Temporary `onlineSet` still mutates engine active-user state

### Root cause

`NewEngineWithOptions` always assigns `e.OnlineSetListener = listener.NewOnlineSetListener(e, nil)` at `internal/factory/engine_factory.go:67`. The temporary branch at `:68-70` replaces only `UserChatListener`, `UserInfoListener`, `UserJoinedListener`, and `UserLeftListener` with dummies. It does not replace or parameterize `OnlineSetListener`, nor does it install a coordinator sink.

`OnlineSetListener.Notify` parses the users and then, at `internal/listener/online_set_listener.go:35-40`, type-asserts `ReplaceActiveUsers` and calls it. `EngineImpl` implements that method at `internal/core/engine_impl.go:312-323`, replacing `e.ActiveUsers`. Thus `TemporaryOnlineSet` is not isolated for its one allowed inbound event.

### Data-flow propagation

```text
transport.Messages()
  -> EngineImpl.StartContext event loop (engine_impl.go:162-178)
  -> DispatchMessage (engine_impl.go:227-258)
  -> case "onlineSet" -> e.OnlineSetListener.Notify
  -> OnlineSetListener.Notify
  -> EngineImpl.ReplaceActiveUsers
  -> temporary engine's ActiveUsers mutated
```

Because the temporary factory does not exist end-to-end (Gap 1), this mutation is not currently observed in a real session, but it is the deterministic behavior once a temporary engine is connected. Passing `nil` as the callback also proves there is no alternate sink (`engine_factory.go:67`).

### Saturn reference boundary

Saturn's temporary profile registers only `onlineSet` (`EngineImpl.java:102-111`), but `EngineSnapshotSession` overrides that listener with `SnapshotListener`, whose `notify` forwards the raw payload to the workflow sink (`EngineSnapshotSession.java:15-19,61-76`). The temporary workflow therefore does not call normal `setActiveUsers`.

### Minimal repair plan

**[RECOMMENDED]** Make listener construction profile-aware. For `TemporaryOnlineSet`, install a dedicated listener that validates/forwards the payload to `SnapshotSink` (or a typed coordinator adapter) and does not call `ReplaceActiveUsers`, `AddActiveUser`, callback side effects, repository logging, subscriptions, or command listeners. Keep `Permanent` behavior unchanged and keep the temporary profile's other listeners inert.

### Compatibility constraints

Do not alter `ReplaceActiveUsers` identity-copy/dedup behavior for MASTER or ordinary REPLICA. Do not route temporary snapshots through `common.Engine` host-state APIs as a workaround. Preserve `onlineSet` parsing compatibility for `users` and legacy `Users` fields. Temporary engines must not register commands or run host autorun/DBZ/identity side effects.

### Tests required to prove repair

1. Construct a temporary engine with a sink, call `DispatchMessage` with valid `onlineSet`, and assert sink receives the exact payload while `ActiveUsers` remains unchanged.
2. Assert temporary `onlineAdd`, `onlineRemove`, `chat`, and `info` do not invoke permanent listeners.
3. Construct MASTER and REPLICA profiles and assert their existing active-user replacement and permanent listener behavior remains intact.
4. Real temporary WebSocket test: server sends `onlineSet`; coordinator receives it and host active users remain unchanged.

## Gap 3 — No live inbound-chat command path proves MASTER → REPLICA ownership

### Root cause

The concrete `saturnCommand.Execute` branch for `replica` exists in `internal/command/registry.go:63-75`: it type-asserts `ReplicaController`, parses the channel, calls `AddReplica`, and replies. The controller itself constructs, starts, then registers the replica (`internal/core/replica_controller.go:19-38`). However, `RegisterUserUtilities` does not register `replica`, `replicaoff`, or `replicastatus`: its canonical list is `internal/command/dispatch_adapter.go:51-57`, and those canonicals are absent.

`commandDefinitionFor` can still find these definitions from the full catalog (`internal/command/handlers.go:298-309`), which explains why direct command-definition and handler tests pass. That lookup does not populate `EngineImpl.EnabledCommands`. Live dispatch uses `common.BuildCommand`, as shown by `DispatchUserCommand` (`internal/listener/message/handlers.go:134-154`), and `BuildCommand` therefore cannot find `replica` unless it was explicitly registered.

### Data-flow propagation

The intended live path is:

```text
MASTER transport.Messages
 -> StartContext
 -> DispatchMessage("chat")
 -> UserChatListener.Notify
 -> message.DefaultChain
 -> ResolveUserMetadata/Audit/.../DispatchUserCommand
 -> BuildCommand(fields[0], engine, message)
 -> authorization
 -> legacyAdapter.Execute
 -> saturnCommand.Execute("replica")
 -> ManagedReplicaController.AddReplica
 -> ReplicaFactory.NewReplica
 -> replica.StartContext (join)
 -> ReplicaManager.Add
```

The actual `main` path (`cmd/zenbot/main.go:48-68`) constructs MASTER, attaches a controller, calls `RegisterUserUtilities(e)`, and starts only the MASTER lifecycle. Since `replica` is not in the registration list, the chain reaches `BuildCommand` and gets `nil`; no authorization or controller side effect occurs. The existing factory integration test (`internal/factory/engine_factory_test.go:15-67`) directly constructs MASTER and REPLICA against one server and waits for two joins. It does not send inbound `chat`, use `DefaultChain`, or verify manager registration, so it cannot detect the registration boundary failure.

### Saturn reference boundary

Saturn's `MsgChannelCommandImpl` is instantiated from the live command factory and remote operations create an `EngineSnapshotSession` through the coordinator (`saturn/src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java:83-105`). Its `EngineImpl.dispatchMessage` resolves registered payload listeners (`EngineImpl.java:294-311`). The analogous Zenbot proof must cross transport, chat listener, authorization, command registration, and controller—not call `commandDefinitionFor` directly.

### Minimal repair plan

**[RECOMMENDED]** Register the concrete replica command definitions in the live `RegisterUserUtilities` path after the controller capability is attached, preserving aliases and ADMIN role. Ensure `replicaoff` and `replicastatus` are also registered if they are part of the same live slice. Then add a two-endpoint or multiplexed `httptest` integration that sends an authorized MASTER chat command and observes the replica join before/around manager visibility.

### Compatibility constraints

Preserve prefix parsing, metadata resolution, authorization-before-side-effects, whisper authorization response, alias collision validation, trimmed channel validation, host-channel rejection, and rollback on manager-add failure. Do not bypass `DispatchUserCommand` or call controller directly from transport code. Do not register commands on REPLICA or temporary engines. Preserve DBZ conditional registration and existing legacy command adapters.

### Tests required to prove repair

1. Register utilities on a real MASTER with an attached controller and assert `replica`, all aliases, `replicaoff`, and `replicastatus` are in `EnabledCommands`.
2. Send an inbound `chat` JSON command from an authorized active user; assert the controller is called only after authorization and a REPLICA join contains the requested distinct channel.
3. Assert manager status contains the channel and replica `onlineSet` reaches the replica's permanent active-user map.
4. Send unauthorized/unknown/whisper commands and assert no replica is created and authorization reply preserves whisper bit.
5. Assert duplicate/host/blank channels fail without leaked started engines.

## Gap 4 — Transport failures bypass `Lifecycle.Errors()`

### Root cause

`EngineImpl.StartContext` selects `Transport.Errors()` at `internal/core/engine_impl.go:162-178`, but the error branch only logs:

```go
log.Printf("engine %s transport: %v", e.Channel, err)
```

No error sink or lifecycle callback is present on `EngineImpl`. Although `factory.EngineOptions` declares `LifecycleErrors chan<- error` at `internal/factory/engine_factory.go:19-24`, `NewEngineWithOptions` never assigns it to the engine (`:35-78`), and `EngineImpl` has no corresponding field. `main` creates a `Lifecycle` at `cmd/zenbot/main.go:61-69` after factory construction and drains `lifecycle.Errors()`, but no transport-to-lifecycle bridge is supplied.

`Lifecycle.Errors()` only receives errors returned by `l.run`'s asynchronous goroutine (`internal/core/lifecycle.go:70-78`). `Lifecycle.run` starts the engine and then `monitor` checks only periodic `Healthy()` (`:96-133`); it does not consume engine transport errors. A read error can therefore be logged while the runtime loop continues or the transport becomes unhealthy without a terminal lifecycle error until a health tick, and a start/dial failure is returned/retried inside `run` rather than delivered through the configured factory option.

### Data-flow propagation

```text
transport.Connection.readLoop/pingLoop
 -> publish(err) -> Connection.Errors()
 -> EngineImpl.StartContext select
 -> log.Printf only
 -> no Lifecycle.Errors()
```

For dial errors, `Connection.Start` publishes the error (`internal/transport/connection.go:78-85,99-106`) and returns a wrapped error; `EngineImpl.StartContext` returns it to `Lifecycle.run`, which retries and eventually returns it. That terminal return is reported by `Lifecycle.Start`'s goroutine, but it is not the `EngineOptions.LifecycleErrors` path and does not preserve per-event transport observability. The two routes are therefore inconsistent.

### Minimal repair plan

**[RECOMMENDED]** Add an explicit, optional error-reporting dependency to the managed engine (or a transport-event callback) and wire `EngineOptions.LifecycleErrors` in `NewEngineWithOptions`. On each non-nil transport error, report a structured/wrapped error with engine type/channel context, apply a defined terminal policy (usually cancel the managed runtime so `Lifecycle.monitor`/run can reconnect or terminate), and suppress duplicate terminal reports. Alternatively, make `Lifecycle` consume an engine error stream through a capability interface; do not rely on log parsing.

### Compatibility constraints

Keep the legacy `Start`/`Stop`, `common.Engine`, and legacy connection behavior intact for old callers. Preserve normal context cancellation as non-failure (the QA change in `internal/core/lifecycle.go:70-78`). Do not close transport channels from the engine. Avoid blocking on an unbuffered sink; the existing lifecycle error channel is buffered. Do not report expected close errors as failures during intentional shutdown.

### Tests required to prove repair

1. Fake `EngineTransport` emits a read error; assert the configured sink receives it with channel/type context and the event loop follows the documented terminal policy.
2. Real WebSocket server closes or causes a read failure; assert `Lifecycle.Errors()` receives a failure without waiting for log output.
3. Dial failure: assert bounded retry count and exactly one terminal lifecycle error after `MaxRetries`.
4. Cancel/normal `StopContext`: assert no spurious lifecycle error.
5. Run focused tests under `go test -race` to cover sink and runtime state synchronization.

## Gap 5 — Replica failures are outside main's lifecycle observability

### Root cause

`main` creates exactly one `Lifecycle`, whose factory returns the MASTER wrapper (`cmd/zenbot/main.go:21-25,61`). Replica creation is instead performed synchronously inside `ManagedReplicaController.AddReplica` (`internal/core/replica_controller.go:19-38`): construct, `StartContext`, then `ReplicaManager.Add`. A failed replica start is returned to `saturnCommand.Execute` (`internal/command/registry.go:63-75`) and then swallowed into a log by `legacyAdapter.Execute` (`internal/command/dispatch_adapter.go:22-27`). It is not sent to `Lifecycle.Errors()`.

During shutdown, `main` calls `lifecycle.Stop` for MASTER and only afterward calls `manager.StopAll` (`cmd/zenbot/main.go:70-78`). This is deterministic cleanup, but it confirms separate ownership rather than a shared lifecycle. There is no replica lifecycle object, error sink in `ReplicaFactory`, or manager/controller event channel. A replica that starts successfully and later loses transport also has no lifecycle monitor; its `StartContext` loop logs transport errors (Gap 4) and no manager removal/retry/observable failure occurs.

### Data-flow propagation

```text
MASTER inbound chat
 -> command adapter
 -> ManagedReplicaController.AddReplica
 -> ReplicaFactory.NewReplica
 -> replica.StartContext
    -> start error returned to command
    -> legacyAdapter logs and discards command error

running replica transport error
 -> replica EngineImpl event loop log only
 -> no replica Lifecycle / no main error sink / manager entry remains
```

`ReplicaManager` owns membership and stop barriers (`internal/core/replica_manager.go:22-71`) but deliberately stores only `Replica` with `Stop(context.Context)`; it has no health/error observation or restart policy. This is the concrete ownership boundary causing the gap, not a failure of map copying or duplicate rejection.

### Saturn reference boundary

Saturn's host `EngineImpl.stop` explicitly iterates and stops replicas (`saturn/src/main/java/org/saturn/app/facade/impl/EngineImpl.java:247-291`), while `ApplicationRunner` serializes host lifecycle/restart and shutdown (`saturn/src/main/java/org/saturn/ApplicationRunner.java:46-165,180-197`). The Go implementation has host shutdown ordering but no equivalent host-owned replica failure event/restart/observability channel.

### Minimal repair plan

**[RECOMMENDED]** Keep `ReplicaManager` as membership/stop authority, but add an explicit replica lifecycle/error observer at the controller/factory boundary. On replica start failure, publish a contextual error to the host-owned sink before returning the command error; on runtime transport/health failure, publish once, remove or mark the replica deterministically, and apply an explicit no-auto-restart or bounded-restart policy. Have `main` drain the host sink and include replica shutdown errors in the same observable reporting path. If a separate `Lifecycle` per replica is chosen, its errors must be multiplexed into the main-owned sink and its cancellation must be coupled to manager removal.

### Compatibility constraints

Preserve manager rejection of blank/host/duplicate channels, start-before-visibility ordering, rollback on `Add` failure, copied maps, sorted status, and exactly-once stop. Do not make `ReplicaManager` construct engines or access repositories. Do not accidentally restart replicas on normal process cancellation. Keep MASTER as process/repository/DB owner and REPLICA as one-channel engine; do not register global commands or host autorun on replicas.

### Tests required to prove repair

1. Inject a replica constructor whose `StartContext` fails; assert the command returns failure, the manager has no entry, and the host/main error sink receives a contextual replica-channel error.
2. Start a real replica, force server close/read failure, and assert one observable failure, deterministic manager removal/marking, and no goroutine leak.
3. Verify successful command-triggered replica startup is visible only after its join/start succeeds.
4. Verify duplicate/host rejection and `Add` rollback do not emit false success or leak lifecycle errors.
5. Shutdown with MASTER plus multiple replicas; assert all stop once, no cancellation noise is reported, and repository/DB closes after engine ownership is quiescent.

## Explicit scope exclusions

The following are intentionally outside this diagnostic's repair scope and must not be silently folded into the five-gap fix:

- No application-code implementation, refactor, or Saturn modification was performed.
- Remote-room `msgchannel`/`msgroom` success behavior remains unimplemented; the current explicit `remote room delivery is not configured` result in `internal/command/registry.go:53-62` is not treated as a successful path.
- Whiskey proxy discovery/configuration remains unavailable because the current Go config has no proxy source; `registry.go:90-91` is an explicit failure, not part of lifecycle repair.
- DBZ SQL/registration semantics, identity-key deduplication, repository contracts, ZOMBIE substitutions, and unrelated dirty work are preserved and not redesigned.
- Legacy `common.Engine` and legacy `HcConnection` compatibility behavior is not removed; the analysis only identifies the managed path that must own new integration.
- No claim is made that full test/race/vet/build output was re-run in this analysis; the verification evidence in the QA handoff was inspected as evidence of the existing suite, not repeated as a repair.
- No external hack.chat/network endpoint behavior is in scope; tests should use deterministic local `httptest` WebSocket servers.

## Verification of this report

The report was written to `.hermes/handoffs/shared-integration-diagnostic.md`. Application source files were not modified by this analysis.
