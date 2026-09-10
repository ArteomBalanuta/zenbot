# Agent Harness Hardening Design

## Objective

Harden Zenbot's agentic harness into one request-local, bounded state machine that validates every provider and tool boundary, preserves correct ordering, supports actionable self-correction, and keeps long-running conversations useful without placing untrusted data in trusted prompt roles.

The refactor preserves the existing command catalog, tool implementations, public `*l` behavior, shared public-room memory, private whisper memory, and OpenAI-compatible provider API.

## Current Defects

1. `execution.ExecuteAll` checks every parallel candidate only against the first descriptor. Two later calls that conflict with each other can execute concurrently.
2. Registry-loop protocol validation does not require unique tool-call IDs and does not bind a successful `contract.Result.CallID` to the originating call.
3. Schema validation is shallow. Nested object fields, array items, nested `required`, and nested `additionalProperties` constraints are not enforced.
4. A tool call on the final configured step can execute successfully and then produce a generic failure because no synthesis step remains.
5. Blank or malformed provider calls can terminate the entire loop instead of becoming structured observations when a usable call ID exists.
6. Fresh-history and rendered-command correction use separate execution paths with different budget and recovery semantics.
7. Recent-room JSON is truncated as raw text and can become invalid JSON. The fixed system message cannot be pruned, and `Projection.Overflow` is not enforced.
8. Untrusted room messages and historical tool evidence are embedded inside the system-role message.
9. Raw memory, recent room messages, and historical evidence can repeat the same information without source-aware deduplication.
10. Tool-only room deliveries are omitted from durable conversation memory, so subsequent turns can lose the action context.
11. Conversation and tool evidence writes are separate operations, allowing partial turn persistence.
12. Runtime keyed locks are retained forever. There is no request-level execution deadline or action-interrupt boundary.

## Architecture

### Turn Engine

`live.TurnEngine` becomes the only generalized tool-loop implementation. A fresh `turn.State` owns one invocation and advances through explicit phases:

```text
ASSEMBLE -> MODEL -> PLAN -> GATE -> EXECUTE -> OBSERVE -> REFLECT -> FINALIZE
```

The engine never stores turn state on a shared service. Its collaborators are immutable composition-time dependencies:

- `assemble.RequestAssembler` creates a trusted policy message and separate untrusted context messages.
- `execution.BatchPlanner` validates provider call identity and creates ordered execution stages.
- `execution.Executor` validates one call and always returns a call-bound result envelope for non-cancellation failures.
- `turn.RecoveryPolicy` classifies observations and decides whether tools remain available, reflection is allowed, or terminal synthesis is required.
- `turn.InterruptHook` evaluates validated action calls before execution.
- `turn.FinalResponseValidator` validates terminal content and permits one bounded correction.

`ToolLoop` remains as a compatibility facade delegating to `TurnEngine` until external constructors are migrated.

### Budgets

`ExecutionLimits` and runtime configuration have independent bounds:

- `MaxSteps`: provider/tool cycles in one turn.
- `MaxToolCalls`: aggregate accepted calls.
- `MaxCallsPerTool`: accepted calls by tool name.
- `MaxToolFailures`: failed executions by tool name.
- terminal reflection: exactly one correction attempt enforced by `TurnEngine`.
- `ToolTimeout`: default tool deadline.
- `RequestTimeout`: deadline covering context load, all provider calls, tools, finalization, and persistence.

Terminal synthesis is reserved independently from tool rounds. When the tool-round budget is exhausted, tools are removed and the model receives accumulated observations for one final answer. No successful observation is discarded solely because another tool call was attempted at the boundary.

### Tool Protocol

Every provider batch must contain nonblank, unique call IDs. Calls with usable IDs but unknown names, invalid arguments, missing capabilities, duplicate semantics, exhausted budgets, or failed prerequisites produce standard error observations. A blank or repeated call ID is a provider protocol violation because observations cannot be mapped unambiguously.

The executor owns result identity. It overwrites tool-returned `CallID` and `ToolName` with the validated originating call identity before constructing the observation. Tool implementations cannot corrupt transcript routing accidentally.

`BatchPlanner` computes stages in provider order. A stage may contain multiple calls only when every call is read-only, idempotent, prerequisite-free, and pairwise resource-compatible with every other call in that stage. Actions always create sequential stages.

### Recursive Contract Validation

The supported schema subset is recursively validated and executed for:

- `type`, `enum`, `required`, and `additionalProperties`
- nested `properties`
- array `items`, `minItems`, and `maxItems`
- string `minLength` and `maxLength`
- numeric `minimum` and `maximum`

Unsupported keywords fail descriptor construction. Argument and result validation use the same recursive validator so advertised and enforced contracts cannot drift.

### Reflection And Recovery

Tool failures remain observations, not Go errors, unless the parent context is cancelled or the transcript itself cannot be mapped safely. Each error envelope includes a stable code and concise model-visible message; recursive validation messages include the failing path. The model receives the same authorized manifest while correction remains possible.

After tools are disabled or the model returns terminal content, `FinalResponseValidator` verifies:

- nonempty content for reply-required modes
- no unresolved tool calls
- fresh-data requirements are backed by a successful current-turn result

One reflection call may correct an invalid terminal answer. Reflection receives a concise structured critique, not hidden chain-of-thought.

### Human-In-The-Loop Hook

`InterruptHook.Review(ctx, ActionRequest)` returns `Allow`, `Deny`, or `Pause`. The default hook allows all currently authorized behavior, preserving existing UX. Deployments may require approval for selected action tools without changing tool implementations.

`Pause` returns a typed `PausedError` containing the pending validated action and opaque resume token. `TurnEngine.ResumeAction` verifies that token, validates pending-call identity, and executes through a fresh executor using the current caller context. Registry membership, argument schema, and capabilities are therefore revalidated immediately before the resumed side effect. Checkpoint persistence and an operator-facing approval transport remain deployment concerns outside the default-allow harness.

### Context And Memory

Provider messages are assembled in authority order:

1. Trusted system policy without chat rows, historical evidence, hashes, trips, or the configured creator identity.
2. Optional compacted conversation summary marked as untrusted memory.
3. Durable conversation turns.
4. Historical tool evidence in a separate untrusted context message.
5. Recent room rows in a separate untrusted context message.
6. The newest contextualized user request as the final message.

`ContextBudgeter` operates on complete semantic units and never slices serialized JSON. The configurable context ceiling supports up to 1,000,000 estimated tokens. It reserves space for trusted policy, the newest request, tool manifests, and terminal synthesis, then admits fresh observations, recent turns, room rows, historical evidence, and summaries in descending priority.

When durable history exceeds the raw-turn threshold, `MemoryCompactor` summarizes the oldest contiguous turns into a structured summary with source range, creation time, and fingerprint. Summaries are untrusted memory, never policy. Existing raw rows remain authoritative in storage until normal TTL cleanup; only the prompt projection is compacted.

Source fingerprints deduplicate repeated memory, room-context, and historical-evidence units before budgeting.

### Persistence And Runtime

One completed turn produces an `AgentTurnRecord` containing user input, assistant outcome, reusable evidence, and timestamps. Repositories persist it atomically after successful visible delivery or confirmed tool-owned room delivery. Delivery ownership and correlation remain request-local runtime metadata.

Runtime execution uses a request deadline derived from configuration. Reference-counted keyed locks are removed after the last waiter exits. Shutdown cancellation is distinguished from ordinary execution failure; tests synchronize on execution completion rather than internal finalizer entry.

## Security Boundaries

- Authorization remains capability-derived and is rechecked immediately before execution.
- Untrusted content never occupies the system role.
- Provider-visible metadata excludes raw trip, hash, creator trip, API keys, SQL text, and internal error causes.
- Dynamic SQL remains parser-validated, read-only, schema-bound, row-limited, and deadline-bound.
- Tool observations expose stable correction data without stack traces, credentials, or repository errors.
- HITL decisions cannot add capabilities and are revalidated on resume.

## Verification

The refactor requires:

- unit tests for recursive schemas, unique identity, result rebinding, pairwise batch conflicts, budgets, terminal synthesis, reflection, and HITL decisions
- integration tests for invalid-argument self-correction, failed-tool degradation, dependent tool chains, mixed action/read ordering, and tool-only memory continuation
- context tests proving JSON validity, role separation, deterministic pruning, deduplication, and 1M-token configuration bounds
- persistence tests proving atomic turn writes
- runtime tests proving request deadlines, keyed-lock reclamation, cancellation, and clean shutdown
- fuzz tests for provider response decoding and schema validation
- `go test -race ./internal/agent/...`, `go vet ./...`, `go test ./...`, and `make check`
