# Shared Integration Architecture Handoff

## Scope and verdict

This is an architecture-only handoff for the next coordinated slice. It integrates the accepted transport/lifecycle/replica foundations with Zenbot's existing engine, factory, command registry, main, and snapshot coordinator. It does **not** implement application code. Saturn is read-only evidence from `/Users/ab/workspace/projects/saturn` on branch `develop` at commit `10a1ea3` (`refactor: simplify listener workflows`).

The intended result is one owner-managed host (`MASTER`) plus zero or more independently owned `REPLICA` engines, all using the bounded `internal/transport.Connection`; command registration remains deterministic and DBZ/identity-compatible; temporary snapshot sessions are routed through the coordinator rather than through the host replica registry.

## Observed source map

### Zenbot current paths and call paths

- `internal/common/engine.go`: legacy `common.Engine` is the broad compatibility contract. It has `Start()`, `Stop()`, `DispatchMessage(string)`, `SendRawMessage(string)`, chat/whisper/addressed send methods, active-user/subscription APIs, command registration, listener setup, and audit methods. It cannot express context-aware lifecycle errors.
- `internal/core/engine_impl.go:15-48`: `EngineImpl` owns identity (`Type`, `Channel`, `Name`, `Password`), `HcConnection *core.Connection`, outbound `chan string`, active users, listeners, repository/services, and enabled commands.
- `internal/core/engine_impl.go:50-87`: legacy `Start` starts `HcConnection.Connect`, busy-waits for `IsWsConnected`, writes the join payload, starts `startSharingMessages`, and waits on `EngineWg`; `Stop` cancels ping, closes the connection, closes the outbound queue, and waits. This is the unsafe compatibility path and must not remain the integration owner.
- `internal/core/engine_impl.go:89-121`: `DispatchMessage` decodes `cmd` and routes `onlineSet`, `onlineAdd`, `onlineRemove`, `chat`, and `info` to the configured listeners; `join` and `session` are ignored.
- `internal/core/engine_impl.go:123-165`: raw messages are queued verbatim; chat methods construct the current JSON format `{ "cmd": "chat", "text": ...}` and preserve current author/whisper/newline behavior; `startSharingMessages` writes queue entries to the legacy connection.
- `internal/core/engine_impl.go:167-191`: active users are replaced/copied or identity-deduplicated using `model.IdentityKey(trip, hash, name)`. Do not change this DBZ/identity-sensitive behavior.
- `internal/core/engine_impl.go:341-359`: `RegisterCommand` expands aliases into `EnabledCommands`; `GetEnabledCommands` is the lookup consumed by `common.BuildCommand`/dispatch.
- `internal/factory/engine_factory.go:17-80`: `NewEngine(etype, *config.Config, repository.Repository) common.Engine` parses `Config.WebsocketUrl`, constructs and wires `EngineImpl`, legacy connection/listeners, repository, authorization/security, service bundle (including optional DBZ), then applies ZOMBIE substitutions. This is the primary factory extension point.
- `internal/command/registry.go:86-106`: `RegisterAll` is an explicit deterministic Saturn catalog. It contains `replica`, `replicaoff`, `replicastatus`, `msgchannel/msgroom`, `whiskey` and aliases, but `saturnCommand.Execute` currently treats most as accepted placeholders (`:53-60`).
- `internal/command/dispatch_adapter.go:37-66`: `RegisterUserUtilities` registers concrete legacy commands, then selected Saturn definitions, conditionally adds identity/admin and DBZ definitions, and wraps them in `legacyAdapter`. This is the live command registration seam.
- `internal/command/handlers.go:19-53`: replies call `SendChatMessage`; raw operations marshal and call `SendRawMessage`. Preserve this separation.
- `internal/listener/message/handlers.go:132-155`: `DispatchUserCommand` checks prefix, builds command, authorizes via `IsUserAuthorized`, emits an authorization failure with the incoming whisper flag, then executes. Replica/admin commands must enter this path, not bypass it.
- `internal/service/services.go:20-43`: `CommandOutput` (`Chat`, `Raw`) and `Bundle` are the service output seam and service ownership boundary. Existing service construction and optional DBZ wiring in the factory must remain intact.
- `cmd/zenbot/main.go:18-48`: current startup opens H2, calls `factory.NewEngine(model.MASTER, c, db)`, registers user utilities, launches `go e.Start()`, then calls `e.Stop()` on interrupt and waits. This is the process lifecycle extension point.
- `internal/listener/online_set_listener.go:10-48`: host online-set listener replaces active users and invokes an optional callback. It is the correct point to trigger host-only autorun/startup behavior, not a replica manager callback.
- `internal/listener/user_chat_listener.go:12-30` and `internal/listener/info_chat_listener.go:12-42`: JSON decode then chain processing; chat derives whisper from `IsWhisper`, `Whisper`, or `Type == "whisper"`.
- `internal/listener/user_joined_listener.go:14-50`: join updates active users, shares subscribed identity data, audits presence, and performs case-insensitive subscription matching. Preserve for MASTER and normal permanent REPLICA profiles.
- `internal/listener/snapshot/coordinator.go:10-307`: `RoomSnapshotCoordinator` owns workflow state, creates a `Session`, accepts `OnSnapshot(sessionID,payload)`, `OnTransportError`, and `OnClosed`, parses one snapshot, applies an operation, flushes raw output, and closes the session. `Session` requires `ID`, `Start`, `Close`, `Flush`, `SendRaw`; `SessionFactory.Create(RoomSnapshotRequest, SnapshotSink)` is the injection point.
- `internal/listener/snapshot/session_factory.go:11-55`: `TemporarySessionRegistry` currently only stores cancellation functions (`Open`, `Close`, `Len`) and is deliberately isolated from host replicas. It is not a session factory and is not connected to coordinator callbacks.
- `internal/transport/connection.go:18-188`: bounded WebSocket transport has `Dialer`, `Config`, `NewConnection`, `Start(context.Context) error`, `Messages()`, `Errors()`, `Connected()`, `SendText`, `SendRaw`, and `Close`. It owns read/ping goroutines and serialized writes. Channels remain open after close by design.
- `internal/core/lifecycle.go:10-167`: `LifecycleEngine` is `Start(context.Context) error`, `Stop(context.Context) error`, `Healthy() bool`; `Lifecycle` creates/restarts engines, monitors health, and bounds retry/stop. Background failures are currently discarded by `go func() { _ = l.run(...) }`; integration should make those observable.
- `internal/core/replica_manager.go:11-91`: `ReplicaManager` owns `map[string]Replica`, rejects blank/host/duplicate channels, supports `Add`, `Remove`, `StopAll`, copied `Replicas`, sorted `Channels`, and a terminal stop barrier.
- `internal/repository/repository.go:21-41`: `Repository`, `AuditRepository`, and `AuthorizationRepository` are separate persistence contracts. A replica must retain the same repository/service/identity semantics unless explicitly configured otherwise.

