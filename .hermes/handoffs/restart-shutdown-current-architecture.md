# Restart / shutdown parity: source-grounded architecture decision

## Verdict

**Do not implement restart/shutdown parity next without an explicit product decision.** The aliases and ADMIN role are source-known and the target has a reusable transport lifecycle primitive, but the privileged command's required *process ownership and terminal semantics* are not defined by the current Zenbot composition. Wiring either command directly to `core.Lifecycle` would be unsafe and would not be Saturn-equivalent: the production lifecycle factory reuses one master engine, does not own replicas/agent/H2 teardown, and the engine's one-time join state is not reset by `StopContext`.

This is an architecture-only handoff. All source facts are tagged **[OBSERVED]** or **[TEST-BACKED]**; proposed work is tagged **[RECOMMENDED]**.

## Authority, aliases, role, and replies

| Canonical | Exact Saturn aliases | Required role | Direct chat reply | Status / failure behavior |
|---|---|---|---|---|
| `restart` | `restart`, `reload`, `re` | ADMIN | none | calls lifecycle; returns `SUCCESSFUL` even if lifecycle throws |
| `shutdown` | `exit`, `quit`, `shutdown` | ADMIN | none | calls lifecycle; returns `SUCCESSFUL` even if lifecycle throws |

- **[OBSERVED]** `RestartCommandImpl` and `ShutdownCommandImpl` both extend `UserCommandBaseImpl`, pass `getAdminAndUserTrips(engine)` to their bases, and inherit its ADMIN minimum role. The base command dispatcher authorizes before invoking the concrete command.  
  **Citations:** Saturn `src/main/java/org/saturn/app/command/impl/admin/RestartCommandImpl.java:16-38`; `.../ShutdownCommandImpl.java:16-38`; `src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:83-103,128-135`; `src/main/java/org/saturn/app/util/Util.java:43-48`.
- **[OBSERVED]** Neither source command parses or validates arguments and neither sends an author-facing acknowledgement. Each catches every `Exception`, logs it, and still returns `Optional.of(Status.SUCCESSFUL)`. This means a caller has no in-chat success/failure proof.  
  **Citations:** Saturn `RestartCommandImpl.execute`; `ShutdownCommandImpl.execute`.
- **[OBSERVED]** Zenbot preserves the exact aliases and `model.ADMIN` metadata in `internal/command/registry.go:RegisterAll`; the source-shaped catalog guard asserts them. However, `handlers.go:newCommand` leaves both on `*saturnCommand` and `saturnCommand.Execute` performs no action for them. `RegisterUserUtilitiesWithDirectAgent` does not include either canonical, so no aliases are currently exposed through normal inbound registration.  
  **Citations:** `internal/command/registry.go:RegisterAll,saturnCommand.Execute`; `internal/command/handlers.go:newCommand`; `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent`; `internal/command/admin_moderator_catalog_guard_test.go:saturnAdminModeratorCatalog,allowedScopedGenericFallbacks`.
- **[TEST-BACKED]** Normal Zenbot inbound dispatch resolves an active user, authorizes against the command role before execution, replies to a known unauthorized author, and then calls the registered command. The adapter itself invokes concrete Saturn-command execution with `context.Background()`.  
  **Citations:** `internal/listener/message/handlers.go:ResolveUserMetadata,DispatchUserCommand.Handle`; `internal/listener/message/dispatch_authorization_test.go:TestDispatchUserCommandRejectsUnauthorizedPrincipal`; `internal/command/dispatch_adapter.go:legacyAdapter.Execute`.

## Exact Saturn process behavior

### Restart

- **[OBSERVED]** The command calls `ApplicationLifecycle.getInstance().restartHost()`. `ApplicationLifecycle` requires a runner binding and delegates to `ApplicationRunner.restartHostNow()`; missing binding throws `IllegalStateException`, which the command catches and nevertheless reports successful status.  
  **Citations:** Saturn `src/main/java/org/saturn/ApplicationLifecycle.java:16-38`; `RestartCommandImpl.execute`.
