# Post-`room_users` rapid live-agent vertical: durable H2 turn memory

## Decision

**Select durable, bounded H2-backed conversation memory for the live agent.** It is the smallest remaining high-value capability that changes every successful live direct/mention/ambient turn without widening the frozen two-tool loop, accepting generated SQL, dispatching commands, or automating moderation.

This is a real live capability, not a persistence-only cleanup: the next eligible invocation receives recent successful conversation turns scoped by the trusted `api.Context.MemoryKey()`, and a successful visible result becomes durable across process restart. The target already bootstraps `agent_memory` and `agent_tool_memory` in the running H2 database (`internal/repository/h2/schema-h2.sql`) and already has the exact memory-key, interface, legacy filtering, and H2 connection dependencies. The missing work is a narrow repository implementation and composition into the existing live paths.

### Candidate comparison

| Candidate | Decision |
|---|---|
| **Durable turn memory** | **Selected.** Existing H2 schema, exact memory-key contract, and `turn.MemoryStore`/`TurnMemory` seams make the prerequisite contained. It improves all existing live modes without a new LLM/tool round trip. |
| `database_schema` / dynamic SQL | Defer. Saturn makes this creator/admin-only `DYNAMIC_SQL` metadata and an ordered prerequisite for dynamic SQL (`DatabaseSchemaTool`, `H2AgentSchemaRepository`). Zenbot’s frozen loop allows one tool call total, and it currently has no resolved SQL bounds or end-to-end trusted capability projection in direct `l`; exposing schema now has low immediate utility and expands privileged database disclosure. |
| `database_query` | Defer. Saturn’s `H2AgentQueryRepository` has six query routes and several identity/room visibility contracts; some return persisted identity fields. It needs a dedicated privacy review, repository contract, result bounds, and public-scope decision. |
| `run_command` gateway | Defer. `RunCommandTool` is an action/room-delivery boundary. It needs a synthetic-command gateway, current authorization replay, output capture, per-command catalogue, duplicate-delivery policy, and a stronger multi-step/action turn policy. |
| Response corrector/router | Defer. Saturn’s `AgentResponseCorrector` owns stale retries, quote-only structured correction, action-claim correction, and evidence-leak recovery. Migrating it would alter provider-call bounds and final response semantics, not add the smallest independent capability. |
| Moderation | Defer. `RoomModerationMonitor` produces action decisions from room/join telemetry and requires protected-principal policy plus engine executor correctness. It has public enforcement risk and is not a rapid read/persistence vertical. |

## Observed evidence

