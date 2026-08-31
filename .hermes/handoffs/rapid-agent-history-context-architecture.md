# Rapid live-agent history/context architecture: bounded public room context

## Decision

**Select the narrower conversation-context vertical, not a live `user_message_history` tool loop.** Add a concrete, read-only, H2-backed `recent public messages for this room` provider to every live request path: public `MENTION`/`AMBIENT` through `live.Runner`, and `DIRECT` through `live.DirectInvoker`. It makes current public discussion available to the model without exposing an unexecutable tool.

[OBSERVED] Saturn already composes `RepositoryAgentConversationContextProvider` into `DefaultAgentRouter` through `AgentRouterFactory.create`, and the router loads it before request assembly for non-whispers (`src/main/java/org/saturn/app/agent/routing/AgentRouterFactory.java:13-37`, `DefaultAgentRouter.routeInSession` and `loadConversationContext`). Its H2 named query limits newest rows then returns them oldest-to-newest (`src/main/java/org/saturn/app/agent/persistence/H2AgentQueryRepository.recentMessagesForRoom`).

[OBSERVED] Zenbot has the compatible data/config/prompt seams: `messages.visibility`, `channel`, `name`, `trip`, `hash`, `created_on`, and `id` already exist in `internal/repository/h2/schema-h2.sql`; inbound chat is audited *before* participation (`message.DefaultChainWithParticipation`, `AuditChatMessage.Handle`); resolved `agent.contextMessageLimit` already defaults to 60 and validates positive (`internal/config/agent_config.go:10-180`); and `assemble.SystemPrompt.Render` already accepts a `recent string` and renders it into `RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA` (`internal/agent/assemble/assemble.go:64-112`).

[OBSERVED] A `user_message_history` tool alone is not viable in the accepted live runner. The target has generic contracts and an executor (`internal/agent/tool.Tool`, `tool.Registry`, `tool/execution.Executor`), plus fresh-data foundations (`internal/agent/turn.FreshDataCoordinator`), but `live.Runner.Run` calls `Assembler.Assemble(..., nil, "", nil, ...)` then exactly one `Client.Complete`; it supplies no definitions, executes no `LlmToolCall`, and cannot feed tool results back (`internal/agent/live/runner.go:34-60`). `live.DirectInvoker.Invoke` has the same one-shot shape (`internal/agent/live/direct.go:22-46`). Publishing the tool now would create a false capability: model calls could not be answered.

[RECOMMENDED] Do not attach a tool descriptor, registry, executor, or fresh-data coordinator in this slice. The next tool-loop vertical may use the context repository as a read-only foundation, but must be designed as an explicit bounded multi-call runner. This slice instead establishes safe, useful room context with no tool exposure and no model-directed database scope.

## Source contract and intentional target adaptation

| Saturn source symbol | Observed source contract | Target adaptation |
|---|---|---|
| `agent/api/AgentConversationContextProvider.load(AgentContext)` and `load(context, author, text)` | Returns serialized recent-room evidence; `none()` returns empty string. | New Go `live.ConversationContextProvider` port returns a serialized JSON envelope and error, with a `NoConversationContext` implementation that returns `""`, nil. |
| `agent/persistence/H2AgentQueryRepository.recentMessagesForRoom` | `LOWER(channel)=LOWER(?) AND visibility='PUBLIC'`; newest bounded by `created_on DESC,id DESC`; outer order is chronological `ASC,ASC`; rows carry `name,trip,hash,message,createdOn,channel`. | `h2.Database.RecentPublicRoomMessages(ctx, room, limit)` uses the same prepared, two-stage query and maps concrete rows. |
| `RepositoryAgentConversationContextProvider.load(context, author, text)` | Loads room evidence and removes the newest chronological row whose `name` and `message` exactly equal inbound author/text, preventing prompt duplication after audit. | `live.RepositoryConversationContextProvider.Load` serializes the same envelope and removes the last exact `(name,message)` match. It deliberately does **not** use nick/trip/hash as an extra exclusion predicate, preserving Saturn’s current contract. |
| `DefaultAgentRouter.loadConversationContext` | Whispers receive no public room context; provider exceptions are logged and treated as empty context. | Both live paths call the provider only when `!inv.Context().Whisper()`. A query/encoding failure degrades to `""` and is logged with request ID; it never changes reply/silence policy. |
| `UserMessageHistoryTool.execute` / `H2AgentQueryRepository.recentMessagesForUser` | Separate public named-user evidence tool: normalized required nick, optional room, default all rooms, max 500, newest-first, with evidence metadata. | Explicitly excluded. There is no target tool loop; room context must not widen into cross-room, nick, trip, or hash lookup. |

