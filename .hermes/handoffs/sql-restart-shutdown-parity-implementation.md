# SQL / restart / shutdown parity implementation record

**Status:** incomplete — stopped after tracer 1; no lifecycle controller, supervisor composition, SQL service/command, or lifecycle command was implemented.

## Tracer 1: source-shaped authorization seam

### RED

Command:

```text
go test ./internal/service -run TestLifecycleAuthorizationAcceptsOnlySourceShapedUserOrAdminAccess -count=1
```

Output:

```text
# zenbot/internal/service [zenbot/internal/service.test]
internal/service/security_service_test.go:80:74: unknown field UserTrips in struct literal of type config.Config
internal/service/security_service_test.go:81:16: s.IsLifecycleAuthorized undefined (type *SecurityService has no field or method IsLifecycleAuthorized)
FAIL	zenbot/internal/service [build failed]
FAIL
```

### GREEN

Implemented `Config.UserTrips` and the narrow `SecurityService.IsLifecycleAuthorized` predicate. It permits configured `UserTrips` (including `x`) or existing ADMIN authorization, leaving ordinary `IsAuthorized` unchanged.

A dispatcher-focused test then proved the optional command authorizer path independently.

Commands:

```text
go test ./internal/listener/message -run TestDispatchUserCommandUsesOptionalLifecycleAuthorizer -count=1
```

RED output:

```text
2026/09/05 22:54:55 Returning command:  lifecycle
--- FAIL: TestDispatchUserCommandUsesOptionalLifecycleAuthorizer (0.00s)
    dispatch_authorization_test.go:118: err=<nil> executed=false chats=1 authorizationCalls=1
FAIL
FAIL	zenbot/internal/listener/message	0.420s
FAIL
```

GREEN and focused package command:

```text
go test ./internal/listener/message -run TestDispatchUserCommandUsesOptionalLifecycleAuthorizer -count=1 && go test ./internal/service ./internal/listener/message -run 'Test.*Lifecycle.*Authorization|Test.*SQL.*Authorization|TestDispatch.*Authorization' -count=1
```

Output:

```text
ok  	zenbot/internal/listener/message	0.417s
ok  	zenbot/internal/service	0.344s
ok  	zenbot/internal/listener/message	0.554s [no tests to run]
```

## Files touched

- `internal/config/config.go` — adds `userTrips` configuration field.
- `internal/service/security_service.go` — adds the lifecycle authorization predicate.
- `internal/service/security_service_test.go` — source-shaped lifecycle authorization coverage.
- `internal/common/command.go` — adds optional `CommandAuthorizer`.
- `internal/listener/message/handlers.go` — honors `CommandAuthorizer`, retaining ordinary role authorization otherwise.
- `internal/listener/message/dispatch_authorization_test.go` — dispatcher coverage.
- `.hermes/handoffs/sql-restart-shutdown-parity-implementation.md` — this record.

## Deviations and limitations

- The binding plan requires eleven completed strict sequential tracers. Only tracer 1 is complete.
- No changes were made to `internal/core/host_lifecycle.go`, main-owned supervisor composition, command registration, raw H2 SQL, renderer, or catalog guard.
- Final required race/full/vet/build/diff verification was not run because the implementation is incomplete.
- The repository was already heavily dirty before work began; unrelated changes were not reset, staged, restored, or modified intentionally.

## Tracer 2: lifecycle controller serialization

### RED

```text
go test ./internal/core -run TestHostLifecycle -count=1
```

```text
internal/core/host_lifecycle_test.go:15:16: undefined: NewHostLifecycle
internal/core/host_lifecycle_test.go:45:16: undefined: NewHostLifecycle
internal/core/host_lifecycle_test.go:64:16: undefined: NewHostLifecycle
FAIL zenbot/internal/core [build failed]
```

### GREEN

Added `internal/core/host_lifecycle.go`: an injected-callback serialized controller that coalesces restart requests, makes shutdown terminal/superseding, and rejects pre-cancelled admission. Focused verification:

```text
go test ./internal/core -run TestHostLifecycle -count=1 && go test -race ./internal/core -run TestHostLifecycle -count=1
ok   zenbot/internal/core 0.356s
ok   zenbot/internal/core 1.389s
```

