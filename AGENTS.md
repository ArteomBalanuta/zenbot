# Agent guide to Zenbot

This file applies to the whole repository. Use it as a navigation and working
guide, not as a substitute for reading the affected source and tests. Update it
when durable project structure or constraints change; do not turn it into a
progress journal, task ledger, or transcript.

## Start here

1. Read [README.md](README.md), then the relevant guide in [docs/index.md](docs/index.md).
2. Inspect `git status --short` and the current branch before editing. Preserve
   unrelated and uncommitted user changes. Do not assume the checkout is clean.
3. Trace the affected entrypoint through its real production composition and
   tests. A helper existing in source does not prove production uses it.
4. Check [.agents/skills](.agents/skills) for relevant shared skills. For branch,
   commit, and pull-request work, read
   [git-pr-writing/SKILL.md](.agents/skills/git-pr-writing/SKILL.md) and its
   supporting [GIT.md](.agents/skills/git-pr-writing/GIT.md).

Shared `.agents/skills/` files are intentional project assets. Preserve them and
keep them tracked. Private agent working directories and temporary plans are
different: do not publish `.hermes/`, `.superpowers/`, worktrees, or handoff logs.
Keep reusable architectural knowledge in `docs/`, not dated implementation plans.

## Project and runtime

Zenbot is a Go Hack.Chat bot with Saturn-compatible command aliases, role-aware
moderation, history, notes/mail, utilities, managed replicas, and an optional LLM
agent. SQLite is the embedded runtime database, accessed through
`modernc.org/sqlite` and `database/sql`. There is no external database server.

The executable reads `config.toml` from its working directory. Run it from the
repository root locally. Agent prompts are runtime files in `resources/agent/`;
they are not embedded. The SQLite schema and `VERSION` are embedded. A standalone
binary without prompt resources is not a complete agent distribution.

## Source map

| Area | Start reading here | Responsibility |
| --- | --- | --- |
| Process composition | [cmd/zenbot/main.go](cmd/zenbot/main.go) | Config, SQLite, agent, factories, lifecycle ownership |
| Host replacement | `cmd/zenbot/host_supervisor.go`, `master_binding.go` | Replace the master without stale process-owned callbacks |
| Configuration | [internal/config](internal/config) | TOML compatibility, defaults, environment resolution, validation |
| Engine construction | [internal/factory/engine_factory.go](internal/factory/engine_factory.go) | Production dependencies and listener composition |
| Room runtime | [internal/core](internal/core) | Room state, connections, moderation, replicas, lifecycle |
| Protocol I/O | [internal/transport](internal/transport) | WebSocket reads, writes, backpressure, cancellation |
| Event handling | [internal/listener](internal/listener) | Chat/info chains, joins/leaves, snapshots |
| Command identity | [internal/command/catalog](internal/command/catalog) | Canonical names, aliases, roles, agent contracts |
| Command execution | [internal/command](internal/command) | Handlers, dispatch, gateway, observations and receipts |
| Application services | [internal/service](internal/service) | History, mail, weather/time, security, other operations |
| Persistence | [internal/repository/sqlite](internal/repository/sqlite) | Queries, schema/upgrades, transactions |
| Agent loop | [internal/agent/live/turn_engine.go](internal/agent/live/turn_engine.go) | Model-selected calls, observations, bounds and final response |
| Agent boundaries | [internal/agent](internal/agent) | Admission, context, memory, tool contracts and execution |
| Runtime text | [resources/agent](resources/agent) | Prompt templates and model-visible tool copy |
| Test support | [internal/testutil](internal/testutil) | Isolated real SQLite fixtures |

Incoming WebSocket events flow through transport, core dispatch, listeners,
command handlers, and services/repositories. Agent command tools return through
the same authorized command boundary; do not create a parallel implementation of
moderation or utility commands. Temporary remote-room sessions are bounded
snapshot operations, not persistent replicas.

## Build and test

Prerequisites: Go 1.26 or newer; the module, Docker, and CI select Go 1.27.1.
The SQLite driver and SQL policy parser are pure Go. SQLite itself is embedded
through a Go module. See [getting started](docs/getting-started.md).

Tests use real SQLite and temporary database directories. Never introduce a
developer-specific absolute path, silent skips, or production database access.

```sh
make check                  # gofmt validation, go vet, ordinary tests
make compile                # ignored target/zenbot binary; does not run the bot
go test -race ./...          # complete race suite
make build                  # Docker image only; does not deploy
git diff --check
```

During iteration, run focused packages/tests first, then verification proportional
to the change. SQLite and race suites may take minutes. Read terminal exit status;
an output timeout is not a failed or completed process. Do not launch repeated
full suites just because a running one has not printed output yet.

