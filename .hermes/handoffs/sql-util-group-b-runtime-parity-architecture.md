# SQL Utility Group B Runtime-Parity Architecture

## Scope, decision, and status

This document is an architecture handoff for the smallest Zenbot runtime wiring needed to expose the accepted SQL utility Group B behavior with Saturn-compatible command semantics. It is a design document, not an implementation record; implementation must not begin from the previous, failed version of this handoff.

- **Decision:** proceed with Saturn-parity runtime wiring.
- **Constraints:** do not over-engineer; the user approved parity functionality and stated that security is not a blocker for this slice. Preserve the existing authorization boundary with the smallest executable bridge rather than adding a security redesign.
- **Current QA status:** the independent report in `.hermes/handoffs/sql-util-group-b-runtime-parity-architecture-qa.md` is FAIL for the prior handoff. This revision addresses each cited execution blocker.
- **Protected documents:** `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` remain unchanged.
- **Implementation boundary:** do not modify application source as part of this handoff task. The paths below are targets for a later implementation only.

## Observed current state

### Accepted Group B repository seam

**[OBSERVED]** The accepted interface is `repository.SqlUtilGroupBRepository` in `internal/repository/sql_util_group_b.go`:

- `DeleteIdentity(context.Context, string, string) (repository.DeleteResult, error)`
- `SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error)`
- `SaturnLastMessages(context.Context, *string, string, int) ([]repository.SaturnLastMessage, error)`

**[OBSERVED]** `internal/repository/h2/sql_util_group_b.go` implements that interface. Its Saturn-shaped reads use the accepted query semantics. `SaturnLastMessages` defaults non-positive counts to 5, accepts nullable `name`, excludes `LEFT`/`JOINED`, and orders newest first. `SaturnRegisteredUsers` returns name/trip rows ordered by trip descending.

**[TEST-BACKED]** `internal/repository/h2/sql_util_row324_group_b_test.go` covers Group B behavior, including deletion row counts, SQL-injection-shaped inputs, nullable last-message input, count defaults, and the authorization requirement.

### Existing service and factory wiring

**[OBSERVED]** The prior handoff incorrectly presented construction and injection as missing work. They already exist:

- `internal/service/services.go`: `service.Bundle`, `service.UserService.GroupB`, `service.MailService.GroupB`, `UserService.SaturnLastMessages`, and `MailService.SaturnRegisteredUsers`.
- `internal/factory/engine_factory.go:NewEngineWithOptions`: type-asserts `repository.SqlUtilGroupBRepository` and injects the same `groupB` instance into both `MailService.GroupB` and `UserService.GroupB`.
- `internal/command/services.go:bundle`: exposes the engine's `service.Bundle` to commands.

**TEST-BACKED** `internal/factory/group_b_test.go` verifies Group B injection into both typed service owners. `internal/service/group_b_test.go` verifies the existing Saturn-shaped service delegators.

Therefore, the implementation scope is command dispatch, service invocation/result adaptation, and the minimal deletion capability bridge—not new factory wiring or a new DI framework.

### Remove currently has no real handler

**[OBSERVED]** `internal/command/registry.go:RegisterAll` registers canonical `remove` with aliases `del` and `delete` at moderator role. However:

- `internal/command/handlers.go:newCommand` has no `case "remove"`.
- `internal/command/identity_commands.go` contains `registerCommand`, `authorizeCommand`, `accessCommand`, and `messagesCommand`, but no remove command and no `DeleteIdentity` call.
- The fallback `internal/command/saturnCommand.Execute` handles `remove` in its accepted-placeholder group and replies **`"remove accepted"`** without changing persistence.

The old owner map was therefore insufficient: `identity_commands.go` is not currently a remove implementation.

### DeleteIdentity has an unexported authorization context

**[OBSERVED]** `internal/repository/h2/sql_util_group_b.go` defines the unexported `withSaturnAuthorization`, `saturnAuthorizationKey`, `authorizedSaturnContext`, and `errSaturnUnauthorized`. `(*h2.Database).DeleteIdentity` rejects an ordinary context and only proceeds with a context produced inside package `h2`.

No current `service.UserService` method delegates deletion, and `internal/service` cannot manufacture the private context. A direct call through the accepted interface from a command/service context is therefore execution-blocked.

### Last-messages still uses the legacy path

**[OBSERVED]** `internal/command/identity_commands.go:messagesCommand.Execute` calls `UserService.LastMessages("", trip, count)`. `UserService.LastMessages` delegates to legacy `repository.IdentityRepository.LastMessages`, not Group B.