### Saturn develop evidence

- `src/main/java/org/saturn/app/facade/impl/EngineImpl.java:59-115`: Saturn owns `replicasMappedByChannel`, a `CommandFactory`, a `ListenerProfile`, and a payload-listener map. `registerDefaultPayloadListeners` always registers `onlineSet`, and for `PERMANENT` also registers online add/remove/chat/info. `dispatchMessage` looks up `cmd` in the map and logs unknown commands.
- `EngineImpl.java:168-199`: `start` creates a `Connection` and starts non-blocking; `sendJoinMessage` builds `{cmd:join,channel,nick,password}`. `EngineImpl.java:201-245` separates chat queue serialization from raw queue serialization.
- `EngineImpl.java:247-317`: `stop` stops replicas then closes the host connection; `addReplica` maps by channel. This establishes ownership ordering and host/replica distinction.
- `src/main/java/org/saturn/app/facade/impl/Connection.java:25-95,132-161`: WebSocket callbacks distinguish `EngineType.REPLICA` from host for thread context, notify connection/incoming listeners, and expose start, nonblocking start, write, close, and connection health.
- `src/main/java/org/saturn/ApplicationRunner.java:46-165,180-197`: host startup/restart is serialized by `lifecycleLock`; health checks restart the host; shutdown stops scheduler, host, services, and DB.
- `src/main/java/org/saturn/app/command/factory/CommandFactory.java:40-69,85-183`: command definitions are discovered, sorted, cached, duplicate/anagram validated, and instantiated against the concrete engine.
- `src/main/java/org/saturn/app/facade/ListenerProfile.java:1-6`: `PERMANENT` versus `TEMPORARY_ONLINE_SET` controls only online-set delivery versus all normal room listeners.
- `src/main/java/org/saturn/app/listener/snapshot/EngineSnapshotSession.java:10-78`: temporary snapshot sessions construct `EngineType.LIST_CMD` with `TEMPORARY_ONLINE_SET`, register a private online-set sink, and expose only start/close/flush/sendRaw.
- `src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java:36-105`: same-room delivery is direct chat; remote delivery creates a UUID workflow, a temporary snapshot session, parses online-set, and submits a `DeliverMessageToRoomOperation`.
- Saturn tests `DefaultRoomSnapshotCoordinatorTest`, `RoomSnapshotOperationsTest`, `GsonOnlineSetPayloadParserTest`, `ReplicaStatusCommandImplTest`, `ReplicaOffCommandImplTest`, `WhiskeySayUserCommandImplTest`, and `WhiskeyAnonUserCommandImplTest` are behavioral references, not files to modify.

