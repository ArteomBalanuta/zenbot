# Agent internals

The optional agent is one iterative LLM/tool loop. The model interprets requests, chooses calls and their order, reacts to observations, and decides when to answer. Zenbot enforces trusted identity, tool contracts, execution permissions, budgets, receipts, and delivery. There is no preliminary semantic planner, runtime-generated task checklist, or second model judging whether the requested work is complete.

Start with [configuration](configuration.md) to enable the agent and [commands](commands.md) for the chat interface. [`newLiveAgent` and `newAgentToolLoop`](../cmd/zenbot/main.go) are the production composition points.

## Admission and identity

Direct `l` requests use [`DirectSubmissionAdapter`](../internal/agent/live/direct_submission.go) and the process-owned runtime. Public mentions and configured ambient participation enter through [`participation.Pipeline`](../internal/agent/participation/invocation.go). Autonomous moderation uses a separately constructed bot/system principal bound to a reviewed target. The legacy synchronous `DirectInvoker` exists in source but is not the production direct-command submission route.

| Invocation | Reply policy | Admission and authority |
| --- | --- | --- |
| `DIRECT` | Required | Explicit command; creator-only admin/permanent-ban capabilities are possible here |
| `MENTION` | Required | Exact public bot mention; capabilities derive from the trusted snapshot |
| `AMBIENT` | Optional | Configured public sampling, with participation filtering and quiet policy |
| `MODERATION` | Silent | Bot/system principal with moderation capability and a fixed reviewed target |

[`InvocationFactory`](../internal/agent/participation/invocation.go) copies room identity and derives capabilities from the configured creator, admin trips, and persisted roles. Ordinary admins receive dynamic SQL and moderation capabilities outside ambient/moderation modes; moderators receive moderation capability on that same basis. The creator branch also grants dynamic SQL and moderation capabilities independently of mode, with admin/permanent-ban capabilities restricted to direct requests. Tool and gateway restrictions still apply. Prompt claims of status do not affect these values.

Whisper invocations expose no tools. Public participation ignores whispers; a private direct request remains a conversational invocation. Public turns share a room memory key, while whisper keys include a separate private namespace and identity chosen from trip, hash, then nick. See [`runtime/contracts.go`](../internal/agent/runtime/contracts.go).

[`runtime/runtime.go`](../internal/agent/runtime/runtime.go) bounds admission, applies a whole-request context, orders turns sharing a memory key, and coalesces ambient work. Locks and execution state are request resources rather than a permanent per-user worker.

## Model/tool protocol

[`live/tool_loop.go`](../internal/agent/live/tool_loop.go) assembles each invocation with a fresh ledger, observation store, transcript, and `read_tool_result`. [`TurnEngine`](../internal/agent/live/turn_engine.go) runs the cycle:

1. Build the caller-visible manifest and budget the initial context.
2. Send the model the current request, context, and eligible native function tools.
3. Accept a structurally valid response without calls as a final candidate, or validate the returned call batch.
4. Reserve call budget and check identity, manifest access, capabilities, argument schemas, prerequisites, and execution ledger state.
5. Execute admitted calls and append one assistant call message with its matching tool observations in original call order.
6. Reproject context and let the same model continue with the results.
7. At the round or call bound, request tool-free terminal synthesis. A bounded structural repair allowance handles invalid final/protocol content; it does not judge semantic completeness.

Call IDs and names must be nonblank, and IDs must be unique across the turn. A malformed/truncated call response executes no calls from that response. Original argument JSON survives validation and transcript serialization; malformed arguments are not silently replaced by an empty object.

Compatible declared-safe reads can run concurrently. Actions and undeclared resource access are barriers in provider order. Dependent or conditional work needs a later model round after its input is known; the executor does not infer dependencies from prose. If an ordered action fails, later actions in the batch receive `ACTION_NOT_EXECUTED`, linked to the failed call, while eligible reads may still run.

The [`OpenAI-compatible client`](../internal/agent/llm/openai/client.go) calls the configured endpoint's `/v1/chat/completions` route. It supplies the model, token limit, tools, and thinking configuration, preserves assistant/tool pairing, and retries eligible transient provider failures within the operation timeout. Provider HTTP retries are distinct from replaying a tool action.

## Execution, receipts, and cancellation

[`tool/execution`](../internal/agent/tool/execution/execution.go) validates calls and [`ledger.go`](../internal/agent/tool/execution/ledger.go) retains per-invocation attempt/failure counts, successful prerequisites, and duplicate state. Validation failures do not spend the invoked-failure budget. Useful coded observations allow the model to repair arguments or choose an eligible alternative.

Effects have explicit states: `NOT_STARTED`, `NOT_COMMITTED`, `COMMITTED`, `PARTIAL`, and `UNKNOWN`. `effectsCommitted`, `actionCount`, and `deliveryCount` preserve positive evidence even when an error occurs. An error is not evidence of rollback. Known failed reads and known uncommitted actions may be retried where contract and budgets permit; committed or uncertain actions must not be replayed as a recovery shortcut.

