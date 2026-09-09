# AutoMove H2 USER-trip repository implementation

## Scope

Recovered only AutoMove step 5 in the already-dirty worktree. No reset, clean, checkout, stage, or commit was used. No command, core, listener, factory, main, schema, migration, or configuration code was changed.

## Production boundary

- Added `repository.AutoMoveTripRepository` in `internal/repository/automove.go`:
  ```go
  UserTrips(context.Context) ([]string, error)
  ```
- Added `(*h2.Database).UserTrips(ctx)` in `internal/repository/h2/user_queries.go`.
- The source SQL is exactly:
  ```sql
  SELECT trip FROM trips WHERE type='USER'
  ```
- The method uses `QueryContext`, appends scanned trip values without normalization or sorting, and returns query, scan, and final `rows.Err()` failures.

## Strict TDD evidence

### First tracer: USER-only lookup and original case

Added `TestUserTripsReturnsOnlyUserTripsWithOriginalCase` using the existing real H2 `openTestDB` fixture. It seeds `USER` rows `Trip-Alpha` and `trip-beta`, plus `MODERATOR` and `PEST` rows, and asserts exactly the two USER rows in fixture query order with original capitalization.

RED before interface/method implementation:

```text
$ go test ./internal/repository/h2 -run '^TestUserTripsReturnsOnlyUserTripsWithOriginalCase$' -count=1
# zenbot/internal/repository/h2 [zenbot/internal/repository/h2.test]
internal/repository/h2/user_queries_test.go:64:18: d.UserTrips undefined (type *Database has no field or method UserTrips)
FAIL    zenbot/internal/repository/h2 [build failed]
FAIL
```

GREEN after the minimal interface, SQL constant, query/scan loop:

```text
$ go test ./internal/repository/h2 -run '^TestUserTripsReturnsOnlyUserTripsWithOriginalCase$' -count=1
ok      zenbot/internal/repository/h2  1.122s
```

### Second tracer: context propagation

The existing real H2 fixture makes a pre-cancelled context deterministic; it has no conventional query/scan/rows fault-injection seam, so no synthetic driver/mock was added.

RED with the initial non-context `DB.Query` implementation:

```text
$ go test ./internal/repository/h2 -run '^TestUserTrips' -count=1
--- FAIL: TestUserTripsPropagatesCanceledContext (0.65s)
    user_queries_test.go:87: UserTrips error = <nil>, want context.Canceled
FAIL
FAIL    zenbot/internal/repository/h2  1.696s
FAIL
```

GREEN after replacing that call with `DB.QueryContext(ctx, selectUserTrips)`:

```text
$ go test ./internal/repository/h2 -run '^TestUserTrips' -count=1
ok      zenbot/internal/repository/h2  1.667s
```

## Final verification

```text
$ go test ./internal/repository/h2 -run '^TestUserTrips' -count=1
ok      zenbot/internal/repository/h2  1.667s

$ go test ./internal/repository/h2 -count=1
ok      zenbot/internal/repository/h2  37.303s

$ go test ./... -count=1
PASS: all packages; internal/repository/h2 38.139s

$ git diff --check
exit 0; no output
```

## Files changed

- New: `internal/repository/automove.go`
- Modified: `internal/repository/h2/user_queries.go`
- Modified: `internal/repository/h2/user_queries_test.go`
- New handoff: `.hermes/handoffs/automove-h2-implementation.md`

## Explicit exclusions

No AutoMove command/core/listener/factory/main composition, schema/migration/configuration work, persistence beyond this read boundary, or unrelated dirty hunk was touched. Query ordering was not imposed: the SQL remains source-exact and the test checks the real H2 fixture's deterministic returned sequence.