**Important source limitation:** Saturn removes by `(author name, text)`, not row ID. Its reverse scan removes the last matching item from the returned oldest-to-newest array (`RepositoryAgentConversationContextProvider.load(..., author, text)`). Therefore a repeated same-nick/same-text current message removes the newest matching duplicate in the bounded result; an older duplicate remains. Zenbot should preserve that behavior rather than inventing a synthetic message ID that the current `AuditChatMessage`/`runtime.Invocation` boundary does not retain.

## End-to-end target shape

```text
inbound public chat
  -> message.ResolveUserMetadata
  -> message.AuditChatMessage
       -> h2.Database.MessageAudit(... visibility PUBLIC/WHISPER)
  -> existing participation: mention / ambient invocation
  -> runtime.Runtime -> live.Runner.Run
       -> non-whisper: RepositoryConversationContextProvider.Load(invocation)
            -> h2.Database.RecentPublicRoomMessages(ctx, room, contextMessageLimit)
            -> JSON {"rows":[oldest ... newest]} minus newest current (name,text)
       -> assemble.Assembler.Assemble(ctx, inv, nil, recentJSON, nil, Talk)
       -> system prompt carries untrusted public-room evidence
       -> one existing provider completion/finalizer/sink

`l` direct command
  -> live.DirectInvoker.Invoke
       -> same non-whisper context load and same assembly argument
       -> existing one provider completion / command reply
```

[OBSERVED] This order is feasible because `DefaultChainWithParticipation` orders `AuditChatMessage{}` before `AgentParticipation{}` (`internal/listener/message/handlers.go:191-199`). The provider may query the just-audited message, then remove one matching current row. It must not write, retry, or alter listener ordering.

## Exact target interfaces, models, and files

### 1. New read-only repository seam

**Add `internal/repository/agent_context.go`:**

```go
package repository

type PublicRoomMessage struct {
    Name, Trip, Hash, Message, Channel string
    CreatedOnMillis                     int64
}

type AgentConversationRepository interface {
    RecentPublicRoomMessages(ctx context.Context, room string, limit int) ([]PublicRoomMessage, error)
}
```

`PublicRoomMessage` is persistence output, not a trusted prompt instruction. Preserve nullable database fields as empty strings using `sql.NullString`, matching `h2.Database.LastMessages` (`internal/repository/h2/identity.go:95-115`). Do not expose database `id`: source provider’s model JSON has no `id`, and it must not become a model-visible stable identifier in this rapid slice.

**Add `internal/repository/h2/agent_context.go`:** implement:

```go
func (d *Database) RecentPublicRoomMessages(
    ctx context.Context, room string, limit int,
) ([]repository.PublicRoomMessage, error)
```

Requirements:

- Reject a nil database / nil `DB`, blank trimmed room, and `limit <= 0` before executing a query. These are programming/configuration errors for this internal port; do not silently query a global room or default the limit.
- Use `QueryContext` and prepared placeholders `$1`, `$2`; use no `fmt.Sprintf` for room or limit.
- SQL must be exactly this semantic shape:

```sql
SELECT name, trip, hash, message, created_on, channel
FROM (
  SELECT id, name, trip, hash, message, created_on, channel
  FROM messages
  WHERE LOWER(channel) = LOWER($1)
    AND visibility = 'PUBLIC'
  ORDER BY created_on DESC, id DESC
  LIMIT $2
) recent
ORDER BY created_on ASC, id ASC
```

