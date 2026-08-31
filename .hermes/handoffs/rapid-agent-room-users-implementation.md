# Rapid agent managed-room `room_users` implementation

## Touched paths

- `internal/core/replica_manager.go`
- `internal/core/room_directory.go`
- `internal/core/room_directory_test.go`
- `internal/agent/tool/contract/room_users.go`
- `internal/agent/tool/room_users.go`
- `internal/agent/tool/room_users_test.go`
- `internal/agent/live/tool_loop.go`
- `internal/agent/live/tool_loop_test.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

## RED / GREEN evidence

- Core RED: `go test ./internal/core -run 'TestEngineRoomUserDirectory' -count=1` failed with `undefined: EngineRoomUserDirectory`.
- Core GREEN: `go test ./internal/core -run 'Test(EngineRoomUserDirectory|ReplicaManager)' -count=1` passed.
- Tool RED: `go test ./internal/agent/tool -run 'TestRoomUsers' -count=1` failed because `RoomUsers` and its snapshot contract were undefined.
- Tool GREEN: `go test ./internal/agent/tool -run 'Test(RoomUsers|UserMessageHistory)' -count=1` passed.
- Loop RED: `go test ./internal/agent/live -run 'TestBoundedToolLoopRunsRoomUsersOnceThenSynthesizesWithoutTools' -count=1` failed with `undefined: NewBoundedToolLoop`.
- Loop GREEN: `go test ./internal/agent/live -run 'Test(ToolLoop|BoundedToolLoop)' -count=1` passed.
- Composition RED: `go test ./cmd/zenbot -run 'TestNewAgentToolLoopFreezesHistoryAndRoomUsers' -count=1` failed because `newAgentToolLoop` had no directory argument.
- Composition GREEN: `go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)|TestNewAgentToolLoop' -count=1` passed.

## Behavior and security

`EngineRoomUserDirectory` reads only the host and a copied `ReplicaManager.ManagedEngines` snapshot. It case-insensitively selects a managed room, returns stored room spelling and copied `ActiveUserNames`, and neither starts/stops nor mutates replicas. Removed replicas are absent from later snapshots; opaque `Replica` implementations remain hidden.

`room_users` is public/read-only/model-data with a closed optional `{room}` schema, a two-second descriptor timeout, no whisper/private/history guidance, and no capability or writes. Omitted room uses trusted invocation room. Input is trimmed and bounded to 100 runes; malformed, unknown-key, blank, overlong, unmanaged, nil-directory, and unavailable results use the normal generic error path. Successful content is only JSON with `room`, nonblank copied sorted users, `count`, `returnedCount`, and `truncated`; output is capped at 200 names while count remains truthful.

`NewBoundedToolLoop` freezes exactly `user_message_history` plus `room_users`. It validates the fixed inventory, permits one total tool call, appends one matching assistant/tool pair, and makes one no-tools synthesis call. Batches, blank IDs, unknown calls, whispers, malformed arguments, length completions, cancellation, and any tool call in completion two fail without a third call. Production creates one directory after host and manager construction and injects it into both direct and live paths. Disabled agent branches remain before provider and loop construction.

## Verification

Passed:

```text
go test ./internal/core -run 'Test(EngineRoomUserDirectory|ReplicaManager)' -count=1
go test ./internal/agent/tool -run 'Test(RoomUsers|UserMessageHistory)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop|BoundedToolLoop)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)|TestNewAgentToolLoop' -count=1
go test -race ./internal/core ./internal/agent/tool ./internal/agent/live -count=1
go test ./...
go build ./...
git diff --check
```

All passed. `go vet ./internal/core` separately reports the pre-existing warning `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`; this slice does not alter that function.
