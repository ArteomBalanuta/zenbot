# Saturn → Zenbot Transport / Replica Lifecycle Architecture

**Scope:** architecture-only handoff for transport, application lifecycle, engine types, listener ordering, replica/remote-room/whiskey boundaries, and parity tests. Saturn is read-only (`develop`); no application code was changed. The target has intentional dirty/untracked migration work and an active DBZ worker; this document deliberately avoids implementing the slice.

## 1. Evidence and status

### Observed Saturn sources

- `src/main/java/org/saturn/ApplicationLifecycle.java`: singleton lifecycle facade; `bind`, `restartHost`, and `shutdown` delegate to a bound `ApplicationRunner`; missing binding logs and throws `IllegalStateException`.
- `src/main/java/org/saturn/ApplicationRunner.java`: configuration, H2-independent `DataBaseService` construction, optional scheduled health checks, host creation/restart, ten-second scheduler termination wait, one-second stop pauses, JVM shutdown hook, and required `autoReconnect` / `healthCheckInterval` reads.
- `src/main/java/org/saturn/app/facade/Base.java`: shared engine/service wiring, two bounded outbound queues (capacity 256), active-user list, configuration fields, engine-type-specific fields, and DB close behavior.
- `src/main/java/org/saturn/app/facade/Engine.java`: lifecycle and transport facade (`start`, `start(Proxy)`, `stop`, `isConnected`, `dispatchMessage`, `addReplica`) plus mutable room/user setters.
- `src/main/java/org/saturn/app/facade/EngineType.java`: `HOST`, `REPLICA`, `LIST_CMD`, `AGENT`. `ListenerProfile.java`: `PERMANENT`, `TEMPORARY_ONLINE_SET`.
- `src/main/java/org/saturn/app/facade/impl/Connection.java`: Java-WebSocket callbacks; `onOpen` disables lost-connection timeout (`setConnectionLostTimeout(0)`), sends ping, and emits `connected`; `onMessage` emits the raw string; `onError` marks error and throws; `startNonBlocking` uses `connect`, `start` uses `connectBlocking`; `isConnected` checks open/closing/flush/error state; listener lookup is mandatory by name.
- `src/main/java/org/saturn/app/facade/impl/EngineImpl.java`: default listener registration order, nonblocking connection creation, join payload, serialized queue draining, raw-vs-chat output, payload dispatch, replica map, recursive replica stopping, and host/replica shutdown behavior.
- `src/main/java/org/saturn/app/command/impl/admin/ReplicaCommandImpl.java`, `ReplicaOffCommandImpl.java`, `ReplicaStatusCommandImpl.java`: normal replica create/remove/status semantics.
- `src/main/java/org/saturn/app/command/impl/admin/WhiskeyReplicaCommandImpl.java`: agent replica creation, proxy enumeration/testing, five-second startup probes, primary/backup proxy storage, and recursive backup reconnect.
- `src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java`: current-room fast path versus temporary remote-room snapshot workflow.
- `src/main/java/org/saturn/app/command/impl/user/WhiskeyAnonUserCommandImpl.java` and `WhiskeySayUserCommandImpl.java`: direct lookup of the `support` replica and immediate queue flush; absent support is not guarded.
- `src/main/java/org/saturn/app/listener/impl/ConnectionListenerImpl.java`, `IncomingMessageListenerImpl.java`, and `OnlineSetListenerImpl.java`: connected→join, inbound dispatch→flush, online snapshot replacement and HOST-only autorun behavior.
- `src/main/java/org/saturn/app/listener/snapshot/DefaultRoomSnapshotCoordinator.java`, `EngineSnapshotSession.java`, `GsonOnlineSetPayloadParser.java`, and operation classes: temporary onlineSet-only session, 30-second timeout, first correlated snapshot, operation capability context, raw delivery, flush, close, and failure reply.
- Relevant tests: `src/test/java/org/saturn/app/facade/impl/EngineImplTest.java`, `src/test/java/org/saturn/app/command/impl/admin/ReplicaOffCommandImplTest.java`, `ReplicaStatusCommandImplTest.java`, `src/test/java/org/saturn/app/listener/snapshot/DefaultRoomSnapshotCoordinatorTest.java`, `RoomSnapshotOperationsTest.java`, and `GsonOnlineSetPayloadParserTest.java`.