- Scan `name`, `trip`, `hash`, `message`, and `channel` through `sql.NullString`; scan `created_on` as `int64`. Close rows, return `rows.Err()`, and wrap SQL failures as `recent public room messages: %w`.
- No schema change is required: `idx_agent_messages_room_visibility_created` already covers `(channel, visibility, created_on DESC, id DESC)` (`internal/repository/h2/schema-h2.sql:67`). Do not perform an exhaustive index/schema audit.

### 2. New live provider/serialization seam

**Add `internal/agent/live/conversation_context.go`:**

```go
type ConversationContextProvider interface {
    Load(context.Context, runtime.Invocation) (string, error)
}

type NoConversationContext struct{}
func (NoConversationContext) Load(context.Context, runtime.Invocation) (string, error) {
    return "", nil
}

type RepositoryConversationContextProvider struct {
    Repository repository.AgentConversationRepository
    MessageLimit int
}

func (p RepositoryConversationContextProvider) Load(
    ctx context.Context, inv runtime.Invocation,
) (string, error)
```

Validation belongs in the constructor `NewRepositoryConversationContextProvider(repo, messageLimit)`, which returns an error for nil repo or non-positive limit. This ensures `newLiveAgent` and `directAgentInvoker` cannot produce a partially configured enabled agent.

The provider output is model-facing JSON, exactly:

```json
{
  "rows": [
    {
      "name": "alice",
      "trip": "trip-a",
      "hash": "hash-a",
      "message": "earlier public text",
      "createdOn": 1710000000000,
      "channel": "lounge"
    }
  ]
}
```

Use a private JSON DTO with the preceding camel-case field names; serialize with `encoding/json`. Always return `{"rows":[]}` for a successful empty query. This matches Saturn’s `rows(...)` envelope and gives `assemble.SystemPrompt.Render` stable evidence syntax. Do not use `fmt.Sprintf` or manually constructed JSON: messages/nicks may contain quotes, slashes, Unicode, and prompt-injection text.

`Load` algorithm:

1. Return `""`, nil immediately for `inv.Context().Whisper()`; public room history must never enter a whisper prompt.
2. Call `Repository.RecentPublicRoomMessages(ctx, inv.Context().Room(), MessageLimit)`.
3. For nonblank `inv.Context().Nick()` and non-nil current message text, reverse-scan returned chronological rows and remove only the first exact `row.Name == inv.Context().Nick() && row.Message == inv.CurrentMessageText()` match. Do not case-fold, trim, match room/name aliases, or add trip/hash predicates. Invocation current text is the trusted inbound event already audited, whereas historical row text remains untrusted evidence.
4. Marshal the remaining chronological rows. Return query/serialization errors to caller; never substitute partial JSON.

[RECOMMENDED] The constructor should copy only references needed for immutable behavior. It does not own `*h2.Database`, its `*sql.DB`, or H2 server lifecycle.

### 3. Thread context through the two live request owners

**Modify `internal/agent/live/runner.go`:** add `ConversationContext ConversationContextProvider` to `Runner`. At the current `Assembler.Assemble` call, load room context and pass it as the fourth argument:

```go
recent, err := loadRecentContext(ctx, r.ConversationContext, inv)
if err != nil { /* log and continue with empty */ }
prepared, err := r.Assembler.Assemble(ctx, inv, nil, recent, nil, assemble.Talk)
```

Keep the provider nil-safe: `nil` is equivalent to `NoConversationContext`, preserving existing focused tests and non-live callers. Put the common whisper/nil/error handling in unexported `loadRecentContext`, not duplicated in direct and runner paths. It should log `agent conversation context load failed requestID=<id>: <error>` only after verifying `ctx.Err()==nil`; return `""` on failure.

**Modify `internal/agent/live/direct.go`:** add `ConversationContext ConversationContextProvider` to `DirectInvoker` and use the same helper before `Assembler.Assemble`. This matters because direct `l` is separately constructed and synchronous; adding context only to `Runner` would violate the stated all-live-request capability.

