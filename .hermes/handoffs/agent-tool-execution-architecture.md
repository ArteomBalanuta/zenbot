# Saturn → Zenbot Agent Tool Contracts, Execution, and Turn Boundary

**Status:** `[RECOMMENDED]` source-grounded architecture handoff; analysis only. No Saturn or Zenbot application source was modified. This document selects the next bounded migration slice and deliberately does not claim a live autonomous-agent integration.

## 1. Scope decision from the frozen inventory

The migration remains **NOT COMPLETE** (`MIGRATION_PLAN.md:11`; `.hermes/migration-audit.md:4`). The plan requires behavioral evidence, not class/file-count parity (`MIGRATION_PLAN.md:26-34,52`). The accepted routing/participation/assembly artifacts explicitly defer tool contracts/execution, turns/freshness, persistence/memory, moderation actions, listeners, and full router parity (`.hermes/handoffs/agent-routing-assembly-architecture.md:9-17,147-154`; `agent-routing-assembly-qa.md:81-91`).

### Selected audit rows

**Owned by this slice, implementable in layers:**

- **Rows #91–#103** (`AgentRoomDirectory`, `AgentToolArgumentReader`, `DatabaseQueryTool`, `DatabaseSchemaTool`, `DatabaseSqlTool`, `EngineAgentRoomDirectory`, `EngineSaturnCommandGateway`, `RoomUsersTool`, `RunCommandTool`, `SaturnCommandGateway`, `SaturnCommandTool`, `SaturnCommandToolCatalog`, `UserMessageHistoryTool`), frozen audit `.hermes/migration-audit.md:114-126`.
- **Rows #104–#107** (`AgentToolDefinitionFactory`, `AgentToolDefinitionJson`, `AgentToolSchemaValidator`, `AgentToolSchemas`), `.hermes/migration-audit.md:127-130`.
- **Rows #108–#127** (`AgentModelVisibleToolResultRenderer` through `ValidatedToolCall`), `.hermes/migration-audit.md:131-150`.

**Required minimum boundary dependencies, not all implementable in this slice:**

- **Rows #128–#143**, the complete `agent.turn` group (`AgentExecutionState` through `AgentUnverifiedActionPolicy`), `.hermes/migration-audit.md:151-166`. A small turn-state/evidence seam is required to make tool budgets, prerequisites, freshness, and result accounting truthful; full turn/freshness parity is a subsequent bounded step.
- Agent persistence rows **#43–#55** and SQL rows **#86–#90** are blocked dependencies for database tools and memory (`.hermes/migration-audit.md:66-78,109-113`). They are not silently closed here.
- Moderation rows **#36–#41**, listener rows **#218–#265**, and command/service rows are blocked for action-tool activation and live delivery (`.hermes/migration-audit.md:59-65,241-288,308-340`).

**Explicitly not selected / avoid overlap:** API rows **#3–#23** are still marked pending in the frozen table but are accepted by the API-contract handoffs; routing/participation/assembly rows **#56–#85** and the already accepted caller portion of row #300 are covered by the accepted artifacts. This document consumes those contracts and does not redefine `AgentContext`, `AgentInvocation`, routing, command-intent filtering, or assembly (`agent-routing-assembly-implementation.md:3-29`; `agent-routing-assembly-qa.md:32-41`).

## 2. Evidence map

### 2.1 Saturn API and contracts

