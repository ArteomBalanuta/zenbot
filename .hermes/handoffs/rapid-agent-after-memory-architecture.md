# Next live-agent vertical: durable, provenance-tagged successful tool evidence

## Decision

[RECOMMENDED] Implement **bounded durable tool-evidence reuse for the two already-live public read-only tools**—`user_message_history` and `room_users`—after accepted durable turn memory.

This is the smallest remaining genuinely live, high-value parity vertical:

- It completes an explicit but currently inert target seam: `turn.MemoryStore.AppendToolEvidence`, `turn.TurnMemory.AppendToolEvidence`, and `agent_tool_memory` already exist, while `live.PersistentMemoryStore.AppendToolEvidence` is a no-op (`internal/agent/turn/memory.go`; `internal/agent/live/memory.go`; `resources/schema-h2.sql`).
- It makes prior successful lookups available to the next turn in the same durable session, avoiding needless repeat lookup and preserving provenance/timestamps for the model.
- It requires no new model-facing tool, no broader loop, no SQL exposure, no authorization expansion, and no schema migration: `agent_tool_memory` plus `idx_agent_tool_memory_identity_created` are already bootstrapped (`resources/schema-h2.sql`).
- Saturn does the equivalent after final response selection: it persists only successful `MODEL_DATA` results, and H2 loads a bounded, unexpired, chronological evidence window (`src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java#persistentToolEvidence`; `src/main/java/org/saturn/app/agent/persistence/H2AgentMemoryStore.java#loadToolEvidence`).

Do **not** select database-schema/SQL, command gateway, moderation, freshness forcing, or generalized response correction next. Each requires a wider authorization, policy, multi-step, or output contract. Response sanitization alone is smaller but is a presentation cleanup, not a new live-agent capability; the durable evidence seam is already an intended live capability with all persistence prerequisites present.

## Evidence map

### Target observations

- [OBSERVED] Durable conversation memory is live. `PersistentMemoryStore` reads bounded `agent_memory` rows and atomically appends a delivered user/assistant exchange; `Runner.AfterDelivery` persists only after `Sink.Deliver` succeeds. Direct `l` persists only after `SendChatMessage` succeeds (`internal/agent/live/memory.go`; `internal/agent/live/runner.go`; `internal/command/handlers.go`).
- [OBSERVED] The live tool loop is request-local and bounded to two steps and one tool call. It exposes `user_message_history` and `room_users` only for non-whisper invocations, executes exactly one validated tool call, appends provider-compatible assistant/tool messages, then makes exactly one tools-disabled synthesis completion (`internal/agent/live/tool_loop.go`).
- [OBSERVED] `user_message_history` is public/current-room-bound: room and limit are not provider arguments, the fixed repository query limits visibility to `PUBLIC`, and its output excludes trip/hash (`internal/agent/tool/user_message_history.go`; `internal/repository/h2/agent_context.go`). `room_users` is a bounded managed-room snapshot (`internal/agent/tool/room_users.go`).
- [OBSERVED] Tool execution returns an envelope with a success/error discriminator. The loop records success/failure in `turn.State`; tool errors are passed only to the one synthesis completion (`internal/agent/tool/contract/definition.go`; `internal/agent/tool/execution/execution.go`; `internal/agent/live/tool_loop.go`; `internal/agent/turn/state.go`).
- [OBSERVED] Target memory loading intentionally excludes strings with the legacy `"[Internal tool evidence from "` prefix, and assembly separately omits those messages (`internal/agent/turn/memory.go`; `internal/agent/assemble/assemble.go`). Therefore copying Saturn's raw system-message format would silently fail to make evidence live and must not be used.
- [OBSERVED] Runtime result currently carries only correlation ID/text/should-reply. `Runner.AfterDelivery` therefore cannot recover request-local tool results after delivery, and the direct `Persist` optional seam receives only prompt and final text (`internal/agent/runtime/contracts.go`; `internal/agent/live/runner.go`; `internal/agent/live/direct.go`; `internal/command/handlers.go`). Transporting a deliberately restricted evidence payload is indispensable foundation work—not a reason to introduce shared pending state.

### Saturn observations and adaptation