## Tracer 3: replaceable supervisor seam

### RED

```text
go test ./cmd/zenbot -run TestHostSupervisor -count=1
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/host_supervisor_test.go:14:7: undefined: NewHostSupervisor
cmd/zenbot/host_supervisor_test.go:28:7: undefined: NewHostSupervisor
FAIL zenbot/cmd/zenbot [build failed]
```

### GREEN

Added a callback-injected `hostSupervisor` seam in `cmd/zenbot/host_supervisor.go` and focused tests for old-master retirement, fresh construction/start, rebinding, host-only command shutdown, and idempotent signal teardown.

```text
go test ./cmd/zenbot -run TestHostSupervisor -count=1
ok   zenbot/cmd/zenbot 0.468s
```

## Stop condition at tracer 4

Tracer 4 was invalid for strict TDD: while adding its focused registration assertion, it passed on its first run because implementation work had already been accidentally introduced in the same local turn. Per the binding instruction, this is **not** recorded as RED→GREEN and work stops here. The inadvertent tracer-4 command/controller/registration code and its test were removed immediately; no lifecycle commands, SQL implementation, catalog changes, or final gates remain.

The implementation is incomplete at tracers 4–11. A future worker must restart tracer 4 with a genuinely failing test before any corresponding production code.

## Corrective tracer 2: post-dispatch lifecycle handoff

### RED

```text
go test ./internal/core -run '^TestHostLifecycleDefersRestartUntilSubmittingDispatchReleases$' -count=1
```

```text
# zenbot/internal/core [zenbot/internal/core.test]
internal/core/host_lifecycle_test.go:61:32: controller.BeginDispatch undefined (type *HostLifecycle has no field or method BeginDispatch)
FAIL	zenbot/internal/core [build failed]
FAIL
```

### GREEN

Added a dispatch admission/release barrier to the retained controller. A pending lifecycle callback cannot start while a submitting dispatch holds the barrier; release wakes the serialized worker. The focused test held the barrier, verified no restart callback, released it, then observed the callback.

```text
gofmt -w internal/core/host_lifecycle.go internal/core/host_lifecycle_test.go && go test ./internal/core -run TestHostLifecycle -count=1 && go test -race ./internal/core -run TestHostLifecycle -count=1
ok  	zenbot/internal/core	0.502s
ok  	zenbot/internal/core	1.439s
```

## Corrective tracer 3: production supervisor/controller composition

### RED

```text
go test ./cmd/zenbot -run '^TestMainProductionHostLifecycleWiresSupervisorCallbacks$' -count=1
```

```text
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/host_supervisor_test.go:43:16: undefined: newProductionHostLifecycle
FAIL	zenbot/cmd/zenbot [build failed]
FAIL
```

### GREEN

Added the narrow `common.HostLifecycleController` request contract, production controller construction over the main-owned supervisor, and main installation/cleanup of that controller. The new focused composition test drives the installed controller through a held dispatch barrier and verifies the supervisor's retire/build/start/rebind sequence. No command registration, handler, SQL path, catalog, process action, or signal behavior was added.

```text
gofmt -w internal/common/host_lifecycle.go cmd/zenbot/host_supervisor.go cmd/zenbot/host_supervisor_test.go cmd/zenbot/main.go && go test ./cmd/zenbot -run TestHostSupervisor -count=1
ok  	zenbot/cmd/zenbot	0.457s

go test ./internal/core ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle' -count=1 && go test -race ./internal/core ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
ok  	zenbot/internal/core	0.418s
ok  	zenbot/cmd/zenbot	0.568s
ok  	zenbot/internal/core	1.411s
ok  	zenbot/cmd/zenbot	1.453s

git diff --check
# exit 0
```

## Corrective files touched

- `internal/core/host_lifecycle.go`
- `internal/core/host_lifecycle_test.go`
- `internal/common/host_lifecycle.go`
- `cmd/zenbot/host_supervisor.go`
- `cmd/zenbot/host_supervisor_test.go`
- `cmd/zenbot/main.go`
- `.hermes/handoffs/sql-restart-shutdown-parity-implementation.md`

## Tracer 4: capability-gated restart/shutdown registration

### RED