An unknown action blocks further actions for the invocation. Read-only tools remain available for investigation, but reading does not automatically clear the block. Duplicate protection is local to a turn; persisted receipts are not a cross-request idempotency guarantee.

Cancellation is cooperative. A timed-out read can return while its handler finishes in the background. Actions execute synchronously so the executor does not return while its own in-process action is still running. An implementation that ignores context can therefore exceed the configured deadline. An action lacking a verified terminal state becomes `ACTION_OUTCOME_UNKNOWN`; the runtime cannot promise rollback or safely infer that retrying will do nothing twice.

Command tools use [`command/agent_gateway.go`](../internal/command/agent_gateway.go). It reconstructs a normal command under the trusted caller, resolves the current master, enforces authorization and moderation targeting, captures output, and returns typed execution receipts. Room-output commands require a delivery receipt; silent mutations require an action receipt. Partial output survives failures. A receipt describes the application's observed execution boundary, not a guarantee of remote human acknowledgement.

## Tool discovery and contracts

The runtime builds a deterministic, versioned manifest from [`tool.Registry`](../internal/agent/tool/tool.go). Only allowed, authorized tools with the caller's capabilities are visible. Invalid descriptors fail construction; internal fallbacks stay hidden and primary intents cannot collide. There is no external plugin discovery or model-created tool installation in this path.

All tools implement:

```go
type Tool interface {
    Name() string
    Descriptor(api.Context) (contract.Descriptor, error)
    Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error)
}
```

[`contract`](../internal/agent/tool/contract/definition.go) descriptors declare purpose, selection guidance, parameter/result schemas, access and capability requirements, read/action effect, delivery mode, idempotency, timeout, prerequisites, and resource reads/writes. The provider projection includes concise selection and result-shape guidance. Local schema checks remain authoritative. Provider strict mode is used only when the parameter schema supports it unchanged, so optional fields preserve omission semantics.

| Tool | Source and scope |
| --- | --- |
| `room_users` | Current room directory; no room argument |
| `user_message_history` | Latest public messages for a named user, optionally restricted to a room; bounded to 500 |
| `saturn_<canonical>` | Actionable catalog command with named JSON arguments and a trusted encoder |
| `database_query` | Fixed named read queries; composed only when agent SQL is enabled |
| `database_schema` | Application schema discovery; SQL enabled and `DYNAMIC_SQL` required |
| `database_sql` | Validated bounded SQL; same capability plus a successful schema prerequisite |
| `read_tool_result` | Original results/arguments from this invocation only; no re-execution |

Current-room presence belongs to `room_users`; remote `saturn_list` rejects the current room and returns its source-owned typed roster. Other command tools advertise their actual output/receipt schemas, not a generic roster schema.

[`internal/command/catalog`](../internal/command/catalog/catalog.go) is the command inventory. [`agent_contract.go`](../internal/command/catalog/agent_contract.go) defines named-field schemas and encoders into existing command grammar, with local cross-field validation and bounded command tails. The model does not author arbitrary chat command strings through the production command adapters.

## Context and retained observations

[`assemble`](../internal/agent/assemble/assemble.go) combines policy resources, immutable invocation metadata, the newest prompt, durable conversation memory, recent public room context, historical evidence, and live tool definitions. [`context.go`](../internal/agent/assemble/context.go) budgets the initial request and each continuation, repair, and terminal synthesis.

Token accounting is estimated. The live tool manifest counts toward the budget, output capacity is reserved, and lower-priority/older context can be pruned. An assistant tool-call message and its matching results form an atomic unit. Trusted policy, the newest request, current runtime status/feedback, and a bounded factual result index are mandatory; inability to fit required metadata fails assembly rather than silently dropping execution receipts.

The index records call IDs, tool names, bounded argument/data previews, status, error codes, and effect receipts. It is untrusted evidence, not instructions or a derived task plan. Large previews can lose detail even when receipt metadata remains.

[`ObservationStore`](../internal/agent/assemble/observations.go) retains full typed results and original argument JSON under their call IDs. Provider observations are bounded structured JSON with truncation/count metadata. Arrays and objects retain selected facts; strings are Unicode-aware. Invalid successful result payloads become `INVALID_TOOL_RESULT` observations while the original is retained.

[`read_tool_result`](../internal/agent/live/observation_tool.go) pages this retained evidence:

| Argument | Meaning |
| --- | --- |
| `callId` | Original call ID in the current invocation |
| `field` | `content` (default) or `arguments` |
| `offset` | Unicode rune offset, default zero |
| `limit` | Requested runes, 1–6000, default 6000 |

A page returns `content`, `nextOffset`, `totalRunes`, `done`, and the original receipt. The complete page is byte-bounded and may contain fewer runes than requested; follow `nextOffset`. A content chunk need not independently parse as JSON. Retrieval consumes the total call budget. Retention does not provide unlimited model access, and terminal synthesis has no tools with which to retrieve omitted details.

## Memory, privacy, and SQL