### Observed Zenbot target

- `cmd/zenbot/main.go`: parses flags, subscribes only to `os.Interrupt`, loads TOML, opens pinned H2 on port 5435, creates `model.MASTER`, registers current utility commands, starts the engine in a goroutine, stops on interrupt, then waits on the connection wait group.
- `internal/core/engine_impl.go`: current engine state and lifecycle. `Start` launches `Connection.Connect`, busy-waits until `IsWsConnected`, writes an inline join payload, launches one outbound sharing goroutine, then waits on `EngineWg`. `Stop` cancels ping, closes connection, closes the outbound channel, and waits for connection workers.
- `internal/core/connection.go`: Gorilla WebSocket connection. `Connect` dials once, publishes the dial error, closes `connectCh`, starts reader and ping goroutines. `IsWsConnected` consumes that channel and calls `log.Fatal` on dial failure. Ping is a single 15-second `select`, not a repeating ticker loop. `Write` has no error return and no write mutex. `Close` is nil-safe.
- `internal/common/engine.go`: broad engine contract already includes lifecycle, raw/chat/whisper/addressed output, user state, command registration, presence logging, and `WaitConnectionWgDone`.
- `internal/factory/engine_factory.go`: constructs `core.EngineImpl`, all listeners, `CoreListener`, `Connection`, service bundle, and a ZOMBIE adaptation that replaces repository and selected listeners with dummy implementations.
- `internal/model/engine_type.go`: target enum is `MASTER`, `REPLICA`, `ZOMBIE`; `EngineTypeName` maps those values to `Master`, `Replica`, `ZOMBIE`.
- `internal/listener/core_listener.go`, `user_chat_listener.go`, `info_chat_listener.go`, `user_joined_listener.go`, `user_left_listener.go`, `online_set_listener.go`: inbound routing and state behavior. Message/info chains are ordered and short-circuit when a handler returns false (`internal/listener/message/chain.go`, `internal/listener/info/chain.go`).
- `internal/listener/snapshot/coordinator.go` and `snapshot.go`: target already has a stronger context-aware workflow coordinator with `PENDING/RUNNING/COMPLETED/FAILED/CANCELLED/TIMED_OUT`, correlated session IDs, duplicate workflow rejection, first-snapshot completion, timeout/cancel/transport-close paths, and raw-send/flush/close session interfaces. `snapshot.go` also has a simpler store/parser coordinator.
- `internal/config/config.go`, `Dockerfile`, and `deploy/h2-server.sh`: TOML config includes URL/name/password/channel/auto reconnect/health interval/db path; H2 is external and pinned by the deployment script. Docker currently copies only binary/resources into an Eclipse Temurin runtime and does not copy `config.toml`.
- Existing target tests include `internal/core/engine_impl_test.go`, `internal/listener/snapshot/coordinator_test.go`, `snapshot_test.go`, `internal/listener/*_test.go`, and command/dispatch tests. There is no complete transport/reconnect/replica lifecycle implementation yet.

## 2. Observed runtime sequences and parity contract

### Startup and normal connection

```text
main
  -> SetupConfig
  -> h2.Open (H2 identity/bootstrap)
  -> factory.NewEngine(MASTER,...)
       -> construct engine state/services
       -> construct CoreListener + Connection
       -> construct OnlineSet/UserJoined/UserLeft/UserChat/UserInfo listeners
       -> ZOMBIE substitution, if selected
  -> explicit command registration
  -> go Engine.Start
       -> Connection.Connect (dial once)
       -> IsWsConnected consumes dial result
       -> send join {cmd:join,channel,nick,nick#password}
       -> start outbound queue drainer
       -> wait for engine workers
```

**[OBSERVED Saturn]** `ApplicationRunner.main` binds the lifecycle singleton before `runner.start`; with auto reconnect enabled it schedules an immediate health check, otherwise it starts the host directly. `EngineImpl.start` uses nonblocking WebSocket connect. `Connection.onOpen` emits `connected`; `ConnectionListenerImpl.notify("connected")` immediately sends the join payload. `IncomingMessageListenerImpl` dispatches each inbound frame and then flushes outbound queues.

