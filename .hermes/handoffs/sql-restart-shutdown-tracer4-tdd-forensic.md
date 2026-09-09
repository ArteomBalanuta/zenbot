# SQL / restart / shutdown tracer-4 strict-TDD forensic boundary

**Forensic status:** tracer-4 production/test artifacts are absent. The repository is compilable, but the approved plan does **not** yet have a fully evidenced tracer-1–3 completion boundary. Strict TDD may not safely begin tracer 4 until the retained tracer-2 and tracer-3 seams are first completed by new corrective RED cycles (or an owner explicitly narrows the binding specification). No source/test was modified by this forensic review.

**Repository inspected:** `/Users/ab/workspace/go-projects/zenbot` at branch `migration/saturn-zenbot-parity`, `HEAD f9079ca`. The worktree was already substantially dirty: 41 tracked modified files, many unrelated untracked files, and no staged changes. This report is the only file created by this review.

## Independently evidenced retained work

### Tracer 1 — retained and focused-test verified

The following tracked diff hunks match the tracer-1 authorization seam:

- `internal/config/config.go`: `Config.UserTrips []string` with `toml:"userTrips"`.
- `internal/service/security_service.go`: `SecurityService.UserTrips`; `NewSecurityService` copies it; `IsLifecycleAuthorized` accepts configured exact/equal-folded user trips and literal `x`, otherwise delegates to existing ADMIN authorization.
- `internal/common/command.go`: optional `CommandAuthorizer { Authorize(*model.User) bool }` only; the ordinary `Command` contract was not changed.
- `internal/listener/message/handlers.go`: `isCommandAuthorized` consults the optional authorizer then retains `Engine.IsUserAuthorized` for all other commands. It also passes a dispatch context to commands which opt into `ExecuteContext`.
- `internal/service/security_service_test.go`: `TestLifecycleAuthorizationAcceptsOnlySourceShapedUserOrAdminAccess` covers configured user trip, `x`, configured admin, persisted ADMIN, and an ordinary denied user.
- `internal/listener/message/dispatch_authorization_test.go`: `TestDispatchUserCommandUsesOptionalLifecycleAuthorizer` proves the optional predicate runs without the ordinary engine authorization call.

Observed verification passed:

```text
go test ./internal/service ./internal/listener/message \
  -run 'Test.*Lifecycle.*Authorization|Test.*SQL.*Authorization|TestDispatch.*Authorization' -count=1
ok   zenbot/internal/service
ok   zenbot/internal/listener/message [no tests to run]
```

The listener package's specifically named dispatch test was present in source and is included by the broader `TestDispatch.*Authorization` expression even though Go reports no separate listener test under the other two patterns.

### Tracer 2 — retained controller/test, but not the binding tracer in full

Untracked, retained files:

- `internal/core/host_lifecycle.go`: `HostLifecycle`, `NewHostLifecycle`, asynchronous serialized worker, restart coalescing, terminal shutdown, and pre-cancelled request rejection.
- `internal/core/host_lifecycle_test.go`: `TestHostLifecycleSerializesAndCoalescesRequests`, `TestHostLifecycleShutdownSupersedesRestartAndIsTerminal`, and `TestHostLifecycleDoesNotAdmitCanceledRequest`.

Observed verification passed:

```text
go test ./internal/core -run TestHostLifecycle -count=1
ok   zenbot/internal/core

go test -race ./internal/core -run TestHostLifecycle -count=1
ok   zenbot/internal/core
```

**Binding gap:** approved tracer 2 requires a test proving a restart callback runs *only after the submitter releases dispatch*. The retained implementation signals the worker in `RequestRestart` before the method returns (lines 46–51), and the worker may invoke the callback as soon as it observes that signal (lines 79–101). The retained tests cover serialization/coalescing/terminal/cancellation but have no dispatch-release barrier. Thus this code is retained and locally tested, but it is not independent proof of the no-self-deadlock/post-dispatch requirement.

### Tracer 3 — retained supervisor/test, but not installed production composition

Untracked, retained files:

- `cmd/zenbot/host_supervisor.go`: callback-injected `hostSupervisor` with retire/build/start/rebind restart sequence, host-only shutdown, and idempotent signal teardown.
- `cmd/zenbot/host_supervisor_test.go`: fake-driven replacement/rebind ordering and shutdown/idempotence tests.

Observed verification passed:

```text
go test ./cmd/zenbot -run TestHostSupervisor -count=1
ok   zenbot/cmd/zenbot
```

**Binding gap:** a complete tracer 3 must extract and install main-owned supervisor/factory composition. Repository-wide exact-symbol search found `NewHostSupervisor` only in these two retained files; no construction/use appears in `cmd/zenbot/main.go`. There is also no `internal/common/host_lifecycle.go` command-facing interface. The fake seam is tested, but no production master construction/rebinding/controller installation is evidenced. It cannot yet make restart/shutdown command registration safe.

## Exact tracer-4+ artifact inventory

