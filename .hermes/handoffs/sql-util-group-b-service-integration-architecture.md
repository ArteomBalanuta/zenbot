# Row #324 Group B — Service Integration Architecture

## Decision summary

**Decision: split the proposed integration; do not wire all three operations as one service slice.**

The accepted Group B repository seam is a valid persistence compatibility boundary, but it is intentionally unwired and does not yet have a safe production authorization bridge. The narrowest safe follow-on is a **service-only, caller-preserving read integration** for Saturn-shaped registered-user and last-message results, with no command behavior change. The delete path must remain blocked until an explicit authorization/capability design is approved. `getRegisteredUsers` belongs with the existing `MailService` owner; `delete` and `lastMessages` belong with `UserService`, but the three operations must not be exposed through the existing public contracts or existing callers by accidental result-shape coercion.

This document is architecture only. No application source was changed.

## Scope and prerequisites

### In scope

- Accepted Group B repository seam from:
  - `internal/repository/sql_util_group_b.go`
  - `internal/repository/h2/sql_util_group_b.go`
- Saturn equivalents:
  - `UserServiceImpl.delete`
  - `UserServiceImpl.lastMessages`
  - `MailServiceImpl.getRegisteredUsers`
- Existing Zenbot service ownership, construction, callers, command exposure, authorization, visibility, transactions, and tests.

### Required prerequisites before implementation

1. **Authorization decision for delete:** choose and approve a service-facing authorization contract. The current repository capability is package-private to `internal/repository/h2`; `service` cannot mint it. The minimum acceptable decision is either:
   - add an explicitly named repository authorization method/capability factory that the service can receive only after the command boundary has authenticated an ADMIN/MODERATOR principal; or
   - change the repository contract to accept a typed authorization evidence value issued by a reviewed authorization owner.
   A service must not manufacture a boolean context value, use `context.Background()` as proof, or expose the existing package-private helper.
2. **Caller decision:** approve whether the next slice is service-only and unwired (recommended), or explicitly authorize command changes. Existing `users`, `mail`, and `messages/lastmessages` visibility and formatting are currently established behavior and must not change implicitly.
3. **Test seam decision:** provide a fake implementation of the Group B repository interface for service unit tests, plus H2 integration coverage for constructor wiring. Do not use the existing rich `IdentityRepository.LastMessages` fake for Saturn-shaped data.

## Observed source and test evidence

### Accepted repository seam

[OBSERVED] `repository.SqlUtilGroupBRepository` declares:

- `DeleteIdentity(context.Context, string, string) (DeleteResult, error)`
- `SaturnRegisteredUsers(context.Context) ([]SaturnRegisteredUser, error)`
- `SaturnLastMessages(context.Context, *string, string, int) ([]SaturnLastMessage, error)`

`DeleteResult` has separate affected-row counts (`TripNamesRows`, `TripRows`, `NameRows`); `SaturnRegisteredUser` is `(Name, Trip)`; `SaturnLastMessage` is `(Name, Message, CreatedOn)`.

[TEST-BACKED] `internal/repository/h2/sql_util_row324_group_b_test.go` verifies exact constants, delete authorization denial/no mutation, links-first atomic deletion and rollback, broad OR link scope, exact parent scope, Saturn registered-user ordering/projection, nullable-name last-message queries, default count 5, exclusion of only `LEFT`/`JOINED`, and separation from Zenbot's existing public-only rich history. The accepted QA handoff records focused, race, full-suite, vet, build, formatting, and whitespace gates as passing.

[OBSERVED] `h2.Database.DeleteIdentity` rejects a context unless it contains the unexported `saturnAuthorizationKey`; `withSaturnAuthorization` and the key are in package `repository/h2`, not package `service`. The operation owns one transaction and returns a zero `DeleteResult` on failure. `SaturnRegisteredUsers` and `SaturnLastMessages` are read-only `*sql.DB` queries and are not authorization-gated by the repository.

### Saturn behavior

[OBSERVED] Saturn `UserService` declares `delete(String name, String trip) -> int`, `deleteByNameOrTrip`, and `lastMessages(String name, String trip, int count) -> List<Message>`. `UserServiceImpl.delete` executes `DELETE_TRIP_NAMES`, `DELETE_TRIP`, and `DELETE_NAME` in one transaction, returns `0` on success and `1` on `SQLException`, and logs the exception. It does not perform authorization itself; the SQL has no authorization predicate. `deleteByNameOrTrip` resolves a unique identity first and returns `1` when no unique identity is found.

[OBSERVED] Saturn `UserServiceImpl.lastMessages` defaults `count <= 0` to 5, binds SQL NULL when `name == null`, binds `trip` and count, and maps rows to the richer Saturn `Message` DTO. Its accepted SQL selects only `name,message,created_on`, excludes only `LEFT` and `JOINED`, orders by `created_on DESC`, and has no public-visibility predicate or id tie-break.