**[TARGET GAP]** Zenbot's `Start` busy-waits on `IsWsConnected`, while `Connect` can publish a nil WebSocket plus a nonnil error and then still dereference it by starting reader/ping workers. Dial failure is fatal to the process (`log.Fatal`) rather than an engine error. There is no health scheduler/reconnect owner, no startup context/deadline, and no deterministic connected callback; joining is performed in `Start` after channel consumption.

### Shutdown, restart, and signals

```text
SIGINT or lifecycle command
  -> stop scheduler / cancel reconnect work
  -> stop replicas (host owns replica map)
  -> stop outbound producer and close transport
  -> wait readers/pingers/writers
  -> close agent/database resources
```

**[OBSERVED Saturn]** shutdown hook calls `stopApplication`; scheduler is shut down and awaited for 10 seconds, then forced with `shutdownNow`. Only after scheduler termination does it call `stopBot`. Host stop is synchronized by `lifecycleLock`; it calls `host.stop`, clears host reference, sleeps 1 second twice around nulling/GC. `EngineImpl.stop` stops all replicas first, closes the socket, then closes agent service and DB. `restartHost` is serialized by the same lock and creates/starts a new HOST engine after stopping the old one.

**[TARGET GAP]** `main` handles only `os.Interrupt`, does not use `signal.Stop`, has no restart/shutdown lifecycle facade, and waits only on the connection group. `EngineImpl.Stop` closes `OutMessageQueue` while other command producers may still send, so a concurrent send can panic; repeated Stop is not idempotent. Restart is not present. A target lifecycle owner must establish cancellation and ownership before adding replica workers.

### Output, update, and raw semantics

**[OBSERVED Saturn]** `Base` owns separate capacity-256 chat and raw queues. `shareMessages` is synchronized and drains one chat item then one raw item per loop, preserving queue separation; chat strings become `{"cmd":"chat","text":...}` through Gson, while raw strings are written unchanged. `OutService.updateAgentMessage` emits `cmd=updateMessage`, `mode`, normalized text, and `customId` when raw capture is available; without raw capture it falls back to chat output and capture. `JsonPayloads` is the required raw protocol builder. `flushMessage` silently logs when no connection exists.

**[TARGET GAP]** Zenbot has one `OutMessageQueue` for both chat and raw (`SendRawMessage` enqueues an already serialized payload). `SendChatMessage`, `SendWhisperMessage`, and `SendAddressedMessage` manually format chat JSON via `escapeJSON`; tests cover newline normalization and trailing backslashes, but there is no distinct raw queue, `updateMessage` method, output capture seam, or write error propagation. `Connection.Write` ignores write errors. The transport slice must not conflate raw protocol JSON with chat text and must centralize JSON encoding (especially trailing backslashes, CRLF, literal `\\n`, and custom IDs).

### Engine types and listener profiles

| Saturn | Zenbot mapping | Required behavior |
|---|---|---|
| `HOST` | `MASTER` | Full permanent listeners, host configuration, autorun commands, owns replica registry and lifecycle. |
| `REPLICA` | `REPLICA` | Full permanent room listeners, independent connection/DB handle, host registration, no host-only autorun. |
| `LIST_CMD` + `TEMPORARY_ONLINE_SET` | temporary snapshot session (new explicit target profile) | Register only onlineSet correlation listener; never dispatch normal chat/info/join/leave side effects. |
| `AGENT` | likely `REPLICA` with agent capability (not `ZOMBIE`) | Whiskey's named replica; preserve agent-specific onlineSet `nicks` shape only where source requires it. |
| n/a | `ZOMBIE` | No persistence and no stateful normal room side effects; factory currently uses `DummyImpl` and dummy chat/join/left listeners. |

Do not rename `MASTER/REPLICA/ZOMBIE` to Saturn names: target code and tests already use the target enum. Add an explicit profile/capability rather than overloading `ZOMBIE` for temporary snapshots.

