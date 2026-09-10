# Admin `grant` / `access`: current architecture and safety decision

## Decision

**BLOCKED: do not expose or treat `grant`/`access` as a safe strict-parity vertical in the current target.** The handler exists in the dirty worktree and the conditional registrar can expose it, but the inbound authorization principal is selected by case-insensitive nickname alone. There is no transport-authenticated caller identity boundary and no test proving a chat event is bound to the authoritative joined principal before a role mutation.

This is not solved by comparing a supplied nick, trip, or hash: all are message/join payload fields and the requested policy explicitly forbids inferring identity from nick/hash. The required unblocker is a transport/session-authenticated principal supplied at ingress (or an existing authoritative connection identity if the transport can expose it), propagated unchanged to command dispatch. Until then, `ADMIN` authorization can be attributed to whichever active user has the same nickname; map iteration makes duplicate-name selection non-deterministic.

The source behavior itself has no protected-principal or anti-escalation policy: an authorized admin may set any target trip to any enum role, including `ADMIN` or `PEST`. Adding a target-protection policy would be a deliberate security divergence, not strict parity. It must not be smuggled into this vertical.

## Evidence and source-target contract map

| Topic | Saturn source | Current Zenbot | Assessment |
|---|---|---|---|
| Canonical / aliases / command role | `[OBSERVED]` `AccessUserCommandImpl.java` declares `grant`, `access`; inherited `UserCommandBaseImpl.getAuthorizedRole()` is `ADMIN`. | `[TEST-BACKED]` `internal/command/registry.go` catalog definition is `access`, aliases `grant,access`, `ADMIN`; `admin_moderator_catalog_guard_test.go` guards it. | Catalog parity exists. |
| Caller authorization | `[OBSERVED]` `UserCommandBaseImpl.execute()` asks `AuthorizationServiceImpl.isUserAuthorized(command, chatMessage)` before execution. Source allows configured admin/user trips or persisted role >= required. | `[OBSERVED]` `listener/message/handlers.go:DispatchUserCommand` authorizes `c.Author`; `ResolveUserMetadata` assigns it solely by `strings.EqualFold(active.Name, message.Name)`. `SecurityService` then checks configured trips/persisted role. | **Blocker:** no authenticated event-to-principal binding. |
| Arguments | `[OBSERVED]` exactly two parsed whitespace arguments; source rejects missing caller trip and sends `\n Set your trip first. Example: <prefix>grant 8Wotmg ADMIN`. `Role.valueOf` is uppercase/case-sensitive. | `[TEST-BACKED]` `identity_commands.go` requires two args/nonblank `message.Trip`, shares exact usage string, and `parseRole` accepts only uppercase strings; `identity_commands_test.go` covers lowercase rejection. | Handler-shaped parity, subject to ingress block. |
| Single target | `[OBSERVED]` `authorizationService.grant(targetTrip, newRole)` inserts or updates `trips.type`; success reply is `\n Granted new Role: ROLE to trip: TARGET`. | `[OBSERVED]` `accessCommand` calls `Security.Authorization.GrantTrip(ctx,target,role)` then same string. `h2/authorization.go` uses a transaction to select/update-or-insert. | Transactional repository is safer than source; source swallows SQL errors, target returns error/no success reply. This is intentional reliability divergence unless strict source failure swallowing is required. |
| Comma target quirk | `[OBSERVED]` source splits first argument on commas and **always grants `USER`**, ignoring the requested role; reply nevertheless prints requested `newRole`. Java `String.split` drops trailing empty elements. | `[TEST-BACKED]` target grants each comma component `USER` and reports requested role; test asserts `first,second, ADMIN` -> two `User` grants. It removes trailing empty entries to emulate Java split. | Behavioral quirk captured. Per-item errors are currently discarded, as source does. |
| Role storage | `[OBSERVED]` Saturn enum order: `ADMIN, MODERATOR, TRUSTED, USER, REGULAR, PEST`; service resolves/updates `trips.type`. | `[TEST-BACKED]` same role order in `internal/model/role.go`; `resources/schema-h2.sql` restricts `trips.type`; H2 grant/resolve tests cover every role and update. | Persistence schema and hierarchy align. |
| Replies / whisper | `[OBSERVED]` replies route to author using original `chatMessage.isWhisper()`. Invalid role emits no response. | `[OBSERVED]` shared `reply` preserves `IsWhisper || Whisper || Type == whisper`; access invalid role replies nothing. Focused target test covers multi reply shape (non-whisper). | Need exact single/missing-trip/whisper tests before any safe exposure. |
| Registration / fallback | `[OBSERVED]` full catalog has the command. | `[OBSERVED]` `RegisterUserUtilitiesWithDirectAgent` registers `access` only when `Users.GroupB != nil && Security != nil`; generic fallback explicitly excludes `access`; `dispatch_integration_test.go` expects no access in a baseline engine. | Conditional registration exists, but gate does not prove `Security.Authorization != nil` and does not prove ingress identity. |
| Agent access | `[OBSERVED]` target `RunCommand` says fixed public informational aliases only; execution uses a fixed allowlist. | `[OBSERVED]` no grant/access agent tool is found; current moderation tool is separately capability-gated. | Keep this unchanged; no agent-tool exposure. |

## Current end-to-end path and exact blocker

```text
chat JSON {nick, trip, hash, text}
  -> UserChatListener.NotifyContext
  -> message.DefaultChain
  -> ResolveUserMetadata: scan active-user map by EqualFold(nick) only
  -> DispatchUserCommand: IsUserAuthorized(c.Author, ADMIN)
  -> legacyAdapter.ExecuteContext
  -> accessCommand.Execute
  -> Security.Authorization.GrantTrip
  -> H2 transaction: SELECT id; UPDATE type or INSERT trips row
  -> SendChatMessage(author, reply, incoming whisper flag)
```

