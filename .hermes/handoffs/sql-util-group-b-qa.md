# Row #324 Group B — Independent QA Handoff

## Verdict: PASS (bounded Group B only)

Independent QA inspected the actual untracked Group B implementation and test files, compared the source and caller semantics against the read-only Saturn checkout, and ran the requested focused and repository-wide gates. No genuine task-owned defect was found; no production or test fix was made.

This PASS is limited to the five authorized Group B contracts. **Full row #324 remains incomplete, and the overall Saturn-to-Zenbot migration remains incomplete.**

## Changed-file attribution

The Group B implementation owns only these new files:

- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `.hermes/handoffs/sql-util-group-b-implementation.md` (pre-existing implementation handoff, inspected)
- This QA handoff: `.hermes/handoffs/sql-util-group-b-qa.md`

The working tree also contains extensive pre-existing staged, modified, and untracked migration work. Group B did not edit existing `RegisteredUsers`, `LastMessages`, callers, schema, agent/sql policy, commands, listeners, providers, routers, transport, remote-room, or Whiskey paths. The pre-existing `AM internal/repository/h2/identity.go` status was preserved and was not attributed to Group B. No protected document was edited.

## Source and caller comparison

Read directly from Saturn:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SqlUtil.java`
- `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`
- `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java`

The five SQL definitions match the Saturn predicates/projections after insignificant text-block whitespace trimming:

- `DELETE_TRIP_NAMES`: link rows where trip matches **OR** name matches.
- `DELETE_TRIP`: exact `trip = ?` parent delete.
- `DELETE_NAME`: exact `name = ?` parent delete.
- `SELECT_NAME_TRIP_REGISTERED`: `DISTINCT`, `(name,trip)` projection, `trip DESC`.
- `SELECT_LAST_N_MESSAGES`: `(name = ? OR trip = ?)`, excludes only `LEFT`/`JOINED`, `created_on DESC`, `LIMIT ?`.

The H2 implementation preserves the exact source constants for contract checking. Its PostgreSQL-wire execution uses `$1/$2` placeholders and a validated integer limit expression because this H2 wire path does not accept the untyped parameter in `LIMIT`; identity values remain bound parameters. No identity/name/trip value is interpolated.

Caller semantics checked against Saturn:

- Delete ordering is links, trip parent, name parent, in one transaction.
- Affected rows are reported separately as `TripNamesRows`, `TripRows`, and `NameRows`.
- Authorization is fail-closed: `DeleteIdentity` rejects an ordinary context and only accepts the package-private capability used by same-package tests; no production caller can mint it.
- Any error returns a zero result and `WithTx` rolls back prior statements.
- Blank, absent, and injection-like values remain parameter data and are no-op deletes.
- `SaturnRegisteredUsers` returns a distinct `Name,Trip` result shape and `Trip DESC`; existing `RegisteredUsers` remains separate with its existing `Trip,Name` shape/order.
- `SaturnLastMessages` has a separate three-field result shape, accepts a nullable name argument, defaults `count <= 0` to 5, excludes only the two presence messages, and does not add the existing public visibility filter or ID tie-break.
- Existing public-only rich `LastMessages` remains untouched and is tested separately.

## Focused tests

Commands and actual results:

```text
go test ./internal/repository/h2 -run 'TestGroupB' -count=1
ok  zenbot/internal/repository/h2  4.275s

go test -race ./internal/repository/h2 -run 'TestGroupB' -count=1
ok  zenbot/internal/repository/h2  5.282s
```

`sql_util_row324_group_b_test.go` covers exact constants, unauthorized no-mutation behavior, typed per-statement affected rows, OR link scope, unrelated-row preservation, exact parent scope, absent/blank/injection-like input, injected second-statement rollback, Saturn registered-user projection/order, separation from existing `RegisteredUsers`, nullable-name input/default count, event exclusions/order, and separation from existing public-only rich `LastMessages`.

The H2 identity/schema boundary was inspected and exercised through the real H2 PostgreSQL-wire fixture. `Open` proves `SELECT H2VERSION()`, and schema metadata confirms identity parent IDs, unique `trips.trip`/`names.name`, foreign keys without cascade, and the required links-first ordering.

## Full verification gates

All requested gates passed in one clean command sequence:

```text
go test ./... -count=1
PASS — all packages green; h2 package ok 18.708s

go test -race ./... -count=1
PASS — all packages green; h2 package ok 18.914s
NOTE — macOS emitted the existing linker warning for internal/agent/sql.test
("malformed LC_DYSYMTAB"); command exit status was 0.

go vet ./...
PASS — no output, exit status 0

go build ./...
PASS — exit status 0

gofmt -l .
PASS — no output

git diff --check
PASS — no output

Untracked Group B whitespace check via git diff --no-index --check
PASS — no whitespace errors
```

Existing focused tests were included in the full suite and remained green, including H2 identity registration/history, user-query contracts, and the prior bounded Group A tests.

## Preservation checks

- Saturn `SqlUtil.java`, `UserServiceImpl.java`, and `MailServiceImpl.java` had no status entries. Saturn source/parser paths were unchanged; the Saturn checkout's unrelated pre-existing weather edits remained untouched.
- `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` hashes were observed and unchanged during QA (`a4a2bfadba585ea0fd7e67208f8f320da76a6c72` and `4970d4f31f52b2d84d5928a348f04b7cea1e86e3`).
- Target schema files were not changed by Group B.
- No Group B references were found in existing callers; the compatibility seam is intentionally unwired.
- Pre-existing dirty/staged/untracked target work was preserved; only the bounded new Group B files and handoff are attributed to this slice.

## Residual limitations

- The compatibility seam is intentionally unwired. No authorized service/command path was invented, and the package-private authorization capability has no production minting path.
- The H2 execution path necessarily rewrites placeholders for PostgreSQL-wire compatibility and renders only the validated integer limit; the source-transcribed constants remain preserved and contract-tested.
- Saturn's equal-timestamp ordering remains unspecified because Saturn orders only by `created_on DESC`; Group B does not add an ID tie-break.
- The typed `SaturnLastMessage.Name` is a Go `string`; the tested nullable behavior is the Saturn caller's nullable **name input** binding, as authorized by the Group B architecture. Existing public history behavior remains unchanged.
- Group A, Group C, row #325, and the remainder of row #324 were not accepted by this QA.

## Final statement

**PASS for the authorized Group B implementation. Full row #324 and the overall migration remain incomplete.**