The Group B result type is `repository.SaturnLastMessage`, with fields `Name`, `Message`, and `CreatedOn`; it has no `Trip`. The command currently renders `name#trip: message`, so implementation must explicitly adapt the Saturn-shaped result to that output. The command's trip argument supplies the output trip component.

Saturn references the complete command paths:

- `src/main/java/org/saturn/app/command/impl/moderator/LastMessagesCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/moderator/RemoveUserCommandImpl.java`

The prior bare filenames were ambiguous.

### Registered-users has two split owners and one direct DB path

**[OBSERVED]** `internal/command/users_nicks.go:usersCommand.Execute` calls `UserService.RegisteredUsers`, which delegates to legacy `UserQueryRepository.RegisteredUsers` and returns `[]repository.RegisteredUser`.

**[OBSERVED]** `internal/command/mail_notes.go:mailCommand.Execute` handles the `"user not registered"` error by calling `MailService.RegisteredUsers()`. That method in `internal/service/services.go` performs direct `DB.Query` formatting. It does not call `MailService.SaturnRegisteredUsers`.

**[RECOMMENDED SCOPE DECISION]** Both runtime-visible registered-user paths are in this parity slice:

1. The mail error response must use the accepted Saturn-shaped Group B method, because Saturn's `MailServiceImpl.executeMail` obtains its unregistered-recipient directory from `getRegisteredUsers`.
2. `!users` must also be made Group B-backed because `usersCommand` owns a separately reachable registered-users command and the requested target map explicitly includes `internal/command/users_nicks.go`. Its existing table formatting can remain; adapt `[]repository.SaturnRegisteredUser` to the existing `[]repository.RegisteredUser` formatting shape or add an equivalent local formatter.

The legacy `UserQueryRepository.RegisteredUsers` and direct `MailService.RegisteredUsers` methods may remain for unrelated compatibility callers, but neither may be used by the declared parity command paths.

## Correct target map

### Saturn reference behavior

- `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`: `deleteByNameOrTrip`, `delete`, and `lastMessages`.
- `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java`: `executeMail` and `getRegisteredUsers` behavior.
- `src/main/java/org/saturn/app/util/SqlUtil.java`: `DELETE_TRIP_NAMES`, `DELETE_TRIP`, `DELETE_NAME`, `SELECT_NAME_TRIP_REGISTERED`, and `SELECT_LAST_N_MESSAGES`.
- `src/main/java/org/saturn/app/command/impl/moderator/RemoveUserCommandImpl.java`: resolves the first argument and calls `engine.userService.deleteByNameOrTrip`, then reports Saturn success/error behavior.
- `src/main/java/org/saturn/app/command/impl/moderator/LastMessagesCommandImpl.java`: calls `engine.userService.lastMessages(null, trip, numberOfMessages)` and formats/escapes the result.

### Zenbot implementation targets

- `internal/repository/sql_util_group_b.go`: existing Group B interface and typed result shapes; do not create a second repository abstraction.
- `internal/repository/h2/sql_util_group_b.go`: existing H2 implementation; add only the minimal exported capability bridge described below, if the implementer confirms this is the selected bridge.
- `internal/service/services.go`: add the smallest service delegators needed for deletion and Group B registered-user reads; retain existing `LastMessages` as legacy API but do not use it for `messagesCommand`.
- `internal/factory/engine_factory.go`: **observed complete for Group B injection; no implementation change expected** unless verification finds regression.
- `internal/command/handlers.go:newCommand`: add construction for canonical `remove`.
- `internal/command/dispatch_adapter.go:RegisterUserUtilities`: add canonical `remove` to the actual inbound registration list after `newCommand` constructs the real command. `commandDefinitionFor("remove")` returns the existing `RegisterAll` definition with aliases `del`, `delete`, `remove`; registering one `legacyAdapter` therefore exposes all three aliases through `Engine.RegisterCommand` and the chat listener. This is required runtime wiring, not duplicate catalog registration.
- `internal/command/registry.go:RegisterAll`: **observed registration already exists; no duplicate registration**. Keep aliases `del`, `delete`, `remove` and moderator role.
- `internal/command/identity_commands.go:messagesCommand.Execute`: switch from `UserService.LastMessages` to `UserService.SaturnLastMessages`, pass `nil` name, preserve count cap/default semantics, and adapt output using the command trip.
- `internal/command/identity_commands.go` or a focused new `internal/command/remove.go`: implement the real moderator remove command. Prefer a focused `removeCommand` in `remove.go` to avoid expanding the unrelated identity command file; either location is acceptable if `handlers.go:newCommand` constructs it.
- `internal/command/users_nicks.go:usersCommand.Execute`: switch the declared `!users` parity path to Group B registered users and preserve its existing user-facing table format.
- `internal/command/mail_notes.go:mailCommand.Execute`: for the unregistered-recipient response, call the Group B-backed mail service method with the command context; do not call `MailService.RegisteredUsers()`.
- `internal/command/identity_commands_test.go`, `internal/command/service_commands_test.go`, and a focused `internal/command/users_nicks_test.go` or mail/runtime parity test file: add command-level runtime reachability tests. Existing tests alone are insufficient because the current message test proves the legacy `IdentityRepository` path.

