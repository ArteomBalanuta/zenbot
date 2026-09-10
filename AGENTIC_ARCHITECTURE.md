# Zenbot Agentic Architecture

## Purpose

Zenbot's agent layer adapts Saturn's Vaelen behavior to Go without giving the model direct access to transport, persistence, or moderation internals. The model proposes text and typed tool calls. Zenbot owns identity, capabilities, schemas, ordering, deadlines, command authorization, room delivery, and durable memory.

The production composition root is `cmd/zenbot/main.go`. Agent implementation lives under `internal/agent`, command execution remains under `internal/command`, and H2-backed context and memory remain under `internal/repository/h2`.

## Active Request Path

```mermaid
flowchart TD
    A[Hack.Chat event] --> B[Message listener chain]
    B --> C{Direct *l command?}
    B --> D{Exact bot mention?}
    B --> E{Moderation candidate?}
    B --> F{Ambient sample eligible?}
    C -->|yes| G[DirectSubmissionAdapter]
    D -->|yes| H[RoomParticipation]
    E -->|yes and enabled| H
    F -->|yes and enabled| H
    G --> I[InvocationFactory]
    H --> I
    I --> J[Immutable invocation and trusted capabilities]
    J --> K[Runtime admission and memory-key lock]
    K --> L[Runner]
    L --> M[Load durable memory]
    M --> N[Load historical tool evidence]
    N --> O[Load recent public room context]
    O --> P[Assembler]
    P --> Q[OpenAI-compatible completion]
    Q --> R{Tool calls present?}
    R -->|no| S[OutputFinalizer]
    R -->|yes| T[Registry tool loop]
    T --> U[Validate manifest, capability, schema and budget]
    U --> V[Execute ordered batch]
    V --> W[Append assistant tool_calls and tool observations]
    W --> Q
    S --> X{Visible reply?}
    X -->|yes| Y[Runtime sink sends room or whisper message]
    X -->|no| Z[Silent completion]
    Y --> AA[Persist conversation and reusable evidence]
```

## Components

### Ingress And Invocation

- `internal/agent/live/direct_submission.go` adapts `*l` into the shared runtime.
- `internal/agent/live/participation.go` adapts exact mentions, optional ambient samples, and semantic moderation candidates.
- `internal/agent/participation/invocation.go` derives capabilities only from the trusted room snapshot. User prompt text cannot grant a capability.
- `cmd/zenbot/main.go` builds the snapshot from the current master, configured admins, and persisted moderator/admin roles.

Invocation modes are:

| Mode | Source | Reply requirement | Capability notes |
|---|---|---:|---|
| `DIRECT` | `*l <prompt>` | Required | Creator may receive admin and permanent-ban tools |
| `MENTION` | Exact public `@bot` mention | Required | Trusted admins/moderators may receive moderation tools |
| `AMBIENT` | Configured public sample | Optional | Privileged action capabilities are withheld |
| `MODERATION` | Semantic candidate | Silent | Only target-bound moderation actions are eligible |

### Runtime And State Isolation

`internal/agent/runtime/runtime.go` provides bounded admission, cancellation, and per-memory-key ordering. Each invocation creates a fresh turn state and a fresh tool execution ledger. Tool counts, failures, duplicate keys, and successful prerequisites never cross request boundaries.

Public turns use `<room>|public` as the memory namespace, so users in one room can continue a shared conversation. Whispers use trip, then hash, then nick identity in a private namespace. Different rooms never share memory.

The runtime invokes `AfterDelivery` only after a visible response is successfully sent. This prevents a provider response that never reached chat from becoming conversational history.

### Prompt Assembly

`internal/agent/assemble/assemble.go` combines:

- the extracted prompt resources under `resources/agent`
- immutable invocation metadata and current user prompt
- bounded durable conversation memory
- recent public room context from H2
- reusable historical tool evidence
- the capability-filtered provider tool manifest

`agent.maxPromptChars` is enforced during assembly. Current prompt intent remains the active task; history and room messages are supporting context.

Named-user profile/history requests require fresh `user_message_history` evidence. That tool reads up to 500 latest `PUBLIC` rows across rooms by default, or one specified room, with timestamps, nick, trip, hash, channel, and text.

### Provider

