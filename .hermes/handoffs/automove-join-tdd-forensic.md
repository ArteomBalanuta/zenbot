# AutoMove join-policy strict-TDD recovery forensic

## Verdict

`AutoMoveJoinAutomation` is behaviorally green but cannot be accepted as strict-TDD-complete. The original join-policy live transcript has antecedent RED → GREEN records for the happy tracer, host gate, disabled/non-source/nil/blank gates, pre-cancel gate, and kick-action error logging. It does **not** record an antecedent RED for the three current slices below:

| Unsupported slice | Current production branch | Current test written without its own antecedent RED |
|---|---|---|
| Exact-case mismatch | `internal/listener/automove_join.go:43-53`, specifically `trip == user.Trip` at line 44 as the case-sensitive discriminator | `TestAutoMoveJoinRequiresExactCaseTripMatch`, `automove_join_test.go:139-146` |
| Trip query error sends neither | `automove_join.go:39-42`, specifically `if err != nil { return }` at lines 40-41 | `TestAutoMoveJoinTripQueryErrorSendsNeither`, `automove_join_test.go:148-155` |
| Notice failure still kicks | `automove_join.go:45-48`: notice-error handling falls through to `KickNickTo` | `TestAutoMoveJoinNoticeFailureStillKicks`, `automove_join_test.go:168-175` |

The implementation transcript proves this ordering: the three tests were included in later test writes/passing checks, while the only recorded join-policy REDs were the happy tracer (transcript `deleg_a10ff143/task-0.log:43-56`), host (`57-70`), gates (`71-80`), pre-cancel (`81-86`), and kick-error logging (`87-98`). The implementation handoff explicitly labels the three unsupported checks only as later focused passes. Per QA, later green checks cannot become historical test-first proof.

## Retained valid join-policy boundary

The **earliest** independently observed join-policy GREEN is the happy-path tracer after transcript lines 45-56. It introduced the standalone type and proves only public literal notice plus dynamic typed kick for an exact successful trip.

For the smallest usable recovery, retain the latest contiguous evidence-backed join-policy checkpoint, not merely that earliest happy GREEN:

- `TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica` (`automove_join_test.go:70-99`), including QA's test-first public-notice correction: `SendNotice(..., false)` must remain at `automove_join.go:45`.
- Host and input/policy gates (`automove_join_test.go:101-137`), pre-cancel gate (`157-166`), and kick-error logging/no retry (`177-191`).
- The surrounding type, dependency seams, literal notice, `ctx` normalization, input/replica/policy gates, successful query/match path, and kick-error logging needed by those retained tests.
- QA's H2 correction is outside this recovery and must remain byte-for-byte untouched: `internal/repository/h2/user_queries.go:20` is `SELECT trip FROM trips WHERE type = 'USER';`.

This preserves every valid later tracer and limits restoration to the three rejected distinctions. No listener/factory/main wiring is part of this operation.

## Why mismatch and query-error cannot be retained

They must be restored; neither can be claimed through an earlier gate tracer.

1. **Mismatch cannot ride on the successful exact-match tracer.** The happy input has identical strings. Both `trip == user.Trip` and `strings.EqualFold(trip, user.Trip)` pass it, so it does not prove case sensitivity. The mismatch test is the only differentiator and lacked its own recorded RED.
2. **Query error cannot ride on cancellation or a no-action gate.** Those stop before calling `UserTrips`; they prove no query occurs. They do not prove what happens after a repository error. Further, the current test fake returns only `(nil, context.Canceled)`, so an earlier `trips, _ := a.Trips.UserTrips(ctx)` implementation would also send neither and make the test pass immediately. The replacement tracer must return both a matching trip and a non-nil error to distinguish error handling from silently ignoring an error.
3. **Notice failure cannot ride on happy notice/kick.** A nil-error notice does not distinguish falling through after an error from returning early. Its test needs an observed RED against an explicit error-return restoration.

## Smallest compilable targeted restoration

Before adding any new tracer, delete only the three unsupported test functions above. Retain the shared fakes and all proven tests; they are used by retained tests. Then make these three isolated fallback restorations in `OnJoin` while preserving `false` at the public-notice call and preserving the H2 files unchanged:

```go
// 1. Restore pre-case-sensitive behavior for the mismatch RED.
if strings.EqualFold(trip, user.Trip) {

// 2. Restore pre-query-error handling for the query-error RED.
trips, _ := a.Trips.UserTrips(ctx)

// 3. Restore pre-notice-continuation behavior for the notice-failure RED.
if _, err := a.SendNotice(user.Name, autoMoveJoinNotice, false); err != nil {
    log.Printf("automove notice failed for %q: %v", user.Name, err)
    return
}
```

