# Approved SQL / restart / shutdown — binding source-exact architecture and sequential TDD plan

**Status:** implementation-ready architecture only.  
**Authority:** the Saturn checkout is `/Users/ab/workspace/projects/saturn`; target citations are repository-relative to Zenbot. This document supersedes the prior deferral conclusions in `sql-current-architecture.md` and `restart-shutdown-current-architecture.md`: approval selects **source-exact observable behavior**, including the source's broad SQL and no-reply lifecycle semantics. It does not add a safety policy that Saturn does not have.

## 1. Binding parity contract

### Catalog, aliases, role, and authorization

| Canonical | Exact aliases | Catalog role | Saturn configured-trip bypass | Target binding |
|---|---|---:|---|---|
| `sql` | `sql` | ADMIN | `adminTrips` only | ADMIN role plus configured `AdminTrips` / persisted ADMIN authorization already used by target dispatch. |
| `restart` | `restart`, `reload`, `re` | ADMIN | `adminTrips` **or** `userTrips` | Preserve the source bypass: configured `UserTrips` may invoke this command despite the inherited ADMIN metadata. |
| `shutdown` | `exit`, `quit`, `shutdown` | ADMIN | `adminTrips` **or** `userTrips` | Preserve the same source bypass. |

**[OBSERVED — Saturn]** All three extend `UserCommandBaseImpl`, whose default `getAuthorizedRole()` is `ADMIN`. SQL passes `getAdminTrips(engine)`; restart and shutdown pass `getAdminAndUserTrips(engine)`. The base dispatch first resolves the command, then authorizes it and only then invokes it. `AuthorizationServiceImpl` permits either a command's configured trips (including literal `x`) or a persisted role at least the command role. Citations: `SqlUserCommandImpl`, `RestartCommandImpl`, `ShutdownCommandImpl`, `UserCommandBaseImpl.execute/getAuthorizedRole`, `Util.getAdminTrips/getAdminAndUserTrips`, and `AuthorizationServiceImpl.isUserAuthorized`.

**[OBSERVED — Zenbot]** `internal/command/registry.go:RegisterAll` already preserves all aliases and `model.ADMIN`. `internal/listener/message/handlers.go:DispatchUserCommand.Handle` resolves an active author and unconditionally calls `Engine.IsUserAuthorized(author, cmd.GetRole())` before execution; `internal/service/security_service.go:IsAuthorizedContext` supports configured `AdminTrips` (including `x`) and H2 authorization. It has no per-command configured-trip allowance and Zenbot configuration has no corresponding `UserTrips` command authorization setting. Therefore restart/shutdown's source `userTrips` bypass is a real missing target seam, not a reason to silently make the commands ADMIN-only.

**[BINDING]** Add an optional `common.CommandAuthorizer` contract (for example `Authorize(*model.User) bool`) and teach `DispatchUserCommand.Handle` to use it when implemented, otherwise retain its existing `Engine.IsUserAuthorized` path unchanged. Only restart/shutdown adapters implement it. Their predicate is `Engine.IsUserAuthorized(user, ADMIN) || configuredLifecycleUserTrips.Contains(user.Trip)`; it must recognize configured `x` exactly as the target admin predicate does. This is necessary because a handler-local check would be unreachable after the existing dispatcher rejects a source-authorized `userTrips` caller. SQL remains on the ordinary ADMIN dispatch path.

### Inbound parsing and whisper routing

* **[OBSERVED]** The target listener recognizes a prefix, obtains the alias with `strings.Fields`, and passes the original `model.ChatMessage` unchanged to the registered command (`internal/listener/message/handlers.go:DispatchUserCommand.Handle`; `internal/model/chat_message.go:GetArguments`). All aliases are case-insensitively registered/resolved by `common.BuildCommand`.
* **[BINDING]** SQL must read `message.Text`, not `args(message)` or a rejoined argument list. This preserves spaces, source text, and the source parser's dependence on the literal lowercase substring `sql `.
* **[BINDING]** SQL replies use the inbound whisper flag exactly as target `command.reply` does: `IsWhisper || Whisper || Type == "whisper"`. The normal target transport then addresses public output as `@author ...` and whispers as `/whisper @author .\n...` (`internal/core/engine_impl.go:SendChatMessage`).
* **[BINDING]** Restart and shutdown consume no arguments and send no acknowledgement, usage, success, or failure reply. Extra text is ignored. This is source behavior, not an omission.

