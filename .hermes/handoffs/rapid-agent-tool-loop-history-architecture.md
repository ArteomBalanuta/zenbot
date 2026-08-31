# Rapid agent vertical: one bounded tool loop with room-scoped user history

## Decision and bounded scope

**Select the first bounded tool loop with exactly one read-only tool: `user_message_history`.** It is the next source-supported live vertical after direct `l`, mention, AGENT relay, ambient/quiet coalescing, and public-room context. It exercises Zenbot's already-present provider tool-call messages, definitions, schema/result contracts, registry, executor, cancellation, and turn accounting, while keeping the first loop small enough to secure and test end-to-end.

[OBSERVED] The accepted public-room context vertical already proves a narrow, real-H2 source of public, exact-room data: `repository.AgentConversationRepository`, `h2.Database.RecentPublicRoomMessages`, and `live.RepositoryConversationContextProvider` (`internal/repository/agent_context.go`, `internal/repository/h2/agent_context.go`, `internal/agent/live/conversation_context.go`). It explicitly excludes whispers, legacy `NULL` visibility, and cross-room rows; it does not provide arbitrary user history.

[OBSERVED] Zenbot has not wired a tool loop into either live entry point. `live.Runner.Run` and `live.DirectInvoker.Invoke` call `Assembler.Assemble(..., nil)` and perform exactly one `LlmClient.Complete` (`internal/agent/live/runner.go`, `internal/agent/live/direct.go`). `cmd/zenbot/main.go` constructs those entry points without a registry, executor, history repository, or turn coordinator. The advanced foundations are therefore not evidence of live behavior.

[OBSERVED] The foundations are sufficient but currently disconnected: `tool.Registry` emits allowed descriptors; `execution.Executor.Execute` enforces allow-list, capability, argument schema, prerequisites, duplicate/per-tool limits, timeout/cancellation, and result schema; `turn.State` enforces tool/step accounting; and OpenAI serialization preserves assistant tool calls and `tool_call_id` messages (`internal/agent/tool/tool.go`, `internal/agent/tool/execution/execution.go`, `internal/agent/turn/state.go`, `internal/agent/llm/openai/client.go`).

[OBSERVED] Saturn's corresponding vertical is much broader. Saturn `src/main/java/org/saturn/app/agent/tool/UserMessageHistoryTool.java` accepts `nick`, optional caller-selected `room`, and a model-selected limit up to 500; Saturn `src/main/java/org/saturn/app/agent/persistence/H2AgentQueryRepository.java` can search all rooms when `room` is omitted. `DefaultAgentRouter` then has a general multi-step/multi-tool policy and corrections. Those behaviors are deliberately **not** copied wholesale.

[RECOMMENDED] Migrate only the useful Saturn contract: public historical evidence for a named nick, rendered as model-visible tool data, then one synthesis completion. The safe Zenbot adaptation binds room and result limit from trusted composition/invocation state, never from the model. This avoids turning Saturn's cross-room lookup into an accidental privacy expansion.

### Explicit non-goals

- No general router, durable chat memory, fresh-data forcing, response correction, tool parallelism, retries, or third completion.
- No other tool: no room directory, command execution, moderation, weather, dynamic SQL, database-schema tool, or database-SQL tool.
- No model-directed SQL, arbitrary query name, caller-selected room, unbounded/model-selected limit, cross-room search, trip/hash filtering, or stable database-ID exposure.
- No database schema/index migration is proposed. The repository addition is one parameterized, fixed read query against the existing `messages` table.

## Source-grounded target contract

### Tool definition

The model receives exactly one OpenAI-style function definition:

```json
{
  "type": "function",
  "function": {
    "name": "user_message_history",
    "description": "Read recent public messages by one named user in this room.",
    "parameters": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "nick": {"type": "string", "minLength": 1, "maxLength": 100}
      },
      "required": ["nick"]
    }
  }
}
```

The descriptor is `AccessUser`, `ReadOnly`, `ModelData`, idempotent, needs no capability, has no prerequisites, has `ResourceReads: ["messages"]`, no writes, and has a short positive timeout (recommend **2 seconds**). Its closed input schema is material: `contract.ValidateArguments` already validates closed object schemas before execution.

`nick` is a model-supplied selector, not an authorization grant. Normalize only for comparison/query identity: trim whitespace, accept one leading `@`, and reject blank or over-100-character values. Preserve the user-visible nick from the row returned by storage. Do not add `room` or `limit` to the model schema.

### Visibility, room, and nick authorization boundary

