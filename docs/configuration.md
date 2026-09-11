# Configuration reference

Zenbot reads `config.toml` from its working directory. Start with the tracked [config.example.toml](../config.example.toml); keep credentials in the ignored local configuration or environment. The binary does not load `.env`, choose an alternate configuration path, or provide application configuration flags. `make run` mounts the selected configuration at `/app/config.toml` and can pass a Docker environment file.

This reference describes the current implementation. Example values are deliberate deployment choices and sometimes differ from defaults for omitted settings.

## Loading and precedence

Top-level connection settings come from TOML. Aliases resolve as `url` before `wsUrl`, `name` before `nick`, and `password` before `trip` before the `TOKEN` environment variable. A whitespace-only primary value permits fallback. `TOKEN` is a fallback, **not an override**: the example's nonempty `trip` placeholder must be replaced or cleared for `TOKEN` to take effect. The trip/password field is the secret used to obtain the bot's trip; `adminTrips`, `userTrips`, and `creatorTrip` contain observed identity tripcodes.

Agent and profiling settings resolve TOML compatibility aliases, apply defaults, read process environment overrides, then validate. Validation runs even when a section is disabled, except checks explicitly conditional on enabling it. There is no configuration reload. Unknown TOML keys are not rejected, so a misspelling can silently have no effect.

Use the prefixed environment names below. Internally, `ValueReader` also accepts exact canonical keys (such as `maxTokens`) and uppercase forms without underscores (`MAXTOKENS`). An exact canonical environment key takes precedence over its prefixed alias; the shared key `enabled` can affect both agent and profiling resolution. This compatibility behavior is best avoided in deployment. `ValueReader` supports explicit runtime and file maps, but production startup supplies only the process environment alongside decoded TOML, not a database or CLI override layer.

Empty environment strings replace string settings; empty or whitespace-only integer/boolean values retain the TOML/default value. Nonempty malformed numbers and booleans fail resolution. A TOML zero often means “apply the default,” whereas an environment zero survives to validation. In particular, use `SATURN_AGENT_MAX_RETRIES=0` to disable retries: TOML `maxRetries = 0` resolves to two retries.

## Connection, identity, and storage

| TOML key | Example | Behavior |
| --- | --- | --- |
| `wsUrl` (`url` takes precedence) | `wss://hack.chat/chat-ws` | WebSocket endpoint; no application-level default. |
| `nick` (`name` takes precedence) | `alphaBot` | Bot nickname; no application-level default. |
| `trip` (`password` takes precedence) | placeholder | Bot trip secret; falls back to `TOKEN` only when both file values are blank. |
| `channel` | `programming` | Initial room. |
| `cmdPrefix` | `*` | Prefix for regular chat commands. |
| `adminTrips` | placeholder | Configured administrator tripcodes. |
| `userTrips` | empty | Configured trusted/user tripcodes used by authorization. |
| `autoReconnect` | `true` | Enables recovery and health checks; omitted defaults to `false`. |
| `healthCheckInterval` | `5` | Health interval in **minutes** when reconnect is enabled. Nonpositive/omitted values use a 15-second runtime fallback. |
| `autorunCommands` | empty | Sent once after initial startup readiness; non-slash entries are prefixed and whispered to the bot itself, slash entries are sent directly. Supply command names without the command prefix. |
| `dbPath` | `database/database` | Required H2 database stem relative to the working directory, or an absolute path. `.mv.db`/`.db` suffixes are stripped; use the stem convention. |

`adminTrips`, `userTrips`, and `autorunCommands` accept TOML string arrays or comma-separated strings. String form trims whitespace and drops empty entries; array form is decoded as supplied. Use arrays when an autorun command contains a comma.

There are no general top-level environment aliases for room, nickname, prefix, database path, or reconnect settings. Connection settings have no comprehensive startup validator; configuration errors may surface when the affected subsystem starts.

The obsolete `proxies` key is not a `Config` field and is ignored if present in
older configurations. It does not configure a proxy pool or proxy selection.

