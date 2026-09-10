# Explicit moderator resurrect remote-snapshot implementation

## Scope

Implemented only the explicit three-argument moderator path: `resurrect <nick> <from> <to>` and the reviewed aliases `move`, `recover`, `heal`, `resurrect`. Zero-argument last-moved behavior, storage, automove/mine/agent paths, coordinator changes, and general remote-command infrastructure remain excluded.

## Source and architecture citations

- Saturn command authority: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ResurrectUserCommandImpl.java`, especially aliases/role (19–29), explicit arity/target normalization (32–50), and replica-then-host live selection/fallback (64–94).
- Saturn snapshot authority: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/listener/snapshot/KickOrResurrectOperation.java` (7–23): normalized identity match, exact absent message, and `kick/nick/to` raw payload.
- Accepted Zenbot seams retained: `internal/common/room_snapshot.go`, `internal/core/room_snapshot.go`, `internal/listener/snapshot/coordinator.go`, `internal/command/nuke.go`, and `internal/listener/snapshot/nuke_operation.go`.

## Implemented behavior

- `common.LiveRoomMover` is a narrow live-source capability.
- `(*core.EngineImpl).MoveFromServingRoom` rejects a cancelled direct context, selects an exact managed replica first and host second, and uses the selected engine's typed `ModerationOperations.KickNickTo`. A handled live error is returned without snapshot fallback.
- `resurrectCommand` requires exactly three arguments, uses the source-shaped exact usage reply ` <prefix>move <nick> <from> <to>`, normalizes only the target, and checks live capability before snapshot capability.
- No live source submits one `resurrect-` workflow using `SourceChannel` and `TargetChannel` equal to `<from>`, destination `<to>`, trusted author/whisper metadata, generic failure reply, and `KickOrResurrectOperation`.
- `KickOrResurrectOperation` owns temporary-session raw JSON. It uses `SameNick`, skips nil/noncanonical users, returns exact ` <nick> isn't in the room` with no send when absent, and reports a non-nil operation error for unavailable/failing temporary sender.
- Registration adds only canonical `resurrect`, thus exactly its reviewed aliases, only when both `LiveRoomMover` and `RoomSnapshotSubmitter` are available. The generic-fallback guard was updated to make resurrect concrete.

## Strict TDD evidence

First required tracer was added before production code.

```text
go test ./internal/command -run '^TestResurrectCommandFallsBackToSnapshotOnlyWhenNoLiveSourceServesFrom$' -count=1
# RED
--- FAIL: TestResurrectCommandFallsBackToSnapshotOnlyWhenNoLiveSourceServesFrom (0.01s)
    resurrect_test.go:52: live moves=[]
FAIL
FAIL    zenbot/internal/command    0.515s
FAIL

# GREEN after the bounded command/capability/operation implementation
go test ./internal/command -run '^TestResurrectCommandFallsBackToSnapshotOnlyWhenNoLiveSourceServesFrom$' -count=1
ok      zenbot/internal/command    0.520s
```

Focused coverage then passed for fallback request metadata/no immediate output, both-capability registration and moderator aliases, live replica/host source actions and cancellation, snapshot exact payload/presence/absence/sender failure, and master composition capabilities.

```text
go test ./internal/command ./internal/listener/snapshot ./internal/core -count=1
ok      zenbot/internal/command             8.812s
ok      zenbot/internal/listener/snapshot   0.730s
ok      zenbot/internal/core                0.747s

go test ./cmd/zenbot -run '^TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster$' -count=1
ok      zenbot/cmd/zenbot 0.393s
```

## Files changed

- `internal/common/resurrect.go` (new)
- `internal/core/resurrect.go` and `internal/core/resurrect_test.go` (new)
- `internal/command/resurrect.go` and `internal/command/resurrect_test.go` (new)
- `internal/listener/snapshot/kick_or_resurrect_operation.go` and `_test.go` (new)
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `cmd/zenbot/room_snapshot_composition_test.go`
- `.hermes/handoffs/resurrect-implementation.md`

## Final verification

```text
go test ./... -count=1    PASS (including internal/repository/h2 36.310s)
go vet ./...               PASS
go build ./...             PASS
git diff --check           PASS
```

## Known limitations

The legacy inbound adapter still invokes commands with `context.Background()`, and the snapshot coordinator has no request context/workflow bridge. Therefore only direct pre-submit cancellation is supported; post-submit caller cancellation is intentionally unchanged.
