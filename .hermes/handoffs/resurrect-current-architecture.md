# Explicit three-argument `resurrect` remote-snapshot parity — Technical Specification

## Scope and decision

Implement one bounded moderator vertical: `resurrect <nick> <from> <to>` and its Saturn aliases `move`, `recover`, and `heal`. The path must select an already-serving managed engine for `<from>` when one exists; otherwise it must obtain an online-set snapshot by creating a temporary session in `<from>`, verify target presence, and send one move-kick from that temporary session.

**Strict exclusions:** zero-argument “last moved user” behavior, `lastKicked`/`kickedTo` storage, automove, mine, agent integrations/tools, generic catalog fallback activation, and new general remote-command infrastructure.

## Evidence record (observed, not recommendations)

### Saturn authority

- **[OBSERVED]** `ResurrectUserCommandImpl.execute()` in `saturn/src/main/java/org/saturn/app/command/impl/moderator/ResurrectUserCommandImpl.java` accepts zero arguments by calling `resurrectLastMovedUser`; that branch is explicitly out of scope here. For a non-empty argument list other than exactly three it replies `" " + <prefix> + "move <nick> <from> <to>"` and returns `Status.FAILED`.
- **[OBSERVED]** The same class normalizes the first argument with `IdentityUtil.normalizeNickTarget`, leaves `<from>` and `<to>` unchanged, and is `Role.MODERATOR` with aliases `move`, `recover`, `heal`, `resurrect`.
- **[OBSERVED]** `moveUser` looks up a live source engine by exact channel key: replica first, then host when `channel.equals(hostRef.channel)`. A found source emits `JsonPayloads.command("kick", "nick", target, "to", destination)` through that engine’s outbound service and shares messages. It does **not** locally pre-check presence.
- **[OBSERVED]** When no serving engine exists, `UserCommandBaseImpl.resurrect(...)` constructs a one-off `LIST_CMD` snapshot session for the source channel and submits `KickOrResurrectOperation`; see `saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java` and `saturn/src/main/java/org/saturn/app/listener/snapshot/KickOrResurrectOperation.java`.
- **[OBSERVED]** `KickOrResurrectOperation.apply(...)` compares the normalized target against snapshot user nicks using identity-equivalent comparison. If absent it returns the exact operation reply `" " + target + " isn't in the room"`; it emits no kick. If present it sends exactly `kick/nick/to` through the temporary snapshot context.
- **[LIMITATION]** The two Saturn files do not define a user-facing reply for a successful explicit move or a distinct transport/snapshot failure reply; source success is an execution status plus the raw action.

### Accepted Zenbot substrate