- **[OBSERVED]** `restartHostNow()` synchronizes on a lifecycle lock, calls `restartHost()`, logs a clean restart only after it returns, and only catches `InterruptedException`. `restartHost()` stops the existing host if present, creates a **fresh** host engine, and starts it.  
  **Citations:** Saturn `src/main/java/org/saturn/ApplicationRunner.java:143-159,161-177`.
- **[OBSERVED]** Source host stop calls `host.stop()`, clears its host reference, waits one second, clears the static host reference, waits another second, and asks the runtime for GC. A source restart replaces the host; it does not execute an OS-level restart or `System.exit`.  
  **Citation:** Saturn `ApplicationRunner.stopHostIfRunning`.
- **[LIMITATION]** The source log says “Host and replicas restarted cleanly,” but the inspected `ApplicationRunner.restartHost` only creates/starts the host. No direct replica stop/recreate orchestration was found on that path. Do not infer replica-restart behavior from the log message.  
  **Citation:** Saturn `ApplicationRunner.restartHost,createHostEngine,stopHostIfRunning`.

### Shutdown

- **[OBSERVED]** The command calls `ApplicationLifecycle.shutdown()`, which delegates to `ApplicationRunner.stopApplication()`. It does **not** call `System.exit`.  
  **Citations:** Saturn `ApplicationLifecycle.shutdown`; `ShutdownCommandImpl.execute`.
- **[OBSERVED]** `stopApplication()` shuts down the static health-check scheduler, waits up to ten seconds, force-shuts it on timeout/interruption, then calls `stopBot()`. `stopBot()` takes the lifecycle lock and calls the same host-stop path. The JVM can remain alive after this command; shutdown-hook registration is separate and calls `stopApplication()` again when the process later exits.  
  **Citations:** Saturn `ApplicationRunner.stopApplication,stopBot,shutdownHook`; `STOP_WAIT_TIMEOUT_SECONDS`.
- **[OBSERVED]** `autoReconnect` controls whether Saturn starts a scheduled health check. That health check serializes under the lifecycle lock and invokes the same host restart after an unhealthy host.  
  **Citation:** Saturn `ApplicationRunner.start,healthCheck,scheduleHealthChecks`.

No focused Saturn tests for these command or lifecycle classes were located under Saturn `src/test`; the contract is direct-source-derived.

## Current Zenbot lifecycle and mismatch

### Observed production ownership

```text
[OBSERVED — Zenbot startup]
main signal context (SIGINT/SIGTERM)
  -> construct H2 server/repository
  -> construct MASTER EngineImpl + room agent + ReplicaManager/controller
  -> register concrete, capability-gated commands
  -> Lifecycle(factory returns lifecycleEngine{same master EngineImpl})
  -> Lifecycle.Start(signal context)
  -> wait for signal context cancellation
  -> roomAgent.Close()
  -> Lifecycle.Stop(15s context)
  -> ReplicaManager.StopAll(remaining context)
  -> deferred H2 DB/server close
```

- **[OBSERVED]** `cmd/zenbot/main.go` alone owns the signal context, database/H2 defers, `roomAgent`, `ReplicaManager`, master engine, and `core.Lifecycle`. The `Lifecycle` value is a local variable and is not installed on the engine or passed to command composition. Only SIGINT/SIGTERM cancellation reaches the main shutdown branch.  
  **Citation:** `cmd/zenbot/main.go:259-337`.
- **[OBSERVED]** Main constructs `core.NewLifecycle(func() core.LifecycleEngine { return lifecycleEngine{e} }, ...)`, where `e` is the same pre-built master every time. The lifecycle's restart calls `Stop(ctx)` then `Start(ctx)`; it is a managed engine/transport cycle, not a factory-created process/host replacement.  
  **Citations:** `cmd/zenbot/main.go:lifecycleEngine,new main lifecycle factory`; `internal/core/lifecycle.go:Start,Stop,Restart`; `internal/core/lifecycle_test.go:TestLifecycleStartStopRestartIsBounded`.
