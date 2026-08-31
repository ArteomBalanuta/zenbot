# Shared Integration Implementation Handoff

## Result

Implemented the shared integration slice without modifying Saturn or migration files. The current repository passes the requested full verification matrix.

## Intentional files changed/created

- `internal/core/engine_impl.go`
  - Added `EngineTransport`, `ManagedEngine`, and listener profile types.
  - Added bounded `StartContext`/`StopContext`, health and engine-type accessors.
  - Managed startup uses `internal/transport.Connection`, sends one join payload, owns the event loop, and avoids closing the legacy outbound queue.
  - Managed outbound chat/raw calls use bounded transport writes; legacy callers still use the existing queue path.
  - Added controller delegation while retaining all legacy `common.Engine` methods and exact payload formatting.
- `internal/core/lifecycle.go`
  - Added buffered observable `Errors()` and reports terminal asynchronous lifecycle failures instead of discarding them.
- `internal/core/replica_controller.go`
  - Added host-owned lifecycle-aware controller. It trims channels, starts a replica before manager visibility, rolls back on manager-add failure, and removes/stops deterministically.
- `internal/factory/engine_factory.go`
  - Added `EngineOptions` and `NewEngineWithOptions`.
  - Preserved service construction, optional DBZ detection, identity repository wiring, and ZOMBIE substitutions.
  - Supports permanent and temporary onlineSet-only listener profiles.
  - Retained `NewEngine(...) common.Engine` compatibility wrapper.
- `internal/factory/replica_factory.go`
  - Added config/repository-sharing replica construction for distinct channels.
- `internal/listener/snapshot/session_factory.go`
  - Added collision retry for temporary IDs.
  - Added `CoordinatedSessionFactory` and idempotent coordinated session close/registry cleanup.
- `internal/command/registry.go`
  - Replaced targeted replica, replicaoff, replicastatus, and same-room msgchannel placeholder behavior with concrete controller/chat paths.
  - Whiskey now fails explicitly when proxy configuration is unavailable rather than claiming success.
- `cmd/zenbot/main.go`
  - Startup now uses managed MASTER transport/lifecycle, host-owned replica manager/controller, SIGINT/SIGTERM cancellation, lifecycle error draining, bounded shutdown, replica shutdown, and DB cleanup.
- `internal/factory/engine_factory_test.go`
  - Real `httptest` WebSocket MASTER/REPLICA integration test; verifies independent join payloads and managed shutdown.
- `internal/listener/snapshot/coordinated_factory_test.go`
  - Verifies temporary registry cleanup and exactly-once underlying session close.

Existing unrelated dirty files were preserved. No Saturn files or migration files were changed by this slice.

## Compatibility adaptations

- Go cannot overload `Start()`/`Stop()`. The broad legacy `common.Engine` contract remains unchanged; managed callers use `StartContext(context.Context)` and `StopContext(context.Context)` through `ManagedEngine`/the lifecycle adapter.
- `NewEngine` still returns `common.Engine`; new callers use `NewEngineWithOptions`.
- Legacy `HcConnection` and queue behavior remain available for old callers, but the new main path never uses the legacy connection.
- Exact existing chat/raw serialization remains in `EngineImpl`, including addressed/whisper formatting and newline normalization.
- DBZ conditional registration and identity/repository service wiring were retained.
- Temporary profile leaves only onlineSet active and uses no host replica-manager registration.

## Actual verification output

All commands were run from `/Users/ab/workspace/go-projects/zenbot`.

- `go test -count=1 ./...` — exit 0; all packages passed.
- `go test -race ./...` — exit 0; all packages passed.
- `go vet ./...` — exit 0; no output.
- `go build ./...` — exit 0; no output.
- `git diff --check` — exit 0; no output.
- `go test -count=1 ./internal/repository/h2 ./internal/service ./internal/command -run DBZ` — exit 0; all selected packages passed.
- `go test -count=1 ./internal/core -run Lifecycle` — exit 0; lifecycle tests passed.
- Focused integration run: `go test -count=1 ./internal/factory ./internal/listener/snapshot ./internal/command ./internal/core` — exit 0.

## Remaining gaps

- Remote-room `msgchannel`/`msgroom` operation still returns an explicit configuration error; a coordinator-backed remote snapshot operation needs to be connected once the application’s room-operation wiring is selected.
- Whiskey reports unavailable configuration because Go `config.Config` has no proxy list/probe source. No speculative proxy behavior was added.
- Transport read errors are currently logged by the engine loop; they are not yet injected into a per-engine lifecycle error sink. Lifecycle terminal errors themselves are observable through `Lifecycle.Errors()`.
- The new factory integration test verifies independent MASTER/REPLICA joins and onlineSet transport setup; it does not exercise a live command-triggered replica because that requires a second configured endpoint per replica.
