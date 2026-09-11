# Runtime and game command audit — 2026-09-11

Historical baseline report: references to open repairs or missing reproductions
describe that audit phase. Current scope, fixes and verification are recorded in
the [consolidated Teardown & Rebuild report](2026-09-11-command-audit-coverage.md).

Scope: read-only audit of master covering all 17 assigned command identities, their aliases, concrete/legacy construction, production registration, parsing, service/repository or lifecycle execution, cancellation, output, and agent contracts. This report records the observed implementation before corrective work. No live SQL, runtime restarts, shutdowns, or external network mutations were performed. Existing unrelated documentation changes were preserved.

The systematic-debugging skill was read and followed: trace root causes before proposing fixes. Newly identified lifecycle failures below are established by control-flow tracing; dedicated executable reproductions have not yet been added. Existing focused tests were run and passed as documented below.

## Complete command coverage

| Command | Construction and registration | Execution and audit outcome |
| --- | --- | --- |
| access | Catalog ADMIN, aliases grant/access; concrete accessCommand. Production gated on Users.GroupB and Security.Authorization. | Requires exactly two arguments and caller trip, validates uppercase role, splits comma-delimited trips, calls Authorization.GrantTrip with ctx. Individual grant uses transaction. Partial multi-target grants and cloning issue documented below. |
| memory | Catalog ADMIN, aliases mem/memory/memstats; concrete memoryCommand, always in utility registration. | Checks cancellation, runs GC, reads runtime.MemStats, formats Alloc/HeapIdle/HeapSys/Sys under inherited JVM labels. No separate substantive execution defect; shared reply errors and cloning behavior apply. |
| mine | Catalog ADMIN, hidden from agent tools; generic saturnCommand only. | Not registered in production. Generic catalog execution returns success without work. No operational mining implementation found. |
| prefix | Catalog ADMIN, concrete prefixCommand; production requires PrefixController. | Uses first nonblank argument, UpdatePrefix updates host and current managed replica snapshot under locks, then replies with previous/new prefix. Intentionally does not persist or affect subsequently created replicas. No separate substantive defect in observed update implementation. |
| replica | Catalog ADMIN, aliases replica/bot/agent, concrete handler; production capability gate. | Parses first room token; rejects host and known duplicate; constructs, starts and registers engine, then reports success. Missing replica command registration and lifecycle defects below. |
| replicaoff | Catalog ADMIN, aliases replicaoff/offline/botoff/agentoff, concrete handler. | Validates room, rejects host/nonexistent replica, manager removes entry then stops runtime. Runtime callback reentrancy deadlock and parent-context lifetime defects below affect this lifecycle. |
| replicastatus | Catalog ADMIN, aliases replicastatus/status, concrete handler. | Checks cancellation and controller; sorts snapshot and reports host/count/channels. Output can list canceled replica dispatchers as active due to lifecycle ownership defect. No separate sorting/formatting defect. |
| restart | Catalog ADMIN, aliases restart/reload/re, concrete restartCommand; production requires lifecycle controller. | Pre-cancellation check then RequestRestart. HostLifecycle serializes/coalesces admission after dispatch completion; supervisor retires old host, builds/starts replacement, then rebinds. Operational errors discarded; replica lifetime and startup deadlock affect restarts. |
| shutdown | Catalog ADMIN, aliases exit/quit/shutdown, concrete shutdownCommand; lifecycle capability gate. | Pre-cancellation check then terminal shutdown admission; supervisor stops only host. Existing tests explicitly preserve host-only behavior, while agent contract says stop bot runtime. Shared cloning issue also applies. |
| sql | Catalog ADMIN, concrete sqlCommand; production requires SQLCommand capability. | Literal source split, newline replacement, RawSQLService.Query on shared H2 DB, table rendering and reply. Embedded delimiter truncation and dispatch/parser disagreement below. Query row scanning, iteration, context and close errors are handled. |
| whiskey | Catalog ADMIN, hidden from agent; generic handler only. | Not registered in production. Generic execution explicitly returns unavailable. WhiskeyProxyOrder is an unused helper, not an operational command route. |
| dbzstr | Catalog REGULAR, aliases dbzstr/dstr/daddstr, concrete dbzCommand; production requires DBZ bundle. | Positive signed-32-bit amount; reads free stats, checks only positive balance, writes strength and replies publicly. Overspend, error masking, and clone no-op below. |
| dfight | Catalog REGULAR, aliases dfight/df, concrete dbzCommand; DBZ gate. | First enemy token; removes matching in-memory enemy, unconditionally levels caller and acknowledges publicly. Missing enemy reward, persistence ordering/error masking, and clone no-op below. |
| dbzhelp | Catalog REGULAR, aliases dbzhelp/dbz/dhelp; concrete dbzCommand, registered without DBZ service. | Pre-cancellation check, public help payload, send error propagated. Payload advertises unavailable command identities. Clone loses canonical and changes behavior. |
| dbzregister | Catalog REGULAR, aliases dbzregister/dreg/dr; concrete dbzCommand; DBZ gate. | Registers caller nickname and acknowledges publicly. Persistence errors ignored and two-step registration can leave partial state. Clone no-op below. |
| dspawn | Catalog REGULAR, concrete dbzCommand; DBZ gate. | Requires first enemy token, synchronized append allowing duplicates, public acknowledgement. Send errors propagate after state mutation. No separate substantive defect in synchronized append; clone no-op below. |
| dbzstats | Catalog REGULAR, aliases dbzstats/dstats/dstat/ds; concrete dbzCommand; DBZ gate. | Reads caller stats with context, propagates read error, renders values or not-found text, normal reply routing. No separate substantive data/formatting defect; clone and shared reply-error behavior apply. |

