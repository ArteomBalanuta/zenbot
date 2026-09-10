# Shared Integration Repair Implementation Handoff

## Outcome

Implemented the minimal boundary repairs in the existing Zenbot abstractions. No Saturn files were modified. Existing unrelated dirty/untracked files were preserved.

## Files changed by this repair

- `internal/listener/online_set_listener.go`: added an isolated `SnapshotOnlineSetListener` that validates `users`/`Users` payloads and forwards the original payload only to its sink; it never calls active-user or permanent listener APIs.
- `internal/listener/online_set_listener_test.go`: regression test for valid temporary online-set forwarding.
- `internal/listener/snapshot/session_factory.go`: added a real transport-backed temporary session adapter, collision-resistant IDs, exact session identity, bounded raw sends, idempotent close/registry cleanup, and one-shot transport error forwarding. Kept the existing injected `New` constructor compatible.
- `internal/listener/snapshot/coordinator.go`: automatically binds coordinator callbacks when the factory supports the binding seam.
- `internal/listener/snapshot/transport_session_test.go`: transport-session sink routing, one-shot error forwarding, and cleanup test.
- `internal/factory/engine_factory.go`: added `SnapshotCoordinator`/`SessionRegistry` options, temporary listener selection, lifecycle error injection, and `NewCoordinatedSessionFactory` wiring to the existing transport configuration.
- `internal/core/engine_impl.go`: wired managed transport start/read failures to an optional lifecycle sink with channel context, one-shot reporting, cancellation-noise suppression, runtime cancellation, and a runtime failure callback.
- `internal/core/replica_controller.go`: added optional host-owned error reporting for construction/start/registration/runtime failures and deterministic runtime manager removal; preserved start-before-visibility and rollback behavior.
- `internal/command/dispatch_adapter.go`: live-registers `replica`, `replicaoff`, and `replicastatus` (including catalog aliases) when the engine exposes the replica-controller capability.
- `cmd/zenbot/main.go`: passes one host-owned transport error sink to MASTER and replicas and drains it alongside lifecycle errors.

## Compatibility decisions

- Legacy `common.Engine`, `NewEngine`, legacy connection, queue serialization, DBZ/identity wiring, and permanent MASTER/REPLICA listeners were not redesigned.
- Temporary sessions stay outside `ReplicaManager`; only `onlineSet` is accepted and forwarded.
- Remote-room delivery and Whiskey remain explicit bounded failures (`remote room delivery is not configured` and `whiskey proxy configuration is unavailable`).
- Lifecycle errors are non-blocking and contextualized; intentional context cancellation and `transport.ErrClosed` are suppressed.
- Replica command authorization still occurs through the existing inbound dispatch path before controller side effects.

## Verification actually run

From `/Users/ab/workspace/go-projects/zenbot`:

- `gofmt -w` on all repair files — exit 0.
- `go test -count=1 ./...` — exit 0; all packages passed.
- `go test -race ./...` — exit 0; all packages passed, no race reports.
- `go vet ./...` — exit 0; no output.
- `go build ./...` — exit 0; no output.
- `git diff --check` — exit 0; no output.
- Focused repair packages `./internal/listener ./internal/listener/snapshot ./internal/core ./internal/factory ./internal/command` — exit 0.
- Focused DBZ/identity/replica/dispatch/catalog run `go test -count=1 ./internal/repository/h2 ./internal/service ./internal/command -run 'DBZ|Identity|Replica|Dispatch|Catalog'` — exit 0.

## Remaining gaps

- The newly added temporary transport test uses the transport abstraction directly rather than a second live `httptest` WebSocket coordinator scenario; the existing real WebSocket coverage remains for managed MASTER/REPLICA joins.
- No new end-to-end inbound WebSocket `chat -> authorization -> controller -> replica join -> manager visibility` test was added in this repair pass; live registration is wired and existing command/replica tests remain green, but that requested proof is still a test gap.
- `Lifecycle.Errors()` and the engine option sink are separate channels because the current lifecycle is constructed after the factory engine in `main`; both are drained by the host. A future API could unify them without changing engine behavior.
- Transport channels intentionally remain open on close per the existing transport contract; session loops stop via context/terminal error rather than channel ownership changes.
