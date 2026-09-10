# Zenbot

Zenbot is the Go rewrite of Saturn, a Hack.Chat moderation and room-automation bot. The current migration branch carries Saturn-compatible commands, aliases, role checks, listeners, host and replica lifecycle, H2 persistence, SQLite import, and the Vaelen agent tool loop.

The intentionally excluded parity scope is `mine`, `whiskey`, `ws`, and `wsa`. See [COMMAND_TOOL_INVENTORY.md](COMMAND_TOOL_INVENTORY.md) for the reviewed command matrix and [MIGRATION_PLAN.md](MIGRATION_PLAN.md) for closure evidence.

## Features

- Saturn command aliases and role-aware dispatch, including admin, moderation, DBZ, persistence, room snapshot, replica, and user utilities
- Host health recovery, autorun commands, replica lifecycle, room snapshots, automove, and support relays
- Embedded file-based H2 storage through H2's PostgreSQL compatibility server
- One-time transactional import of a legacy `<dbPath>.db` SQLite database into `<dbPath>.mv.db`
- OpenAI-compatible agent with shared public-room memory, private whisper memory, contextual tools, bounded generated SQL, and command execution
- Deterministic flood and raid detection plus optional semantic moderation
- Self-contained Docker image and idempotent Make targets for deployment and database operations
- Opt-in regular-command latency tracing and Go runtime profiles for diagnosing websocket, listener, database, and command stalls

## Quick Start

### Docker

Docker is the simplest production path. The image contains the Go binary, Java runtime, pinned H2 `2.3.232` jar, prompts, and schema.

```sh
cp config.example.toml config.toml
# Edit the ignored config.toml with the real bot and optional agent values.
make build
make run
make logs
```

`make run` mounts `config.toml` read-only at `/app/config.toml` and mounts `database/` at `/app/database`. If `config.toml` is absent it uses the sanitized `config.example.toml`. If an ignored `.env` exists, Docker loads it after the TOML values; otherwise the host `SATURN_AGENT_API_KEY` is forwarded when set.

### Regular Command Profiling

Structured command tracing is disabled by default and deliberately excludes the `*l` agent command. Enable it in the ignored production `config.toml` while reproducing slow regular commands:

```toml
[profiling]
enabled = true
listenAddress = "0.0.0.0:6060"
slowCommandThresholdMillis = 250
slowStageThresholdMillis = 25
slowTransportThresholdMillis = 25
blockProfileRate = 1000000
mutexProfileFraction = 5
```

`make run` publishes the profiler only on host loopback at `127.0.0.1:6060`. After rebuilding, reproduce a slow command and inspect the structured timing records:

```sh
make rebuild
docker logs zenbot 2>&1 | grep -E 'command\.profile|transport\.profile'
make profile-goroutines
make profile-block
make profile-mutex
make profile-cpu PROFILE_SECONDS=30
```

`command.profile.started` reports `ws_queue_ms`, JSON `parse_ms`, and the inbound queue depth. Stage events identify slow listener handlers such as message audit, pending mail, YouTube preview, agent participation, command lookup, authorization, concrete handler execution, and command audit. `transport.profile.inbound_enqueue` proves websocket-reader backpressure; `transport.profile.write` separates writer-lock delay from the network write. A high `ws_queue_ms` with a slow earlier handler indicates head-of-line blocking in the synchronous message consumer.

Go runtime profiles are process-wide, but regular command execution carries `zenbot.command` and `zenbot.room` pprof labels. Reproduce without concurrent `*l` requests and use those labels when filtering samples so agent work does not distort the diagnosis.

The complete environment override surface is listed in `.env.example`. Override `PROFILING_PORT` when the host port is occupied; keep `profiling.listenAddress` and `CONTAINER_PROFILING_PORT` aligned if changing the in-container port.

Useful lifecycle commands:

```sh
make restart
make rebuild
make stop
make status
make backup-db
make fresh-db
```

`make fresh-db` stops the container, archives any legacy SQLite source under `database/backups/`, and removes the H2 files. The next startup creates a fresh schema. `make backup-db` stops Zenbot before copying the H2 file. `make db-check` opens the stopped database read-only with the pinned H2 jar and queries its version and application-table count; it does not mistake mere file existence for a database check.

### Local

Local development requires Go 1.24 or newer, Java 21 or newer, and H2 `2.3.232`:

```sh
cp config.example.toml config.toml
export H2_JAR="$HOME/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"
make check
make compile
./target/zenbot
```

Zenbot starts and owns the local H2 compatibility server; do not start a second server on port `5435`. `deploy/h2-server.sh` remains available only for explicit external-server diagnostics.

## Configuration

The tracked [config.example.toml](config.example.toml) documents every required and optional setting. Local `config.toml`, `.env`, and `database/` are ignored. Docker also excludes them from its build context.

Important root values:

- `dbPath` is the database stem. `database/database` creates `database/database.mv.db`.
- `wsUrl`, `nick`, and `trip` are Saturn-compatible connection keys. Legacy Zenbot `url`, `name`, and `password` remain accepted.
- `userTrips`, `adminTrips`, and `autorunCommands` accept either TOML arrays or Saturn comma-separated strings.
- `autoReconnect` and `healthCheckInterval` control host recovery.

Agent provider values:

- `agent.endpoint` is the OpenAI-compatible base URL; Zenbot appends `/v1/chat/completions`.
- `agent.model` may remain blank when the endpoint selects its model.
- `agent.creatorTrip` is required when the agent is enabled; no creator identity is compiled into the binary or tracked example.
- `agent.apiKeyEnv` names the optional bearer-token environment variable.
- `agent.thinkingEnabled` is sent as `chat_template_kwargs.enable_thinking`.
- `agent.maxCompletionTokens`, timeout, retry, queue, prompt, and output values are enforced at their runtime boundaries. `agent.maxPromptChars` is checked by Unicode code point before any provider call.
- Keep `agent.maxCompletionTokens` close to the useful reply size. Very large values extend truncated provider generations without bypassing `agent.maxOutputChars`.

Agent execution values:

- `agent.maxSteps` and `agent.maxToolCallsPerTurn` bound one request-local tool loop.
- `agent.maxCallsPerTool` prevents repeated variants of one tool from exhausting a turn.
- `agent.maxToolFailures` disables a failing tool for the remainder of that request.
- `agent.toolTimeoutMillis` supplies the default timeout when a tool descriptor has no override.
- `agent.memoryTurns` and `agent.memoryTtlHours` bound durable conversation memory.
- `agent.memoryRawTurns` keeps the newest complete turns verbatim; older loaded turns are summarized into untrusted H2-backed memory without deleting their authoritative raw rows.
- `agent.memorySummaryMaxChars` bounds the persisted summary projection.
- `agent.contextMessageLimit` controls recent public room context; explicit named-user history can retrieve up to 500 public messages with timestamps and identity metadata.
- `agent.maxContextTokens` supports ceilings up to 1,000,000 estimated tokens, while `agent.contextReserveTokens` reserves policy/request/output capacity. The loop reprojects before every worker-model call, accounts for the live tool manifest, keeps the original objective/task state mandatory, drops assistant-call/tool-result pairs atomically, and never slices JSON.

The complete `SATURN_AGENT_*` environment surface is listed in [.env.example](.env.example). Environment values take precedence over TOML. The ignored production `config.toml` remains the source of truth when no override is supplied.

## Agent Behavior

The direct syntax is:

```text
*l <prompt>
```

Exact public mentions of the bot also enter the same room-scoped agent runtime. Public users in one room share conversation memory; whispers use a private user-and-room key. Ambient participation is disabled unless explicitly configured. A polite request to remain quiet suppresses ambient participation without posting repeated acknowledgements.

Ordinary prompts receive direct, natural-language answers. The agent does not force responses into quotations or another fixed template; quotations are used only when relevant to the request. Requests for live data or Saturn actions remain tool-first, and successful room-delivery tools are not repeated as prose. Every proposed final answer passes through a separate model-backed semantic completion gate; unfinished planning or promises return to the bounded tool loop with evaluator feedback instead of being delivered.

The model receives one checked, caller-filtered semantic manifest and can request multiple tools in one response. Every caller-visible operation has one unique primary intent. Each of the 59 exposed `saturn_<command>` tools has command-specific strict typed parameters, routing guidance, negative constraints, and valid examples; conditional commands use validated `oneOf`/`const` branches, and raw command strings or generic argument tails are not accepted. The five explicit non-actionable commands are `l`, `mine`, `whiskey`, `ws`, and `wsa`. There is no keyword router or preliminary discovery/model call.

Action and command tools execute synchronously and sequentially in provider order. Only independent, idempotent, read-only tools with non-conflicting resource metadata can fan out concurrently. A cancelled action without a verified terminal result becomes non-retryable `ACTION_OUTCOME_UNKNOWN`. Command success requires typed committed/delivery outcomes rather than transport or legacy status alone. Full results remain request-local; the model receives descriptor-bounded, valid JSON observations. Correctable failures and semantically incomplete candidate answers retain the currently executable manifest for self-correction; terminal failures and exhausted bounds receive a tool-free synthesis call, and an unsatisfied bounded result fails closed. Stateful calls support an injectable pause/deny hook, and resumed actions are reauthorized before synchronous execution.

Moderators can request moderation actions through natural language. The configured creator receives direct admin and permanent-ban capabilities. The gateway still performs Saturn role checks and binds autonomous moderation actions to the reviewed author.

See [AGENTIC_ARCHITECTURE.md](AGENTIC_ARCHITECTURE.md) for the full flow and contracts.

## Remote Room Listing And Replicas

`room_users` is the agent's current-room-only presence source and accepts no room parameter. Requests about another room route to `saturn_list`; its underlying `*list <other-room>` workflow opens a bounded credentialed snapshot session using a protocol-valid temporary identity such as `msg_xxxxxxxx`, waits for `onlineSet`, formats the remote users, and closes the temporary connection. The agent adapter rejects same-room `saturn_list` calls before gateway execution. Workflow failures preserve the command-specific error message instead of replacing it with a generic room-operation response.

Replica lifecycle is implemented independently through `ManagedReplicaController` and `ReplicaFactory`. Replicas use the configured bot identity and retain their own websocket engine while remaining owned by the master process.

## Persistence And Migration

The runtime uses H2 only. SQLite remains a read-only migration dependency:

1. Zenbot starts H2 and applies the idempotent embedded schema.
2. If `<dbPath>.db` exists and the corresponding H2 application tables are empty, Zenbot reads the SQLite schema and rows in batches inside one H2 transaction.
3. It recreates indexes, restores identity counters, verifies every table count, and commits.
4. Only after a successful commit does it rename the SQLite database and sidecars with `.bak` suffixes.
5. If both stores contain data, startup fails rather than merging ambiguous records.

Messages default to `PUBLIC`; whispers are explicitly stored as `WHISPER`. The named-user history tool reads public rows across rooms unless a room is requested.

## Verification

```sh
make check
make compile
make build
```

The H2 integration suite uses `H2_JAR` when set and otherwise checks the pinned local Maven path used by the tests. The Docker build independently downloads and checksum-verifies the same H2 release.
