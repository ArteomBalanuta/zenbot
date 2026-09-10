# Identity slice architecture handoff

Scope: `register/reg`, `authorize/auth`, `grant/access`, and `messages/lastmessages`. This is a source-grounded handoff; no application code was changed.

## 1. Runtime surface and aliases

- `[OBSERVED]` `RegisterAll` registers canonical `register` with aliases `reg`, `register`, role `MODERATOR`; `authorize` has `authorize`, `auth`, role `MODERATOR`; `access` has `grant`, `access`, role `ADMIN`; `messages` has `messages`, `lastmessages`, role `MODERATOR` (`internal/command/registry.go`, `RegisterAll`). Alias lookup is case-insensitive in `commandDefinitionFor`; command argument parsing is `ChatMessage.GetArguments()` with the command token removed (`internal/command/handlers.go`, `args`).
- `[OBSERVED]` The concrete constructors are already selected by `newCommand` for all four canonicals (`internal/command/handlers.go`). The migration plan nevertheless says these aliases remain unregistered until concrete implementation and focused tests pass (`MIGRATION_PLAN.md`, current status checkpoint); reconcile that checkpoint with the current target tree before closure.
- `[TEST-BACKED]` The focused target test asserts the alias/canonical/role matrix for all four command families and exercises `reg`, `register`, and `lastmessages` output (`internal/command/identity_commands_test.go`). The captured file appears truncated/incomplete around the test bundle initialization; repair or replace only that focused test as part of implementation, not as evidence of a passing test.

## 2. Command behavior and exact output

### Register / reg

- `[OBSERVED]` Fewer than two arguments replies `Example: <prefix>reg merc g0KY09`, returns `FAILED`, nil error. Otherwise the first two arguments are independently `TrimSpace`d.
- `[OBSERVED]` It checks name and trip registration. Neither registered: calls `UserService.Register(name, trip, REGULAR)` and on success replies `User has been registered successfully, now you can msg him by name: <name>`. Name absent/trip present: calls `RegisterNameByTrip`, replies `New name: <name>, assigned to trip: <trip>`. Name present/trip absent: calls `RegisterTripByName`, replies `New trip: <trip>, assigned to user named: <name>`. Both present: replies `Name <name> and trip <trip> are already registered.` and returns `FAILED`, nil.
- `[OBSERVED]` Service errors return `FAILED` with the error and additionally send `Something went wrong` for the mutating branches. Replies use the command author's name and preserve whisper status through `reply` (`handlers.go`).
- `[SATURN]` Java uses the same branch/output text, but passes raw argument strings to the service and represents service failure as integer code `1`; registration success is code `0` (`RegisterUserCommandImpl.java`, `UserServiceImpl.register`).

### Authorize / auth

- `[OBSERVED]` No argument replies ` example: <prefix>auth cmdTV+` and returns `FAILED`, nil. Otherwise first argument is trimmed, `SecurityService.AuthorizeTrip` is called, and the command replies ` authorized trip: <trip>` with `SUCCESSFUL` (`identity_commands.go`).
- `[OBSERVED]` `AuthorizeTrip` trims and ignores blank trips. With an authorization repository it calls `GrantTrip(context.Background(), trip, ADMIN)`; on repository error it logs and returns without appending to in-memory `AdminTrips`. Without a repository it appends the trip unless exact-string duplicate (`security_service.go`).
- `[SATURN]` The Java command uses the first raw argument and delegates to `modService.auth`, then emits the same success text. Its command authorization is `MODERATOR` (`AuthorizeTripCommandImpl.java`). The exact `modService.auth` implementation is outside this bounded source set; target persistence is therefore the explicit target behavior to verify.

### Grant / access