All concrete commands above are materialized through catalog definitions and the legacyAdapter in production. The adapter invokes definition.New directly, checks configured-user-trip exceptions for its explicit whitelist, and audits status. Agent gateway also invokes definition.New directly after its own catalog authorization. Thus SaturnCommand.NewInstance defects are real interface-contract failures but not on these two principal execution routes.

## Priority findings and execution traces

### P1: New replicas have no registered commands

`internal/factory/replica_factory.go:17-21` directly returns NewEngineWithOptions. `internal/factory/engine_factory.go:64-66` initializes EnabledCommands empty and never registers handlers. Whole-tree search found the only production RegisterUserUtilitiesWithDirectAgent invocation at `cmd/zenbot/main.go:488`, for the master. Replica listeners use normal user-chat dispatch, but there is no deferred command registration.

Trace: authorized `!replica lounge` → factory constructs empty command map → websocket joins lounge → manager registers replica → command reports success → lounge `!ping`/`!help` cannot resolve.

Regression recommendation: create a replica through production composition, dispatch ping in the replica room, and assert its response and populated registry. Existing real_websocket_replica_test verifies websocket join only.

### P1: Replica lifetime is tied to its initiating command context

`internal/core/replica_controller.go:51` passes request ctx to StartContext. `internal/core/engine_impl.go:187` derives the permanent dispatcher context from that parent, and `212-213` simply returns when canceled. The transport's websocket reader/pinger are not closed by that branch and manager registration is not removed.

Trace: human command inherits host runtime context, or agent tool uses bounded execution context → replica starts → tool cancellation or old-host retirement cancels parent → replica dispatcher exits → websocket and manager entry remain → status still reports active, duplicate start rejected, room commands no longer processed.

Regression recommendation: start under cancelable command context, cancel after successful creation, verify replica continues processing until explicit removal; repeat across host replacement. Permanent ownership must be separated from request startup cancellation.

### P1: Replica transport failure waits for its own dispatcher

`internal/core/replica_controller.go:45-49` installs synchronous runtimeFailure callback calling manager.Remove with background context. `internal/core/engine_impl.go:226-229` invokes reportTransportError from the dispatch goroutine; `163-164` calls the callback synchronously. Removal invokes StopContext, whose `250-257` waits for runtimeDone. Only the currently blocked dispatcher closes runtimeDone via its defer at `208`.

Trace: transport error → dispatcher report → synchronous callback → manager removes replica → StopContext closes transport and waits → same dispatcher cannot return to execute close(done). Background context provides no deadline.

Regression recommendation: inject a transport error after replica startup and assert manager removal and runtime completion within a bounded timeout. Cleanup must not wait synchronously on its own goroutine.

### P1: Initial join-send failure deadlocks startup

`internal/core/engine_impl.go:202-204` calls StopContext(background) when initial join send fails. The goroutine owning close(runtimeDone) starts only at `207-208`. StopContext therefore waits forever for completion from a goroutine never launched.

Trace: Transport.Start succeeds → join SendText fails, for example because startup context was canceled → StopContext closes transport and waits for runtimeDone → StartContext never returns. Applies to replicas and fresh master creation.

Regression recommendation: scripted successful transport start followed by failed join send; assert prompt error return and transport cleanup.

### P2: Fighting absent enemies grants unlimited rewards

`internal/command/dbz.go:85-87` always calls LevelUp after Fight. `internal/service/dbz.go:68-76` returns no indication whether an enemy existed. Repeated `!dfight nonexistent` therefore levels a registered character and grants five free points each time. Matching enemy is consumed before persistence succeeds.

Regression recommendation: absent and already-consumed enemy grant nothing; concurrent fights award once; failed level persistence retains the enemy. Catalog contract describes fighting a named spawned enemy.

### P2: Strength allocation overspends and is not atomic

`internal/command/dbz.go:66-76` checks only that free balance is positive. `internal/repository/h2/dbz.go:34` unconditionally adds amount and subtracts it from free_stats. With five free points, `!dstr 100` gives +100 strength and −95 free points. Separate read and update also permit concurrent overspending.

Regression recommendation: exact balance, insufficient balance, maximum accepted amount and concurrent allocations. Enforce positive affordable spend atomically in repository and check affected rows.

### P2: DBZ errors and mid-operation cancellation become success

Writes are discarded at `internal/command/dbz.go:45,76,86`. `internal/service/dbz.go:55-56` converts FreeStats read failure to −1 with nil error. Failed registration says registered, failed allocation echoes amount, failed leveling congratulates caller, and read failure says no free points. Cancellation during repository operations can also disappear through these paths. Both human output and agent outcome become misleading.

