# Agent SQL Policy Architecture — Rows #86–#90

**Status:** architecture/design only; no application code or Saturn source is changed by this handoff.
**Scope:** `AgentSqlErrorCode`, `AgentSqlPolicy`, `AgentSqlPolicyException`, `JSqlParserAgentSqlPolicy`, and `ValidatedAgentSql` only.
**Migration verdict:** remains **NOT COMPLETE**. This document does not close persistence, database tools, routing, providers, listeners, H2, or any audit row other than the five named SQL rows after implementation and evidence.

## 1. Evidence and constraints

### [OBSERVED] Target

- `internal/agent/sql/policy.go` currently owns `Policy{MaxRows int, AllowWrite bool}` and `Policy.Validate(query string) error`. It trims whitespace, calls `pg_query.Parse`, and uses a case-insensitive textual prefix check for six write keywords. It has no closed error codes, schema input, validated value, fingerprint, exact-one-statement rule, AST allowlist, table/function checks, or focused tests.
- `internal/agent/persistence/schema.go` currently contains only `Column{Name, Type, Nullable}`, `Table{Name, Columns}`, and `Schema{Tables}`. It is a structural model, not metadata discovery or an H2 execution layer.
- `internal/config/agent_sql_config.go` contains only `Enabled`, `MaxRows`, and `TimeoutMillis`. `internal/config/agent_config.go` embeds it as `AgentConfig.SQL`; no SQL parser/tool caller is wired by this slice.
- `go.mod` and `go.sum` pin `github.com/pganalyze/pg_query_go/v6 v6.1.0`; no second SQL parser is present.

### [OBSERVED] Saturn contract

Authoritative sources are under the read-only Saturn checkout:

- `src/main/java/org/saturn/app/agent/sql/AgentSqlErrorCode.java` defines exactly nine closed values: `EMPTY_SQL`, `SQL_TOO_LONG`, `MALFORMED_SQL`, `FORBIDDEN_STATEMENT`, `FORBIDDEN_TABLE`, `FORBIDDEN_FUNCTION`, `TIMEOUT`, `RESULT_TOO_LARGE`, `EXECUTION_FAILED`.
- `AgentSqlPolicy.java` is a functional interface: `ValidatedAgentSql validate(String sql, AgentDatabaseSchema schema)`.
- `AgentSqlPolicyException.java` is a runtime exception carrying a non-null code, message, and optional cause; `code()` returns the code.
- `ValidatedAgentSql.java` is an immutable non-null `(sql, fingerprint)` value.
- `JSqlParserAgentSqlPolicy.java` rejects blank SQL; counts Unicode code points against `config.maxSqlChars()`; parses exactly one statement; allows only `SELECT`; recursively rejects data-changing/invalid CTE shapes; extracts referenced tables; rejects unknown/internal tables; rejects `load_extension`, `readfile`, `writefile`, and names beginning `pragma_`; preserves original SQL; and hashes original UTF-8 bytes as lowercase SHA-256.
- Identifier normalization in Saturn strips supported `"name"`, `` `name` ``, and `[name]` wrappers and compares using `Locale.ROOT` lower case.
- `AgentDatabaseSchema.java` defensively copies tables and exposes case-insensitive lookup/table names. `AgentSqlConfig.java` has `maxSqlChars` plus execution/result bounds; its defaults include `dynamicSqlMaxSqlChars=4000`, but execution is outside this slice.

### [TEST-BACKED] Saturn vectors

`src/test/java/org/saturn/app/agent/sql/JSqlParserAgentSqlPolicyTest.java` covers accepted simple selects, joins, nested subqueries, unions, read-only CTEs, and `SELECT 1`; rejects insert/update/delete/create/drop/values/pragma/attach/detach/vacuum; rejects internal/unknown tables and dangerous functions/table functions; rejects two statements; distinguishes blank, malformed, and overlong inputs; accepts double-quoted and backtick identifiers; and verifies original SQL plus a stable lowercase 64-character fingerprint.