H2 startup additionally reads these environment-only settings:

| Variable | Default | Meaning |
| --- | --- | --- |
| `JAVA` | `java` | Java executable name or path. |
| `H2_JAR` | none locally; `/opt/h2/h2.jar` in Docker | Required H2 JAR. Supported version is pinned to 2.3.232 by downloads; startup checks H2 identity, not exact version. |
| `TOKEN` | none | Bot credential fallback described above. |

The application starts H2 on port 5435; this port is not configurable through TOML or an application environment alias. Docker supplies Java and the pinned JAR. Changing Make's database mount variables does not rewrite `dbPath`.

## Agent provider and execution

All keys in this section belong under `[agent]`. Environment names have the prefix `SATURN_AGENT_`; the table shows the suffix. “Default” means the resolved value with the setting omitted, not necessarily the example value.

| TOML key | Environment suffix | Default | Meaning |
| --- | --- | --- | --- |
| `enabled` | `ENABLED` | `false` | Enable agent runtime. |
| `endpoint` | `ENDPOINT` | `http://localhost:16261` | Absolute HTTP(S) base URL; client appends `/v1/chat/completions`. Do not include that route or `/v1` in the base. |
| `model` | `MODEL` | empty | Model identifier; omitted from provider payload when empty. |
| `apiKeyEnv` | `API_KEY_ENV` | `SATURN_AGENT_API_KEY` | Name of the environment variable holding the API key. There is no TOML `apiKey` field. Empty environment override disables key lookup. |
| `timeoutSeconds` | `TIMEOUT_SECONDS` | `30` | Provider call timeout. Canonical TOML `timeoutMillis` takes precedence when nonzero; environment `TIMEOUT_MILLIS` takes precedence over `TIMEOUT_SECONDS`. |
| `requestTimeoutMillis` | `REQUEST_TIMEOUT_MILLIS` | `180000` | Runtime request deadline across the turn. |
| `maxCompletionTokens` | `MAX_COMPLETION_TOKENS` | `1024` | Provider response token budget; canonical TOML `maxTokens` wins when nonzero. Example uses `4096`. |
| `thinkingEnabled` | `THINKING_ENABLED` | `false` | Provider thinking option; example enables it. Provider/model support is required. |
| `maxConcurrentRequests` | `MAX_CONCURRENT_REQUESTS` | `2` | Concurrent runtime requests. |
| `queueCapacity` | `QUEUE_CAPACITY` | `0` | Pending request capacity; zero has no pending queue. |
| `maxSteps` | `MAX_STEPS` | `5` | Model/tool-loop step bound. |
| `maxToolCallsPerTurn` | `MAX_TOOL_CALLS_PER_TURN` | `4` | Live tool-call budget; canonical TOML `maxTools` wins when nonzero. |
| `toolTimeoutMillis` | `TOOL_TIMEOUT_MILLIS` | `10000` | Per-tool timeout. |
| `maxToolCalls` | `MAX_TOOL_CALLS` | `4` | Legacy aggregate setting: parsed and validated, but not wired into the live loop. Use `maxToolCallsPerTurn` to change the live budget. |
| `maxCallsPerTool` | `MAX_CALLS_PER_TOOL` | `2` | Per-tool call bound. |
| `maxToolFailures` | `MAX_TOOL_FAILURES` | `2` | Tool failure bound. |
| `maxPromptChars` | `MAX_PROMPT_CHARS` | `8000` | Prompt character bound. |
| `maxContextTokens` | `MAX_CONTEXT_TOKENS` | `16000` | Context token budget. |
| `contextReserveTokens` | `CONTEXT_RESERVE_TOKENS` | `2048` | Reserved context budget; must be smaller than `maxContextTokens`. |
| `maxOutputChars` | `MAX_OUTPUT_CHARS` | `8000` | Final output character bound. |
| `maxRetries` | `MAX_RETRIES` | `2` | Additional provider retry attempts. |
| `retryBackoffMillis` | `RETRY_BACKOFF_MILLIS` | `250` | Retry delay; an environment zero is accepted, but the provider replaces nonpositive delay with 100 ms. |

