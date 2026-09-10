# Agent SQL Policy Implementation — Rows #86–#90

**Status:** implemented and verified for this bounded slice; overall migration remains **NOT COMPLETE**.

## Scope and files

Task-owned files changed:
- `internal/agent/sql/policy.go` — `AgentSqlErrorCode`, `AgentSqlPolicy`, `AgentSqlPolicyError`, `JSqlParserAgentSqlPolicy`, `ValidatedAgentSql`, compatibility `Policy`.
- `internal/agent/sql/policy_test.go` — focused policy/security tests.
- `.hermes/handoffs/agent-sql-policy-implementation.md` — this evidence handoff.

No config/schema adapter was needed. No Saturn files were changed. Existing unrelated dirty/untracked files were preserved.

## Implementation evidence

- `AgentSqlErrorCode` exposes exactly the nine Saturn values: `EMPTY_SQL`, `SQL_TOO_LONG`, `MALFORMED_SQL`, `FORBIDDEN_STATEMENT`, `FORBIDDEN_TABLE`, `FORBIDDEN_FUNCTION`, `TIMEOUT`, `RESULT_TOO_LARGE`, `EXECUTION_FAILED`.
- `AgentSqlPolicy` is schema-aware: `Validate(sql string, schema Schema) (ValidatedAgentSql, error)` with `Schema.TableNames() []string`.
- `AgentSqlPolicyError` exposes `Code`, safe `Message`, and retained `Cause`; `Error`, `Unwrap`, `CodeValue`, `errors.Is`, and `errors.As` semantics are present. Public validation messages do not contain SQL or parser diagnostics. `NewAgentSqlPolicyError` rejects empty/unknown codes or blank messages.
- `JSqlParserAgentSqlPolicy` uses pinned `pg_query_go/v6` `ParseToJSON` (verified dependency v6.1.0), then validates the parsed AST JSON shape: one statement, root `SelectStmt`, no write/utility node, no `VALUES`, no `INTO`, recursive CTE/query traversal, table allowlist, dangerous function/table-function traversal, and qualified/internal/unknown table rejection. It does not use the old textual write-prefix guard.
- Empty/blank SQL, invalid UTF-8, malformed SQL, over-limit SQL, multiple statements, forbidden statements, tables, and functions map to distinct policy codes. `attach`, `detach`, `pragma`, and `vacuum` parser failures map to `FORBIDDEN_STATEMENT` for Saturn parity.
- Length uses `utf8.RuneCountInString`, preserving Unicode code-point semantics. The configured limit is positive-only in effect (`<=0` means unlimited for this injectable pure policy constructor); no SQL feature is enabled or wired.
- Successful values preserve the exact original SQL and compute lowercase SHA-256 over original UTF-8 bytes via `hex.EncodeToString`.
- Quoted identifiers are normalized for schema names and policy comparisons; backtick/bracket identifier syntax is translated only for parser probing while original SQL/fingerprint remain unchanged.
- Compile-time assertions verify `JSqlParserAgentSqlPolicy` implements `AgentSqlPolicy` and the test schema implements `Schema`.
- Compatibility `Policy.Validate` remains read-only and delegates to the new policy; `AllowWrite` is not used to broaden acceptance.

## TDD RED/GREEN evidence

RED was observed before implementation:

```text
go test ./internal/agent/sql
FAIL ... undefined: AgentSqlErrorCode
FAIL ... undefined: AgentSqlPolicyError
FAIL ... undefined: NewJSqlParserAgentSqlPolicy
FAIL ... undefined: ValidatedAgentSql
```

During the first GREEN run, focused tests exposed two real behavior defects:

```text
--- FAIL: TestSQLPolicyRejectsBlankAndWrites
    accepted "VALUES (1)"
--- FAIL: TestSQLPolicyUnicodeLength
    error type <nil>
```

Fixes were AST-JSON `valuesLists` rejection and correcting the test boundary to one code point over the 10-code-point query (`max=9`). The focused suite then passed.

## Verification output

```text
gofmt -w internal/agent/sql/policy.go internal/agent/sql/policy_test.go

go test -count=1 ./internal/agent/sql
ok   zenbot/internal/agent/sql  0.447s

go test -race -count=1 ./internal/agent/sql
ok   zenbot/internal/agent/sql  1.424s
(macOS linker emitted a non-fatal malformed LC_DYSYMTAB warning.)

go test -count=1 ./...
PASS — all packages, including zenbot/internal/agent/sql

go test -race ./...
PASS — all packages, including zenbot/internal/agent/sql
(macOS linker emitted the same non-fatal LC_DYSYMTAB warning.)

go vet ./...
PASS — no output

go build ./...
PASS — no output

git diff --check
PASS — no output
```

## Source references

Architecture contract: `.hermes/handoffs/agent-sql-policy-architecture.md`, especially §§2–5 and focused matrix §6.
Pinned parser APIs: `github.com/pganalyze/pg_query_go/v6` v6.1.0; `ParseToJSON` was probed against `SELECT`, `VALUES`, CTE, dangerous function, table-function, and multi-statement inputs before implementation.
Saturn parity references named by architecture: `src/main/java/org/saturn/app/agent/sql/AgentSqlErrorCode.java`, `AgentSqlPolicy.java`, `AgentSqlPolicyException.java`, `ValidatedAgentSql.java`, `JSqlParserAgentSqlPolicy.java`, and `src/test/java/org/saturn/app/agent/sql/JSqlParserAgentSqlPolicyTest.java` in read-only checkout `/Users/ab/workspace/projects/saturn`.

## Interruption and limitations

- A temporary probe-edit command targeting `/tmp/probe.go` was denied by the environment. It was not retried, and it did not modify the repository. Existing probe output was used instead.
- The implementation uses the verified `ParseToJSON` representation rather than generated protobuf getter dispatch. It explicitly rejects all known modifying/utility node shapes and recursively inspects all represented nested values; future parser node shapes not represented by the current v6.1.0 JSON should be treated as a parser-version review item before widening acceptance.
- The pure policy constructor accepts an integer limit directly; existing `AgentSqlConfig` was intentionally not changed, and execution-only codes remain reserved. SQL execution, H2/repositories, tools/registries/callers, providers/listeners/router/commands, and feature enabling are explicitly excluded.
- No claim is made that the overall migration is closed.