**No assembler/prompt schema change:** `assemble.Assembler.Assemble` already takes `recent string`, truncates to current prompt budget, and `SystemPrompt.Render` labels it untrusted (`internal/agent/assemble/assemble.go:240-279`, `64-112`). Preserve its current `{"rows":[]}` fallback for an empty string.

### 4. Composition and lifecycle prerequisite

**Modify `cmd/zenbot/main.go` only:**

1. After `h2.Open` succeeds, construct exactly one `live.RepositoryConversationContextProvider` from `db` and `resolved.ContextMessageLimit` for each enabled-agent composition path.
2. Change `newLiveAgent` to accept the existing `*h2.Database` (or the narrow `repository.AgentConversationRepository`) and inject the provider into its `live.Runner`.
3. Change `directAgentInvoker` likewise, and move its call in `main` until after `db` opens. It must receive the same H2 repository object; do not open a second H2 connection/server or add a global.
4. Leave disabled agent composition as `message.PassParticipation{}` and no provider construction/query. Existing direct command registration behavior when the invoker is nil remains unchanged.
5. Keep shutdown order: `roomAgent.Close()` already precedes host/H2 deferred teardown (`cmd/zenbot/main.go:203-212`). The provider owns no close operation, so no new lifecycle hook is required.

The source’s `H2ReadOnlyConnectionFactory` opens a dedicated read-only connection, but Zenbot’s existing H2 seam is a single already-open `*sql.DB` (`h2.Database.DB`, `h2.Open` at `internal/repository/h2/database.go:261-322`). Use this seam; do **not** introduce a new H2 URL/config/read-only factory. The query is structurally fixed, parameterized, and read-only. Database process ownership remains exclusively with `h2.Open`/`Database.Close`.

## Visibility, identity, room, and ordering semantics

| Dimension | Required behavior | Evidence/source |
|---|---|---|
| Visibility | Include only rows with `visibility = 'PUBLIC'`. Exclude `WHISPER` and legacy `NULL` visibility. | Saturn `H2AgentQueryRepository.recentMessagesForRoom`; target schema constraint at `schema-h2.sql:15-20`; target `LastMessages` tests already prove whisper exclusion. |
| Room | Case-insensitive exact channel equality via `LOWER(channel)=LOWER($1)`. No all-room fallback. Context uses `inv.Context().Room()`, not model input. | Saturn `recentMessagesForRoom`; target `runtime.Context.Room()` is immutable (`runtime/contracts.go:36-64`). |
| Name | Every returned row includes `name`; name is used only for exact current-event duplicate removal. It is not a user-history search input in this slice. | Saturn provider `load(context, author, text)`. |
| Trip/hash | Returned as descriptive public-event identity fields only; neither constrains room context nor authorizes it. No trip/hash lookup, aggregation, filtering, or capability decision occurs here. | Saturn `messageRow`; target `model.ChatMessage`/`runtime.Context` fields. |
| Whisper invocation | Return empty context and do not query. This avoids copying public-room history into private conversations. | Saturn `DefaultAgentRouter.loadConversationContext`. |
| Limit | Exactly resolved positive `agent.contextMessageLimit`, already default 60. Database selects the newest N rows, then presentation is chronological. | Saturn max generic limit 60; target config defaults/validation. |
| Tie ordering | Newest selection: `(created_on DESC, id DESC)`. Model presentation: `(created_on ASC, id ASC)`. Equal timestamps are deterministic. | Saturn `recentMessagesForRoom`; target has `id` identity key. |
| Current inbound event | Because audit precedes participation, remove the newest matching `(name,message)` row from the bounded chronological result. Preserve an older duplicate. | Saturn provider reverse scan; target chain order. |
| Prompt trust | Context is serialized data inserted only in the existing `RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA` prompt field; it is not a tool result or instructions. | `resources/agent/system/system-policy.txt`; `assemble.SystemPrompt.Render`. |

## Tool exposure, assembly, and turn impact

