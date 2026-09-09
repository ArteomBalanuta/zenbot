# Lifecycle completion implementation record

**Status: incomplete — strict tracer sequence was not completed.**

## Tracer 1: master binding / snapshot reply

### RED

```text
go test ./cmd/zenbot -run '^TestMasterBindingSnapshotReplyUsesReboundMaster$' -count=1
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/room_snapshot_composition_test.go:17:13: undefined: newMasterBinding
cmd/zenbot/room_snapshot_composition_test.go:18:32: undefined: masterReplySender
FAIL    zenbot/cmd/zenbot [build failed]
FAIL
```

### GREEN

Added `cmd/zenbot/master_binding.go` with synchronized `masterBinding` and a resolver-backed snapshot sender. The focused architecture command passed:

```text
gofmt -w cmd/zenbot/master_binding.go cmd/zenbot/room_snapshot_composition_test.go && go test ./cmd/zenbot -run 'Test.*MasterBinding|Test.*Snapshot.*Rebind' -count=1
ok      zenbot/cmd/zenbot    0.483s
```

## Tracer 2: persistent directory rebinding

### RED

```text
go test ./internal/core -run '^TestEngineRoomUserDirectoryRebindsHostWithoutReplacingPersistentDirectory$' -count=1
# zenbot/internal/core [zenbot/internal/core.test]
internal/core/room_directory_test.go:22:15: undefined: NewEngineRoomUserDirectory
FAIL    zenbot/internal/core [build failed]
FAIL
```

### GREEN

Added a synchronized directory host binding, `NewEngineRoomUserDirectory`, and `RebindHost`, retaining the existing replica manager. Focused race verification:

```text
gofmt -w internal/core/room_directory.go internal/core/room_directory_test.go && go test -race ./internal/core -run 'TestEngineRoomUserDirectory(RebindsHostWithoutReplacingPersistentDirectory|FindsManagedHostAndReplicaSnapshots|LookupRacesSafelyWithReplicaChanges)' -count=1
ok      zenbot/internal/core  1.490s
```

## Tracer 3: common initial/fresh graph path

### RED

```text
go test ./cmd/zenbot -run '^TestHostSupervisorInitialStartBuildsStartsAndPublishes$' -count=1
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/host_supervisor_test.go:18:14: s.StartInitial undefined (type *hostSupervisor has no field or method StartInitial)
FAIL    zenbot/cmd/zenbot [build failed]
FAIL
```

### GREEN

Added `hostSupervisor.StartInitial`, which executes build → start → publish/rebind without retiring a bootstrap master:

```text
gofmt -w cmd/zenbot/host_supervisor.go cmd/zenbot/host_supervisor_test.go internal/command/agent_gateway.go cmd/zenbot/main.go && go test ./cmd/zenbot -run '^TestHostSupervisorInitialStartBuildsStartsAndPublishes$' -count=1
ok      zenbot/cmd/zenbot    0.474s
```

## Production changes made before a valid remaining tracer sequence

`cmd/zenbot/main.go` now composes a `masterBinding`, persistent directory/replica manager/agent, extracted local `buildMasterGraph`, real supervisor retire/build/start/rebind callbacks, and `StartInitial`; it removes the competing fixed `core.Lifecycle` owner. Command shutdown remains supervisor host-only; signal teardown retains agent close + current host stop + replica stop ordering under `SignalTeardown`.

`newLiveAgent` now resolves master-facing output, failure delivery, trusted snapshots, and command-gateway requests through the current-master resolver. `internal/command.NewResolvingAgentCommandGateway` preserves the existing public allowlist by delegating to the existing gateway per request.

This production expansion is **not** accepted as tracers 3–7 evidence: no focused RED→GREEN production graph, callback composition, exclusive-owner, shutdown/signal, or end-to-end lifecycle parity tracer was completed/recorded. Final acceptance gates were deliberately not run because the binding architecture requires them only after all seven valid tracer cycles.

## Last focused compilation/regression check

```text
gofmt -w cmd/zenbot/main.go cmd/zenbot/master_binding.go cmd/zenbot/host_supervisor.go internal/core/room_directory.go internal/command/agent_gateway.go && go test ./cmd/zenbot ./internal/core ./internal/command -run 'Test(MasterBinding|HostSupervisor|RoomSnapshot|EngineRoomUserDirectory|NewLiveAgent)' -count=1
ok      zenbot/cmd/zenbot    0.593s
ok      zenbot/internal/core 0.682s
ok      zenbot/internal/command 0.570s [no tests to run]
```

## Compilation and diff verification

```text
go test -run '^$' ./...
# exit 0; all packages compiled successfully with tests skipped

git diff --check
# exit 0
```

## Files changed

- `cmd/zenbot/main.go`
- `cmd/zenbot/master_binding.go` (new)
- `cmd/zenbot/room_snapshot_composition_test.go`
- `cmd/zenbot/host_supervisor.go`
- `cmd/zenbot/host_supervisor_test.go`
- `internal/core/room_directory.go`
- `internal/core/room_directory_test.go`
- `internal/command/agent_gateway.go`
- this handoff

No reset, restore, clean, stage, or commit was performed. No real lifecycle, signal, process exit, database, H2, replica, or agent shutdown action was executed.

## Resume attempt: stop condition at lifecycle-completion tracer 4

The required tracer-4 target already exists in the dirty worktree: its
production composition (`main`'s non-nil retire/build/start/rebind callbacks)
and the focused callback-order test were present before this attempt. Running
the required focused target therefore produced an immediate green result:

```text
go test ./cmd/zenbot -run '^TestMainProductionHostLifecycleWiresSupervisorCallbacks$' -count=1
ok      zenbot/cmd/zenbot    0.356s
```

This is a source-target collision, not a witnessed tracer-4 RED→GREEN cycle.
The existing test currently proves fake supervisor callback order and dispatch
deferral; it is not independent strict-TDD evidence for the already-installed
production composition. Per the lifecycle-completion architecture's explicit
stop rule, no tracer-4 test/source rewrite, no tracer-5 through tracer-7 work,
and no final full/race/vet/build gates were run. No Go source/test was changed
in this resume attempt; only this record and the primary parity implementation
handoff were updated. A future owner must explicitly authorize either a clean
rebaseline of the pre-existing lifecycle-composition artifacts or their
dirty-worktree-safe manual reduction before a new RED-capable tracer sequence.