### Listener registration and dispatch ordering

**[OBSERVED Saturn]** `EngineImpl.registerDefaultPayloadListeners` inserts `onlineSet` first, then (permanent profile only) `onlineAdd`, `onlineRemove`, `chat`, `info`. `dispatchMessage` extracts `cmd`, ignores `join`, looks up the registered listener, and logs/returns for unknown or malformed payloads. `onlineSet` replaces active users and HOST then executes comma-separated autorun commands. For ordinary chat/info, listener chains preserve handler order and stop on the first handler returning false.

**[TARGET]** `factory.NewEngine` creates `CoreListener` and `Connection` before the typed listeners, then assigns listeners in online/chat/info/join/left order. `DispatchMessage` currently uses a switch and directly invokes all five fields; `join` and `session` are no-ops. The architecture should replace this with an ordered registration table so temporary sessions can register only `onlineSet`, unknown commands remain nonfatal, and the order is test-visible. Preserve `onlineSet` full replacement, validation, and callback-after-replacement semantics.

### Replica, remote-room, and whiskey boundaries

**Replica command boundary:** Saturn `ReplicaCommandImpl` trims the first argument, rejects blank/host/duplicate channels, creates a new `EngineImpl(REPLICA)` with a separate DB connection, sets channel, nick=`hostNick + "Replica"`, password, registers it before starting, then replies with current replica count. `ReplicaOff` stops then removes; `ReplicaStatus` reports host channel/count and map keys. Target should expose an engine-manager/host-owned `map[string]*EngineImpl` with atomic add/remove and idempotent stop; do not put replica lifecycle in command parsing.

**Whiskey boundary:** Saturn `WhiskeyReplicaCommandImpl` creates `EngineType.AGENT` replicas, tests configured SOCKS proxies concurrently, waits for each future without an explicit timeout, sleeps 5 seconds after each nonblocking start, chooses the first healthy result, stores remaining backup proxies, and recursively consumes backups on reconnect. Fast path starts direct and registers only after start. Preserve duplicate behavior, default name `portal`, proxy parsing, and exact replies, but replace unbounded recursion/sleeps with context-aware deadlines and iterative retry while documenting the observable adaptation. Proxy TLS trust-all is source behavior and a security risk requiring explicit approval before reproducing.

**Remote-room boundary:** `MsgChannelCommandImpl` normalizes `?`, joins message arguments, uses a current-room direct chat path, otherwise creates UUID workflow + `LIST_CMD` temporary engine/session. The temporary session receives one correlated `onlineSet`; `DeliverMessageToRoomOperation` considers a room empty if every snapshot user is `isMe`, otherwise sends raw `chat` addressed to `*`, flushes, replies, and closes. The operation receives only narrow context (`reply`, `sendRaw`) and must not gain the host's general engine or DB access.

**Whiskey user boundary:** `WhiskeySay` and `WhiskeyAnon` write directly to the `support` replica and call `shareMessages`. Preserve this as a named cross-room gateway, but return a controlled unavailable-replica error rather than target nil-pointer panic unless strict defect parity is explicitly required.

## 3. Target architecture and proposed interfaces

These are recommendations, not existing APIs.

```go
type EngineProfile int
const ( PermanentProfile EngineProfile = iota; TemporaryOnlineSetProfile )

type Transport interface {
    Start(ctx context.Context) error       // establishes read/write ownership
    Close(ctx context.Context) error       // idempotent
    Connected() bool
    SendText(ctx context.Context, payload string) error
    SendRaw(ctx context.Context, payload []byte) error
    Errors() <-chan error
    Messages() <-chan []byte
}

type EngineManager interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Restart(ctx context.Context) error
    Healthy() bool
    AddReplica(ctx context.Context, channel string, e Engine) error
    RemoveReplica(ctx context.Context, channel string) (Engine, error)
    Replicas() map[string]Engine // snapshot copy, never caller-owned map
}

type SnapshotSession interface {
    ID() string
    Start(ctx context.Context) error
    Close(ctx context.Context) error
    Flush(ctx context.Context) error
    SendRaw(ctx context.Context, payload []byte) error
}
```