- **[OBSERVED]** `EngineImpl.StartContext` sends the join only when `joined.Swap(true)` is false. `StopContext` cancels/closes the transport but never resets `joined`. Therefore restarting this same `EngineImpl` through the existing `Lifecycle.Restart` can start a transport receive loop without emitting a new join payload.  
  **Citation:** `internal/core/engine_impl.go:StartContext,StopContext`.
- **[OBSERVED]** Main's orderly shutdown first closes the room-agent runtime, then stops the master lifecycle, then calls terminal `ReplicaManager.StopAll`; database/server teardown is deferred. `ReplicaManager.StopAll` permanently rejects later replica additions. None of those operations is part of `Lifecycle.Restart`.  
  **Citations:** `cmd/zenbot/main.go:327-337`; `internal/core/replica_manager.go:StopAll`; `internal/agent/runtime/runtime.go:Close` (shutdown contract).
- **[OBSERVED]** `core.Lifecycle` has retries/health monitoring for one `LifecycleEngine`. On health failure it stops the unhealthy instance and calls its factory again; under current main composition that again wraps the same master engine. Config has `AutoReconnect` and a health interval fields, but main uses hard-coded lifecycle retry settings and does not wire those config fields into this lifecycle.  
  **Citations:** `internal/core/lifecycle.go:NewLifecycle,run,monitor`; `internal/config/config.go:Config`; `cmd/zenbot/main.go:313`.
- **[TEST-BACKED]** The current lifecycle tests exercise a fake engine's bounded Start/Stop/Restart and confirm cancellation is not reported as an error. They do not test master re-join, signal-triggered full teardown, H2 cleanup, agent closing, replicas, or an inbound privileged command.  
  **Citation:** `internal/core/lifecycle_test.go`.

### Consequences

1. **[OBSERVED GAP]** A command receives only `common.Engine`, whose contract exposes `Start`/`Stop` but neither context-aware process lifecycle action nor a host/supervisor interface. The command cannot safely reach main's local `Lifecycle`, signal cancellation, room-agent closure, replica manager, or DB cleanup.  
   **Citations:** `internal/common/engine.go:Engine`; `cmd/zenbot/main.go`.
2. **[OBSERVED GAP]** Calling `Engine.Start`/`Stop` in a command would use the older blocking connection path rather than the managed production transport lifecycle; calling `StartContext`/`StopContext` would require a core type assertion and still omit process-owned components.  
   **Citations:** `internal/common/engine.go:Start,Stop`; `internal/core/engine_impl.go:Start,Stop,StartContext,StopContext`.
3. **[OBSERVED GAP]** Existing `Lifecycle.Restart` is a useful lower-level seam for tests, but is not a safe implementation substitute for process restart because it reuses the master, misses re-join, does not recreate dependencies, and races semantically with the still-running room agent and replicas.
4. **[OBSERVED]** `help` currently advertises restart/reload and shutdown/exit text despite normal registration excluding those commands. This documentation/output inconsistency must not be converted into privileged exposure merely to align help.  
   **Citation:** `internal/command/help.go` versus `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent`.

## Required product decisions (blockers)

An explicit owner decision is required before code work because the following choices alter externally observable operations and security semantics; Saturn source does not choose a safe target policy.

1. **Restart meaning:** reconnect/rebuild the master transport only; rebuild the whole in-process application graph; or request an external supervisor/container to replace the process. The current managed lifecycle supports only an incomplete same-engine transport cycle.
2. **Shutdown meaning:** graceful in-process stop while keeping the Go process alive (nearest Saturn reading), process termination after cleanup, or supervisor-directed stop. A chat command must not silently choose `os.Exit`, signal itself, or kill a process that may host other work.
3. **Scope and ordering:** whether restart/shutdown affects room-agent work, replicas, auto-move state, H2 server/DB, temporary snapshot sessions, and deferred resource cleanup; whether replicas are stopped or reconstructed on restart.
4. **Caller feedback / delivery:** source sends no reply and returns success after caught errors, but graceful action can close the transport before an acknowledgement flushes. Decide whether strict no-reply parity is intended or whether a pre-action acknowledgement/failure reply is required.
5. **Failure/retry policy:** what to report and whether command requests are idempotent/coalesced while a restart/shutdown is in progress; do not expose Saturn's unconditional-success/log-only defect without approval.
6. **Authority and audit policy:** retain catalog ADMIN dispatch authorization as the minimum; decide whether configured admin trips, H2 roles, a second operational allowlist, whisper-only execution, confirmation, and audit events are required for a process-affecting action.

