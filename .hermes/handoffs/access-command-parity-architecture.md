# `grant` / `access` Saturn-parity implementation architecture

## Scope decision

This document **supersedes** `.hermes/handoffs/access-command-current-architecture.md` for the user-approved scope: implement the Saturn `AccessUserCommandImpl` vertical with Saturn's existing `ADMIN` authorization requirement. Reuse Zenbot's existing shared dispatch authorization gate exactly; do **not** add authenticated-principal infrastructure or alter authorization policy.

This is a narrow Go rewrite of Saturn's command and persistence behavior. It is not an identity-boundary redesign. The existing nickname-to-active-user resolution remains an observed Zenbot limitation, accepted for this parity slice rather than expanded here.

## Source contract

### Command catalog and authorization

| Contract | Saturn evidence | Zenbot implementation target |
|---|---|---|
| Canonical catalog identity | `AccessUserCommandImpl.java` has `@CommandAliases({"grant", "access"})`. | `internal/command/registry.go` definition `access`, aliases `grant`, `access`. |
| Required role | `UserCommandBaseImpl.getAuthorizedRole()` returns `Role.ADMIN`; `UserCommandBaseImpl.execute()` calls `authorizationService.isUserAuthorized(cmd, chatMessage)` before command `execute()`. | Keep catalog role `model.ADMIN`; reuse `message.DispatchUserCommand.Handle` unchanged: `c.Engine.IsUserAuthorized(c.Author, cmd.GetRole())` before `ExecuteContext`/`Execute`. |
| Authorization decision | `AuthorizationServiceImpl.isUserAuthorized` allows an exact configured trip or `x`, otherwise compares persisted `resolveRole(chatMessage.getTrip())` with the command minimum. `Role.ADMIN` is the highest source role. | Reuse `SecurityService.IsAuthorized` → `IsAuthorizedContext` → `AuthorizationRepository.IsTripAuthorized`; H2 checks configured `AdminTrips` (including `x`) before persisted role hierarchy. No command-local authorization check. |
| Existing dispatch author | `DispatchUserCommand` consumes `c.Author`; `ResolveUserMetadata` currently selects an active user by case-insensitive name. | Do not change either handler, event payload, `Context`, active-user lookup, or transport. The shared gate above is the sole authorization gate for `grant`/`access`. |

### Parsing, mutation, replies, and whisper routing

[OBSERVED] Saturn `UserCommandBaseImpl` removes the prefix, trims, parses whitespace-separated arguments, and `AccessUserCommandImpl.execute()` requires exactly two arguments plus a non-null `chatMessage.trip`.

| Input / outcome | Required Go behavior |
|---|---|
| Aliases | Both `!grant` and `!access` route to the same `access` definition; alias spelling is not normalized in the emitted success text. |
| Missing argument count or absent trip | Return `model.FAILED`; send exactly `\n Set your trip first. Example: <prefix>grant 8Wotmg ADMIN` to the message author. Source checks `trip == null`, not blank; retain current Go `strings.TrimSpace(message.Trip) == ""` only as the existing target behavior already present in `internal/command/identity_commands.go`, not as new policy. |
| Role spelling | Parse only source enum literals `ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, `PEST`, case-sensitively. A bad/lowercase role returns `model.FAILED`, performs no write, and emits no reply. |
| One target | Call `GrantTrip(ctx, target, requestedRole)`. On success return `model.SUCCESSFUL` and reply exactly `\n Granted new Role: ROLE to trip: TARGET`. |
| Comma target quirk | If the raw first argument contains `,`, split it on commas. Grant **every resulting item `USER`**, ignoring the requested role. Drop trailing empty items to match Java `String.split(",")`; retain leading/interior empty items. Return `model.SUCCESSFUL` and reply exactly `\n Granted new Roles: ROLE to trips: [item1 item2]`. The displayed role is requested `ROLE`, even though every write is `USER`. |
| Comma write errors | Preserve the source-shaped best-effort quirk: execute each `GrantTrip(ctx, trip, model.USER)` call and deliberately ignore individual errors; still send the plural success reply and return successful. This is limited to the comma branch. |
| One-target write error | Propagate the repository error, return `model.FAILED`, and send no success reply, matching current Go error plumbing. Saturn logs/swallows its SQL exceptions and may claim success; do not redesign H2 transaction/error behavior to emulate that defect. |
| Reply recipient / whisper | Use the existing `reply` helper and command message. It sends to `message.Name` and preserves `message.IsWhisper || message.Whisper || message.Type == "whisper"`. Shared-dispatch unauthorized replies remain owned by `DispatchUserCommand` and use `c.Author.Name` with incoming `Message.IsWhisper`. |

## Existing target path to retain

```text
chat payload
  -> listener.UserChatListener
  -> message.DefaultChain
  -> ResolveUserMetadata (existing case-insensitive active-user lookup)
  -> DispatchUserCommand
       BuildCommand(alias)
       IsUserAuthorized(c.Author, cmd.GetRole()==ADMIN)   [shared gate]
  -> legacyAdapter.ExecuteContext
  -> accessCommand.Execute
  -> Security.Authorization.GrantTrip
  -> h2.Database.GrantTrip
  -> reply / Engine.SendChatMessage