Recommended `core.EngineImpl` additions: `context.Context`/cancel ownership, `sync.Once` stop, a `sync.RWMutex` around replica registry and listener table, separate `OutMessageQueue` and `OutRawMessageQueue` (or a typed outbound envelope), `Connected` state guarded by mutex/atomic, an explicit `EngineProfile`, and a transport interface injected by factory. `common.Engine` should expose only stable command-facing operations; do not expose concrete `*EngineImpl` or internal maps to commands.

Recommended lifecycle owner: `internal/core/lifecycle.go` (new), with `Start`, `Stop`, `Restart`, health ticker, reconnect policy, and signal-independent context. `cmd/zenbot/main.go` should only load config/H2, build a lifecycle, register commands, call `Run(ctx)`, and cancel on `SIGINT`/`SIGTERM`. H2 startup remains fail-closed and `SELECT H2VERSION()` remains owned by `internal/repository/h2`.

Recommended snapshot integration: adapt existing `internal/listener/snapshot.RoomSnapshotCoordinator` to a `SessionFactory` backed by a temporary transport/engine profile. Keep workflow state/correlation and narrow `RoomSnapshotContext`; never register temporary sessions in the host replica map.

## 4. Non-overlapping file-level change map

### Task-owned transport/lifecycle/replica files

- **Create** `internal/transport/connection.go`: Gorilla transport adapter with dial context, read loop, write serialization, ping/pong policy, close/error events, and explicit send errors.
- **Create** `internal/transport/connection_test.go`: dial failure, close, write failure, ping cancellation, concurrent writes, and message/error delivery tests using a local WebSocket test server.
- **Create** `internal/core/lifecycle.go`: lifecycle state machine, health/reconnect ticker, restart/stop ordering, context and idempotence.
- **Create** `internal/core/lifecycle_test.go`: startup, shutdown, restart, scheduler cancellation, signal-equivalent cancellation, and timeout tests.
- **Create** `internal/core/replica_manager.go`: host-owned replica registry, atomic duplicate/add/remove, stop snapshot, backup reconnect policy.
- **Create** `internal/core/replica_manager_test.go`: duplicate channels, stop ordering, concurrent add/remove, backup exhaustion, and no post-stop sends.
- **Extend, serialized with DBZ** `internal/core/engine_impl.go`: transport/profile/listener-table seam, typed outbound queues, safe stop, join construction, update/raw send semantics, and manager delegation. Avoid DBZ-specific edits.
- **Extend, serialized with DBZ** `internal/core/connection.go` only if the DBZ worker agrees to move connection ownership; otherwise leave it as a compatibility shim and have new transport code implement the lifecycle.
- **Extend, serialized with DBZ** `internal/common/engine.go`: only minimal stable interfaces needed by lifecycle/replica commands; do not remove current DBZ service accessors.
- **Extend, serialized with DBZ** `internal/factory/engine_factory.go`: inject transport/profile and build permanent versus temporary engines; preserve existing DBZ service wiring.
- **Extend** `internal/listener/snapshot/coordinator.go` and/or **create** `internal/listener/snapshot/session_factory.go`: bridge temporary session transport and lifecycle cancellation without changing operation capabilities.
- **Extend** `internal/config/config.go`: typed reconnect/connect/stop/ping/proxy settings and defaults; keep H2 fields intact.
- **Extend, serialized with DBZ** `cmd/zenbot/main.go`: lifecycle owner and SIGTERM handling only; do not alter DBZ registration logic without coordination.
- **Extend** `internal/model/engine_type.go` only if profile metadata is required; retain `MASTER`, `REPLICA`, `ZOMBIE` values.
- **Create** `internal/command/replica.go`, `remote_room.go`, `whiskey.go`: concrete command boundaries and manager/session interfaces; preserve aliases and exact replies.
- **Extend, serialized with DBZ** `internal/command/registry.go` and `dispatch_adapter.go`: register concrete replica/remote-room/whiskey commands, not generic placeholders. DBZ worker owns adjacent registry edits; serialize changes or merge manually.
- **Create** `internal/command/replica_test.go`, `remote_room_test.go`, `whiskey_test.go`, plus transport-backed integration tests under `internal/integration/` if needed.
- **Extend** `Dockerfile`, `deploy/h2-server.sh`, and documentation only for lifecycle environment variables/health operation; preserve H2-only deployment and do not reintroduce SQLite.

