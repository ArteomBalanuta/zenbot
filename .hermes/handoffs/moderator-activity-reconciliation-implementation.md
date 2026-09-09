# Moderator activity S5 reconciliation implementation

## Scope and result

[IMPLEMENTED] The selected Saturn moderator `active` / `activity` database-error result contract is reconciled in the existing activity vertical only.

`internal/service/activity.go:ActivityService.Stats` now converts an `ActivityRepository.ActivityStats` error into its literal `err.Error()` result and returns a nil Go error. Therefore the existing valid-invocation branch in `internal/command/activity.go:activityCommand.Execute` sends exactly `Stats: \\n` followed by that text, retaining the inbound whisper flag, and returns `model.SUCCESSFUL, nil`.

No interface, repository, query, schema, factory, catalog/registration, listener, agent/config, Saturn source, prefix, lastonline, S2/S3/S4, or other command path was edited.

## Source trace

Read-only Saturn authority was re-read before edits:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ActivityCommandImpl.java:ActivityCommandImpl.execute`
  - exact aliases are `active`, `activity`; role is `MODERATOR`.
  - `requiredArgument(0, "active 8Wotmg")` supplies the first-argument-only trim/usage behavior.
  - a valid target calls `engine.sqlService.executeActivityStats(target)`, replies `"Stats: \\n%s"`, and returns successful.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/SQLServiceImpl.java:executeActivityStats`
  - binds `trip.trim()`; returns `No activity found.` for no rows and the escaped `Text` table payload for rows.
  - catches `SQLException` and returns `e.getMessage()` as a normal string result.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/util/TableGenerator.java:generateTable`
  - confirms Java `String.length()` / UTF-16 width semantics, two-space padding, even widths, borders, and row shape.

Target seams re-read before modification:

- `internal/command/activity.go:activityCommand.Execute`
- `internal/command/activity_test.go`
- `internal/service/activity.go:ActivityService.Stats`
- `internal/service/activity_test.go`
- `internal/repository/activity.go:ActivityRepository`
- `internal/repository/h2/activity.go:Database.ActivityStats`
- `internal/repository/h2/activity_test.go`

## TDD evidence

### RED

Tests were changed before production code and each was run independently against the prior propagation behavior.

```text
go test ./internal/service -run '^TestActivityServiceReturnsRepositoryErrorTextAsSourceResult$' -count=1 -v
=== RUN   TestActivityServiceReturnsRepositoryErrorTextAsSourceResult
    activity_test.go:53: err=sentinel activity database failure
--- FAIL: TestActivityServiceReturnsRepositoryErrorTextAsSourceResult (0.00s)
FAIL    zenbot/internal/service
```

```text
go test ./internal/command -run '^TestActivityCommandRepliesWithRepositoryErrorTextAsSourceResult$' -count=1 -v
=== RUN   TestActivityCommandRepliesWithRepositoryErrorTextAsSourceResult
    activity_test.go:65: status=FAILED err=sentinel activity database failure chats=[]
--- FAIL: TestActivityCommandRepliesWithRepositoryErrorTextAsSourceResult (0.01s)
FAIL    zenbot/internal/command
```

### GREEN

Minimal production change:

```go
if err != nil {
    return err.Error(), nil
}
```

Focused GREEN:

```text
go test ./internal/service -run 'TestActivityService' -count=1 -v
PASS    zenbot/internal/service

go test ./internal/command -run 'TestActivity(Command|.*Activity|RegisterUserUtilitiesAddsActivity)' -count=1 -v
PASS    zenbot/internal/command
```

Regression tests added/replaced:

- `TestActivityServiceReturnsRepositoryErrorTextAsSourceResult` asserts sentinel error text and nil error.
- `TestActivityCommandRepliesWithRepositoryErrorTextAsSourceResult` asserts one exact `alice|Stats: \\n<sentinel>|true` reply, successful/nil result, one repository call, trimmed first target `trip`, ignored later argument, and inbound whisper delivery.

Existing tests remain the proof for aliases/role, normal renderer, no rows, missing target usage/no query, and cancellation before/during query. The command still checks `ctx.Err()` before service work and again before replying; cancellation remains `FAILED` with no reply, intentionally retained Go safety rather than represented as Saturn parity.

## Verification results

All required activity gates passed in `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./internal/repository/h2 -run '^TestActivityStats' -count=1 -v
PASS    zenbot/internal/repository/h2

go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory \
  -run 'Test(Activity|RegisterUserUtilitiesAddsActivity|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' \
  -count=1 -race
PASS    all four packages

go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory -count=1
PASS    all four packages

go test ./... -count=1
PASS    all packages

go vet ./internal/command
PASS

gofmt -d internal/service/activity.go internal/service/activity_test.go internal/command/activity_test.go
PASS (no output)

git diff --check
PASS (exit 0)
```

Known out-of-scope diagnostic was observed and intentionally not changed:

```text
go vet ./internal/core
internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files changed by this reconciliation

- `internal/service/activity.go`
- `internal/service/activity_test.go`
- `internal/command/activity_test.go`
- `.hermes/handoffs/moderator-activity-reconciliation-implementation.md`

The worktree was already dirty before this task, including the activity files as untracked pre-existing work. No reset, clean, checkout, staging, commit, or push was performed. Final `git status --short --branch` retained all pre-existing dirty paths and adds only this handoff to the untracked handoff collection.

## Exclusions and limitations

- The H2 repository remains the persistence boundary and still returns query/database errors unchanged; no repository query/schema modification was made.
- No interface expansion occurred; `Stats(context.Context, string) (string, error)` is unchanged.
- Error text disclosure is preserved solely for strict Saturn parity. It is not proposed as a general error-handling policy.
- Saturn has no focused activity command/service test located in its test tree; the error contract is source-derived from the exact command/service/table symbols above and locked here by deterministic Go service/command tests plus real-H2 query tests.
