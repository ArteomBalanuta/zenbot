# `dbzstats` strict-TDD provenance forensic

## Verdict

**`dbzstats` does not have valid, preserved strict-TDD tracer evidence for its full authorized command-dispatch → real-H2 → reply path.** Current package tests are green, but current green is not RED→GREEN evidence.

The implementation and its tests first appear together in commit `48777d8029be8741d430b07559b37352177d793a` (`feat: migrate Saturn parity foundation`, 2026-08-31 10:34:02 +03:00). Its parent, `41b8b12`, contains none of the DBZ command/service/repository files or tests. No later commit changes the DBZ-specific files; `f9079ca` only changes shared composition/dispatch files. Therefore history cannot show a prior failing `dbzstats` tracer, the expected failure, or a subsequent minimal production GREEN change.

Do not accept the current passing packages as strict TDD provenance or as end-to-end acceptance.

## Exact history and handoff evidence

| Evidence | What it establishes | What it does not establish |
|---|---|---|
| `git log --all --reverse -- internal/command/dbz.go internal/command/dbz_test.go internal/service/dbz.go internal/service/dbz_test.go internal/repository/dbz.go internal/repository/h2/dbz.go internal/repository/h2/dbz_test.go` | The only path-history commit is `48777d8`. | No independent pre-implementation test commit or RED run exists in reachable history. |
| `git ls-tree -r 48777d8^ -- <all DBZ paths>` | Empty output: the parent has no DBZ implementation or tests. | It cannot prove ordering inside the one aggregate commit. |
| `git show 48777d8` | Production (`dbz.go`, service, repository, H2 implementation), DBZ tests, registry/adapter/factory composition, and DBZ handoffs were introduced in the same commit. | A same-commit test beside production code is not proof that the test was run and observed failing first. |
| `.hermes/handoffs/dbz-architecture.md` | Architecture specified listener-level DBZ aliases, whisper, role rejection, and real-H2 testing as required (lines 179–194). | It is a design/test plan, not execution evidence. |
| `.hermes/handoffs/dbz-implementation.md` | The implementation report explicitly says listener-level golden coverage for aliases, whisper payload, role rejection, and DBZ-less dispatch was not added (lines 62–65). | It provides no RED command/output for `dbzstats`. |
| `.hermes/handoffs/dbz-qa.md` | QA reported component/H2 checks and explicitly retained listener-level golden gaps (lines 48–94). | Its PASS verdict is scoped QA, not strict-TDD tracer provenance. |
| `.hermes/handoffs/dbz-command-current-architecture.md` | It correctly identifies the absent authorized real-H2 dispatch tracer and proposes its exact shape (lines 86–120). | It is an untracked current architecture handoff, created after the DBZ implementation; it is not antecedent RED evidence. |

No relevant existing handoff records an independently committed/retained sequence of: (1) a new authorized `!dbzstats` test, (2) an observed expected failure because the DBZ read path was absent/broken, (3) the minimal production change, and (4) the passing rerun.

## What current tests actually prove

### Component and persistence evidence

- `internal/command/dbz_test.go`
  - Proves catalog aliases have `REGULAR` metadata and concrete construction is available with a **stub** DBZ repository.
  - Does **not** execute `dbzstats`, assert its stats reply, call `DispatchUserCommand`, authorize a principal, or use H2.
- `internal/service/dbz_test.go`
  - Proves `StatsText` exact seven-line formatting from a **stub** repository.
  - Does **not** prove the caller name reaches the service through command dispatch, H2 query behavior, or output transport.
- `internal/repository/h2/dbz_test.go`
  - Proves real-H2 registration/mutations and missing `Stats`/`FreeStats` repository behavior.
  - Does **not** execute command construction, authorization, dispatch, service rendering, or a chat reply.
- `internal/listener/message/dispatch_authorization_test.go`
  - Proves generic authorization ordering with synthetic commands and synthetic output.
  - Does **not** register/execute a DBZ command or use a real H2 DBZ service.
- `internal/command/dispatch_integration_test.go`
  - Exercises selected utility aliases through `UserChatListener`, but its DBZ assertion is only that `dbz` remains unregistered when the test engine has no DBZ bundle.

### Implemented but untraced production path

The production chain is present: DBZ canonical/aliases are `REGULAR` in `internal/command/registry.go`; `RegisterUserUtilitiesWithDirectAgent` conditionally registers the DBZ canonicals when `Bundle.DBZ != nil`; dispatch resolves and authorizes before `ExecuteContext`; `dbzCommand.Execute` calls `StatsText(ctx, message.Name)`; `StatsText` calls `Repo.Stats`; H2 runs the joined query; `reply` carries inbound whisper state.

This source inspection shows intended wiring, not acceptance evidence that one authorized inbound DBZ request produces the exact real-H2 reply.

