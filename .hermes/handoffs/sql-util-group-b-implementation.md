# Row #324 Group B — Implementation Handoff

## Scope

Implemented only the authorized Group B compatibility slice from `sql-util-group-b-authorized-architecture.md`. This is not full row #324 acceptance and does not claim migration completion. No Saturn source, command/listener/provider/router/transport/remote-room/Whiskey, agent/sql policy, schema, or existing `RegisteredUsers`/`LastMessages` caller was changed.

## Exact semantics implemented

- Added the five source-transcribed Saturn SQL constants in `internal/repository/h2/sql_util_group_b.go`:
  `DELETE_TRIP_NAMES`, `DELETE_TRIP`, `DELETE_NAME`, `SELECT_NAME_TRIP_REGISTERED`, and `SELECT_LAST_N_MESSAGES` (package-bounded names).
- Added `repository.DeleteResult` with separate affected-row fields: `TripNamesRows`, `TripRows`, and `NameRows`.
- Added `repository.SaturnRegisteredUser{Name, Trip}` and `repository.SaturnLastMessage{Name, Message, CreatedOn}` plus an intentionally unwired `SqlUtilGroupBRepository` contract.
- `DeleteIdentity` is fail-closed unless the context carries the package-private authorization capability. It executes the three deletes atomically in Saturn order: links first, trip parent second, name parent third. Any error returns a zero result and rolls back all prior statements.
- The link delete preserves Saturn OR scope: all links for the supplied trip OR all links for the supplied name. Parent deletes remain exact single-column equality deletes. Absent, blank, and injection-like values are parameter data and are no-op results.
- `SaturnRegisteredUsers` returns the Saturn `(Name,Trip)` projection with `DISTINCT` and `Trip DESC`; existing `RegisteredUsers` remains untouched.
- `SaturnLastMessages` is separately named and returns only `(Name,Message,CreatedOn)`, accepts nullable name, defaults non-positive count to 5, excludes only `LEFT`/`JOINED`, orders `created_on DESC`, and does not enforce `PUBLIC` or add an id tie-break. Existing public-only rich `LastMessages` remains untouched.
- H2 PostgreSQL-wire execution uses `$n` placeholders where H2 requires them; identity values remain parameters. The bounded validated integer limit is rendered as an integer expression because this H2 wire path cannot encode an untyped parameter in `LIMIT` (the exact Saturn constant remains preserved and is contract-tested).

## TDD evidence

### RED before H2 implementation

Command:

```text
go test ./internal/repository/h2 -run 'TestGroupB' -count=1
```

Result: failed at compile time with the expected missing Group B symbols, including `deleteTripNames`, `deleteTrip`, `deleteName`, `selectNameTripRegistered`, `selectLastNMessages`, `DeleteIdentity`, `SaturnRegisteredUsers`, `SaturnLastMessages`, and the authorization seam. The only pre-implementation production addition was the narrow repository contract/types file.

### GREEN focused result

Command:

```text
go test ./internal/repository/h2 -run 'TestGroupB' -count=1
```

Result:

```text
ok  zenbot/internal/repository/h2  4.309s
```

Real-H2 tests cover exact constants, authorization denial, typed affected-row results, OR scope, unrelated-row preservation, absent/blank/injection-like inputs, injected mid-transaction rollback, Saturn projection/order, nullable name/default count, Saturn event filtering, and preservation of existing public-only rich history behavior.

## Files created

- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `.hermes/handoffs/sql-util-group-b-implementation.md`

Existing APIs/callers and existing `RegisteredUsers`/`LastMessages` implementations were not modified.

## Verification results

- `gofmt -w ...`: passed.
- `git diff --check`: passed.
- `go test ./... -count=1`: passed; all packages green.
- `go test -race ./... -count=1`: passed; all packages green. macOS emitted an existing linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exit status was 0.
- `go vet ./...`: passed.
- `go build ./...`: passed.
- Saturn preservation check: `git -C /Users/ab/workspace/projects/saturn status --short -- src/main/java/org/saturn/app/util/SqlUtil.java` returned no output.
- Target preservation check: the three implementation/test files were the only target paths reported as new by the scoped status check; protected `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` remained pre-existing staged additions and were not modified by this slice.

## Limitations and exclusions

- The compatibility interface and methods are deliberately unwired; no production caller can mint the package-private authorization context. No authorization policy was invented.
- No separate public primitive methods for the three deletes were exposed.
- Saturn's equal-timestamp last-message ordering remains unspecified, as required for exact parity.
- The H2 driver limitation requires a bounded integer limit expression at execution time; identity/name/trip values are still parameterized.
- This handoff does not cover Group A beyond preserving it, Group C, row #325, or full row #324.
