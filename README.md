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
- `agent.maxCompletionTokens`, timeout, retry, queue, prompt, and output values are enforced at their runtime boundaries.

Agent execution values:

- `agent.maxSteps` and `agent.maxToolCallsPerTurn` bound one request-local tool loop.
- `agent.maxCallsPerTool` prevents repeated variants of one tool from exhausting a turn.
- `agent.maxToolFailures` disables a failing tool for the remainder of that request.
- `agent.toolTimeoutMillis` supplies the default timeout when a tool descriptor has no override.
- `agent.memoryTurns` and `agent.memoryTtlHours` bound durable conversation memory.
- `agent.contextMessageLimit` controls recent public room context; explicit named-user history can retrieve up to 500 public messages with timestamps and identity metadata.

The complete `SATURN_AGENT_*` environment surface is listed in [.env.example](.env.example). Environment values take precedence over TOML. The ignored production `config.toml` remains the source of truth when no override is supplied.

## Agent Behavior

The direct syntax is:

```text
*l <prompt>
```

Exact public mentions of the bot also enter the same room-scoped agent runtime. Public users in one room share conversation memory; whispers use a private user-and-room key. Ambient participation is disabled unless explicitly configured. A polite request to remain quiet suppresses ambient participation without posting repeated acknowledgements.

Ordinary prompts receive direct, natural-language answers. The agent does not force responses into quotations or another fixed template; quotations are used only when relevant to the request. Requests for live data or Saturn actions remain tool-first, and successful room-delivery tools are not repeated as prose.

The model can request multiple tools in one response. Action and command tools execute sequentially in provider order. Only independent, idempotent, read-only tools with non-conflicting resource metadata can fan out concurrently. Tool errors are returned as observations, the same authorized manifest is retained for correction calls, and the model synthesizes the final answer after observations.

Moderators can request moderation actions through natural language. The configured creator receives direct admin and permanent-ban capabilities. The gateway still performs Saturn role checks and binds autonomous moderation actions to the reviewed author.

See [AGENTIC_ARCHITECTURE.md](AGENTIC_ARCHITECTURE.md) for the full flow and contracts.

## Known Room Snapshot Issue

`*list <current-room>` reads the host engine's active-user snapshot and works without opening another connection. `*list <other-room>` currently reaches the remote snapshot workflow but its generated temporary nick uses the form `msg-xxxxxxxx`. Hack.Chat accepts only letters, numbers, and underscores in a nick, so it rejects that join before sending `onlineSet`; the coordinator then emits the generic `Unable to complete room operation.` fallback. The target-room spelling is not the underlying failure.

Replica lifecycle is implemented independently through `ManagedReplicaController` and `ReplicaFactory`; replicas use the configured bot identity and are not blocked by this temporary-nick defect. The remote snapshot correction is to generate a protocol-valid identity such as `msg_xxxxxxxx` and preserve the request-specific failure message.

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
