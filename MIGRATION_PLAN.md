# Saturn → Zenbot Strict-Parity Migration Plan

> **For Hermes:** Implement this plan slice-by-slice, preserving unrelated dirty-worktree changes and requiring the gates below before closure.

**Goal:** Complete the Saturn Java → Zenbot Go migration with strict observable parity, H2-only persistence, and evidence for every audited source unit, SQL occurrence, repository/service method, schema object, command, listener, and agent behavior.

**Source:** `/Users/ab/workspace/projects/saturn`, branch `develop` (read-only)

**Target:** `/Users/ab/workspace/go-projects/zenbot`, branch `master`

**Current verdict:** **NOT COMPLETE.** The verified audit records 325 Saturn Java source units, 12 SQL tables, 18 indexes, 197 SQL query occurrences, 88 repository/service methods, and 71 Zenbot Go files. The existing target baseline passed `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`; those are baseline health gates, not migration-completion evidence.

### Execution-priority override — rapid command and agent parity

This section is the active implementation priority. It supersedes the exhaustive test-first and bug-hunting cadence elsewhere in this plan until the command and live-agent capabilities below are present. Preserve Saturn-observable behavior where it is implemented, but prioritize shipping the missing capabilities over expanding audit, SQL, or hardening scope.

1. **First:** implement the missing command behavior identified by the command inventory: remaining admin (`memory`, `mine`, `prefix`, lifecycle), moderator (activity, automove, captcha, color/flair, mute/shadow-ban/nuke/overflow/resurrect), remote-room/Whiskey, replica, DBZ, user last-online, and `l` command paths. A registered alias, generic acknowledgement, no-op, or “not configured” response is not command parity.
2. **Second:** make the Saturn agent package live: construct it from runtime configuration, wire participation/relay and `l` into the listener/command path, then add the minimum required room automation, routing, tools, memory, and cancellation behavior for real requests. The existing private `internal/agent/**` packages and copied resources are foundation only, not completion.
3. **Third:** connect remote-room/session/replica/Whiskey behavior and the listener ordering needed by those commands. Reuse the existing target snapshot, replica, transport, and service foundations rather than redesigning them.
4. **Testing policy:** require one focused baseline test per delivered capability/slice plus the relevant package test and a final `go test ./...` before accepting a batch. Do not block rapid migration on extensive unit-test matrices, repeated stress suites, race sweeps, broad test-only refactors, or speculative hardening unless a concrete failure blocks functionality.
5. **Bug-hunting policy:** defer proactive bug hunting, forensic excursions, and unrelated cleanup. Record discovered defects, but fix them only when they block the active parity capability, corrupt its observable behavior, or prevent the baseline gate.

#### SQL implementation shortcuts — explicitly deferred

For this rapid-parity phase, use the existing H2 repository/schema/service seams and add only SQL needed to make the active command or agent capability function. The following remain documented debt rather than blockers for command/agent delivery: exhaustive mapping of all 197 SQL occurrences, metadata/index audits, migration/backfill completeness, broad SQL-policy hardening, SQLite-elimination cleanup, and exhaustive transaction/error-edge coverage. Do not invent a parallel persistence layer; route new capability work through the existing H2 target boundary and return to these deferred SQL obligations after command and agent live integration are established.

### Current status checkpoint

