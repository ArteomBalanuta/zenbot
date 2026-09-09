# Room-snapshot production composition implementation

## Scope

Implemented only the remaining production composition for the accepted room-snapshot prerequisite. No command catalog, dispatch, nuke/resurrect/mine/automove, agent, or replica-management behavior was added or changed.

## TDD record

### RED

Added `cmd/zenbot/room_snapshot_composition_test.go` with `TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster` before adding the helper.

Command:

```sh
go test ./cmd/zenbot -run '^TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster$' -count=1
```

Output:

```text
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/room_snapshot_composition_test.go:14:10: undefined: newRoomSnapshotEngineOptions
FAIL	zenbot/cmd/zenbot [build failed]
FAIL
```

The failure was the expected missing helper.

### GREEN

Added `newRoomSnapshotEngineOptions`, which creates a `TemporarySessionRegistry`, creates `factory.NewCoordinatedSessionFactory` with the master transport configuration, creates/binds a `RoomSnapshotCoordinator`, and returns both coordinator and registry through `factory.EngineOptions`.

Command:

```sh
go test ./cmd/zenbot -run '^TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster$' -count=1
```

Output:

```text
ok  	zenbot/cmd/zenbot	0.465s
```

After binding the returned options into production `main`, the same test was rerun:

```text
ok  	zenbot/cmd/zenbot	0.397s
```

The focused aggregate initially exposed a nondeterministic assertion against `RoomSnapshotRequest.validate`, which iterates a map. The test was corrected to use complete identity fields and assert the deterministic `operation cannot be nil` coordinator validation. This did not change production behavior.

Final exact GREEN and focused output:

```text
ok  	zenbot/cmd/zenbot	0.392s
ok  	zenbot/cmd/zenbot	0.668s
ok  	zenbot/internal/listener/snapshot	0.341s
ok  	zenbot/internal/core	0.557s
ok  	zenbot/internal/factory	0.861s
```

## Production binding

`main` now builds snapshot options for the master only, passes them to `factory.NewEngineWithOptions`, and installs a reply sink that delivers coordinator replies to the request author through the master engine. Replica `EngineOptions` are unchanged, so temporary sessions remain owned by `CoordinatedSessionFactory` and are not replica-managed.

## Verification

- `go test ./cmd/zenbot ./internal/listener/snapshot ./internal/core ./internal/factory -count=1` — PASS.
- `go test ./... -count=1` — PASS, including `internal/repository/h2` (36.337s).
- `go vet ./...` — PASS (no output).
- `go build ./...` — PASS (no output).
- `git diff --check` — PASS (no output).

## Touched paths

- `cmd/zenbot/main.go`
- `cmd/zenbot/room_snapshot_composition_test.go`
- `.hermes/handoffs/nuke-composition-implementation.md`

## Limitation

`snapshot.RoomSnapshotRequest` currently carries the author but no trusted inbound whisper-mode field. The production reply sink therefore uses whisper delivery (`true`) for snapshot replies. Exact preservation of a future request's inbound whisper/public mode requires a later, explicitly scoped request-delivery field and command tracer; this prerequisite intentionally does not change that API or activate any command.