- **[OBSERVED]** The accepted `room_users` composition freezes exactly `user_message_history` and `room_users`, one total tool call, two provider completions, and no tools on completion two (`internal/agent/live/tool_loop.go`, `ToolLoop.Complete`, `NewBoundedToolLoop`; `.hermes/handoffs/rapid-agent-room-users-qa.md`). Memory must not change those bounds or register another tool.
- **[OBSERVED]** The target has an unconnected memory abstraction: `turn.MemoryStore` has `Load`, `Append`, and `AppendToolEvidence`; `TurnMemory` removes legacy/internal-evidence messages and translates store errors to `ErrMemoryLoad`/`ErrMemoryPersistence` (`internal/agent/turn/memory.go`). No `live.Runner`, `live.DirectInvoker`, or `ToolLoop` owns a `TurnMemory` today (`internal/agent/live/runner.go`, `internal/agent/live/direct.go`, `internal/agent/live/tool_loop.go`).
- **[OBSERVED]** H2 startup exposes a shared `*sql.DB` after identity verification and schema bootstrap (`internal/repository/h2/database.go`, `Open`). The live composition passes that same `*h2.Database` to both direct and room agents (`cmd/zenbot/main.go`, `directAgentInvoker`, `newLiveAgent`); it is therefore the correct connection owner. No extra H2 server, alternate file connection, or raw DDL parsing is required.
- **[OBSERVED]** Current schema already contains durable tables/indexes: `agent_memory(identity_key, role, content, created_on, expires_on)` and `agent_tool_memory(identity_key, tool_name, content, created_on, expires_on)`, with `idx_agent_memory_identity_created`, `idx_agent_memory_expires`, and `idx_agent_tool_memory_identity_created` (`internal/repository/h2/schema-h2.sql`).
- **[OBSERVED]** `api.Context.MemoryKey()` provides the intended isolation contract: public state is one room-wide `utf16RuneLength:room|public` bucket; whisper state is segregated by trip, then hash, then nick (`internal/agent/api/api.go`, `MemoryKey`). Runtime/API adapters preserve capabilities, identity and this context (`internal/agent/runtime/adapters.go`, `ToAPIInvocation`, `FromAPIInvocation`).
- **[OBSERVED]** Saturn persists only after a final visible response: `DefaultAgentRouter` calls `turnMemory.append` and then `appendToolEvidence` after `responseFinalizer.prepare` returns a replying result (`src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java`). `H2AgentMemoryStore` selects unexpired rows ordered newest-first then reverses them before prompt use, and writes a user/assistant pair transactionally with expiry cleanup (`src/main/java/org/saturn/app/agent/persistence/H2AgentMemoryStore.java`).
- **[LIMITATION]** Saturn also persists successful tool evidence in `agent_tool_memory`. The current Zenbot live loop does not expose successful result metadata to a post-turn owner, and source tool outputs include untrusted model data. Evidence persistence is deliberately excluded from this first durable-memory vertical rather than silently persisting arbitrary tool results.

## Selected contract

### Data ownership and visibility

1. The memory owner is a new `internal/repository/h2` implementation satisfying a **new narrow repository interface** in `internal/repository`, not an `internal/agent/turn` import of H2.
2. It uses only the already-open `*sql.DB` from `h2.Database.DB`, with parameterized SQL and request context. It never starts/stops H2, creates another connection/server, runs migrations, queries arbitrary tables, or exposes raw database errors to the model/chat.
3. Public memory is shared only within the exact trusted room key. Whisper memory is never loaded into public context and is partitioned with the existing trip/hash/nick fallback. The model never chooses a memory key, room, identity, TTL, or limit.
4. Persist only a completed exchange whose finalizer returned `shouldReply == true` and whose delivered text is nonblank. Do not persist no-reply marker outcomes, blank/error/provider/tool failures, cancellation, ambient suppression, or transport failures.
5. Tool evidence is **not written or loaded** in this slice; `agent_tool_memory` remains untouched. This preserves the existing tool-loop’s model-only evidence boundary and avoids durable disclosure of `room_users`/history content until a dedicated evidence policy exists.

### New resolved configuration

Add the following under existing `[agent]` configuration, preserving current defaults for all other fields:

```toml
memoryTurns = 6
memoryTtlMinutes = 1440
```

- `memoryTurns` is a positive integer number of completed user/assistant exchanges to retain in the prompt. Bound it to `1..60` in `AgentConfig.Validate`/`Resolve`.
- `memoryTtlMinutes` is a positive integer TTL, bound to `1..525600` (one year) before conversion to `time.Duration`; reject overflow/non-positive values.
- Use resolved defaults **6 turns** and **24 hours** only when the field is absent/zero before environment resolution, consistent with existing agent defaults. Add matching environment keys through the existing `ValueReader`: `memoryTurns`, `memoryTtlMinutes`; do not invent a second configuration system.
- The disabled-agent path remains a pass-through and must not construct a memory repository, access the DB, or read/write memory.

These are an intentional bounded target adaptation of Saturn’s validated `memoryTurns`/`memoryTtl` (`src/main/java/org/saturn/app/agent/config/AgentConfig.java`). The target names TTL in minutes because its TOML model is integer-based and existing agent duration configuration uses millis/minutes; it must be documented in `config.example.toml` during implementation.