## Target interfaces and extension points

The following are proposed concrete seams; application implementation should be a separate phase.

1. **Context-aware engine adapter (preserve `common.Engine`)**

```go
type ManagedEngine interface {
    common.Engine
    Start(context.Context) error
    Stop(context.Context) error
    Healthy() bool
    EngineType() model.EngineType
    ReplicaChannels() []string
}
```

`EngineImpl` should implement the context-aware methods through `transport.Connection`, while legacy `Start()`/`Stop()` remain only as compatibility shims that create bounded contexts. Avoid adding a second conflicting `Start` method to `common.Engine`; use a distinct managed interface and type assertions in lifecycle/factory/main.

2. **Engine runtime dependency injection**

```go
type EngineOptions struct {
    Transport transport.Config
    ListenerProfile ListenerProfile
    ReplicaManager *ReplicaManager
    SnapshotCoordinator *snapshot.RoomSnapshotCoordinator
    SessionRegistry *snapshot.TemporarySessionRegistry
    LifecycleErrors chan<- error
}
func NewEngineWithOptions(model.EngineType, *config.Config, repository.Repository, EngineOptions) (*core.EngineImpl, error)
```

Retain `NewEngine(...) common.Engine` as a compatibility wrapper. `NewEngineWithOptions` must wire transport, listeners, command state, services, and profile without changing DBZ/ZOMBIE rules.

3. **Transport-to-engine event ownership**

```go
type EngineTransport interface {
    Start(context.Context) error
    Messages() <-chan []byte
    Errors() <-chan error
    Connected() bool
    SendText(context.Context, string) error
    SendRaw(context.Context, []byte) error
    Close(context.Context) error
}
```

The engine runtime loop should select on `Messages`, `Errors`, and context cancellation, call `DispatchMessage(string(payload))`, and surface errors to lifecycle. Join must be sent once after connection becomes usable. `SendRawMessage` and chat methods should enqueue or directly call a bounded output writer but preserve exact existing payload bytes.

4. **Replica construction/controller**

```go
type ReplicaFactory interface {
    NewReplica(context.Context, string) (ManagedEngine, error)
}
type ReplicaController interface {
    AddReplica(context.Context, string) error
    RemoveReplica(context.Context, string) error
    ReplicaChannels() []string
}
```

The concrete controller should validate a trimmed channel, reject the host channel, construct `REPLICA` with the same config/repository policy, start it before `ReplicaManager.Add` becomes visible (or roll back on Add failure), and stop/remove deterministically. Do not let `ReplicaManager` construct engines or access DB.

5. **Snapshot factory connected to coordinator**

```go
type CoordinatedSessionFactory struct {
    Registry *TemporarySessionRegistry
    New func(context.Context, RoomSnapshotRequest, SnapshotSink) (Session, error)
}
func (f *CoordinatedSessionFactory) Create(RoomSnapshotRequest, SnapshotSink) (Session, error)
```

`Create` opens a registry context, constructs a temporary `LIST_CMD`/`TEMPORARY_ONLINE_SET`-equivalent engine using the shared transport, binds `onlineSet` to the provided sink, and returns a session whose transport errors/close are forwarded to the coordinator exactly once. `Close` must remove the registry entry. This must never enter the host `ReplicaManager`.

6. **Observable lifecycle failures**

