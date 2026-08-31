# Replica lifecycle implementation handoff

## Outcome
Implemented the bounded, isolated transport/lifecycle slice as new code. Existing DBZ and migration work was preserved; no shared DBZ-owned files were edited.

## Exact files added

- `internal/transport/connection.go`
  - Gorilla WebSocket adapter using `DialContext`.
  - Serialized writes with a mutex and write deadlines.
  - Explicit dial/send/ping/read errors through an error channel.
  - Idempotent close via `sync.Once`.
  - Reader cancellation by closing the socket and ping cancellation via a ticker/select.
  - Buffered message/error event channels.
- `internal/transport/connection_test.go`
  - Local `httptest` Gorilla server round trip, concurrent writes, idempotent close, and dial failure/cancellation.
- `internal/core/lifecycle.go`
  - Context-owned asynchronous lifecycle owner.
  - Idempotent stop behavior, restart, health ticker, bounded retry count, retry interval, stop deadline, and cancellation-aware waits.
- `internal/core/lifecycle_test.go`
  - Start/stop/restart ownership and bounded lifecycle test.
- `internal/core/replica_manager.go`
  - Host-owned concurrency-safe add/remove/stop-all registry.
  - Rejects host channel, nil/blank replicas, and duplicates.
  - Returns copied maps and sorted channel snapshots.
- `internal/core/replica_manager_test.go`
  - Host/duplicate rejection, copied-map isolation, and stop ownership.
- `internal/command/replica.go`
  - Narrow command boundary helpers for replica channel parsing/removal/status formatting, msgchannel parsing, and bounded ordered Whiskey proxy selection.
  - Preserves target spellings and aliases at the existing registry boundary; no generic registry rewrite was performed.
- `internal/command/replica_test.go`
  - Replica/status/msgchannel parsing and proxy order tests.
- `internal/listener/snapshot/session_factory.go`
  - Temporary session registry with per-session cancellation and independent IDs; sessions are not inserted into host replicas.
- `internal/listener/snapshot/session_factory_test.go`
  - Open/close/cancel/no-leak correlation test.

## Verification

Ran after `gofmt`:

```text
go test -count=1 ./...   PASS
go test -race ./...      PASS
go vet ./...             PASS
go build ./...           PASS
git diff --check         PASS
```

The full test and race outputs reported all existing packages passing, including `internal/repository/h2`, `internal/service` (DBZ-related tests), `internal/command`, `internal/core`, `internal/listener/snapshot`, and the new `internal/transport` package.

## Shared-file preservation

The pre-existing dirty/untracked worktree was not reset, stashed, rewritten, or deleted. No edits were made to the architecture-listed shared files:

- `internal/core/engine_impl.go`
- `internal/common/engine.go`
- `internal/factory/engine_factory.go`
- `internal/command/registry.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/handlers.go`
- `internal/service/services.go`
- `cmd/zenbot/main.go`
- `internal/repository/repository.go`

`git status` still shows those paths in their pre-existing modified/untracked state, plus only the intentional files listed above.

## Explicit source-vs-target adaptations and remaining gaps

- Proxy retry is bounded by `MaxAttempts` and context cancellation rather than Saturn's unbounded recursive backup retry. Configured proxy order and remaining-backup ordering are retained.
- No Saturn trust-all TLS behavior was copied. The transport uses Gorilla's configured dialer and therefore normal certificate validation; callers can explicitly provide a configured dialer if policy permits.
- This phase intentionally does not retrofit `EngineImpl`/`Connection`, factory construction, `main`, or the existing command registry because those are DBZ-shared files requiring serialized integration.
- The command file provides narrow parsing/controller boundaries, not full replica construction or output registration. Integration with the concrete engine factory and exact Saturn response strings remains for the coordinated shared-file phase.
- The temporary snapshot registry is an isolation primitive; wiring it into the existing coordinator's `SessionFactory` remains for the shared integration phase.
- Lifecycle errors from a background retry loop are currently internal to the owner; a future integration should expose an owner error/event channel to the host runner.
- No external services were used.