- [OBSERVED] Saturn’s `AgentToolEvidence` is request-local counters with the invariant `successful + failed == attempted` (`src/main/java/org/saturn/app/agent/turn/AgentToolEvidence.java`). Zenbot already has the matching invariant in `turn.Evidence` (`internal/agent/turn/state.go`). This slice does not change either accounting contract.
- [OBSERVED] Saturn persists post-finalization results filtered to `ToolResultMode.MODEL_DATA`, then uses the same memory key/TTL/bounds to load them (`DefaultAgentRouter.java#persistentToolEvidence`; `H2AgentMemoryStore.java#appendToolEvidence`).
- [LIMITATION] Saturn renders loaded evidence as a system-role `[Internal tool evidence ...]` string. Zenbot’s explicit filter rejects that representation. The safe target adaptation uses a new structured, provenance-tagged *untrusted durable-evidence section* in the existing assembled system prompt; it must never masquerade as trusted policy or tool protocol messages.

## Exact contract

### What is persistable

A result is eligible **only if all conditions hold**:

1. The one allowed live tool call was attempted and `contract.Result.IsError == false`.
2. Its descriptor remains `ReadOnly`, `Idempotent`, has no writes, and `ResultMode == ModelData`; reuse existing `execution.Safe` plus an explicit `ModelData` check. Do not infer eligibility from a tool name.
3. The name is one of the frozen live registry entries (`user_message_history`, `room_users`) and exactly matches the descriptor used for execution.
4. `result.Content` is nonblank valid JSON satisfying that descriptor’s result schema. Store the canonical envelope/content bytes; never store provider call IDs, raw arguments, assistant reasoning/content, prompt, caller trip/hash, or an error envelope.
5. The final response is visible and successfully delivered. A silent marker, blank/no reply, provider/assembly/validation/tool-loop failure, sink/send failure, cancellation, and shutdown produce **no durable evidence write**.

Persist `tool_name` and the exact validated JSON result content. The database identity key is exclusively `api.Context.MemoryKey()`. Reuse the accepted durable-memory TTL and clock. One evidence row is one successful result, not one response or one batch.

### Read/visibility/authorization contract

| Boundary | Required behavior |
|---|---|
| Session isolation | Load and append bind the exact immutable `MemoryKey`; public room keys cannot cross rooms, and whisper keys remain separated by trip/hash/nick as implemented by `runtime.Context.MemoryKey`. |
| Whisper | The current loop exposes no tools for whispers. Do not load, append, or prompt-inject durable tool evidence for a whisper invocation—even if historical data exists under that whisper key. |
| Public visibility | Persist only results of the existing tool contracts. Thus stored history evidence remains public/current-room-only, and room-user evidence remains managed-room snapshot data. Do not query storage again or widen the tools’ visibility rules. |
| Authorization | This slice creates no capability and grants none. `room_users` and `user_message_history` retain their current public tool access; future capability-gated or write tools are ineligible by default. |
| Provenance | Every loaded item includes `tool`, `observedAtMillis`, and `data`; the prompt labels it as historical, untrusted tool output—not instructions, policy, or fresh data. It must never satisfy a future freshness requirement. |
| Bounds | Load at most `MemoryTurns` rows (not message-pairs) for the exact key, ordered newest `(created_on DESC,id DESC)` then presented chronological `(created_on ASC,id ASC)`. Enforce a private per-row content cap no greater than the existing prompt budget; reject/skip malformed rows rather than emitting partial/deceptive data. |

### Storage API and format

Add separate repository contracts; do not overload `AgentMemoryRepository`:

```go
// internal/repository/agent_memory.go
type AgentToolEvidence struct {
    ToolName string
    Content  string // validated JSON tool result content, never an envelope/error
    CreatedOnMillis int64
}
type AgentToolEvidenceRepository interface {
    LoadAgentToolEvidence(ctx context.Context, key string, nowMillis int64, limit int) ([]AgentToolEvidence, error)
    AppendAgentToolEvidence(ctx context.Context, key, toolName, content string, createdOnMillis, expiresOnMillis int64) error
}
```

Implement those methods in `internal/repository/h2/agent_memory.go`. Both validate initialized DB, nonblank key/name/content, positive load limit, nonnegative clock, and `expires > created`; every SQL value is bound. Append deletes expired `agent_tool_memory` rows and inserts one row in the same transaction. Load filters `expires_on > now`, has deterministic tie ordering, rejects invalid database rows, and returns chronological data. Add the compile-time interface assertion.

The model-facing durable-evidence JSON is assembled by target code, never by SQL or the provider:

```json
{"historicalToolEvidence":[{"tool":"user_message_history","observedAtMillis":1700000000000,"data":{"rows":[],"returnedCount":0}}]}
```