- The user has approved this mapping plan and explicitly requires migration of **all Saturn commands**, including replica, remote-room, whiskey, prefix, DBZ, lifecycle, admin, moderator, user, and agent-backed commands. Previously identified foundation blockers are scope risks to resolve, not permanent exclusions.
- Saturn command audit: **64 command definitions, 64 concrete command classes, and 161 aliases**. Zenbot's catalog contains the audited definitions, but only the verified concrete implementations are runtime-registered; generic placeholder behavior is not counted as migrated.
- Accepted and independently verified Zenbot slices: utility commands; mail/notes; weather/time; say/afk/list; safe moderation subset; info; users/nicks; help/howto; subscriptions; private snapshot coordinator; private agent runtime; prompt catalog/resources; provider-neutral LLM contracts and OpenAI-compatible adapter; private request assembler.
- Subscription parity was accepted with exact public/whisper acknowledgements, in-memory state, case-insensitive matching, joined-user inclusion, forced-whisper notifications, deterministic H2 rendering, and focused/full verification.
- Private agent boundaries remain unwired from live command dispatch: `l` is not exposed until the remaining runtime, tools, memory, lifecycle, routing, and persistence behavior is migrated and verified.
- Active implementation slice: identity registration/authorization and last-message behavior for `register/reg`, `authorize/auth`, `grant/access`, and `messages/lastmessages`, using the existing H2, service, authorization, and dispatch foundations. These aliases remain unregistered until their concrete implementation and focused tests pass.
- Required next foundation slices include complete transport/session/replica lifecycle, remote-room operations, whiskey replica management, prefix propagation, DBZ persistence/services/commands, remaining moderation/admin/user commands, full listener ordering, and final agent integration.
- No application-code changes from this checkpoint are considered complete without focused parity evidence, real-H2 coverage where applicable, race/vet/build verification, and confirmation that unrelated dirty-worktree changes remain intact.

### Consolidation / accepted work

The following evidence is accepted only for the bounded scopes stated here. It does not alter the frozen audit totals, close unrelated rows, or change the overall verdict: **NOT COMPLETE**.

#### Accepted evidence: AgentMentionParser row #56 contract slice

- **Scope accepted:** the bounded `AgentMentionParser` contract for row **#56** only: literal `@` recognition, Unicode/case-insensitive matching, evidenced word boundaries, all-match removal, and the documented cleanup/punctuation behavior.
- **Task-owned target files:** `internal/agent/participation/policies.go` and `internal/agent/participation/policies_test.go`.
- **Source and evidence paths:** `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`, `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/agent/room/AgentMentionParserTest.java`, `.hermes/handoffs/agent-mention-parser-architecture.md`, `.hermes/handoffs/agent-mention-parser-implementation.md`, `.hermes/handoffs/agent-mention-parser-qa.md`, and `.hermes/handoffs/agent-mention-parser-acceptance.md`.
- **Verification recorded by the acceptance artifact:** focused parser tests, focused race tests, the Saturn parser test, full tests, full race tests, vet, build, formatting, and `git diff --check` passed. The acceptance remains limited to the parser contract; it does not accept broader agent-room integration, routing, listener wiring, or any other row.

#### Accepted evidence: bounded SQL Utility Group B repository/runtime sub-scope