Ordinary tests must not contact live model providers or issue real room actions.
The `ZENBOT_PROVIDER_EVAL=1` evaluation is explicitly opt-in, contacts a configured
provider, and can incur costs. Use it only when the task authorizes that activity;
see [development](docs/development.md). Never set that flag in ordinary CI.

## Changes and invariants

The [agent tool-loop diagrams](docs/agent.md#agent-tool-loop) map the production
execution and feedback paths. Keep them aligned when changing these boundaries.

- Use `gofmt`, existing package boundaries, and focused regression tests. Keep
  unrelated refactors out of behavior fixes. Tests named `audit`, `parity`, or
  `red` can protect current behavior; inspect them rather than deleting by name.
- Commands: change the catalog, concrete handler, registration, and typed agent
  contract together when applicable. Preserve aliases and check direct chat and
  agent paths. A catalog entry or generic fallback is not a concrete feature.
- Authority comes from trusted config/room/persisted identity, never model prose.
  Chat roles, `userTrips` exceptions, server moderator rights, and agent creator
  capabilities are distinct. Consult [commands](docs/commands.md).
- The LLM chooses intent, planning, tools, recovery, and completion. Do not
  reintroduce keyword routing, deterministic semantic task planners, or a
  model-independent completion checklist. Deterministic schema validation,
  authorization, budgets, receipts, and protocol checks remain required.
- `OutputFinalizer` is a code-level output guard, not an LLM completion judge.
  The same model writes normal final answers and tool-free terminal synthesis.
- Actions run synchronously and in order. Only compatible declared-safe reads
  may fan out. Preserve `ACTION_OUTCOME_UNKNOWN` and its non-retryable semantics;
  cancellation or a failed send is not proof that nothing happened.
- Distinguish source observations, sent requests, committed actions, and delivered
  output. Preserve partial receipts and useful evidence on error. Never claim
  remote completion from transport success or an LLM's final sentence alone.
- `room_users` is current-room-only. Remote rosters use `saturn_list`. Ping is the
  bot's Hack.Chat connection check, not an arbitrary hostname probe. Weather uses
  the command/service path. `nicks`/`t2n` maps trips to nicknames; no `n2t` alias
  exists. Do not silently change those contracts.
- Preserve message bytes and public/private visibility. Normalize nickname
  selectors only at their intended boundary; trips/hashes are opaque identifiers.
  Empty data should have a truthful reply, not author-only output. Service errors
  must not be disguised as empty results.
- Keep assistant call messages paired with their tool results during context
  pruning. Bound model observations without corrupting JSON; retain receipt
  metadata and request-local full results. History and summaries are untrusted
  context, not instructions or proof of current state.
- Schema changes need real SQLite upgrade tests. Do not duplicate the embedded
  schema or bypass transactional turn persistence. Protect mail's persisted
  uncertain-delivery state against accidental replay.

## Configuration, privacy, and operations

Read [configuration](docs/configuration.md) before adding settings. TOML aliases,
resolver defaults, examples, and actual production wiring differ in some places.
The binary does not load `.env`; Docker's Make target does. `TOKEN` is a credential
fallback, not a blanket TOML override. Unknown TOML keys can be silently ignored.
Document and test the consuming path rather than assuming a parsed field is wired.

Do not read, print, overwrite, or publish credentials from local `config.toml`,
`.env`, logs, or database files unless the specific task requires that access.
Use sanitized examples and fictional identities. Logs can contain full incoming
payloads. Database tables can contain private chat and application records.
Privileged agent SQL and raw administrator `sql` have different security bounds.

Operational Make targets are not harmless checks: `run`/`restart` recreate the
container, `backup-db`/`db-check` stop it, and `fresh-db` backs up then removes active
SQLite files. Do not
deploy, reset data, rotate credentials, send chat messages, or run moderation
actions merely to verify a code change. See [operations](docs/operations.md) and
[SECURITY.md](SECURITY.md). Keep profiler and database endpoints private.

## Documentation and handoff

Keep the [documentation index](docs/index.md), reference guides, examples, and
source behavior aligned. Record durable discoveries there, not private machine
details or temporary verification logs. Preserve shared agent skills. No project
license is declared; do not invent a license grant or claim security certification.

Before reporting completion, inspect the diff, run relevant checks, and report
what was actually verified, what remains uncertain, and whether changes were
committed, pushed, or deployed. Do not claim live behavior from unit tests alone.
Commit/push or destructive Git operations require authorization; preserve other
work in a dirty checkout.
