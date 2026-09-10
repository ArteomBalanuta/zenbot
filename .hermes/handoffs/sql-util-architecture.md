# Row #324 — `org.saturn.app.util.SqlUtil` Architecture Specification

## Status

- **Status:** Architecture handoff ready; implementation is **blocked pending exact source verification**.
- **Complexity:** **HIGH**.
- **Scope authorization:** Full row #324 migration is authorized, including the H2/repository contracts and focused persistence tests. No caller migration beyond the affected existing repository paths is authorized.
- **Hard gate:** Any exact SQL constant, identifier, placeholder, or call path not verified directly in `src/main/java/org/saturn/app/util/SqlUtil.java` and its cited callers must remain blocked. Do not invent, normalize, or infer a constant from its category.

## Exact audit/source evidence

The architecture audit identifies:

- **Audit row:** `#324`
- **Audited symbol:** `org.saturn.app.util.SqlUtil`
- **Saturn source:** `src/main/java/org/saturn/app/util/SqlUtil.java`
- **Migration target:** `internal/repository/h2/*.go`
- `SqlUtil` contains **31 public SQL string constants** used by:
  - `AuthorizationServiceImpl`
  - `UserServiceImpl`
  - `MailServiceImpl`
  - `ModServiceImpl`
  - `NoteServiceImpl`
  - `LogRepositoryImpl`
  - `UserJoinedListenerImpl`
- Saturn has no dedicated `SqlUtilTest`.
- Saturn’s H2 schema source is `schema-h2.sql`.
- Existing Zenbot H2/repository anchors are `internal/repository/h2/database.go`, `internal/repository/h2/user_queries.go`, and existing H2 compatibility/persistence tests.
- Existing Zenbot `internal/agent/sql` validation policy is accepted policy, **not** an equivalent `SqlUtil` constant catalog.

The source file and every listed caller are the authority for exact constant names, SQL text, ordering, placeholder count, result shape, and call-path usage. This handoff deliberately does not fabricate source text that was not verified in the supplied evidence.

## 31-constant inventory

The inventory below reserves all 31 audited constants in the required functional groups. Names and SQL are intentionally marked **BLOCKED / SOURCE-VERIFY REQUIRED** until transcribed from the Saturn source. A slot is not evidence that an inferred constant exists.

| Group | Count | Inventory slots |
|---|---:|---|
| `trips` | 4 | `TRIPS_01`–`TRIPS_04` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `trip_names` | 3 | `TRIP_NAMES_01`–`TRIP_NAMES_03` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `names` | 4 | `NAMES_01`–`NAMES_04` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `messages` | 4 | `MESSAGES_01`–`MESSAGES_04` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `mail` | 4 | `MAIL_01`–`MAIL_04` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `commands` | 3 | `COMMANDS_01`–`COMMANDS_03` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `moderation` | 3 | `MODERATION_01`–`MODERATION_03` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `logging` | 2 | `LOGGING_01`–`LOGGING_02` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `lounge` | 2 | `LOUNGE_01`–`LOUNGE_02` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| `role queries` | 2 | `ROLE_01`–`ROLE_02` — **BLOCKED / SOURCE-VERIFY REQUIRED** |
| **Total** | **31** | Every slot requires exact source transcription and caller cross-reference. |

Before implementation, replace each slot with the exact public Java constant identifier, exact SQL literal, exact source location, and verified caller(s). If the source yields a different functional grouping, preserve the source-backed grouping and keep the total at 31; do not force a guessed categorization.

## SQL and H2 compatibility contract