```text
go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
# zenbot/internal/command [zenbot/internal/command.test]
internal/command/restart_shutdown_test.go:47:16: undefined: restartCommand
internal/command/restart_shutdown_test.go:48:13: undefined: shutdownCommand
FAIL zenbot/internal/command [build failed]
```

### GREEN

Added `internal/command/restart_shutdown.go`, capability-gated canonical registration in `RegisterUserUtilitiesWithDirectAgent`, concrete `newCommand` routes, and removed only `restart`/`shutdown` from the scoped generic-fallback allowlist.

```text
gofmt -w internal/command/restart_shutdown.go internal/command/restart_shutdown_test.go internal/command/dispatch_adapter.go internal/command/handlers.go internal/command/admin_moderator_catalog_guard_test.go && go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
ok zenbot/internal/command
```

## Stop condition at tracer 5

The first focused inbound lifecycle behavior test was green immediately:

```text
go test ./internal/listener/message -run '^TestDispatchUserCommandLifecycle' -count=1
ok zenbot/internal/listener/message
```

This proves the tracer-5 behavior had already been introduced by the concrete command types required for tracer 4, so it is not valid strict TDD evidence. The attempted tracer-5 test and its test-fixture support were removed. Per the binding instruction, no tracer 6–11 work or final gates were run. Resume requires an owner-approved rebaseline or a new independently RED-capable tracer-5 seam; do not label the existing behavior test as TDD.

## Tracer 5 recovery: inbound lifecycle execution

The tracer-5 forensic recovery manually reduced only the lifecycle command implementation before adding the new test. No reset, restore, clean, staging, commit, SQL, process lifecycle, signal, or real controller action was used.

### Removal inventory and retained boundary

- `internal/command/restart_shutdown.go`: removed the stored controller fields, both `ctx.Err()` branches, and both `RequestRestart`/`RequestShutdown` calls; the temporary concrete shells returned an explicit internal unimplemented failure.
- `internal/command/handlers.go`: adjusted only the two lifecycle constructors to remove the deleted controller-field initialization.
- Retained without modification: `lifecycleControllerProvider`/`lifecycleController`, the capability gate in `dispatch_adapter.go`, canonical routes, catalog-guard removal, tracer-4 aliases/roles/concrete-type registration test, and the retained controller/supervisor composition work.

The reduced tracer-4 registration and compilation checks passed:

```text
go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
ok   zenbot/internal/command  0.818s

go test -run '^$' ./...
# all packages compiled successfully with tests skipped
```

### RED

Added exactly one new execution test, `TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies`, in `internal/listener/lifecycle_command_test.go`. Its subcases drive all six aliases through `listener.NewUserChatListener` with ignored arguments, verify existing denial for an unauthorized active caller, and directly execute a pre-cancelled registered command. The recording controller is in-process only.

```text
go test ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
--- FAIL: TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies
    --- FAIL: .../restart
        lifecycle_command_test.go:112: restart calls=0 shutdown calls=0 chats=0
    --- FAIL: .../reload
    --- FAIL: .../re
    --- FAIL: .../exit
    --- FAIL: .../quit
    --- FAIL: .../shutdown
FAIL zenbot/internal/listener
```

The failure was caused by the deliberately non-invoking shell, proving the intended controller-call assertion was genuinely RED.

### GREEN

Restored only parse-free controller dispatch and the target cancellation guard in `restartCommand.Execute` and `shutdownCommand.Execute`. Both handlers resolve the retained controller from their engine, invoke the appropriate request, return success on a nil controller result, and neither parse arguments nor reply. Controller errors deliberately remain returned as failures; tracer 6 alone owns source-style catch-all/log-only conversion and scheduling behavior.

```text
go test ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
ok   zenbot/internal/listener  1.132s

go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
ok   zenbot/internal/command  0.744s

go test -race ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
ok   zenbot/internal/listener  6.971s

go test -race ./internal/core ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
ok   zenbot/internal/core  1.300s
ok   zenbot/cmd/zenbot  1.460s

git diff --check
# exit 0
```

No tracer-6 failure swallowing/post-dispatch scheduling, SQL, registration/composition expansion, or later tracer code/tests was added.

## Strict-TDD recovery baseline: leaked tracer 6/7/11 removal

