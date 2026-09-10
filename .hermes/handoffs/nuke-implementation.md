# Moderator nuke remote-snapshot implementation

## Scope and result

Implemented the isolated canonical moderator `nuke` vertical. It is registered only when the serving engine exposes `common.RoomSnapshotSubmitter`; direct execution also rejects an engine that lacks that capability. No aliases beyond Saturn's canonical `nuke` alias were added. No resurrect, mine, automove, agent, replica lifecycle, or semantic activation work was changed.

## Source grounding

- Saturn command authority: `../saturn/src/main/java/org/saturn/app/command/impl/moderator/NukeCommandImpl.java`, `execute()` lines 40–72. It removes `@` from arguments, returns the exact `\\n Example: <prefix>nuke hotlinks` usage reply for no arguments, authorizes `MODERATOR`, and begins a snapshot workflow for the requested room.
- Saturn operation authority: `../saturn/src/main/java/org/saturn/app/listener/snapshot/NukeRoomOperation.java`, `apply()` lines 22–49. It normalizes snapshot nicknames, sends `ban` actions in snapshot order with a 200 ms delay, and uses `lockroom` on the temporary target-room session.
- Zenbot lifecycle seam retained unchanged: `internal/listener/snapshot/coordinator.go`, `RoomSnapshotRequest`, `RoomSnapshotCoordinator.Submit`, `workflow.receive`, and `workflow.fail`. The command submits only a request; the coordinator retains session creation/routing/terminal cleanup; `NukeRoomOperation` owns snapshot-derived raw actions.
- Existing protocol convention: `internal/core/moderation_operations.go`, `moderationPayload`, `BanNick`, and `LockRoom` establish JSON `{cmd:"ban",nick:...}` and `{cmd:"lockroom"}` shapes.

## Behavior

- `nuke` strips every `@` from its first room argument, populates workflow ID, author, inbound source channel, target channel, and inbound whisper mode, and submits through `common.RoomSnapshotSubmitter`.
- A valid command emits no command-level raw action or generic reply. The request asks the coordinator to publish `Unable to complete room operation.` on workflow submission/lifecycle failure; the operation reports `Failed to nuke <target>` on ban/lock failure.
- `NukeRoomOperation` serially marshals and sends each normalized ban, waits 200 ms after each successful ban, then sends `{cmd:"lockroom"}` only if every ban succeeded. A failed ban terminates the operation without locking.
- Legacy limit: `legacyAdapter.Execute` in `internal/command/dispatch_adapter.go` intentionally invokes commands with `context.Background()` because the legacy inbound `Command` interface accepts neither context nor error. The concrete command rejects a pre-cancelled context when directly invoked, but dispatch cannot propagate inbound cancellation through that legacy adapter. The existing coordinator API itself has no request context.

## Strict TDD evidence

Initial command tracer was written before nuke production code.

```sh
go test ./internal/command -run '^TestNukeCommandSubmitsNormalizedRemoteSnapshotWithoutImmediateReply$' -count=1
# RED
--- FAIL: TestNukeCommandSubmitsNormalizedRemoteSnapshotWithoutImmediateReply (0.01s)
    nuke_test.go:39: command emitted immediate output: chats=[moderator| nuke accepted|true] raws=[]
FAIL
FAIL    zenbot/internal/command    0.515s
FAIL
```

```sh
go test ./internal/command -run '^TestNukeCommandSubmitsNormalizedRemoteSnapshotWithoutImmediateReply$' -count=1
# GREEN
ok      zenbot/internal/command    0.532s
```

Operation tracer was then written before `NukeRoomOperation`.

```sh
go test ./internal/listener/snapshot -run '^TestNukeRoomOperationBansNormalizedSnapshotUsersBeforeLockingRoom$' -count=1
# RED
# zenbot/internal/listener/snapshot [zenbot/internal/listener/snapshot.test]
internal/listener/snapshot/nuke_operation_test.go:12:15: undefined: NewNukeRoomOperation
FAIL    zenbot/internal/listener/snapshot [build failed]
FAIL
```

```sh
go test ./internal/command ./internal/listener/snapshot -run '^(TestNukeCommandSubmitsNormalizedRemoteSnapshotWithoutImmediateReply|TestNukeRoomOperationBansNormalizedSnapshotUsersBeforeLockingRoom)$' -count=1
# GREEN
ok      zenbot/internal/command    0.522s
ok      zenbot/internal/listener/snapshot    0.831s
```

Registration tracer:

```sh
go test ./internal/command -run '^TestNukeRegistrationRequiresRoomSnapshotSubmitter$' -count=1
# RED
--- FAIL: TestNukeRegistrationRequiresRoomSnapshotSubmitter (0.27s)
    nuke_test.go:36: nuke was not registered with room snapshot submitter
FAIL
FAIL    zenbot/internal/command    0.686s
FAIL

# GREEN
ok      zenbot/internal/command    0.700s
```

Failure-reply request tracer:

```sh
go test ./internal/command -run '^TestNukeCommandRequestsSourceShapedFailureReply$' -count=1
# RED
--- FAIL: TestNukeCommandRequestsSourceShapedFailureReply (0.01s)
    nuke_test.go:54: failure request=[{WorkflowID:nuke-13083769d85629c7033492ba9173548b Author:moderator Whisper:false SourceChannel:source-room TargetChannel:hotlinks DestinationChannel: ReplyMessage: Operation:{delay:200000000}}]
FAIL
FAIL    zenbot/internal/command    0.505s
FAIL

# GREEN
ok      zenbot/internal/command    0.439s
```

## Tests added/updated

- `internal/command/nuke_test.go`: valid request/no immediate reply, registration and execution capability gates, exact missing-usage reply, normalization, source/author/whisper fields, submission failure request shape, and direct pre-cancelled context behavior.
- `internal/listener/snapshot/nuke_operation_test.go`: decoded raw payload shapes and order, nick normalization, and no lock after a failed ban.
- `internal/command/admin_moderator_catalog_guard_test.go`: removes `nuke` from the generic-fallback allowlist after concrete replacement.

## Touched files

- `internal/command/nuke.go` (new)
- `internal/command/nuke_test.go` (new)
- `internal/listener/snapshot/nuke_operation.go` (new)
- `internal/listener/snapshot/nuke_operation_test.go` (new)
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `.hermes/handoffs/nuke-implementation.md` (this handoff)

## Verification

```sh
go test ./internal/command ./internal/listener/snapshot -count=1
# PASS: command 8.553s; listener/snapshot 0.328s

go test ./... -count=1
# PASS (including internal/repository/h2 36.296s)

go vet ./...
# PASS (no output)

go build ./...
# PASS (no output)

git diff --check
# PASS (no output)
```

## Trade-offs

The target protocol lock action has no channel field because the coordinator's temporary session is connected to `TargetChannel`; this follows Saturn's `lockroom` payload and Zenbot's existing typed moderation convention. The source's operation catches a ban failure and continues to lock, but the accepted composition boundary explicitly requires a lock only after every ban succeeds, so this implementation stops on the first failure and returns the source-shaped failure result.