- **Tool exposure:** none. Do not alter `internal/agent/tool/tool.go`, `tool/execution`, tool contracts, `turn.FreshDataCoordinator`, `turn.State`, or `resources/agent/tool-copy.json`.
- **Assembler:** no signature change. It already has the exact `recent` input, truncation, and system prompt insertion seam. The only changed call sites are the live runner and direct invoker, from `""` to safely loaded serialized context.
- **Provider request:** still one completion. `PreparedRequest.Tools()` remains empty, and no `LlmToolCall` is executed.
- **Turn/memory:** no persistent agent memory, tool evidence, session locks, fresh-history enforcement, response correction, or multi-step budget changes. Existing `turn.UserMessageHistory` heuristics remain unused in live flow and must not imply the new room context is user-history evidence.
- **Future boundary:** a later `user_message_history` vertical should add a `tool.Tool` implementation with Saturn-compatible `{nick, limit?, room?}` closed JSON schema, named-user all-room/default behavior, 500 cap, newest-first ordering, returned-count/timestamp metadata, and an explicit bounded tool-loop runner. It must not be smuggled into this context PR.

## Error and silence rules

| Condition | Required outcome |
|---|---|
| Disabled agent | No context provider is created or queried; existing pass-through/direct behavior remains. |
| Public invocation and no public rows | Provider passes `{"rows":[]}` to assembly. Normal model behavior continues. |
| Whisper invocation | Provider is not queried; assembly receives `""`, which existing system prompt renders as its empty-row fallback. |
| H2 query error, row scan error, cancellation, or JSON marshal failure | Live helper logs operational context (unless parent context is already canceled) and returns empty context. Existing runner/direct provider error behavior decides the request; no database error text is sent to the room/user. |
| Context cancellation before/during query | Return/observe `ctx.Err()`; do not retry. Runtime/direct caller retains current cancellation handling. |
| Exact no-reply marker / ambient blank/provider failure after successful context load | Existing `MarkerFinalizer`/runtime silence semantics remain unchanged. Context errors never create a reply, failure reply, retry, or marker. |
| SQL injection-shaped room/name/message | Room is bound as a query parameter; names/messages are only scanned and JSON-encoded. No model-provided SQL or query choice exists. |
| `user_message_history` model tool call | Impossible to produce in this slice because zero tool definitions are exposed. If an unrelated client fabricated a call, existing live one-shot runner still does not execute it; do not claim support. |

## Focused TDD plan

Perform vertical RED→GREEN steps; do not write production code before the corresponding focused failure.

1. **RED: `internal/repository/h2/agent_context_test.go`** using `internal/testutil/h2fixture.Open` and a real H2 PG-wire database. Insert public/current-room rows, a whisper, `NULL` visibility legacy row, other-room public row, and tied timestamps. Assert only current-room PUBLIC values are returned; newest-window selection honors descending `(created_on,id)` and returned order is ascending `(created_on,id)`; `name/trip/hash/channel` and text survive special characters; bad room/limit reject before querying.
2. **GREEN:** add `repository.PublicRoomMessage`, `repository.AgentConversationRepository`, and `h2.Database.RecentPublicRoomMessages`; run the focused real-H2 test.
3. **RED: `internal/agent/live/conversation_context_test.go`** with a fake narrow repository. Assert constructor rejects nil/non-positive inputs; empty returns exactly `{"rows":[]}`; output JSON escapes untrusted content; public provider calls with trusted invocation room and configured limit; whisper does not call repository; it removes only newest exact `(name,message)` duplicate and preserves older duplicate; repository errors propagate to helper/provider contract.
4. **GREEN:** add the live provider and shared `loadRecentContext` fallback helper. Unit-test helper logs/degrades only for a live query error, while cancellation is returned to caller rather than turned into a normal lookup.
5. **RED: `internal/agent/live/runner_test.go`** with a capture assembler/catalog or local test server client. For a public mention/ambient invocation, prove the provider JSON reaches `Assembler`/system message under `RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA`; for a whisper prove no public content reaches request messages; provider failure still performs the one normal completion with empty context and has no extra reply/failure action.
6. **RED: add `internal/agent/live/direct_test.go`** (currently absent). Prove public `DirectInvoker` receives the same context and whisper `l` does not; preserve current trim/empty-response behavior.
7. **RED: `cmd/zenbot/live_agent_test.go`**. With a real/fake narrow repository passed to construction, prove enabled room runner and direct invoker use `ContextMessageLimit`; disabled configuration does not require a repository; no second H2 opener is introduced. This is composition-only; do not hit network.
8. **GREEN:** make only the files listed below pass one test at a time. Then run the full focused batch and real-H2 gates.

