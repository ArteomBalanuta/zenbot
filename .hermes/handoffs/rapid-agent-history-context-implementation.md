# Rapid agent public-room context implementation

## Delivered

Implemented the H2-backed, visibility-safe current public-room conversation context vertical for live `MENTION`/`AMBIENT` and direct `l` requests.

### Files added

- `internal/repository/agent_context.go`
- `internal/repository/h2/agent_context.go`
- `internal/repository/h2/agent_context_test.go`
- `internal/agent/live/conversation_context.go`
- `internal/agent/live/conversation_context_test.go`
- `internal/agent/live/direct_test.go`

### Files modified

- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `internal/agent/live/direct.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

## Behavior and security boundary

`h2.Database.RecentPublicRoomMessages` is a fixed parameterized `QueryContext` query. It rejects nil/uninitialized DBs, blank rooms, and non-positive limits before query execution. It filters exactly `visibility = 'PUBLIC'` and `LOWER(channel) = LOWER($1)`, chooses the newest limit using `(created_on DESC, id DESC)`, then presents chronological `(created_on ASC, id ASC)` order. Nullable strings map to empty strings.

`RepositoryConversationContextProvider` serializes model-facing escaped JSON as `{"rows":...}`. It never queries whispers, uses only the invocation’s trusted room and configured limit, and reverse-removes only the newest exact current `(name,message)` duplicate. No whisper, legacy `NULL` visibility, or other-room row enters its real-H2 test evidence.

Both `live.Runner` and `live.DirectInvoker` use shared `loadRecentContext`. A normal repository/encoding fault logs `agent conversation context load failed requestID=<id>: ...` and supplies empty context without changing completion/finalization behavior. A canceled context remains cancellation. Nil providers preserve previous empty-context behavior.

Enabled composition now creates one provider over the existing opened `*h2.Database`; direct-invoker creation moved after H2 opening and both consumers receive that same object. Disabled composition remains provider-free. There is no second H2 open/server/process and no context lifecycle ownership.

## TDD evidence

Observed RED before each production slice:

1. Real H2 query test: `RecentPublicRoomMessages undefined` (three compiler failures).
2. Provider/helper test: `NewRepositoryConversationContextProvider` and `loadRecentContext` undefined.
3. Runner context test: unknown `ConversationContext` field on `Runner`.
4. Direct context test: unknown `ConversationContext` field on `DirectInvoker`.
5. Composition test: too many arguments to `newLiveAgent` before repository injection.

The initial H2 GREEN exposed an actual PG-wire binding detail: H2 rejected an integer `$2` limit (`unable to encode 2 ... unknown OID 705`). The implementation keeps the parameterized `$2` query and binds `strconv.Itoa(limit)`; the real-H2 test then passed.

## Verification

Passed:

- `go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessages' -count=1`
- `go test ./internal/agent/live -run 'Test(RepositoryConversationContext|LoadRecentContext|Runner.*Context|DirectInvoker)' -count=1`
- `go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1`
- `go test -race ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1`
- `go test ./internal/repository/h2 ./internal/agent/live ./cmd/zenbot -count=1`
- `go test ./...`
- `go build ./...`
- `git diff --check`

`go vet ./...` remains non-zero only for the pre-existing known warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

It was not changed because it is outside this slice.

## Explicit no-tool boundary

No tool descriptor, registry, executor, tool loop, fresh-data coordinator, dynamic SQL, cross-room/named-user history, memory, schema/index migration, listener ordering, moderation, relay, protected-document change, commit, or push was added.