| Boundary | Required behavior |
|---|---|
| Visibility | Query only `visibility = 'PUBLIC'`. `WHISPER` and legacy/unknown `NULL` visibility are excluded. Tool output contains no private-room or whisper data. |
| Room | Use only `inv.Context().Room()` supplied by the existing trusted invocation pipeline. Require it non-blank. The query must match room case-insensitively but bind the original trusted string; it must not trim/normalize into another room. No tool argument can widen or change scope. |
| Nick | The sole model argument is a nonblank bounded nick. Use a bound case-insensitive comparison (`LOWER(name) = LOWER($2)`); it must never interpolate nick into SQL. It may name another user, but returns only that user's **public messages in the already trusted invocation room**. This preserves the existing public-room visibility model for direct `l`, mention, AGENT relay, and ambient. |
| Caller identity | No elevated capability is needed because scope is public and current-room-only. Caller trip/hash are not queried or emitted. Whisper invocations do not expose this tool at all. |
| Result limit | `historyLimit` is a private composition constant set to `min(resolved.ContextMessageLimit, 60)` and at least 1. It is never provider input. This keeps the tool bounded without a new configuration surface. |

The fixed repository result is chronological presentation of the newest window, selected by `(created_on DESC, id DESC)` and returned `(created_on ASC, id ASC)`, matching the already accepted room-context convention. The tool output envelope is JSON produced by `encoding/json`, not formatted prompt text:

```json
{"rows":[{"name":"alice","message":"...","createdOn":1700000000000,"channel":"room"}],"returnedCount":1}
```

Only `name`, `message`, `createdOn`, and `channel` appear. Do not emit `trip` or `hash`, even though existing public context currently carries them. Historical message content is untrusted model input; the system prompt must continue to treat room/tool evidence as untrusted data, never instructions.

## Proposed end-to-end shape

```text
accepted direct l | mention | AGENT relay | ambient
  -> existing immutable runtime.Invocation (trusted room/nick/whisper)
  -> live.ToolLoop.Complete(ctx, invocation, recent)
     -> conversation-context lookup (existing; empty for whisper)
     -> Assembler.Assemble(... history tool definition ...)
     -> LLM completion #1
        -> no tool calls: existing finalization/output path
        -> exactly one user_message_history call:
             validate -> fixed public-room repository query -> JSON result envelope
             append assistant(tool_call) + tool(tool_call_id) messages
             -> LLM completion #2, with tools omitted
             -> must contain no tool calls -> existing finalization/output path
        -> any other/batch/second tool call: bounded-loop failure
  -> runtime sink / direct command response
```

The tool-loop owner is request-local. It owns no goroutine, cache, or mutable cross-request registry state. It passes the invocation context to every assembly, repository, execution, and completion call; cancellation/deadline terminates the current operation and prevents the follow-up completion.

## Staged implementation design

### Stage A — fixed, visibility-safe history repository and one tool

**Owner:** `@developer`; require `@senior` review for data-scope query and result envelope.

1. Add to `internal/repository/agent_context.go` without changing the existing interface:

   ```go
   type AgentUserMessageHistoryRepository interface {
       RecentPublicRoomMessagesForNick(context.Context, room, nick string, limit int) ([]PublicRoomMessage, error)
   }
   ```

2. In `internal/repository/h2/agent_context.go`, add `recentPublicRoomMessagesForNickSQL` and:

   ```go
   func (d *Database) RecentPublicRoomMessagesForNick(
       ctx context.Context, room, nick string, limit int,
   ) ([]repository.PublicRoomMessage, error)
   ```

   Validate initialized DB, nonblank room/nick, and positive limit before querying. Use `QueryContext` with only bound values. Keep the fixed `PUBLIC` predicate and ordered newest-window/chronological-return shape from `RecentPublicRoomMessages`. Map nullable SQL strings safely. Add compile-time assertion for the new interface.

3. Add `internal/agent/tool/user_message_history.go` with:

   ```go
   type UserMessageHistory struct {
       Repository repository.AgentUserMessageHistoryRepository
       Limit      int
   }

   func (t UserMessageHistory) Name() string
   func (t UserMessageHistory) Descriptor(api.Context) (contract.Descriptor, error)
   func (t UserMessageHistory) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error)
   ```

   `Execute` parses only the validated `nick`, derives `room` solely from `api.Context`, invokes the fixed repository once, and JSON-serializes the restricted envelope. Repository errors become `TOOL_EXECUTION_FAILED` with a stable generic message; never leak driver/SQL errors. A malformed result or serialization failure also produces that stable tool error.

4. The new tool is registered only when agent composition is enabled. Build one frozen registry containing only this tool and allow only `user_message_history`. Do not reuse future/multi-tool inventories.

### Stage B — narrow loop orchestrator and validation

**Owner:** `@senior` for orchestration/cancellation review; `@developer` for implementation.

Add `internal/agent/live/tool_loop.go`:

```go
type ToolLoop struct {
    Assembler *assemble.Assembler
    Client    llm.LlmClient
    Executor  *execution.Executor
    Tools     []any // immutable tool definitions captured at composition
    Limits    turn.ExecutionLimits
}

func (l ToolLoop) Complete(
    ctx context.Context,
    inv runtime.Invocation,
    recent string,
) (llm.LlmResponse, error)
```

`Complete` is the only new loop owner. It must not accept a repository, raw SQL, arbitrary tool list, or callback from an untrusted caller. It allows **at most one follow-up completion** after completion #1. It implements these exact transitions:

1. Check `ctx.Err`, collaborators, `Limits.MaxSteps >= 2`, and `Limits.MaxToolCalls >= 1`; otherwise error before provider work.
2. Assemble using the existing `Assembler`. For a public invocation pass the frozen single tool definition; for a whisper pass `nil` definitions and accept only a no-tool completion. Call completion #1 once.
3. Advance step 1. If finish reason is `length` and no call exists, return a truncation error. If there are no tool calls, return response #1 unchanged.
4. If tool calls exist, require **exactly one** call named `user_message_history`; reject batches, unknown names, blank call IDs, and malformed/invalid arguments through the existing executor. Reserve exactly one call through `turn.State` before execution and mark its result exactly once.
5. Render `contract.Result.Envelope()` as the tool message content. Append the provider-compatible pair: assistant message retaining response #1 content/tool call, then `tool` message with the same `tool_call_id`. Preserve call IDs; never manufacture a replacement ID except the existing freshness coordinator, which is not used in this slice.
6. Advance step 2 and make completion #2 with the updated messages and **no tools**. If context is canceled/deadline-exceeded, return it; do not issue completion #2. Reject `length`, blank non-silent content, or any second response tool call as bounded-loop failures. There is no repair prompt, no retry, and no third completion.

The executor's existing `Ledger` is constructed per `ToolLoop.Complete` call as `execution.NewLedger(map[string]int{"user_message_history": 1}, 1)`. This makes duplicate and second same-tool calls impossible within the request even if the loop guard were regressed. The outer loop independently permits only one call and one follow-up.

**Result rendering:** success uses `contract.Result.Envelope()` with the restricted JSON rows. Executor-produced errors also use that envelope so the one allowed follow-up can say data is unavailable; the system prompt must forbid inventing history after a tool error. Do not persist tool results or feed them into conversation history in this slice.

### Stage C — live composition without changing ingress

**Owner:** `@developer`; `@senior` review of direct/whisper output parity.

1. Extend `live.Runner` with `ToolLoop *ToolLoop`. In `Run`, load public context exactly as today, then delegate first/follow-up completion to `ToolLoop` when present. A nil loop preserves the existing one-completion behavior for isolated tests and disabled composition.
2. Extend `live.DirectInvoker` with the same `ToolLoop` and the existing `Finalizer` interface. It uses the same helper and current trusted invocation construction; it must not instantiate its own registry/repository/executor. Apply `MarkerFinalizer` to the completed direct response before returning text, so an exact no-reply marker is silent rather than command-visible. Therefore `l` and room-triggered invocations expose identical tool semantics and marker behavior.
3. In `cmd/zenbot/main.go`, make one private `newAgentToolLoop(resolved config.ResolvedAgentConfig, db repository.AgentUserMessageHistoryRepository, assembler *assemble.Assembler, client llm.LlmClient) (*live.ToolLoop, error)` used by both `newLiveAgent` and `directAgentInvoker`. Thread `db` through the already-open composition; do not open a second H2 connection/server.
4. Update those two composition function signatures to request the narrow history interface in addition to the existing conversation interface (or introduce one local combined interface). Assert the supplied DB implements both before enabled wiring. Disabled configuration still returns before provider/tool construction.
5. `Assembler.Assemble` already accepts tool definitions and safely projects assistant/tool pairs (`internal/agent/assemble/assemble.go`). Reuse it; do not modify system prompt ordering, message history, listener ordering, AGENT relay, room snapshots, or runtime admission/coalescing.

## Output, failure, cancellation, and silence semantics

| Situation | Direct `l` | Mention / AGENT relay | Ambient | Rule |
|---|---|---|---|---|
| First response has no tool call and visible text | existing direct reply | existing runtime sink | existing runtime sink | Preserve current output path. |
| Successful one history call + valid visible synthesis | reply once | reply once | reply only if finalizer says visible | Tool data itself is never sent to chat. |
| Exact configured no-reply marker | no returned command text | no sink delivery | no sink delivery | Route direct through the same marker policy instead of exposing the marker. |
| Tool validation/query/result failure; follow-up returns a valid visible answer | reply once, with no claim that unavailable history was read | reply once | reply only if visible | The model sees only a stable tool-error envelope. |
| Tool failure plus bad/empty/second-tool final response | command error | existing reply-required failure sink text | silent/log only | No partial tool result or fallback prose is emitted. |
| Initial/follow-up provider or assembly failure | command error | existing reply-required failure sink text | silent/log only | Keep `runtime.Mode.RequiresReply()` policy. |
| Parent cancellation/deadline or runtime close | return cancellation error | no late reply; runtime handles required failure only while open | silent | Pass the same context throughout; never convert cancellation to synthetic tool data. |

