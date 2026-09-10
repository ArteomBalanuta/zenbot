# Next bounded parity vertical: S5 moderator `active` / `activity` reconciliation

## Decision

**Select exactly one vertical: Saturn moderator activity statistics (`active`, `activity`).**

This is the smallest independent post-prefix parity closure with usable target seams already present: a concrete command, a narrow typed repository interface, real-H2 query coverage, renderer coverage, factory composition, conditional registration, and a source-shaped catalog row. It touches no accepted S2/S3/S4/prefix implementation contract and no listener, raw moderation, replica, agent, schema, or configuration path.

**Implementation role:** `@developer` with a targeted QA/reconciliation pass. **Complexity:** small (two production behavior edits at most, plus focused test replacement/addition). **Risk:** medium only because a strict Saturn error-path port deliberately changes the target's current failure/no-reply behavior to a source-shaped successful `Stats:` reply containing the database error text.

## Evidence boundary

- **[OBSERVED] Saturn authority:** `src/main/java/org/saturn/app/command/impl/moderator/ActivityCommandImpl.java:ActivityCommandImpl.execute`, `SQL_STATS_PER_HOUR_OF_WEEK`; `src/main/java/org/saturn/app/service/impl/SQLServiceImpl.java:executeActivityStats`; `src/main/java/org/saturn/app/service/impl/util/TableGenerator.java:generateTable`; and `src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:requiredArgument`.
- **[TEST-BACKED] Current Zenbot partial activity path:** `internal/command/activity.go`, `internal/command/activity_test.go`, `internal/service/activity.go`, `internal/service/activity_test.go`, `internal/repository/activity.go`, `internal/repository/h2/activity.go`, and `internal/repository/h2/activity_test.go` all exist and were read—not inferred from filenames. The focused command/service/real-H2/catalog run below passed in this dirty worktree.
- **[TEST-BACKED] Accepted neighboring work:** `lastonline` is already concrete, capability-gated, real-H2-covered, and accepted by `.hermes/handoffs/last-online-qa.md`; it is not part of this vertical. Its command (`internal/command/last_online.go`), service (`service.UserService.LastOnline`), repository interface/query, H2 tests, and acceptance record were inspected.
- **[TEST-BACKED] Generic fallback status:** `internal/command/handlers.go:newCommand` resolves canonical `active` to `*activityCommand`; `internal/command/admin_moderator_catalog_guard_test.go:allowedScopedGenericFallbacks` excludes `active`; catalog tests pass. Thus the remaining work is not catalog activation or a fallback replacement.

## Observed Saturn contract

1. `ActivityCommandImpl` is `MODERATOR`, with exact aliases `active`, `activity`.
2. `ActivityCommandImpl.execute` calls `requiredArgument(0, "active 8Wotmg")`. `UserCommandBaseImpl.requiredArgument` accepts only a nonblank first argument and returns its trimmed value. Missing/blank input has already emitted the source usage reply; this command returns `FAILED` after `Example: <prefix>active 8Wotmg`.
3. On a valid target, the command calls `engine.sqlService.executeActivityStats(target)`, always sends `Stats: \\n` plus the returned string to the author using the inbound whisper route, then returns `SUCCESSFUL`.
4. `SQLServiceImpl.executeActivityStats` binds the trimmed target to the source CTE, returns `No activity found.` for zero rows, and formats nonempty output as literal `\\n```Text\\n` + `TableGenerator.generateTable(...)` + literal `\\n ```. `TableGenerator` uses Java `String.length()` (UTF-16 code units), two-space padding, even-width columns, and the exact table borders/padding.
5. Crucially, `SQLServiceImpl.executeActivityStats` catches `SQLException` and returns `e.getMessage()` **as a result string**. Therefore `ActivityCommandImpl` still sends `Stats: \\n<database-error-text>` and returns `SUCCESSFUL`. This is the source contract even though it is not a desirable error policy.
6. The source CTE groups exact stored `trip` values by day/hour, filters the final rows with `LOWER(trip) = LOWER(?)`, orders `trip, day_number, hour`, and computes each exact-case trip's percentage independently.

There is no focused Saturn `ActivityCommandImpl` test under `src/test`; source class/service/table inspection is the authority for the error-path and formatting details.

## Current Zenbot state and precise gap

### Already source-aligned partial work

- `internal/command/registry.go:RegisterAll` retains the exact `active`/`activity` aliases and `model.MODERATOR` role.
- `internal/command/handlers.go:newCommand` uses the concrete `activityCommand`, not `saturnCommand`.
- `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` registers canonical `active` only when `service.Bundle.Activity` and `Activity.Repo` are nonnil; expansion through the definition exposes both aliases.
- `internal/factory/engine_factory.go:NewEngineWithOptions` composes `service.ActivityService` only when the supplied repository implements `repository.ActivityRepository`.
- `internal/repository/h2/activity.go:Database.ActivityStats` uses a bound `$1` H2 PostgreSQL-wire query, trims input, preserves case-insensitive final filtering/exact-case grouping/order, and returns typed rows.
- `internal/service/activity.go:ActivityService.Stats` renders the Saturn no-row string and a UTF-16-aware TableGenerator-compatible payload.
- Current targeted proof passed:

```text
go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory \
  -run 'Test(Activity|RegisterUserUtilitiesAddsActivity|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' \
  -count=1 -v