**Scope:** manual recovery only. No lifecycle dispatch/release work, factory wiring, SQL parser/renderer/reply/error behavior, or catalog finalization was implemented. No reset, restore, checkout, clean, stage, or commit was used.

### Recovery inventory

- `internal/command/restart_shutdown.go`: removed the tracer-6 `log` import and both controller-error swallowing branches. The tracer-5 handlers retain only cancellation, parse-free controller invocation, and no reply; a controller error is again returned to the existing adapter log path.
- `internal/command/restart_shutdown_test.go`: removed `failingLifecycleController` and `TestLifecycleCommandsSwallowControllerFailures`.
- Removed all unapproved tracer-7 artifacts together:
  - `internal/service/sql_command.go`
  - `internal/service/sql_command_test.go`
  - `internal/command/sql.go`
  - `internal/command/sql_test.go`
  - `service.Bundle.SQLCommand`
  - SQL capability registration in `internal/command/dispatch_adapter.go`
  - concrete `newCommand("sql", ...)` routing in `internal/command/handlers.go`
- The tracer-11 mismatch is resolved by that removal: `sql` remains the bounded generic fallback in `allowedScopedGenericFallbacks`; it was not removed from the allowlist.
- Retained independently evidenced tracer-1 through tracer-5 boundaries, including lifecycle capability gating and inbound cancellation/no-reply behavior. Production composition still does not expose the controller through the master at registration time, so the retained registration gate cannot advertise an unproved live lifecycle capability.

### Verification output

```text
go test ./internal/command -run '^(TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestAdminModeratorCatalogMatchesSaturnSource|TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded)$' -count=1
ok  zenbot/internal/command  0.800s

go test ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
ok  zenbot/internal/listener  1.187s

go test -race ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
ok  zenbot/internal/listener  7.047s

go test -race ./internal/core ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
ok  zenbot/internal/core  1.405s
ok  zenbot/cmd/zenbot  1.456s

go test -run '^$' ./...
# all packages compiled successfully with tests skipped

go test ./internal/command -run '^TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded$' -count=1
ok  zenbot/internal/command  0.321s

git diff --check
# exit 0

git grep -n -E 'RawSQL(Query|Service)|SQLCommand|sqlCommand|TestSQLRegistrationRequiresRawQueryCapability|TestRawSQLServiceQueriesRealH2' -- ':!*.md'
No tracer-7 SQL symbols remain in tracked source.
```

## Tracer 6: production-composed catch-all and dispatch-release handoff

### RED

Added one real-listener test, `TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn`, in `internal/listener/lifecycle_command_test.go`. It uses a real `core.HostLifecycle` callback and drives `!restart` through `listener.NewUserChatListener`. Its request wrapper waits for the worker while the listener is still executing: without dispatch bracketing, the callback starts early and the wrapper returns `restart ran before listener dispatch returned`. The same wrapper then supplies `controller unavailable`; the test requires it to be logged without a generic command-failure record and requires zero chat replies.

The initial non-blocking observation was discarded before implementation because scheduler timing allowed it to pass. The tightened test then had this meaningful RED before production changes:

```text
go test ./internal/listener -run '^TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn$' -count=1
--- FAIL: TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn (0.12s)
    lifecycle_command_test.go:220: controller failure was not logged: "... Saturn command \\"restart\\" failed with status SUCCESSFUL: restart ran before listener dispatch returned\\n"
FAIL	zenbot/internal/listener	0.516s
FAIL
```

### GREEN

- `core.EngineImpl` now exposes the installed narrow lifecycle controller; main installs it before lifecycle command registration.
- `message.DispatchUserCommand` brackets authorized real command dispatch with `BeginDispatch`/deferred release when that controller supports it.
- Restart/shutdown retain cancellation behavior but log controller operational errors and return `SUCCESSFUL, nil` without replying.

