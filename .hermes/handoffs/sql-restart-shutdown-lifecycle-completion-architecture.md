# Lifecycle completion architecture — production fresh-master composition

**Status:** implementation architecture only; no Go source/test edit is authorized by this handoff.
**Purpose:** remove QA's remaining full-lifecycle blocker: production `main` installs `hostSupervisor` with `build`, `start`, and `rebind` all nil, so a successful command request cannot produce a new join-capable master.
**Baseline:** dirty branch `migration/saturn-zenbot-parity`, commit `f9079ca`; this handoff is the only intended new file.

## 1. Evidence and non-negotiable contract

### [TEST-BACKED] Current blocker

`cmd/zenbot/main.go:258-266` constructs:

```go
supervisor := NewHostSupervisor(e, retire, nil, nil, nil)
hostLifecycle := newProductionHostLifecycle(supervisor)
e.SetHostLifecycleController(hostLifecycle)
```

`hostSupervisor.Restart` correctly fails before retirement when `build == nil` (`cmd/zenbot/host_supervisor.go:34-64`; `TestHostSupervisorRestartWithoutFreshBuilderFailsBeforeRetiringMaster`). QA therefore prevented destructive partial restart, but did not produce Saturn's replacement host. See `.hermes/handoffs/sql-restart-shutdown-parity-qa.md:5-10,45-47`.

### [OBSERVED] Required source behavior already approved

* Restart replaces **only the host**: stop current host, construct fresh host, start it; it does not stop/recreate replicas. Shutdown stops the host and does not call `System.exit` or close the database. `.hermes/handoffs/sql-restart-shutdown-parity-architecture.md:127-178` records the inspected Saturn authorities and binding decision.
* Restart/shutdown requests are already command-gated and deferred until dispatch has returned: `internal/core/host_lifecycle.go:40-125`, `internal/listener/message/handlers.go:173-226`, `internal/command/restart_shutdown.go`.
* A new `*core.EngineImpl` is necessary. `StartContext` emits join only once through `joined.Swap(true)`, while `StopContext` does not reset it (`internal/core/engine_impl.go:173-249`). Reusing the old instance cannot be a fresh join-capable host.
* Command shutdown must remain non-exiting and host-only. `hostSupervisor.Shutdown` already retires only its current master (`cmd/zenbot/host_supervisor.go:66-78`); it must not call `roomAgent.Close`, `ReplicaManager.StopAll`, H2 stop, or DB close.

## 2. Missing composition: one mutable master binding and one graph builder

### 2.1 Introduce a main-owned master binding

Add an unexported, synchronization-safe `masterBinding` in `cmd/zenbot`:

```go
type masterBinding struct {
    mu sync.RWMutex
    e  *core.EngineImpl
}
func (b *masterBinding) Current() *core.EngineImpl
func (b *masterBinding) Rebind(next *core.EngineImpl) // next must be non-nil
```

It is the sole mutable reference to the current master. It replaces the unsynchronized captured local `var master *core.EngineImpl` at `main.go:232-244`.

The snapshot reply callback must read `binding.Current()` for every send, rather than close over the initial master:

```go
roomSnapshotReplySink(func(author, reply string, whisper bool) (string, error) {
    current := binding.Current()
    if current == nil { return "", errNoCurrentMaster }
    return current.SendChatMessage(author, reply, whisper)
})
```

This keeps the single snapshot coordinator/session registry alive (no temporary-session shutdown on restart) while routing replies to the replacement host. It is source-compatible host replacement, not a snapshot-policy change. Current capture site: `cmd/zenbot/main.go:231-239`; reply use: `internal/listener/snapshot/coordinator.go:248-252,307-313`.

### 2.2 Extract `buildMasterGraph` from `main`

Extract the existing master-only composition into a closure/function owned by `main`:

```go
type masterGraph struct { Engine *core.EngineImpl }

type masterGraphBuilder func(context.Context) (*masterGraph, error)
```

`buildMasterGraph(ctx)` must do exactly the following for **each** master generation:

1. Call `factory.NewEngineWithOptions(model.MASTER, c, db, masterOptions)` to obtain a new engine/transport/join state. Reuse process-owned `c`, `db`, `transportErrors`, `snapshotOptions`/its coordinator, and shared auto-move state. Factory construction evidence: `internal/factory/engine_factory.go:46-110`.
2. Install the shared, non-terminal `ReplicaManager` through a new `core.ManagedReplicaController` using the existing `ReplicaFactory` closure; install the existing support relay. This gives the new host the same access to already-running replicas without stopping or recreating them. Current one-time sites: `cmd/zenbot/main.go:252-256`; manager non-terminal behavior is `internal/core/replica_manager.go:22-55`.
3. Install the already-created `HostLifecycleController` on the new engine **before** utility registration, then call `command.RegisterUserUtilitiesWithDirectAgent(newEngine, roomAgent.DirectSubmitter)`. The controller is required for restart/shutdown registration (`internal/command/dispatch_adapter.go:81-92`) and must be available on every fresh master.
4. Install a new user-chat listener for the new engine, using the persistent room-agent participation adapter described in §3. This listener is master-local and must never be retained from the old engine.
5. Return the unstarted graph. Do not start transport, create/close the room agent, stop replicas, close H2/DB, or close temporary sessions here.

The builder returns an error rather than calling `log.Fatal`, so `hostSupervisor` can preserve existing controller error handling (logged internally and command-success/no-reply). Initial process startup remains the only place that may use `log.Fatal` after the builder returns an error.

### 2.3 Supply all four production callbacks

After construction of process-shared dependencies (`binding`, manager, replica factory, snapshot coordinator, persistent agent), compose:

```go
supervisor := NewHostSupervisor(
    binding.Current(),
    func(ctx context.Context, old any) error {
        host, ok := old.(*core.EngineImpl)
        if !ok { return fmt.Errorf("master host has unexpected type %T", old) }
        return host.StopContext(ctx)
    },
    func(ctx context.Context) (any, error) {
        graph, err := buildMasterGraph(ctx)
        if err != nil { return nil, err }
        return graph.Engine, nil
    },
    func(ctx context.Context, candidate any) error {
        host, ok := candidate.(*core.EngineImpl)
        if !ok { return fmt.Errorf("fresh master has unexpected type %T", candidate) }
        return host.StartContext(ctx)
    },
    func(candidate any) {
        binding.Rebind(candidate.(*core.EngineImpl))
        rebindMasterDependents(candidate.(*core.EngineImpl))
    },
)
```

The existing supervisor ordering is binding: **validate builder → capture old → retire old → build → start → publish/rebind** (`cmd/zenbot/host_supervisor.go:38-64`). Do not publish the candidate before `StartContext` succeeds; otherwise asynchronous send/direct paths can target an unstarted engine. Do not attempt rollback by restarting the retired instance after any successful retirement: it has consumed its join state and is not the required fresh host.

For initial startup, call the same build/start/rebind path (or an equivalent `supervisor.StartInitial`) rather than separately constructing `e` in `main`. This proves production has one fresh-graph path and makes the initial and restart graph identical. `hostSupervisor` needs a small explicit initial-start method if its current `Restart` semantics must retain a non-nil old master; do not simulate startup by retiring a bootstrap engine.

## 3. Required rebind set: the current room agent cannot be left on the old master

### [OBSERVED] Captures that make current composition unsafe

| Capturing dependency | Current capture | Required replacement behavior |
|---|---|---|
| Snapshot reply sink | `main.go:233-238` closes over local `master` | Resolve `masterBinding.Current()` at send time (§2.1). |
| Room-user directory | `core.EngineRoomUserDirectory{Host: e, Replicas: manager}` at `main.go:246` | Make host lookup rebindable/indirect; keep the same manager. `FindRoomUsers` currently directly reads `Host` (`internal/core/room_directory.go:17-42`). |
| Participation snapshot | `newLiveAgent` closes over its `engine` for channel, active users, bot identity, and command prefix (`main.go:176-197`) | Resolve current master for each participation/direct snapshot. |
| Agent output/failure sinks | `newLiveAgent` closes over `engine.SendChatMessage` (`main.go:152-166`) | Resolve current master at delivery time. |
| Agent tool command gateway | `command.NewAgentCommandGateway(engine)` is created once at `main.go:148`; gateway stores `common.Engine` (`internal/command/agent_gateway.go:18-22,43-86`) | Use a resolver gateway that gets the current master once per `Execute`, then delegates to a new normal gateway for that request. It must retain the existing fixed public-agent allowlist. |
| Per-master command listener/controller/relay | installed directly on old `e` (`main.go:251-256`) | Reinstall on each fresh engine in `buildMasterGraph`. |