The tool loop reserves a terminal response without tools after exhaustion. These limits do not guarantee a model will perform every requested action. A blank model and API key can be appropriate for a local provider; neither is required by agent configuration validation.

`localhost` in the Docker container refers to the container. Set `endpoint` to a provider address reachable from that container.

## Agent memory and participation

These keys also belong under `[agent]`; environment suffixes use `SATURN_AGENT_`.

| TOML key | Environment suffix | Default | Meaning |
| --- | --- | --- | --- |
| `creatorTrip` | `CREATOR_TRIP` | empty | Required nonblank tripcode when the agent is enabled. |
| `ambientEnabled` | `AMBIENT_ENABLED` | `false` | Enable ambient participation. Legacy `ambient = true` also enables it; either TOML true wins before environment resolution. |
| `ambientEveryMessages` | `AMBIENT_EVERY_MESSAGES` | `8` | Ambient message cadence. |
| `quietMinutes` | `QUIET_MINUTES` | `15` | Quiet-state duration in minutes. |
| `contextMessageLimit` | `CONTEXT_MESSAGE_LIMIT` | `60` | Bounded recent room context. |
| `noReplyMarker` | `NO_REPLY_MARKER` | `[[SATURN_NO_REPLY]]` | Marker used to suppress a reply. |
| `memoryTurns` | `MEMORY_TURNS` | `30` | Durable conversation turn bound, 1–60. |
| `memoryRawTurns` | `MEMORY_RAW_TURNS` | `min(20, memoryTurns)` | Newest raw turns retained after compaction, 1–`memoryTurns`. |
| `memorySummaryMaxChars` | `MEMORY_SUMMARY_MAX_CHARS` | `12000` | Unicode-character ceiling for persisted untrusted summary. |
| `memoryTtlHours` | `MEMORY_TTL_HOURS` | `168` | Memory TTL; canonical TOML `memoryTtlMinutes` wins when nonzero. Resolved minutes must be 1–525600. |

Room conversation memory is shared public room context, not a private per-user notebook. If a file explicitly sets `memoryRawTurns`, lowering `memoryTurns` through the environment does not automatically clamp that explicit value.

## Moderation

Moderation settings are under `[agent]` and are independent of the ambient conversation switch. `moderationEnabled` / `SATURN_AGENT_MODERATION_ENABLED` defaults to false; the example enables it. The following numeric settings default as shown. Their environment names are `SATURN_AGENT_` plus the uppercase underscore form of the listed TOML key (for example `SATURN_AGENT_MODERATION_MESSAGE_BURST_COUNT`).

| TOML key | Default |
| --- | --- |
| `moderationMessageBurstCount` | `6` |
| `moderationMessageBurstWindowSeconds` | `5` |
| `moderationRepeatedMessageCount` | `4` |
| `moderationRepeatedMessageWindowSeconds` | `10` |
| `moderationSecondBreachWindowSeconds` | `30` |
| `moderationPostKickWindowSeconds` | `600` |
| `moderationJoinBurstCount` | `8` |
| `moderationJoinBurstWindowSeconds` | `10` |
| `moderationSameHashJoinCount` | `5` |
| `moderationSameHashJoinWindowSeconds` | `20` |
| `moderationSuspiciousNameJoinCount` | `5` |
| `moderationSuspiciousNameJoinWindowSeconds` | `20` |
| `moderationActionCooldownSeconds` | `30` |

Canonical TOML aliases take precedence when nonzero: `moderationJoinWindowSeconds` over `moderationJoinBurstWindowSeconds`; `moderationSameHashCount`/`moderationSameHashWindowSeconds` over the corresponding `SameHashJoin` names; and `moderationNameClusterCount`/`moderationNameClusterWindowSeconds` over the corresponding `SuspiciousNameJoin` names. The prefixed environment names use the longer names in the table. Every resolved moderation threshold must be positive when moderation is enabled.