- `src/main/java/org/saturn/app/agent/api/AgentTool.java:14-106` defines the tool boundary: stable lowercase `name`, description, contextual parameters, availability, prerequisites, descriptor, and `execute(AgentContext, JsonObject)`. Expected invalid input/capability/command outcomes are returned as `AgentToolResult`; unexpected implementation failures may throw.
- `src/main/java/org/saturn/app/agent/api/AgentToolDescriptor.java:17-35,75-108` is the immutable provider-facing contract. It validates a lowercase identifier (`[a-z][a-z0-9_]{0,63}`), nonblank metadata, parameter/result schemas, negative-use guidance, timeout, immutable sets/lists, and resource read/write declarations. `isIdempotent` is an execution property, not authority; only read-only idempotent tools without prerequisites are parallel candidates (`:11-15,247-294`).
- `AgentToolResult.java:6-85` is the observable result: call ID, tool name, content, error flag/code, JSON serialization, and stable default `TOOL_EXECUTION_FAILED` for errors. `ToolResponseEnvelope.java:8-92` fixes model-visible JSON to `{"status":"success","data":...}` or `{"status":"error","data":null,"error":{"code":...,"message":...}}`; invalid status/missing error fields are rejected, and non-JSON success content becomes a JSON string.
- `AgentContext.java` (the accepted API contract) carries room, nick, optional trip/hash, whisper privacy, room-user snapshot, capabilities, and optional moderation target. Its `MemoryKey()` uses UTF-16 room length and separates public from whisper identity (`internal/agent/api/api.go:41-123`; accepted API handoff). Privileges are trusted context data, never prompt prose.

### 2.2 Saturn schema, validation, and definitions

- `src/main/java/org/saturn/app/agent/tool/contract/AgentToolSchemaValidator.java:17-109` requires object-root parameter schemas, object properties, boolean `additionalProperties`, declared required names, and validates required/unknown arguments, primitive/object/array/null types, enum, code-point string lengths, integer-ness, and numeric bounds.
- The same validator validates result schemas and result values (`:40-54,118-132,180-245`), supporting `any`, string, boolean, number, integer, object, array, and null. Invalid schemas throw; invalid calls/results return deterministic messages. Saturn tests cover malformed declarations, required/closed objects, primitive/structured types, enum/length/numeric bounds, and result required fields (`src/test/java/org/saturn/app/agent/tool/contract/AgentToolSchemaValidatorTest.java:13-260`).
- `AgentToolSchemas.java` supplies canonical open/closed object schema construction; `AgentToolDefinitionJson.java` and `AgentToolDefinitionFactory.java` are the provider definition serialization/factory rows. The target must preserve deep-copy/validation boundaries and deterministic field shape, but should not claim provider wire parity without golden evidence.
- `SaturnCommandToolCatalog.java` scans only the user and moderator command packages, requires `@CommandAliases` and `UserCommand`, sorts by tool name, rejects duplicate tool names, and exposes a structured `arguments` string capped at 4,000 characters. It assigns user commands `ROOM_MESSAGE`, moderator commands `MODERATION_COMMANDS`, and ban/unban commands `PERMANENT_BAN`; command timeout is 10 seconds. `SaturnCommandTool.java` checks contextual capabilities before delegating to the gateway and maps rejection to `COMMAND_REJECTED`.

### 2.3 Saturn execution, admission, and result behavior