`MIGRATION_PLAN.md` and `.hermes/migration-audit.md` identify Saturn rows #86–#90 as pending and require focused evidence; `.hermes/handoffs/next-migration-slice-diagnostic.md` explicitly excludes SQL execution, database tools/registry, H2 resources/repositories, provider/listener/router wiring, and Saturn changes.

## 2. Proposed Go contract

### [RECOMMENDED] Closed codes and typed errors

Create a package-owned closed set (Go cannot enforce enum closure at compile time, so expose only named constants and document unknown values as invalid):

```go
type ErrorCode string
const (
    EmptySQL ErrorCode = "EMPTY_SQL"
    SQLTooLong ErrorCode = "SQL_TOO_LONG"
    MalformedSQL ErrorCode = "MALFORMED_SQL"
    ForbiddenStatement ErrorCode = "FORBIDDEN_STATEMENT"
    ForbiddenTable ErrorCode = "FORBIDDEN_TABLE"
    ForbiddenFunction ErrorCode = "FORBIDDEN_FUNCTION"
    Timeout ErrorCode = "TIMEOUT"
    ResultTooLarge ErrorCode = "RESULT_TOO_LARGE"
    ExecutionFailed ErrorCode = "EXECUTION_FAILED"
)
```

Prefer `AgentSqlErrorCode` as the exported Go type if parity naming is important; do not retain a second competing code type. `AgentSqlPolicyError` should contain `Code ErrorCode`, a safe stable `Message`, and `Cause error`, implement `Error() string`, `Unwrap() error`, and expose `CodeValue() ErrorCode` (or `Code() ErrorCode`, provided it does not conflict with a field). Constructors must reject an invalid/empty code. `errors.Is` should work through `Unwrap`; `errors.As` should retrieve the typed error. Model-visible messages must be fixed generic strings (for example, “SQL could not be parsed”); never include SQL text, paths, schema contents, parser dumps, or secrets. Parser errors may remain as causes for logs/tests but must not leak through `Error()`.

Validation uses only `EMPTY_SQL`, `SQL_TOO_LONG`, `MALFORMED_SQL`, `FORBIDDEN_STATEMENT`, `FORBIDDEN_TABLE`, and `FORBIDDEN_FUNCTION`. `TIMEOUT`, `RESULT_TOO_LARGE`, and `EXECUTION_FAILED` remain reserved for the later execution boundary and must not be fabricated by pure validation.

### [RECOMMENDED] Interfaces and values

Use provider-neutral interfaces in `internal/agent/sql`:

```go
type Schema interface { TableNames() []string }
type AgentSqlPolicy interface {
    Validate(sql string, schema Schema) (ValidatedAgentSql, error)
}
type ValidatedAgentSql struct { SQL string; Fingerprint string }
```

A concrete adapter over `internal/agent/persistence.Schema` should be the smallest compatibility bridge: normalize `Schema.Tables[].Name` into a set. Alternatively, add a method on the existing persistence model only if that does not imply discovery or execution. Reject a nil schema as a programmer/configuration error before parsing (or convert it to `FORBIDDEN_TABLE` if the public boundary must never expose a non-policy error); choose and test one behavior consistently. The value must be immutable by convention: preserve the exact input string and a validated lowercase 64-hex fingerprint.

Do not expose `MaxRows`, `AllowWrite`, timeout, result limits, or execution methods on this validation interface. `AllowWrite` is unsafe ambiguity: this boundary is always read-only.

## 3. Validation pipeline and semantics

### [RECOMMENDED] Ordered pipeline