## Minimal design and authorization bridge

### Selected bridge: optional exported capability method, not exported context internals

**[RECOMMENDED]** A minimal bridge is required. Do not remove the existing H2 authorization check and do not expose `saturnAuthorizationKey` or `authorizedSaturnContext`.

Use a narrow optional capability that keeps both selector resolution and private context creation inside `internal/repository/h2`:

1. Add an exported H2 method with the exact narrow contract `(*h2.Database).DeleteIdentityAuthorized(ctx context.Context, nameOrTrip string) (repository.DeleteResult, error)`. It trims the one selector, rejects blank input, and performs Saturn's `resolveRegisteredIdentity` equivalent with one query over `trip_names JOIN names JOIN trips`, matching `LOWER(n.name) = LOWER(?) OR LOWER(t.trip) = LOWER(?)`. It must require exactly one `(name, trip)` row: zero rows and ambiguous/multiple rows return the same failure result/error path without calling delete. For the unique row, it calls the existing private `withSaturnAuthorization(ctx)` and then the existing two-string `DeleteIdentity(ctx, resolvedName, resolvedTrip)`. Thus the required `(name, trip)` pair is resolved before the delete transaction; never pass the selector as one field and an empty string as the other.
2. Add a small optional interface in `internal/repository/sql_util_group_b.go` (for example, `SaturnAuthorizedDeleteRepository`) with exactly `DeleteIdentityAuthorized(context.Context, string) (DeleteResult, error)`. This is a capability check, not a replacement for `SqlUtilGroupBRepository` and not a generalized DI layer.
3. Add `UserService.DeleteIdentity(ctx context.Context, nameOrTrip string)` in `internal/service/services.go`. It type-asserts `s.GroupB` to that optional capability, calls the selector-based method, and returns a clear `"authorized Group B delete unavailable"` error when the capability is absent. The command passes its single trimmed first argument to this service seam; no command imports `internal/repository/h2`.
4. Implement the corresponding selector-based method on Group B test fakes. No caller can forge the private authorization context, and the existing `DeleteIdentity(ctx, name, trip)` remains the authorization-gated two-string repository operation.

This is the smallest executable path that honors the current H2 design, works through the existing factory/service ownership, and avoids security hardening or a broad abstraction. An alternative of exporting `h2.WithSaturnAuthorization(context.Context)` or adding a general resolver repository is explicitly rejected: it would expose private capability internals or broaden the seam without improving runtime ownership.

### Service/result adaptations

- `UserService.SaturnLastMessages(ctx, nil, trip, count)` remains the Group B call. `messagesCommand` renders each `SaturnLastMessage` as `message.Name + "#" + trip + ": " + message.Message`, preserving the current truncation and escaping rules. Do not infer a trip from `SaturnLastMessage`; the result type intentionally does not carry one.
- Add a `UserService.SaturnRegisteredUsers(ctx)` delegator, or use an explicitly shared service-level helper, so `usersCommand` can read the same accepted Group B shape without direct DB access.
- `MailService.SaturnRegisteredUsers(ctx)` already exists and must be used by the mail error branch. Convert rows to the existing Saturn-style `name trip\\n` response in command/service code; do not re-query `MailService.DB`.

## Minimal implementation sequence (design only)