Current wiring limitation: deterministic join and message automation reads the original TOML-backed `Config.Agent`, rather than the resolved copy. Its enable switch, creator protection, and numeric thresholds therefore do not receive environment overrides or resolver defaults. Join automation requires every canonical join threshold to be explicitly positive; the example's compatibility aliases alone do not activate it. Message automation requires its message thresholds explicitly in TOML and is composed inside the enabled agent runtime. Semantic moderation uses resolved settings and also depends on ingress readiness. Treat the table as resolver behavior, not a guarantee that every monitor receives the resolved values.

## Agent SQL tools

Flat `[agent]` keys below are supported alongside nested `[agent.sql]`. Positive flat limits override nested limits. A flat `dynamicSqlEnabled = true` enables SQL even if nested `enabled` is false; a flat false does not disable a nested true. Environment overrides apply last. The example enables SQL; the omitted-setting default is false.

| Flat `[agent]` key | Nested `[agent.sql]` key | Environment suffix (`SATURN_AGENT_`) | Default |
| --- | --- | --- | --- |
| `dynamicSqlEnabled` | `enabled` | `DYNAMIC_SQL_ENABLED` | `false` |
| `dynamicSqlMaxSqlChars` | `maxSqlChars` | `DYNAMIC_SQL_MAX_SQL_CHARS` | `4000` |
| `dynamicSqlMaxRows` | `maxRows` | `DYNAMIC_SQL_MAX_ROWS` | `50` |
| `dynamicSqlMaxColumns` | `maxColumns` | `DYNAMIC_SQL_MAX_COLUMNS` | `32` |
| `dynamicSqlMaxCellChars` | `maxCellChars` | `DYNAMIC_SQL_MAX_CELL_CHARS` | `2000` |
| `dynamicSqlMaxResultChars` | `maxResultChars` | `DYNAMIC_SQL_MAX_RESULT_CHARS` | `32000` |
| `dynamicSqlTimeoutMillis` | `timeoutMillis` | `DYNAMIC_SQL_TIMEOUT_MILLIS` | `1000` |

These settings control agent database tools, including bounded read-only SQL and schema access. They do not configure the regular administrator SQL command. All six numeric limits must resolve to 1–1000000, even when agent SQL is disabled.

## Validation limits

Agent timeout in milliseconds must be positive. The resolved token, step, tool-call, per-tool call, tool-failure, prompt/context/reserve, concurrency, tool-timeout, and request-timeout values must be 1–1000000. Retry count, retry backoff, and queue capacity must be 0–1000000. Summary and output character limits must be 1–1000000. Memory limits are documented above. Enabled agents additionally require positive ambient cadence, quiet duration, and context-message limit, plus nonblank creator trip and no-reply marker, even if ambient mode is disabled.

The examples contain nonempty credential placeholders, which are not detected as placeholders by validation. Replace them before running against a real room.

## Profiling

Keys belong under `[profiling]`. Environment names use `ZENBOT_PROFILING_` plus the suffix below. Profiling is opt-in and instruments regular commands; the direct `l` agent path is excluded from those command traces. The Go runtime profiles are process-wide.

| TOML key | Environment suffix | Default | Allowed values |
| --- | --- | --- | --- |
| `enabled` | `ENABLED` | `false` | Boolean |
| `listenAddress` | `LISTEN_ADDRESS` | `0.0.0.0:6060` | `host:port`, port 1–65535 |
| `slowCommandThresholdMillis` | `SLOW_COMMAND_THRESHOLD_MILLIS` | `250` | 1–3600000 |
| `slowStageThresholdMillis` | `SLOW_STAGE_THRESHOLD_MILLIS` | `25` | 1–3600000 |
| `slowTransportThresholdMillis` | `SLOW_TRANSPORT_THRESHOLD_MILLIS` | `25` | 1–3600000 |
| `blockProfileRate` | `BLOCK_PROFILE_RATE` | `1000000` | 0–1000000000 nanoseconds |
| `mutexProfileFraction` | `MUTEX_PROFILE_FRACTION` | `5` | 0–1000000 |