These changes remain compilable with existing imports, retain all non-rejected valid behaviors, preserve the public-notice QA fix, and deliberately make each missing outcome fail once its strengthened test is restored. They are a **manual recovery checkpoint**, not historical evidence: record the restoration and its scoped diff. Do not reset, clean, checkout, stage, or commit.

## Tests to remove and restore

### Remove before the manual restoration

Remove exactly these functions from `internal/listener/automove_join_test.go`:

1. `TestAutoMoveJoinRequiresExactCaseTripMatch` (lines 139-146).
2. `TestAutoMoveJoinTripQueryErrorSendsNeither` (lines 148-155).
3. `TestAutoMoveJoinNoticeFailureStillKicks` (lines 168-175).

Do **not** remove or weaken `TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica`; its `noticeWhisper` assertion is QA's valid RED → GREEN preservation test. Do not remove the retained host/gates, cancellation, or kick-error test.

### Restore replacements one at a time

- **Mismatch replacement:** same external assertion as current test is sufficient, because `strings.EqualFold("Trip", "trip")` now causes a notice and kick. It must be the first test reintroduced after the `EqualFold` fallback is in place.
- **Query-error replacement:** use a fake that returns `[]string{"trip"}, context.Canceled`, not only an error. Assert `UserTrips` was called once and notices/kicks are zero. This fails against `trips, _ := ...` because the matching returned trip would be acted upon.
- **Notice-error replacement:** inject a `SendNotice` error and assert exactly one `KickNickTo`; it fails against the restored error `return`.

## Exact one-at-a-time recovery order

All commands run from `/Users/ab/workspace/go-projects/zenbot`.

1. **Record the current valid baseline before changing source.**
   ```text
   go test ./internal/listener -run '^(TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica|TestAutoMoveJoinDoesNothingForHost|TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser|TestAutoMoveJoinPreCancelledContextSendsNeither|TestAutoMoveJoinLogsActionErrorWithoutRetry)$' -count=1
   git diff --check
   ```
2. **Targeted checkpoint only.** Delete the three named unsupported tests, apply the three fallback restoration deltas above, then run:
   ```text
   gofmt -w internal/listener/automove_join.go internal/listener/automove_join_test.go
   go test ./internal/listener -run '^(TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica|TestAutoMoveJoinDoesNothingForHost|TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser|TestAutoMoveJoinPreCancelledContextSendsNeither|TestAutoMoveJoinLogsActionErrorWithoutRetry)$' -count=1
   git diff --check
   ```
   Record this as manual restoration, not RED → GREEN evidence.
3. **Tracer 1: exact case.** Add only the mismatch test; run and record RED (it should report actions because `EqualFold` matched). Change only `strings.EqualFold` to `==`; rerun the same test to GREEN, then the retained-plus-mismatch listener selection:
   ```text
   go test ./internal/listener -run '^TestAutoMoveJoinRequiresExactCaseTripMatch$' -count=1
   go test ./internal/listener -run '^TestAutoMoveJoinRequiresExactCaseTripMatch$' -count=1
   go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
   ```
4. **Tracer 2: query error.** Add only the strengthened error-with-matching-trip test; run and record RED (it should act because the error is ignored). Change only `trips, _ := ...` to `trips, err := ...; if err != nil { return }`; rerun it to GREEN, then `^TestAutoMoveJoin`.
5. **Tracer 3: notice error continuation.** Add only the notice-error test; run and record RED (it should observe zero kicks because of the temporary return). Remove only that `return`, retaining the error log and `false` public-notice argument; rerun it to GREEN, then `^TestAutoMoveJoin`.
6. **End verification after all three individual GREEN records exist.**
   ```text
   go test -race ./internal/listener -run '^TestAutoMoveJoin' -count=1
   go test ./internal/listener -count=1
   git diff --check
   ```

Do not batch the three tests before any source GREEN. Do not touch `internal/repository/h2/user_queries.go` or its QA literal test during this recovery.

## Non-mutating current baseline

Executed for this forensic report:

```text
go test ./internal/listener -run '^(TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica|TestAutoMoveJoinDoesNothingForHost|TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser|TestAutoMoveJoinPreCancelledContextSendsNeither|TestAutoMoveJoinLogsActionErrorWithoutRetry|TestAutoMoveJoinRequiresExactCaseTripMatch|TestAutoMoveJoinTripQueryErrorSendsNeither|TestAutoMoveJoinNoticeFailureStillKicks)$' -count=1
PASS: ok zenbot/internal/listener 0.462s

git diff --check
PASS: exit 0, no output
```

No source or test file was modified by this audit; only this handoff was created.
