# Tracer-7 baseline failure forensic: SQL catalog state vs. restart nil controller

**Verdict:** the SQL catalog guard failure is the expected pre-tracer-11 intermediate state. The restart panic is an independent, real nil-safety defect in the concrete lifecycle handler/test seam. Do not start tracer 8 until the restart/shutdown panic has a witnessed RED→GREEN recovery and `go test ./internal/command -count=1` reaches the expected SQL-only guard failure.

## Exact baseline

```text
go test ./internal/command -count=1
```

fails in this order:

1. `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded`

   ```text
   SqlUserCommandImpl (sql) generic fallback=false, allowed transitional fallback=true
   ```

2. `TestSaturnCatalogHas64ConcreteFactories` panics:

   ```text
   zenbot/internal/command.(*restartCommand).Execute
   internal/command/restart_shutdown.go:30
   zenbot/internal/command.TestSaturnCatalogHas64ConcreteFactories
   internal/command/catalog_test.go:29
   ```

The focused reproduction is deterministic:

```text
go test ./internal/command -run '^TestSaturnCatalogHas64ConcreteFactories$' -count=1
```

It produces the same nil-pointer panic at `restart_shutdown.go:30`.

## Expected SQL failure — do not repair now

`newCommand("sql", ...)` now returns concrete `*sqlCommand` (`internal/command/handlers.go:385-386`), while `allowedScopedGenericFallbacks` still contains `sql` (`internal/command/admin_moderator_catalog_guard_test.go:117-119`). The guard intentionally asserts that genericness exactly equals the transitional allowlist membership, so it reports `generic fallback=false, allowed transitional fallback=true`.

This is the documented early tracer-11 catalog-finalization mismatch. Current SQL is deliberately only a tracer-7 registration shell: `internal/command/sql.go:10-18` returns an unavailable error after its cancellation check; no tracer-8 parser/query/reply behavior is present. The real-H2 factory/capability check is present in `internal/command/sql_registration_test.go` and the factory wire sets `Bundle.SQLCommand` only for non-nil SQL DBs (`internal/factory/engine_factory.go:73-84`).

**Required disposition:** leave `sql` in `allowedScopedGenericFallbacks` until tracer 11 has its own approved/witnessed RED. Do not make the guard green by removing `sql`, and do not revert the concrete SQL route merely to mask the guard.

## Restart panic data flow

1. `RegisterAll` always includes `restart` and `shutdown` catalog definitions (`internal/command/registry.go:203-207`).
2. `newCommand("restart", ...)` and `newCommand("shutdown", ...)` always create concrete handlers (`internal/command/handlers.go:381-384`), independent of runtime capability registration.
3. Runtime registration is correctly capability-gated: `RegisterUserUtilitiesWithDirectAgent` includes lifecycle aliases only if `lifecycleController(e) != nil` (`internal/command/dispatch_adapter.go:87-89`).
4. `TestSaturnCatalogHas64ConcreteFactories` does not use runtime registration. It iterates every catalog definition, builds each against bare `&commandEngineStub{}`, and calls `Execute(context.Background())` (`internal/command/catalog_test.go:20-29`). That stub embeds `common.Engine` but does not implement `HostLifecycleController()` (`internal/command/handlers_test.go:13-25`).
5. `lifecycleController` therefore returns nil because the engine does not satisfy its private provider interface (`internal/command/restart_shutdown.go:11-20`).
6. `restartCommand.Execute` immediately calls `lifecycleController(c.engine).RequestRestart(ctx)` (`:26-33`), dereferencing nil. `shutdownCommand.Execute` has the identical unguarded `RequestShutdown` call (`:40-47`) and would panic once restart no longer stops the catalog loop.

The real production/controller route is not evidence against this defect: `core.EngineImpl` exposes its installed controller (`internal/core/engine_impl.go:270-273`), main installs it before utility registration (`cmd/zenbot/main.go:258-268`), and listener dispatch holds/releases the controller dispatch barrier around authorized execution (`internal/listener/message/handlers.go:181-225`). Those conditions make a live registered lifecycle command normally non-nil. They do not make a direct concrete factory safe, and catalog test construction intentionally exercises that unsupported-capability seam.

## Root cause and minimal TDD-safe recovery

**Root cause:** concrete factory selection was made unconditional at catalog level, but `restartCommand.Execute` and `shutdownCommand.Execute` assume the separate capability gate has already run. The catalog test deliberately bypasses that gate, supplying an engine with no lifecycle-provider method. This is not a controller scheduling, authorization, adapter-context, or production composition failure.

**First RED target:** add a narrow direct-execution regression test in `internal/command/restart_shutdown_test.go`, separate subcases for `restart` and `shutdown`, constructed through `newCommand` (or their definitions) with `&commandEngineStub{}`. It must assert:

- no panic;
- `model.FAILED` plus a non-nil missing-controller error;
- no chat/raw side effect.

The existing `TestSaturnCatalogHas64ConcreteFactories` is also a deterministic end-to-end red-capable regression target and must be rerun after the focused test.

**Minimal GREEN:** in each lifecycle handler, resolve the controller once, test it for nil before invocation, and return `model.FAILED` with a stable internal error such as `host lifecycle controller is not configured`. Retain the current cancellation check first; retain parse-free/no-reply behavior; retain the tracer-6 log-and-success bridge only after a non-nil controller is obtained. No registration change, catalog change, dispatcher change, supervisor change, SQL change, or test-fixture capability injection is required.

This missing-capability branch is not externally reachable for correctly registered commands because registration already excludes lifecycle aliases without a controller. It is the minimum defensive behavior needed for direct construction and prevents process-killing listener/test panics if composition and registration ever diverge.

## Focused checks already run

```text
go test ./internal/command -run '^TestSaturnCatalogHas64ConcreteFactories$' -count=1
# FAIL: deterministic restart nil-pointer panic

go test ./internal/command -run '^(TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded|TestSQLRegistrationRequiresRawQueryCapability)$' -count=1
# FAIL: only the intentional SQL guard mismatch; lifecycle registration and SQL capability tests do not report failure

git diff --check
# exit 0
```

No Go source/test was changed, staged, reset, restored, cleaned, or committed by this forensic.
