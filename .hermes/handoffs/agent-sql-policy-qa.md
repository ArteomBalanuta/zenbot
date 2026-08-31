# Agent SQL Policy QA — Rows #86–#90

**Repository:** `/Users/ab/workspace/go-projects/zenbot`  
**Reference source:** `/Users/ab/workspace/projects/saturn` (read-only; SQL-policy source/tests inspected)  
**Disposition:** **PASS for the bounded SQL-policy slice, with one genuine defect fixed; overall migration remains NOT COMPLETE.**

## Scope

Audited only `internal/agent/sql/policy.go` and `internal/agent/sql/policy_test.go`, against the Saturn SQL policy/error/value implementation and `JSqlParserAgentSqlPolicyTest.java`, plus the SQL architecture/implementation handoffs. SQL execution, H2/repositories/resources, database tools/registry/callers, config wiring, providers/listeners/router/commands, and broader migration closure were excluded.

## Findings and fix

- **Fixed:** `SELECT ... FOR UPDATE` (and other row-locking SELECTs) parsed successfully and bypassed the read-only policy because the JSON walker checked `intoClause` and `valuesLists` but not `lockingClause`. Saturn’s architecture contract explicitly requires row-locking rejection. The policy now maps any non-nil `SelectStmt.lockingClause` to `FORBIDDEN_STATEMENT`.
- Added regression coverage in `TestSQLPolicyRejectsBlankAndWrites` for `SELECT * FROM messages FOR UPDATE`.
- No other bounded defect was found in the inspected pinned-parser behavior. The generic JSON walker recursively visits the current pg_query v6.1.0 representation, including CTEs, subqueries, set-operation arms, expressions, `RangeFunction`/nested `FuncCall`, and `RangeTableSample` relations. Future parser-node shapes remain a parser-version review limitation, not an exercised current bypass.

## Audit evidence

- **Closed parity:** Go defines exactly Saturn’s nine codes: `EMPTY_SQL`, `SQL_TOO_LONG`, `MALFORMED_SQL`, `FORBIDDEN_STATEMENT`, `FORBIDDEN_TABLE`, `FORBIDDEN_FUNCTION`, `TIMEOUT`, `RESULT_TOO_LARGE`, `EXECUTION_FAILED`. Pure validation emits only the first six.
- **Typed errors:** `AgentSqlPolicyError` supports `Error`, `Unwrap`, `CodeValue`, `errors.Is`, and `errors.As`; constructor rejects unknown codes and blank messages. Public messages are fixed and do not include SQL/parser detail; parser/invalid-UTF-8 causes remain wrapped privately.
- **Input/length:** empty, whitespace/comment-only, invalid UTF-8, malformed SQL, and over-limit SQL are distinct and safe. Length uses `utf8.RuneCountInString` (Unicode code points), and original SQL is retained.
- **Statement policy:** exactly one parsed statement; only root `SelectStmt`; `VALUES`, writes, utility/unsupported statements, `INTO`, modifying CTEs, multiple statements, and now row locking reject as `FORBIDDEN_STATEMENT`. `attach`, `detach`, `pragma`, and `vacuum` parser failures preserve Saturn’s unsupported-leading-keyword mapping.
- **Traversal/security:** recursive AST-JSON walking covers nested queries, unions, joins, CTE bodies/aliases, table references, ordinary functions, and table functions. Unknown/internal/schema- or catalog-qualified tables reject as `FORBIDDEN_TABLE`; dangerous `load_extension`, `readfile`, `writefile`, and `pragma_*` reject as `FORBIDDEN_FUNCTION`. Quoted identifiers are normalized for double-quote, backtick, and bracket forms; current probes confirmed safe quoted allowlist behavior and dangerous nested/table-function rejection.
- **Value/fingerprint:** successful values preserve exact input and compute lowercase 64-hex SHA-256 over original UTF-8 bytes.
- **Compatibility/wiring:** legacy `Policy.Validate` delegates to an unconditionally read-only policy with an empty schema; `AllowWrite` cannot broaden acceptance. Literal call-site search found no production compatibility callers. No SQL tool, execution path, registry, or feature wiring was added.

## Actual verification

All commands were run in `/Users/ab/workspace/go-projects/zenbot` after the fix:

- `gofmt -w internal/agent/sql/policy.go internal/agent/sql/policy_test.go` — PASS.
- `go test -count=1 ./internal/agent/sql` — PASS (`0.386s`).
- `go test -race -count=1 ./internal/agent/sql` — PASS (`1.409s`); macOS emitted a non-fatal linker `malformed LC_DYSYMTAB` warning.
- `go test -count=1 ./...` — PASS; all packages green, SQL package `0.863s`.
- `go test -race ./...` — PASS; same non-fatal macOS linker warning.
- `go vet ./...` — PASS, no output.
- `go build ./...` — PASS, no output.
- `git diff --check` — PASS, no output.

## Changed-file attribution

This QA pass changed only:

- `internal/agent/sql/policy.go` — row-locking rejection.
- `internal/agent/sql/policy_test.go` — row-locking regression vector.
- `.hermes/handoffs/agent-sql-policy-qa.md` — this report.

`agent-sql-policy-architecture.md` and `agent-sql-policy-implementation.md` were already present as untracked handoffs before this QA pass and were read but not modified. Existing unrelated target worktree changes, including `internal/agent/turn/**`, were not edited.

The Saturn checkout was not modified by this pass in any SQL-policy file. Its working tree was already dirty in unrelated historical files (`.saturn_gap_fix_*`, `.target3_registry_*`, weather/listener artifacts); therefore a whole-checkout clean-diff assertion is not possible. No Saturn SQL-policy diff was observed or introduced.

## Limitations and exclusions

- This is not an overall migration-completion claim.
- No SQL was executed and no H2/repository/tool/provider/listener/router/command integration was tested or changed.
- The implementation is tied to the inspected `pg_query_go/v6` JSON shape. A future parser version introducing security-relevant node shapes requires renewed fail-closed review before widening acceptance.
- Saturn’s own unrelated dirty checkout prevented claiming that its entire working tree was clean; SQL-policy source/test paths remained read-only for this task.