- `AgentToolCallValidator.java:18-146` applies allowed-tool filtering, contextual registry lookup, descriptor/name consistency, JSON-object parsing (blank means `{}`, `null` is invalid), schema validation, and canonical invocation-key creation. Canonical JSON sorts object keys recursively while preserving array order (`:77-120`), enabling duplicate detection independent of object-key order.
- `AgentToolExecutionPolicy.java` classifies only read-only, idempotent, prerequisite-free descriptors as `PARALLEL_READ`. Writes, non-idempotent reads, prerequisites, and actions are barriers. Resource read/write conflicts prevent a shared parallel batch; unknown resources are conservative (`AgentToolExecutionPolicy.java` symbols `classify`, `conflicts`, `compatible`).
- `AgentToolCallScheduler.java:63-95,112-169` executes contiguous parallel-safe calls concurrently, leaves sequential/dependent calls as barriers, retains provider order in returned results, and returns an immutable list. It converts null results, exceptions, interruptions, cancellation, and batch deadlines to coded `AgentToolResult` values (`:191-247`). Tests prove contiguous fan-out/order, dependent barriers, continuation after one parallel failure, immutable empty output, null-result handling, cancellation/interruption, and failure codes (`AgentToolCallSchedulerTest.java:20-231`).
- `AgentToolExecutor.java:116-233,236-285` is the higher-level boundary: validate, check disabled state, enforce prerequisites, reserve duplicate/limit state, create execution context, run middleware/invoker, validate result schema, record success/failure, notify observers, and map expected failures. It exposes `executeAll` for provider-ordered batches and closes request-local executors. `AgentToolExecutionLedger.java` is synchronized and tracks invocation keys, in-flight calls, successes, failures, disabled tools, and per-tool limits.
- `AgentToolInvoker.java` runs each tool with a deadline and cancels the future on timeout. `CancellationToken.java` is monotonic (`NONE → EXPLICIT/DEADLINE`); `AgentToolBatchContext.java` carries deadline and cancellation and reports `NONE`, `EXPLICIT`, or `DEADLINE`.
- `AgentModelVisibleToolResultRenderer.java` uses descriptor result mode to replace room-delivery results with a fixed room-delivery response while retaining errors and model-data envelopes. `AgentToolResultCoordinator.java` records attempts/success/failure, enforces required fresh-data failures, records successful tool evidence, records run-command outcomes, and appends one tool message per call in call order. This is the first direct turn-boundary dependency.

### 2.4 Saturn concrete tools and capability boundaries

- `DatabaseQueryTool.java` is named-query/read-only access through `AgentQueryRepository`; it explicitly does not accept generated SQL and points dynamic use to `DatabaseSqlTool`.
- `DatabaseSchemaTool.java` reads schema through `AgentSchemaRepository`; `DatabaseSqlTool.java` requires the dynamic-SQL capability and an `AgentSqlRepository`; `AgentSqlPolicy`/`JSqlParserAgentSqlPolicy`/`ValidatedAgentSql` define the parse/allowlist/fingerprint boundary. These tools cannot be honestly enabled until H2 repositories, SQL policy, and visibility filtering have accepted tests.
- `UserMessageHistoryTool.java` is user-scoped history and therefore must preserve room/name/trip/hash scope, PUBLIC/WHISPER visibility, and `(created_on,id)` ordering. The migration plan makes `messages.visibility` a security boundary for history, agent, and tool queries (`MIGRATION_PLAN.md:28-31,61-65,175-196`).
- `RoomUsersTool.java` and `AgentRoomDirectory` expose trusted room membership/directory state. `RunCommandTool.java`, `SaturnCommandTool.java`, and gateways are action surfaces: they must route through the existing command authorization/delivery path, not invoke command implementations directly.
- `AgentToolArgumentReader.java` accepts only JSON string primitives that are nonblank after trimming; absent, non-string, or blank values are empty. This exact small behavior should be table-tested rather than generalized into permissive coercion.

### 2.5 Saturn turn/freshness boundary

- `AgentTurnState.java:11-39,46-177,180-280` is mutable state owned by one turn/session lock, intentionally not thread-safe. It owns execution steps, tool budget reservation, correction flags, tool evidence counts, successful commands/tools, and successful result snapshots.
- `AgentExecutionState` owns step and tool limits; `AgentToolBudgetPolicy.reserve` disables tools and requests finalization without tools when reservation fails (`AgentToolBudgetPolicy.java`). This must be invoked before scheduling a batch, not after partial execution.
- `AgentFreshDataPolicy.java:15-179` requires `user_message_history` evidence for history synthesis, validates exact one-tool calls and optional target nick, rejects malformed target arguments, and rejects a repeated previous assistant answer after fresh lookup. `AgentFreshDataCoordinator`, `AgentFreshDataFinalValidator`, `AgentFreshDataTurnPolicy`, `AgentFreshnessPolicy`, `AgentMessageHistory`, `AgentToolEvidence`, `AgentTurnMemory`, `AgentTurnPolicyChain`, and `AgentUnverifiedActionPolicy` complete the pending turn rows; their tests are the acceptance source for the next turn slice.
- The turn state is per invocation, while scheduler parallelism is within one provider response. Therefore no concurrent tool worker may mutate `AgentTurnState` directly; results must be joined in provider order and applied by one owner goroutine at the turn boundary.