- **[OBSERVED]** `internal/common/room_snapshot.go` exposes the command-facing capability only as `RoomSnapshotSubmitter.SubmitRoomSnapshot(snapshot.RoomSnapshotRequest) error`.
- **[OBSERVED]** `internal/listener/snapshot/coordinator.go` owns validation, session creation/start, one correlated snapshot parse, terminal state, outcome/reply publishing, flush/close after successful operation execution, and close on failure/cancel/timeout/transport close. `RoomSnapshotOperation` is `Apply(RoomSnapshotContext, Snapshot) (OperationResult, error)`; `RoomSnapshotContext.SendRaw` is supplied only from the temporary session.
- **[OBSERVED]** `internal/listener/snapshot/session_factory.go` gives temporary sessions their own registry entry and context, joins `request.SourceChannel`, routes temporary transport errors/closes to the coordinator, and removes the entry on close. `factory.NewCoordinatedSessionFactory` is documented as not registering a temporary session as a replica in `internal/factory/engine_factory.go`.
- **[TEST-BACKED]** `internal/listener/snapshot/coordinator_test.go` covers successful first-snapshot cleanup, operation error, transport error, close, explicit cancellation, start failure, timeout, and late-payload rejection.
- **[OBSERVED]** `internal/command/nuke.go` is the accepted command shape: reject pre-cancelled direct contexts, submit a populated request without immediate raw/send reply, return successful only after submit succeeds, and keep snapshot actions in `internal/listener/snapshot/nuke_operation.go`.
- **[OBSERVED]** Snapshot field naming has a material transport fact: `CoordinatedSessionFactory.Create` creates its `transportSession` with `join: req.SourceChannel` (`internal/listener/snapshot/session_factory.go`), while `nukeCommand` currently fills `SourceChannel` from the invoking message and remote room in `TargetChannel`. For this `resurrect` remote fallback, the request must set `SourceChannel` to `<from>` so the temporary session actually joins the source room. The invoking room is not a required delivery field: accepted `roomSnapshotReplySink` delivers by `Author` and `Whisper`.
- **[OBSERVED]** `internal/core/moderation_operations.go` provides `common.ModerationOperations.KickNickTo(context.Context, common.NickTarget, common.Channel) error`, which normalizes the nick and serializes `{"cmd":"kick","nick":...,"to":...}`. This is the existing typed live-engine action boundary.
- **[OBSERVED]** `core.EngineRoomUserDirectory` in `internal/core/room_directory.go` can only return immutable users for a host/managed replica; it does not return an action-capable serving engine. `ReplicaManager.ManagedEngines()` is the existing controlled view of managed replicas in `internal/core/replica_manager.go`.
- **[OBSERVED]** `cmd/zenbot/main.go` already composes the master snapshot registry/coordinator/factory, installs the coordinator through `factory.EngineOptions`, uses a reply sink that preserves `RoomSnapshotRequest.Whisper`, and sets a managed replica controller after master construction.
- **[OBSERVED]** `internal/command/dispatch_adapter.go` registers only concrete canonical commands. It adds `nuke` only when `common.RoomSnapshotSubmitter` is present; inbound legacy dispatch invokes `Execute(context.Background())`, so it cannot preserve an inbound cancellation context.
- **[LIMITATION]** `RoomSnapshotRequest`, `RoomSnapshotOperation`, and `RoomSnapshotContext` have no `context.Context`; coordinator cancellation is workflow-ID based. A direct command can reject a context already cancelled before submission, but cancellation after submit cannot currently cancel the snapshot workflow.

## Recommended Technical Specification

### Standard versus High Complexity/Risk

| Area | Rating | Reason |
|---|---|---|
| Command parsing, aliases, normalized explicit request, snapshot operation | **Standard** | Follows accepted `nuke` command/operation split and existing typed `KickNickTo` payload vocabulary. |
| Serving-engine selection and temporary-session lifecycle boundary | **High Complexity/Risk** | It crosses master/managed-replica ownership and can otherwise misroute a moderation action or treat a temporary session as a replica. Senior review required for this narrow capability seam. |
| Coordinator modifications | **High Complexity/Risk — avoid for this slice** | The accepted coordinator has terminal-state races and cleanup semantics already covered. Reuse it unchanged unless a new RED test proves a concrete blocker. |

### File/module map

| File | Required responsibility |
|---|---|
| `internal/common/resurrect.go` **(new)** | Define the narrow command capability for “try a live managed source move” without exposing core/replica internals. |
| `internal/core/resurrect.go` **(new)** | Implement the capability on `*core.EngineImpl`, selecting host then managed replica by source channel and calling the selected source engine’s typed `KickNickTo`. No temporary session creation here. |
| `internal/command/resurrect.go` **(new)** | Own argument count, usage, target normalization, pre-submit cancellation check, live-first choice, request construction, and status/error return. It owns no raw JSON. |
| `internal/listener/snapshot/kick_or_resurrect_operation.go` **(new)** | Own parsed-snapshot presence verification and the fallback raw `kick/nick/to` serialization/sending. |
| `internal/command/handlers.go` | Add the concrete `resurrect` case to `newCommand`; it must replace the generic `saturnCommand` branch for this canonical only. |
| `internal/command/dispatch_adapter.go` | Add canonical `resurrect` only when the full live-or-snapshot capability set is present (rule below). |
| `internal/command/resurrect_test.go` **(new)** | Direct command, registration, aliases, exact usage, live/fallback action ownership, and cancellation RED/GREEN tests. |
| `internal/listener/snapshot/kick_or_resurrect_operation_test.go` **(new)** | Snapshot presence and exact raw action tests. |
| `internal/core/resurrect_test.go` **(new)** | Host/managed-replica source selection, no opaque replica selection, and typed action forwarding tests. |
| `cmd/zenbot/main.go` | No new production composition is required if `*core.EngineImpl` implements the recommended capability through its installed managed replica controller. Add/update composition test only to prove the master satisfies both capabilities. |

