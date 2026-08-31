# Shared Integration Runtime-Failure Implementation Handoff

## Outcome

Closed the documented replica lifecycle coverage gap with a dedicated real local Gorilla WebSocket regression test. The test passes, including under the race detector.

The test exposed one production defect: a managed replica runtime transport failure was delivered twice to the host-owned sink (once by `EngineImpl.LifecycleErrors` and once by the replica runtime callback). The minimal shared-integration fix makes the runtime callback the sole reporting path when configured; engines without a runtime callback continue to report through `LifecycleErrors`. Intentional shutdown remains silent because `context.Canceled` and `transport.ErrClosed` are suppressed and the test verifies no cancellation/close noise is delivered.

## Files changed

- `internal/command/real_websocket_replica_failure_test.go`
  - Starts a local `httptest` Gorilla WebSocket server.
  - Starts a real MASTER through `factory.NewEngineWithOptions`.
  - Creates a real REPLICA through `factory.ReplicaFactory` and `core.NewManagedReplicaController`.
  - Confirms the replica is manager-visible before failure.
  - Forces the replica server-side WebSocket close/read failure.
  - Asserts exactly one host-owned sink error with `replica-room` runtime context.
  - Asserts deterministic `ReplicaManager` removal, replica handler exit, both real WebSocket connections, no cancellation/close noise, and bounded goroutine cleanup.
- `internal/core/engine_impl.go`
  - Production fix only: when `runtimeFailure` is configured, `reportTransportError` calls that callback instead of also publishing the same failure to `LifecycleErrors`.
  - No unrelated lifecycle, DBZ, identity, ZOMBIE, permanent-listener, or bounded remote-room/Whiskey behavior was changed.

## Verification actually run

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

- `gofmt -w internal/core/engine_impl.go internal/command/real_websocket_replica_failure_test.go` — exit 0.
- RED proof before the production fix: `go test -count=1 ./internal/command -run TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry -v` — exit 1 as expected; the test observed 2 sink errors (engine transport + replica runtime).
- Focused regression: `go test -count=1 ./internal/command -run TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry -v` — exit 0; PASS.
- Focused race regression: `go test -race -count=1 ./internal/command -run TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry -v` — exit 0; PASS; no race reports.
- Focused packages: `go test -count=1 ./internal/listener ./internal/listener/snapshot ./internal/core ./internal/factory ./internal/command` — exit 0; all five packages passed.
- `git diff --check` — exit 0; no whitespace errors.
- Stability check: `go test -count=10 ./internal/command -run TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry` — exit 0; all 10 runs passed.

The gap is claimed closed only because the dedicated real WebSocket test passes in normal and race runs.