1. **Input:** do not trim the value used for output/fingerprint. Treat nil or `strings.TrimSpace(sql)==""` as `EMPTY_SQL`.
2. **Length:** count Unicode code points with `utf8.RuneCountInString(sql)` (equivalent to Java `codePointCount` for valid UTF-8 Go strings), not bytes and not UTF-16 code units. Invalid UTF-8 should be rejected as malformed or, safer, before parsing; define the chosen result in tests. Compare `>` configured max, matching Saturn.
3. **Parse:** call `pg_query.Parse(sql)` against the original string. Require a non-nil result, exactly one `ParseResult.Stmts` entry, non-nil `RawStmt.Stmt`, and a traversable AST. Parse failure is `MALFORMED_SQL`, except the explicitly documented unsupported-leading-keyword mapping (`attach`, `detach`, `pragma`, `vacuum`) which is `FORBIDDEN_STATEMENT` for Saturn parity.
4. **Statement kind:** inspect the root `Node` oneof, not text prefixes. Accept only `GetSelectStmt()` (and, by explicit AST tests, set-operation SELECT trees represented inside it). Reject `VALUES` and every non-select node as `FORBIDDEN_STATEMENT`. Reject a root utility statement, data-changing CTE, `INTO`, row-locking, or any modifying construct.
5. **Recursive read-only walk:** traverse every nested `SelectStmt`, `CommonTableExpr.Ctequery`, sublink/query node, set-operation arm, from item, expression, and function node. Unknown node shapes in security-relevant positions are uncertainty and must fail closed as `FORBIDDEN_STATEMENT` (or `MALFORMED_SQL` only when the parser itself returned an error).
6. **Function check:** inspect `FuncCall.Funcname` and function expressions/table functions, normalize the final identifier component, and reject exact `load_extension`, `readfile`, `writefile`, or `pragma_` prefix as `FORBIDDEN_FUNCTION`. Function names must be checked even when nested in projections, predicates, subqueries, CTEs, joins, or table-function FROM items.
7. **Table check:** collect physical `RangeVar` relations in all query levels and CTE bodies. Normalize quoted/unquoted names case-insensitively and compare to the supplied schema allowlist. Reject catalog/schema-qualified references unless the schema contract explicitly represents and allows that qualification; safest initial rule is to permit only an unqualified physical table name matching the allowlist. Reject internal names such as `information_schema.tables` through this allowlist. CTE aliases are logical names, not physical tables: maintain a scoped CTE-alias set and do not demand aliases such as `recent` from the physical schema. A reference to a CTE alias is valid only when its definition was recursively validated.
8. **Fingerprint:** after all checks pass, SHA-256 the original `[]byte(sql)` and format lowercase hexadecimal with `hex.EncodeToString`; do not use `pg_query.Fingerprint`, `Normalize`, or deparsed SQL because Saturn hashes original UTF-8 bytes.
9. **Return:** return `ValidatedAgentSql{SQL: sql, Fingerprint: ...}` only on complete success.

### [RECOMMENDED] pg_query_go API boundary

Verified v6.1.0 APIs include `Parse(string) (*ParseResult,error)`, `ParseToJSON`, `SplitWithParser`, `SplitWithScanner`, and `Fingerprint`; use `Parse`, not the fingerprint helper. `ParseResult` has `Stmts []*RawStmt`; `RawStmt` has `Stmt *Node`, `StmtLocation`, and `StmtLen`. `Node` is a protobuf oneof with getters including `GetSelectStmt`, `GetQuery`, `GetRawStmt`, `GetRangeVar`, `GetRangeFunction`, `GetFuncCall`, `GetCommonTableExpr`, and many statement getters.

`SelectStmt` exposes `FromClause`, `WithClause`, `Larg`, `Rarg`, `Op`, `ValuesLists`, `IntoClause`, and clauses. `CommonTableExpr` exposes `Ctename` and `Ctequery`; `RangeVar` exposes `Catalogname`, `Schemaname`, and `Relname`; `FuncCall` exposes `Funcname` and `Args`; `RangeFunction` exposes `Functions`. A robust walker should dispatch the complete relevant protobuf oneof and treat an unhandled non-empty node as uncertainty, rather than recursively guessing through generated internals. Add parser-shape probes before implementation is accepted, because PostgreSQL AST/dialect behavior may differ from Saturn's JSqlParser/SQLite-oriented SQL.

