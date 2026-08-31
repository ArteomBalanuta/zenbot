# Rapid agent public-room context QA

## Verdict

**Accepted after two targeted hardening repairs.** The visibility-safe H2 conversation-context vertical satisfies the bounded public-room contract for both asynchronous room invocations and synchronous direct invocations. No tool-loop, broader history lookup, schema migration, relay/memory behavior, or protected documentation was changed.

## Evidence

### Real-H2 repository contract

`internal/repository/h2/agent_context_test.go` uses `internal/testutil/h2fixture.Open`, not a SQL mock. It proves:

- only `visibility = 'PUBLIC'` is returned; `WHISPER` and legacy `NULL` visibility rows do not enter evidence;
- room matching is case-insensitive and exact, with no cross-room row;
- newest-window selection is `(created_on DESC, id DESC)` and returned presentation is chronological `(created_on ASC, id ASC)`, including equal timestamps;
- nullable `trip`, `hash`, and `message` map safely to empty strings;
- special-character text survives database mapping;
- nil/uninitialized database, blank room, and non-positive limits fail before a query;
- leading/trailing whitespace does not broaden exact-room scope, and an injection-shaped room is a bound value rather than SQL.

### Provider and live-path contract

- `RepositoryConversationContextProvider` emits the fixed `{"rows": [...]}` JSON envelope via `encoding/json`; names/messages with quotes, slashes, and Unicode are escaped.
- Whisper invocations return empty context without repository access.
- The provider takes only the trusted invocation room and configured limit, then removes the newest exact `(name,message)` current-event duplicate while preserving an older duplicate.
- `Runner` and `DirectInvoker` use the same `loadRecentContext` helper. Normal lookup errors log and degrade to empty context while retaining the ordinary completion. A parent cancellation or a provider-returned wrapped `context.Canceled`/`context.DeadlineExceeded` remains an error and is not logged/degraded as an ordinary lookup failure.
- Enabled composition receives the already-open narrow repository (`db`) in both construction paths; disabled composition returns before provider creation. There is no H2 opener in this vertical beyond the existing `h2.Open` lifecycle.

## Repairs made during QA

1. **Exact room scope:** `RecentPublicRoomMessages` previously assigned `strings.TrimSpace(room)` before binding the query. That meant a trusted room value such as `" lounge "` could read `"lounge"`. It now uses trimming only to reject blank input and binds the original room exactly. A real-H2 regression test covers whitespace and an injection-shaped room.
2. **Cancellation preservation:** `loadRecentContext` previously degraded a provider-returned `context.DeadlineExceeded` when the parent context had not yet reported cancellation. It now propagates wrapped `context.Canceled` and `context.DeadlineExceeded` before normal fallback/logging. The focused regression test was red before this repair and green after it.

## Verification performed

Passed:

```text
go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessages' -count=1
go test ./internal/agent/live -run 'Test(RepositoryConversationContext|LoadRecentContext|Runner.*Context|DirectInvoker)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test -race ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1
go test ./internal/repository/h2 ./internal/agent/live ./cmd/zenbot -count=1
go test ./...
go build ./...
git diff --check
```

`go vet ./...` remains non-zero exclusively for the pre-existing, out-of-scope warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Security boundary and exclusions

The database query is fixed, read-only, and parameterized. Visibility and room scope are selected only from trusted invocation/configuration inputs; historical row fields are serialized untrusted data for the pre-existing prompt boundary. No model-directed SQL, user-name/cross-room history search, stable database ID exposure, tool registration/execution, dynamic query construction, retry/cache, durable memory, listener-order change, schema/index change, commit, or push was added.
