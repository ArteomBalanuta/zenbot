# Admin SQL parity: current architecture and decision gate

## Verdict

**Do not implement or register `sql` without an explicit product/security policy decision.** The source command is an administrator-gated, raw-SQL data-exfiltration and database-introspection capability, not a normal parameterized repository operation. The target already makes its live `*sql.DB` reachable from composition, so implementing it mechanically is technically easy but would expose a broad production database capability to chat.

The required decision is not merely whether an administrator role is sufficient. It must select the supported SQL language and operational controls: (a) no command, (b) a fixed typed/report-query catalog, (c) read-only ad-hoc SQL, or (d) source-compatible ad-hoc SQL. Each option has different safety, data-disclosure, auditing, output-limiting, cancellation, and operational-owner implications. Until selected, the capability gate must keep every `sql` alias unregistered.

## [OBSERVED] Saturn behavior

### Catalog, role, aliases, and dispatch

* `org/saturn/app/command/impl/admin/SqlUserCommandImpl.java` is annotated `@CommandAliases(aliases = {"sql"})`: canonical observed alias set is **only `sql`**.
* `SqlUserCommandImpl(EngineImpl, ChatMessage, List<String>)` calls `super(message, engine, getAdminTrips(engine))`. `Util.getAdminTrips(EngineImpl)` splits the configured comma-separated `engine.adminTrips` value (`org/saturn/app/util/Util.java`, `getAdminTrips`).
* It does not override `UserCommandBaseImpl.getAuthorizedRole`; the base returns `Role.ADMIN` (`UserCommandBaseImpl.getAuthorizedRole`).
* The source dispatcher is `UserCommandBaseImpl.execute`: it asks the command factory for the alias, calls `AuthorizationServiceImpl.isUserAuthorized`, executes only when authorized, then audits command name, parsed arguments, status, channel, and timestamp through `engine.logRepository.logCommand` (`UserCommandBaseImpl.execute`).
* `AuthorizationServiceImpl.isUserAuthorized` accepts either configured command trips (including literal `x`) or a database role at least the command role; on denial it replies `msg mercury for access.` using the inbound whisper flag (`AuthorizationServiceImpl.isAllowedByApplicationConfig`, `isAllowedByDb`, `isUserAuthorized`).

### Parsing and query execution

* `SqlUserCommandImpl.execute` **does not use the base parsed argument list**. It passes the raw `chatMessage.getText()` to `engine.sqlService.executeSql(cmd, true)`.
* `SQLServiceImpl.executeSql` uses `cmd.split("sql ")`, selects element `[1]`, and converts literal `\\n` sequences into newline characters. Thus its observed grammar relies on the lowercase substring `sql ` in the raw command text. A missing separator/argument causes an unchecked array-index failure before a reply; there is no usage response or blank-query validation in this command/service path.
* The command always passes `withOutput=true`. The only reachable branch calls `SQLServiceImpl.executeFormatted`, which creates a plain JDBC `Statement` and calls `executeQuery(sql)`. The `executeUpdate` branch exists only for other callers of `executeSql(..., false)` and is not reached by `SqlUserCommandImpl`.
* `executeFormatted` reads all rows and all columns without a command-level limit, obtains `ResultSetMetaData` column names, maps SQL `NULL` to text `null`, renders a `TableGenerator` table inside `\n```Text\n...\n ````, and applies `StringEscapeUtils.escapeJava` to successful output.
* SQL exceptions are caught in `SQLServiceImpl.executeFormatted` and returned as the raw database exception message. They are then delivered as a normal successful command reply; the handler returns `Status.SUCCESSFUL` irrespective of the service string. There is no timeout, row cap, output-byte cap, statement allowlist, transaction/read-only setting, result redaction, or audit of raw SQL text demonstrated by this path.

### Delivery and effective database privilege

* The exact reply is `Result: \\n` concatenated with the service result, sent to `chatMessage.getNick()` and preserving `isWhisper()` (`SqlUserCommandImpl.execute`).
* `Base` constructs both `SQLServiceImpl` and `AuthorizationServiceImpl` from the same JDBC `Connection` (`org/saturn/app/facade/Base`, constructor). Therefore the command uses that application connection's database permissions, not a command-specific limited principal.
* Source handling is not safely describable as a strict “SELECT-only” feature. Its reachable JDBC API is `Statement.executeQuery`, but no lexical/parser allowlist exists and the service accepts raw SQL as the intended payload. SQL injection prevention is inapplicable to this ad-hoc interface: user-supplied SQL is deliberately executable. Whether the driver/database accepts side-effecting query forms is database-specific and must not be treated as a security boundary.

## [OBSERVED] Zenbot target mapping

### Existing catalog and dispatch

