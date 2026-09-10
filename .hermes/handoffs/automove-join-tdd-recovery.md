# AutoMove join-policy strict-TDD recovery

## Scope and outcome

Recovered only the three branches rejected by `automove-join-tdd-forensic.md`, one tracer at a time. QA parity remains intact:

- Public notice remains `SendNotice(user.Name, autoMoveJoinNotice, false)`.
- H2 files were not touched; `internal/repository/h2/user_queries.go` remains outside this recovery.
- No reset, clean, checkout, staging, or commit was performed.
- Only `internal/listener/automove_join.go`, `internal/listener/automove_join_test.go`, and this handoff were modified.

Final behavior:

1. Trip matching is exact/case-sensitive (`trip == user.Trip`).
2. A `UserTrips` error returns without notice or kick, even if the repository also supplied a matching trip.
3. A notice error is logged and the dynamic typed kick still happens once.

## Retained baseline

Before source/test recovery work:

```text
go test ./internal/listener -run '^(TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica|TestAutoMoveJoinDoesNothingForHost|TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser|TestAutoMoveJoinPreCancelledContextSendsNeither|TestAutoMoveJoinLogsActionErrorWithoutRetry)$' -count=1
ok  	zenbot/internal/listener	0.342s

git diff --check
PASS: exit 0, no output
```

## Manual recovery checkpoint (not TDD proof)

Removed exactly the three unsupported tests:

- `TestAutoMoveJoinRequiresExactCaseTripMatch`
- `TestAutoMoveJoinTripQueryErrorSendsNeither`
- `TestAutoMoveJoinNoticeFailureStillKicks`

Then made exactly the documented, compilable fallback restorations:

```go
trips, _ := a.Trips.UserTrips(ctx)
if strings.EqualFold(trip, user.Trip) {
if _, err := a.SendNotice(user.Name, autoMoveJoinNotice, false); err != nil {
    log.Printf("automove notice failed for %q: %v", user.Name, err)
    return
}
```

Checkpoint verification:

```text
gofmt -w internal/listener/automove_join.go internal/listener/automove_join_test.go
go test ./internal/listener -run '^(TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica|TestAutoMoveJoinDoesNothingForHost|TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser|TestAutoMoveJoinPreCancelledContextSendsNeither|TestAutoMoveJoinLogsActionErrorWithoutRetry)$' -count=1
ok  	zenbot/internal/listener	0.361s

git diff --check
PASS: exit 0, no output
```

This checkpoint was deliberately manual restoration only; it is not represented as antecedent RED→GREEN evidence.

## One-tracer-at-a-time RED→GREEN transcript

### 1. Exact-case mismatch

Added only `TestAutoMoveJoinRequiresExactCaseTripMatch`.

RED against the `strings.EqualFold` fallback:

```text
go test ./internal/listener -run '^TestAutoMoveJoinRequiresExactCaseTripMatch$' -count=1
--- FAIL: TestAutoMoveJoinRequiresExactCaseTripMatch (0.00s)
    automove_join_test.go:144: case mismatch actions: notices=1 kicks=1
FAIL
FAIL	zenbot/internal/listener	0.370s
FAIL
```

Changed only `strings.EqualFold(trip, user.Trip)` to `trip == user.Trip`.

GREEN:

```text
go test ./internal/listener -run '^TestAutoMoveJoinRequiresExactCaseTripMatch$' -count=1
ok  	zenbot/internal/listener	0.360s

go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  	zenbot/internal/listener	0.258s
```

### 2. Query error with matching returned trip

Added only `TestAutoMoveJoinTripQueryErrorSendsNeither`, with the fake returning `[]string{"trip"}, context.Canceled` and asserting one repository call plus zero notices/kicks.

RED against the ignored-error fallback:

```text
go test ./internal/listener -run '^TestAutoMoveJoinTripQueryErrorSendsNeither$' -count=1
--- FAIL: TestAutoMoveJoinTripQueryErrorSendsNeither (0.00s)
    automove_join_test.go:153: query error actions: queries=1 notices=1 kicks=1
FAIL
FAIL	zenbot/internal/listener	0.365s
FAIL
```

Changed only the query handling to:

```go
trips, err := a.Trips.UserTrips(ctx)
if err != nil {
    return
}
```

GREEN:

```text
go test ./internal/listener -run '^TestAutoMoveJoinTripQueryErrorSendsNeither$' -count=1
ok  	zenbot/internal/listener	0.363s

go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  	zenbot/internal/listener	0.261s
```

### 3. Notice error continues to kick

Added only `TestAutoMoveJoinNoticeFailureStillKicks` with an injected `SendNotice` error and an assertion of exactly one kick.

RED against the temporary notice-error `return`:

```text
go test ./internal/listener -run '^TestAutoMoveJoinNoticeFailureStillKicks$' -count=1
2026/09/05 15:42:01 automove notice failed for "joined": notice failed
--- FAIL: TestAutoMoveJoinNoticeFailureStillKicks (0.00s)
    automove_join_test.go:173: kicks = 0, want 1
FAIL
FAIL	zenbot/internal/listener	0.411s
FAIL
```

Removed only that temporary `return`, retaining the error log and public `false` argument.

GREEN:

```text
go test ./internal/listener -run '^TestAutoMoveJoinNoticeFailureStillKicks$' -count=1
ok  	zenbot/internal/listener	0.363s

go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  	zenbot/internal/listener	0.258s
```

## Final verification

```text
gofmt -w internal/listener/automove_join.go internal/listener/automove_join_test.go
go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  	zenbot/internal/listener	0.255s

go test -race ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  	zenbot/internal/listener	1.409s

go test ./internal/listener -count=1
ok  	zenbot/internal/listener	0.259s

git diff --check
PASS: exit 0, no output
```