## Required files and bounded change list

**Add**

- `internal/repository/agent_context.go`
- `internal/repository/h2/agent_context.go`
- `internal/repository/h2/agent_context_test.go`
- `internal/agent/live/conversation_context.go`
- `internal/agent/live/conversation_context_test.go`
- `internal/agent/live/direct_test.go`

**Modify**

- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `internal/agent/live/direct.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

**Do not modify**

- `internal/repository/h2/schema-h2.sql` or `resources/schema-h2.sql` (existing schema/index are sufficient)
- agent config/default docs (the required `contextMessageLimit` field already exists)
- `internal/agent/assemble/assemble.go` or prompt resources
- tool contracts/registry/execution, turn/freshness/memory, SQL policy, command/listener chain ordering, relay topology, moderation, or protected migration documentation.

## Verification gates

Run from `/Users/ab/workspace/go-projects/zenbot`. The H2 fixture requires the locally pinned H2 jar/environment already used by existing `h2fixture` tests; do not replace real H2 coverage with SQL mocks.

```sh
gofmt -w \
  internal/repository/agent_context.go \
  internal/repository/h2/agent_context.go \
  internal/repository/h2/agent_context_test.go \
  internal/agent/live/conversation_context.go \
  internal/agent/live/conversation_context_test.go \
  internal/agent/live/direct.go internal/agent/live/direct_test.go \
  internal/agent/live/runner.go internal/agent/live/runner_test.go \
  cmd/zenbot/main.go cmd/zenbot/live_agent_test.go

go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessages' -count=1
go test ./internal/agent/live -run 'Test(RepositoryConversationContext|ConversationContext|Runner.*Context|DirectInvoker)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test -race ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1
go test ./internal/repository/h2 ./internal/agent/live ./cmd/zenbot -count=1
go test ./...
go build ./...
git diff --check
```

Acceptance requires the real-H2 test to prove visibility filtering, case-insensitive room scoping, deterministic same-timestamp ordering, newest-window selection, special-character round-trip, and no secret whisper/NULL/other-room leakage. Do not waive that test merely because the provider unit test passes.

## Exclusions, risks, and senior routing

**Exclusions:** named user history/cross-room evidence; tools/tool loop; database SQL tool; dynamic SQL; durable agent memory; response-corrector/full Saturn router; freshness enforcement; changes to message audit or listener ordering; schema/index migration; retry/backoff/cache; moderator/admin capability changes; direct/mention/ambient admission changes; replica/relay changes; protected docs; commits/resets/cleanup of dirty work.

**Primary risks:** leaking whispers or legacy NULL visibility; using newest-first rows directly in prompt; choosing the wrong tie breaker; duplicating the just-audited input; accidentally applying public room context to whispers; wiring only asynchronous room agent and forgetting `l`; and accidentally exposing a tool that the one-shot live runner cannot execute.

**Routing:**

- `@developer`: repository port/H2 query, provider JSON, runner/direct injection, and focused tests.
- **Senior persistence/security reviewer:** review fixed SQL, `visibility='PUBLIC'`, case-insensitive room scope, and all real-H2 assertions.
- **Senior live-agent reviewer:** review the source adaptation, whisper suppression, duplicate-removal semantics, direct + room composition, and fallback/silence behavior.

Do not combine this vertical with the subsequent tool-loop implementation. After acceptance, the evidence/model boundary and H2 query provide a stable prerequisite for a separately reviewed `user_message_history` tool vertical.