* The target catalog already declares `def("sql", []string{"sql"}, model.ADMIN)` in `internal/command/registry.go`, `RegisterAll`, so alias/role metadata matches source.
* `newCommand` has no `sql` case. It falls through to placeholder `saturnCommand.Execute`; the `case "access", "memory", "mine", "prefix", "restart", "shutdown", "sql":` branch does nothing and returns success (`internal/command/registry.go`, `newCommand`, `saturnCommand.Execute`).
* Crucially, `RegisterUserUtilitiesWithDirectAgent` registers a curated concrete list only (`internal/command/dispatch_adapter.go`, `RegisterUserUtilitiesWithDirectAgent`). `sql` is not in that list, hence it is not publicly registered today. This is an effective no-capability gate and must remain so until policy and implementation exist.
* Incoming chat follows `listener.NewUserChatListener` → default message chain → `message.DispatchUserCommand.Handle` (`internal/listener/user_chat_listener.go`, `NewUserChatListener`; `internal/listener/message/handlers.go`, `DefaultChain`, `DispatchUserCommand.Handle`). It trims the prefix, uses `strings.Fields`, resolves aliases through `common.BuildCommand`, and requires a resolved active `Author` plus `Engine.IsUserAuthorized` before `cmd.Execute`.
* Target denial sends `" you are not authorized to run: %s command."` to the author using `Message.IsWhisper`; `DispatchUserCommand.Handle` does not execute the command after denial. `internal/listener/message/dispatch_authorization_test.go`, `TestDispatchUserCommandRejectsUnauthorizedPrincipal`, test-backs this dispatch gate.
* `core.EngineImpl.IsUserAuthorized` delegates to `service.SecurityService.IsAuthorized`; persisted H2 authorization is used when the repository supports `repository.AuthorizationRepository` (`internal/core/engine_impl.go`, `IsUserAuthorized`; `internal/factory/engine_factory.go`, `NewEngineWithOptions`; `internal/service/security_service.go`, `IsAuthorizedContext`). `model.ADMIN` is the strongest target role (`internal/model/role.go`, `Role`).

### Database access, output, and test seam

* `h2.Database.SQLDB() *sql.DB` exposes the raw database handle (`internal/repository/h2/audit.go`, `SQLDB`). `factory.NewEngineWithOptions` detects that method and constructs services with the same `*sql.DB` (`internal/factory/engine_factory.go`, lines 73–90). Existing service code uses parameterized SQL for fixed operations, such as `MailService.QueueResolved` and `NoteService.Save` (`internal/service/services.go`); it does not provide an ad-hoc SQL abstraction.
* H2 runs a local PostgreSQL-wire server and opens the `sa` application connection; `h2.Open` then bootstraps the full schema (`internal/repository/h2/database.go`, `Open`, `bootstrap`). The schema contains chat messages, identities/roles, notes, mail, command audit, agent memory, and agent tool memory (`internal/repository/h2/schema-h2.sql`). An admin raw-query reply could disclose all of these to a chat recipient.
* `command.reply` calls `Engine.SendChatMessage` and intentionally discards delivery errors (`internal/command/handlers.go`, `reply`). The underlying engine returns a delivery error and formats public replies as `@author ...`, whispers as `/whisper @author .\n...` (`internal/core/engine_impl.go`, `SendChatMessage`). A SQL implementation must choose deliberately whether source-like “best-effort reply, success” semantics or target-style propagated delivery failure are desired; this is also policy-sensitive.
* `internal/testutil/h2fixture.Open` is a real, isolated H2 PostgreSQL-wire fixture with an owned temporary server and cleanup. `internal/command/last_online_test.go`, `TestLastOnlineAliasesDispatchAgainstRealH2AndRequireQueries`, demonstrates the intended command test pattern: prove aliases absent without the backing capability, use real H2 with seeded rows, then dispatch JSON through `listener.NewUserChatListener` and assert the chat reply.

## Security consequence and mandatory policy questions

Ad-hoc SQL cannot be made safe by string escaping, placeholders, or a role check alone: the SQL itself is the requested program. The raw H2 connection can inspect sensitive tables and metadata; unbounded query results can leak data or create oversized outbound messages; expensive expressions can consume the shared database; engine/driver-specific query forms may exceed a presumed read-only intent. Source behavior returns database errors to chat, which can reveal schema and implementation details.

Before implementation, product/security must answer all of the following explicitly:

1. Is this parity feature intentionally supported at all, and in which deployments/environments?
2. Is parity defined as a curated report/query catalog, arbitrary read-only ad-hoc SQL, or source-compatible raw SQL? “Read-only” must define allowed statement grammar and whether CTEs, `CALL`, metadata, multi-statements, or database functions are permitted.
3. What data classes may be returned? In particular, may messages, private/whisper records, trip identities, authorization state, mail, agent memory, and tool evidence be exposed to a chat administrator?
4. What controls apply: separate read-only DB principal/connection, transaction read-only enforcement, SQL parser/allowlist, statement deadline, row/column/output-byte caps, redaction, rate limit/concurrency limit, and audit retention/redaction for submitted SQL?
5. What delivery contract applies for public vs whisper use, database/format errors, partial results, and failed chat sends?

Until those decisions are recorded, there is no safe default mapping from the source behavior to Zenbot.

## [RECOMMENDED] capability gate and implementation boundary after approval