### Narrow interfaces and signatures

```go
// internal/common/resurrect.go
// LiveRoomMover handles only a source channel already served by the host or a
// managed replica. handled=false means “no live source is serving from”.
type LiveRoomMover interface {
    MoveFromServingRoom(ctx context.Context, from string, target NickTarget, to Channel) (handled bool, err error)
}
```

```go
// internal/core/resurrect.go
func (e *EngineImpl) MoveFromServingRoom(
    ctx context.Context, from string, target common.NickTarget, to common.Channel,
) (handled bool, err error)
```

Required behavior of this implementation:

1. Reject an already-cancelled `ctx` before lookup/action (`handled=false`, `ctx.Err()`).
2. Compare `<from>` by exact channel value, first against a managed replica and then the host, matching Saturn’s observable `replicasMappedByChannel.get(channel)` then `channel.equals(host.channel)` precedence. Do not inspect opaque `Replica` values or normalize/trim `<from>` for the decision.
3. If no host/managed source matches, return `(false, nil)`.
4. If a source matches, invoke that source’s `common.ModerationOperations.KickNickTo(ctx, target, to)` and return `(true, err)`. Do not fall back to a temporary snapshot after a live source is found, including a live action error: source Saturn chooses live when it exists.
5. The caller, not this method, decides whether `(false, nil)` means submit snapshot fallback.

No change is recommended to these accepted seams:

```go
// existing
common.RoomSnapshotSubmitter.SubmitRoomSnapshot(snapshot.RoomSnapshotRequest) error
snapshot.RoomSnapshotOperation.Apply(snapshot.RoomSnapshotContext, snapshot.Snapshot) (snapshot.OperationResult, error)
common.ModerationOperations.KickNickTo(context.Context, common.NickTarget, common.Channel) error
```

### Exact registration and execution gate

**Registration rule:** in `RegisterUserUtilitiesWithDirectAgent`, append only canonical `resurrect` **iff** the engine implements both `common.LiveRoomMover` and `common.RoomSnapshotSubmitter`. It must add no separate aliases: registering the one catalog definition exposes exactly its reviewed aliases `move`, `recover`, `heal`, `resurrect`, all `MODERATOR`. Neither capability alone is sufficient. Do not register a catalog placeholder and do not make registration depend on agent, automove, mine, or a generic raw sender.

**Execution rule:** after direct-context cancellation and exact-arity checks, normalize `<nick>` with `util.NormalizeNickTarget`. Call `LiveRoomMover.MoveFromServingRoom(ctx, from, common.NickTarget(nick), common.Channel(to))` exactly once.

- `handled=true, err=nil`: return `SUCCESSFUL`; submit no snapshot and emit no command-owned raw/reply.
- `handled=true, err!=nil`: return `FAILED, err`; submit no snapshot and emit no success reply.
- `handled=false, err!=nil`: return `FAILED, err`; submit no snapshot.
- `handled=false, err=nil`: assert/use `RoomSnapshotSubmitter`, generate a `resurrect-` workflow ID, and submit one request with `Author`, inbound `Whisper`, `SourceChannel: from` (the field the current factory joins), `TargetChannel: from` (the operation’s room/error context), `DestinationChannel: to`, `ReplyMessage: "Unable to complete room operation."`, and `Operation: snapshot.NewKickOrResurrectOperation(nick)`. Do not place the invoking message channel in `SourceChannel`: that would join the wrong room under the accepted factory implementation.
- fallback submit error: return `FAILED, err`; do not emit a command reply/raw action.
- fallback submit success: return `SUCCESSFUL, nil`; do not report move success before the asynchronous operation completes.

Although registration proves both capabilities in normal inbound use, direct construction must still fail explicitly if a required capability is absent. This preserves fail-closed behavior for unit callers and composition regressions.

### Command and action semantics