TOML zero sampling values are replaced by defaults; environment zero disables the corresponding sampling. Bounds and listener syntax are validated even with profiling disabled. When enabled, `/debug/pprof/` is served without application authentication. The Docker Make target publishes its host port on `127.0.0.1` by default; native execution with the default listen address binds all interfaces. Use a loopback listen address for local native profiling.

## Make variables versus application environment

The [Makefile](../Makefile) controls build/container operations. Its variables are not application settings and do not override TOML by themselves. For example, `make run SATURN_AGENT_ENABLED=true` does not automatically pass that value into Docker.

`make run` passes `ENV_FILE` using Docker `--env-file` when that file exists. Otherwise, it forwards only a nonempty host environment variable named by `AGENT_API_KEY_ENV` (default `SATURN_AGENT_API_KEY`). It does not forward all exported `SATURN_AGENT_*`, profiling, `TOKEN`, `JAVA`, or `H2_JAR` values. If the environment file exists, even that API-key fallback is skipped. A custom key name must also be selected by the application's `apiKeyEnv`/`SATURN_AGENT_API_KEY_ENV`; the Make variable does not set it automatically.

| Make variable | Default | Purpose |
| --- | --- | --- |
| `IMAGE_NAME`, `CONTAINER_NAME` | `zenbot` | Image and container names. |
| `DOCKER`, `GO` | `docker`, `go` | Tool executables. |
| `APP_DIR` | `/app` | Container mount destination base; does not change image working directory or entrypoint. |
| `CONFIG_FILE` | local `config.toml` if present, otherwise `config.example.toml` | Read-only configuration mount source. |
| `ENV_FILE` | repository `.env` | Optional Docker environment file. |
| `DATABASE_DIR` | repository `database` | Host database directory mounted in the container. |
| `DATABASE_STEM` | `$(DATABASE_DIR)/database` | Stem used by database maintenance targets. |
| `DATABASE_FILE` | `$(DATABASE_STEM).mv.db` | H2 file checked/backed up/reset by maintenance targets. |
| `LEGACY_DATABASE_FILE` | `$(DATABASE_STEM).db` | Legacy SQLite file archived by `fresh-db`. |
| `DATABASE_BACKUP_DIR` | `$(DATABASE_DIR)/backups` | Backup/archive destination. |
| `CONTAINER_DATABASE_STEM` | `$(APP_DIR)/database/$(notdir $(DATABASE_STEM))` | Database check target stem inside container. |
| `H2_CHECK_URL` | `jdbc:h2:file:$(CONTAINER_DATABASE_STEM);ACCESS_MODE_DATA=r;IFEXISTS=TRUE` | JDBC URL for `db-check`. |
| `AGENT_API_KEY_ENV` | `SATURN_AGENT_API_KEY` | Host key name forwarded only when no environment file exists. |
| `PROFILING_HOST`, `PROFILING_PORT` | `127.0.0.1`, `6060` | Published host address/port and profiling client target. |
| `CONTAINER_PROFILING_PORT` | `6060` | Published container port; keep consistent with application listen address. |
| `PROFILE_SECONDS` | `30` | CPU profile capture duration. |
| `STOP_TIMEOUT` | `30` | Docker stop grace period in seconds. |
| `TARGET_DIR` | repository `target` | Local compiled binary directory. |

The [`.env.example`](../.env.example) is a template for Docker's environment-file format. A native process needs its environment supplied separately and still requires `config.toml` in its working directory.

Implementation references: [top-level loading](../internal/config/config.go), [agent resolution](../internal/config/agent_config.go), [value lookup](../internal/config/value_reader.go), [profiling resolution](../internal/config/profiling_config.go), [runtime composition](../cmd/zenbot/main.go), and [H2 startup](../internal/repository/h2/database.go).