### Command status/error and audit behavior

* **[OBSERVED — Saturn]** SQL has no validation. A raw-text parse failure can throw before it replies. SQL database exceptions are converted to a result string and sent; the command returns `SUCCESSFUL`. Restart/shutdown catch every `Exception`, log it, send no reply, and return `SUCCESSFUL`.
* **[OBSERVED — Zenbot]** `legacyAdapter` logs a returned command error and otherwise does not expose a status. It has no source-style command audit call (`internal/command/dispatch_adapter.go:legacyAdapter`; compare Saturn `UserCommandBaseImpl.execute` with target `repository.CommandAudit`).
* **[BINDING]** A controller/database failure that Saturn catches must be logged server-side and yield `model.SUCCESSFUL, nil` from restart/shutdown and a SQL reply with the raw driver error string for query errors. Do **not** add a command audit write, confirmation message, retry, or user-visible diagnostic: these would be new behavior.
* **[TARGET-ONLY CANCELLATION RULE]** Every concrete command checks `ctx.Err()` before any side effect and returns `FAILED, ctx.Err()` with no reply/call when cancelled. Saturn has no context. This is required by the target context-aware dispatch contract and is the only cancellation adaptation. SQL must pass the received context to the database without adding a deadline; lifecycle requests must not begin after cancellation.

## 2. SQL vertical — exact operational semantics

### Source behavior to reproduce

**[OBSERVED]** `SqlUserCommandImpl.execute` passes `chatMessage.getText()` to `SQLServiceImpl.executeSql(cmd, true)` and sends:

```text
Result: \\n<service result>
```

back to the author, preserving whisper status. `SQLServiceImpl.executeSql` calls Java `cmd.split("sql ")`, selects index 1, and replaces literal `\\n` with LF. With `withOutput=true`, it calls `Statement.executeQuery`; this is the only branch reachable from the chat command. It performs no statement classification, row/column/output limit, timeout, transaction policy, query audit, redaction, or blank-query check. `SQLException` in `executeFormatted` becomes `e.getMessage()`; it is sent as the result. Citations: Saturn `SqlUserCommandImpl.execute` and `SQLServiceImpl.executeSql/executeFormatted`.

Successful output is built as follows:

1. JDBC metadata `getColumnName(1..N)` supplies headers.
2. Every row and every column is read with `getString`; SQL NULL renders exactly `null`.
3. `TableGenerator.generateTable` emits its two-leading/newline padded, centered ASCII table.
4. `executeFormatted` wraps it in `\n```Text\n` and `\n ````, then applies `StringEscapeUtils.escapeJava`.

Citations: Saturn `SQLServiceImpl.executeFormatted`; `service/impl/util/TableGenerator.generateTable`.

**Meaning of approved arbitrary SQL.** The source chat route is arbitrary user-supplied SQL sent to a query-only JDBC API, not a typed report catalog and not a parser-enforced SELECT-only facility. The Go command must likewise issue the raw payload through `QueryContext`, without consulting `internal/agent/sql.Policy`, `JSqlParserAgentSqlPolicy`, a table allowlist, max rows, a read-only transaction, or a new statement timeout. `internal/agent/sql` is a distinct agent-tool policy whose explicit read-only/restricted behavior would be non-parity for this human ADMIN command.

### Exact target design

Add a narrow raw-query dependency; do not leak `*sql.DB` through `common.Engine`:

```go
// internal/service/sql_command.go (new)
type RawSQLQuery interface {
    Query(context.Context, string) (SQLTable, error)
}
type SQLTable struct { Columns []string; Rows [][]string }
```

`service.RawSQLService` owns the existing application `*sql.DB` supplied by `h2.Database.SQLDB()` and uses `db.QueryContext(ctx, rawSQL)`. It must:

* obtain column labels from `rows.Columns()`;
* scan each row as `any`, then render `nil` as literal `null` and other values with the closest driver/JDBC string representation established by the real-H2 parity tests;
* drain all rows, return a database error verbatim as the service result path, and close `Rows`;
* add no max rows, byte cap, deadline, validation, redaction, transaction wrapper, or query audit.

`internal/command/sql.go` (new) owns only source parsing, exact rendering, and reply. It receives the dependency through `service.Bundle` (add `SQLCommand RawSQLQuery` or an equivalently named dedicated field) rather than type-asserting a database in command code. `factory.NewEngineWithOptions` creates it only when `repo.SQLDB()` is non-nil, alongside existing DB-backed services. `newCommand("sql", ...)` returns `*sqlCommand`.

**Raw parser binding.** Implement source-equivalent literal splitting rather than `strings.Fields`:

```text
raw := message.Text
parts := strings.Split(raw, "sql ")
query := strings.ReplaceAll(parts[1], `\n`, "\n")
```

The implementation must deliberately document/cover these quirks:

* it is case-sensitive at the payload split even though target alias lookup is case-insensitive (`!SQL SELECT 1` resolves the alias then has no reply/result);
* no separator/payload produces no reply and a logged command failure rather than a panic that can kill the Go listener goroutine;
* Java `String.split` is regex splitting with trailing empty parts removed. A final empty payload therefore also fails before a reply; reproduce the same externally visible no-reply behavior;
* the first text after the first matching `sql ` is used; anything before it and any subsequent `sql ` separator follows Java split-array semantics, rather than a cleaned/rejoined query.

The no-reply parser-error adaptation is necessary because target commands return errors rather than throwing unchecked exceptions; it is externally equivalent to the Saturn handler's lack of reply. It must be an internal logged error, not a new user message.

### SQL output compatibility seam

Create a dedicated renderer in `internal/command/sql.go` or `internal/command/sql_table.go` (new). It must be tested against fixed golden strings and implement Saturn's `TableGenerator`, not a generic Go table library:

* ASCII `+`, `-`, `|`; padding size 2; each column maximum starts with its header, includes all cell widths, and is rounded up to an even width.
* Two leading LFs before the first border; centered cells with the source's odd-length extra trailing space; a border after headers and once at end; row-height 1.
* On a successful table, wrap/escape exactly as Saturn does: the literal prefix `\n```Text\n`, then the table, then the literal suffix produced by `string.append("\n ```")` in Saturn (one space plus three closing backticks); apply Java-style escaping of backslash, LF, CR, tab, quote, control characters, and relevant Unicode escapes.

The final-code-fence spelling must be fixed from a direct Saturn golden-vector run/probe before implementation acceptance; do not guess from markdown rendering. This is a test-data prerequisite, not a product decision. Java calculates widths with UTF-16 `String.length`, while Go's `len`, rune count, and terminal display widths differ for non-BMP Unicode. Implement Java UTF-16-code-unit width for source parity and include BMP/non-BMP golden vectors.

### SQL registration and failure rules

Register `sql` only if all are true:

1. `service.Bundle.SQLCommand` is non-nil;
2. the engine has normal target authorization; and
3. the concrete definition is selected (never `*saturnCommand`).

It must remain unavailable for engine stubs/ZOMBIE configurations without the raw-query service. This is a dependency gate, not a new safety policy. SQL's only alias is `sql`; registration covers it exactly.

| Event | Required result |
|---|---|
| Unknown/inactive/unauthorized caller | Existing dispatcher denies before parser/query; target denial text/routing remains unchanged; no DB call. |
| Cancelled context | `FAILED, context.Canceled`; no DB call/reply. |
| Raw parser defect/blank payload | Logged error; no reply; no DB call. |
| Query succeeds (including zero rows) | `SUCCESSFUL`; one `Result: \\n` reply with escaped formatted table; preserve whisper. |
| Database returns an error | `SUCCESSFUL`; one `Result: \\n` reply concatenated with the raw driver error string; preserve whisper. |
| Chat delivery fails | Preserve existing `reply` best-effort behavior: status remains successful and delivery error is discarded. |
| Context cancels during query/row scan | Return `FAILED` with cancellation and no result reply. No synthesized timeout/error text. |

## 3. Lifecycle vertical — exact ownership and ordering

### Source behavior to reproduce

**Restart.** Saturn `RestartCommandImpl` calls `ApplicationLifecycle.getInstance().restartHost()`, catches every exception, logs, sends no reply, and returns success. `ApplicationLifecycle` delegates to its bound `ApplicationRunner`. Under its lifecycle lock, `restartHostNow` stops the old host, constructs a fresh host `EngineImpl`, and starts it. Stopping calls `host.stop()`, clears its host reference, waits one second, clears the static host, waits another second, and requests GC. The source code does not stop/recreate replicas on this path despite its log wording. Citations: Saturn `RestartCommandImpl.execute`, `ApplicationLifecycle.restartHost`, `ApplicationRunner.restartHostNow/restartHost/stopHostIfRunning/createHostEngine`.

**Shutdown.** Saturn `ShutdownCommandImpl` calls `ApplicationLifecycle.shutdown()`, catches every exception, sends no reply, and returns success. `ApplicationRunner.stopApplication` shuts down the health scheduler, waits up to 10 seconds (force-shutting it on timeout/interruption), then takes the lifecycle lock and stops the host by the same host-stop path. It neither calls `System.exit` nor closes the database in this path. The later JVM shutdown hook separately invokes `stopApplication`. Citations: Saturn `ShutdownCommandImpl.execute`, `ApplicationLifecycle.shutdown`, `ApplicationRunner.stopApplication/stopBot/shutdownHook`.

### Required target composition prerequisite

Zenbot cannot safely implement either command by calling `core.Lifecycle.Restart` or `common.Engine.Stop`:

* `cmd/zenbot/main.go` exclusively owns the signal context, H2 lifetime, room agent, `ReplicaManager`, master engine, and local `*core.Lifecycle`.
* `core.Lifecycle` is a one-engine transport manager. Main's factory returns a wrapper around the same `*core.EngineImpl`; `Restart` is `Stop` then `Start`.
* `EngineImpl.StartContext` sends join only once (`joined.Swap(true)`); `StopContext` does not reset it. Same-engine restart can reconnect without a new join.
* `ReplicaManager.StopAll` is terminal and rejects later additions; `roomAgent.Close` is terminal; neither is part of `Lifecycle.Restart`.

Citations: `cmd/zenbot/main.go:main`, `internal/core/lifecycle.go:Restart`, `internal/core/engine_impl.go:StartContext/StopContext`, `internal/core/replica_manager.go:StopAll`, and `internal/agent/runtime/runtime.go:Close`.

Therefore **composition must precede command registration**. Extract main's currently inline master construction and orderly signal teardown into a process-owned supervisor. Do not install lifecycle ownership on `EngineImpl`, do not type-assert in command code, and do not use `os.Exit`, self-signals, or `exec`.

```go
// internal/common/host_lifecycle.go (new)
type HostLifecycleController interface {
    RequestRestart(context.Context) error
    RequestShutdown(context.Context) error
}
```

`internal/core/host_lifecycle.go` (new) owns a serialized request queue/state machine, but receives all process operations as injected callbacks. `cmd/zenbot/main.go` owns the concrete supervisor and supplies the callbacks. The controller must expose no raw process/database/agent handles to `internal/command`.

### Binding target lifecycle sequence

**Request transport.** An inbound command runs on the current engine's receive/listener goroutine. Calling `StopContext` synchronously from it can wait for its own receiver goroutine. The controller must therefore accept a request synchronously, arrange execution after dispatch returns, and make only one lifecycle operation active at a time. The command still returns `SUCCESSFUL` with no reply, matching Saturn's observable chat contract. This asynchronous handoff is a necessary Go topology adaptation; it must be verified with a no-self-deadlock tracer.

**Restart sequence (binding target equivalent).**

1. Authorize with the source-shaped configured-trip/role policy; command has no parse/reply work.
2. Queue one restart request. If a lifecycle operation is already active, coalesce duplicate restart requests; a queued/active shutdown supersedes a restart.
3. After the dispatch callback is clear, stop only the current master host transport with a bounded background stop context, retire the old master, and construct/start a **fresh master graph** through an extracted factory. A fresh master necessarily gets a fresh transport and a fresh join state.
4. Preserve H2/database ownership and do **not** call `ReplicaManager.StopAll`, `roomAgent.Close`, temporary-session shutdown, or H2 close as part of restart. This is the direct source mapping: source `restartHost` replaces only host.
5. Rebind the supervisor's current-master reply/direct-command references to the newly constructed master. Existing target room-agent/replica dependencies that retain the retired master are a composition defect; the extracted graph factory must either recreate/rebind them explicitly or prevent their use. No source-compatible restart can leave them targeting a stopped master.
6. Log internal completion/failure only. Failure does not send chat output or reverse a completed stop.

**Shutdown sequence (binding target equivalent).**

1. Authorize and queue shutdown as above. It is terminal for the supervisor instance and cancels/rejects subsequent restart requests.
2. Stop lifecycle health/host transport with a bounded background context; do **not** stop replicas, agent runtime, temporary sessions, H2, or DB as part of the command action.
3. Keep the Go process and H2/database alive, quiescent with no host, just as Saturn's `stopApplication` does not call `System.exit` or close its database.
4. The existing OS-signal shutdown path remains a separate terminal process teardown and may close room agent, replicas, DB, and H2 in its current order. It must be idempotent if invoked after command shutdown.

The source never specifies caller feedback, retry, or an OS exit. The above deliberately does not invent them.

### Genuine source/target incompatibility

There is one material target seam with no present implementation: Zenbot's room-agent, snapshot reply sink, replica controller, and direct submission wiring capture the original master pointer in `cmd/zenbot/main.go`. Saturn can create a fresh host because its runner owns construction; target main does not yet have a replaceable graph factory or rebinding protocol. This is resolvable in implementation by extracting/rebinding the target composition as specified above; **it does not require a product-policy decision**.

No user decision remains for SQL scope, restart meaning, shutdown process termination, caller acknowledgement, or writes: source-exact approval selects raw query-capable SQL, fresh-host restart, in-process non-exiting shutdown, and no command reply. If an owner instead requires a full-graph restart, replica shutdown, SQL restrictions, or termination, that is intentionally a different product behavior and must be authorized separately.

### Lifecycle registration/failure rules

Register `restart` and `shutdown` only when the engine exposes an installed `common.HostLifecycleController`; register neither on an ordinary engine, replica, agent, ZOMBIE engine, test stub without the fake controller, or agent command gateway. Add neither alias to `internal/command/agent_gateway.go:publicAgentCommandAliases`.

| Event | Required result |
|---|---|
| Missing controller | Commands are absent; no side effect. |
| Unauthorized caller | Existing target denial before command execution; controller not called. |
| Cancelled context | `FAILED, context.Canceled`; controller not called; no reply. |
| Valid restart/shutdown request | `SUCCESSFUL, nil`; no reply; exactly one queued controller request. |
| Controller returns/surfaces an operational failure | Log it; command's observable result remains success/no reply, matching Saturn catch-all behavior. |
| Repeated restart | One active/queued restart; coalesced duplicates create no extra host. |
| Shutdown during restart | Shutdown wins; no fresh host may begin after terminal shutdown state. |
| Restart after shutdown | Rejected/coalesced internally; command still reports source-style success/no reply. |
| OS signal after command shutdown | Idempotent process teardown; no panic/double close. |

## 4. Exact file ownership

| Path | Change / responsibility |
|---|---|
| `internal/common/host_lifecycle.go` **new** | Narrow command-facing request interface only. |
| `internal/core/host_lifecycle.go` **new** + focused tests | Serialized/coalescing controller state machine; callback injection; no command or `os` dependencies. |
| `cmd/zenbot/main.go` | Extract build/start/retire/rebind supervisor composition, install controller before utility registration, retain signal teardown ownership. |
| `cmd/zenbot/*lifecycle*_test.go` **new** | Fake-driven main composition/order tests; no process kill or production endpoint. |
| `internal/common/command.go` | Add optional `CommandAuthorizer` interface; no change to the ordinary `Command` contract. |
| `internal/listener/message/handlers.go` | Honor `CommandAuthorizer` for restart/shutdown before the existing role check; preserve current authorization behavior for every other command. |
| `internal/config/config.go` and config loader **only if needed** | Add source-shaped configured `UserTrips` to support restart/shutdown authorization bypass; do not repurpose H2 auto-move user trips. |
| `internal/service/security_service.go` | Provide a narrow configured lifecycle-trip predicate while retaining persisted ADMIN-role acceptance and global role ordering. |
| `internal/command/dispatch_adapter.go` | Capability-gate `sql`, `restart`, `shutdown`; use definitions only after their concrete dependencies exist. |
| `internal/command/handlers.go` | Add `sqlCommand`, `restartCommand`, `shutdownCommand` cases to `newCommand`; remove these three from generic `saturnCommand` no-op branch. |
| `internal/command/sql.go`, `internal/command/sql_table.go` **new** | Raw source parser, Saturn renderer/escaping, response. |
| `internal/command/restart_shutdown.go` **new** | Parse-free no-reply lifecycle handlers; controller invocation/target cancellation adaptation. |
| `internal/service/sql_command.go` **new** | Dedicated raw `QueryContext` adapter and table data contract. |
| `internal/service/services.go` | Add the narrow SQL command dependency to `service.Bundle`. |
| `internal/factory/engine_factory.go` | Wire raw-query service from existing `SQLDB()` only; no additional DB exposure. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Remove only these three names from `allowedScopedGenericFallbacks` after their concrete tests are green. |
| `internal/command/help.go` | No text change required for source catalog parity; add availability behavior tests so help advertising does not imply registration. |

**No unrelated scope:** do not alter H2 schema/server configuration, agent SQL policy, generic agent command grants, role ordering, replica semantics outside required rebinding, snapshot protocol, other command catalog rows, Saturn sources, or existing dirty changes.

## 5. Strict sequential TDD tracer plan

Each numbered item is a complete vertical tracer. Do not write the next test or production code before the current test has been observed RED, then made minimally GREEN, then refactored with the relevant package tests green. No real shutdown, self-signal, `os.Exit`, or external restart may occur in tests.

### Prerequisite composition verticals

1. **RED — source authorization adapter.** Add `internal/service/security_service_test.go` and `internal/listener/message/dispatch_authorization_test.go`: SQL accepts target existing ADMIN authorization; restart/shutdown accept configured `UserTrips`, `AdminTrips`, literal `x`, and persisted ADMIN role; an ordinary USER remains denied. Prove the optional command authorizer is used only for lifecycle commands and existing commands retain the ordinary dispatcher role check. Confirm failure because neither the narrow source-shaped predicate nor dispatcher extension exists.  
   **GREEN:** add the smallest optional command-authorizer/config plumbing; do not change `IsAuthorized` for unrelated commands. Run `go test ./internal/service ./internal/listener/message -run 'Test.*Lifecycle.*Authorization|Test.*SQL.*Authorization|TestDispatch.*Authorization' -count=1`.
2. **RED — lifecycle controller serialization.** In `internal/core/host_lifecycle_test.go`, with recording callbacks/channels, prove one restart request runs only after the submitter releases dispatch; two restarts coalesce; shutdown supersedes restart; post-shutdown restart cannot build a host; cancelled request is not admitted.  
   **GREEN:** controller queue/state only. Run `go test ./internal/core -run TestHostLifecycle -count=1` then `go test -race ./internal/core -run TestHostLifecycle -count=1`.
3. **RED — replaceable production composition.** In `cmd/zenbot` tests, fake the extracted graph factory and assert restart retires old master then constructs/starts a fresh master, rebinds command/reply dependencies, and does not stop agent/replicas/H2. Assert shutdown stops host only, remains process-alive, and later signal teardown is idempotent.  
   **GREEN:** extract main-owned supervisor/factory minimally. This vertical must prove a new join-capable engine rather than treating `core.Lifecycle.Restart` as parity. Run `go test ./cmd/zenbot -run TestHostSupervisor -count=1`.

### Lifecycle command verticals

4. **RED — capability-gated catalog/aliases.** Add `internal/command/restart_shutdown_test.go`: engines without a controller expose none of six aliases; recording controller engines expose exact aliases/ADMIN role and concrete command types; catalog guard fails until the generic allowlist is reduced.  
   **GREEN:** gated registration plus concrete `newCommand` routes. Run `go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1`.
5. **RED — inbound behavior.** Dispatch each alias through `listener.NewUserChatListener` with an active authorized caller. Assert one controller request, zero chats, and ignored arguments. Dispatch an unauthorized active caller and assert target denial plus zero controller calls. Use a cancelled context direct execution test for no call/no reply.  
   **GREEN:** tiny parse-free handlers. Run command and listener focused tests.
6. **RED — source catch-all and no-self-deadlock.** Make controller callbacks fail and assert handler status remains successful/no reply while failure is recorded; use a channel-backed fake receiver to prove request execution waits until dispatch release.  
   **GREEN:** log-only failure bridge and post-dispatch scheduling; run focused race tests again.

### SQL command verticals

7. **RED — raw-query service and registration gate.** Add `internal/service/sql_command_test.go` against `h2fixture.Open`: a service with raw DB executes a query; a bundle/engine without it does not register `sql`. Do not use `internal/agent/sql`.  
   **GREEN:** narrow service bundle/factory wiring and registration guard. Run service/factory/command focused tests.
8. **RED — exact raw parser.** `internal/command/sql_test.go` first asserts `!sql SELECT 1` passes exact raw SQL; literal `\\n` becomes LF; `!SQL SELECT 1`, `!sql`, and `!sql ` create no reply/DB call and return an internal error.  
   **GREEN:** source-shaped parser only. Run its test alone to witness RED then GREEN.
9. **RED — real H2 reply/output.** Seed a real H2 fixture and dispatch public and whisper `!sql SELECT ...` through the real listener. Assert exactly one addressed/whispered reply, exact `Result: \\n` prefix, NULL text, column names, zero-row table, and golden ASCII/Java-escaped formatting. Add non-BMP width and control/quote/backslash vectors.  
   **GREEN:** dedicated Saturn renderer and reply implementation; no limits/validation.
10. **RED — error/cancellation parity.** Query malformed/driver-rejected SQL and assert one successful-status raw-error reply; use pre-cancelled context and assert no query/reply; induce chat-send failure and assert successful status remains. Read back database state after rejected query only to document the source `QueryContext` API path—not to add a write policy.  
    **GREEN:** error branches only. Do not turn errors into safe generic text.
11. **RED — catalog finalization/integration.** Assert `sql`, all restart aliases, and all shutdown aliases are concrete, dependency-gated, unauthorized-before-side-effect, absent from agent gateway, and absent when capability is missing.  
    **GREEN:** remove exactly `sql`, `restart`, and `shutdown` from `allowedScopedGenericFallbacks`; no other catalog change.

### Required verification after the last GREEN

Run, retaining actual output in the implementation handoff:

```text
gofmt -w <only files changed by the implementation>
go test -count=1 ./internal/service ./internal/core ./internal/command ./internal/listener/message ./cmd/zenbot
go test -race -count=1 ./internal/core ./internal/command ./cmd/zenbot
go test -count=1 ./...
go build ./...
git diff --check
git status --short
```

`go vet ./...` is also required as a reportable baseline gate; if it fails on unrelated dirty work, record the exact output and do not attribute it to this vertical. Verify every repository-relative citation above resolves, all three generic fallbacks are removed only after concrete behavior tests exist, the final diff has no application changes outside the file map, and no tests invoked a real process lifecycle action.

## 6. Baseline and QA record

**[OBSERVED]** This task began in an already dirty repository at commit `f9079ca feat: advance Saturn parity migration`; `git diff --check` returned success before this handoff. Existing deferred handoffs and the current roadmap were read, along with full Saturn command/service/lifecycle authorities and target catalog, dispatch, H2, factory, main, engine, lifecycle, replica, agent-runtime, and focused-test paths.

**[QA checklist for this architecture]** Citations were verified against the current checkouts; the document creates one new handoff only and does not modify Go source/tests, stage, reset, clean, commit, execute SQL, restart, shut down, or signal a process.
