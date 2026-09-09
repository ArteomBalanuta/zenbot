# SQL / restart / shutdown tracer-7 boundary forensic

**Verdict: hunk-by-hunk manual recovery is required. Do not resume at the remaining tracer-7 factory wire or begin tracer 8.**

**Inspected state:** `/Users/ab/workspace/go-projects/zenbot`, branch `migration/saturn-zenbot-parity`, dirty and unstaged. This forensic created this handoff only; it did not edit Go source/tests, stage, reset, restore, clean, or commit.

## Independently observable current behavior

### Lifecycle tracer 5 is present and exercised

`internal/listener/lifecycle_command_test.go:88-150` dispatches all six aliases through the real user-chat listener against an in-process controller. It proves one matching request, no success reply, ignored arguments, denial before a controller call, and no call for a pre-cancelled direct execution. The focused test passes.

`internal/command/restart_shutdown.go:26-47` contains those tracer-5 execution semantics, plus more:

- cancellation returns `FAILED` with no controller call;
- valid requests call `RequestRestart`/`RequestShutdown`;
- no reply is sent.

### Tracer-6 catch-all behavior has leaked into the handler

`restart_shutdown.go:30-33` and `:44-47` log a controller error and nevertheless return `SUCCESSFUL, nil`. `internal/command/restart_shutdown_test.go:81-95` passes with a failing controller and observes only successful status/nil error.

That is the tracer-6 failure-swallowing behavior, but it is not sufficient independent tracer-6 evidence:

- the test does not route through dispatch, assert zero replies, or assert the log record;
- the required no-self-deadlock behavior is not connected to real dispatch. `HostLifecycle.BeginDispatch` is called only by `internal/core/host_lifecycle_test.go:61` and `cmd/zenbot/host_supervisor_test.go:49`, not by `DispatchUserCommand.Handle` (`internal/listener/message/handlers.go:180-204`) or production composition;
- production main constructs `hostLifecycle` (`cmd/zenbot/main.go:261-268`) but never exposes it through `HostLifecycleController()` on the master. The capability gate at `internal/command/dispatch_adapter.go:87-89` therefore excludes restart/shutdown in the real process. The test stubs are the only engines exposing that method.

Consequently, tracer 6 is neither a clean completed TDD boundary nor an operationally complete vertical. The production lifecycle-installation/dispatch-release prerequisite also remains incomplete despite the isolated controller/supervisor tests passing.

### Partial tracer 7 is present and locally tested

The following tracer-7 portions are implemented:

- `internal/service/sql_command.go:9-60` defines `RawSQLQuery`, `SQLTable`, and a raw `db.QueryContext` service that reads column labels, drains rows, renders nil as `null`, and closes rows.
- `internal/service/sql_command_test.go:10-23` uses real H2 and passes for `SELECT 7 ... NULL`.
- `service.Bundle.SQLCommand` exists (`internal/service/services.go:27-42`).
- `RegisterUserUtilitiesWithDirectAgent` gates `sql` on a non-nil bundle capability (`internal/command/dispatch_adapter.go:90-92`).
- `newCommand("sql", ...)` resolves a concrete `*sqlCommand` (`internal/command/handlers.go:385-386`), and `internal/command/sql_test.go:17-37` proves stub capability registration/absence.

The required factory wire is absent. `factory.NewEngineWithOptions` obtains `repo.SQLDB()` and creates `e.Services` at `internal/factory/engine_factory.go:73-90`, but never assigns `SQLCommand: &service.RawSQLService{DB: db}`. Therefore production factory engines have a nil SQL capability and cannot register `sql`; no factory test covers the required wire.

`internal/command/sql.go:10-18` is intentionally only a failing/unavailable shell. It neither accesses the raw-query bundle nor parses, queries, renders, replies, handles query errors, or implements SQL cancellation behavior.

## Premature later-tracer leakage audit

No tracer-8, 9, or 10 SQL behavior was found in the SQL command path:

- no literal `sql ` split, literal `\\n` conversion, case-sensitive parser behavior, or malformed-payload no-reply path;
- no table renderer, Java escaping, UTF-16 width handling, `Result: \\n` reply, whisper routing, raw driver-error reply, or SQL-specific cancellation/send-failure test;
- no reference from the human SQL command path to `internal/agent/sql` policy.

One tracer-11 catalog-finalization hunk **has** leaked prematurely: `newCommand` is already concrete for `sql`, but `allowedScopedGenericFallbacks` still contains `sql` (`internal/command/admin_moderator_catalog_guard_test.go:117-119`). The final catalog guard fails exactly as a result:

```text
SqlUserCommandImpl (sql) generic fallback=false, allowed transitional fallback=true
```

This is not a valid finalization state. Tracer 11 is not otherwise implemented; it must not be repaired by simply removing `sql` from the allowlist now.

## Required manual recovery, in order

Do not use reset/restore/clean/staging or broad deletion in this shared dirty worktree.

1. **Restore a tracer-5-only lifecycle command boundary.** In `internal/command/restart_shutdown.go`, retain only the already-tested cancellation guard and parse-free controller call. Remove the `log` import and the two error-swallowing/logging branches; return the request error/status in the pre-tracer-6 form. Remove `TestLifecycleCommandsSwallowControllerFailures` with the behavior it pre-established. Keep the tracer-5 listener test and the tracer-4 capability registration test.
2. **Recover the missing production prerequisite before tracer 6.** Add a new RED proving the real master exposes the installed controller to command registration and that actual dispatch brackets the request with `BeginDispatch`/release so callback execution starts only after dispatch returns. Make the smallest production composition/dispatcher change GREEN, then rerun focused lifecycle and race tests. Do not regard a manually invoked `BeginDispatch` in a core-only test as proof of the listener path.
3. **Restart tracer 6 from a fresh RED.** Recreate a focused dispatch-facing failure test that proves controller failure produces successful/no-reply behavior, and a channel-backed real-dispatch release test. Watch it fail against the recovered tracer-5 handler, then add only the log-only failure bridge and post-dispatch handoff. Preserve no parser or SQL changes in this cycle.
4. **Rebaseline tracer 7 only after tracer 6 is green.** The raw service/bundle/registration shell already exists, but its strict RED history is not recorded. Either obtain explicit owner approval to retain those independently passing artifacts, then write a new factory-wire RED; or manually remove all tracer-7 artifacts together (`sql_command.go`, its test, `Bundle.SQLCommand`, SQL capability gate/concrete route/shell/test) and redo tracer 7 from scratch. Without that approval/recovery, a factory test added now would be a mixed green-state continuation rather than a clean tracer.
5. **Only then implement the factory wire in a single tracer-7 GREEN.** It must set `SQLCommand: &service.RawSQLService{DB: db}` only when `SQLDB()` is non-nil and prove production factory registration. Keep `sqlCommand.Execute` unavailable; parser/reply work is tracer 8+.
6. **Leave the catalog guard unchanged until tracer 11.** Its current failure is expected evidence of the premature concrete SQL route. After recovery, ensure it is green by temporarily restoring the SQL generic fallback or by removing the early concrete route as part of the approved full tracer-7 recovery. Do not remove `sql` from the allowlist until tracer 11 has its own witnessed RED.

## Scoped baseline record

Passed in the inspected state:

```text
go test ./internal/command -run '^(TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestLifecycleCommandsSwallowControllerFailures|TestSQLRegistrationRequiresRawQueryCapability)$' -count=1
go test ./internal/service -run '^TestRawSQLServiceQueriesRealH2$' -count=1
go test ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
go test -race ./internal/core ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
go test -run '^$' ./...
git diff --check
```

The focused catalog guard intentionally fails as documented above. The compile-only repository baseline passes; none of these checks validates a clean strict-TDD history for the leaked tracer-6/7 code.
