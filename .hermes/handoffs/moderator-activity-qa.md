# Moderator activity S5 QA

## Acceptance verdict

**ACCEPTED.** The completed `active` / `activity` reconciliation matches the bounded Saturn error-result contract and retains the target's cancellation boundary. QA added one missing pre-cancel regression test; no production code, repository/interface/query, catalog/registration/factory, or excluded path was changed.

## Source trace

Read-only Saturn authority:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ActivityCommandImpl.java:16-45`
  - aliases `active`, `activity`; `MODERATOR`; `requiredArgument(0, "active 8Wotmg")`; valid target calls `executeActivityStats`, replies `Stats: \\n` + result, and returns successful.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/SQLServiceImpl.java:98-120`
  - binds `trip.trim()`, returns `No activity found.` for no rows and the escaped `Text` table payload for rows; catches `SQLException` and returns `e.getMessage()` as the normal result.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/util/TableGenerator.java:15-150`
  - Java UTF-16 `String.length()` width, two-space padding, even widths, and exact border/row layout.

Zenbot trace audited:

`activityCommand.Execute` -> `ActivityService.Stats` -> `ActivityRepository.ActivityStats` -> H2 `Database.ActivityStats` -> rendered/source-shaped result -> author reply with inbound whisper flag.

`internal/service/activity.go:17-30` maps a repository error to `err.Error(), nil`; `internal/command/activity.go:26-34` then sends exactly `Stats: \\n` plus that result and returns `SUCCESSFUL, nil` unless the Go context is canceled.

## Tests and results

All commands were run in `/Users/ab/workspace/go-projects/zenbot`.

- `go test ./internal/service -run 'TestActivityService' -count=1 -v` — PASS: table payload, no-row text, sentinel repository error text/nil error.
- `go test ./internal/command -run 'TestActivity(Command|.*Activity|RegisterUserUtilitiesAddsActivity)' -count=1 -v` — PASS: aliases/role, trimmed first argument only, source usage/no query, exact sentinel-error reply/whisper/success/nil, cancellation before and during query, capability registration.
- `go test ./internal/repository/h2 -run '^TestActivityStats' -count=1 -v` — PASS: real-H2 case-insensitive final filtering, exact-case grouping, day/hour ordering, trimmed bound input, and no rows.
- `go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory -run 'Test(Activity|RegisterUserUtilitiesAddsActivity|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' -count=1 -race` — PASS: h2, service, command, factory (factory had no matching tests).
- `go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory -count=1` — PASS.
- `go test ./... -count=1` — PASS all packages.
- `go vet ./internal/command` — PASS.
- `gofmt -d internal/service/activity.go internal/service/activity_test.go internal/command/activity.go internal/command/activity_test.go internal/repository/activity.go internal/repository/h2/activity.go` — zero output.
- `git diff --check` — PASS (exit 0).

### QA hardening

Added `TestActivityCommandDoesNotReplyWhenContextCancelsBeforeQuery` in `internal/command/activity_test.go`. It verifies a pre-canceled context returns `FAILED` with `context.Canceled`, invokes no repository call, and sends no reply. It passed independently:

```text
go test ./internal/command -run '^TestActivityCommandDoesNotReplyWhenContextCancelsBeforeQuery$' -count=1 -v
PASS  zenbot/internal/command
```

The pre-cancel guard was already present in production; this is regression coverage, not a production defect/fix. The existing during-query test remains green. This Go cancellation behavior is intentionally a target safety boundary, not a claim of Saturn source parity.

### Coverage note

`go test ./internal/service ./internal/command -run 'TestActivity' -cover` passed and reported package-level coverage of `12.7%` for `internal/service` and `3.0%` for `internal/command`. These are whole-package denominators, not activity-only coverage.

## Static review

- `internal/command/registry.go:133` retains exactly canonical `active`, aliases `active`/`activity`, and `MODERATOR`; `internal/command/handlers.go:342-343` resolves it to concrete `*activityCommand`.
- Catalog guard is included in the race group and remains green; `active` is not an allowed generic fallback.
- `internal/factory/engine_factory.go:77-79` retains typed conditional composition only when `repository.ActivityRepository` is available. Command capability registration remained green.
- `internal/repository/activity.go:16-18` remains the single narrow method: `ActivityStats(context.Context, string) ([]ActivityStat, error)`.
- `internal/repository/h2/activity.go:52-55` remains the existing bound query (`LOWER(trip) = LOWER($1)`, `QueryContext(..., strings.TrimSpace(trip))`) with no widening or rewrite. No repository/interface/query file was modified by QA.
- Sentinel error text is covered solely as Saturn source parity; it is not generalized as an error-handling policy.

## Known diagnostic

`go vet ./internal/core` still reports the known excluded copylock warning and was not changed:

```text
internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files and exclusions

QA modified only:

- `internal/command/activity_test.go` — added pre-canceled-context regression coverage.
- `.hermes/handoffs/moderator-activity-qa.md` — this record.

The worktree was pre-existing dirty, including the untracked activity implementation/test vertical. No reset, clean, checkout, staging, commit, or push occurred. No changes were made to lastonline, prefix, S2/S3/S4, catalog/registration/factory/repository/schema/query, agent/config/listener/Saturn/plans, or unrelated dirty work.