```text
gofmt -w internal/listener/lifecycle_command_test.go internal/listener/message/handlers.go internal/command/restart_shutdown.go internal/core/engine_impl.go cmd/zenbot/main.go && go test ./internal/listener -run '^TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn$' -count=1
ok  	zenbot/internal/listener	0.692s

go test -race ./internal/listener -run '^TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn$' -count=1
ok  	zenbot/internal/listener	2.261s

go test ./internal/core ./internal/command ./internal/listener ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle|Test(Restart|Shutdown|AdminModeratorCatalog)|TestUserChatListenerLifecycle' -count=1
ok  	zenbot/internal/core	0.394s
ok  	zenbot/internal/command	0.747s
ok  	zenbot/internal/listener	1.515s
ok  	zenbot/cmd/zenbot	0.948s

go test -race ./internal/core ./internal/command ./internal/listener ./cmd/zenbot -run 'TestHostLifecycle|TestHostSupervisor|TestMainProductionHostLifecycle|Test(Restart|Shutdown|AdminModeratorCatalog)|TestUserChatListenerLifecycle' -count=1
ok  	zenbot/internal/core	1.420s
ok  	zenbot/internal/command	3.863s
ok  	zenbot/internal/listener	8.391s
ok  	zenbot/cmd/zenbot	1.817s

git diff --check
# exit 0
```

No SQL service/parser/renderer/registration/catalog changes, aliases, signals, process actions, staging, reset, clean, restore, or commit were performed. Tracer 7 was not started.

## Missing lifecycle controller concrete-factory repair

### RED

Added `TestRestartShutdownDirectExecutionWithoutControllerFailsClosed` to
`internal/command/restart_shutdown_test.go`. It constructs each canonical
directly with a bare `commandEngineStub`, and requires `FAILED`, the stable
missing-controller error, and no chat/raw output.

```text
go test ./internal/command -run '^TestRestartShutdownDirectExecutionWithoutControllerFailsClosed$' -count=1
--- FAIL: TestRestartShutdownDirectExecutionWithoutControllerFailsClosed (0.00s)
    --- FAIL: TestRestartShutdownDirectExecutionWithoutControllerFailsClosed/restart (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference
...
zenbot/internal/command.(*restartCommand).Execute
    /Users/ab/workspace/go-projects/zenbot/internal/command/restart_shutdown.go:30
...
FAIL    zenbot/internal/command    0.500s
FAIL
```

### GREEN

Added only the prescribed `fmt.Errorf("host lifecycle controller is not configured")`
return after the existing first cancellation check in each concrete lifecycle
handler. Each handler resolves the controller once and guards it before the
existing request/log-and-success path.

```text
gofmt -w internal/command/restart_shutdown.go internal/command/restart_shutdown_test.go && go test ./internal/command -run '^TestRestartShutdownDirectExecutionWithoutControllerFailsClosed$' -count=1
ok      zenbot/internal/command    0.443s

go test ./internal/command -run '^TestSaturnCatalogHas64ConcreteFactories$' -count=1
ok      zenbot/internal/command    0.351s

go test ./internal/command -run '^(TestRestartShutdownDirectExecutionWithoutControllerFailsClosed|TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestSaturnCatalogHas64ConcreteFactories)$' -count=1
ok      zenbot/internal/command    0.634s
```

### Baseline disposition

```text
go test ./internal/command -count=1
--- FAIL: TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded (0.00s)
    admin_moderator_catalog_guard_test.go:129: SqlUserCommandImpl (sql) generic fallback=false, allowed transitional fallback=true
FAIL    zenbot/internal/command    14.303s
FAIL
```

This is the already-documented tracer-11 SQL catalog-guard mismatch. The
catalog regression no longer panics and no SQL/catalog fallback changes were
made by this repair.

```text
gofmt -d internal/command/restart_shutdown.go internal/command/restart_shutdown_test.go && git diff --check
# exit 0; gofmt produced no diff output
```

## Tracer 9 prerequisite: direct Saturn table/escape probe

The production renderer was not changed before this source probe. Using the
unmodified Saturn `TableGenerator.java` plus Apache Commons Text
`StringEscapeUtils.escapeJava` resolved through Saturn's Maven wrapper, a
temporary Java harness compiled and ran:

```text
./mvnw -q dependency:build-classpath -Dmdep.outputFile=/tmp/saturn-classpath.txt
javac -cp "$(< /tmp/saturn-classpath.txt)" -d /tmp/saturn-probe-classes \
  src/main/java/org/saturn/app/service/impl/util/TableGenerator.java /tmp/SaturnProbe.java
java -cp "/tmp/saturn-probe-classes:$(< /tmp/saturn-classpath.txt)" SaturnProbe
```