### Confirmed absent (no manual removal is needed)

The following approved-plan paths do not exist in the worktree, tracked or untracked:

```text
internal/common/host_lifecycle.go
internal/command/restart_shutdown.go
internal/command/restart_shutdown_test.go
internal/command/sql.go
internal/command/sql_table.go
internal/command/sql_test.go
internal/service/sql_command.go
internal/service/sql_command_test.go
```

Repository-wide Go-symbol search also found none of:

```text
restartCommand
shutdownCommand
sqlCommand
HostLifecycleController
RawSQLQuery
RawSQLService
SQLCommand
```

There are no concrete `restart`/`shutdown` routes in `newCommand`, no capability-gated registration in `RegisterUserUtilitiesWithDirectAgent`, no lifecycle controller contract, no raw-SQL bundle field/service/factory wiring, and no focused tracer-4 test. The alleged accidental tracer-4 registration code/test is therefore absent.

### Present but pre-existing/non-registration catalog material — retain

The static Saturn catalog still contains definitions for `restart` (`restart`, `reload`, `re`), `shutdown` (`exit`, `quit`, `shutdown`), and `sql` (`sql`) in `internal/command/registry.go:204`. `internal/command/handlers.go` leaves all three in the generic `saturnCommand` fallback (its `newCommand` switch has no concrete cases), and `internal/command/admin_moderator_catalog_guard_test.go:118` still lists all three in `allowedScopedGenericFallbacks`.

These are the expected pre-tracer-4 state. They are **not registered** by `internal/command/dispatch_adapter.go`: its canonical registration list has none of `restart`, `shutdown`, or `sql`. Do not remove these catalog/fallback entries before a genuine tracer-4 RED; tracer 4 is specifically the vertical that establishes capability gating/concrete routes and then reduces the generic allowlist.

### Residual unproven code: do not delete automatically

No residual tracer-4 artifact needs removal. However, the retained tracer-2/3 code is not sufficient evidence for their full binding requirements. If an owner decides it was introduced outside a valid prior RED rather than accepting it as retained earlier work, manual removal—not an automated reset—would be required:

1. Delete the complete untracked `internal/core/host_lifecycle.go` and `internal/core/host_lifecycle_test.go` files together.
2. Delete the complete untracked `cmd/zenbot/host_supervisor.go` and `cmd/zenbot/host_supervisor_test.go` files together.
3. Do **not** edit the unrelated dirty tracked files, the static registry catalog, fallback allowlist, tracer-1 authorization hunks, or any other untracked work.
4. Recreate tracers 2 and 3 from a witnessed RED, one vertical at a time.

This review did not apply those removals.

## Baseline commands and observed results

```text
git diff --check
# exit 0

go test -run '^$' ./...
# exit 0; every package compiled (no tests run)

go vet ./...
# exit 0; no output

go test ./internal/core -run TestHostLifecycle -count=1
# ok zenbot/internal/core

go test -race ./internal/core -run TestHostLifecycle -count=1
# ok zenbot/internal/core

go test ./cmd/zenbot -run TestHostSupervisor -count=1
# ok zenbot/cmd/zenbot
```

The primary tree is therefore compilable and vet-clean at the inspected dirty state. These are baseline/compilation checks, not the final migration acceptance suite.

## Minimal safe strict-TDD resume point

1. **Do not begin tracer 4 yet.** First add a corrective tracer-2 RED that deterministically holds the dispatcher/submission boundary and proves no lifecycle callback begins until dispatch has released it. Observe that test fail for the missing post-dispatch handoff, then implement only that handoff and rerun the focused race test.
2. Then add a corrective tracer-3 RED that constructs the real main-owned graph through the supervisor, installs a narrow command-facing lifecycle controller, and proves retirement/fresh master/start/rebind plus host-only shutdown against production-shaped callbacks. Observe RED, then minimally wire it GREEN.
3. Re-run focused tracer-2 race and tracer-3 tests. Only after both are green and their retained code is accepted may tracer 4 start.

### Exact required first tracer-4 RED (after the above)

Create **only** `internal/command/restart_shutdown_test.go`; do not create/modify `restart_shutdown.go`, `internal/common/host_lifecycle.go`, registry definitions, factory wiring, or catalog guard first. The test must use:

- an engine without an installed lifecycle controller and assert **none** of `restart`, `reload`, `re`, `exit`, `quit`, `shutdown` is registered;
- a recording-controller engine and assert exactly those aliases are registered with `model.ADMIN`, resolving to concrete restart/shutdown command types rather than `*saturnCommand`;
- the existing catalog guard and assert it fails until only `restart` and `shutdown` are removed from `allowedScopedGenericFallbacks` (leave `sql` there).

Run exactly:

```text
go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
```

The required initial result is a meaningful failure because lifecycle capability gating and concrete `newCommand` routes do not exist—not a compile typo, not a test fixture defect, and not an already-passing test. Only after capturing that RED may the smallest tracer-4 registration/route implementation be written.