1. `ctx.Err()` first: `FAILED, err`, no live move, snapshot submit, reply, or raw action.
2. Parse `args(c.message)`. Only length **3** is valid. Any other count, including zero, replies exactly source-shaped `" "+prefix+"move <nick> <from> <to>"`, returns `FAILED, nil`, and performs no action. Canonical source spelling remains `move` even when invoked as `recover`, `heal`, or `resurrect`.
3. Normalize only target with `util.NormalizeNickTarget`; blank/bare target is a failed usage case with the same exact usage reply and no action. Preserve `<from>`/`<to>` literally as parsed, matching Saturn’s explicit command.
4. Use the execution gate above.
5. `KickOrResurrectOperation.Apply` scans `snapshot.Users` in order. A nil user or a user nick that cannot be canonicalized is non-matching, not a panic. Match with `util.SameNick`; the operation’s captured target is normalized at construction (constructor may return/retain validation error, or command supplies a known-good normalized target).
6. If absent, return `snapshot.Absent(" "+target+" isn't in the room")`, call `SendRaw` zero times, and return no Go error. The coordinator delivers the absent reply through `roomSnapshotReplySink` using request whisper mode.
7. If present, marshal exactly `{"cmd":"kick","nick":<normalized target>,"to":<destination>}` and call `ctx.SendRaw` once. On missing sender, marshal failure, or send failure, return `snapshot.Failed()` plus a non-nil error so coordinator marks workflow failed, publishes generic `ReplyMessage`, and closes the temporary session. On success return `snapshot.Success()` with no reply.

**Typed/raw ownership:** the live path is typed (`LiveRoomMover` → existing `ModerationOperations.KickNickTo`); the remote fallback operation owns the raw payload because only the coordinator-created temporary session has the correct room transport. `command` owns no raw JSON; `core` owns no fallback raw JSON/session; the coordinator owns neither command policy nor source selection.

### Proposed lifecycle/data-flow sequence

```text
[PROPOSED — explicit three-argument slice]
Moderator → legacy dispatch: !move @Alice source-room destination-room
legacy dispatch → resurrectCommand.Execute(context.Background())
resurrectCommand → util.NormalizeNickTarget: Alice
resurrectCommand → LiveRoomMover: MoveFromServingRoom(source-room, Alice, destination-room)

alt host or managed replica serves source-room
  LiveRoomMover → source ModerationOperations.KickNickTo
  source engine → source transport: {cmd:kick,nick:Alice,to:destination-room}
  resurrectCommand → caller: SUCCESSFUL
else no live source
  resurrectCommand → RoomSnapshotSubmitter: SubmitRoomSnapshot(request target=source-room,destination=destination-room)
  submitter → Coordinator → CoordinatedSessionFactory: create temporary session (not a replica)
  temporary session → source-room transport: {cmd:join,channel:source-room}
  source-room transport → Coordinator: onlineSet payload (one correlated session)
  Coordinator → KickOrResurrectOperation: Apply(context, parsed Snapshot)
  alt Alice present (SameNick)
    operation → temporary session SendRaw: {cmd:kick,nick:Alice,to:destination-room}
    Coordinator → temporary session: Flush, Close, registry removal
  else absent
    operation → Coordinator: Absent(" Alice isn't in the room")
    Coordinator → roomSnapshotReplySink: author + reply + request.Whisper
    Coordinator → temporary session: Flush, Close, registry removal
  end
  resurrectCommand → caller: SUCCESSFUL after Submit only
end
```

### Error, timeout, and cancellation semantics

| Condition | Required result |
|---|---|
| Pre-cancelled direct context | `FAILED, context.Canceled` (or context error), no side effect. |
| Wrong arity / invalid normalized nick | exact usage reply; `FAILED, nil`; no side effect. |
| Live capability missing in direct construction | `FAILED` plus explicit configuration error; no fallback. |
| Live source found, typed kick fails | `FAILED, err`; no snapshot fallback and no success reply. |
| No source found, submitter missing | `FAILED` plus explicit configuration error; no raw/reply. |
| Snapshot submit/create/start/parse/transport close/error/timeout | Preserve coordinator’s existing generic `"Unable to complete room operation."` delivery only when `ReplyMessage` is non-empty; state is terminal and temporary session is closed/removed. |
| Explicit `Coordinator.Cancel(workflowID, reason)` | Existing coordinator cancels and closes; command has no context-to-workflow cancellation bridge in this bounded slice. |
| Target absent in valid snapshot | `OutcomeAbsentTarget`, exact absent reply through original whisper/public delivery mode, no kick. |
| Temporary raw kick fails | Non-nil operation error; coordinator records failure, emits generic configured failure reply, closes/removes session. |
| Late snapshot/transport event after terminal state | Ignored by existing coordinator (`false`); operation must not execute again. |

