# Lifecycle concrete-factory missing-controller repair

## Decision

**[OBSERVED]** `RegisterAll` always retains the `restart` and `shutdown` catalog definitions (`internal/command/registry.go:RegisterAll`), and `newCommand` always constructs `*restartCommand` / `*shutdownCommand` (`internal/command/handlers.go:newCommand`). Normal runtime exposure remains correctly capability-gated: `RegisterUserUtilitiesWithDirectAgent` appends those canonicals only when `lifecycleController(e) != nil` (`internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent`).

**[FAULT]** Catalog construction deliberately bypasses runtime registration: `TestSaturnCatalogHas64ConcreteFactories` constructs every definition with a bare `*commandEngineStub` and calls `Execute` (`internal/command/catalog_test.go`). That stub has no `HostLifecycleController()` provider. `lifecycleController` returns nil, then both handlers dereference it in `RequestRestart` / `RequestShutdown` (`internal/command/restart_shutdown.go`). The witnessed focused catalog test panics first at restart line 30; shutdown has the same defect.

**[REQUIRED MINIMAL REPAIR]** Make direct concrete execution fail closed when the controller is absent. Do not change catalog entries, aliases, roles, registration gating, controller composition, dispatch, SQL, test fixture capabilities, or lifecycle scheduling.

## Source and target effect

| Execution state | Required result |
|---|---|
| Cancelled before execution | Preserve current first branch: `model.FAILED, ctx.Err()`; no controller request and no reply. |
| Non-nil lifecycle controller; request succeeds | Preserve current behavior: `model.SUCCESSFUL, nil`; exactly one request; no reply. |
| Non-nil lifecycle controller; request returns error | Preserve tracer-6/source-shaped bridge: log the failure and return `model.SUCCESSFUL, nil`; no reply. |
| Missing lifecycle controller | New defensive outcome: `model.FAILED` plus a non-nil error whose exact text is `host lifecycle controller is not configured`; no request and no reply. |

The missing-controller branch is source-compatible with target capability semantics: commands are not normally registered without the controller, so an authorized inbound caller cannot reach it in correctly composed production. It closes the direct-factory/catalog seam and prevents a panic if registration/composition diverges. It must **not** change the existing no-reply, cancellation, or log-and-success behavior after a controller was successfully resolved.

## Exact production change

Only edit `internal/command/restart_shutdown.go`.

For each `Execute` method, retain the existing cancellation check as the first operation. Immediately after it, resolve the controller **once** into a local and guard it before calling either request method. Use `fmt.Errorf` (add only the `fmt` import) for the stable error.

```go
controller := lifecycleController(c.engine)
if controller == nil {
    return model.FAILED, fmt.Errorf("host lifecycle controller is not configured")
}
if err := controller.RequestRestart(ctx); err != nil {
    log.Printf("lifecycle restart request failed: %v", err)
}
return model.SUCCESSFUL, nil
```

`shutdownCommand.Execute` is structurally identical except for `RequestShutdown` and its existing `lifecycle shutdown request failed` log text. Do not factor this into registration logic or alter `lifecycleController`; its nil result remains the correct capability signal.

## First RED test

Add `TestRestartShutdownDirectExecutionWithoutControllerFailsClosed` to `internal/command/restart_shutdown_test.go`. It is a direct handler test, intentionally separate from registration gating. Use table cases:

```go
cases := []struct {
    canonical string
}{
    {canonical: "restart"},
    {canonical: "shutdown"},
}
for _, tt := range cases {
    t.Run(tt.canonical, func(t *testing.T) {
        engine := &commandEngineStub{users: map[string]*model.User{}}
        command := newCommand(tt.canonical, nil, model.ADMIN, engine,
            &model.ChatMessage{Text: "!" + tt.canonical, Name: "alice"})

        status, err := command.Execute(context.Background())

        if status != model.FAILED { /* report */ }
        if err == nil || err.Error() != "host lifecycle controller is not configured" { /* report */ }
        if len(engine.chats) != 0 || len(engine.raws) != 0 { /* report */ }
    })
}
```

The production change is deliberately not written before this test has been witnessed RED. On the current baseline, the direct subcases panic; depending on test structure, the first `restart` subcase aborts before shutdown runs. The test's explicit acceptance criterion is no panic plus the failed/error/no-output result for **both** canonicals after GREEN.

Do not use `RegisterUserUtilities` for this test: registration correctly hides these aliases without a controller and would not exercise the catalog-factory defect. Do not add a fake capability to `commandEngineStub`, because that would mask the seam catalog construction must support safely.

## Required preservation tests

Keep the existing `TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete` unchanged as coverage that missing-capability runtime registration exposes none of the six aliases and a controller-backed engine exposes concrete handlers.

Add or retain focused behavior tests only if missing after the RED test:

1. **Cancellation precedence:** construct each direct handler with a recording controller and an already-cancelled context. Assert `FAILED, context.Canceled`, zero restart/shutdown calls, and no chat/raw output. This proves cancellation still wins over capability resolution.
2. **Non-nil success:** recording controller receives exactly one corresponding request; handler returns `SUCCESSFUL, nil`; no output.
3. **Non-nil request failure:** controller returns a sentinel error; handler still returns `SUCCESSFUL, nil`, with no output. The log need not be asserted.

These are behavior-preservation checks, not a reason to broaden this repair into lifecycle/controller tests.

## Verification sequence

1. Witness the new direct-execution test RED before production edit:

```text
go test ./internal/command -run '^TestRestartShutdownDirectExecutionWithoutControllerFailsClosed$' -count=1
```

2. Apply the minimal guard and run the same test GREEN.
3. Run the deterministic catalog regression that currently panics:

```text
go test ./internal/command -run '^TestSaturnCatalogHas64ConcreteFactories$' -count=1
```

It must complete without a lifecycle nil-pointer panic. The package may then stop at the known independent SQL catalog-guard mismatch documented in `.hermes/handoffs/sql-restart-shutdown-tracer7-failure-forensic.md`; do not remove `sql` from `allowedScopedGenericFallbacks` or otherwise repair that tracer-11-owned condition here.

4. Run focused lifecycle-command coverage and formatting checks:

```text
gofmt -w internal/command/restart_shutdown.go internal/command/restart_shutdown_test.go
go test ./internal/command -run '^(TestRestartShutdownDirectExecutionWithoutControllerFailsClosed|TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestSaturnCatalogHas64ConcreteFactories)$' -count=1
git diff --check
```

No real restart, shutdown, signal, process exit, supervisor, database, or SQL operation belongs in this repair.

## Scope boundary

This specification adds one defensive nil-capability return in each concrete lifecycle handler and one narrow direct-construction regression. It intentionally leaves the broader already-installed lifecycle controller, capability-gated registration, no-reply operational contract, and the unrelated SQL transitional guard untouched.