- `[OBSERVED]` Requires exactly two command arguments and a nonblank invoking message trip; otherwise replies the literal `\\n Set your trip first. Example: <prefix>grant 8Wotmg ADMIN` and returns `FAILED`, nil. Role parsing trims and uppercases, accepting exactly `ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, `PEST`; unknown roles fail silently (`accessCommand`, `parseRole`).
- `[OBSERVED]` Single target calls `Authorization.GrantTrip(ctx, target, parsedRole)` and replies `\\n Granted new Role: <ROLE> to trip: <target>`. A comma-containing target is split; each trimmed trip is granted `USER` (not the parsed role), then the reply is `\\n Granted new Roles: <ROLE> to trips: <Go slice formatting>`. The multi-target path ignores individual grant errors and still returns success.
- `[SATURN]` Java requires the same shape and sender trip, but `Role.valueOf` is case-sensitive and does not trim the role. Single target grants the parsed role. Multi-target uses `targetTrip.split(",")` without trimming, grants every target `Role.USER`, and formats Java `List.toString()` (including its spacing). It emits the same literal escaped-newline output (`AccessUserCommandImpl.java`).

### Messages / lastmessages

- `[OBSERVED]` Fewer than two arguments or non-integer count replies `Example: <prefix>lastmessages g0KY09 3`, returns `FAILED`, nil. Counts above 30 first reply `Retrieving at max 30 messages! ` and are clamped to 30. The service is called as `LastMessages("", trimmedTrip, count)`.
- `[OBSERVED]` Each result is rendered as `\n<name>#<trip>: <message>\n`; messages longer than 200 Go bytes are truncated to the first 200 bytes plus `...`. The complete string is passed through `escapeJava` (Go `strconv.Quote` body), so newlines and quotes are escaped before sending. Returns `SUCCESSFUL` unless the service errors (`identity_commands.go`).
- `[SATURN]` Java follows the same usage, >30 warning/clamp, `\n...\n` rendering, 200-character prefix plus `...`, and Apache `StringEscapeUtils.escapeJava`. It passes null name and the requested trip to `lastMessages`; service SQL failures are logged and result in an empty list (`LastMessagesCommandImpl.java`, `UserServiceImpl.lastMessages`).

## 3. Roles, authorization, and parsing

