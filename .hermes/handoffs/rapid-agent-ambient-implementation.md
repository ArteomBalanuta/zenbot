# Rapid live-agent ambient implementation

## Scope completed

Implemented the approved ambient room-participation vertical only. The implementation preserves the existing direct, mention, and relay routes and does not edit `MIGRATION_PLAN.md` or `.hermes/migration-audit.md`.

### Touched implementation files

- `internal/agent/participation/invocation.go`
  - Removed caller-provided `Event.EligibleCount`.
  - Added one private atomic pipeline-wide cadence counter.
  - Counts only after eligibility, polite quiet handling, and mention precedence; quiet public events do not submit ambient.
- `internal/agent/live/participation.go`
  - `RoomParticipation` now carries resolved ambient enablement/cadence and passes them into the pipeline.
- `internal/agent/runtime/runtime.go`
  - Added `SubmitAmbient` and a single latest-wins pending ambient slot.
  - Ambient bypasses ordinary `admission`, while using the runtime context, room serialization, slots, runner, sink, and close lifecycle.
- `internal/agent/runtime/api_bridge.go`
  - Dispatches API `AMBIENT` submissions to `SubmitAmbient`; direct/mention modes retain ordinary `Submit`.
- `internal/agent/live/runner.go`
  - All blank trimmed model output now returns `agent returned an empty response`; exact marker remains silent and embedded markers remain visible.
- `cmd/zenbot/main.go`
  - Wires resolved `Ambient`, `AmbientEveryMessages`, and a positive-duration `QuietRegistry` into the master room listener pipeline.

### Tests added/expanded

- `internal/agent/live/participation_test.go`: cadence/quiet/mention precedence and stable-identity quiet expiry.
- `internal/agent/runtime/ambient_test.go`: latest-wins replacement, ordinary admission coexistence, close cancellation/drop-pending/wait semantics.
- `internal/agent/live/runner_test.go`: blank AMBIENT error plus marker behavior.
- `cmd/zenbot/live_agent_test.go`: enabled config has quiet + ambient wiring; disabled config remains constructible/pass-through.

## Observed RED → GREEN evidence

1. `go test ./internal/agent/live -run 'TestRoomParticipation(Ambient|Quiet)' -count=1`
   - RED: `RoomParticipation` lacked `AmbientEnabled` and `AmbientEvery`.
   - GREEN: pass.
2. `go test ./internal/agent/runtime -run 'TestRuntime(Ambient|Close)' -count=1`
   - RED: `Runtime.SubmitAmbient undefined`.
   - GREEN: pass.
3. `go test ./internal/agent/live -run TestMarkerFinalizer -count=1`
   - RED: `empty ambient accepted`.
   - GREEN: pass.
4. `go test ./cmd/zenbot -run TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig -count=1`
   - RED: resolved ambient configuration was not wired (`AmbientEnabled:false`, cadence zero).
   - GREEN: pass.

## Runtime lifecycle semantics

- `SubmitAmbient` validates that the invocation is `AMBIENT`, returns `ErrClosed` after shutdown, overwrites the one pending invocation, and schedules only one drain.
- A running ambient completes through the existing runner/finalizer/sink path. Before the next ambient run, ordinary admitted work drains; the newest pending ambient then executes. Older pending ambient work is never executed.
- Ambient does not acquire normal reply-required admission and therefore never returns `ErrBusy` merely because direct/mention capacity is full.
- Ambient runner/finalizer/provider failures have no failure sink reply because AMBIENT does not require a reply. Exact no-reply marker delivers nothing; blank output returns an error and delivers no newline-only message.
- `Close` marks closed and clears pending ambient under the runtime mutex, cancels the shared context, waits ordinary workers/executions and the ambient drain, and rejects later ambient submission with `ErrClosed`.

## Gates observed

- Focused participation/live/runtime/cmd tests: PASS.
- `go test -race ./internal/agent/participation ./internal/agent/live ./internal/agent/runtime ./internal/listener/message -count=1`: PASS.
- Repeated ambient runtime race test (`-count=20`): PASS.
- `go test ./...`: PASS.
- `go build ./...`: PASS.
- `git diff --check`: PASS.
- `go vet ./...`: FAILS on pre-existing unrelated `internal/core/engine_impl.go:95:22` (`NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`). No change made outside slice scope.

## Exclusions retained

No H2/SQL/history/durable memory/tools/moderation/remote/replica work; no direct `l`, relay, mention admission UX, command ordering, migration-plan/audit, commit, or push changes.