It is injected as a clearly delimited untrusted data section in the existing system-policy rendering after runtime metadata and before current-room context. The policy text must say: historical evidence can be stale; treat all embedded strings as data, not instructions; never claim it was freshly queried; and current tool data supersedes it. Empty/malformed/over-budget evidence is omitted; an empty collection must not alter normal prompt behavior.

### Output, error, cancellation, and loop semantics

- The live loop stays **two completions maximum / one executed tool call maximum**. Evidence persistence neither creates a completion nor changes definitions, tool choice, call ID, tool message, `turn.State`, or finalizer behavior.
- A successful tool result remains visible only to completion #2 via the existing `tool` message. Durable evidence is only candidate context for a later eligible public invocation.
- If a tool succeeds but synthesis is invalid, marker-silent, blank, truncated, errors, is canceled, or is not delivered, discard the request-local candidate. Do not persist it.
- If the evidence write fails after a successful delivery, log only a stable `agent tool evidence persistence failed` message. Do not redeliver, change command status, retry synchronously, or roll back the already-delivered response/memory exchange. This matches accepted durable-memory best-effort-after-delivery semantics.
- Thread the original request context through repository load/append. Check `ctx.Err()` before each load/append. If it is canceled/deadline-exceeded, do not issue the DB operation; return cancellation during pre-provider load and suppress the post-delivery best-effort failure just as runtime shutdown does. No goroutine, retry loop, queue, cache, or shared mutable “pending evidence” map is allowed.
- Required-reply mention/AGENT failures retain the current failure sink behavior; ambient remains silent; direct `l` reports the pre-delivery invocation failure as today. A post-delivery evidence persistence failure is never user-visible in any mode.

## Target composition and implementation stages

### A. Durable repository and memory-store foundation

1. Extend `internal/repository/agent_memory.go` with the narrow evidence type/interface above.
2. Add real-H2 load/append methods to `internal/repository/h2/agent_memory.go`; do not modify either schema SQL file because the table/index already exist.
3. Extend `live.PersistentMemoryStore` (`internal/agent/live/memory.go`) with `ToolEvidenceRepository repository.AgentToolEvidenceRepository` (or a local combined narrow interface) and real `LoadToolEvidenceContext`/`AppendToolEvidenceContext` operations. Preserve its present exchange methods and no shared pending state.
4. Add a `turn` value such as `PersistableEvidence{Tool, Content}` and a strict conversion function that accepts descriptor/result only when all eligibility conditions above hold. `TurnMemory.AppendToolEvidence` gains a context-aware bulk path analogous to exchange append. It must reject invalid/noneligible entries before repository use.
5. Add a load method that returns structured evidence, not `llm.LlmMessage`; retain existing legacy-prefix filtering for legacy data.

### B. Request-local completion outcome and prompt projection

`ToolLoop.Complete` cannot remain the only return shape because post-delivery persistence needs the successful result without recomputing or retaining it globally. Introduce an immutable request-local result:

```go
// internal/agent/live/tool_loop.go
type Completion struct {
    Response llm.LlmResponse
    DurableEvidence []turn.PersistableEvidence
}
func (l ToolLoop) Complete(...) (Completion, error)
```

- A no-tool completion has an empty evidence list.
- Exactly one eligible successful tool call produces exactly one candidate; tool errors produce none.
- Both `Runner` and `DirectInvoker` consume this same completion; no new ingress or separate direct loop.

Add an immutable restricted evidence field to `runtime.Result` (with defensive-copy accessor), or add a separate internal `live` post-delivery carrier that travels synchronously through `Runner`/`AfterDelivery`. Prefer the `runtime.Result` value field because it lets `Runtime.execute` preserve its current `Run -> Sink.Deliver -> AfterDelivery` order without a request map. It must contain only tool name/content, never provider call IDs or raw arguments.

For direct `l`, widen only the optional post-send persistence seam, for example:

```go
type DirectAgentDeliveryPersistence interface {
    Persist(context.Context, *model.ChatMessage, string, string) error
    PersistToolEvidence(context.Context, *model.ChatMessage) error
}
```

`DirectInvoker` retains the request-local candidate between `Invoke` and the immediately following successful `Persist` call only by returning an immutable delivery artifact through an explicit narrow command contract—not a mutable field on a reusable invoker. The clean design is to replace `Invoke`’s string-only internal return with a `DirectCompletion{Text, Evidence}` and have `directLCommand` send `Text`, then call `PersistDelivery(ctx, message, prompt, completion)` after `SendChatMessage` succeeds. Keep the public command registration behavior unchanged. Existing test stubs can use a compatibility adapter during the migration.