## Acceptance status

| Acceptance claim | Status | Reason |
|---|---|---|
| Exact service stats text | Component-tested, but no strict-TDD provenance | Stub service test passes; no preserved RED→GREEN sequence. |
| Missing-row repository semantics | Real-H2-tested, but no strict-TDD provenance | H2 test passes; no preserved RED→GREEN sequence. |
| DBZ aliases/role/construction | Component-tested, but no strict-TDD provenance | Metadata test passes with a stub repository. |
| Authorized inbound `!dbzstats` / `!ds` uses the active caller name, reads real H2, and sends exactly one reply | **Not accepted** | No test spans dispatch, authorization, registered adapter, concrete DBZ command, real H2, and captured reply. |
| Whisper preservation on that real-H2 tracer | **Not accepted** | `reply` source inspection preserves whisper flags, but no DBZ dispatch test asserts it. |
| Missing-character reply on that same dispatch path | **Not accepted** | Repository-only missing read is insufficient. |
| Denied `REGULAR` DBZ request does not invoke DBZ read and uses shared unauthorized behavior | **Not accepted** | Generic dispatch authorization coverage is not DBZ/read-repository evidence. |
| Strict TDD provenance for the DBZ read vertical | **Failed / unavailable** | No retained antecedent RED and Green transition exists. |

## Owned versus unrelated worktree hunks

### Historical DBZ-owned introduction (`48777d8`)

The DBZ slice introduced these owned areas together: `internal/repository/dbz.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/dbz_test.go`, `internal/service/dbz.go`, `internal/service/dbz_test.go`, `internal/service/services.go` (bundle field), `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/command/handlers.go`, `internal/command/registry.go`, `internal/command/dispatch_adapter.go`, and `internal/factory/engine_factory.go`. The three tracked DBZ handoffs were introduced in the same foundation commit.

### Current dirty worktree

At inspection, no DBZ-specific production/test/schema file has an unstaged or staged change:

- `internal/command/dbz.go`, `internal/command/dbz_test.go`
- `internal/service/dbz.go`, `internal/service/dbz_test.go`
- `internal/repository/dbz.go`
- `internal/repository/h2/dbz.go`, `internal/repository/h2/dbz_test.go`, `internal/repository/h2/schema-h2.sql`
- `resources/schema-h2.sql`

Shared files are heavily dirty for unrelated migration work. The only DBZ-text match in their current zero-context diff is whitespace alignment of the existing `Bundle.DBZ` field in `internal/service/services.go`; no DBZ behavior changed. Current edits to `dispatch_adapter.go`, `handlers.go`, `registry.go`, `engine_factory.go`, and `listener/message/handlers.go` are shared-infrastructure/other-command work. They must not be attributed to the DBZ read vertical without a separate scoped review.

## Precise missing test-only tracer and TDD classification

The missing acceptance tracer should use real production seams where feasible:

1. Isolated real H2 fixture with a distinct `goku` DBZ row (for example level 2, free 5, stats 3/4/5/6).
2. Real `service.DBZService` over that H2 database in the engine bundle.
3. Normal DBZ registration through `RegisterUserUtilities...`.
4. Resolved active `model.User{Name: "goku"}`, an authorization seam permitting `REGULAR`, and a recording `SendChatMessage`.
5. `DispatchUserCommand.Handle` for an inbound whisper `!ds` (and separately a missing-row `!dbzstats`).
6. Assertions: exactly one reply to `goku`, whisper true, and the exact seven-line text; then exact missing-row text. A denied variant must show no DBZ read/reply beyond the shared unauthorized response.

**Classification:** adding this test now is a necessary recovery of the untested end-to-end acceptance boundary, **not** a post-GREEN regression test under strict TDD. The present implementation already exists, so a correctly written tracer is expected to pass immediately; that result is tests-after evidence and cannot retrospectively become a RED observation.

If the project accepts the existing implementation as a legacy baseline, the tracer may still be added as valuable regression coverage, but it must be labelled a provenance waiver/post-implementation regression test—not strict TDD. To recover strict provenance for production behavior itself, the team must intentionally establish a controlled RED against an absent/disabled behavior and then reintroduce the minimum production path, or explicitly approve a legacy-code exception. A test-harness-only failure (such as initially missing real-H2 composition) is not a valid production RED.

## Verification performed on this dirty checkout

```text
go test -count=1 ./internal/command ./internal/service ./internal/repository/h2
ok  zenbot/internal/command        10.038s
ok  zenbot/internal/service         2.247s
ok  zenbot/internal/repository/h2  37.392s

git diff --check && git diff --cached --check
exit 0; no output
```

These results establish current package health and whitespace validity only. They do not close the acceptance or provenance gaps above.
