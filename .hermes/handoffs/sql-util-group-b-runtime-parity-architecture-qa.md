# SQL Utility Group B Runtime-Parity Architecture QA

## Verdict

**PASS — implementation gate OPEN, with two implementation corrections recorded below.**

The repaired architecture is accurate against the current Zenbot and Saturn sources and resolves the two blockers identified by the prior QA: it specifies one-selector resolution to one `(name, trip)` pair inside H2, and it identifies `internal/command/dispatch_adapter.go:RegisterUserUtilities` as the required legacy runtime-registration owner. The proposal remains minimal and does not require source changes to the factory or protected documents.

## Independent evidence: current Zenbot source

### Remove, fallback, and runtime registration

- `internal/command/registry.go:RegisterAll` already defines canonical `remove` with aliases `del`, `delete`, and `remove`, role `model.MODERATOR`.
- `internal/command/handlers.go:newCommand` currently has no `case "remove"`; an unresolved canonical therefore falls through to `saturnCommand`.
- `internal/command/registry.go:saturnCommand.Execute` currently includes `remove` in the accepted-placeholder switch and emits `" remove accepted"` without persistence mutation.
- `internal/command/dispatch_adapter.go:RegisterUserUtilities` is the actual legacy inbound registration path. Its current canonical list omits `remove`; catalog presence alone does not populate `Engine.RegisterCommand` or the chat listener.
- The repaired target map and sequence correctly require a focused real `removeCommand`, `handlers.go:newCommand` construction, and adding canonical `remove` to `RegisterUserUtilities`. Because `commandDefinitionFor("remove")` returns the existing definition whose aliases are `del`, `delete`, and `remove`, one `legacyAdapter` registration exposes all three aliases without duplicate catalog entries.

**Implementation correction 1 (minor):** place `remove` in the registration branch whose prerequisite is the user service / authorized-delete capability, not behind an unrelated `Security != nil` check unless the runtime invariant explicitly guarantees both. The architecture's required observable result is that a configured Group B user service makes all three aliases reachable; tests should verify alias registration and actual listener dispatch.

### Messages and registered-user paths

- `internal/command/identity_commands.go:messagesCommand.Execute` currently requires a trip and count, caps counts above 30, then calls legacy `UserService.LastMessages("", trip, n)`, rendering `m.Name + "#" + m.Trip`.
- `internal/service/services.go:UserService.LastMessages` delegates to legacy `IdentityRepository.LastMessages`.
- Current `repository.SaturnLastMessage` has `Name`, `Message`, and `CreatedOn`, but no `Trip`; the repaired handoff correctly requires an explicit adaptation using the command trip. It also correctly requires `SaturnLastMessages(ctx, nil, trip, count)`, preserving the 30 cap, non-positive Group B default of 5, newest-first ordering, `LEFT`/`JOINED` exclusion, truncation, and escaping.
- `internal/command/users_nicks.go:usersCommand.Execute` currently calls legacy `UserService.RegisteredUsers` through `Queries`; the repaired handoff correctly includes this separate runtime-visible path and requires Group B-backed rows with existing table formatting.
- `internal/command/mail_notes.go:mailCommand.Execute` currently handles `user not registered` with `b.Mail.RegisteredUsers()`, whose implementation directly queries `MailService.DB`. The repaired handoff correctly switches only this directory response to `MailService.SaturnRegisteredUsers(ctx)` and leaves the separate mail `Queue` validation path alone.

### Existing Group B injection

- `internal/factory/engine_factory.go:NewEngineWithOptions` type-asserts `repository.SqlUtilGroupBRepository` and injects the same instance into both `MailService.GroupB` and `UserService.GroupB`.
- Existing `internal/factory/group_b_test.go` verifies this shared injection; existing service tests verify Saturn-shaped delegators. The repaired architecture correctly marks this seam complete and does not propose factory or DI changes.

### H2 authorization internals and one-selector resolution