### Explicitly not owned by this slice

Do not modify Saturn. Do not modify identity implementation, DBZ repositories/services/commands, service bundle internals, schema, H2 repository methods, agent runtime, or unrelated moderation/user command files. Do not rewrite the dirty worktree.

## 5. Shared files requiring serialization with the DBZ worker

**Must be coordinated before any edit:**

1. `internal/core/engine_impl.go` — DBZ implementation already touched engine service-bundle/accessor and output seams.
2. `internal/common/engine.go` — DBZ depends on the current command-facing interface.
3. `internal/factory/engine_factory.go` — DBZ service wiring is already present here.
4. `internal/command/registry.go` — DBZ canonical/alias definitions and catalog roles are worker-owned.
5. `internal/command/dispatch_adapter.go` — DBZ conditional runtime registration is worker-owned.
6. `internal/command/handlers.go` — DBZ implementation handoff identifies concrete construction/output propagation here.
7. `internal/service/services.go` — `Bundle.DBZ` is worker-owned.
8. `cmd/zenbot/main.go` — both lifecycle startup and DBZ runtime registration can converge here.
9. `internal/repository/repository.go` — DBZ repository interface is worker-owned; lifecycle should not alter it.

Use separate commits or a serialized lock/merge pass for these paths. Task-owned new files under `internal/transport`, `internal/core/lifecycle.go`, `internal/core/replica_manager.go`, and dedicated replica/remote-room/whiskey tests are disjoint, subject to imports and final integration review.

## 6. Parity and failure requirements

- Preserve Saturn's join payload field meanings and `nick#password` construction; use JSON encoding rather than string interpolation.
- Preserve onlineSet replacement, temporary `onlineSet`-only profile, agent `nicks` parsing where applicable, and HOST-only autorun.
- Preserve permanent registration order: onlineSet, onlineAdd, onlineRemove, chat, info. Assert order and short-circuit behavior with spy listeners.
- Preserve separate chat/raw semantics. Raw update payloads (`updateMessage`, `mode`, `text`, `customId`) must not be wrapped in chat. All payloads must be valid JSON with trailing backslashes and newline forms preserved correctly.
- Preserve replica duplicate/host rejection, add-before-start behavior for normal replicas, status ordering contract (sort target output if nondeterminism is unacceptable), and remove/stop behavior.
- Preserve whiskey proxy order, primary/backup semantics, default `portal`, five-second source probe meaning as a configurable deadline, and exact success/error response text. Bound retries and cancellation explicitly.
- Preserve remote-room empty-room result (`all users are self`), one-shot correlation, late-event rejection, operation reply, raw delivery, flush, and close.
- Preserve ZOMBIE's no-persistence/dummy normal-listener boundary; do not accidentally give it a DB connection or replica ownership.
- Stop must be idempotent and must prevent sends after queue closure. Transport errors must be observable to lifecycle/reconnect policy, not silently swallowed.
- Config errors and H2 errors fail closed before WebSocket startup. H2 identity/version checks remain in repository startup, not transport.

## 7. Focused and integration test plan

### Focused tests