## Recommended architecture after approval

### Capability gate and ownership

- **[RECOMMENDED]** Add a narrow process-owned capability in `internal/common` (for example `HostLifecycleController`) with context-aware `RequestRestart` and `RequestShutdown` methods. It belongs outside `command`/`core` implementation details so command registration can gate without importing `cmd/zenbot` or type-asserting `*core.EngineImpl`.
- **[RECOMMENDED]** Implement that capability at the composition root or in a dedicated supervisor component that main owns. It must serialize lifecycle requests, expose an in-progress/terminal state, and be given explicit callbacks for the approved shutdown/restart sequence. It must never be a command-owned goroutine and must not directly use `os.Exit`, self-signals, or a raw `exec` restart.
- **[RECOMMENDED]** Register `restart` and `shutdown` only when the master engine implements the approved capability **and** main has installed the complete supervisor before `RegisterUserUtilitiesWithDirectAgent`. Without it, expose none of the aliases; direct construction without the capability must fail closed with no process/output side effect.
- **[RECOMMENDED]** Keep these commands absent from `AgentCommandGateway` and from LLM/agent capability grants. The current gateway permits a fixed public-command allowlist only, and must not be broadened to privileged host control.  
  **Citation:** `internal/command/agent_gateway.go:publicAgentCommandAliases,Execute`; `internal/agent/api/api.go:Capability`.

### Graceful paths to decide and encode

```text
[RECOMMENDED — only after product approval]
ADMIN inbound command
  -> normal DispatchUserCommand authorization
  -> capability-gated concrete command
  -> HostLifecycleController serializes request

shutdown:
  -> stop admission / close room agent
  -> stop master and all replicas with bounded contexts
  -> stop temporary sessions and DB/H2 in chosen order
  -> either remain quiescent OR terminate/request supervisor (product choice)

restart:
  -> execute approved full stop sequence
  -> build a fresh, join-capable master graph and approved replica policy
     OR request external supervisor replacement
  -> do not reuse current same-engine Lifecycle.Restart as parity implementation
```

- **[RECOMMENDED]** If an in-process restart is selected, factor main's construction into a fresh-instance factory; recreate/reset any one-shot transport state and explicitly decide persistence of services/state. Do not reset `EngineImpl.joined` as an isolated patch: that would hide only one defect while leaving agent, replica, and resource ownership undefined.
- **[RECOMMENDED]** If process termination is selected, the controller must run cleanup first and surface a bounded error policy. This architecture task did not invoke, propose executing, or test a real termination action.

## File map

| Path | Current responsibility / expected role |
|---|---|
| `internal/command/registry.go` | Preserve source catalog aliases/ADMIN metadata; remove restart/shutdown from generic no-op only after concrete handlers pass. |
| `internal/command/handlers.go` | Route canonicals to concrete lifecycle command types only after approved capability exists. |
| `internal/command/dispatch_adapter.go` | Add exact capability-gated registration only; do not statically expose privileged aliases. |
| `internal/command/restart_shutdown.go` (new, suggested) | Small parse-free command handlers, context/capability checks, approved reply/status policy; no OS/process calls. |
| `internal/common/host_lifecycle.go` (new, suggested) | Narrow command-facing controller contract and request/result types. |
| `cmd/zenbot/main.go` | Current sole composition/ownership location; must install a supervisor and own approved sequence, not leak it through `EngineImpl`. |
| `internal/core/lifecycle.go` | Existing per-engine lifecycle evidence/test seam; not sufficient for host/process parity unchanged. |
| `internal/core/engine_impl.go` | Managed transport state, including one-shot `joined`; constraints for any fresh-instance design. |
| `internal/core/replica_manager.go`, `internal/core/replica_controller.go` | Replica termination/ownership constraints. |
| `internal/agent/runtime/runtime.go` | Room-agent shutdown/cancellation constraint. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Preserve Saturn row; remove only approved concrete routes from generic fallback allowlist. |
| `internal/command/help.go` | Reconcile advertised privileged commands only after exposure/behavior is approved. |

