# Next Migration Slice Diagnostic — Agent SQL Policy Boundary

**Date:** 2026-08-30  
**Target:** `/Users/ab/workspace/go-projects/zenbot`  
**Source:** `/Users/ab/workspace/projects/saturn` (`develop`, read-only)  
**Decision:** Select the bounded **agent SQL validation/policy contract** as the next safe implementation slice. This is a pure, deny-by-default security boundary that can reuse the existing Go parser dependency and accepted tool/execution contracts without activating live providers, listeners, commands, persistence, or autonomous routing.

> This diagnostic does not claim overall migration completion. The frozen audit and migration plan remain **NOT COMPLETE**.

## 1. Selection rationale

**[OBSERVED]** The migration plan explicitly places SQL policy in the remaining agent work (Slice 7), while the audit marks the five `org.saturn.app.agent.sql` units as `needs implementation`. The current target has a parser-backed `internal/agent/sql/policy.go`, but no focused tests or complete Saturn contract.

The next bounded gap with the best implementation shape is audit rows **#86–#90**: five closely related `org.saturn.app.agent.sql` units. They form one cohesive policy boundary, have direct Saturn source and focused tests, and can be tested without external services. They are also a prerequisite for safe `database_sql` tooling, but this slice must stop at validation and must not enable that tool.

A pure SQL policy slice is preferred over broad listener/router wiring because it has:

- a narrow input/output contract;
- existing target package ownership (`internal/agent/sql`) and an existing PostgreSQL parser dependency;
- a source test suite with concrete acceptance vectors;
- a security-sensitive deny-by-default boundary whose incomplete target implementation should not be silently treated as parity;
- no need to modify Saturn, schema resources, H2 startup, or application wiring.

## 2. Selected frozen audit rows and files

### Selected audit rows

From `.hermes/migration-audit.md:109-113`:

| Audit row | Saturn unit | Saturn evidence | Current target owner/status |
|---:|---|---|---|
| #86 | `AgentSqlErrorCode` | `src/main/java/org/saturn/app/agent/sql/AgentSqlErrorCode.java` | `internal/agent/sql` has no equivalent closed error-code contract |
| #87 | `AgentSqlPolicy` | `src/main/java/org/saturn/app/agent/sql/AgentSqlPolicy.java` | no target interface accepting SQL plus a target schema |
| #88 | `AgentSqlPolicyException` | `src/main/java/org/saturn/app/agent/sql/AgentSqlPolicyException.java` | no typed policy exception/code surface |
| #89 | `JSqlParserAgentSqlPolicy` | `src/main/java/org/saturn/app/agent/sql/JSqlParserAgentSqlPolicy.java` | `internal/agent/sql/policy.go` is only a partial parser/prefix guard |
| #90 | `ValidatedAgentSql` | `src/main/java/org/saturn/app/agent/sql/ValidatedAgentSql.java` | no validated SQL value carrying the original SQL and fingerprint |

The audit marks all five rows `needs implementation`; none is closed merely by the existence of the current Go file.

### Target files to inspect/modify in a future implementation pass

- `internal/agent/sql/policy.go` — extend or split into the policy, error, and validated-value contract while preserving package ownership.
- `internal/agent/sql/policy_test.go` — new focused table-driven parity/security tests.
- `internal/config/agent_sql_config.go` — inspect and, only if necessary, extend configuration with the Saturn-required SQL character limit while preserving existing config compatibility.
- `internal/agent/persistence/schema.go` — adapt only the schema-name contract required by policy; do not mistake this structural model for an H2 schema inspector.
- `internal/agent/tool` callers — **not in this slice**; future database SQL tooling may consume the policy only after this contract and H2/visibility boundaries are accepted.

Do not modify `internal/repository/h2/**`, schema resources, listener/command registration, `cmd/zenbot/main.go`, or Saturn for this slice unless a separate approved dependency decision is made.

## 3. Saturn evidence

### Source contract

`src/main/java/org/saturn/app/agent/sql/AgentSqlPolicy.java` is a functional interface with `ValidatedAgentSql validate(String sql, AgentDatabaseSchema schema)`.

`AgentSqlErrorCode.java` defines the closed codes:

`EMPTY_SQL`, `SQL_TOO_LONG`, `MALFORMED_SQL`, `FORBIDDEN_STATEMENT`, `FORBIDDEN_TABLE`, `FORBIDDEN_FUNCTION`, `TIMEOUT`, `RESULT_TOO_LARGE`, and `EXECUTION_FAILED`.

`AgentSqlPolicyException.java` carries an `AgentSqlErrorCode`, message, and optional cause. `ValidatedAgentSql.java` is an immutable record of non-null `sql` and `fingerprint`.

`JSqlParserAgentSqlPolicy.java` provides the concrete behavior:

1. Reject null/blank SQL as `EMPTY_SQL`.
2. Enforce `config.maxSqlChars()` by Unicode code-point count as `SQL_TOO_LONG`.
3. Parse exactly one statement; malformed syntax is `MALFORMED_SQL`, parser-unsupported leading keywords (`attach`, `detach`, `pragma`, `vacuum`) are `FORBIDDEN_STATEMENT`.
4. Permit only a `SELECT`; reject `VALUES`, table statements, and all non-SELECT forms as `FORBIDDEN_STATEMENT`.
5. Recursively reject data-changing/invalid CTE shapes.
6. Resolve referenced tables and compare normalized names against the supplied `AgentDatabaseSchema`; unknown or internal tables are `FORBIDDEN_TABLE`.
7. Reject dangerous functions `load_extension`, `readfile`, `writefile`, and `pragma_*` as `FORBIDDEN_FUNCTION`.
8. Preserve the original SQL string and return a lowercase hexadecimal SHA-256 fingerprint of its UTF-8 bytes.
9. Normalize supported quoted identifiers (`"name"`, `` `name` ``, `[name]`) and compare case-insensitively.

### Saturn focused tests

`src/test/java/org/saturn/app/agent/sql/JSqlParserAgentSqlPolicyTest.java` is present and directly covers:

- accepted `SELECT`, joins, subqueries, unions, CTEs, and `SELECT 1`;
- rejection of insert/update/delete/create/drop/values/pragma/attach/detach/vacuum;
- unknown/internal tables;
- dangerous functions and pragma table functions;
- multiple statements, including two `SELECT`s;
- distinct blank, malformed, and overlong error codes;
- quoted identifiers;
- stable 64-character lowercase SHA-256 fingerprints and preservation of the SQL text.

`H2AgentSqlRepositoryTest.java` and `SaturnAgentToolsTest.java` show the validated SQL value is later consumed by the agent SQL repository/tool boundary, but that execution/tool integration is intentionally outside the selected slice.

## 4. Current Go gap

`internal/agent/sql/policy.go` currently defines only:

- `Policy{MaxRows int, AllowWrite bool}`;
- `Policy.Validate(query string) error`;
- whitespace rejection;
- `pg_query_go/v6` parse success as the only syntax check;
- a case-insensitive textual prefix check for `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, and `TRUNCATE` when writes are disabled.

Observed gaps relative to Saturn:

- no `AgentSqlErrorCode` equivalent or typed exception preserving a stable code/cause;
- no `AgentSqlPolicy` interface taking a schema;
- no `ValidatedAgentSql` result or SHA-256 fingerprint;
- no exact-one-statement enforcement;
- no SELECT-only AST policy (text prefixes can be bypassed by other statement forms, comments, CTEs, or unsupported constructs);
- no recursive CTE/data-changing-CTE policy;
- no referenced-table extraction or schema allowlist;
- no internal/unknown table rejection;
- no dangerous-function rejection;
- no quoted-identifier normalization;
- no Unicode code-point SQL-length limit matching Saturn;
- no Saturn distinction between empty, malformed, forbidden statement, forbidden table, and forbidden function;
- no focused tests (`go test ./internal/agent/sql` currently reports `[no test files]`).

`internal/config/agent_sql_config.go` currently exposes only `Enabled`, `MaxRows`, and `TimeoutMillis`; it does not expose Saturn's `maxSqlChars`. `internal/agent/persistence/schema.go` is only `Column`, `Table`, and `Schema` data structures and is not an H2 metadata/schema execution layer. The accepted Stage A/B handoff explicitly says SQL/database policy and concrete database tools remain outside that accepted execution slice.

## 5. Scope and non-scope

### In scope

- Implement the five selected SQL policy rows as provider-neutral, request-local validation values and errors.
- Reuse the existing `internal/agent/persistence.Schema` shape or add the smallest explicit schema-name adapter required for allowlisting.
- Preserve input SQL exactly in the validated value.
- Implement deterministic table/function/statement checks and SHA-256 fingerprinting.
- Add focused tests mirroring every Saturn test category, including error-code identity and quoted identifiers.
- Keep policy deny-by-default and independent of network, H2 connections, tools, providers, listeners, and live routing.

### Explicitly out of scope

- Executing SQL or implementing `AgentSqlRepository`, H2 read-only connections, schema introspection, memory persistence, or transactions.
- Implementing/activating `DatabaseSqlTool`, `DatabaseQueryTool`, `DatabaseSchemaTool`, or any tool registry changes.
- `UserMessageHistoryTool` or message visibility/scope behavior; that requires the accepted H2 repository and security boundary, including PUBLIC/WHISPER and `(created_on,id)` ordering.
- Live provider/listener/router wiring, `l` activation, command execution, moderation actions, remote-room operations, and Whiskey proxy behavior.
- Changes to Saturn, canonical H2 schema resources, unrelated command/service files, or broad configuration cleanup.
- Claiming that SQL policy acceptance closes agent persistence, database-tool, turn/router, or overall migration rows beyond #86–#90.

## 6. Risk assessment

**Risk: medium-high despite a small code footprint.** This is a security boundary. A permissive parser or incomplete table/function traversal can turn a nominal validation layer into a bypass.

Primary risks:

1. PostgreSQL `pg_query` AST behavior may differ from Saturn JSqlParser for CTEs, table functions, quoting, unions, and dialect-specific syntax. Establish behavior with tests before choosing adapters.
2. A text-prefix write check is not an AST policy and must not be retained as the sole guard.
3. Table extraction must include joins, subqueries, CTE references, functions/table functions, and quoted identifiers without treating CTE aliases as physical allowlist tables.
4. Fingerprinting must hash the original UTF-8 SQL, not normalized/reformatted SQL, to preserve Saturn’s observed contract.
5. Error messages may contain internal parser details; stable model-visible messages and wrapped causes must be separated.
6. Extending config or persistence structs can accidentally imply live SQL/H2 readiness. Keep the boundary pure and feature-disabled.
7. Existing `pg_query_go` dependency and any parser behavior must be verified in the current target checkout; do not add a second parser without an explicit decision.

## 7. Required architecture, implementation, and QA stages

### Architecture stage

1. Confirm the five audit rows and source/test paths above against the frozen audit and read-only Saturn checkout.
2. Decide whether `pg_query_go/v6` can provide the needed AST traversal. Record unsupported constructs explicitly; do not silently accept them.
3. Define target types: closed error codes, typed policy error with `errors.Is`/cause behavior as appropriate, schema-name input, policy interface, and validated SQL value.
4. Define dialect/normalization rules and a fail-closed policy for parser traversal uncertainty.
5. Record the configuration decision for `maxSqlChars` and ensure no SQL execution or tool activation is part of the change.

### Implementation stage

1. Add the closed error-code and typed error/value contracts in `internal/agent/sql`.
2. Implement exact-one-statement, SELECT-only, CTE, schema allowlist, dangerous-function, quoted-identifier, and Unicode-length checks over the chosen parser boundary.
3. Compute SHA-256 over the original SQL and return the validated value only after all checks pass.
4. Keep public errors stable and safe; retain parser causes internally where the target error contract permits.
5. Add no callers outside the SQL policy package. Do not wire database tools or provider/router paths.

### QA stage

1. Run RED-first focused tests for missing target behavior, then GREEN after implementation.
2. Mirror Saturn vectors for valid reads, all forbidden statement classes, multiple statements, unknown/internal tables, dangerous functions, blank/malformed/overlong input, quoting, and fingerprint stability.
3. Add negative tests for comments, lowercase/mixed-case keywords, CTE aliases, nested subqueries, unions, parser errors, NULL/empty schema, and any parser-specific edge cases.
4. Verify no ordinary write/query/tool path was activated by searching call sites and the registry.
5. Run:
   - `gofmt` on task-owned files;
   - `go test -count=1 ./internal/agent/sql`;
   - `go test -race -count=1 ./internal/agent/sql`;
   - `go test -count=1 ./...`;
   - `go test -race ./...`;
   - `go vet ./...`;
   - `go build ./...`;
   - `git diff --check`.
6. Independently inspect the diff/status and confirm only task-owned SQL policy files plus the handoff changed; Saturn status/content must remain unchanged.

## 8. Acceptance criteria

The slice is accepted only when all criteria below are backed by actual test output and source inspection:

- [ ] Audit rows #86–#90 each have a named Go owner and focused tests.
- [ ] Null/blank, malformed, overlong, forbidden statement, forbidden table, and forbidden function cases produce distinct stable codes matching Saturn.
- [ ] Exactly one read-only `SELECT` is accepted; writes, `VALUES`, unsupported statements, and multiple statements are rejected.
- [ ] Nested queries, joins, unions, CTEs, and quoted identifiers are handled according to the documented policy; no CTE alias is incorrectly required as a physical schema table.
- [ ] Unknown/internal tables and dangerous functions are rejected fail-closed.
- [ ] The validated value preserves the original SQL and carries a stable lowercase 64-character SHA-256 fingerprint of its UTF-8 bytes.
- [ ] Focused normal/race tests, full normal/race tests, vet, build, formatting, and diff checks pass.
- [ ] No database tool, provider, listener, router, command, moderation, remote-room, Whiskey, H2, or Saturn changes are introduced.
- [ ] The overall migration remains explicitly reported as **NOT COMPLETE**.

## 9. Verification of this diagnostic

This artifact was written at `/Users/ab/workspace/go-projects/zenbot/.hermes/handoffs/next-migration-slice-diagnostic.md`. Before final reporting, verify that it is non-empty, contains all required sections, and that every cited existing source/target path exists; `internal/agent/sql/policy_test.go` is intentionally listed as a future implementation file and need not exist yet. Application code and Saturn source must remain untouched by this diagnostic pass.
