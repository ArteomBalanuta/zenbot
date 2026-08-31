# Slice 1 live agent mention implementation

## Scope delivered

Implemented Slice 1 only: configured public mention admission through `participation.Pipeline`, async runtime execution, exact marker finalization, addressed sink/failure output, injected listener placement, trusted snapshot construction, and shutdown-before-transport ordering. Relay topology was not changed.

## TDD evidence

1. **Configuration RED** — `go test ./internal/config -run 'TestAgentConfigParticipation' -count=1` initially failed to compile because the seven participation fields were absent from `AgentConfig`/`ResolvedAgentConfig`.
   **GREEN** — after resolver/default/validation implementation: `ok zenbot/internal/config`.
2. **Runner/finalizer RED** — `go test ./internal/agent/live -run 'TestMarkerFinalizer|TestRunnerRejects' -count=1` initially failed because `MarkerFinalizer` and `Runner` did not exist.
   **GREEN** — `ok zenbot/internal/agent/live`.
3. **Failure-sink RED** — runtime test initially failed because `FailureSinkFunc` and `NewWithFailureSink` did not exist.
   **GREEN** — runtime supports reply-required failure delivery only. The focused test first exposed a test admission-capacity issue, corrected to a capacity-one queue; race then exposed an unsynchronized test observation, corrected with a mutex.

## Configuration prerequisites

`AgentConfig` now resolves `creatorTrip`, `ambientEveryMessages`, `quietMinutes`, `contextMessageLimit`, `noReplyMarker`, `maxConcurrentRequests`, and `queueCapacity` from `ValueReader`. Defaults: `595754`, `8`, `15`, `60`, `[[SATURN_NO_REPLY]]`, `1`, `0`. Required participation values validate only when `agent.enabled=true`; disabled agent resolution needs no provider credentials.

## Runtime/lifecycle semantics

- `live.Runner` assembles a `Talk` request with nil history/recent/tools, completes asynchronously through the runtime, and finalizes output.
- Exact trimmed no-reply marker suppresses delivery. Empty DIRECT/MENTION output is an error. Embedded marker prose remains reply content.
- `runtime.NewWithFailureSink` delivers runner failures only for reply-required invocations and not after runtime cancellation.
- Main wires `RoomParticipation` before command dispatch. A claimed mention stops dispatch even if admission returns `ErrBusy`/`ErrClosed`.
- Success sink sends `SendChatMessage(nick, "\n"+text, whisper)`; failure sink sends the required fixed message.
- The room runtime is closed after signal cancellation and before lifecycle/replica/DB teardown.
- Trusted snapshots use engine room, copied active-user names where the engine supplies its read-safe snapshot, resolved creator trip, and copied config admin trips; roles are nil.

## Files touched

- `internal/config/agent_config.go`
- `internal/config/agent_config_participation_test.go` (new)
- `internal/agent/live/runner.go` (new)
- `internal/agent/live/runner_test.go` (new)
- `internal/agent/live/participation.go` (new)
- `internal/agent/runtime/contracts.go`
- `internal/agent/runtime/runtime.go`
- `internal/agent/runtime/runtime_test.go`
- `internal/listener/message/handlers.go`
- `internal/listener/user_chat_listener.go`
- `internal/core/engine_impl.go`
- `cmd/zenbot/main.go`

## Verification

All green:

```text
go test ./internal/config ./internal/agent/live ./internal/agent/runtime ./internal/listener/message ./internal/listener ./cmd/zenbot -count=1
go test -race ./internal/agent/runtime ./internal/agent/live ./internal/listener/...
go test ./...
git diff --check
```

## Deferred intentionally

Slice 2 relay topology; ambient/quiet policy activation; moderation; tools; durable memory/history; SQL/H2 changes; remote/replica changes.
