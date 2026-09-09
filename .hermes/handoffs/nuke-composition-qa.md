# Room-snapshot composition QA

## Verdict

**PASS — prerequisite is hardened and the nuke command slice may proceed.**

The audit found and corrected two material gaps before approval:

1. The reply sink forced all coordinator replies to whisper delivery. `RoomSnapshotRequest` now carries the trusted command-delivery `Whisper` flag, and the master reply sink forwards that exact value.
2. `go test -race` exposed a race between workflow timer assignment and terminal cleanup. Timer access is now synchronized through `workflow.stopTimer`.

No nuke handler, operation, registration, alias, generic command-routing, agent/public-tool, replica-option, or replica-lifecycle behavior was added.

## Composition audit

- `internal/common.RoomSnapshotSubmitter` is a narrow request-submission capability. `go list -deps ./internal/common` confirmed it has no dependency on `zenbot/internal/core`.
- `core.EngineImpl` retains only a private coordinator pointer and forwards requests unchanged; it creates no session, transport, protocol payload, or snapshot parsing path.
- The master-only helper creates the temporary registry, coordinated session factory, and coordinator. The coordinator constructor binds the factory callbacks; session registration, routing, closure, and registry removal remain in `snapshot.CoordinatedSessionFactory`.
- Replica construction remains unchanged: no snapshot options are passed to replicas and no temporary session is registered with the replica manager.
- Reply routing explicitly targets the request author and preserves the request's trusted `Whisper` mode.
- Search found no `SubmitRoomSnapshot`, `NewNuke`, or `NukeRoom` implementation in `internal/command`; existing static catalog/help references were pre-existing and no command behavior was activated.

## TDD / hardening evidence

- Added `TestRoomSnapshotReplySinkPreservesRequestWhisperMode` before implementation. It initially failed to compile because `RoomSnapshotRequest.Whisper` and `roomSnapshotReplySink` did not exist; after the minimal production change it passed.
- Initial focused race run failed with a real race in `snapshot.(*workflow).fail` reading `w.timer` concurrently with `RoomSnapshotCoordinator.Submit` assigning it. The synchronization fix was verified by the final race run below.

## Final verification

All commands ran from `/Users/ab/workspace/go-projects/zenbot` and passed:

```sh
go test ./internal/listener/snapshot ./internal/core ./internal/factory ./cmd/zenbot -count=1
# PASS

go test ./... -count=1
# PASS (including internal/repository/h2 in 39.069s)

go vet ./...
# PASS (no output)

go build ./...
# PASS (no output)

git diff --check
# PASS (no output)

go test -race ./internal/listener/snapshot ./internal/core ./internal/factory ./cmd/zenbot -count=1
# PASS

go test -cover ./internal/core ./internal/factory ./cmd/zenbot ./internal/listener/snapshot -count=1
# PASS: core 51.5%, factory 56.4%, cmd/zenbot 33.7%, listener/snapshot 78.5%
```

## Touched files

Prerequisite implementation already present and audited:

- `internal/common/room_snapshot.go`
- `internal/core/room_snapshot.go`
- `internal/core/room_snapshot_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/room_snapshot_test.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/room_snapshot_composition_test.go`

QA hardening changes:

- `internal/listener/snapshot/coordinator.go` — delivery-mode request field and race-free timer cleanup.
- `cmd/zenbot/main.go` — explicit reply-sink adapter preserving delivery mode.
- `cmd/zenbot/room_snapshot_composition_test.go` — exact whisper/public routing test.
- `.hermes/handoffs/nuke-composition-qa.md` — this report.

## Limitations

- No nuke command exists in this slice. Its future command tracer must populate `RoomSnapshotRequest.Whisper` from the validated inbound message context; the composed sink now preserves it exactly.
- This QA validates composition, lifecycle seams, and race safety with test transports. It does not make a live websocket connection or execute moderation protocol actions.
- The repository had substantial unrelated dirty changes. None were reset, cleaned, staged, committed, or intentionally modified.