1. `internal/transport/connection_test.go`: local server handshake, connected event, inbound raw frame, dial timeout, close before connect, server close/error, write error, concurrent writer serialization, ping cancellation, and no nil-WebSocket dereference.
2. `internal/core/lifecycle_test.go`: deterministic fake transport; start→join, health success/no restart, failed connection retry, backoff/deadline, cancellation during dial, stop waits workers, restart stops old engine before new engine, repeated Stop/Restart behavior.
3. `internal/core/replica_manager_test.go`: add duplicate/host channel, remove missing, stop all snapshot, concurrent map access, backup proxy sequence and exhausted backups.
4. Engine payload golden tests: join, chat, whisper, addressed public/whisper, raw command, updateMessage, real newline, literal `\\n`, CRLF, quotes, and trailing backslash; inspect actual wire bytes/queued envelopes.
5. Listener registration tests: permanent order, temporary onlineSet-only set, unknown command, malformed JSON, onlineSet replacement, HOST autorun only, and ZOMBIE dummy listeners.
6. Snapshot coordinator tests: correlated session, duplicate/late snapshot, malformed snapshot, operation error, timeout, cancel, close, raw-send failure, flush-before-close, and no host replica-map insertion.
7. Command tests: replica/replicaoff/status exact aliases/parsing/replies; msgroom current/remote/empty/error paths; whiskey no-proxy/proxy primary/backup/duplicate/default name/cancel; support replica absent behavior.

### Integration gates

- Run a local Gorilla WebSocket server and real engine factory for MASTER and REPLICA; assert join and listener event order.
- Run two engines against the server and verify host registry, replica stop ordering, and no cross-room listener leakage.
- Run a remote-room snapshot workflow against a fake session server and assert one snapshot, raw `chat` to `*`, flush, reply, and close.
- Exercise H2-backed MASTER startup only after `h2.Open` and `SELECT H2VERSION()` pass; temporary snapshot/ZOMBIE engines must not open a DB.
- After DBZ worker merge, run `gofmt`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and `go build ./...`; inspect dirty paths to confirm no unrelated changes were lost.

## 8. Risks, limitations, and complexity

- **Current target transport is unsafe:** dial errors do not stop subsequent worker setup; `log.Fatal` is embedded in a library method; connect result is single-consumer; ping is not periodic; writes have no serialized ownership or returned error.
- **Shutdown race:** closing a shared outbound channel while commands enqueue can panic. This is the highest correctness risk and overlaps `engine_impl.go`/`common.Engine` with DBZ.
- **Source quirks versus safe adaptation:** Saturn throws from WebSocket `onError`, disables lost-connection detection, uses sleeps and unbounded recursive proxy retry, and has nil-sensitive whiskey support. Preserve observable successful behavior but document bounded/cancelable target adaptations.
- **Engine enum mismatch:** Saturn has four operational types while Zenbot has three. Temporary snapshot and agent semantics cannot be represented safely by enum alone; use profile/capability.
- **Nondeterministic maps:** Saturn status joins `ConcurrentHashMap.keySet`; target maps are unordered. Golden tests must either assert set membership or define a documented stable ordering.
- **Source test coverage:** Saturn has focused facade/replica-off/status/snapshot tests but no complete lifecycle/reconnect transport suite; target tests must establish the missing contract.
- **Security:** source whiskey proxy code installs a trust-all SSL context. Reproducing that behavior is dangerous; prefer normal certificate validation unless a migration owner explicitly accepts the deviation.
- **Complexity: High (8/10).** The target has partial transport/listener/snapshot foundations, but lifecycle ownership, retries, safe shutdown, temporary sessions, proxy failover, exact output semantics, and integration with DBZ-shared files must be completed without disturbing a dirty migration worktree.

## 9. Architecture acceptance checklist

- [ ] Source citations above remain valid against Saturn `develop`.
- [ ] MASTER/REPLICA/ZOMBIE and temporary snapshot profile behavior is explicit and tested.
- [ ] Lifecycle owns cancellation, retries, deadlines, restart, and idempotent shutdown.
- [ ] WebSocket reads/writes/pings have clear ownership and errors are not discarded.
- [ ] Chat, raw, and updateMessage payloads are separate and JSON-safe.
- [ ] Listener registration order and temporary isolation are deterministic.
- [ ] Replica manager owns add/remove/stop/backup state; commands do not own goroutines or maps.
- [ ] Remote-room workflow is narrow, correlated, one-shot, timeout-bound, and close-safe.
- [ ] Whiskey proxy testing/reconnect is bounded, cancelable, and preserves source response semantics.
- [ ] All shared paths listed in §5 are serialized with the DBZ worker.
- [ ] Focused tests and full race/vet/build gates pass after implementation.
- [ ] Only the handoff artifact is modified during this architecture phase.