### Repository API and record shape

Add to `internal/repository/agent_context.go` (or a new `internal/repository/agent_memory.go`):

```go
type AgentMemoryRepository interface {
    LoadAgentMemory(ctx context.Context, key string, nowMillis int64, turns int) ([]AgentMemoryMessage, error)
    AppendAgentMemory(ctx context.Context, key, user, assistant string, createdOnMillis, expiresOnMillis int64) error
}

type AgentMemoryMessage struct {
    Role    string
    Content string
}
```

`h2.Database` implements it in `internal/repository/h2/agent_memory.go`.

- `LoadAgentMemory` requires nonblank key, `turns >= 1`, and a nonnegative clock value. It selects only `role IN ('user','assistant')`, `expires_on > $now`, newest first with `LIMIT turns*2`, then reverses in Go so the LLM receives chronological order. It returns an allocated empty slice, never nil, for no rows.
- `AppendAgentMemory` requires a nonblank key/user/assistant, `expiresOnMillis > createdOnMillis`, and `ctx.Err()==nil`. In one transaction: delete only expired rows (`expires_on <= createdOnMillis`), then insert exactly the user and assistant records. It must use `BeginTx`, `Rollback` on every failed path, prepared statements, and commit only after both inserts succeed.
- Use Unix **milliseconds**, matching Zenbot’s `messages.created_on` and H2 schema conventions. Saturn’s seconds are not copied because target schema/runtime are already millisecond-oriented.
- DB failures are wrapped internally with operation context, but the live layer returns only stable memory sentinel errors; no SQL text, database path, table name, connection address, raw H2 error, or persisted content reaches a provider or chat response.

### Turn facade and composition APIs

Keep `turn.MemoryStore` as the live-facing abstraction. Add `internal/agent/live/memory.go`:

```go
type PersistentMemoryStore struct {
    Repository repository.AgentMemoryRepository
    Turns      int
    TTL        time.Duration
    Clock      func() time.Time
}

func (s PersistentMemoryStore) Load(api.Context) ([]llm.LlmMessage, error)
func (s PersistentMemoryStore) Append(api.Context, role, content string) error
```

`PersistentMemoryStore` buffers a single pending user message per call **only through `TurnMemory.Append`’s two ordered calls**. Prefer the safer small interface change instead: add `AppendExchange(ctx api.Context, user, assistant string) error` to `turn.MemoryStore`, make `TurnMemory.Append` call it when supported, and let `PersistentMemoryStore` implement that optional exchange seam atomically. Retain the existing `Append` fallback for `InMemoryStore` and unit tests. Do **not** model a pending exchange as shared mutable state on `PersistentMemoryStore`.

Construct `turn.TurnMemory` once for each agent composition helper and inject it into both:

```go
type Runner struct {
    // existing fields
    Memory *turn.TurnMemory
}

type DirectInvoker struct {
    // existing fields
    Memory *turn.TurnMemory
}
```

`newLiveAgent` and `directAgentInvoker` receive a shared `repository.AgentMemoryRepository` from the existing `agentRepositories` aggregate (extend it to embed `repository.AgentMemoryRepository`). `main` passes the same `db` as today. Each helper creates its own lightweight `PersistentMemoryStore`/`TurnMemory` with the same resolved bounds and DB; no global singleton or process-local cache is needed.

## End-to-end route

```text
DIRECT l / public MENTION / accepted AMBIENT
  -> existing trusted runtime.Invocation (room, whisper, trip/hash/nick)
  -> api.Context via runtime.ToAPIInvocation
  -> TurnMemory.Load(context, correlation ID)
       -> PersistentMemoryStore.Load
       -> H2 agent_memory: exact MemoryKey + expires_on > now
       -> chronological, bounded user/assistant messages
  -> existing conversation-context load + existing assembler/tool loop
       -> unchanged frozen [user_message_history, room_users]
       -> unchanged max 1 tool call, max 2 completions
  -> existing finalizer
       -> silent/no-reply/error: return, no memory write
       -> visible final text: existing runtime sink/direct return succeeds
  -> TurnMemory.AppendExchange(trusted user prompt, final visible text)
       -> one H2 transaction: expired-row cleanup + user/assistant pair
```