```

No direct `accessCommand` caller-role lookup is permitted. No second authorization decision is permitted. Existing shared dispatch denial remains: no mutation or command execution; if `c.Author != nil`, reply ` you are not authorized to run: <typed-alias> command.` using `Message.IsWhisper`.

## H2 persistence contract

[OBSERVED] Saturn `AuthorizationServiceImpl.grant` resolves whether `trips.trip` exists, then updates `trips.type` or inserts `(type, trip, created_on)`. `SqlUtil` names the statements; source SQL errors are logged and swallowed.

[OBSERVED] Zenbot already owns the typed persistence seam:

* `internal/repository/repository.go`: `AuthorizationRepository` exposes only `IsTripAuthorized`, `GrantTrip`, and `ResolveRole`.
* `internal/repository/h2/authorization.go:Database.GrantTrip`: trims target trip, validates the role, starts a transaction, selects `trips.id` by `trip`, updates `type` for an existing row or inserts `(type, trip, created_on)` otherwise, and commits. On an error it returns that error and rolls back through the deferred error path.
* `resources/schema-h2.sql`: existing `trips` schema has a unique `trip`, required `created_on`, and only the six Saturn role names in the `type` check.

Reuse this repository method unchanged. Do not add raw SQL, a new repository interface, another service, an application-config mutation, a transaction wrapper, or a schema migration. `SecurityService.AuthorizeTrip` is not the `access` mutation path because it also mutates `AdminTrips`; `access` must retain the existing direct persisted `Authorization.GrantTrip` path.

## Registration capability gate

`RegisterUserUtilitiesWithDirectAgent` currently exposes `register`, `access`, and `messages` together only when `bundle(e)`, `Users`, `Users.GroupB`, and `Security` are non-nil. The `accessCommand` itself dereferences `Security.Authorization`.

The implementation must register `access` (and therefore both aliases) only when all existing composition prerequisites are present:

```go
b := bundle(e)
b != nil && b.Users != nil && b.Users.GroupB != nil &&
b.Security != nil && b.Security.Authorization != nil
```

Keep the existing Group B registration grouping unless the implementation needs to split this list solely to make the condition explicit. Do not expose `access` through the generic `saturnCommand` fallback. Do not register it in a baseline/stub engine lacking the writable `AuthorizationRepository`; this prevents a nil dereference and preserves the current non-exposure baseline.

`internal/factory/engine_factory.go` already supplies a `SecurityService` with `Authorization` when the repository implements `AuthorizationRepository`; H2 does. No new factory capability, interface, transport field, or dependency injection path is required.

## Strictly sequential one-tracer RED → GREEN plan

Run one tracer at a time. Each starts with exactly one failing focused test, verify its RED failure, make only the minimal production change for that tracer, run it green, then continue. Do not begin the next tracer while the current one is red.

1. **RED — catalog and shared `ADMIN` dispatch.** Add one listener-to-command tracer with an active author whose persisted/configured role authorizes `ADMIN`; prove `!grant target ADMIN` reaches the concrete `access` definition only after the existing `DispatchUserCommand` `IsUserAuthorized(c.Author, ADMIN)` call. The initial failure should be missing/non-exposed concrete access registration, not a new identity abstraction. **GREEN:** register the existing concrete definition under the writable composition gate; leave `ResolveUserMetadata` and `DispatchUserCommand` unchanged.
2. **RED — single target, aliases, exact response, whisper.** Through the same listener tracer, prove both `grant` and `access` invoke one-target `GrantTrip(target, ADMIN)`, create/update exactly one H2 `trips` role, and emit `\n Granted new Role: ADMIN to trip: target` to the author with incoming whisper state. **GREEN:** connect/reuse `accessCommand` and `reply`; no alternate sender or command-local authorization.
3. **RED — source input failures.** Prove wrong argument count or missing target-side trip gets the exact `Set your trip first` reply and `FAILED`; prove lowercase `admin` yields `FAILED` with no H2 write/reply. **GREEN:** retain the two-argument/trip guard and exact, uppercase-only `parseRole` map.
4. **RED — comma quirk.** Prove `!access first,second, ADMIN` writes `first` and `second` as `USER`, returns success, and emits `\n Granted new Roles: ADMIN to trips: [first second]`; add a trailing-comma assertion showing Java-style trailing blank removal. **GREEN:** retain bounded split/trim-trailing-empty behavior and ignored per-item errors.
5. **RED — normal H2 error.** Force a normal branch `GrantTrip` error and prove `FAILED`, no success reply, and no committed partial state. **GREEN:** propagate only the normal-branch error from existing `Database.GrantTrip`; do not swallow it or redesign its transaction.
6. **RED — registration negative and no agent widening.** In the baseline engine, prove neither alias is registered when the bundle lacks `Security.Authorization`; prove the agent command/tool surfaces still reject or omit `grant` and `access` with no repository call. **GREEN:** narrow only registrar capability checks; do not add an agent tool, an allowlist entry, or a transport route.

After every green tracer, run its focused package(s). At the end run:

```sh
go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message
git diff --check
```

## Explicit exclusions

The implementation must not introduce or modify:

* authenticated-principal resolution, identity inference, nickname/trip/hash matching policy, session binding, or active-user resolution;
* a transport protocol, payload field, command context protocol, or dispatch rewrite;
* an agent tool, agent-command allowlist entry, agent capability, or agent-facing `grant`/`access` path;
* authorization roles, role hierarchy, configured-trip behavior, protected-target/escalation policy, or any new authorization policy;
* H2/SQL schema, table/index/constraint, persistence interface, transaction boundary, retry behavior, or error model beyond the command's existing normal-branch propagation;
* broader command registration or work in `authorize`, `register`, `messages`, moderation, proxy/whiskey, or unrelated commands.

## Evidence and QA record

* [OBSERVED] Saturn command behavior (adjacent source checkout): `../../projects/saturn/src/main/java/org/saturn/app/command/impl/admin/AccessUserCommandImpl.java`; inherited `ADMIN` gate and argument parsing: `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`; service behavior: `../../projects/saturn/src/main/java/org/saturn/app/service/impl/AuthorizationServiceImpl.java`.
* [OBSERVED / TEST-BACKED] Zenbot command and focused command tests: `internal/command/identity_commands.go`, `internal/command/identity_commands_test.go`; catalog: `internal/command/registry.go`; registration: `internal/command/dispatch_adapter.go`.
* [OBSERVED / TEST-BACKED] Zenbot shared dispatch and authorization tests: `internal/listener/message/handlers.go`, `internal/listener/message/dispatch_authorization_test.go`, `internal/service/security_service.go`, `internal/repository/h2/authorization.go`, and `internal/repository/h2/authorization_identity_test.go`.
* [BASELINE] Before this document was written, `go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message` and `git diff --check` passed.

This task creates only this handoff document; it does not alter application source, tests, staging, or commits.