`Parse` accepts PostgreSQL grammar, while Saturn policy is exercised against SQLite/H2-oriented forms. Backticks/bracket quoting and SQLite `pragma_*` constructs therefore require explicit probes. Do not silently widen acceptance when pg_query parses something Saturn would reject.

## 4. CTEs, tables, functions, and quoting

- **CTEs:** validate every CTE body recursively; reject a missing/non-select body and any modifying CTE. Support multiple CTEs and recursive CTE syntax only when every represented arm remains read-only. Scope aliases per query level; aliases shadow physical names only as logical references.
- **Joins/subqueries/unions:** walk every FROM item and nested query. A union is acceptable only when both arms are SELECT/read-only and all physical tables are allowlisted.
- **Tables:** collect `RangeVar` physical relations, not aliases. Include nested scopes and joins. Reject unknown, internal, catalog-qualified, temporary, or parser-ambiguous relations by default. An empty schema therefore accepts `SELECT 1` but rejects any table reference.
- **Functions:** function calls and table functions are separate AST shapes; check both. Normalize the full function identifier conservatively and compare the final component plus `pragma_` rule. Unknown/unhandled function-like nodes fail closed rather than being assumed safe.
- **Quoted identifiers:** normalize `"messages"`, `` `messages` ``, and `[messages]` to `messages`, lower-casing with `strings.ToLower` (Unicode behavior should be documented; ASCII table names are expected). Do not strip arbitrary punctuation or alter embedded escaped quote semantics. Test mixed-case and schema-qualified forms explicitly.

## 5. Configuration decision

### [RECOMMENDED]

Add `MaxSQLChars`/`MaxSqlChars` to `internal/config.AgentSqlConfig` only as the configuration value needed by this pure policy. Preserve existing TOML compatibility (`enabled`, `maxRows`, `timeoutMillis`); do not change defaults for existing fields. Use an explicit positive default matching Saturn (`4000`) and validate the bound. If the current loader does not yet resolve nested SQL values, keep the policy constructor injectable with a concrete limit and defer loader wiring to a separately approved config slice; do not make dynamic SQL enabled or wire tools as a side effect. Record the exact spelling chosen (`maxSqlChars` is the Saturn concept) and test default/override/invalid values.

No SQL is executed. No database tool, registry, repository, provider, listener, router, command, H2 resource, or Saturn file is changed.

## 6. Focused test matrix

Create `internal/agent/sql/policy_test.go` with table-driven tests and error extraction via `errors.As`:

| Area | Required cases | Expected |
|---|---|---|
| Empty | nil, `""`, spaces/tabs/newlines | `EMPTY_SQL` |
| Length | exact limit; one over; multibyte `é`/emoji proving code-point not byte count | success / `SQL_TOO_LONG` |
| Parse | malformed `SELECT FROM messages`; unterminated literal/comment; invalid UTF-8 decision | `MALFORMED_SQL` or documented safe result |
| Statements | insert/update/delete/create/drop/alter/truncate/values; pragma/attach/detach/vacuum; comments/lowercase/mixed case | `FORBIDDEN_STATEMENT` |
| Cardinality | select; select with trailing semicolon; two selects; select plus write; empty/comment-only tail | one accepted only; otherwise forbidden |
| Read-only AST | `SELECT 1`; joins; nested subqueries; unions; nested expressions/functions | accepted when safe |
| CTE | one/multiple read-only CTEs; nested CTE; recursive read-only; data-changing/invalid CTE; CTE alias absent from schema | safe accepted; bad forms forbidden; alias not table-forbidden |
| Tables | allowed; unknown; `information_schema`; nested/joined/union tables; empty schema; qualified names | allowed or `FORBIDDEN_TABLE` |
| Quoting | double quote, backtick, brackets; mixed case; aliases and escaped identifiers | normalized policy behavior |
| Functions | safe nested function; `load_extension`, `readfile`, `writefile`, `pragma_table_info`; table-function position and nested calls | `FORBIDDEN_FUNCTION` for dangerous |
| Error safety | stable public messages do not contain SQL/parser detail; cause retained for parse error; `errors.Is`/`errors.As`; non-null code | contract holds |
| Value | original whitespace/case preserved; 64 lowercase hex; same input stable; different input differs; known SHA-256 vector | exact value |
| Robustness | nil schema; nil AST/protobuf fields; unhandled node; parser-version edge forms | deterministic documented fail-closed behavior |