### 3.1 Minimal rebindable adapters

Do **not** recreate or close `roomAgent` on restart. Its runtime is intentionally process-owned and terminal on `Close` (`cmd/zenbot/main.go:102-106`; `internal/agent/live/service.go:8-21`). Instead, add only the following indirect adapters:

1. **`core.EngineRoomUserDirectory`**: change it to hold a synchronized master resolver (or add synchronized `RebindHost(*EngineImpl)`) rather than a public immutable `Host *EngineImpl`. The persistent directory object remains passed into the agent tool loop; its replica lookup remains the same shared manager lookup.
2. **`currentMasterGateway` in `cmd/zenbot` (or `internal/agent/commandgateway`)**: implement `commandgateway.Gateway` by loading the current master at method entry and delegating. It must not cache an engine or broaden `publicAgentCommandAliases`.
3. **`newLiveAgent` input seam**: replace its single captured `common.Engine` dependency with a narrow resolver/callback used for (a) output and failure sends, (b) participation/direct trusted snapshots, (c) the resolver gateway, and (d) optional message automation lookup. The agent service/runtime, queue, memory, LLM client, and direct submitter object stay alive.
4. **Rebindable participation**: the object retained by `roomAgent.Participation` must resolve the current master on each `Handle`/snapshot. It must not retain the first master through the `snapshot` closure. If message automation remains enabled, construct/replace the master-specific monitor during `rebindMasterDependents`, but keep the one persistent pipeline/runtime.

`rebindMasterDependents(next)` is therefore a real operation, not a no-op callback. Its minimum ordered actions are:

```text
1. binding.Rebind(next)                     // future reply/output/snapshot resolution
2. directory.RebindHost(next)               // room_users uses new host
3. roomAgent.RebindMaster(next/binding)     // participation snapshot + monitor resolver
4. (new master already has its listener, controller, replica controller, relay, utilities)
```

Any `rebind` API should accept only a non-nil `*core.EngineImpl`, not `any`; retain `any` only at the existing supervisor boundary if changing it is out of scope. This makes the test seam concrete and prevents a silently invalid production callback.

## 4. Exclusive master ownership: mandatory correction

### [OBSERVED] Current conflict

`main` also creates `core.NewLifecycle(func() core.LifecycleEngine { return lifecycleEngine{e} }, ...)` after installing the supervisor (`cmd/zenbot/main.go:271-283`). That lifecycle stores and monitors the original engine and recreates whatever its factory returns on health failure (`internal/core/lifecycle.go:51-120`). Its current factory always returns the same old `e`; it cannot coexist as a second owner with a supervisor that stops/replaces `e`.

If retained unchanged, a command restart can stop the old host while `Lifecycle.monitor` observes it unhealthy and starts/reuses that retired instance. That races the fresh graph and violates both fresh join and single-owner guarantees.

### [BINDING] Choose one owner in code, not two

The `hostSupervisor`/fresh graph builder must become the exclusive production owner of master start, stop, replacement, and current-master publication. Remove the current `core.Lifecycle` instance from main's master path, including its factory that closes over `e`.

If health-driven reconnect is required to remain enabled, it must submit the same serialized supervisor restart request (or be refactored so its factory calls the same fresh graph builder and its start completion performs the same publication/rebind). It may not directly hold or restart an engine. This is an implementation integration requirement, not permission to invent a new retry, timeout, or health policy; preserve the existing configured/hard-coded behavior only after routing ownership through the one supervisor.