## 3. Current Zenbot foundations and exact gaps

- Accepted API contracts are in `internal/agent/api/api.go`, `result.go`, and `identity.go`; accepted routing/assembly explicitly uses them (`.hermes/handoffs/agent-routing-assembly-implementation.md:14-29`). Do not add a second public tool API.
- `internal/agent/runtime/contracts.go:91-145` provides private runtime `Invocation`, `Result`, `Runner`, `Sink`, and factory seams. `internal/agent/runtime/runtime.go:35-141` provides bounded admission, workers, memory-key locking, cancellation, no-reply suppression, result delivery, and idempotent close. This is an invocation runtime, not a tool scheduler.
- `internal/agent/runtime/adapters.go:9-55` explicitly converts API/runtime values and documents lossy nullable text and incompatible runtime error-code conversion. Tool results must not be forced through `runtime.Result`; retain a tool-specific internal type and translate only at the router boundary.
- `internal/agent/assemble/assemble.go:18-48,64-112,205-240` is pure and intentionally has no provider, dispatch, repository, network, or execution orchestration. It already models `ToolEvidence`, prepared tools, paired tool-call projection, deterministic fingerprints, and tool-result filtering. Tool definitions should be supplied into it through an explicit immutable slice.
- `internal/agent/sql/policy.go:9-30` currently validates PostgreSQL-parsed SQL and rejects a small set of write prefixes when `AllowWrite` is false. This is not yet evidence of Saturn parser/allowlist parity: it lacks the Saturn named repository/visibility/fingerprint boundary and should remain blocked for dynamic SQL activation.
- `internal/agent/persistence/schema.go:3-11` is only a structural `Column/Table/Schema` model. It is not an H2 repository, schema inspector, memory store, or query executor. Do not implement database tools against this placeholder.
- `internal/agent/moderation/action.go:3-17` defines action names and an `ActionExecutor`, but no engine-backed action execution. Do not activate moderation or command mutation tools on this interface alone.
- `internal/listener/message/handlers.go:128-155` has an `AgentParticipation` no-op and dispatches normal commands later in the chain. The accepted QA explicitly says live listener/engine autonomous-agent wiring remains excluded (`agent-routing-assembly-qa.md:83-89`).
- There is no current target `internal/agent/tool`, `tool/contract`, `tool/execution`, or `agent/turn` implementation. The target has no production caller that constructs and runs tool batches. This makes pure contracts/execution and test doubles implementable, but concrete repository/action tools and live turn integration blocked.

## 4. Proposed target architecture

### 4.1 Package/file map

```text
internal/agent/tool/
  tool.go                 # Tool interface and immutable descriptor boundary
  argument_reader.go      # nonblank string and strict JSON argument helpers
  room_directory.go       # interface only; engine adapter later
  command_gateway.go      # interface only; command adapter later
  database_tools.go       # concrete tools only after repository seams accepted
  history_tool.go         # visibility-scoped history adapter

internal/agent/tool/contract/
  schema.go               # schema constructors and deep-copy helpers
  validator.go            # parameter/result/schema validation
  definition.go           # provider-neutral definition DTO
  definition_json.go      # deterministic serialization

internal/agent/tool/execution/
  call.go                 # parsed/canonical validated call
  registry.go             # frozen contextual registry
  policy.go               # access/effect/resource classification
  ledger.go               # request-local duplicate/limit/failure state
  scheduler.go            # contiguous parallel-read batches
  executor.go             # validation, admission, timeout, result validation
  cancellation.go         # batch token/deadline
  result.go               # model-visible envelope/rendering/coordinator

internal/agent/turn/
  state.go                # one-owner turn state and evidence
  policy.go               # step/tool/freshness policy chain
  freshness.go            # fresh-history requirement and final validation

internal/agent/runtime/
  tool_runner.go          # runner adapter: one invocation owns one turn/tool lifecycle
  runtime.go              # existing bounded invocation admission remains owner
```