Do not claim post-submit caller cancellation parity: the accepted legacy adapter supplies `context.Background()` and the coordinator API has no context field. Adding a context/workflow cancellation bridge changes shared substrate behavior and is out of scope.

## Test map and TDD order

### First exact RED tracer

Before implementation, add only this direct command test and run it red:

```sh
go test ./internal/command -run '^TestResurrectCommandFallsBackToSnapshotOnlyWhenNoLiveSourceServesFrom$' -count=1
```

It must construct an engine stub implementing both capabilities where `MoveFromServingRoom` returns `(false, nil)`, invoke `!move @Alice source-room destination-room`, and assert all of the following exactly:

- status is `SUCCESSFUL`, error is nil;
- live mover is called once with `from == "source-room"`, normalized `Alice`, and `destination-room`;
- exactly one snapshot request is submitted with target/source/destination/author/whisper fields above;
- no command-engine raw message or immediate chat is emitted;
- request operation is the concrete kick-or-resurrect operation.

The test must fail first because no concrete resurrect command/case exists; it is the acceptance tracer for remote fallback, not a broad catalog test.

### Focused tests required before acceptance

| Location | Test coverage |
|---|---|
| `internal/command/resurrect_test.go` | exact three-argument usage for zero, 1, 2, and >3; `@` normalization; aliases/role; pre-cancel no side effects; live host/replica handled path uses typed mover and does not submit; live error does not fallback; no-live path submits exact request; missing capabilities fail closed; submit error returns failed. |
| `internal/command/dispatch_adapter_test.go` or `resurrect_test.go` | baseline registry differs by exactly canonical/its four reviewed aliases only when **both** capability interfaces exist; no alias registers when either is absent; registered role is `MODERATOR`. |
| `internal/core/resurrect_test.go` | host selection; managed-replica selection; no match returns `(false,nil)`; opaque replica is not selected; selected engine receives `KickNickTo` with normalized typed values; cancellation is checked before action. |
| `internal/listener/snapshot/kick_or_resurrect_operation_test.go` | case-insensitive presence, `@` normalization, absent exact reply/no raw, successful exact decoded JSON (`cmd`, `nick`, `to`), missing sender/send error results, malformed/nil snapshot user does not panic or kick. |
| `internal/listener/snapshot/coordinator_test.go` | reuse existing lifecycle matrix; add only a concrete operation integration assertion if needed that `AbsentTarget` publishes original whisper mode and successful fallback flushes/closes once. |
| `cmd/zenbot/room_snapshot_composition_test.go` | master composition still supplies snapshot submitter and now the live mover capability; temporary sessions remain outside replica manager. |

### Verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after the implementation owner’s changes:

```sh
go test ./internal/command ./internal/listener/snapshot ./internal/core -count=1
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

## Documentation baseline recorded before implementation

All cited Zenbot and Saturn paths/symbols listed above were read and resolved before this handoff was written. The repository was already dirty; this handoff does not authorize reset, clean, checkout, staging, or commits.

```text
go test ./internal/command ./internal/listener/snapshot ./internal/core -count=1
# recorded result: PASS (see handoff verification update if the implementation worktree changes)

git diff --check
# recorded result: PASS (no whitespace errors)
```

## Completeness checklist

- [x] Source evidence is separated from recommendations.
- [x] Source aliases, moderator authorization, usage, target normalization, live-versus-snapshot selection, presence check, payload/delivery, and excluded zero-argument behavior are explicit.
- [x] Capability-gated registration and execution are fail-closed and canonical-only.
- [x] Typed live action and fallback raw-action/session ownership are distinct.
- [x] Existing timeout, terminal cleanup, late-event, and cancellation limits are stated without inventing end-to-end cancellation.
- [x] First exact RED tracer, focused test map, and risk rating are included.