`internal/agent/llm/openai/client.go` calls `<agent.endpoint>/v1/chat/completions`. It sends the optional model, completion-token bound, contextual tools, and `chat_template_kwargs.enable_thinking`. The configured operation timeout covers retries and retry backoff. Only transient transport/status failures are retried.

Provider accounting is schema-open optional metadata. Integer usage counters are
retained through the stable `Usage()` API, while nested details and unfamiliar
value shapes are ignored. Provider additions therefore cannot invalidate an
otherwise usable completion.

Tool manifests remain present on every correction round. This is required for the model to repair invalid arguments or choose a fallback tool after receiving an error observation.

## Tool Contracts

All tools implement `internal/agent/tool.Tool`:

```go
type Tool interface {
    Name() string
    Descriptor(api.Context) (contract.Descriptor, error)
    Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error)
}
```

A descriptor defines the stable name, label, capability description, negative guidance, strict JSON parameter schema, result schema, access, effect, result mode, idempotency, timeout, prerequisites, and resource reads/writes. Construction validates every registered descriptor before the live loop is accepted.

The standard result envelope is:

```json
{
  "status": "success",
  "data": {}
}
```

Failures use the same envelope shape with `status: "error"`, `data: null`,
and an `error` object containing the stable `code` and model-visible
`message`. The enclosing OpenAI tool message carries the tool-call identity;
the JSON envelope therefore does not duplicate the tool name.

Expected command validation or authorization failures become `COMMAND_REJECTED` observations. Context cancellation remains a Go error so shutdown and request cancellation are not misrepresented as model-correctable failures.

### Production Tool Inventory

| Tool family | Effect | Parallel eligibility | Access |
|---|---|---:|---|
| `user_message_history` | Read-only model data | Yes | Public invocation |
| `room_users` | Read-only model data | Yes | Public invocation |
| `database_query` | Fixed read-only query | Yes | Public invocation |
| `database_schema` | Read-only schema | Yes | `DYNAMIC_SQL` |
| `database_sql` | Bounded read-only SQL after schema | Sequential prerequisite chain | `DYNAMIC_SQL` |
| `run_command` | Room-delivery action | Never | Contextual enum and capabilities |
| `saturn_<command>` | One concrete Saturn command | Never | Command-specific capability |

The complete command mapping is in [COMMAND_TOOL_INVENTORY.md](COMMAND_TOOL_INVENTORY.md).

## Tool Loop

The active generalized loop is split between `internal/agent/live/tool_loop.go`, `internal/agent/live/registry_loop.go`, and `internal/agent/tool/execution/execution.go`.

1. The registry computes descriptors for the current trusted context and hides unauthorized tools.
2. The assembler sends one provider request with all eligible definitions.
3. If the model returns no calls, its content moves to finalization.
4. Every returned call must have an ID and registered name.
5. The executor validates capability prerequisites and the strict argument schema before invoking code.
6. The request-local ledger rejects duplicate call-and-argument keys, per-tool overuse, missing successful prerequisites, and tools disabled after repeated failures.
7. Consecutive independent calls may execute concurrently only when every descriptor is read-only, idempotent, prerequisite-free, and resource-compatible.
8. Any action, command, dependency, or resource conflict creates an ordering barrier and executes sequentially in provider order.
9. Results are restored to source order and appended as one assistant `tool_calls` message followed by matching `tool` observations.
10. The same authorized manifest is sent to the provider for correction or final synthesis.
11. The loop exits when no tool calls remain or returns an error when a configured bound is exceeded.

The relevant bounds are:

| Setting | Enforced by | Meaning |
|---|---|---|
| `maxSteps` | turn state | Maximum provider/tool iterations |
| `maxToolCallsPerTurn` | turn state | Aggregate calls reserved by one request |
| `maxCallsPerTool` | execution ledger | Distinct calls to one tool |
| `maxToolFailures` | execution ledger | Failures before that tool is disabled for the request |
| `toolTimeoutMillis` | executor | Default when a descriptor has no timeout |
| descriptor timeout | executor | Per-tool override |

## Command Execution

Command identity and aliases have one source in `internal/command/catalog/catalog.go`. `internal/command/registry.go` materializes chat handlers from that catalog, while `internal/command/agent_catalog.go` derives the agent manifest from it.

`internal/command/agent_gateway.go` reconstructs a normal Saturn command message using the current prefix and trusted caller identity. It resolves the current master for every invocation, applies command/capability authorization, binds autonomous moderation to its reviewed author, captures command output, and records the command audit result.

