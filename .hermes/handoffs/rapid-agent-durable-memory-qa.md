# Durable H2 turn memory independent QA

## ACCEPT

The durable-memory vertical passes the required real-H2, delivery ordering, race, full-test, build, and whitespace gates after two focused repairs.

## Repairs made

1. `internal/command/handlers.go`: direct `l` treated a finalizer no-reply (`""`) as a successful blank transport send and then persisted the blank exchange. It now returns success without delivery or persistence for blank/no-reply content.
   - Regression coverage: `TestDirectLCommandOnlyPersistsAfterVisibleSuccessfulDelivery` proves no-reply has zero sends/writes; sink failure has one attempted send and zero writes; successful visible delivery persists exactly once.
2. `PersistentMemoryStore.AppendExchange` used `context.Background()`, so cancellation could be discarded between post-delivery ownership and the H2 write. Added request-context-aware `TurnMemory.AppendContext` / `PersistentMemoryStore.AppendExchangeContext`; direct and runtime post-delivery paths now pass their active context.
   - Regression coverage: `TestPersistentMemoryStoreDoesNotAppendAfterCancellation` proves a cancelled request records no exchange.
3. `TestRuntimeCallsPostDeliveryOnlyAfterSuccessfulVisibleSink` adds runtime proof that post-delivery persistence runs once only after a visible successful sink delivery, never after sink failure or no-reply.

## Audit findings and proof

- **Exact isolation:** H2 load binds `identity_key = $1`; API/runtime `MemoryKey` keeps public and whisper buckets distinct. The real-H2 test covers public versus whisper key isolation and the runtime contract test verifies distinct public/whisper keys.
- **Bounds/order/expiry:** H2 selects only user/assistant messages with `expires_on > now`, obtains newest bounded rows (`turns * 2`), and orders the retained set chronologically for prompt use. Real-H2 tests cover newest turn selection, chronological output, strict equality expiry exclusion, and an allocated empty result.
- **Atomic persistence:** `AppendAgentMemory` uses `BeginTx`, deferred rollback, expiry cleanup, a prepared parameterized insert statement, two inserts, then commit. No model content is interpolated into SQL. H2 errors remain behind `ErrMemoryLoad` / `ErrMemoryPersistence` at the live boundary; persistence errors after delivery only log a correlation ID and do not redeliver.
- **No tool memory:** no query or write targets `agent_tool_memory`; the frozen two-tool, one-call/two-completion loop remains unchanged.
- **Prompt order/load-once:** runner/direct load before assembly; assembler emits system → durable memory → current contextualized prompt, with existing recent-room context embedded in the system prompt. Tool completion two extends the prepared first request and never reloads durable memory.
- **Delivery ordering:** direct persistence happens after successful `SendChatMessage`; runtime persistence happens after successful `Sink.Deliver`, including mention and ambient. No-reply, blank, provider/finalizer/tool errors, cancellation, and sink failure have no write path. Persistence failure does not cause a failure message or another send.
- **Concurrency:** persistent exchange state is request-local; no shared pending-message buffer exists. The H2 transaction makes each inserted pair atomic, and runtime serializes by `MemoryKey`.
- **Disabled branch:** `newLiveAgent` and `directAgentInvoker` return before memory construction when disabled.

## Commands actually run

```text
go test ./internal/config -run 'Test.*Agent.*Memory' -count=1                         PASS
go test ./internal/repository/h2 -run 'Test.*AgentMemory' -count=1                  PASS
go test ./internal/agent/turn ./internal/agent/assemble ./internal/agent/live \
  -run 'Test.*(Memory|ToolLoop|Direct|Runner)' -count=1                              PASS
go test ./internal/agent/runtime -run 'Test.*(Memory|Runtime|Ambient)' -count=1    PASS
go test ./internal/command -run 'TestDirectLCommand.*' -count=1                     PASS
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1                  PASS
go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live \
  ./internal/agent/runtime ./internal/command -count=1                              PASS
go test ./...                                                                          PASS
go build ./...                                                                         PASS
git diff --check                                                                       PASS
go vet ./...                                                                           BLOCKED
```

`go vet ./...` reports the known unrelated pre-existing core warning: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.

## Exclusions

No changes to `agent_tool_memory`, tool inventory/router, moderation, dynamic SQL/schema policy, H2 startup/schema, or protected documents. No commit, reset, or cleanup of unrelated dirty work.
