# Managed-room `room_users` QA

## ACCEPT

The bounded managed-room `room_users` vertical meets the approved architecture after the scoped QA repairs below. No protected documents, commits, pushes, H2/history, moderation, command routing, remote `list`, or general-router behavior were changed.

## Inspected behavior

- `EngineRoomUserDirectory` reads only the host and a copied `ReplicaManager.ManagedEngines` snapshot. It does not start, stop, mutate, or retain engines. Lookup is trimmed/case-insensitive; returned room spelling and user snapshots are copied. Opaque replicas are excluded and removal is absent on later lookups.
- `room_users` uses a closed optional `room` schema (1–100), defaults only to the trusted invocation room, has a two-second read-only/model-data descriptor, and suppresses private/whisper use through the loop. It emits only `room`, nonblank copied sorted `users`, truthful `count`, `returnedCount`, and `truncated`; it caps exposed users at 200 and returns generic failure envelopes for unavailable/unmanaged lookup.
- `NewBoundedToolLoop` accepts exactly the frozen `user_message_history` + `room_users` inventory, allows one total call and two completions, preserves matching assistant/tool IDs, and forces the second request to omit tools. Unknown, malformed, blank-ID, batch, whisper, first/second `length`, second-call, and cancellation paths fail before unallowed access or a third completion.
- Main composition creates one manager/directory after host construction and injects the same directory into direct and live paths. Disabled paths return before provider, loop, and directory-dependent construction.

## QA repairs

1. Replaced `strings.ToLower` user sorting with `golang.org/x/text/cases.Fold`, which performs Unicode case folding rather than simple lowercase conversion. Raw-string ordering remains the deterministic tie-break.
2. Added a regression test for Unicode fold ordering (`S`, long s `ſ`, `t`). It failed before the repair (`"S,t,ſ"`) and passes after it.
3. Added a concurrent core test that repeatedly looks up host/replica snapshots while a managed replica is added/removed; it passes under `-race`.

Changed by QA:

- `internal/agent/tool/room_users.go`
- `internal/agent/tool/room_users_test.go`
- `internal/core/room_directory_test.go`

## Verification outputs

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

The targeted concurrent directory test also passed separately under `go test -race ./internal/core -run 'TestEngineRoomUserDirectoryLookupRacesSafelyWithReplicaChanges' -count=1`.

`go vet ./internal/core` still reports the known pre-existing warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

This vertical did not modify `NewEngineImpl`; no out-of-scope correction was made.

## Security and exclusion proof

- The final inventory is hard-frozen to the two named tools; `NewHistoryToolLoop` remains only the explicit one-tool compatibility wrapper.
- The directory exposes no raw replica map, lifecycle method, unmanaged room inventory, connection data, private identity metadata, or persistence data.
- Tool output is model-only JSON and the second completion remains the sole chat-facing synthesis path.
- Scoped diff/status inspection showed only implementation-owned paths plus this requested handoff; QA edits were limited to the three files listed above. `git diff --check` and no-index whitespace checks passed. A scan of added QA code found no secret, shell, eval, or exec patterns.