1. Add/confirm the optional authorized-delete capability in `internal/repository/sql_util_group_b.go` and implement its H2 wrapper in `internal/repository/h2/sql_util_group_b.go`: accept one trimmed `nameOrTrip`, resolve it case-insensitively to exactly one registered `(name, trip)` pair using the existing joined identity tables, then call the existing private-authorized two-string delete transaction with both resolved values. Retain private context creation and existing `DeleteIdentity` behavior.
2. Add `UserService.DeleteIdentity(ctx context.Context, nameOrTrip string)` and, if needed, `UserService.SaturnRegisteredUsers(ctx)` in `internal/service/services.go`; add focused service tests for selector forwarding, capability success, and unavailable capability.
3. Implement `removeCommand` in `internal/command/remove.go` (or the equivalent focused identity file). Match Saturn's first-argument name-or-trip identity selector contract, trim it, invoke `UserService.DeleteIdentity(ctx, selector)`, and preserve Saturn success/error status and user-visible response; do not leave the fallback `"remove accepted"` path reachable for the registered command.
4. Add `case "remove"` to `internal/command/handlers.go:newCommand`; leave the existing `registry.go:RegisterAll` catalog definition and aliases intact.
5. Add canonical `remove` to `internal/command/dispatch_adapter.go:RegisterUserUtilities`'s `canonicals` list. Its existing `commandDefinitionFor` lookup and `legacyAdapter.GetAliases` must register `remove`, `del`, and `delete` with `Engine.RegisterCommand`, making all three reachable through the legacy inbound chat listener. Do not add a second catalog definition.
6. Change `messagesCommand.Execute` in `internal/command/identity_commands.go` to call the Saturn-shaped service method with `nil` name and adapt the result explicitly. Preserve the existing maximum-30 command behavior and Saturn's non-positive default of 5.
7. Change `usersCommand.Execute` in `internal/command/users_nicks.go` to the Group B registered-user service path, preserving output formatting. Change the unregistered-recipient branch in `internal/command/mail_notes.go` to `MailService.SaturnRegisteredUsers(ctx)` and format the returned rows.
8. Add runtime parity tests, including alias dispatch through `RegisterUserUtilities`/the chat listener, then run the focused command/service/factory/repository suites. Do not modify protected documents or unrelated dirty files.

## Proposed runtime sequence

```text
command registry catalog: remove/del/delete
              |
              v
RegisterUserUtilities -> commandDefinitionFor("remove")
              |
              v
legacyAdapter + aliases -> Engine.RegisterCommand -> chat listener
              |
              v
handlers.go:newCommand -----> removeCommand -----> UserService.DeleteIdentity(ctx, selector)
                                                        |
                                                        v
                              H2 resolves selector -> unique (name, trip)
                                                        |
                                                        v
                                    optional authorized-delete capability
                                                        |
                                                        v
                                    h2 DeleteIdentityAuthorized(ctx, nameOrTrip)
                                      (private h2 auth context)

messagesCommand -----------------> UserService.SaturnLastMessages
usersCommand --------------------> UserService.SaturnRegisteredUsers
mailCommand error branch --------> MailService.SaturnRegisteredUsers
                                      |
                                      v
                         existing GroupB instance injected by
                         engine_factory.go into service.Bundle
                                      |
                                      v
                         SqlUtilGroupBRepository / H2 methods
```

**[OBSERVED]** The factory/service injection segment of this diagram already exists, as do the `RegisterAll` catalog definition and the `legacyAdapter` registration mechanism. **[RECOMMENDED]** The missing implementation work is adding `remove` to `RegisterUserUtilities`, constructing the real command in `handlers.go:newCommand`, resolving the selector inside H2, and traversing the authorization capability.

## Saturn semantics to preserve

- `RemoveUserCommandImpl.execute`: trim the first argument as Saturn's single name-or-trip identity selector; `UserService.DeleteIdentity(ctx, selector)` must resolve it case-insensitively to exactly one registered `(name, trip)` pair before invoking the authorized two-string delete. Preserve the success/error outcome rather than accepting without mutation.
- `LastMessagesCommandImpl.execute`: pass nullable name (`nil` in Go), preserve default count 5 for non-positive values, cap command requests above 30, exclude `LEFT` and `JOINED`, order newest first, truncate displayed messages at 200 characters as current Zenbot behavior does, and escape the final output consistently.
- `MailServiceImpl.executeMail`: when the receiver is absent, use the registered-user directory from the Saturn-shaped Group B path and preserve the existing explanatory response.
- `SqlUtil.java` semantics remain represented by the accepted H2 implementation; no duplicate SQL utility or command-level DB query is introduced.

## Runtime parity tests required

The current repository has no runtime parity tests for these command paths. Existing repository/service tests and factory injection tests do not prove command reachability.

Add focused tests with Group B-aware fakes and dispatch through `newCommand`/the command registry:

1. **Remove dispatch and execution** (`internal/command/identity_commands_test.go` or `remove_runtime_parity_test.go`): call `RegisterUserUtilities` on a listener-backed engine, dispatch `!remove`, `!del`, and `!delete` through the actual chat listener, verify all aliases are registered and construct the real remove command, and verify they call `UserService.DeleteIdentity` with the one trimmed selector, do not produce `"remove accepted"`, and preserve success/error behavior.
2. **Authorization capability and selector resolution** (service/repository-focused test): ordinary `DeleteIdentity(ctx, name, trip)` remains rejected by H2; the authorized selector capability trims and case-folds a name-or-trip value, resolves exactly one registered row to the correct `(name, trip)` pair, passes both values to the existing delete transaction, and returns `DeleteResult`; zero/multiple matches do not delete; an implementation with no optional capability returns the documented unavailable error.
3. **Last messages runtime reachability** (`internal/command/identity_commands_test.go`): a fake must fail the test if legacy `IdentityRepository.LastMessages` is called, record `SaturnLastMessages(ctx, nil, trip, count)`, and return typed rows. Verify `name#trip` adaptation, truncation/escaping, max-30 cap, and non-positive default forwarding/behavior.
4. **Mail registered-users path** (`internal/command/service_commands_test.go` or `mail_runtime_parity_test.go`): make the legacy direct-DB directory unavailable or observable, return a Group B row from `SaturnRegisteredUsers`, and verify the unregistered-recipient response uses that row.
5. **`!users` registered-users path** (`internal/command/users_nicks_test.go` or shared runtime parity test): verify `usersCommand` reaches the Group B Saturn-shaped method and preserves the existing table output. A fake legacy `UserQueryRepository.RegisteredUsers` must not be required.
6. **Factory preservation** (`internal/factory/group_b_test.go`): retain the existing test proving one Group B repository instance is injected into both service owners; no new factory wiring test is needed unless implementation changes the constructor.

## Complexity, tradeoffs, and ownership

- **Complexity:** low to moderate. Existing Group B interface, H2 implementation, service delegators, and factory injection are reused. New work is command dispatch, thin service methods, explicit result adaptation, a tiny optional capability, and focused tests.
- **Security tradeoff:** the user-approved scope does not require additional hardening. The private H2 authorization context remains private; the bridge exposes only the intended operation through a typed capability.
- **No broad abstraction:** do not add a generalized dependency-injection framework, alternate SQL repository, command-wide DB access, or parallel service graph.
- **Primary implementation owners:** `internal/command/handlers.go`, `internal/command/remove.go` (or focused identity equivalent), `internal/command/identity_commands.go`, `internal/command/users_nicks.go`, `internal/command/mail_notes.go`, and thin additions in `internal/service/services.go`.
- **Existing owners not to duplicate:** `internal/factory/engine_factory.go` and `internal/command/registry.go` already provide the relevant Group B injection and remove registration.

## Explicit exclusions and NOT COMPLETE boundaries

- **NOT COMPLETE:** unrelated Saturn migration work outside SQL utility Group B runtime parity.
- **NOT COMPLETE:** changes to protected `MIGRATION_PLAN.md` or `.hermes/migration-audit.md`.
- **NOT COMPLETE:** redesigning repository interfaces beyond the one narrow optional authorized-delete capability, adding a new SQL abstraction, or introducing generalized dependency injection.
- **NOT COMPLETE:** broad security hardening; security is not a blocker for this approved parity slice.
- **NOT COMPLETE:** unrelated command, service, engine, database, transport, or domain migrations.
- **NOT COMPLETE:** performance tuning, schema redesign, or backend support beyond the accepted repository methods and existing H2 implementation.
- **NOT COMPLETE:** claiming full Saturn-to-Zenbot migration completion.

## Completion checklist for the later implementation

- [ ] This corrected handoff is reviewed as the implementation source of truth; the prior QA FAIL findings are resolved.
- [ ] Real remove command and `handlers.go:newCommand` dispatch replace the `"remove accepted"` fallback, and `RegisterUserUtilities` makes `remove`, `del`, and `delete` runtime-dispatchable.
- [ ] Minimal optional authorized-delete capability accepts one selector, resolves exactly one case-insensitive registered `(name, trip)` pair before deletion, and reaches H2 without exporting private context internals.
- [ ] Last-messages command no longer calls legacy `IdentityRepository.LastMessages`.
- [ ] Last-messages typed result is explicitly adapted to `name#trip` output.
- [ ] Mail's unregistered-recipient directory no longer uses `MailService.DB` direct query.
- [ ] `!users` scope is explicitly Group B-backed through `users_nicks.go`.
- [ ] Runtime parity tests cover remove, last-messages, mail registered users, and `!users`.
- [ ] Existing Group B factory/service injection remains intact.
- [ ] Protected documents remain unchanged and unrelated migration remains NOT COMPLETE.