- **Scope accepted:** the bounded Group B repository/runtime behavior only: the five constants `DELETE_TRIP_NAMES`, `DELETE_TRIP`, `DELETE_NAME`, `SELECT_NAME_TRIP_REGISTERED`, and `SELECT_LAST_N_MESSAGES`; their typed H2 repository seam, authorized atomic delete behavior, Saturn-shaped reads, the enumerated command routing/dispatch, and the bounded service/factory integration described by the handoffs.
- **Task-owned runtime files:** `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/command/identity_commands.go`, `internal/command/mail_notes.go`, `internal/command/users_nicks.go`, `internal/command/remove.go`, `internal/command/runtime_parity_red_test.go`, `internal/repository/sql_util_group_b.go`, `internal/repository/h2/sql_util_group_b.go`, `internal/repository/h2/sql_util_row324_group_b_test.go`, and `internal/service/services.go`.
- **Task-owned service-integration files:** `internal/service/services.go`, `internal/service/group_b_test.go`, `internal/factory/engine_factory.go`, and `internal/factory/group_b_test.go`. Existing factory/catalog wiring inspected by the runtime slice is not thereby reclassified as newly changed.
- **Exact architecture, implementation, QA, and acceptance paths:** `.hermes/handoffs/sql-util-group-b-authorized-architecture.md`, `.hermes/handoffs/sql-util-group-b-implementation.md`, `.hermes/handoffs/sql-util-group-b-qa.md`, `.hermes/handoffs/sql-util-group-b-acceptance.md`; `.hermes/handoffs/sql-util-group-b-runtime-parity-architecture.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-architecture-qa.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-implementation.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-qa.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-acceptance.md`; and `.hermes/handoffs/sql-util-group-b-service-integration-architecture.md`, `.hermes/handoffs/sql-util-group-b-service-integration-implementation.md`, `.hermes/handoffs/sql-util-group-b-service-integration-qa.md`, `.hermes/handoffs/sql-util-group-b-service-integration-acceptance.md`.
- **Verification recorded by those artifacts:** focused Group B tests, focused runtime-parity tests, full tests, race tests, `go vet`, build, and Make test/vet/build gates passed; the existing macOS `LC_DYSYMTAB` warning during race execution exited 0. The mail evidence remains bounded: no isolated command-level test drove the real unregistered-recipient branch through `Queue`; this does not become an unqualified end-to-end claim.
- **Explicit boundary:** Group B evidence does **not** close all `SqlUtil` behavior. Full row **#324: NOT COMPLETE**; Group A and Group C remain separate; row **#325 (`Util`): NOT COMPLETE**; and the overall migration remains **NOT COMPLETE**. No unrelated command, service, listener, agent, schema, provider, transport, or SQLite-elimination work is accepted.

#### 325-row reconciliation rules

1. Row IDs **1–325** in the exhaustive Java mapping are the sole primary key for the class/unit ledger.
2. Validate exactly 325 row IDs, uniqueness, and contiguity before reconciliation; fail closed on duplicates, gaps, or out-of-range IDs.
3. Attach evidence many-to-many to primary rows. Handoffs, source files, tests, SQL constants, repository methods, and Go files are evidence items, not additional Java rows.
4. Keep separate ledgers and totals for SQL tables, SQL indexes, SQL occurrences, repository/service methods, and Zenbot Go files. Never add those inventories to the 325-row total.
5. Deduplicate evidence by `(row_id, evidence_scope)` and merge overlapping handoff citations; do not increment a status count per handoff or per file.
6. Record partial Group B coverage as a bounded sub-scope; it does not close parent row **#324** or all `SqlUtil` behavior.
7. Recompute totals only from the deduplicated row ledger and assert that status totals sum to exactly 325. Preserve **2 implemented / 10 intentional adaptations / 313 needing implementation** as historical pre-reconciliation context until reconciliation is complete; do not hand-edit replacement totals.
8. Require architecture, implementation, QA, and acceptance evidence for every future status change, each tied to the exact row and evidence scope. Keep Group C, row #325, and all unsupported rows pending/not complete absent independent evidence.

#### Residual-work index by subsystem

This index identifies remaining reconciliation/implementation work without inventing or changing statuses:

| Subsystem | Residual scope to reconcile or implement |
|---|---|
| Agent API, config, memory, and routing | Remaining contracts, configuration, memory, routing, lifecycle, error, and integration behavior outside the bounded row #56 parser contract |
| Commands | Command families and dispatch behavior outside the bounded Group B paths, including replica, remote-room, Whiskey, prefix, DBZ, lifecycle, admin, moderator, user, and agent-backed work |
| Persistence / H2 / SQL | Full row #324 `SqlUtil` behavior, Group A/C boundaries, SQL tables/indexes/occurrences, transactions, migrations, visibility, and repository/service integration |
| Providers, listeners, and transport | Outstanding provider adapters, listener ordering/lifecycles, event delivery, transport boundaries, retries/timeouts, and shutdown behavior |
| Resources | Remaining classpath/layout, defaults, templates, localization/message resources, loading, and malformed/missing-resource behavior |
| Tests, QA, and documentation | Row-ledger reconciliation, focused parity tests, cross-boundary/full/race/vet/build/Make evidence, and synchronized architecture/acceptance records |