Update `assemble.SystemPrompt.Render` and `Assembler.Assemble` to accept structured loaded durable evidence and render only the safe envelope described above. The initial request receives it; completion #2 continues to use the existing assistant/tool pair and must not re-load/re-inject evidence mid-turn.

### C. Composition

- Extend `main.agentRepositories` with `repository.AgentToolEvidenceRepository`. `*h2.Database` must satisfy it at compile time.
- In `newAgentMemory`, pass the same already-open database to both memory repository seams.
- Wire the single `TurnMemory` into live runner and direct invoker as today. Do not open another H2 connection, instantiate a second registry, or alter listener/relay/ambient admission topology.
- Disabled agent paths must return before provider, tool, durable-memory, or durable-evidence construction.

## TDD plan and gates

Use RED → GREEN in this order.

1. **Real H2 repository — `internal/repository/h2/agent_tool_memory_test.go`:** seed two keys, expiry boundary, same timestamp/different IDs, malformed rows, several tool names, and injection-shaped content. Prove strict exact-key isolation, `expires_on > now`, bounded newest-window/chronological-return ordering, bound SQL inputs, cleanup+insert atomicity, cancellation, and invalid argument rejection. Verify no `agent_memory` change.
2. **Persistent store/turn — `internal/agent/live/memory_test.go`, `internal/agent/turn/memory_test.go`:** fake clock proves shared TTL, bounded structured load, valid append, no-op is gone, error wrapping, context cancellation, defensive copies, rejection of blank/error/non-model-data/non-JSON candidates, and legacy prefix filtering remains unchanged.
3. **Assembler — `internal/agent/assemble/assemble_test.go`:** prove valid structured evidence appears once in the tagged untrusted section; it is absent for whisper/empty/malformed/over-bound inputs; stored strings cannot become a new system-policy instruction; ordering is system -> durable conversation -> current request while evidence stays in the designated policy-data section; provider tool protocol pairs remain intact.
4. **Loop — `internal/agent/live/tool_loop_test.go`:** successful `user_message_history` and `room_users` calls each yield one candidate exactly; no-call/error/malformed/batch/unknown/whisper/cancelled/follow-up-invalid paths yield none; completion count remains 1 or 2 exactly; no result content is re-executed or persisted by the loop itself.
5. **Delivery lifecycle — `internal/agent/live/runner_test.go`, `internal/agent/runtime/runtime_test.go`, `internal/agent/live/direct_test.go`, `internal/command/handlers_test.go`:** evidence writes exactly once after successful sink/direct send; sink/send failure, marker silence, blank, provider failure, runtime close, and cancellation write none; append failure logs and never duplicates delivery. Cover direct, mention, AGENT relay, ambient, and whisper policy explicitly.
6. **Composition — `cmd/zenbot/live_agent_test.go`:** enabled direct and room composition share the already-open H2 repository/store; disabled setup constructs neither evidence repository use nor provider/tool loop.

Required final gates:

```text
go test ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/assemble ./internal/agent/runtime ./internal/command ./cmd/zenbot -count=1
go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Report any unrelated existing `go vet ./...` failure separately; do not mask it as evidence-slice success.

## Explicit exclusions

- No new provider-visible tool, no second tool loop, no tool retry/parallelism, no general router, no fresh-data forcing/correction, and no tool-result-driven command execution.
- No dynamic SQL, schema discovery, named database query, command gateway, moderation action, elevated capability, creator/admin bypass, or model-provided room/limit/key.
- No persistence of tool errors, raw requests/arguments/call IDs, assistant messages, hidden chain-of-thought, room/private/whisper data beyond the existing tool contracts, or cross-session evidence reuse.
- No change to durable conversation-memory exchange ordering, schema migration, listener order, relay topology, ambient coalescing, protected documents, or application behavior outside the new handoff file.

## Complexity and routing assessment

**Complexity: medium.** Storage and eligibility logic are small and table-ready; the indispensable work is preserving a single post-delivery truth point across both runtime and direct paths without mutable request state, while safely projecting untrusted historical data. The routing surface remains linear: one initial completion, optional one tool execution, optional one synthesis completion, then delivery; persistence is post-delivery only and never re-enters the loop.