1. Preserve each source SQL statement’s semantics exactly: selected columns, joins, predicates, grouping, ordering, limits, mutation behavior, and statement boundaries.
2. Record and test the exact placeholder contract for every statement: placeholder count, positional order, Go argument type/order, nullable arguments, and expected result columns. Do not convert placeholders by assumption.
3. Preserve semicolons only where the source/caller requires them. Do not append or remove semicolons merely for style.
4. Validate every statement against Zenbot’s actual H2 connection and schema, not only a parser or mocked database.
5. Reconcile every table and column against Saturn `schema-h2.sql` and the existing Zenbot H2 schema/migrations. Any schema mismatch requires an explicit repository-level compatibility decision, not a silent SQL rewrite.
6. Account for H2 behavior versus the Saturn/PostgreSQL wire assumptions: identifier case, quoted identifiers, boolean representation, timestamp types, generated keys, `LIMIT`/ordering behavior, null comparisons, and transaction semantics.
7. Keep dynamic values parameterized. Dynamic SQL is limited to verified, allow-listed structural identifiers where unavoidable; user-controlled values must never be interpolated.
8. Preserve deterministic ordering wherever callers or tests depend on it. If source ordering is absent, do not invent a semantic ordering without a caller-backed requirement; test the observed contract explicitly.
9. Preserve nullability and conversion behavior at the repository boundary. Distinguish SQL `NULL` from zero values and empty strings.

## Zenbot mapping and abstraction boundaries

- Start from `internal/repository/h2/database.go` for connection, transaction, schema setup, and generated-key conventions.
- Extend or reuse methods in `internal/repository/h2/user_queries.go` where the existing method owns the same repository contract. Do not create a second query catalog, parallel database wrapper, or duplicate user-query abstraction.
- Map each verified constant to the existing affected repository method/call path used by the corresponding Zenbot H2 repository implementation. The mapping table must contain: source constant, source caller, target Go method, SQL operation, arguments, result shape, transaction requirement, and test name.
- Add repository methods only when an existing method cannot represent the verified contract without semantic loss. Keep public API changes confined to affected existing repository paths.
- `internal/agent/sql` remains the validation/policy layer. It must not become a replacement `SqlUtil` catalog, and its policy must not be changed for this row.

## Transactions, rollback, and generated keys

- Use the existing Zenbot transaction conventions from `database.go`; do not introduce a competing transaction helper.
- Mutations that are one logical operation must execute atomically. On any statement or scan failure, rollback and return the original error with useful context; never report success after a failed commit.
- Commit only after all required statements and result handling succeed. Verify rollback behavior with an injected/real failure path where the existing test infrastructure permits it.
- For inserts, use the existing generated-key mechanism and verify the key is read from the correct result. Do not assume PostgreSQL `RETURNING` is accepted by H2; choose the established H2-compatible path already used by Zenbot.
- Verify no transaction leaks, rows leaks, or commit/rollback ambiguity on early returns.

## Focused real-H2 test matrix

Tests must use a real H2-backed repository/database setup and the actual compatible schema, not mocks alone.

| Area | Required cases |
|---|---|
| Query contract | Every one of the 31 verified constants executes through its mapped existing repository path with exact argument order and expected result shape. |
| Reads | Empty result, one row, multiple rows, deterministic ordering, and `NULL` values for every nullable column. |
| Mutations | Insert/update/delete success, affected-row count, repeat/idempotency behavior where caller requires it, and no partial write on failure. |
| Transactions | Commit success; rollback after statement failure; rollback after scan/generated-key failure; no leaked transaction/rows. |
| Generated keys | Insert returns the H2-generated key through the repository contract and subsequent read finds the row. |
| Schema compatibility | Fresh schema initialization from the Zenbot H2 path; quoted/unquoted identifiers and all referenced tables/columns. |
| SQL edge cases | Semicolon handling, placeholder count/order, timestamps, booleans, null predicates, and H2 syntax for each statement. |
| Security boundary | Values containing quotes, semicolons, wildcard characters, and SQL-like text remain parameters and cannot alter query structure. |
| Caller coverage | Authorization, user, mail, moderation, note, logging, and joined-listener paths represented by the verified source callers; unsupported paths remain uncalled. |

## Risks and mitigations