No residual index entry is a status claim. Any future status change must first cite architecture, implementation, QA, and acceptance evidence for its exact scope.

---

## 1. Strict-parity contract and working rules

1. Treat `/Users/ab/workspace/go-projects/zenbot/.hermes/migration-audit.md` as the frozen inventory and source of truth for scope. Do not mark a row complete without focused evidence.
2. Preserve Saturn behavior, including aliases, parsing whitespace/case, exact output text, listener ordering and short-circuiting, null/default behavior, time units, retries/timeouts, authorization thresholds, duplicate handling, transaction boundaries, and known quirks/defects.
3. Preserve the security meaning of `messages.visibility`: history and agent queries must retain PUBLIC/WHISPER filtering, room/name/trip scope, and `(created_on,id)` tie ordering.
4. H2 is the executing database. Retain pinned H2 `2.3.232` PostgreSQL-wire-server behavior, verify `SELECT H2VERSION()`, and fail closed if the engine is not H2.
5. Preserve transaction boundaries for trip/name registration, mail, moderation, audit, and agent memory/tool writes.
6. Work only in the target repository. Do not overwrite or revert unrelated dirty changes. Do not add SQLite compatibility as a new abstraction.
7. “Complete” means implemented behavior plus focused tests/evidence, not merely a compiled analogue or a file/class-name match.

## 2. Source-to-target mapping

| Saturn concern | Verified source scope | Exact Zenbot target scope | Required parity |
|---|---|---|---|
| Lifecycle and application wiring | `src/main/java/org/saturn/ApplicationLifecycle.java`, `ApplicationRunner.java` | `cmd/zenbot/main.go`, `internal/core/**` | Startup/shutdown order, fail-closed H2 startup, dependency wiring, retries/timeouts, signal handling, and listener registration order |
| Facade/transport | `org.saturn.app.facade`: `Base`, `Engine`, `EngineType`, `ListenerProfile`; `org.saturn.app.facade.impl`: `Connection`, `EngineImpl` | `internal/common/**`, `internal/core/**`, `internal/transport/**` (create/extend only where target architecture requires) | Connection lifecycle, engine type/profile, message send/update semantics, raw-message capability, output capture, and error behavior |
| Configuration | Agent config classes plus target configuration references audited for SQLite removal | `config.toml`, `internal/agent/**`, `cmd/zenbot/main.go` | Explicit H2 database stem, agent values/defaults/limits, no legacy SQLite flags or runtime semantics |
| Models and DTOs | `org.saturn.app.model`: `MessageAuditEvent`, `Role`, `Status`, `TimeResponse`, `WmoWeatherInterpCodes`; `org.saturn.app.model.dto/**`; payload DTOs | `internal/model/**` | Field meaning, enum values, nullability/defaults, serialization, time units, and payload compatibility |
| Command framework and groups | `org.saturn.app.command/**`; admin, dbz, moderator, and user implementations (audit rows 144–211) | `internal/command/**` | Command catalog/factory, aliases, argument normalization, authorization, exact responses, side effects, and duplicate/error behavior for every command group |
| Listeners and event pipelines | `org.saturn.app.listener/**`, including connection, incoming/info/online/user events, info/message handler chains, and snapshot operations | `internal/listener/**` | Registration and execution order, chain short-circuit rules, snapshot decode, room operations, joins/leaves, message auditing, command dispatch, mail, AFK, previews, and agent participation |
| Services | `org.saturn.app.service/**` and `service.impl/**` (audit rows 285–317) | `internal/service/**` | Implement/adapt all listed services and implementations; preserve the 88 audited methods, return/error contracts, side effects, SQL semantics, and transaction boundaries |
| H2 persistence/schema | `H2Database`, `H2SchemaBootstrapper`, `SqliteToH2Migrator`, `MessageSchemaMigrator`, agent persistence classes | `internal/repository/h2/**`, canonical `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql` | One-for-one schema objects, constraints/indexes, visibility migration, H2-only startup, translated migration behavior, row counts, and metadata-verified semantics |
| Security/moderation | `AuthorizationService*`, moderation agent package, moderator commands, `messages.visibility` policy | `internal/service/**`, `internal/command/**`, `internal/agent/**`, `internal/listener/**`, `internal/repository/h2/**` | Role thresholds, bans/shadow bans/mutes/kicks/overflow/captcha, protected principals, authorization, filtering, auditability, and fail-closed SQL policy |
| Agent packages/resources | `org.saturn.app.agent.api`, `config`, `llm`, `moderation`, `persistence`, `room`, `routing`, `sql`, `tool`, `tool.contract`, `tool.execution`, `turn`, plus `package-info.java` | `internal/agent/**` and repository-backed resources under `resources/**` | Cover all 143 audited agent units (including package-info), config loading, OpenAI-compatible transport, prompts/resources, routing/classification, tools/contracts/execution, cancellation/budgets, turns/freshness, memory, SQL allowlisting/fingerprints, and moderation |
| Utilities | `org.saturn.app.util/**`: `Constants`, `DBZUtil`, `DateUtil`, `IdentityUtil`, `JsonPayloads`, `SeparatorFormatter`, `SqlUtil`, `Util` | `internal/model/**` or `internal/service/**` (as appropriate); SQL methods in `internal/repository/h2/**` | Preserve formatting, hashing/identity, JSON payloads, date/time units, SQL naming/mapping, and utility quirks |