- `internal/repository/h2/sql_util_group_b.go` currently keeps `saturnAuthorizationKey`, `withSaturnAuthorization`, `authorizedSaturnContext`, and `errSaturnUnauthorized` private.
- `(*h2.Database).DeleteIdentity(ctx, name, trip)` rejects an ordinary context and only invokes the existing delete transaction for a context carrying the private authorization value.
- `internal/service/services.go` has no deletion delegator today, and the public Group B interface currently exposes only the two-string, authorization-gated operation.
- The repaired bridge is precise: optional `DeleteIdentityAuthorized(context.Context, string nameOrTrip)`; H2 trims/rejects blank input, queries the joined `trip_names`/`names`/`trips` identity relation case-insensitively, requires exactly one distinct `(name, trip)` row, creates the private authorized context inside package `h2`, and calls the existing two-string delete with both resolved values.
- Zero matches and more than one distinct pair are correctly specified as no-op failures; the command must return failure/error behavior and must not call deletion. A unique match resolves the stored canonical name/trip values before deletion, avoiding the incorrect selector-plus-empty-field call.
- `UserService.DeleteIdentity(ctx, nameOrTrip)` type-asserting the optional capability and returning a clear unavailable-capability error is the smallest service bridge. It does not export H2 authorization context internals or add a broad resolver abstraction.

**Implementation correction 2 (minor clarification):** tests and implementation should state the failure contract explicitly: blank, missing, and ambiguous selectors return a non-success result/error (or the documented Saturn-equivalent failure), and the delete transaction is not invoked. The architecture already requires this behavior, but it should be asserted directly rather than inferred from a generic SQL error.

## Saturn source verification

- `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`: `resolveRegisteredIdentity` trims, rejects blank, uses case-insensitive name-or-trip matching, returns empty for zero or multiple rows, and `deleteByNameOrTrip` deletes the resolved exact name/trip or returns failure code `1`.
- The same file's `lastMessages` uses nullable name, defaults non-positive counts to 5, excludes `LEFT`/`JOINED`, and constructs message results with the requested trip.
- `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java:executeMail` obtains the registered-user directory only when the receiver is absent; `getRegisteredUsers` formats `name trip\\n`.
- `src/main/java/org/saturn/app/util/SqlUtil.java` contains the delete statements, registered-user query, and last-message query cited by the repaired architecture.
- `RemoveUserCommandImpl.java` uses the first trimmed argument, calls `deleteByNameOrTrip`, reports failure on code `1`, and replies `User has been removed successfully` otherwise.
- `LastMessagesCommandImpl.java` requires two arguments, caps above 30, calls `lastMessages(null, trip, count)`, escapes output, and truncates long messages to the first 200 characters plus `...`.

## Minimal implementation and test gate review

The proposed sequence is minimal and correctly ordered:

1. Add the optional selector-based capability and H2 wrapper while retaining private authorization internals and the existing delete transaction.
2. Add service delegators and update fakes.
3. Implement real `removeCommand` with one trimmed selector and Saturn success/error behavior.
4. Add `case "remove"` to `newCommand`.
5. Add canonical `remove` to `RegisterUserUtilities`; preserve `RegisterAll` and aliases.
6. Switch messages to Group B with `nil` name and explicit trip adaptation.
7. Switch `!users` and mail's unregistered-recipient directory to Group B.
8. Add alias-level listener tests and Group B-aware command/service/repository tests.
9. Run focused suites and verify protected/unrelated files.

Required tests are sufficient when they assert:

- `!remove`, `!del`, and `!delete` dispatch through `RegisterUserUtilities` and the actual chat listener to the real handler, mutate through the service capability, and never emit `remove accepted`.
- Selector success, blank/missing, zero-match, and ambiguous/multiple-match behavior; ordinary H2 two-string deletion remains unauthorized; absent optional capability returns the documented unavailable error.
- Messages never call legacy `IdentityRepository.LastMessages`, forward `nil` name, preserve cap/default behavior, and adapt/truncate/escape output.
- Mail and `!users` use Group B methods rather than the legacy direct DB/query paths.
- Existing factory shared-instance Group B injection remains passing.

## Verification performed

Focused baseline command/service/repository/factory suites were run from the target repository:

```text
go test ./internal/repository/... ./internal/service/... ./internal/command/... ./internal/factory/...
?    zenbot/internal/repository [no test files]
ok   zenbot/internal/repository/h2 (cached)
ok   zenbot/internal/service (cached)
ok   zenbot/internal/command (cached)
ok   zenbot/internal/factory (cached)
```

These are baseline tests only; they do not yet prove the proposed new runtime-parity paths, so implementation must add the focused tests above.

## Implementation gate status

**OPEN / APPROVED TO IMPLEMENT**, subject to the two minor corrections above:

1. Ensure runtime registration is conditioned on the actual user/delete capability rather than an unrelated security-service prerequisite.
2. Assert explicit no-delete failure behavior for blank, missing, and ambiguous selectors.

No application source or protected document was modified during this QA pass. Only this QA artifact was overwritten. The architecture remains limited to SQL Utility Group B runtime parity; it does not claim full Saturn migration completion.