### 4.2 Interfaces

```go
// Tool is the target equivalent of Saturn AgentTool.
type Tool interface {
    Name() string
    Descriptor(ctx api.Context) (Descriptor, error)
    Execute(ctx api.Context, args json.RawMessage) (ToolResult, error)
}

type Descriptor struct {
    Name, Label, Description, Category string
    Access Access
    Effect Effect
    ResultMode ResultMode
    Parameters json.RawMessage
    RequiredCapabilities, RequiredSuccessfulTools []string
    Idempotent bool
    Timeout time.Duration
    ResultSchema json.RawMessage
    ResourceReads, ResourceWrites []string
}

type ToolRegistry interface {
    Find(ctx api.Context, name string) (Tool, bool)
    Definitions(ctx api.Context) []Definition
}

type ToolExecutor interface {
    Execute(ctx context.Context, agent api.Context, calls []llm.ToolCall, batch Batch) []ToolResult
}

type TurnBoundary interface {
    Begin(invocation runtime.Invocation) (*turn.State, error)
    ApplyToolResults(state *turn.State, calls []llm.ToolCall, results []ToolResult) error
    Finalize(state *turn.State, response llm.Response) (turn.Decision, error)
}
```

`Descriptor` and `ToolResult` must be immutable-by-convention at package boundaries: deep-copy JSON on ingress/egress, return copied slices, and keep mutable ledger/turn state request-local. `Execute` may return expected tool errors as data; unexpected infrastructure failures are converted by the executor to stable codes and logged without exposing exception text.

### 4.3 End-to-end sequence

```text
accepted API Invocation
  -> router/turn owner creates turn.State and batch deadline
  -> assembler receives immutable tool definitions
  -> provider returns ordered ToolCalls
  -> call validator: allowed tool -> registry/context -> JSON object -> schema
  -> canonical invocation key + ledger reservation + prerequisite check
  -> execution policy classifies each descriptor
       contiguous independent reads: parallel batch
       dependent read/action: sequential barrier
  -> executor runs with deadline/cancellation/middleware
  -> result schema validation + ledger outcome
  -> scheduler joins in provider order
  -> turn owner applies evidence/freshness/prerequisite state
  -> result renderer appends model-visible tool messages
  -> next provider/final-synthesis step, or explicit failure/no-reply decision
  -> runtime Sink only delivers reply-required final result
```

No listener should call a tool directly, and no tool should call the provider or mutate turn state. Command/database/moderation adapters remain capability-controlled seams.

## 5. Security and capability boundaries

1. **Context is authority.** Registry availability and command/database access derive from trusted `api.Context` capabilities and target identity; never from tool arguments, prompt prose, model claims, or a command string.
2. **Two gates for every call.** First the invocation-level allowlist (`TOOL_NOT_ALLOWED`), then descriptor availability/capability (`TOOL_NOT_AUTHORIZED`). Unknown names are `UNKNOWN_TOOL`; malformed descriptors are `INVALID_TOOL_CONTRACT`.
3. **Schema is a security boundary.** Parse only JSON objects; reject unknown closed-schema fields, wrong primitive/structured types, enum/length/bounds violations. Do not coerce strings to numbers/bools.
4. **SQL is deny-by-default.** `database_query` uses named, read-only repository methods. `database_sql` requires `DYNAMIC_SQL` plus Saturn-equivalent parser/allowlist/fingerprint and a read-only connection unless a separately accepted admin contract exists. All message/history queries retain visibility, scope, and tie ordering.
5. **Actions are barriers and authorization adapters.** `run_command`, Saturn command tools, and moderation actions never bypass command gateway/service authorization. `PERMANENT_BAN`, `ADMIN_COMMANDS`, and moderation capability checks must be explicit and tested.
6. **No sensitive error leakage.** Model-visible errors contain stable code and safe message, not SQL text, stack traces, credentials, or internal exception messages. Logs may retain correlation/tool identifiers but should avoid raw arguments.
7. **Immutable definitions/results.** Deep-copy schemas and JSON values to prevent a tool/provider from mutating the registry or a prior turn’s evidence.
8. **Visibility-safe history.** `UserMessageHistoryTool` cannot use a generic repository method that omits PUBLIC/WHISPER filtering; blocked until the H2 query contract is proven.