`UserJoinedListener.Notify` adds `model.GetUser(join JSON)` to the active-user map. Neither it nor `ResolveUserMetadata` exposes a connection/session identity token. `ResolveUserMetadata` copies the selected active hash into the message but does not compare message trip/hash with the selected user, and comparisons would still be an inference from unauthenticated payload data. `GetActiveUserByName` also returns the first matching map entry. No focused test demonstrates duplicate nick rejection, mismatch rejection, or binding to an authenticated transport principal.

Therefore this vertical would create a persistent authorization mutation based on an ambiguous principal. Existing `authorize` has the same ingress weakness; its existence is evidence of an existing debt, not approval to widen the write surface.

## Bounded unblocker interface and file map

`[RECOMMENDED]` Add no new public agent capability and no nick/hash-derived identity helper. Add one narrow ingress capability:

```go
// internal/common (or listener/message): populated only by authenticated transport session state.
type CommandPrincipal struct {
    Subject string // opaque, stable transport/session subject; never nick/trip/hash
    User    *model.User // authoritative snapshot attached by the session layer
}
type AuthenticatedCommandPrincipalResolver interface {
    ResolveCommandPrincipal(*model.ChatMessage) (*CommandPrincipal, bool)
}
```

Only the websocket/transport owner may implement it. `ResolveUserMetadata` must obtain the principal through this resolver, reject absence/mismatch fail-closed for privileged dispatch, and `DispatchUserCommand` must authorize `principal.User`. Do not let commands query active users by name for authorization. The message trip remains only a source-compatible input precondition (`grant` requires it nonblank), not authority.

After that prerequisite, keep the command mutation boundary narrow:

* `internal/repository/repository.go`: reuse `AuthorizationRepository`; do not add raw SQL capability.
* `internal/repository/h2/authorization.go`: retain `GrantTrip(ctx, trip, role)` transaction. Its select/update-or-insert is the sole persistence write.
* `internal/service/security_service.go`: add a non-nil, context-aware grant method only if it avoids the current direct `b.Security.Authorization` dereference; do not mutate `AdminTrips` for `access`.
* `internal/command/identity_commands.go`: retain a concrete `accessCommand`; call the narrow service method.
* `internal/command/dispatch_adapter.go`: register only when the authenticated-principal dispatch capability **and** writable authorization boundary are composed. A type assertion on `Security != nil` is insufficient because `Security.Authorization` can be nil.
* `internal/listener/message/handlers.go` and transport composition: own authenticated caller resolution and fail-closed behavior.
* `internal/agent/**`: no changes. `run_command` remains its fixed public list.

## Sequential strict TDD design and first RED (after the ingress prerequisite)

Do not start the access handler/red tests first while caller identity remains ambiguous. First vertical slices:

1. **RED:** an `ADMIN`-role command with a chat nick matching an admin active user but no authenticated session principal produces no mutation/reply; expected failure is missing principal, not an arbitrary nickname lookup. **GREEN:** dispatch resolves only the transport principal and fails closed. Then refactor the resolver ownership.
2. **RED:** two same-nick active users with distinct authoritative sessions never let one session use the other user's role. **GREEN:** resolver keys on opaque session subject; dispatch no longer scans active users. This test proves the actual blocker is closed.
3. **RED:** with a trusted authenticated admin principal and writable H2 repository, `!grant target ADMIN` persists exactly one `trips` row with `ADMIN` and sends exactly `\n Granted new Role: ADMIN to trip: target` using the incoming whisper flag. **GREEN:** register the concrete adapter only under complete composition.
4. **RED:** `!access a,b, ADMIN` persists `a` and `b` as `USER`, preserves source-shaped success reply, and excludes Java-dropped trailing blanks. **GREEN:** use bounded comma handling; intentionally document ignored per-item errors if retained for parity.
5. **RED:** missing/blank invoker trip returns `FAILED` with the source usage reply; lowercase role returns `FAILED` with no write/reply; repository error returns `FAILED`, no success reply, and transaction leaves state unchanged. **GREEN:** minimal command/service error paths.
6. **RED:** an engine lacking authenticated-principal dispatch or a non-nil authorization writer does not register either alias. **GREEN:** make registration capability-based, preserving baseline `access` absence.
7. **RED:** agent `run_command` rejects `grant` and `access` with no repository call. **GREEN:** no production widening should be necessary; retain as regression coverage.

Run each RED alone and observe the expected failure before implementation; then run that test green, then the focused package set.

## Risks, QA, and no-scope

* `[RISK]` The source has no test located for `AccessUserCommandImpl`; exact behavior is source-read, while current target behavior has focused tests only for aliases, grant error, case-sensitive role parsing, and comma semantics.
* `[RISK]` Current target persistence is transactional/error-propagating whereas Saturn logs and swallows SQL failures. Preserve target atomicity unless product explicitly prioritizes source's false-success behavior.
* `[RISK]` `accessCommand` currently can panic if `b.Security` exists but `b.Security.Authorization` is nil; the registrar's current gate permits that shape. Registration must require the writer, not merely the service object.
* `[RISK]` Do not add target-side protected-principal checks in a claimed strict-parity slice. If desired, make a separately approved policy command with explicit UX/audit semantics.
* `[QA]` Focused read-only validation completed: `go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message` passed; `git diff --check` passed.
* `[NO-SCOPE]` No changes to whiskey/proxy, mine, restart/shutdown, raw admin SQL, semantic moderation, agent public tools, source/tests, reset/clean/checkout/stage/commit. This handoff is the only file created by this task.