The audit’s class-by-class table remains the exhaustive row-level mapping. Every row currently marked “needs implementation” requires a target implementation and focused verification. The rows marked implemented or intentional target adaptation still require parity tests before closure.

## 3. Persistence and schema obligations

Canonicalize and verify the 12 tables in both exact target resources:

- `internal/repository/h2/schema-h2.sql`
- `resources/schema-h2.sql`

Required tables: `banned_users`, `executed_commands`, `mail`, `messages`, `notes`, `trips`, `names`, `trip_names`, `dbz_characters`, `dbz_stats`, `agent_memory`, and `agent_tool_memory`.

Verify with real H2 metadata/AST checks—not text matching—columns, types, NULL/NOT NULL, identity, CHECK, foreign keys, UNIQUE constraints, indexes, and transaction/visibility behavior. Reproduce all 18 audited indexes, including the five visibility-aware message indexes, mail/notes/executed-command indexes, banned-user indexes, and both agent-memory index pairs. Assert index column order and DESC components using metadata.

Map the 197 audited SQL occurrences to named Go repository methods under `internal/repository/h2/**` (not ad hoc string copies). Cover legacy trip/name/mail/message/ban/note/DBZ queries, agent memory/query/schema/SQL operations, migration queries, visibility/index migration, service SQL, and command SQL. Focused tests must verify result ordering, limits, case behavior, null/default behavior, update counts, generated keys, transaction commit/rollback, and error mapping.

H2 migration requirements:

- Extend `internal/repository/h2/**` for H2 database opening, read-only connections, bootstrap, schema introspection, transactions, agent persistence, and repository methods.
- Retain `internal/repository/h2/database.go` as H2-only after the migration window; remove migration-only `LegacySQLite*` fields/branches.
- Execute and accept the one-time SQLite→H2 migration before deleting migration-only compatibility. Then remove `internal/repository/legacy_sqlite_migrator.go`, migration-only tests in `internal/repository/h2/migration_test.go`, SQLite dependency entries in `go.mod`/`go.sum`, and SQLite config/flags/docs as specified by the audit.

## 4. Ordered implementation slices

### Slice 0 — Inventory freeze and non-regression harness

- Pin the Saturn inventory at the audited source revision and maintain a row ledger for all 325 units, 12 tables, 18 indexes, 197 SQL occurrences, and 88 methods.
- Add/organize focused parity tests without changing application behavior solely to satisfy counts.
- Record baseline results and preserve unrelated target changes.

