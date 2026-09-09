# Saturn To Zenbot Migration Audit

## Verdict

The current branch implements the requested Saturn behavioral surface in Go,
except for the explicitly excluded `mine`, `whiskey`, `ws`, and `wsa`
commands. This audit supersedes the earlier declaration-based report that
marked consolidated Go behavior as missing merely because it did not have a
one-to-one Java class.

## Audit Method

The source of truth is Saturn's `develop` branch under
`/Users/ab/workspace/projects/saturn`. The target is this Zenbot migration
branch. Review is organized around observable contracts:

- startup, connection, and lifecycle transitions
- command names, aliases, roles, arguments, output, and side effects
- listener ordering and event transformations
- service and persistence behavior
- schema, indexes, visibility, and legacy data migration
- agent invocation, context, tools, correction, and delivery
- production configuration and container operation

Java classes that only split one contract into interfaces, implementations,
factories, records, or facades are allowed to map to a smaller Go composition.
The tests named below are the acceptance evidence; file-count similarity is
not treated as functional proof.

## Functional Mapping

| Saturn area | Zenbot implementation | Acceptance evidence |
|---|---|---|
| `ApplicationRunner`, lifecycle | `cmd/zenbot/main.go`, `cmd/zenbot/startup.go` | `cmd/zenbot/startup_parity_test.go`, engine tests |
| Config loader and agent config | `internal/config` | config compatibility and example tests |
| Command annotations/factory/implementations | `internal/command/catalog`, registry, typed handlers | command parity, handler, integration, and service-command tests |
| Facade/engine and protocol queues | `internal/core`, `internal/common` | engine and room-snapshot tests |
| Chat/info listener chains | `internal/listener` | listener handler, join/leave, snapshot, and preview tests |
| Services and repositories | `internal/service`, `internal/repository` | service tests and real H2 repository tests |
| H2 bootstrap/upgrades | `internal/repository/h2/database.go`, embedded schema | schema-upgrade and repository tests |
| SQLite-to-H2 utility | `internal/repository/h2/sqlite_migrator.go` | migration integration tests |
| Agent API/config/provider | `internal/agent/api`, `internal/config`, `internal/agent/llm` | API, config, provider, and QA tests |
| Agent router/tool loop | `internal/agent/live`, `internal/agent/tool/execution`, `internal/agent/turn` | loop, executor, turn, and runner tests |
| Agent persistence and SQL | H2 agent repositories, `internal/agent/sql`, database tools | persistence, policy, schema, and SQL tool tests |
| Agent moderation/participation | `internal/agent/moderation`, `internal/agent/participation` | join/message monitor and participation tests |
| Prompt resources | `resources/agent` | prompt loader and assembly tests |
| Deployment | `Dockerfile`, `Makefile`, examples | dry-run, build, and full test gates |

## High-Risk Contracts Checked

### Commands

- All 64 source definitions remain visible in the catalog.
- All aliases are owned by that catalog and shared by chat and agent paths.
- All 60 in-scope commands instantiate concrete handlers.
- `l` enters the agent directly and cannot be selected recursively as a tool.
- Creator, moderator, configured-trip, and permanent-ban boundaries are
  preserved by trusted context plus the normal command authorization path.
- Command output is captured for tool observations without suppressing normal
  room delivery or dropping optional engine capabilities.

### Agent

- Each request receives fresh turn state, budgets, failure counts, and tool
  ledger state.
- Public conversation memory is shared within a room; whispers remain private.
- Tool definitions stay present during correction rounds.
- Invalid arguments, unavailable commands, and expected command rejection are
  model-visible error observations rather than process failures.
- Independent safe reads can fan out; actions and resource conflicts create
  sequential barriers.
- Named-user evidence reads up to 500 latest public messages across rooms with
  timestamp and identity metadata.
- Required replies reject empty output, no-reply-only output, and leaked raw
  internal evidence.

### Persistence

- H2 2.3.232 is verified at startup and is the only runtime engine.
- The embedded schema is authoritative and upgrades old files idempotently.
- Message visibility defaults to `PUBLIC`; new whispers are explicit
  `WHISPER`; history tools query only public rows.
- Legacy SQLite is opened read-only, copied transactionally in batches,
  verified by table counts, and archived only after commit.
- Existing non-empty H2 plus non-empty SQLite fails closed instead of merging
  ambiguous state.

### Operations

- Real `config.toml`, `.env`, and database files are ignored and excluded from
  image builds.
- Docker includes Java plus the checksum-pinned H2 jar and mounts config and
  database state at runtime.
- `fresh-db` archives legacy SQLite before removing H2, preventing an old
  source from being imported unexpectedly on the next boot.

## Approved Non-Parity

| Item | Decision |
|---|---|
| `mine` | Excluded by request |
| `whiskey` | Excluded by request |
| `ws` | Excluded with Whiskey relay commands |
| `wsa` | Excluded with Whiskey relay commands |
| One Go type per Java declaration | Not required; behavioral consolidation is intentional |

## Closure Commands

```sh
make check
make compile
go test -race ./...
docker build -t zenbot:parity-check .
git diff --check
```

No declaration-count claim can replace these executable gates.