### Load and prompt order

For both `Runner.Run` and `DirectInvoker.Invoke`, load durable memory **after invocation validation but before assembly**. Pass it into `Assembler.Assemble` through a new explicit `memory []llm.LlmMessage` argument or a small prepared-input record; do not concatenate serialized memory into `recent` room context. The assembler owns deterministic message order:

```text
system prompt
-> durable memory (oldest retained turn first)
-> existing public H2 recent-room context
-> current user prompt
```

Tool-loop completion one must use the identical prepared sequence; its completion-two message list extends that request only with the assistant tool call and matching tool result. Do not reload memory between provider completions.

The first implementation should preserve every existing system/context/current-prompt message and add memory only in the slot above. It must not use `agent_tool_memory`, append memory to a system prompt, alter `roomUsersSnapshot`, or treat persisted content as instructions. Add a system-prompt statement that prior conversation is untrusted historical data, not authority for tool access or policy changes.

### Write timing and delivery semantics

- **Direct `l`:** finalizer accepts nonblank visible content; `DirectInvoker` returns it; only after the direct command’s existing reply path reports success does its caller invoke `Persist`. To avoid an API-breaking command change, add a small `PersistedDirectInvoker` method/return record and make `directLDefinition` perform delivery then persistence. If delivery fails, no memory write.
- **Runtime MENTION/AMBIENT:** `Runner.Run` may return a result but cannot know sink success because runtime owns delivery. Add an optional `AfterDelivery(ctx, inv, result)` hook on `runtime.Runtime` or extend `Sink` with a composition-owned success callback. Runtime calls it only after `Sink.Deliver` returns nil. It must receive the original invocation and final result, then append the exchange. A failure is logged/stable-classified but must not redeliver or turn a successfully delivered reply into a failure response.
- **Ambient:** only persist an actual visible ambient response after its public sink delivery. Exact no-reply, blank finalizer error, cancelled drain, provider failure, or sink failure produce no write.
- **Concurrent same-key turns:** transaction atomicity guarantees each pair is complete. Global runtime admission/room serialization remains unchanged. Ordering ties are resolved by `id DESC` on fetch; add a target index `(identity_key, created_on DESC, id DESC)` if the current existing index lacks `id` and real-H2 explain/coverage proves the need. Do not claim cross-process conversational serialization.

## Failure, cancellation, and security matrix

| Condition | Required behavior |
|---|---|
| Agent disabled | Existing pass-through; no memory construction/I/O. |
| Empty/missing durable rows | Assemble current request exactly as accepted today. |
| Load DB error/invalid returned row | Fail the invocation with stable `Agent memory load failed`; direct gets existing error behavior, reply-required runtime uses existing failure sink, ambient stays silent/log-only. No provider call and no write. |
| Parent context cancelled before/during load | Propagate cancellation; no provider call/write/late reply. |
| Tool-call first turn | Memory loads once; current frozen tool loop remains unchanged; no tool evidence persists. |
| Exact no-reply marker | Existing silence; no write. |
| Blank/finalizer/provider/tool error | Existing failure behavior; no write. |
| Sink/direct delivery error | No write, no retry, no duplicate delivery. |
| Persist transaction failure after successful delivery | Keep delivered result; log stable memory-persistence failure with correlation ID only; no failure chat, retry, or second delivery. |
| Whisper/public boundary | `MemoryKey` enforces existing public vs whisper partition; no cross-load or fallback. |
| Expired record | Excluded by `expires_on > now`; cleanup happens only during a successful append transaction. |
| Malformed/legacy persisted content | `TurnMemory.Load` applies existing legacy/internal-evidence filtering; invalid role/content is skipped/causes stable load error per repository validation, never placed in a system message. |