Add an optional error sink or `Errors() <-chan error` to `Lifecycle`; retain asynchronous `Start` but make terminal factory/start/health errors observable. Main must own and drain/cancel it, otherwise retries can silently die.

7. **Listener profile**

Use an explicit Go enum (recommended `Permanent`, `TemporaryOnlineSet`) in engine options. `Permanent`: onlineSet, onlineAdd, onlineRemove, chat, info. `TemporaryOnlineSet`: onlineSet only, with no chat command dispatch, subscriptions, mail, DBZ, or permanent host side effects. `MASTER` and ordinary `REPLICA` use permanent semantics; snapshot sessions use temporary semantics.

## Serialized file-level change map

Apply in this order; each step must compile before the next:

1. `internal/core/engine_impl.go` — add transport-backed managed start/stop/health, typed engine identity/profile fields, safe event loop and bounded output writer; retain legacy methods and exact chat/raw serialization as compatibility wrappers.
2. `internal/common/engine.go` — only if required, add the smallest optional capability interfaces in a new section; do not break existing implementers or alter DBZ/identity method semantics.
3. `internal/factory/engine_factory.go` — add `EngineOptions`/`NewEngineWithOptions`, inject transport and profile, preserve repository/service/DBZ/ZOMBIE construction; add a `ReplicaFactory` closure/controller.
4. `internal/listener/online_set_listener.go`, `user_chat_listener.go`, `info_chat_listener.go`, and existing join/left listeners — make profile registration explicit; route transport events through existing listeners without duplicate processing.
5. `internal/listener/snapshot/session_factory.go` — evolve the temporary registry into a coordinator-bound session factory; ensure cancellation, transport error, close, and cleanup are idempotent.
6. `internal/listener/snapshot/coordinator.go` — accept session ID event routing from the factory; avoid double terminal transitions and ensure operation flush/close errors become outcome failures where appropriate.
7. `internal/core/replica_manager.go` — add a controller-facing lifecycle-aware API only if needed; preserve host rejection, duplicate rejection, stop barrier, copied map, and sorted status.
8. `internal/command/registry.go`, `dispatch_adapter.go`, `handlers.go`, and new/updated command handlers — replace only the `replica`, `replicaoff`, `replicastatus`, `msgchannel/msgroom`, and `whiskey` placeholders with concrete controller/coordinator calls; preserve prefix and authorization ordering and DBZ conditional registration.
9. `cmd/zenbot/main.go` — construct one managed MASTER, register commands once, create lifecycle/replica ownership, start with context, expose lifecycle errors, and shutdown in order: cancel lifecycle -> stop replicas -> stop host transport -> close repository/DB.
10. `internal/command/*_test.go`, `internal/core/*_test.go`, `internal/factory/*_test.go`, `internal/listener/snapshot/*_test.go`, and a focused integration test file — add tests described below. No Saturn or unrelated migration files are in scope.

## DBZ and identity preservation constraints

- Keep `repository.DBZRepository` detection and `service.Bundle.DBZ` conditional exactly as in `internal/factory/engine_factory.go:48-63`; DBZ aliases remain `model.REGULAR` and are absent when DBZ is unavailable.
- Do not change DBZ SQL, duplicate registration/non-atomic insert behavior, strength-only free-stat decrement, enemy process-local state, or command authorization. The accepted `.hermes/handoffs/dbz-qa.md` records these as compatibility requirements.
- Keep identity APIs and `repository.IdentityRepository` attached to the same repository/service bundle for MASTER and persistent REPLICA engines. Do not use pointer identity as a replacement for `model.IdentityKey(trip, hash, name)`; active-user replacement/deduplication must remain stable.
- Replica channel is routing identity; user `(trip,hash,name)` remains user identity. Never use channel as a DBZ character or trip identity.
- Temporary snapshot engines must not mutate the host active-user map, subscription set, DBZ state, identity tables, command registry, or host repository lifecycle. Their only accepted inbound payload is `onlineSet`; their only outbound operation is the coordinator-approved raw/chat payload.
- Preserve exact raw payloads for moderation (`kick`, `ban`, `unban`, `unbanall`, `lockroom`, `unlockroom`) and exact chat formatting/newline/whisper behavior in `EngineImpl` and tests.

## Startup, shutdown, and command registration sequences

### Startup (proposed)