Current tests explicitly preserve these behaviors: TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails; TestDBZStrengthReadErrorPublishesSourceEquivalentPublicNoFreeSuccess; TestDBZStrengthAcknowledgesPublicAmountWhenWriteFails; TestDBZFightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy. Corrective work must update those compatibility assertions rather than preserve the defects.

Regression recommendation: each repository read/write error and cancellation returns a failed outcome without a success acknowledgement; preserve accurate committed-effect reporting where applicable.

### P2: DBZ multi-statement writes leave partial state

`internal/repository/h2/dbz.go:15-22` inserts character then stats in separate operations. `25-30` updates level then free points separately. Failure in the second write leaves an orphan character or level without points. Registration comment explicitly preserves non-atomic parity.

Regression recommendation: second-statement failure rolls back, retry registration after failed creation succeeds without orphan state, and level/free-point updates commit together.

## Other confirmed contract gaps

- SQL source truncation: `internal/command/sql.go:20-26` uses unrestricted strings.Split on `sql ` and only parts[1]. `!sql SELECT 'sql hello'` submits `SELECT '` instead of the complete requested query. Uppercase command and anagram lookup can resolve dispatch but fail this literal parser. Existing tests preserve uppercase rejection; embedded delimiter is untested. Test exact query forwarding with embedded delimiters and whitespace, plus supported dispatch spellings. Query execution, rows scanning/iteration and close errors otherwise have correct handling.
- SQL error outcome: database errors are rendered but return SUCCESSFUL (`internal/command/sql.go:36-38`), so an agent can receive succeeded status for failed SQL. This is existing compatibility behavior and should be assessed alongside shared gateway outcome reporting.
- Concrete cloning: `internal/command/dbz.go:103-104` omits canonical, causing successful no-op clones when DBZ exists. `internal/command/handlers.go:71-72` uses aliases[0] as canonical, causing access→grant, memory→mem and shutdown→exit to become generic unsupported handlers. Main human and agent routes use definition.New and do not invoke this cloning method. Add table-driven clone parity tests if preserving this interface.
- DBZ help: `internal/command/dbz.go:14-24` advertises /train, /fight, /claim, /strength, /agility, /vitality and /energy, which have no matching catalog/handler routes. Existing exact-payload test preserves this obsolete help. Derive or rewrite help around actual command names and mechanics.
- Access partial grants: `internal/command/identity_commands.go:123-125` commits each trip separately. `grant aaa,,bbb ADMIN` grants aaa then fails on empty trip; storage failure on a later target similarly leaves partial grants without identifying successes. Agent encoding rejects malformed list elements, but storage failure affects both routes. Prevalidate all targets, then transact multi-target grant or report per-target outcomes.
- Lifecycle errors: `internal/core/host_lifecycle.go:115,119` discards supervisor callback errors after successful request admission. `cmd/zenbot/host_supervisor.go:71-82` can retire the old host before replacement build/start fails. Async admission is reasonable, but operational failure needs an observable error path and recovery strategy.
- Shutdown scope: `cmd/zenbot/host_supervisor.go:93-104` stops only host; main awaits process context cancellation separately before process-owned teardown. Existing TestHostSupervisorShutdownStopsOnlyHostAndSignalTeardownIsIdempotent explicitly preserves this. Agent metadata says stop bot runtime. Clarify the contract rather than silently broadening shutdown authority.
- Prefix scope: `internal/core/prefix_state.go:3-5` explicitly limits updates to current replicas without persistence. Host and current-replica update locking is sound in the inspected implementation; subsequent replica or restarted-host prefixes can differ by intended construction behavior.
- Shared best-effort reply helper ignores delivery errors, affecting several audited commands. Root audit owns common capture/dispatch consequences; this report does not duplicate that broader finding.

## Extra route discovery

Whole-tree command construction/registration search found no additional implemented command identities beyond catalog and legacy unlock/unlockroom (assigned to another auditor). Legacy wrappers for existing identities do not add identities. Mine and whiskey remain catalog placeholders excluded from production registration and agent tool exposure. WhiskeyProxyOrder is unused. DBZ service methods for agility/vitality/energy exist, but no command route invokes them; they must not be counted as implemented commands.

## Verification performed

The following focused command passed across internal/command, internal/service, internal/core and cmd/zenbot; internal/service reported no matching tests:

```sh
go test ./internal/command ./internal/service ./internal/core ./cmd/zenbot -run '^(TestDBZ(RegisterAcknowledgesSuccessWhenPersistenceFails|StrengthReadErrorPublishesSourceEquivalentPublicNoFreeSuccess|StrengthAcknowledgesPublicAmountWhenWriteFails|FightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy)|TestSQLCommandParsesRawSourcePayloadOnly|TestHostLifecycle|TestHostSupervisor|TestUpdatePrefix)' -count=1
```

Passing tests confirm the documented existing compatibility behavior. Dedicated reproductions for newly identified lifecycle ownership/deadlock and missing-registration defects remain recommended follow-up work. This audit did not implement corrections.