The signal path is separate process teardown. It must retain its current terminal order—close room agent, stop current master with bounded context, `manager.StopAll`, then deferred DB/H2 cleanup—but obtain the master from the supervisor/binding and be idempotent after command shutdown. Command shutdown itself remains host-only/non-exiting.

## 5. Failure semantics and edge cases

| Condition | Required behavior |
|---|---|
| Fresh builder absent | Fail before old retirement; already covered by `TestHostSupervisorRestartWithoutFreshBuilderFailsBeforeRetiringMaster`. |
| Retire fails | Do not build or publish a candidate. Old remains supervisor current reference; log through existing lifecycle command path. |
| Build fails after successful retire | Leave no master published; do not resurrect old host; command stays success/no-reply and logs internal error. |
| Start fails | Do not publish/rebind candidate. Best-effort stop the unstarted/partially started candidate inside the start/cleanup callback only if it owns a transport; do not touch replicas/agent/H2/DB. |
| Rebind panics/fails | Do not allow a partial reference swap. `rebind` must be non-failing by construction: validate all persistent adapters before startup and make their updates synchronized. If an error-returning rebind is retained, supervisor must not publish `master` until it succeeds. |
| Shutdown request | Retire only current master; set no new master; do not call process teardown. Existing terminal controller state rejects/coalesces later restart requests (`internal/core/host_lifecycle.go:71-85`). |
| OS signal after command shutdown | Run one idempotent full process teardown; `SignalTeardown` is the existing once-only seam (`cmd/zenbot/host_supervisor.go:79-85`). |

## 6. Irreconcilable ownership conflict in the current source

There is no source-only callback fill that can honestly meet all requirements while retaining current captures:

* Filling `build/start/rebind` in `main` alone leaves the persistent agent's output, direct submission snapshot, participation snapshot, command gateway, and directory targeting the old master.
* Recreating `roomAgent` to fix those captures violates the explicit restart-only-host requirement and is unsafe because its runtime `Close` is terminal.
* Retaining the current `core.Lifecycle` alongside the supervisor creates two concurrent owners of the original master.

Thus the current combination of **“do not recreate room agent”** and **“all room-agent/direct dependencies must target the new master”** is irreconcilable *without* the narrow resolver/rebinding seam in §3; it is not a product-policy ambiguity. The minimal solution is persistent agent/runtime plus rebindable master-facing adapters, and exclusive supervisor ownership. No policy decision is needed for replicas, H2/DB, acknowledgements, SQL scope, or process exit: the approved contract already fixes them.

## 7. Strict sequential RED → GREEN tracers

Each tracer is one vertical slice. Do not write the next test or production edit before recording the prior focused failure for the intended missing behavior. All callbacks are fakes; no real endpoint, H2 teardown, signal, process exit, or room-agent close is allowed.

1. **Master binding / snapshot reply — RED.** In `cmd/zenbot`, build a binding with old then new fake sender. Assert a snapshot reply after rebind uses only new sender. Current captured closure cannot provide this through a named binding seam.
   **GREEN:** add `masterBinding` and resolver reply sink. Run `go test ./cmd/zenbot -run 'Test.*MasterBinding|Test.*Snapshot.*Rebind' -count=1`.
2. **Persistent dependent rebind — RED.** Using fake masters, persistent directory, resolver gateway, and a live-agent seam fake, prove an already-created direct snapshot/output/gateway operation uses master B after rebind, while the same room-agent service object was never closed/recreated.
   **GREEN:** add only resolver adapters and `RebindMaster`; run focused main/live/core directory tests with `-race`.
3. **Fresh graph builder — RED.** Fake `factory`-level graph construction behind an injected builder. Assert initial build configures a new master with listener, controller, shared replica manager/relay, and utilities; a second build returns a distinct master with fresh transport/join state, without replica/agent/H2 callbacks.
   **GREEN:** extract `buildMasterGraph`; run `go test ./cmd/zenbot -run TestProductionMasterGraph -count=1`.