**Gate:** inventory totals match the audit; no unrelated files are reverted; existing baseline commands remain green.

### Slice 1 — H2 foundation and schema

- Complete H2 open/read-only/bootstrap/transaction primitives in `internal/repository/h2/**`.
- Canonicalize both schema files and verify all 12 tables, constraints, 18 indexes, generated keys, and `messages.visibility`.
- Implement schema migration/backfill/index creation and fail-closed H2 version validation.

**Gate:** real-H2 metadata tests pass for every table/index/constraint and transaction visibility test.

### Slice 2 — Models, common types, config, and transport

- Implement/adapt `internal/model/**`, `internal/common/**`, `internal/core/**`, `internal/transport/**`, and agent configuration in `internal/agent/**`.
- Wire explicit H2 stem configuration and remove SQLite flags from `cmd/zenbot/main.go` only after migration acceptance is available.
- Verify payload/enum/format compatibility and connection/output-update behavior.

**Gate:** serialization, config-default, transport lifecycle, and startup/shutdown tests pass.

### Slice 3 — Repository methods and SQL coverage

- Implement named H2 methods under `internal/repository/h2/**` for all 197 SQL occurrences and all repository-facing portions of the 88-method inventory.
- Preserve SQL semantics through typed parameters, exact ordering/limits, visibility predicates, generated IDs, and transaction boundaries.
- Add focused tests per query family and method, including failure/error codes.

**Gate:** every SQL ledger row has an exercised Go method and focused parity evidence; no unowned occurrence remains.

### Slice 4 — Services and security/moderation

- Implement/adapt `internal/service/**` for Agent, Authorization, DBZ, DataBase, Log, Mail, Mod, Note, Ping, SCP, SQL, Search, User, Weather, and their audited implementations.
- Implement moderation and authorization paths across `internal/service/**`, `internal/command/**`, `internal/listener/**`, and `internal/agent/**`.
- Verify bans/shadow bans/mutes, role grants, protected principals, captcha/lock state, audit logging, and public/whisper visibility filtering.

**Gate:** all 88 method rows have behavior tests; security tests prove forbidden reads/actions fail closed and authorized paths preserve thresholds.

### Slice 5 — Command catalog and command groups

- Complete `internal/command/**`: base interface/implementation, aliases, factory/catalog, then admin, DBZ, moderator, and user groups.
- Preserve exact command names, aliases, parsing, whitespace/case handling, authorization, output text, side effects, duplicate behavior, and error responses.
- Include commands represented by the audit, including registration, mail/notes, moderation, replica/admin, DBZ, SQL/activity, and user utility commands.

**Gate:** command catalog completeness and alias matrix pass; end-to-end command tests cover each group and all state-changing commands.

### Slice 6 — Listeners, snapshots, lifecycle ordering

- Implement `internal/listener/**` interfaces, event listeners, info/message handler chains, and snapshot operations.
- Wire connection and listener ordering through `internal/core/**` and `cmd/zenbot/main.go`.
- Verify short-circuit behavior, snapshot decode/operation outcomes, joins/leaves, message dispatch, audit/mail/AFK/preview handling, and agent relay ordering.

**Gate:** deterministic listener-order and chain-short-circuit tests pass, including reconnect/startup/shutdown paths.

### Slice 7 — Agent packages, resources, and integration

- Complete `internal/agent/**` for API/config/LLM/provider, moderation, persistence, room automation, routing, SQL policy, tools/contracts/execution, and turn state/policies.
- Add/adapt prompt and contract resources under `resources/**`, including schema/tool definitions and verified quote/prompt catalogs where required by the audited mapping.
- Integrate agent service, memory/tool writes, conversation context, cancellation, budgets, freshness, safe SQL, tool-result rendering, and moderation.

**Gate:** every agent unit—including `package-info`—has a target owner and focused test; real-H2 memory/query/schema/tool tests pass; SQL policy rejects non-read-only or out-of-scope access.