## First RED and test approach (no real restart/kill)

**First RED:** `TestRestartShutdownAreNotRegisteredWithoutHostLifecycleController` in a new `internal/command/restart_shutdown_test.go`.

It should prove that an engine lacking the controller exposes none of `restart`, `reload`, `re`, `exit`, `quit`, or `shutdown`; with a recording fake controller it exposes all aliases at ADMIN and resolves to concrete handlers rather than `*saturnCommand`. This fails against the current registration implementation because it has no lifecycle capability or registration path.

Then add deterministic tests with an injected fake controller, never a real process action:

1. Alias/role and normal unauthorized inbound dispatch: each exact alias is ADMIN and no controller call occurs for a denied active user.
2. Direct execution: pre-cancelled context makes no controller call/output; missing capability fails closed with no OS signal, `os.Exit`, `exec`, or raw transport action.
3. Approved policy tests: exact no-reply/source-success-on-controller-error behavior **only if** the product explicitly accepts it; otherwise assert the selected acknowledgement/error contract before implementation.
4. Controller tests: concurrent restart/shutdown requests are serialized/coalesced according to the decision; timeouts/cancellation run only injected cleanup callbacks; shutdown becomes terminal only if that policy is selected.
5. Composition tests: main installs the controller before command registration; requested shutdown/restart invokes callbacks in the approved agent/master/replica/H2 order. Use fakes/channels and bounded contexts, not real signals, process kills, socket production endpoints, or H2 server termination.
6. Regression: lifecycle tests must cover a fresh master or a supervisor request, not treat the current same-engine `Lifecycle.Restart` test as end-to-end parity proof.

## Risk rating

**High risk — blocked pending product decision.** These aliases grant an ADMIN chat actor control over application availability. The target has partial transport lifecycle mechanics but no command-to-process ownership boundary, and an incorrect implementation can leave a bot unjoined, orphan agent/replica work, prematurely tear down H2, falsely claim success, or terminate an embedding process. A narrow capability gate and fake-driven tests reduce implementation risk only after the operational contract is approved.

## Evidence and baseline record

- **[OBSERVED]** Recent accepted target verticals are `nuke`, `resurrect`, `automove`, and the replica family; their shared snapshot/replica machinery is evidence, not permission to fold restart/shutdown into those slices. Mine remains deferred and was not used as implementation scope.  
  **Citations:** `.hermes/handoffs/nuke-resurrect-current-architecture.md`; `.hermes/handoffs/automove-current-architecture.md`; `.hermes/handoffs/replica-current-architecture.md`; `.hermes/handoffs/mine-current-architecture.md`.
- **[OBSERVED]** This dirty worktree had pre-existing application/test modifications and handoffs. This pass creates only `.hermes/handoffs/restart-shutdown-current-architecture.md`; it does not modify application/test code and does not invoke restart, shutdown, process kill, or restart.
- **[TEST-BACKED]** Required read-only baseline passed after source inspection:

```text
$ go test ./internal/command ./cmd/zenbot ./internal/core -count=1 && git diff --check
ok  	zenbot/internal/command	9.722s
ok  	zenbot/cmd/zenbot	1.048s
ok  	zenbot/internal/core	0.841s
# git diff --check: exit 0, no output
```

- **[QA]** Citation QA resolved every existing repository-relative Zenbot citation and inspected each cited Saturn authority/lifecycle source named above. The three intentionally absent paths in the file map/RED section are explicitly marked `(new, suggested)`; they are recommendations, not current-source citations. The only target write is this handoff.