[OBSERVED] Saturn `MailServiceImpl.getRegisteredUsers` executes `SELECT_NAME_TRIP_REGISTERED`, appends `name + " " + trip + "\\n"`, and returns a string. It catches SQL errors, logs them, and returns an empty string. The Saturn `MailService` interface does not declare this method even though `MailServiceImpl` exposes it publicly. No dedicated Saturn test for `getRegisteredUsers` or delete was found. `H2CommandPersistenceCompatibilityTest` only asserts that Saturn `lastMessages("Alice", "trip-a", 1)` returns one row; `UserServiceImplTest` covers an unrelated recent-alias query.

### Zenbot service ownership and construction

[OBSERVED] `internal/service/services.go` currently defines concrete service owners, not service interfaces:

- `UserService{Queries repository.UserQueryRepository, Identity repository.IdentityRepository}`.
- `UserService.RegisteredUsers` delegates to `Queries.RegisteredUsers` (legacy `RegisteredUser{Trip,Name}`).
- `UserService.LastMessages` delegates to `Identity.LastMessages` (legacy rich `model.Message`, including Zenbot's public-only semantics).
- `MailService{DB *sql.DB, Out CommandOutput}` owns mail writes, pending mail, delivery status, and `MailService.RegisteredUsers() string`.

The Group B contract is not a field in either service today.

[OBSERVED] `internal/factory/engine_factory.go:58-68` constructs the bundle only when the repository exposes `SQLDB() *sql.DB`; it type-asserts the legacy `UserQueryRepository` and `IdentityRepository`, then constructs `MailService{DB: db}` and `UserService{Queries: q, Identity: identity}`. The construction point is the exact place to optionally inject a `repository.SqlUtilGroupBRepository`, while retaining nil-safe behavior for repositories that do not implement it and preserving ZOMBIE behavior.

### Existing callers and command exposure

[OBSERVED] `internal/command/users_nicks.go:13-26` sends `Users: ...` using `UserService.RegisteredUsers` and `formatRegisteredUsers`, whose output is the legacy `Trip,Name` shape. This is an established public command and must not be fed `SaturnRegisteredUser` values.

[OBSERVED] `internal/command/mail_notes.go:12-36` calls `MailService.Queue`; on `user not registered`, it calls `MailService.RegisteredUsers()` to build the mail help/error payload. That string path is an existing visibility surface. Replacing it with a raw Saturn result or changing its error behavior would be an unrelated command change.

[OBSERVED] `internal/command/identity_commands.go:149-190` handles `messages`/`lastmessages` as a MODERATOR command, parses a trip and count, clamps count above 30, calls `UserService.LastMessages("", trip, n)`, formats `model.Message` as `name#trip: message`, truncates text to 200 bytes, and escapes output. This command is already wired to the legacy public-only history contract. It must not consume `SaturnLastMessage` rows because those intentionally include WHISPER rows and omit the `PUBLIC` predicate.

[OBSERVED] `internal/command/handlers.go:132-154` dispatches commands only after `ResolveUserMetadata` and `Engine.IsUserAuthorized(author, command.Role)`. `messages` is registered as MODERATOR and `remove`/`delete` is cataloged as MODERATOR in `internal/command/handlers.go:131-134`, but `newCommand` has no concrete remove/delete case; the generic `saturnCommand` currently returns an acknowledgement for `remove`/related placeholders rather than deleting persistence. There is therefore no existing Zenbot delete caller to safely retrofit.

[OBSERVED] `internal/service/security_service.go` delegates persisted authorization to `AuthorizationRepository.IsTripAuthorized`; `internal/repository/h2/authorization.go` resolves role and configured-trip wildcard. This authorizes command execution but does not issue a capability understood by `h2.DeleteIdentity`. `EngineImpl.IsUserAuthorized` is the engine-level command gate and is not a repository transaction/capability boundary.

## Recommended target architecture

### Split A — Registered-user read (narrowest potentially safe owner)

**Owner:** existing `MailService` for a typed Saturn directory read. This is the only operation that may be suitable for the next narrow service-only integration, and only while it remains unwired from public commands.

**Target files:**

- Modify `internal/service/services.go` only to add narrow fields and methods, for example:
  - `MailService.GroupB repository.SqlUtilGroupBRepository`
  - `MailService.SaturnRegisteredUsers(ctx) ([]repository.SaturnRegisteredUser, error)`
- Modify `internal/factory/engine_factory.go:58-68` to type-assert the Group B interface and inject the same repository implementation into the appropriate service owner(s).
- Add focused service tests in `internal/service/services_test.go` (or a new `internal/service/group_b_test.go`) using a fake Group B repository.
- Add constructor-wiring assertions in `internal/factory/engine_factory_test.go` if the existing fixture can expose the bundle without altering runtime behavior.

**Input validation and mapping:**

- Require a non-nil `context.Context`; if nil is considered possible at the service boundary, normalize to `context.Background()` only for read compatibility and document it. Prefer rejecting nil in tests/implementation if repository convention supports it.
- `SaturnRegisteredUsers`: no caller-supplied filter; return the typed slice, preserving `Name,Trip` order from the repository. Do not format to a string inside the service.
- Return repository errors unchanged (or with `%w` context); never convert read failures to an empty successful result. The existing `MailService.RegisteredUsers() string` behavior remains unchanged and is not a compatibility adapter.

**Authorization and visibility:**

- This registered-user method may be service-callable only by an explicitly approved internal caller while unwired. It must not replace `users` formatting or the mail error directory in this slice.
- `SaturnRegisteredUsers` is a broad directory projection. Preserve its typed shape and do not expose it through the existing `users` formatter without an explicit output/visibility approval.

**Transaction ownership:**

- Reads do not open transactions; repository owns query execution and row closure.
- No service-level transaction should wrap this read. The service delegates one operation and returns its typed result.

### Split B — Saturn last-message compatibility (separate visibility/caller decision)

**Potential owner:** existing `UserService`, but not the existing `LastMessages` method.

**Decision:** do not integrate this operation in the same slice as registered-user reads. Although the repository seam is read-only, Saturn semantics omit Zenbot's `PUBLIC` predicate and can return whisper rows. The existing `messages/lastmessages` command is a MODERATOR public-chat output path built around `[]model.Message`; substituting `[]SaturnLastMessage` would cross a security and result-shape boundary.

**Potential target files after explicit approval:** `internal/service/services.go` for a separately named typed method and field, `internal/factory/engine_factory.go:58-68` for injection, and focused service/factory tests. No existing command, listener, agent context, or mail path may call it without a separate visibility review and caller authorization decision.

**Required service semantics:** preserve nullable `name`, validate/define blank `trip` behavior, preserve the repository's `count <= 0` default of 5, avoid silently importing the command's max-30 clamp, return `[]repository.SaturnLastMessage` unchanged in shape, and propagate storage errors. No service transaction is required; the repository owns the read.

**Minimum next decision:** name an authorized internal caller and approve whether whisper rows may be disclosed to that caller. Until then, keep the method unwired.

### Split C — Delete integration (blocked pending authorization design)

**Potential owner:** existing `UserService`, not `MailService`.

**Potential target files after authorization approval:**

- `internal/service/services.go`: add a dedicated delete method taking explicit authorization evidence, not a raw boolean.
- `internal/factory/engine_factory.go`: inject Group B repository and the approved authorization provider/capability issuer.
- `internal/repository/sql_util_group_b.go`: revise the contract only if the approved capability design requires it; preserve the existing `DeleteResult` shape.
- `internal/command/handlers.go` or a dedicated command file: only if a separate authorization/command change is explicitly approved. The current placeholder `remove/delete` path is not a safe caller.
- Focused tests in `internal/service/group_b_test.go`, `internal/command/..._test.go`, and existing H2 Group B tests.

**Required service contract semantics:**

- Validate both `name` and `trip` according to the approved identity policy before calling persistence. At minimum reject blank values for a production delete service; do not reinterpret absent/blank no-op repository compatibility behavior as a successful user-facing delete.
- Require an authenticated principal and an ADMIN/MODERATOR policy decision tied to the target scope. The authorization check must happen before repository mutation and must be represented by typed evidence/capability accepted by the repository.
- Map unauthorized calls to a sentinel/domain error (for example `ErrForbidden`) without invoking the repository. Map validation to a sentinel/domain error (for example `ErrInvalidInput`). Wrap storage failures while preserving `errors.Is`. Do not expose Saturn's integer `0/1` status; return the typed `DeleteResult` so callers can distinguish link, trip, and name effects.
- Let the repository own the transaction and links-first ordering. The service must not issue individual deletes, retry partial statements, or start a competing transaction.
- Preserve Saturn's broad `trip OR name` link-delete scope only after the caller and authorization policy explicitly accept that deleting a name can unlink identities outside the selected trip. This is the highest-risk semantic edge.

**Current blocker:** the only accepted delete authorization mechanism is an unexported H2 package capability. No service owner can legally mint it, and no production command caller exists. Wiring `UserService.DeleteIdentity` now would require either weakening the fail-closed repository boundary or inventing an authorization policy, both explicitly outside accepted Group B scope. The minimum target decision is therefore: approve a service-facing capability issuer/typed evidence contract and nominate the command/authorization owner; otherwise delete remains unwired.

## Result-shape and compatibility boundaries

| Saturn operation | Accepted repository result | Proposed service result | Existing Zenbot result/caller | Decision |
|---|---|---|---|---|
| `delete` | `DeleteResult` with 3 counts | same typed result, after authorization | no concrete delete service/caller; placeholder command only | blocked/split |
| `lastMessages` | `[]SaturnLastMessage{Name,Message,CreatedOn}` | same typed slice | `[]model.Message`, public-only, moderator command | never substitute |
| `getRegisteredUsers` | `[]SaturnRegisteredUser{Name,Trip}` | same typed slice | `MailService.RegisteredUsers() string`; `UserService.RegisteredUsers()` is `Trip,Name` | separate method only |

## Focused implementation tests and QA gates

### Service tests

1. Group B repository absent: bundle construction remains nil-safe and existing services/callers behave unchanged.
2. `UserService.SaturnLastMessages` delegates exact nullable name, trip, and count; returns typed rows without converting visibility or fields.
3. Non-positive last-message count preserves the documented default; repository error is returned and not converted to an empty result.
4. `MailService.SaturnRegisteredUsers` preserves `Name,Trip` ordering and propagates errors.
5. Existing `UserService.LastMessages`, `RegisteredUsers`, and `MailService.RegisteredUsers` tests remain unchanged and continue to prove legacy behavior is not replaced.

### Wiring and command-preservation tests

1. A repository implementing both legacy and Group B interfaces receives both interfaces in the bundle.
2. A legacy-only repository still constructs existing services with no panic and no Group B methods.
3. `users`, `mail` error directory, and `messages/lastmessages` continue using their existing result types and output exactly.
4. No command is added or re-bound by the read-only service slice.
5. Existing command authorization test continues to deny moderator commands for unauthorized authors.

### Delete gates, only after authorization approval

1. Missing principal/evidence: no repository call and `ErrForbidden`.
2. Blank identities: no repository call and `ErrInvalidInput`.
3. Authorized call: exact name/trip passed; typed three-count result returned.
4. Repository failure: error wrapped, no fabricated success; H2 rollback tests remain green.
5. Broad OR link scope and unrelated-row preservation are retained and explicitly surfaced in command-level review.

### Required commands

- `go test ./internal/service ./internal/factory ./internal/command -count=1`
- `go test ./internal/repository/h2 -run 'TestGroupB' -count=1`
- `go test -race ./internal/service ./internal/factory ./internal/command -count=1`
- `go test ./... -count=1`
- `go test -race ./... -count=1`
- `go vet ./...`
- `go build ./...`
- `gofmt -l .` and `git diff --check`

A QA report must also verify that `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, Saturn source, and unrelated dirty/staged/untracked files were not modified.

## Complexity, owner, and non-goals

**Complexity:**

- Read-only service wiring: **low to medium** (roughly 2 service fields/methods, factory injection, fakes, and preservation tests); main risk is visibility/result-shape misuse.
- Delete integration: **medium to high** after authorization is designed; it crosses command authorization, service contract, repository capability, and destructive-scope review.

**Implementation owner:**

- Primary: Zenbot `internal/service` owner for the read-only compatibility methods and `internal/factory` owner for dependency injection.
- Required co-owner before delete: Zenbot authorization/security owner plus command owner; repository owner reviews the capability contract.

**Explicit non-goals:**

- No source changes in this architecture task.
- No Saturn changes.
- No changes to existing `RegisteredUsers`, `LastMessages`, mail-directory, or command output/visibility behavior.
- No new commands, command aliases, listener changes, agent/sql policy changes, providers, transports, remote-room, Whiskey, Group C, row #325, or unrelated production registration.
- No schema or migration changes.
- No public standalone delete primitive.
- No conversion of Saturn-shaped sensitive rows into public `model.Message` history.
- No invented service-wide error taxonomy beyond the minimum delete decision described above.

## Limitations and open decisions

[LIMITATION] Saturn has no focused tests for `delete` or `getRegisteredUsers`; those behaviors are source-observed and repository Group B tests provide the stronger Zenbot persistence evidence. Saturn's service methods catch SQL exceptions and use sentinel-like return behavior, but Zenbot's current service layer generally propagates errors, so blindly copying Saturn error swallowing would be unsafe.

[LIMITATION] The Group B repository tests prove H2 semantics and interface behavior, not production bundle construction or service caller behavior; those are the missing gates this architecture proposes.

[RECOMMENDED] Implement Split A first as typed, unwired service methods. Do not proceed with Split B until the authorization capability decision and destructive-scope approval are recorded.

## Verification record

- Inspected current Zenbot source, tests, repository seam, factory, service bundle, command dispatch, authorization, and dirty-tree state.
- Inspected Saturn guidance plus the Saturn `UserService`, `UserServiceImpl`, `MailService`, `MailServiceImpl`, and available tests.
- Protected documents were not edited.
- This handoff is the only file intentionally created by this task; application source remains untouched.