# PASS: real-H2 activity query/no-row; service renderer/no-row/error; command aliases/role/first arg/whisper/usage/error/cancellation/registration; catalog guards
```

### Strict-parity gap to close

`internal/service/activity.go:ActivityService.Stats` currently returns an error when `Repo.ActivityStats` fails. `internal/command/activity.go:activityCommand.Execute` converts that to `FAILED` and produces no reply. The existing tests intentionally lock this non-source behavior:

- `internal/service/activity_test.go:TestActivityServicePropagatesRepositoryError`
- `internal/command/activity_test.go:TestActivityCommandDoesNotReplyOnServiceFailure`

This differs from Saturn's `SQLServiceImpl.executeActivityStats` catch-and-return behavior and `ActivityCommandImpl.execute` successful `Stats:` reply. The selected vertical is strictly limited to reconciling that error-result contract while preserving all already-working input/query/render/registration behavior.

## Exact file, interface, and data-flow map

```text
Inbound chat
  -> listener command dispatch / legacyAdapter
  -> common.CommandDefinition.New (registry canonical active)
  -> handlers.go:newCommand -> activityCommand.Execute(ctx)
  -> service.Bundle.Activity (*service.ActivityService)
  -> repository.ActivityRepository.ActivityStats(ctx, trimmedTrip)
  -> h2.Database.ActivityStats -> bound H2 CTE / typed []ActivityStat
  -> ActivityService.Stats -> source table/error-result string
  -> command.reply -> engine.SendChatMessage(author, text, inbound whisper mode)
```

| Path / symbol | Required responsibility for this vertical |
|---|---|
| `internal/command/activity.go:activityCommand.Execute` | Preserve cancellation before work/reply, source first-argument trimming/usage/whisper behavior, and treat the source-shaped service result as replyable text. It must return `SUCCESSFUL` after a valid invocation including the source query-error-result case. |
| `internal/service/activity.go:ActivityService.Stats` | Keep `Stats(ctx, trip) (string, error)` if possible to avoid widening interfaces. Convert an `ActivityStats` execution error to its `err.Error()` result with a nil command-level error, mirroring `SQLServiceImpl`; retain true Go-side only errors only where no source-equivalent result can be constructed (none are currently expected in this renderer). |
| `internal/repository/activity.go:ActivityRepository` | No interface expansion: retain `ActivityStats(context.Context, string) ([]ActivityStat, error)`. It remains the transport/persistence error boundary. |
| `internal/repository/h2/activity.go:Database.ActivityStats` | No query or schema rewrite. Continue returning H2/database errors to the service so the service maps them to source result text. |
| `internal/command/activity_test.go` | Replace the current no-reply/error assertion with exact source-shaped `Stats: \\n<error>` reply and `SUCCESSFUL`; retain aliases, MODERATOR role, first argument, usage/no-query, whisper, cancellation, and capability-gating tests. |
| `internal/service/activity_test.go` | Replace error propagation expectation with exact returned database-error text and nil error. Keep renderer/no-row tests; add an error-text regression that is not tied to a driver-specific SQL wording. |
| `internal/repository/h2/activity_test.go` | Retain real-H2 case/group/order/no-row tests. Add no fake SQL parser or broad persistence changes. |
| `internal/factory/engine_factory.go`, `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/command/registry.go`, `internal/command/admin_moderator_catalog_guard_test.go` | Read-only regression surfaces for this slice unless a test disproves the current facts above. Do not co-edit their accepted/shared hunks merely to make the vertical appear self-contained. |

## Explicit exclusions

- Do not modify `lastonline` command/service/repository/tests or its accepted output/query behavior.
- Do not modify S2 simple moderation, S3 active-target moderation, S4 shadow-ban code/tests, ADMIN prefix code/tests, generic fallback allowlist, raw moderation operations, agent semantic moderation, listener order, replica/snapshot/lifecycle machinery, configuration, schema/bootstrap, or the full migration catalog.
- Do not port unrelated SQL-service operations, arbitrary ADMIN `sql`, `automove`, `nuke`, `resurrect`, or any remote/proxy capability.
- Do not “improve” Saturn's error disclosure or substitute an unavailable/friendly fallback; strict parity requires the bounded source error-result behavior.
- Preserve the existing dirty worktree: no reset, clean, checkout, staging, commit, or push. This architecture task itself only creates this handoff.

## TDD implementation sequence

1. Start by recording `git status --short --branch`; treat all current dirty/untracked paths as pre-existing. Read the exact current `ActivityCommandImpl`, `SQLServiceImpl.executeActivityStats`, `TableGenerator.generateTable`, and target activity tests before edits.
2. **RED, service:** change/add one deterministic test for a sentinel repository error. It must expect `ActivityService.Stats` to return that error's text and a nil error, matching source `SQLException -> e.getMessage()`.
3. **RED, command:** replace/add a command test using the same sentinel error. Assert one exact author reply `Stats: \\n<sentinel text>`, preserved whisper flag, `model.SUCCESSFUL`, and nil error. Assert the repository was called exactly once with the trimmed first token.
4. Run the two RED tests independently and record the failures against the current propagation/no-reply implementation.
5. **GREEN:** make the smallest change in `ActivityService.Stats` to turn repository execution errors into result text; retain its current signature and avoid any registry/factory/repository/schema edit. Confirm `activityCommand.Execute` now sends the result and returns successful for the valid target path.
6. Run focused service/command tests, then the real-H2 activity tests and catalog/registration tests. Add no production code for cancellation: retain the current no-side-effect canceled-context behavior as a target runtime safety invariant, documented as an intentional Go boundary rather than Saturn evidence.
7. Run the scoped regression and full gate. Inspect `git diff --check`; compare status to the pre-task status. Only the activity production/test files needed by the behavior change may differ, plus this handoff if retained.

## Required verification commands

Run in `/Users/ab/workspace/go-projects/zenbot` after the focused RED step and again after GREEN:

```bash
go test ./internal/service -run 'TestActivityService' -count=1 -v
go test ./internal/command -run 'TestActivity(Command|.*Activity|RegisterUserUtilitiesAddsActivity)' -count=1 -v
go test ./internal/repository/h2 -run '^TestActivityStats' -count=1 -v
go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory \
  -run 'Test(Activity|RegisterUserUtilitiesAddsActivity|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' \
  -count=1 -race