```text
main
  -> load Config + open H2 repository
  -> create shared MASTER transport/options
  -> factory.NewEngineWithOptions(MASTER, ... Permanent)
  -> create ReplicaManager(host channel) and coordinator/session factory
  -> attach controller/coordinator capabilities to engine
  -> command.RegisterUserUtilities(managed engine)
       -> concrete legacy commands
       -> Saturn definitions (replica/msgroom/whiskey real handlers)
       -> optional identity/admin and DBZ definitions
  -> lifecycle.Start(ctx)
       -> factory creates/starts MASTER
       -> transport.Start dials
       -> engine event loop consumes Messages/Errors
       -> on first connected state, send exact join JSON once
       -> onlineSet replaces host users and may run host autorun
```

`RegisterUserUtilities` must run before accepting chat payloads. Replica commands are authorized by the existing `DispatchUserCommand` chain before controller execution.

### Shutdown (proposed)

```text
SIGINT/context cancel
  -> lifecycle.Stop(ctx)
       -> cancel engine loops
       -> stop/remove replicas through ReplicaManager.StopAll
       -> stop MASTER transport with bounded timeout
       -> wait for event/output goroutines
  -> close repository/DB exactly once
  -> return first error and log/drain lifecycle error channel
```

No goroutine may close a shared output channel while another producer can send. Prefer context cancellation and owner-managed channel lifetime over the current `close(OutMessageQueue)` behavior.

### Replica command sequence

`chat payload -> CoreListener -> EngineImpl.DispatchMessage -> UserChatListener -> message.Chain -> ResolveUserMetadata/Audit/... -> DispatchUserCommand -> authorization -> concrete replica handler -> ReplicaController -> ReplicaFactory(REPLICA) -> transport dial/join -> ReplicaManager.Add`. `replicaoff` removes from manager then stops; `replicastatus` reads sorted `Channels`; `whiskey` probes configured proxies in order and creates a bounded replica using the selected proxy/backup order.

## Raw, chat, listener, and profile semantics

- **Raw:** `SendRawMessage` must send already-serialized protocol JSON unchanged. Moderation and snapshot operations use raw output; never pass raw JSON through chat wrapping.
- **Chat:** `SendChatMessage(author,message,whisper)` keeps current semantics: public addressed text is `@author payload`; whisper is `/whisper @author payload`; payload newline normalization follows `SendAddressedMessage`/`SendWhisperMessage`; output JSON remains `{ "cmd": "chat", "text": "..."}` with current escaping.
- **Inbound chat:** `model.ChatMessage` must continue recognizing `Whisper`, `IsWhisper`, and `Type == "whisper"`; authorization failure replies use the inbound whisper bit.
- **Inbound info:** `InfoChatListener` remains permanent-only. Temporary snapshot sessions must not run info/chat/user-join chains.
- **Online set:** permanent engines parse `users`, replace active users, and run host-only startup callback; temporary sessions parse the snapshot through `snapshot.Parse`/coordinator parser and deliver to the workflow sink, not the host store.
- **MASTER:** owns process lifecycle, repository/DB, host channel, command registry, autorun, subscriptions, and replica manager.
- **REPLICA:** owns one non-host channel and its own transport/listeners but shares the configured repository/identity policy; it must not register global commands, start host autorun, or recursively create replicas.
- **Temporary profile:** one workflow/session ID, isolated context, onlineSet-only listener, no command registration or persistence side effects, mandatory cleanup on success/failure/timeout/cancel.

## Focused and integration test plan

### Focused tests (before live integration)

- `internal/core`: managed MASTER and REPLICA start/stop, one join per connection, healthy state, context cancellation, output writer shutdown, lifecycle terminal error observability, and no use of legacy `core.Connection`.
- `internal/factory`: options wire all listeners/services correctly; MASTER and REPLICA get permanent profiles; temporary factory gets onlineSet-only profile; DBZ present/absent and ZOMBIE paths preserve existing behavior.
- `internal/command`: registry contains concrete `replica`, `replicaoff`, `replicastatus`, `msgchannel/msgroom`, `whiskey`; aliases remain collision-free; authorization occurs before side effects; replica manager receives trimmed channels; current-room msgroom avoids temporary session.
- `internal/listener/snapshot`: coordinator-bound temporary factory routes a real session ID, forwards one transport error/close, cleans registry on success/failure/timeout/cancel, and never changes host store/manager.
- Raw/chat golden tests: moderation JSON bytes; public addressed chat; whisper chat; CRLF and literal `\\n` normalization; snapshot operation raw send. Compare exact strings, not decoded maps.