Also add compile-time interface assertions and verify old `Policy.Validate` callers (currently none found) are not silently broadened. Do not add tests that execute SQL.

## 7. File map and ownership

### [RECOMMENDED] Task-owned

- `internal/agent/sql/policy.go`: replace the prefix guard with the policy implementation or split into `errors.go`, `policy.go`, and `validated.go` if clearer.
- `internal/agent/sql/policy_test.go`: focused parity/security matrix above.
- `internal/config/agent_sql_config.go`: only if the max-character configuration decision is accepted.
- `internal/agent/persistence/schema.go`: only the smallest name adapter/method, if required.

### Explicitly not owned

`internal/repository/h2/**`, `resources/schema-h2.sql`, `cmd/zenbot/main.go`, `internal/agent/tool/**`, tool registries/callers, providers, listeners, routers, commands, persistence execution, and all files in `/Users/ab/workspace/projects/saturn`.

## 8. Risks and mitigations

- **Dialect mismatch (medium-high):** pg_query is PostgreSQL, Saturn parser/tests are JSqlParser with SQLite features. Mitigate with parser probes and deny unknown shapes.
- **Traversal bypass:** generated protobuf trees are broad. Mitigate with explicit oneof dispatch, recursive query/CTE/expression coverage, and fail-closed unhandled-node tests.
- **CTE confusion:** aliases can be mistaken for physical tables. Maintain lexical scopes and validate definitions separately.
- **Function/table-function bypass:** visit both ordinary and FROM-function nodes at every depth.
- **Error leakage:** fixed public messages plus wrapped private causes; never echo SQL.
- **Config regression:** preserve current fields/default behavior and leave feature disabled/unwired.
- **Fingerprint drift:** hash original UTF-8 bytes, not trimmed/normalized/deparsed SQL.

## 9. Acceptance gates

The architecture is implementable only after these gates are planned; implementation is accepted only with actual output proving them:

1. All five rows #86–#90 have named Go owners and focused tests; no Saturn changes.
2. Closed codes exactly match Saturn; validation never emits execution-only codes.
3. Null/blank, malformed, overlong, forbidden statement/table/function cases are distinct and safe.
4. Exactly one AST-validated read-only SELECT is accepted; writes, VALUES, unsupported/uncertain constructs, modifying CTEs, and multiple statements fail closed.
5. CTE aliases, joins, subqueries, unions, table functions, quoted identifiers, internal/unknown tables, and nested dangerous functions have evidence.
6. Original SQL and lowercase 64-character SHA-256 fingerprint are preserved/verified.
7. Run `gofmt`, `go test -count=1 ./internal/agent/sql`, `go test -race -count=1 ./internal/agent/sql`, `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
8. Inspect `git status`/diff and confirm only intended SQL policy/config/schema-adapter files plus this handoff changed; existing unrelated dirty files remain intact.
9. Explicitly verify no database tool, execution, H2, provider, listener, router, command, or broader migration closure was introduced. Overall migration remains **NOT COMPLETE**.

## 10. Evidence limitations

The current target has no SQL-policy tests (`go test ./internal/agent/sql` reports no test files), and no implementation was attempted here. The exact pg_query generated API surface was inspected from the pinned v6.1.0 module with `go doc`; parser behavior for the required SQLite/H2 vectors is still an implementation-time probe requirement. Saturn remains read-only.