Security controls:

- Prepared statements only; model strings are values, never identifiers/SQL.
- No memory read is advertised as a tool and no raw memory is sent to chat.
- Store only final visible assistant output and trusted current prompt, not whispers in public buckets, tool envelopes, capabilities, trips/hashes beyond the opaque existing key, API keys, provider requests, errors, or transport metadata.
- Keep TTL/bounds server-controlled configuration; the model cannot request a larger history.
- Do not write a redacted copy of user prompts in this slice. Existing trusted prompt construction is the persistence input; if later routing transforms user text, a dedicated data-retention review must decide which canonical form is durable.

## Implementation stages

### Stage A — bounded config and repository boundary

**Paths:** `internal/config/agent_config.go`, `config.example.toml`, `internal/repository/agent_memory.go`, `internal/repository/h2/agent_memory.go`, focused tests in `internal/config`, `internal/repository/h2`.

1. Add/resolve/validate bounded `memoryTurns` and `memoryTtlMinutes`.
2. Add the narrow repository contract and real-H2 implementation over `Database.DB`.
3. Do not change schema DDL unless a real-H2 test proves the existing index cannot meet the exact ordered lookup. Any schema/index change must be in `internal/repository/h2/schema-h2.sql` and H2 bootstrap, with an upgrade-safe `CREATE INDEX IF NOT EXISTS` path.

### Stage B — atomic live-facing memory store

**Paths:** `internal/agent/turn/memory.go`, new `internal/agent/live/memory.go`, focused tests.

1. Add optional atomic-exchange capability to the existing memory abstraction without breaking `InMemoryStore` tests.
2. Implement `PersistentMemoryStore` using an injected repository/clock/config bounds. It maps H2 messages to `llm.LlmMessage` only for `user`/`assistant`, clones all content, and maps failures to existing sentinels.
3. Preserve `TurnMemory` legacy and internal-evidence filtering. Do not add evidence writes.

### Stage C — assembly and live delivery composition

**Paths:** `internal/agent/assemble/assemble.go`, `internal/agent/live/{runner.go,direct.go,tool_loop.go}`, `internal/agent/runtime/*` only if the post-delivery hook requires it, `internal/command/handlers.go`, `cmd/zenbot/main.go`, and focused tests.

1. Thread one explicit memory message slice into assembly in deterministic order; update tool-loop initial assembly and direct/non-tool paths together.
2. Inject memory into both direct and runtime runners using the same real `h2.Database` repository from `main`.
3. Add a post-successful-delivery persistence seam. `directLCommand.Execute` currently calls `reply`, which discards `SendChatMessage` errors (`internal/command/handlers.go`); replace only that direct-`l` path with a delivery-aware call that returns the error before `Persist`. Keep provider/tool/finalizer call count and normal runtime admission untouched.
4. Disabled configuration stays before client/catalog/assembler/memory setup as in current `newLiveAgent`/`directAgentInvoker` branches.

## Focused TDD plan and gates

Perform each RED → GREEN tracer separately.