- **PostgreSQL wire/H2 syntax:** SQL accepted by one dialect may fail or change semantics in H2. Execute every statement on real H2 and document dialect-specific adaptations.
- **Quoted identifiers:** Case-sensitive quoted names can diverge from unquoted H2 names. Match the schema and preserve required quoting exactly.
- **Semicolons:** Driver behavior differs for trailing or embedded semicolons. Test the exact statement text and remove only when proven unnecessary and semantics-preserving.
- **Nulls:** `NULL` is not equal to an empty or zero value. Use correct predicates and nullable Go scans.
- **Ordering:** Unordered SQL results are unstable. Preserve source ordering; test all caller-visible ordering requirements.
- **Timestamps:** Time zones, precision, and driver conversion can differ. Verify round trips and comparisons using the repository’s established time types.
- **SQL injection boundaries:** Parameterize all data values; allow-list any structural choice. No string concatenation from caller/user input.
- **Shared dirty files:** Preserve unrelated dirty/untracked files and accepted slices. Inspect and edit only task-owned files; do not reset, format, or “clean up” unrelated changes.

## File map and task-owned files

### Source/reference files (read-only for this task)

- Saturn: `src/main/java/org/saturn/app/util/SqlUtil.java`
- Saturn callers: `AuthorizationServiceImpl`, `UserServiceImpl`, `MailServiceImpl`, `ModServiceImpl`, `NoteServiceImpl`, `LogRepositoryImpl`, `UserJoinedListenerImpl`
- Saturn schema: `schema-h2.sql`
- Zenbot policy/reference: `internal/agent/sql`
- Zenbot H2 foundation: `internal/repository/h2/database.go`
- Zenbot existing query anchor: `internal/repository/h2/user_queries.go`
- Existing H2 compatibility/persistence tests

### Task-owned implementation files

- `internal/repository/h2/database.go` — only if an existing transaction/schema/generated-key contract must be extended.
- `internal/repository/h2/user_queries.go` — reuse first; add only verified missing methods.
- Additional files under `internal/repository/h2/` — only for directly affected existing repository paths, with no duplicate abstraction.
- Focused H2 persistence/compatibility test files adjacent to the affected repository code.
- This handoff: `.hermes/handoffs/sql-util-architecture.md`.

No production registration, broad `Util` row #325 work, or agent/router/provider/listener/command/transport/remote-room/Whiskey expansion is task-owned. Do not modify Saturn. Do not modify `internal/agent/sql` policy.

## Explicitly unsupported or unselected scope

- Any constant not verified in the Saturn source remains **unsupported and blocked**.
- No inferred aliases, renamed constants, convenience queries, or “helpful” SQL normalization.
- No migration of callers outside the affected existing repository paths.
- No dedicated broad utility layer for row #325.
- No unrelated production registration.
- No expansion into agent, router, provider, listener, command, transport, remote-room, or Whiskey paths.
- No replacement of `internal/agent/sql` policy with a constant catalog.
- No new call path may be enabled solely because a query can be made to compile.

## Implementation and QA gates

1. **TDD RED:** After exact source verification, add focused real-H2 tests for each mapped contract; confirm they fail for the missing implementation or mismatch.
2. **GREEN:** Implement the smallest change in the existing repository abstractions, preserving exact placeholder/result/transaction contracts.
3. **Refactor:** Remove only duplication introduced by the implementation; do not broaden scope or alter accepted policy.
4. **Verification:** Run the focused real-H2 matrix, then the relevant existing repository/H2 tests. Record actual results; do not claim unrun tests.
5. **Independent QA gate:** A separate reviewer/agent must compare every implemented constant and caller mapping against `SqlUtil.java`, check the 31-count inventory, inspect SQL/schema compatibility, and verify that unsupported paths stayed untouched.
6. **Dirty-tree gate:** Confirm unrelated dirty/untracked files were preserved and the diff contains only task-owned changes.
7. **Final unblock condition:** Implementation may proceed only when all 31 exact constants, their callers, SQL text, placeholders, schema references, and supported call paths are source-verified. Until then, this row remains blocked.

**Handoff conclusion:** This is an implementation-ready architecture boundary, but exact constant implementation is intentionally blocked until direct source verification. No caller migration beyond affected existing repository paths is authorized.