### Slice 8 — SQLite elimination and release closure

- Run the signed one-time SQLite→H2 migration acceptance.
- Delete migration-only implementation/tests, remove `mattn/go-sqlite3`, SQLite config/runtime flags, scripts, Docker/CI references, database backups from normal runtime, and rewrite README/docs H2-only.
- Keep H2-only startup verification and quarantine legacy artifacts outside release/runtime.

**Gate:** all zero-reference commands from the audit return zero output, except the intentional audit/plan wording where the gate is scoped to application/runtime/config/docs artifacts as appropriate; `go list -deps` and `go mod graph` contain no SQLite dependency.

### Slice 9 — Independent QA and closure decision

- Run the complete command suite below and regenerate the row ledger.
- Independently review parity evidence, dirty-worktree preservation, security boundaries, schema metadata, and acceptance artifacts.
- Mark closure only when no pending row or failed gate remains.

## 5. Tests and mandatory gates

Run from `/Users/ab/workspace/go-projects/zenbot`:

```sh
gofmt -w <only files intentionally changed in the implementation slices>
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
rg -n -i "sqlite|sqlite3|database\\.db|ZENBOT_MIGRATE_SQLITE|ZENBOT_LEGACY_SQLITE" .
go list -deps ./... | rg -i "sqlite|mattn/go-sqlite3"
go mod graph | rg -i "sqlite|mattn/go-sqlite3"
```

Expected results:

- `go fmt`, `go vet`, `go test ./...`, `go test -race ./...`, and `go build ./...` succeed.
- The SQLite reference and dependency searches produce zero output at final closure.
- Real H2 tests verify `SELECT H2VERSION()`, schema metadata, index metadata, transactions, row visibility, and fail-closed startup.
- Focused parity tests cover every class/unit row, every SQL occurrence family, every repository/service method, every command group, listener order, transport lifecycle, and agent package/resource contract.
- Security tests prove `messages.visibility` cannot be bypassed by history, agent, tool, or SQL paths.

## 6. Explicit exclusions

- Do not modify Saturn source or claim source changes in the target.
- Do not rewrite unrelated dirty target work, perform broad cleanup, or revert existing user/agent changes.
- Do not add new product behavior, new commands, new schema objects, new permissions, or new agent capabilities absent from the audited Saturn scope.
- Do not preserve SQLite as a normal startup/runtime option after the one-time migration acceptance.
- Do not substitute filename/class-count parity for behavioral evidence.
- Do not close the migration on compilation or baseline tests alone.
- Do not weaken authorization, visibility, SQL read-only policy, transaction boundaries, error contracts, ordering, or known Saturn quirks for convenience.

## 7. Acceptance criteria

The migration is accepted only when all of the following are true:

1. The 325-unit ledger is complete, including `org.saturn.app.agent.package-info` as an explicit row; no pending class/unit row remains.
2. All 12 tables and 18 indexes exist in canonical H2 schema resources with metadata-proven columns, constraints, types, order, and visibility semantics.
3. All 197 SQL occurrences map to tested named Go/H2 methods with preserved behavior; all 88 repository/service methods have focused evidence.
4. Lifecycle, transport, config, models, commands/aliases, listeners/order, services, security/moderation, agent packages/resources, memory, tools, turns, and cancellation are behaviorally integrated under the exact target paths named above.
5. H2 is the only executing database; migration-only SQLite code, dependencies, flags, scripts, docs, and runtime references are removed after signed acceptance.
6. `messages.visibility` remains an enforced security boundary for all relevant history/agent/tool queries, with scope and `(created_on,id)` ordering preserved.
7. Real-H2 integration, focused parity, security, migration, race, vet, format, and build gates all pass.
8. An independent QA pass confirms no failed gate, no unexplained deviation, and no unrelated dirty-worktree change was lost.
9. Final closure updates the audit/ledger with evidence links or test names and changes the verdict from **NOT COMPLETE** only after every criterion passes.