### Minimum real MASTER/REPLICA WebSocket integration tests

Add a Go test using `httptest.NewServer` plus `websocket.Upgrader` (pattern already proven in `internal/transport/connection_test.go:13-66`) and injected `transport.Config.Dialer`/URL:

1. **MASTER factory test:** construct a real `MASTER` with `factory.NewEngineWithOptions`; server records handshake, receives exactly one join payload, sends a representative `onlineSet` then `chat`; assert active users/listener dispatch and exact outbound chat/raw payload. Stop with timeout and assert server observes close/no goroutine leak.
2. **REPLICA factory test:** construct a real `REPLICA` through the same factory/controller path for a distinct channel; server records replica join channel/nick/password and sends `onlineSet`; assert replica active users, permanent listeners, channel identity, and manager registration. Assert it cannot register the host channel.
3. **MASTER + REPLICA ownership test:** start both against independent local WebSocket endpoints, issue the concrete replica command/controller, assert two joins and distinct channels, then stop manager and host; assert both sockets close and lifecycle errors are empty.
4. **Temporary snapshot minimum:** create a real temporary session through the coordinator-bound factory, server sends `onlineSet`, assert operation receives it and sends only its expected raw/chat payload; assert session registry length returns to zero. This is the smallest test that closes the currently documented coordinator gap.

Use short explicit deadlines and server-side channels; avoid external hack.chat. Run with `go test -count=1` and `go test -race` for the focused packages.

## Risks and assumptions

- Go cannot overload `Start()`/`Stop()`; the managed interface must coexist with legacy `common.Engine` through embedding/type assertions or a deliberate compatibility rename.
- Existing `EngineImpl.Start` blocks on a wait group and legacy connection may fatal on dial failure. Integration must route all new startup through transport/lifecycle and leave the old path unused by the new main.
- `transport.Connection` channels are intentionally not closed; event-loop termination must use context/connection state, not range over channels.
- `Lifecycle` currently loses background errors and can return nil from `Start` before factory/start failure; exposing terminal errors is required for honest main behavior.
- Replica repository sharing is assumed compatible with current DBZ/identity behavior; if concurrent H2 access is not safe, that must be proven in focused tests rather than silently giving replicas DummyImpl.
- Proxy configuration/Whiskey construction is not present in the current Go config (`internal/config/config.go`); add only the minimum explicit fields or an injected probe/factory, not speculative global proxy behavior.
- Existing `saturnCommand.Execute` placeholders must not remain reachable for the targeted commands after wiring; other placeholders remain outside this slice.
- `TemporarySessionRegistry.newID` has a random-read fallback that is not unique (`snapshot-16`); coordinator integration should fail closed or replace IDs with a collision-resistant monotonic/UUID strategy.
- `RoomSnapshotCoordinator.find` scans active workflows by session ID; acceptable for this slice but a session-index map may be needed if concurrency/volume grows.
- No Saturn files may be modified. The Saturn working tree is already dirty; treat it as read-only evidence.

## Complexity rating

**High (8/10).** The individual foundations are small and accepted, but integration crosses transport ownership, incompatible legacy/context lifecycle contracts, factory dependency construction, command authorization/alias compatibility, two listener profiles, temporary workflow cleanup, and real concurrent WebSocket shutdown. The largest risks are duplicate event dispatch, silent lifecycle death, channel-close races, and accidentally changing DBZ/identity semantics. The recommended slice is implementable in one coordinated change series with the serialized file map and focused tests as gates, but should not be collapsed into a single untested patch.

## Evidence and completion checklist

- [x] Current Zenbot paths/symbols and call paths cited.
- [x] Saturn `develop` paths/symbols cited without modification.
- [x] Concrete target interfaces/signatures specified.
- [x] Serialized file-level map specified.
- [x] DBZ and identity constraints specified.
- [x] Startup/shutdown and command registration sequences specified.
- [x] Raw/chat/listener/profile semantics specified.
- [x] Focused tests and minimum real MASTER/REPLICA WebSocket tests specified.
- [x] Risks, assumptions, and complexity rating specified.
- [x] Application code not implemented by this phase.