The model cannot directly send websocket payloads, write the database, change its role, or select an arbitrary command outside the contextual manifest.

## Memory And Evidence

`internal/agent/turn/memory.go` and `internal/agent/live/memory.go` adapt H2 repositories into bounded memory. Conversation rows are loaded by memory key, TTL, and turn count. Public and whisper contexts remain separate.

Reusable tool evidence is persisted only for successful eligible model-data tools. Command room-delivery results are not replayed as fresh facts. Required named-user history is fetched again instead of reusing an old prose profile.

Recent room context and user-history queries read only `PUBLIC` messages. New whispers are stored as `WHISPER`; schema upgrades classify legacy null visibility as `PUBLIC` for Saturn compatibility.

## Output And Failures

`internal/agent/live/runner.go` sends provider content through `OutputFinalizer` before delivery. It:

- normalizes response formatting
- preserves ordinary provider answers instead of coercing them into a quotation or fixed template
- rejects empty required replies
- strips the no-reply marker from otherwise valid text
- rejects leaked internal tool evidence
- applies the configured Unicode output bound
- supports intentional silence for non-required ambient/moderation turns

Reply-required runtime failures use the configured failure sink. Tool failures do not crash the runtime; they become coded observations unless the enclosing context was cancelled.

## Operational Observability

Every production invocation carries its request ID through structured `slog`
events. The same `request_id`, `mode`, `room`, and `nick` fields connect runtime,
context, provider, tool-loop, delivery, and persistence activity. Logs record
counts, stage names, finish reasons, error codes, HTTP status, and durations;
they do not record prompts, API keys, tool arguments/results, database rows, or
conversation contents.

The primary lifecycle events are:

- `agent.request.started`, `agent.request.completed`, and `agent.request.failed`
- `agent.context.loaded` and `agent.context.load_failed`
- `agent.request.assembled`, `agent.loop.started`, `agent.loop.cycle`,
  `agent.loop.completed`, and `agent.loop.failed`
- `agent.llm.request.started`, `agent.llm.attempt.retrying`,
  `agent.llm.request.completed`, and `agent.llm.request.failed`. Invalid JSON
  also emits `agent.llm.response.malformed` with the parser offset, response
  size, media type, and SHA-256 fingerprint, never the response body.
- `agent.tool.batch_started`, `agent.tool.started`, `agent.tool.completed`, and
  `agent.tool.batch_completed`
- `agent.correction.started`, with `stage` distinguishing command, freshness,
  tool-follow-up, and synthesis calls
- `agent.response.finalized`, `agent.response.finalization_failed`,
  `agent.delivery.completed`, and persistence failure events

For Docker deployments, follow all events with `docker logs -f zenbot`. Filter
for one turn by its `request_id`, or filter `agent.` to inspect only agentic
activity. A generic room failure now always has a matching
`agent.request.failed` event containing the wrapped server-side cause.

## Configuration

`internal/config/agent_config.go` accepts both native Zenbot names and Saturn TOML/environment names. Environment values take precedence over TOML. See [config.example.toml](config.example.toml) and [.env.example](.env.example) for the complete surface.

The creator trip must be supplied explicitly when the agent is enabled. It is
retained in the ignored production configuration and is not compiled into the
binary or published in the tracked examples.

The local real `config.toml`, `.env`, and database files are intentionally ignored and excluded from Docker build contexts. `make run` mounts the chosen config rather than baking environment-specific values into the image.

## Verification Map

- `internal/agent/live/registry_loop_test.go`: correction loop, multi-tool ordering, parallel read fan-out, per-tool and failure limits
- `internal/agent/tool/execution/execution_test.go`: validation, authorization, cancellation, default/descriptor timeouts, ledger behavior
- `internal/agent/tool/*_test.go`: schemas and individual tool contracts
- `internal/agent/runtime/*_test.go`: admission, cancellation, memory-key ordering, ambient coalescing
- `internal/agent/turn/*_test.go`: freshness, evidence, memory, and policy state
- `cmd/zenbot/live_agent_test.go`: production composition, contextual command visibility, moderator/admin role propagation
- `internal/repository/h2/agent_*_test.go`: persistent context, memory, schema, SQL, and 500-message history