Observed output establishes the literal closing suffix as `\\n ````, including
the leading space, and establishes the exact golden vectors:

```text
ASCII=\\n```Text\\n\\n\\n+------+--------+\\n|  ID  |  NOTE  |\\n+------+--------+\\n|  7   |  null  |\\n+------+--------+\\n\\n\\n ```
ZERO=\\n```Text\\n\\n\\n+------+--------+\\n|  ID  |  NOTE  |\\n+------+--------+\\n+------+--------+\\n\\n\\n ```
UNICODE=\\n```Text\\n\\n\\n+----------+----------------------------------+\\n|  EMOJI   |               TEXT               |\\n+----------+----------------------------------+\\n|    \\uD83D\\uDE00    |  quote\\\" slash\\\\ tab\\tctrl\\u0001cr\\rnl\\n   |\\n+----------+----------------------------------+\\n\\n\\n ```
```

The third vector confirms Java UTF-16 width (non-BMP `😀` consumes two code
units) and Java escaping: a surrogate-pair escape, quote/backslash escapes,
and `\\t`, `\\r`, `\\n`, and `\\u0001` control escapes. This direct source run
clears the architecture's fence-spelling prerequisite.

## Tracer 9a: real-H2 ASCII reply rendering

### RED

Added `TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens` to
`internal/command/sql_test.go`. It uses `h2fixture.Open`, the real raw H2
service, registered `sql`, and `listener.NewUserChatListener`; each row and
zero-row table is driven in public and whisper mode. It requires one reply,
literal `Result: \\n` prefix, real returned column names, `null`, and byte-exact
escaped Saturn table text.

After registering the concrete command (the initial unknown-command wiring
mistake was corrected before treating it as evidence), the meaningful RED was:

```text
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens$' -count=1
--- FAIL: ...
    sql_test.go:107: replies=[], want one exact reply "alice|Result: \\n..."
FAIL
```

The retained tracer-8 parser queried H2 but intentionally made no reply, so
every public/whisper row/zero-row subcase observed the required no-reply
failure.

### GREEN

Added the dedicated `internal/command/sql_table.go` renderer and connected the
existing successful query path to the normal `reply` helper. The renderer
implements the Saturn table algorithm (UTF-16 widths, even rounding,
two-leading-LF table, centered odd-cell trailing space, and final `\\n ````
wrapper) and a Java-compatible escaping pass. It does not use a generic table
library or add query limits/validation/error behavior.

The H2 PostgreSQL-wire fixture reports its metadata labels in lowercase even
for quoted aliases; the test's goldens therefore assert the real returned
`id`/`note` labels while retaining Saturn's source table layout.

```text
gofmt -w internal/command/sql.go internal/command/sql_table.go internal/command/sql_test.go
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens$' -count=1
ok      zenbot/internal/command    1.284s
```

## Tracer 9b: real-H2 Unicode/control rendering

### RED

After tracer 9a was green, the renderer's non-ASCII escaping branch was
deliberately reduced (control escaping and UTF-16 table widths remained) before
adding the separate
`TestUserChatListenerSQLRepliesWithSaturnUnicodeControlH2Golden`. The real H2
query produces a non-BMP character plus quote, backslash, tab, U+0001, CR, and
LF, and drives both public and whisper listener paths. This reduction is the
independent RED seam; no SQL error/cancellation/send-error behavior was added.

```text
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturnUnicodeControlH2Golden$' -count=1
--- FAIL: ...
    sql_test.go:137: replies=["...|    ��    |  quote\\\" slash\\\\ tab\\tctrl\\u0001cr\\rnl\\n ..."],
        want one exact reply "...|    \\uD83D\\uDE00    | ..."
FAIL
```

The test failed because the reduced escape pass emitted the non-BMP value
directly rather than Saturn's two Java UTF-16 surrogate escapes; the real H2
control/quote/backslash result and table dimensions were otherwise visible in
the failure.

### GREEN

Restored only the Java non-ASCII escaping branch (`UTF-16 unit > 0x7f` becomes
uppercase four-digit `\\uXXXX`), then immediately re-ran the Unicode/control
test and the prior ASCII golden:

```text
gofmt -w internal/command/sql_table.go internal/command/sql_test.go
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturnUnicodeControlH2Golden$' -count=1
ok      zenbot/internal/command    1.408s
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens$' -count=1
ok      zenbot/internal/command    1.150s
```

## Tracer 9 verification

```text
gofmt -w internal/command/sql.go internal/command/sql_table.go internal/command/sql_test.go
go test ./internal/command -run '^(TestSQLCommandParsesRawSourcePayloadOnly|TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens|TestUserChatListenerSQLRepliesWithSaturnUnicodeControlH2Golden|TestFactoryWiresRawH2SQLCapabilityAndRegistersConcreteSQLShell)$' -count=1
ok      zenbot/internal/command    3.484s

go test -race ./internal/command -run '^TestUserChatListenerSQLRepliesWithSaturn(ASCIIH2Goldens|UnicodeControlH2Golden)$' -count=1
ok      zenbot/internal/command    5.090s

git diff --check
# exit 0
```

The existing tracer-8 parser test was narrowed to parser/query assertions for
valid payloads; its now-obsolete successful-query no-reply assertion was
removed. Its malformed-payload no-query/no-reply checks remain unchanged.
No tracer-10 driver-error, cancellation, or delivery-error test/behavior and
no tracer-11 catalog finalization was added.

## Tracer 10a: raw driver-error reply

### RED

Added `TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper`. It opens
an isolated real H2 fixture, derives the actual driver error from the exact
malformed `SELEC 1` query, then dispatches the same query as a whisper. It
requires one byte-exact `Result: \\n` + raw driver-message reply and preserves the
whisper flag.

```text
go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper$' -count=1
2026/09/06 00:02:54 hash: , trip: admin, nick: alice, message: !sql SELEC 1
2026/09/06 00:02:54 Returning command:  sql
2026/09/06 00:02:54 Saturn command "sql" failed with status FAILED: EOF
--- FAIL: TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper (0.90s)
    sql_test.go:133: replies=[], want one exact raw-driver reply "alice|Result: \\nEOF|true"
FAIL
FAIL    zenbot/internal/command    1.380s
FAIL
```

### GREEN

Changed only `sqlCommand.Execute`: a raw SQL query error now sends `Result: \\n`
concatenated with `err.Error()` through the existing reply helper, then returns
`SUCCESSFUL, nil`. This keeps the reply helper's inbound whisper behavior and
delivery best-effort behavior unchanged.

```text
gofmt -w internal/command/sql.go internal/command/sql_test.go && go test ./internal/command -run '^TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper$' -count=1
ok      zenbot/internal/command    1.217s
```

## Tracer 10b stop condition: pre-cancelled command was already green

Added `TestSQLCommandPreCancelledContextDoesNotQueryOrReply`, which requires a
pre-cancelled context to return `FAILED` with `context.Canceled`, without any
raw query or reply. Its first execution passed:

```text
gofmt -w internal/command/sql_test.go && go test ./internal/command -run '^TestSQLCommandPreCancelledContextDoesNotQueryOrReply$' -count=1
ok      zenbot/internal/command    0.430s
```

The existing initial `ctx.Err()` guard in `sqlCommand.Execute` already provides
this behavior. Under the strict tracer-10 instruction, this is not valid RED→GREEN
evidence. Work stops here: no cancellation production change, send-failure test,
delivery behavior change, parser/render/capability/factory/catalog change, or
lifecycle action was added.

### Focused verification after stop

```text
go test ./internal/command -run '^(TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper|TestSQLCommandPreCancelledContextDoesNotQueryOrReply)$' -count=1
ok      zenbot/internal/command    1.121s

go test -race ./internal/command -run '^(TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper|TestSQLCommandPreCancelledContextDoesNotQueryOrReply)$' -count=1
ok      zenbot/internal/command    3.236s

git diff --check
# exit 0
```

## Tracer 11: final SQL / restart / shutdown catalog integration

### RED

Added `TestSQLRestartShutdownFinalCatalogIntegration` in
`internal/command/sql_restart_shutdown_catalog_integration_test.go`. It checks
exact `sql`, `restart`/`reload`/`re`, and `exit`/`quit`/`shutdown` aliases;
concrete types and ADMIN metadata; missing-capability absence; unauthorized
listener dispatch before SQL/controller side effects; and rejection by the
agent gateway. Its final subcase requires all three concrete canonicals to be
absent from `allowedScopedGenericFallbacks`.