## 6. Concurrency, admission, cancellation, and serialization

- **Admission:** existing runtime bounds accepted invocations (`ErrBusy`, `ErrClosed`); tool execution adds request-local call-count and per-tool budgets. Tool batch admission must fail before starting calls when the turn budget cannot reserve the complete requested batch, matching `AgentToolBudgetPolicy`.
- **Parallelism:** only contiguous `PARALLEL_READ` calls may fan out. Resource conflicts, unknown resource metadata, non-idempotence, prerequisites, and every action create barriers. Returned result order is exactly provider order even when completion order differs.
- **Turn ownership:** parallel workers may produce results but may not mutate `turn.State`, freshness flags, or ledger state unsafely. Join results, then apply state transitions serially on the session/memory-key owner.
- **Session serialization:** retain accepted runtime `Context.MemoryKey()` locking, not room-only locking. Public and whisper contexts must not share a turn state. Distinct memory keys may execute concurrently.
- **Cancellation:** batch cancellation and deadline must stop not-yet-started calls, cancel futures, restore interruption state where applicable, and return `TOOL_BATCH_CANCELLED` or `TOOL_BATCH_DEADLINE`. Individual descriptor timeout returns `TOOL_TIMEOUT`. Cancellation is not success and cannot satisfy freshness/prerequisites.
- **Shutdown:** runtime close prevents new invocation admission and cancels accepted work. Tool executor close must stop owned workers; no result should be delivered after runtime cancellation.
- **Deterministic serialization:** canonical invocation keys sort object keys recursively and preserve array order. Definition lists and command catalog entries are sorted by stable tool name. Result arrays preserve call order. JSON envelopes have fixed status/data/error fields. Do not assert byte-for-byte provider JSON parity absent Saturn serialization golden tests.

## 7. Error policy

| Condition | Code / behavior |
|---|---|
| Tool not in invocation allowlist | `TOOL_NOT_ALLOWED` |
| Registry miss | `UNKNOWN_TOOL` |
| Descriptor construction/name/schema failure | `INVALID_TOOL_CONTRACT` |
| Invalid JSON/object/schema argument | `INVALID_ARGUMENTS` |
| Capability failure | `TOOL_NOT_AUTHORIZED` |
| Required successful tool absent | `MISSING_PREREQUISITE` |
| Canonical duplicate | `DUPLICATE_TOOL_CALL` |
| Per-tool limit | `TOOL_CALL_LIMIT_REACHED` |
| Repeated tool failures | `TOOL_DISABLED` |
| Tool returned nil/no result | `TOOL_EXECUTION_FAILED` |
| Descriptor/result schema mismatch | `INVALID_TOOL_RESULT` |
| Descriptor deadline | `TOOL_TIMEOUT` |
| Batch deadline/cancel | `TOOL_BATCH_DEADLINE` / `TOOL_BATCH_CANCELLED` |
| Interrupted execution/wait | `TOOL_INTERRUPTED`, restore interrupt flag |
| Command gateway rejection | `COMMAND_REJECTED` |
| Required fresh tool fails | turn-boundary error; do not synthesize as fresh |