- `[OBSERVED]` Persisted role names are exactly the six schema values: `ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, `PEST`. Go maps all six in both directions and rejects an invalid role on persistence; an unknown/blank trip resolves to `REGULAR` (`authorization.go`).
- `[OBSERVED]` Configured trips are checked first, case-insensitively after trim; `x` is a wildcard. Otherwise Go authorizes with `role <= required`, reflecting the target's strongest-first numeric ordering comment (`authorization.go`, `security_service.go`).
- `[SATURN]` Java config allowlisting uses exact `contains` for trip and `x`; DB authorization uses `userRole.getValue() >= minRequiredRole.getValue()`. Confirm the model enum numeric convention with the target model before changing either comparison; parity depends on the ordering being equivalent.
- `[RISK]` `grant/access` checks the invoker trip only for presence in the command implementation; actual command authorization is registry metadata/external dispatch behavior and is not demonstrated by the focused test. Add an end-to-end authorization test, including config wildcard, persisted role threshold, nil user, blank trip, and repository failure.

## 4. Persistence and SQL semantics

- `[OBSERVED]` Name/trip existence queries are `COUNT(*)` with `LOWER(column)=LOWER($1)` and trimmed input. `Register` trims, validates nonblank values, inserts name and trip with millisecond timestamps, reads IDs inside one `WithTx` transaction, then inserts `trip_names`; trip type comes from the role. `RegisterNameByTrip` resolves trip case-insensitively, inserts the name, then links it. `RegisterTripByName` resolves name case-insensitively, inserts a `REGULAR` trip, then links it (`internal/repository/h2/identity.go`).
- `[SATURN]` Java registration uses generated keys from `INSERT_NAMES`, `INSERT_TRIPS`, and `INSERT_TRIP_NAME`, all inside `runInTransaction`; service methods return `0`/`1` for success/failure (full registration) while the by-name/by-trip void methods catch/log SQL failures. Java link-selection SQL uses exact `name`/`trip` matching, unlike the target's case-insensitive lookup in the corresponding Go methods (`UserServiceImpl.java`).
- `[OBSERVED]` `GrantTrip` trims and validates trip, begins a context transaction, updates an existing exact-match trip or inserts a new trip with role and millisecond timestamp, and commits; deferred rollback is attempted when the local `err` is nonnil. `ResolveRole` exact-matches trip and converts the stored type; missing rows return `REGULAR`. `IsTripAuthorized` checks configured trips then persisted role (`authorization.go`).
- `[SATURN]` Java `grant` first queries exact trip. Existing rows are updated; missing rows are inserted. Insert/update catch SQL exceptions and only log, with no transaction around the operation (`AuthorizationServiceImpl.java`).
- `[OBSERVED]` `LastMessages` defaults nonpositive counts to 5 and runs dynamic SQL: `(name=$1 OR trip=$2)`, excludes messages `LEFT` and `JOINED`, orders `created_on DESC, id DESC`, and applies the interpolated limit. Target scans nullable trip/name/hash/message/channel into `model.Message` and returns query/scan/rows errors (`identity.go`).
- `[SATURN]` Java binds nullable name, trip, and count parameters to `SELECT_LAST_N_MESSAGES`; it constructs each DTO with the requested trip rather than the row trip, closes resources, logs SQL errors, and returns whatever rows were accumulated (or empty) (`UserServiceImpl.lastMessages`).
- `[OBSERVED GAP]` The target last-message query shown here has no `visibility` predicate. The migration plan explicitly requires PUBLIC/WHISPER filtering, room/name/trip scope, and `(created_on,id)` ordering for history/agent reads. Enforce that policy in the named H2 repository method and cover it with real-H2 tests; do not weaken the schema CHECK (`schema-h2.sql` source has `visibility IN ('PUBLIC','WHISPER')`).

## 5. Target gaps and exact implementation surface

Primary files for this slice:

1. `internal/command/identity_commands.go` — parity corrections for argument handling, role parsing, multi-target error semantics, count/format behavior, and output escaping.
2. `internal/command/handlers.go` — concrete dispatch integration and shared argument/reply semantics only if required by the parity fix.
3. `internal/command/registry.go` — preserve/verify alias and role definitions and runtime registration.
4. `internal/command/identity_commands_test.go` — repair the focused test and add command/output/error matrix.
5. `internal/service/services.go` — keep the user-service identity façade aligned with repository contracts.
6. `internal/service/security_service.go` — verify authorize persistence/error and configured-trip/role fallback semantics.
7. `internal/repository/repository.go` — keep `AuthorizationRepository` and identity contracts explicit; add context/error semantics only if required by the chosen parity design.
8. `internal/repository/h2/identity.go` — registration/last-message SQL, visibility policy, ordering, limits, and transaction/error behavior.
9. `internal/repository/h2/authorization.go` — role persistence, grant transaction behavior, case semantics, and authorization threshold.

If the visibility policy is not already identical in the target schema resources, the migration plan names these additional exact resources to reconcile: `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql` (`MIGRATION_PLAN.md`, persistence obligations). They were outside this bounded target read and must be inspected before editing.

## 6. Focused verification plan

- Alias/catalog table: every alias resolves case-insensitively to the expected canonical and role; verify `newCommand` returns concrete implementations.
- Register matrix: missing args, both-new, name-only-new, trip-only-new, both-existing, whitespace, case-insensitive duplicate checks, blank values, repository error, and rollback after each failed insert/link.
- Authorize matrix: missing/blank input, trim, duplicate, repository insert/update, repository error, configured admin list, and exact output.
- Grant matrix: missing sender trip, wrong arity, all six roles, lowercase/whitespace role, invalid role, single target, comma targets, target trimming, per-target error, and exact Java-compatible List/slice output.
- Messages matrix: missing/non-numeric/negative/zero counts, 30 clamp warning ordering, >200-byte content, quotes/newlines, JOINED/LEFT exclusion, deterministic `(created_on,id)` order, visibility filtering, and service/query error propagation.
- Real-H2 transaction tests: generated identity/link rows, commit and rollback for each registration path, grant insert/update, duplicate constraints, and role CHECK values. Run the migration-plan gates relevant to the slice: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`.

## 7. Risks and decisions to preserve

- `[RISK]` Java and Go differ in role case/trim behavior, multi-target trimming, and error swallowing; changing these silently changes observable command success/output.
- `[RISK]` Java uses generated keys and explicit connection transactions; Go currently re-queries IDs and relies on `WithTx`. Verify atomicity and H2 behavior rather than assuming equivalence.
- `[RISK]` Go's dynamic SQL limit is safe only because count is parsed as an integer, but the repository must retain the cap/default semantics and deterministic tie ordering.
- `[RISK]` Missing visibility filtering is a security boundary, not merely a query optimization; history must not expose WHISPER rows outside the permitted scope.
- `[RISK]` Do not infer source `modService.auth` details or uninspected target schema state from filenames. Resolve those dependencies before implementation, while preserving unrelated dirty-worktree changes.