go test ./internal/repository/h2 ./internal/service ./internal/command ./internal/factory -count=1
go test ./... -count=1
go vet ./internal/command
git diff --check
git status --short --branch
```

Do **not** treat `go vet ./internal/core` as an activity acceptance gate. The known out-of-scope diagnostic remains:

```text
internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

It must be recorded but not fixed by this slice.

## Acceptance conditions

1. `active`/`activity` remain exactly MODERATOR aliases in `RegisterAll`, resolve to `*activityCommand`, and are absent from the generic fallback allowlist.
2. A missing/blank first argument sends exactly `Example: <current-prefix>active 8Wotmg`, returns `FAILED`, and executes no query; a valid invocation trims and uses only the first argument and preserves whisper delivery.
3. For zero rows, reply is `Stats: \\nNo activity found.` and status is `SUCCESSFUL`.
4. For rows, the reply has Saturn's literal escaped newline/fence/table/trailing-space shape; table width behavior remains Java UTF-16-compatible. Real-H2 proof confirms case-insensitive final filtering, exact-case grouping, day/hour ordering, and bound input.
5. For a repository/database error, target returns `SUCCESSFUL`, no Go error, and sends exactly `Stats: \\n` plus the underlying error text. This must be covered by both service and command tests.
6. A canceled Go context before or during the query emits no reply and returns `FAILED`; this is retained target cancellation safety and must not be misrepresented as Saturn's source behavior.
7. Conditional factory/registration behavior remains: activity is composed/registered only when the typed Activity service/repository seam exists.
8. All listed focused/race/package/full-suite/diff gates pass. The accepted prefix evidence (`.hermes/handoffs/admin-prefix-qa.md`) records `go test ./... -count=1` as PASS for all packages, and the accepted S4 evidence independently records the same full-suite gate as PASS; this activity work must not regress that baseline.
9. The only known nonzero static check remains the explicitly out-of-scope `go vet ./internal/core` copylock warning above. No unrelated dirty path is changed, and no commits/pushes occur.

## Evidence limitations

Saturn has no focused activity command or SQL rendering test located in its current test tree. The contract above is therefore source-derived from the exact command/service/table symbols, while the Zenbot behavior is locked by deterministic command/service and real-H2 tests. No application code, test, config, plan, Saturn source, or existing handoff was modified by this architecture pass.