Expected errors are data and remain in provider order. Unexpected errors are logged with correlation ID/tool/call ID and normalized to `TOOL_EXECUTION_FAILED`. A failed result increments failure accounting; it does not satisfy a prerequisite or fresh-data requirement. Any model-visible room-delivery acknowledgement is emitted only according to descriptor `ResultMode` and only after successful execution.

## 8. Staged migration strategy

### Stage A — pure contracts and schema (implementable now)

Port rows #104–#107 and the API-facing portions of #91–#92. Add immutable Go descriptor/definition/result types, schema constructors, strict validator, canonical JSON, argument reader, and focused tests. Keep provider serialization provider-neutral. Do not add concrete DB/command behavior.

### Stage B — registry, ledger, policy, scheduler (implementable now)

Port rows #109–#127 except integrations that require unavailable turn/persistence types. Freeze contextual registries, classify resource-safe reads, enforce duplicate/limit/failure budgets, run contiguous parallel batches, normalize cancellation/timeouts/errors, and validate result schemas. Add deterministic fake tools and race tests.

### Stage C — minimum turn boundary (small dependency slice)

Introduce a target `turn.State` carrying attempted/success/failure evidence, successful tool names/results, tool-enabled state, step/tool reservations, and cancellation/deadline ownership. Apply results serially after scheduler join. Add freshness interfaces with explicit `RequiredFreshTool`/nick metadata, but do not claim full Saturn freshness until rows #128–#143 and their focused tests are ported.

### Stage D — concrete read-only tools after H2/SQL acceptance

Only after rows #43–#55 and #86–#90 have named H2 methods and real-H2 tests, implement `RoomUsersTool`, `UserMessageHistoryTool`, `DatabaseQueryTool`, `DatabaseSchemaTool`, and `DatabaseSqlTool`. Verify visibility, room/name/trip/hash scope, ordering, limits, schema metadata, read-only policy, generated errors, and transaction behavior. Keep dynamic SQL disabled until policy acceptance.

### Stage E — command gateway/action tools after command/moderation acceptance

After command catalog/factory, services, moderation, and listener boundaries are accepted, implement `SaturnCommandGateway`, `EngineSaturnCommandGateway`, `SaturnCommandToolCatalog`, `SaturnCommandTool`, `RunCommandTool`, and moderation action adapters. Preserve command authorization, aliases, exact output, side effects, transaction boundaries, and action ordering. Actions remain unregistered until end-to-end authorization/delivery tests pass.

### Stage F — router/assembler integration and live caller last

Extend the accepted assembler input with frozen definitions and tool evidence; route provider tool calls through the executor and turn boundary. Keep the runtime `Runner`/`Sink` seam and memory-key lock. Only after listener ordering, persistence, moderation, provider, lifecycle, and cancellation tests pass may the agent participation handler be wired; until then, `AgentParticipation` remains intentionally unwired.

## 9. Acceptance tests

### Contract/schema

- Descriptor rejects blank/invalid names, empty negative-use guidance, invalid timeout, malformed parameter/result schemas, invalid required declarations, and bad examples; defensive copies are proven.
- Validator reproduces Saturn test vectors for object roots, closed objects, required fields, unknown fields, primitive/structured types, enum, code-point lengths, integer/bounds, result types, and required result fields.
- Canonical invocation keys deduplicate equivalent object key order but not array order; definition/catalog ordering is stable.
- Success/error envelopes have exact status/data/error shape, parse JSON success data, stringify invalid JSON success content, and reject invalid envelopes.

### Executor/scheduler

- Unknown, disallowed, unavailable, malformed, duplicate, prerequisite, limit, disabled, result-schema, command-rejected, timeout, interrupted, cancellation, deadline, null-result, and thrown-exception paths return the table’s stable codes.
- Only contiguous independent idempotent reads fan out; actions/dependent reads/resource conflicts are barriers; results preserve provider order; one parallel failure does not suppress a sibling result.
- Per-tool limits and repeated-failure disabling are request-local, synchronized, and race-clean. Closing an executor cancels owned work.
- Middleware/observer hooks cannot bypass validation or invoke a tool twice; continuation is one-shot and errors are normalized.

