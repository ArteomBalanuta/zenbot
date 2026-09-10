# Saturn To Zenbot Migration Closure Plan

## Goal And Scope

This branch migrates Saturn's observable bot behavior from Java to Go. It uses
behavioral parity rather than one-Go-file-per-Java-class parity: Java command,
service, facade, and listener classes may be consolidated when their user
contract and state transitions remain equivalent.

The only approved exclusions are `mine`, `whiskey`, `ws`, and `wsa`. They are
kept in the command catalog as explicit exclusions so a future contributor
cannot mistake their absence for accidental drift.

## Closure Matrix

| Area | Saturn behavior retained in Zenbot | Primary evidence | Status |
|---|---|---|---|
| Startup and lifecycle | Saturn config aliases, scalar-list parsing, token handling, autorun, reconnect, health checks, restart/shutdown, host/replica ownership | `cmd/zenbot/startup.go`, `cmd/zenbot/startup_parity_test.go` | Implemented |
| Configuration | Bot, H2, provider, execution, memory, SQL, participation, and moderation values; TOML plus environment precedence | `internal/config/agent_config.go`, `internal/config/saturn_compatibility_test.go`, `internal/config/example_config_test.go` | Implemented |
| Command identity | 64 reviewed definitions and aliases from one catalog, with four explicit exclusions | `internal/command/catalog/catalog.go`, `COMMAND_TOOL_INVENTORY.md` | Implemented |
| Command dispatch | Every in-scope command resolves to a concrete handler; no generic placeholder fallback | `internal/command/production_registry_parity_test.go` | Implemented |
| Authorization | Role checks, configured user-trip exceptions, creator-only permanent bans, and contextual agent capabilities | `internal/command/dispatch_adapter.go`, `internal/command/agent_gateway.go` | Implemented |
| User commands | Help, profile/history, mail, notes, subscriptions, weather, DBZ, room messaging, formatting, and aliases | `internal/command/*_test.go` | Implemented |
| Moderation commands | Authorization, captcha, lock, mute, kick, bans, shadow bans, activity, nuke, resurrect, automove, color/flair, and auditing | `internal/command/*_test.go`, `internal/service/*_test.go` | Implemented |
| Event listeners | Chat/info dispatch, whisper conversion, self filtering, audit, join/leave tracking, AFK rename, YouTube previews, and ordered handler semantics | `internal/listener/**/*_test.go` | Implemented |
| Room state | Atomic snapshots of users, moderators, master, and channel state; join/leave persistence | `internal/core/room_snapshot.go`, `internal/core/room_snapshot_test.go` | Implemented |
| H2 persistence | Embedded schema, H2 2.3.232 identity check, schema upgrades, indexes, visibility, services, and repositories | `internal/repository/h2/**/*_test.go` | Implemented |
| SQLite transition | Read-only one-time import, DDL translation, batched transactional copy, identity reset, index recreation, count verification, and archival | `internal/repository/h2/sqlite_migrator.go`, `internal/repository/h2/migration_test.go` | Implemented |
| Agent runtime | Direct and mention routing, isolated turns, shared room memory, private whisper memory, correction loop, strict contracts, bounded tools, safe read fan-out, and final synthesis | `internal/agent/**/*_test.go`, `AGENTIC_ARCHITECTURE.md` | Implemented |
| Agent command tools | One contextual tool per in-scope command plus capability-filtered `run_command`; normal command pipeline remains authoritative | `internal/command/agent_catalog_test.go`, `internal/agent/tool/saturn_command_test.go` | Implemented |
| Moderation automation | Join-burst/name heuristics, message flood detection, optional semantic moderation, target binding, and configured disable switch | `internal/agent/moderation/**/*_test.go`, `internal/agent/participation/*_test.go` | Implemented |
| Packaging and operations | CGO-enabled Go binary, Java/H2 runtime, checksum-pinned H2 jar, ignored production config, mounted database, lifecycle and backup targets | `Dockerfile`, `Makefile`, `config.example.toml`, `.env.example` | Implemented |

## Design Decisions

- H2 is the only runtime database. SQLite remains solely as a read-only import
  dependency for existing Saturn files.
- The embedded schema at `internal/repository/h2/schema-h2.sql` is the single
  schema source. A second unconsumed copy was removed to prevent drift.
- Public chat memory is shared per room. Whisper memory is isolated by stable
  identity and room.
- Action tools always execute sequentially. Only independent read-only,
  idempotent tools with compatible resource metadata may fan out.
- Commands execute through the same handler and authorization pipeline whether
  invoked by chat or by the model.
- `l` is a direct agent entry point and is never exposed as an agent tool.
- The Go implementation does not reproduce Java package structure where a
  smaller interface or composition boundary expresses the same behavior.

## Required Closure Gates

Run these from the repository root before integration:

```sh
make check
make compile
go test -race ./...
docker build -t zenbot:parity-check .
make -n rebuild
make -n fresh-db
git diff --check
```

Focused parity tests must also continue to enforce:

- every in-scope catalog command builds a concrete handler
- command aliases and agent tools derive from one catalog
- role and creator capabilities change the model-visible manifest
- command actions cannot enter read-only parallel batches
- failed tool observations remain available for model self-correction
- named-user requests can load 500 latest public messages with metadata
- old H2 files upgrade idempotently and legacy SQLite imports preserve rows

## Completion Rule

The migration is complete only when all gates pass and the command inventory
contains no accidental exclusion or placeholder. The four approved exclusions
do not block completion.