1. **Config RED:** default/explicit/invalid zero/negative/overflow `memoryTurns` and TTL resolution. Then green only config changes.
2. **Real-H2 repository RED:** use `internal/testutil/h2fixture`; prove empty result is nonnil, chronological newest-N exchange selection, exact key isolation, public/whisper key separation, strict expiry boundary, atomic pair write/rollback, context cancellation, and no SQL error text in returned errors. Then green `agent_memory.go`.
3. **Store/turn RED:** injected clock proves load maps chronological role messages and filters legacy/internal evidence; successful `AppendExchange` makes two records; repository failure maps to existing memory sentinels. Prove no `agent_tool_memory` write.
4. **Assembly/loop RED:** preloaded memory appears once between system and current prompt in direct, runtime, and tool-loop completion one; completion two reuses the same messages and receives no reloaded memory; frozen two-tool definitions, one call, and two completions are unchanged.
5. **Delivery ordering RED:** scripted direct and runtime sinks prove no persistence on no-reply, error, cancellation, or delivery failure; one durable pair after a successful visible direct/mention/ambient delivery; persistence failure never produces a second send/failure reply.
6. **Composition RED:** enabled direct and live paths each use the real shared H2 repository; disabled path opens no agent memory seam. Restart the H2-backed fixture/client composition and prove a completed prior turn is loaded.

Run from `/Users/ab/workspace/go-projects/zenbot`, formatting only slice-owned files:

```sh
gofmt -w \
  internal/config/agent_config.go \
  internal/repository/agent_memory.go internal/repository/h2/agent_memory.go \
  internal/agent/turn/memory.go internal/agent/live/memory.go \
  internal/agent/assemble/assemble.go internal/agent/live/runner.go \
  internal/agent/live/direct.go internal/agent/live/tool_loop.go \
  internal/agent/runtime/runtime.go cmd/zenbot/main.go

go test ./internal/config -run 'Test.*Agent.*Memory' -count=1
go test ./internal/repository/h2 -run 'Test.*AgentMemory' -count=1
go test ./internal/agent/turn ./internal/agent/assemble ./internal/agent/live -run 'Test.*(Memory|ToolLoop|Direct|Runner)' -count=1
go test ./internal/agent/runtime -run 'Test.*(Memory|Runtime|Ambient)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/runtime -count=1
go test ./...
go build ./...
git diff --check
```

A real-H2 gate is required; mocks alone cannot validate the transaction, expiry predicate, ordering, or shared database composition. Record unrelated broad failures rather than changing existing dirty work.

## Exclusions

- No new tool, tool inventory change, generalized/multi-step tool router, tool-result persistence, fresh-data policy, `database_schema`, `database_query`, dynamic SQL, or H2 SQL-policy work.
- No `run_command`, command output capture, command catalogue, command authorization changes, room action, or duplicate output handling.
- No response corrector/sanitizer/stale retry/quote-only behavior, moderation monitor/executor, listener reorder, relay topology change, or protected-principal policy change.
- No changes to public conversation-context/history query semantics, room-users lookup, replica lifecycle, H2 server startup, schemas unrelated to an empirically required index, credentials, or provider configuration.
- No protected-document edits, commits, resets, cleanup, or formatting of unrelated dirty files.

## Risks and routing

| Risk | Control |
|---|---|
| Persisting a reply that was never delivered | Write only from a post-successful-delivery seam; test sink failure. |
| Privacy crossover | Reuse `api.Context.MemoryKey` unchanged; table queries bind exact key only. |
| Partial exchange | One transaction inserts both user/assistant entries; rollback test required. |
| Prompt inflation | Configured max 60 turns, exact `turns*2` SQL limit, existing assembler bounds, no evidence persistence. |
| Tool-loop regression | Explicit regression tests retain two tools / one call / two completions. |
| DB failures becoming duplicate chat behavior | Persistence errors are operational-only after delivery; never retry or redeliver. |
| Dirty worktree damage | Scope edits to the listed paths and requested handoff; preserve all existing modifications. |

- **`@developer`:** Stage A H2 repository/config; Stage B store facade; Stage C assembly/composition plus focused tests.
- **`@senior`:** approve post-delivery ownership, public/whisper retention boundary, transaction/expiry semantics, and exact no-tool-evidence exclusion before merge.
- **Independent QA:** run real-H2/restart and race gates; inspect SQL parameter binding/rollback, prompt order, delivery-before-persistence proof, disabled branch, and frozen room-users/history tool-loop invariants.