### Turn boundary

- Budget reservation happens before any call starts; exhausted budget disables tools and causes explicit no-tool finalization.
- Scheduler results are applied to turn state exactly once, in provider order; parallel workers never race turn-state mutation.
- Successful tools satisfy prerequisites; errors do not. Tool evidence counts attempted/successful/failed calls and is immutable when projected into assembly.
- Required fresh history checks exact tool name and target nick; malformed/mismatched calls fail closed; failed history cannot produce fresh synthesis; repeated previous assistant output is rejected where Saturn tests require it.
- A public and whisper invocation with the same room has separate turn state; same `MemoryKey` serializes while distinct keys can overlap.

### Integrations and gates

- Real H2 tests prove named-query/schema/history tools preserve visibility and ordering; SQL policy rejects writes and out-of-scope objects.
- Command/moderation integration proves capability thresholds, protected principals, exact gateway output, action barriers, and no direct command bypass.
- Live caller tests are deferred until listener/lifecycle rows are accepted; then prove no duplicate submissions, command short-circuit preservation, final-only delivery, cancellation on shutdown, and ambient behavior.
- Required verification for an implementation pass: focused tool/turn tests, `go test -race ./internal/agent/...`, `go test ./...`, `go vet ./...`, `go build ./...`, `git diff --check`, and dirty-worktree comparison. Baseline green alone is not migration evidence (`MIGRATION_PLAN.md:153-175`).

## 10. Exact exclusions and unsupported claims

- No application implementation is included in this architecture handoff; no Saturn source is modified.
- No closure of API rows #3–#23, routing/participation/assembly rows #56–#85, or accepted row #300; those are consumed as existing boundaries.
- No full `agent.turn`/freshness parity claim: rows #128–#143 remain pending until all policy/coordinator/final-validator behavior is ported and tested.
- No H2 repository, agent memory, message-history, schema, SQL, DBZ, or persistence claim from the current placeholders.
- No moderation action, command implementation, listener chain, engine lifecycle, transport, ambient production activation, or live `l` command claim.
- No provider-specific wire/JSON parity claim for tool definitions/results without Saturn golden serialization evidence.
- No arbitrary DAG scheduler claim: Saturn implements contiguous parallel batches plus barriers, not general dependency scheduling.
- No new capability, permission, command, schema table/index, or product behavior is proposed beyond the audited Saturn scope (`MIGRATION_PLAN.md:177-185`).
- No SQLite compatibility or alternate persistence abstraction is introduced; H2 remains the executing database requirement.

## 11. Complexity and risk

**Complexity: high-medium.** Pure schema/descriptor/validator work is medium and relatively isolated. Scheduler/executor work is high because duplicate accounting, deadlines, result ordering, resource conflicts, and cancellation interact. Concrete tools and the turn boundary are high-risk due to H2 visibility/security, mutable one-owner state, and deferred listener/moderation dependencies.

Primary risks are: leaking whisper/public history through a generic repository; treating `isIdempotent` as permission; allowing unknown-resource tools into a parallel batch; mutating turn state from workers; satisfying freshness with failed or stale evidence; mapping tool errors through runtime’s incompatible `Result`; exposing command/admin actions before gateway authorization; and activating a partial router that appears functional while silently skipping persistence, freshness, or moderation. The staged gates above are intended to prevent each false-parity mode.

## 12. Verification of this handoff

This file is the only file written by this analysis. The source paths and handoff references cited above were inspected in the target or read-only Saturn checkout. The target application source was not modified; the pre-existing dirty/untracked worktree was preserved. A final independent structural/source check must confirm this artifact is non-empty, headings are present, selected audit rows match `.hermes/migration-audit.md`, and every cited path resolves before this handoff is treated as complete.
