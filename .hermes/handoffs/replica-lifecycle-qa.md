# Replica lifecycle / transport QA handoff

## Verdict

QA PASS for the exercised task-owned transport, lifecycle, replica-manager, command-boundary, and temporary-session slice. This is a focused QA pass, not a claim that the full architecture checklist is complete: the shared integration seams and several protocol-level gaps remain explicitly listed below. The PASS is backed by the artifact and fresh command output in this handoff.

## Handoff and source inspection

Read and reviewed:

- `.hermes/handoffs/replica-lifecycle-implementation.md`
- `.hermes/handoffs/replica-lifecycle-architecture.md`
- `.hermes/handoffs/dbz-qa.md`
- `internal/transport/connection.go`
- `internal/transport/connection_test.go`
- `internal/core/lifecycle.go`
- `internal/core/lifecycle_test.go`
- `internal/core/replica_manager.go`
- `internal/core/replica_manager_test.go`
- `internal/command/replica.go`
- `internal/command/replica_test.go`
- `internal/listener/snapshot/session_factory.go`
- `internal/listener/snapshot/session_factory_test.go`

## QA changes and defects fixed

Exact files modified by this QA pass:

- `internal/transport/connection.go`
- `internal/transport/connection_test.go`
- `internal/core/lifecycle.go`
- `internal/core/lifecycle_test.go`
- `internal/core/replica_manager.go`
- `internal/core/replica_manager_test.go`
- `.hermes/handoffs/replica-lifecycle-qa.md`

Defects fixed:

1. Transport `Start` could dial and launch reader/ping goroutines after `Close` had already completed. `Start` now rejects a closed connection, and the post-dial path closes a socket if concurrent close wins the race. Regression coverage is in `TestConnectionCloseBeforeStartPreventsDial`.
2. `ReplicaManager.Add` accepted whitespace-only channels. It now trims and rejects blank channels.
3. `ReplicaManager.StopAll` did not establish a terminal ownership barrier; a concurrent or subsequent `Add` could register a replica after shutdown. The manager now marks itself stopped while taking the stop snapshot and rejects later adds. Regression coverage is in `TestReplicaManagerRejectsBlankChannelsAndStopsNewAdds`.
4. `Lifecycle.Stop(nil)` could panic through `context.WithTimeout`. Nil stop contexts now mean `context.Background()`.

No DBZ or identity production/test files were modified by this QA pass.

## Behavior covered / verified

- Real local Gorilla WebSocket handshake and round trip.
- Concurrent text writes through the transport write mutex.
- Inbound message delivery, dial failure/cancellation, idempotent close, and no nil-WebSocket dereference on close-before-start.
- Lifecycle asynchronous start, health monitoring, stop, restart, bounded retry structure, cancellation-aware waits, and idempotent stop behavior as exercised by the existing lifecycle tests and source inspection.
- Replica host-channel rejection, duplicate rejection, nil/blank validation, copied registry snapshots, remove/stop ownership, sorted channel status, concurrent-safe locking, and terminal stop barrier.
- Temporary snapshot session IDs/context cancellation, registry isolation, close cleanup, and duplicate-close rejection. The broader coordinator package tests also passed.
- Replica/msgchannel parsing, `?` normalization, status sorting, and bounded ordered Whiskey proxy selection/backup order.

## Actual verification results

All commands below were run in `/Users/ab/workspace/go-projects/zenbot` after formatting intentional lifecycle files.

```text
gofmt -w internal/transport/connection.go internal/transport/connection_test.go internal/core/lifecycle.go internal/core/replica_manager.go internal/core/replica_manager_test.go
```
Completed successfully. `gofmt -l` over all claimed lifecycle/transport/replica/command/snapshot implementation and test files returned no output.

```text
go test -count=1 ./internal/transport ./internal/core ./internal/command ./internal/listener/snapshot
```
PASS:
```text
ok  zenbot/internal/transport       0.972s
ok  zenbot/internal/core             0.627s
ok  zenbot/internal/command          3.025s
ok  zenbot/internal/listener/snapshot 1.132s
```

```text
go test -count=1 ./...
```
PASS. All listed packages passed, including command, core, listener/snapshot, repository/h2, service, and transport; packages without tests reported `[no test files]`.

```text
go test -race ./...
```
PASS (exit 0). All packages passed; no race reports.

```text
go vet ./...
```
PASS (exit 0, no output).

```text
go build ./...
```
PASS (exit 0, no output).

```text
go test -count=1 ./internal/command ./internal/repository/h2 ./internal/service
```
PASS:
```text
ok  zenbot/internal/command       1.780s
ok  zenbot/internal/repository/h2  10.623s
ok  zenbot/internal/service        0.743s
```

```text
git diff --check
```
PASS (exit 0, no output).

`git status --short` confirms the intentionally dirty pre-existing migration/DBZ/identity worktree remains present. The lifecycle files listed above are the only non-handoff files changed by this QA pass.

## Remaining gaps / limitations

- The implementation handoff itself documents that lifecycle errors from background retry are internal and not exposed through an owner error/event channel.
- `Lifecycle.Start` is asynchronous and reports factory/start failures through its background run rather than synchronously; no public terminal error is available.
- Transport event channels remain open after close by design; callers must use `Connected`/context ownership. There is no public transport `done` channel or wait method.
- Existing transport tests do not exhaustively assert server-side close/error event semantics, write-failure propagation after remote close, ping/pong behavior, or deadline expiry under a blocked peer.
- Replica stop iteration is map-order based; manager-level stop ownership is safe, but deterministic replica shutdown order is not specified.
- Temporary session registry supplies isolated IDs and cancellation but does not itself implement correlated event routing; integration into the existing snapshot coordinator/session factory remains a shared integration task.
- Command helpers are narrow boundaries, not full concrete replica construction/registry wiring; exact Saturn response integration and aliases in the live command registry remain for the coordinated shared-file phase.
- No shared files were changed, so full engine/factory/main integration, real MASTER/REPLICA WebSocket factory tests, raw/chat payload golden tests, and host listener-profile integration remain open.

## Preservation confirmation

DBZ behavior and identity behavior were preserved. No DBZ-owned or identity-owned files were edited by this QA pass. The DBZ-focused command, H2, and service tests passed. No Saturn files or `/Users/ab/workspace/projects/saturn` content was touched. Unrelated dirty and untracked worktree changes were not reset, stashed, rewritten, or deleted.