[`TurnMemory`](../internal/agent/turn/memory.go) and [`PersistentMemoryStore`](../internal/agent/live/memory.go) load bounded history by memory key, turn count, and TTL. Older complete user/assistant pairs can be summarized by a tool-free model call. H2 persists the summary, source fingerprint, and covered row ID; raw rows remain authoritative until TTL cleanup.

Production appends conversation, outcome, and eligible read evidence in a single [`AppendAgentTurn`](../internal/repository/agent_turn.go) transaction. Only successful model-data results whose durable schemas validate become reusable evidence. Room deliveries and unknown actions do not become reusable read facts. Historical evidence is labeled historical and must not be mistaken for live presence.

Recent-room and user-history queries explicitly select `PUBLIC` messages. Whispers have separate visibility and conversation keys. Legacy null visibility is classified as public by the compatibility upgrade. These controls limit context selection; they do not mean data stays on the host. The configured model provider receives the assembled request, including selected history, identity metadata, and tool data; memory summarization also calls that provider.

Dynamic SQL has a broader privilege boundary than public-message retrieval. [`agent_schema.go`](../internal/repository/h2/agent_schema.go) discovers application base tables in H2's `PUBLIC` schema. That name is a SQL namespace, not a filter for public messages: capability holders can query application tables admitted by the discovered schema, potentially including private application data. Enable and grant this capability accordingly.

[`sql/policy.go`](../internal/agent/sql/policy.go) parses exactly one read-only `SELECT`, validates tables against discovered schema, and restricts functions and statement forms. Writes, locking selects, and server-internal tables are rejected. Execution is additionally bounded by timeout, SQL length, rows, columns, cell size, and result size. Parser compatibility does not imply that every accepted SQL expression will execute on H2; failures are returned as coded observations. The ordinary privileged chat SQL service is a separate boundary from these agent SQL tools.

Agent structured events avoid raw prompt/result bodies, but the overall application also has legacy logging paths such as [`CoreListener.Notify`](../internal/listener/core_listener.go), which logs incoming payloads. Do not treat process logs as privacy-filtered agent context.

## Final delivery and interrupted turns

The model chooses its final answer and can use the configured no-reply marker when permitted. For reply-required turns, suppression requires verified tool-owned room delivery and is rejected after unknown actions or when committed silent actions need confirmation. This checks delivery evidence; it does not prove that every part of a compound task was completed.

[`OutputFinalizer`](../internal/agent/live/runner.go) checks required nonempty content, internal evidence/protocol leakage, formatting, and Unicode output bounds. Runtime delivers through the current room/whisper sink, and `AfterDelivery` persists after visible delivery or verified tool-owned output.

If a later provider call, context projection, or finalization fails after tools ran, the partial completion retains receipts and valid evidence in an `IncompleteTurnError`. The runner attempts a bounded persistence of an interrupted-turn record and eligible reads. The interruption record is untrusted metadata, with call identity/status/effect counts rather than raw action bodies. Cancellation permits a separate short persistence attempt. This is a recovery record, not an automatic resume mechanism.

[`FailureReply`](../internal/agent/live/failure_reply.go) can report that the answer is incomplete and distinguish success, not-started/failure, partial, and unknown outcomes using receipts. It excludes raw arguments/results and does not suggest replaying the task. Cancellation follows the no-send path. See [operations](operations.md) for diagnosis and logs.

## Extending and verifying the agent

For a command-backed tool, update the command catalog and its typed argument contract together with the command behavior. For a standalone tool, implement `Tool`, provide an accurate descriptor/result schema, and register it in production composition. Declare reads/writes and idempotency conservatively: they affect concurrency and retry behavior. Preserve action/delivery receipts on error and honor the passed context.

Prompt resources live under [`resources/agent`](../resources/agent), loaded by [`prompt.Catalog`](../internal/agent/prompt/prompt.go). Change the resources actually used by assembly and test their loading; filenames alone do not imply an active planning phase. Model-visible copy must agree with executable capabilities and output schemas.

Verify the affected boundaries using [development](development.md). Useful test locations are:

| Change | Relevant tests |
| --- | --- |
| Tool contract or command adapter | `internal/agent/tool/contract`, `internal/agent/tool`, `internal/command/catalog` |
| Receipt, retry, cancellation, batching | `internal/agent/tool/execution` |
| Loop/protocol/finalization | `internal/agent/live`, `internal/agent/llm/openai` |
| Projection and retrieval | `internal/agent/assemble`, `internal/agent/live/observation_tool_test.go` |
| Memory and public/private selection | `internal/agent/turn`, `internal/repository/h2/agent_*_test.go` |
| Production wiring and trusted identity | `cmd/zenbot/live_agent_test.go` |
| Model-dependent behavior | `cmd/zenbot/provider_eval_test.go` (opt-in live provider with safe tool doubles) |

The loop's limits constrain resource use and execution mechanics. They do not ensure sound reasoning, fresh evidence selection, correct conditional actions, complete answers, or appropriate silence. Provider/model choice and thinking settings can change behavior. Evaluate the configured model for the tasks it will perform; deterministic protocol tests and a few live trials are not a universal reliability guarantee.