For this stage, a tool error does **not** establish fresh evidence and does not authorize a factual user-history claim. The follow-up exists only so the model can transparently report unavailability. It gets no tools, so it cannot loop.

## Focused TDD and real-H2 gates

Implement RED → GREEN in this order; do not write a broad router first.

1. **Real H2 — `internal/repository/h2/agent_user_history_test.go`:** seed `PUBLIC`, `WHISPER`, `NULL` visibility, other-room, same timestamp, null fields, special-character rows. Prove the new method returns only requested nick/public/exact room rows, newest bounded window in chronological order, and never returns trip/hash. Prove blank inputs, invalid limit, whitespace-shaped room, and injection-shaped room cannot broaden scope.
2. **Tool — `internal/agent/tool/user_message_history_test.go`:** prove exact closed descriptor/schema, `@nick` normalization, trusted room passed to repository, fixed private limit, JSON escaping/envelope, generic repository error, and no query for malformed/blank argument.
3. **Loop — `internal/agent/live/tool_loop_test.go`:** scripted LLM proves request #1 exposes exactly one definition; valid tool call yields assistant/tool pair with matching ID and exactly one no-tools follow-up; response #2 tool call fails; batch/unknown/malformed call fails before repository use; no-call path makes one completion; tool error is rendered as an error envelope then has only one follow-up; `MaxSteps`/`MaxTools` reject before provider use.
4. **Cancellation — same loop test:** canceled context before start calls neither provider nor tool; cancel during blocked repository/tool yields no follow-up; deadline/timeout maps through executor and does not start a third call.
5. **Live integration — `internal/agent/live/runner_test.go` and `direct_test.go`:** exercise direct, mention-shaped public, whisper, and ambient invocations through the common loop. Assert a whisper request has an empty definitions list and any hallucinated tool call fails without repository access; room calls use public room context; ambient error remains silent; marker is silent in both entry points.
6. **Composition — `cmd/zenbot/live_agent_test.go`:** enabled composition creates the same history-loop policy for direct and live paths using the already-open narrow repository; disabled agent remains pass-through/no provider/no tool construction.

Run from `/Users/ab/workspace/go-projects/zenbot` after formatting only files in this slice:

```sh
gofmt -w internal/repository/agent_context.go internal/repository/h2/agent_context.go \
  internal/agent/tool/user_message_history.go internal/agent/live/tool_loop.go \
  internal/agent/live/runner.go internal/agent/live/direct.go cmd/zenbot/main.go

go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessagesForNick' -count=1
go test ./internal/agent/tool -run 'TestUserMessageHistory' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop|Runner.*Tool|Direct.*Tool|.*History)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent|Test.*DirectAgent' -count=1
go test -race ./internal/agent/tool ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1
go test ./...
go build ./...
git diff --check
```

`go vet ./...` is a required observation gate; the current known unrelated warning (`internal/core/engine_impl.go:95:22` copies a lock by value) must be reported rather than silently attributed to this slice.

## Complexity, risks, and routing

**Complexity: medium-high.** The code footprint is intentionally small, but this is the first request that can make a model-selected action lead to database evidence and a second provider request. Incorrect room binding, tool-message pairing, cancellation, or response-count enforcement creates security/cost/correctness failures.

**Route:** assign Stage A implementation to a repository/tool owner, Stage B and final merge review to a senior agent/runtime engineer, and require a separate reviewer to inspect the real-H2 test data and both live composition paths after implementation. Do not delegate the overall boundary decision to a junior-only change.

**Primary risks and mitigations:**

- **Cross-room/private leakage:** no model `room` parameter; fixed `PUBLIC` SQL predicate; real-H2 negative fixtures.
- **Unbounded spend/recursion:** one allowed call, one call ID, `MaxSteps >= 2`, exactly two maximum completions, no tools on follow-up.
- **Hallucinated history after failure:** error envelope plus system guidance; no fresh-evidence success flag/persistence.
- **Cancellation leakage:** single propagated context, executor timeout, no follow-up after context error, runtime close behavior retained.
- **Dirty worktree damage:** limit edits to the listed slice files and new focused tests; do not reformat or modify existing unrelated dirty files.