4. **Production callback composition — RED.** Replace `TestMainProductionHostLifecycleWiresSupervisorCallbacks` with a production composition test that proves its callbacks are non-nil and executes exactly `retire-old, build-new, start-new, publish/rebind`. Also assert old is not current during/after success and B is current for snapshot/direct paths.
   **GREEN:** wire all callbacks in `main`; run `go test ./cmd/zenbot -run 'Test(HostSupervisor|MainProductionHostLifecycle|ProductionMasterGraph)' -count=1`.
5. **Single owner — RED.** A fake health/restart trigger plus command restart proves no old-engine lifecycle factory starts after supervisor retirement.
   **GREEN:** remove/refactor main's `core.Lifecycle` ownership to submit the same supervisor operation. Run focused tests with `-race`.
6. **Shutdown/signal regression — RED.** Command shutdown stops only current master and preserves agent/replica/H2/DB counters; later signal teardown closes/stops each terminal dependency once.
   **GREEN:** route signal teardown through current supervisor/binding and retained once guard. Run `go test ./cmd/zenbot -run 'Test.*(Shutdown|Signal).*' -count=1`.
7. **End-to-end lifecycle parity regression — RED.** Real in-process listener/host-lifecycle fakes issue `restart`, wait dispatch release, and assert a new join-capable master, no chat reply, no replica/agent/H2/DB shutdown, and all resolver paths on the new master. Then issue `shutdown` and assert non-exiting host-only behavior.
   **GREEN:** only integration glue needed by the proven lower tracers.

After tracer 7:

```text
gofmt -w <only implementation-touched files>
go test -count=1 ./internal/service ./internal/core ./internal/command ./internal/listener ./internal/listener/message ./cmd/zenbot
go test -race -count=1 ./internal/core ./internal/command ./internal/listener ./cmd/zenbot
go test -count=1 ./...
go vet ./...
go build ./...
git diff --check
git status --short
```

## 8. File map

| Path | Required responsibility |
|---|---|
| `cmd/zenbot/main.go` | Extract fresh master graph builder, own binding/resolver composition, supply real supervisor callbacks, eliminate competing fixed-engine lifecycle ownership, retain signal teardown ownership. |
| `cmd/zenbot/host_supervisor.go` | Preserve fail-before-retire; add typed/publish-after-start/rebind guarantees and an explicit initial-start path if required. |
| `cmd/zenbot/*_test.go` | Strict tracer coverage for callbacks, captured dependency rebind, no terminal restart side effects, ownership, shutdown/signal. |
| `internal/core/room_directory.go` | Make host lookup rebindable and synchronization-safe; retain shared replica lookup. |
| `internal/agent/live/*` and only needed `cmd/zenbot` adapter code | Introduce narrow current-master resolver for output, trusted snapshots, and participation; retain one runtime/service. |
| `internal/command/agent_gateway.go` or a narrow adjacent adapter | Resolver-backed gateway that preserves public alias restrictions. |
| `internal/core/lifecycle.go` only if health handling is retained | Route through the sole fresh-master supervisor operation; do not let it own a fixed engine. |

No changes belong in SQL command/service/renderer, agent SQL policy, replica manager semantics, H2 schema/server configuration, lifecycle command aliases, or command authorization.

## 9. Validation performed for this architecture

* Read QA record and binding architecture; inspected current supervisor, main composition, factory, engine, lifecycle, replica manager/controller, room directory, snapshot coordinator, live agent/direct submission/participation, command gateway, and listener dispatch cited above.
* Focused baseline passed in the current dirty tree:

```text
git diff --check
# exit 0

go test ./cmd/zenbot ./internal/core ./internal/command ./internal/listener -run 'Test(HostSupervisor|MainProductionHostLifecycle|HostLifecycle|Restart|Shutdown|UserChatListenerLifecycle|SQLRestartShutdownFinalCatalogIntegration)' -count=1
ok  zenbot/cmd/zenbot
ok  zenbot/internal/core
ok  zenbot/internal/command
ok  zenbot/internal/listener
```

* This handoff creates no app/test edit, runs no restart/shutdown/signal, stages nothing, and makes no commit.