Keep `sql` out of `RegisterUserUtilitiesWithDirectAgent` by default. Do **not** make registration depend merely on `bundle(e).DB` or `SQLDB()`: those are broad raw-handle affordances and would conflate database presence with an approved privileged-query policy.

After an approved policy, introduce a narrow explicit capability, for example a command-local `SQLQueryCapability` supplied only by the composition root after it has selected the policy and database principal. It should receive a `context.Context`, expose a structured query/result contract rather than `*sql.DB`, and make the chosen constraints testable. The registration condition should require both that capability and the existing command role. The existing listener authorization remains the first gate; the capability must be the second gate preventing accidental registration in tests, ZOMBIE engines, and configurations without the approved SQL policy.

If policy chooses fixed reports, add typed repository methods instead of any raw SQL input. If it chooses ad-hoc reads, the capability needs a real parser/allowlist plus DB-side read-only protection; a simple `strings.HasPrefix(strings.ToUpper(sql), "SELECT")` check is not sufficient. Source-compatible arbitrary SQL should be treated as a high-risk, separately approved operational console feature rather than normal chat-command parity.

## First RED test after a policy is chosen

Create `internal/command/sql_test.go` with a real-H2 dispatch test patterned after `TestLastOnlineAliasesDispatchAgainstRealH2AndRequireQueries`:

1. Build an engine without the new capability; call `RegisterUserUtilities`; assert `sql` is absent.
2. Build an engine with the explicitly approved capability, an active authorized ADMIN author, and `h2fixture.Open`; seed only policy-approved fixture data.
3. Dispatch `!sql <approved query>` via `listener.NewUserChatListener`; assert exactly the approved result format and correct whisper/public delivery.
4. Add RED cases for a denied author, blank/malformed request, non-approved/write statement, sensitive-table/redacted result, database error disclosure policy, row/output cap, cancellation/timeout, and a failed `SendChatMessage` if errors are to propagate.
5. Read back the real H2 state after every rejected query to prove it had no side effect. If policy permits writes, use an isolated designated test table and require explicit expected state transition and audit assertions instead.

## Precise target file map and scope controls

| Area | Current verified symbol/path | Likely post-decision ownership |
|---|---|---|
| Catalog metadata | `internal/command/registry.go`: `RegisterAll`, `newCommand` | Replace only the `sql` placeholder with a concrete command after a decision. |
| Public capability registration | `internal/command/dispatch_adapter.go`: `RegisterUserUtilitiesWithDirectAgent` | Add `sql` only behind a narrow explicit capability. |
| Command parsing/reply | `internal/command/handlers.go`: `args`, `reply`; new `internal/command/sql.go` | Define approved parser, output, error, and delivery semantics. |
| Dispatch authorization | `internal/listener/message/handlers.go`: `DispatchUserCommand.Handle` | Existing ADMIN role gate; no broad dispatch rewrite. |
| Authorization wiring | `internal/factory/engine_factory.go`, `internal/service/security_service.go` | Reuse existing auth only; do not weaken role ordering. |
| Raw H2 facility | `internal/repository/h2/audit.go`: `SQLDB`; `internal/repository/h2/database.go`: `Open` | Do not expose more raw DB plumbing; inject a policy-specific boundary. |
| Real database test fixture | `internal/testutil/h2fixture/h2fixture.go`: `Open` | Use for integration assertions and read-back. |
| Regression tests | `internal/command/last_online_test.go`, `internal/command/dispatch_integration_test.go`, `internal/listener/message/dispatch_authorization_test.go` | Add focused SQL tests only after the policy contract exists. |

### Risks

* **Critical — data disclosure:** the current H2 schema contains private operational and agent data; raw results are delivered through chat.
* **Critical — authorization is not containment:** ADMIN authorization controls caller identity, not database privileges, exposure scope, output volume, or cost.
* **High — semantic ambiguity:** source uses `executeQuery` with raw SQL and no allowlist; calling it “read-only” would be an unsupported conclusion.
* **High — reliability:** no source row/output/time limits; the target's reply helper swallows delivery errors.
* **Medium — fidelity ambiguity:** source parses raw text with `split("sql ")`, while target command utilities normally parse `ChatMessage.GetArguments`; preserving the source bug versus adopting explicit validation requires a product choice.

### No-scope list for this documentation-only candidate

* Do not implement or register `sql`.
* Do not add a generic raw-SQL repository/API or expose `*sql.DB` beyond its existing factory use.
* Do not alter Saturn/Zenbot role semantics, existing listener dispatch, output escaping, H2 schema, connection configuration, or authorization persistence.
* Do not work on `mine`, `restart`, or `shutdown` (the latter two remain separately deferred pending product decision).
* Do not reset, clean, checkout, stage, or commit the dirty repository.

## Verification

[TEST-BACKED] Required read-only baseline passed from `/Users/ab/workspace/go-projects/zenbot`:

```text
ok  zenbot/internal/command        9.761s
ok  zenbot/internal/repository/h2  37.047s
ok  zenbot/internal/service        2.863s
```

`git diff --check` ran after the tests in the required chained command and returned success. The pre-existing worktree was dirty before this documentation task; this handoff is the only file created by this task.