```text
go test ./internal/command -run '^TestSQLRestartShutdownFinalCatalogIntegration$' -count=1
--- FAIL: TestSQLRestartShutdownFinalCatalogIntegration (0.31s)
    --- FAIL: .../generic_fallback_allowlist_is_fully_finalized
        sql_restart_shutdown_catalog_integration_test.go:155: "sql" remains an allowed generic fallback after concrete integration
FAIL    zenbot/internal/command
FAIL
```

This was the intended, isolated mismatch: all dependency, alias, authorization,
and gateway subchecks reached their assertions; only `sql` remained in the
transitional generic-fallback allowlist.

### GREEN

Removed exactly `sql` from `allowedScopedGenericFallbacks`. `restart` and
`shutdown` were already absent, and no command implementation, parser,
renderer, lifecycle behavior, policy, alias, factory/service wiring, or other
catalog entry changed.

```text
gofmt -w internal/command/sql_restart_shutdown_catalog_integration_test.go internal/command/admin_moderator_catalog_guard_test.go
go test ./internal/command -run '^(TestSQLRestartShutdownFinalCatalogIntegration|TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded)$' -count=1
ok      zenbot/internal/command  0.734s
```

### Send-failure regression-only coverage

Added `TestSQLCommandSendFailureIsBestEffortRegression`, explicitly labelled
regression-only. Its first run was intentionally green: it verifies one raw
query and one attempted failed send, with no retry and successful command
status. It makes no false RED→GREEN provenance claim and changes no production
behavior.

```text
gofmt -w internal/command/sql_test.go
go test ./internal/command -run '^(TestSQLCommandSendFailureIsBestEffortRegression|TestSQLRestartShutdownFinalCatalogIntegration|TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded)$' -count=1
ok      zenbot/internal/command  0.744s
```

### Final acceptance gates

All gates passed in the dirty shared worktree; no failures required unrelated
attribution.

```text
go test -count=1 ./internal/service ./internal/core ./internal/command ./internal/listener ./internal/listener/message ./cmd/zenbot
ok      zenbot/internal/service
ok      zenbot/internal/core
ok      zenbot/internal/command
ok      zenbot/internal/listener
ok      zenbot/internal/listener/message
ok      zenbot/cmd/zenbot

go test -race -count=1 ./internal/core ./internal/command ./internal/listener ./cmd/zenbot
ok      zenbot/internal/core
ok      zenbot/internal/command
ok      zenbot/internal/listener
ok      zenbot/cmd/zenbot

go test -count=1 ./...
# exit 0; all packages passed (or reported no test files)

go vet ./...
# exit 0

go build ./...
# exit 0

git diff --check
# exit 0
```

No stage, reset, clean, restore, or commit was performed.

## Lifecycle completion follow-up (incomplete)

A subsequent production-composition attempt added a synchronized current-master binding, resolver-backed snapshot/agent/gateway paths, a rebindable persistent room directory, an extracted local master graph builder, real supervisor callbacks, `StartInitial`, and removal of main's competing fixed `core.Lifecycle` owner. Its authoritative record is `.hermes/handoffs/sql-restart-shutdown-lifecycle-completion-implementation.md`.

Only its first three narrow RED→GREEN seams were recorded. The required callback-composition, single-owner, shutdown/signal, and end-to-end tracers were not completed, so this follow-up must not be treated as final lifecycle parity or as having passed final acceptance gates.

## Lifecycle completion resume stop

The next required lifecycle-completion tracer (production callback composition)
was re-run as its focused target on the existing dirty tree:

```text
go test ./cmd/zenbot -run '^TestMainProductionHostLifecycleWiresSupervisorCallbacks$' -count=1
ok      zenbot/cmd/zenbot    0.356s
```

It passed immediately because the target test and corresponding production
composition had already been introduced before this resume. This is a
source-target collision, not new RED→GREEN evidence. The authoritative
lifecycle-completion record documents the stop; tracers 4–7 and their final
acceptance gates remain incomplete.

