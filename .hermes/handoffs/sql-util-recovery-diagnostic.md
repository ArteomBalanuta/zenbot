# Row #324 SqlUtil Recovery Diagnostic

## Verdict and scope

**Diagnostic only. Row #324 remains unaccepted.** No Zenbot application code, tests, or Saturn files were modified by this diagnostic. The authorized boundary is existing Zenbot H2/repository paths only; row #325, `internal/agent/sql`, and excluded service/listener/command/router/provider/transport/remote-room/Whiskey work remain out of scope.

The smallest defensible Group A subset is **nine constants**: the two trip-role mutations, the three registration mutations, the two audit inserts, the nicks query, and the role query. These can reuse existing Zenbot contracts without inventing a repository method or silently changing a result shape. Existing tests prove useful behavior, but they are not task-owned row-#324 tests; therefore this subset is implementable next, not accepted now.

## Evidence locations

- Saturn source of truth: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SqlUtil.java` (31 `public static final String` declarations; direct count verified with a source regex).
- Saturn authorization callers: `src/main/java/org/saturn/app/service/impl/AuthorizationServiceImpl.java`, methods `grant`, `resolveRole`, private `findRoleByTrip`, `insert`, and `update` (constants used at lines 3, 114, 133, and 149 in the checked source).
- Saturn identity/history callers: `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`, methods `isSeenRecently`, `lastOnline`, `lastMessages`, `registerNameByTrip`, `getNicksByTrip`, and `setSessionDurationAndJoinedDateTime`; registration insert fragments are in the same file.
- Saturn audit caller: `src/main/java/org/saturn/app/service/impl/LogRepositoryImpl.java`, methods `logCommand` and `logMessage`.
- Zenbot repository contracts: `internal/repository/repository.go` (`AuthorizationRepository`, `AuditRepository`) and `internal/repository/user_queries.go` (`IdentityRepository`, `UserQueryRepository`).
- Zenbot H2 implementations: `internal/repository/h2/authorization.go`, `identity.go`, `audit.go`, and `user_queries.go`.
- Zenbot H2 schema: `internal/repository/h2/schema-h2.sql`.
- Existing baseline tests: `internal/repository/h2/authorization_identity_test.go`, `audit_test.go`, `identity_test.go`, and `user_queries_test.go`.

## Group A: exact existing target contracts

The following nine source constants are the bounded implementation target. The names below are transcribed exactly from Saturn; the caller and Zenbot method are exact symbols, not inferred aliases.

| Saturn constant | Saturn caller/method | Existing Zenbot contract and method | Why bounded Group A |
|---|---|---|---|
| `INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES` | `AuthorizationServiceImpl.insert` | `internal/repository/h2/authorization.go:GrantTrip` insert branch; `repository.AuthorizationRepository.GrantTrip` | Trip/role insertion with role, trip, timestamp and atomic commit already exists. |
| `UPDATE_TRIPS_SET_TYPE_WHERE_TRIP` | `AuthorizationServiceImpl.update` | `authorization.go:GrantTrip` update branch; `AuthorizationRepository.GrantTrip` | Existing branch updates the persisted trip role and commits transactionally. |
| `INSERT_NAMES` | `UserServiceImpl.registerNameByTrip` / `register` registration flow | `internal/repository/h2/identity.go:Register`, `RegisterNameByTrip`; `repository.IdentityRepository` | Existing name insert/link path is atomic and has a target contract. |
| `INSERT_TRIPS` | `UserServiceImpl.registerTripByName` / `register` registration flow | `identity.go:Register`, `RegisterTripByName`; `IdentityRepository` | Existing trip insert/link path has role mapping, generated identity lookup, and rollback behavior. |
| `INSERT_TRIP_NAME` | `UserServiceImpl.registerNameByTrip` and `registerTripByName` registration flows | `identity.go:Register`, `RegisterNameByTrip`, `RegisterTripByName`; `IdentityRepository` | Existing link creation is part of the atomic identity contract. |
| `INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES` | `LogRepositoryImpl.logCommand` | `internal/repository/h2/audit.go:CommandAudit`; `repository.AuditRepository.CommandAudit` and compatibility `Database.LogCommand` | Seven persisted fields and the existing audit result contract are present. |
| `INSERT_INTO_MESSAGES` | `LogRepositoryImpl.logMessage` | `audit.go:MessageAudit`; `repository.AuditRepository.MessageAudit` and compatibility `Database.LogMessage` | Seven persisted fields, visibility handling, and audit persistence are represented. |
| `GET_NICKS_BY_TRIP` | `UserServiceImpl.getNicksByTrip` | `internal/repository/h2/user_queries.go:NicksByTrip`; `repository.UserQueryRepository.NicksByTrip` | One-column distinct-name result and lower-case trip binding are represented. |
| `SELECT_ROLE_BY_TRIP` | `AuthorizationServiceImpl.findRoleByTrip`, reached by `grant`/`resolveRole` | `authorization.go:ResolveRole`; `repository.AuthorizationRepository.ResolveRole` | Persisted role lookup, no-row fallback, and role conversion are represented. |

This is a reuse-first subset, not a claim that Zenbot contains Saturn's Java constants or byte-for-byte SQL. The target contract is the acceptance unit. In particular, `GrantTrip` uses transaction control and updates by persisted id; `MessageAudit` validates/defaults visibility; and the Go APIs return errors/IDs where Saturn methods swallow SQL errors or return void. Those are target-contract semantics that must be tested, not silently described as literal Saturn parity.

## Required task-owned tests before accepting Group A

Add a new, clearly task-owned file under `internal/repository/h2/` (recommended name: `sql_util_row324_test.go`) without changing unrelated tests. Tests must use the real H2 helper (`openTestDB`) and assert behavior, not merely that a statement executes.

Required coverage:

1. `INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES` and `UPDATE_TRIPS_SET_TYPE_WHERE_TRIP`: insert every supported `model.Role`, resolve it, update it, verify commit visibility, invalid role/blank trip errors, and rollback/no partial row on failure.
2. `INSERT_NAMES`, `INSERT_TRIPS`, `INSERT_TRIP_NAME`: verify generated IDs are linked, registration-by-trip and registration-by-name preserve the expected relationship, duplicate input leaves no partial name/trip/link rows, and quote/null/empty inputs follow the existing contract.
3. `INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES`: verify all six bound values plus channel, generated-ID propagation, quote-containing arguments, and error behavior. Do not assume `MAX(id)` is concurrency-safe; if the existing contract retains that implementation, the test must document the bounded single-connection semantics rather than claim general concurrent key correctness.
4. `INSERT_INTO_MESSAGES`: verify all seven fields, default PUBLIC visibility, WHISPER visibility, invalid visibility rejection, nullable fields, quote-containing text, generated-ID propagation, and failed writes do not create rows.
5. `GET_NICKS_BY_TRIP`: verify case-insensitive trip lookup, DISTINCT behavior, empty results, and error propagation/result closure.
6. `SELECT_ROLE_BY_TRIP`: verify existing role round-trip, blank and unknown trip REGULAR fallback, invalid persisted role failure, and query error propagation.
7. Cross-cutting persistence: where the method is transactional, assert commit visibility and rollback; where it is a single audit insert, assert no false success or fabricated ID. Keep assertions on returned shape, cardinality, null semantics, and deterministic behavior promised by the Go contract.

A test that only repeats an existing baseline assertion is not enough: the task-owned file must identify the row-#324 constants and test the relevant boundary explicitly. Do not add tests for blocked constants by inventing repository APIs.

## Remaining ledger partition and blockers

The other 22 constants must stay documented, not forced into implementation. The exact Saturn names and disposition are:

- **Group B (five shape/interface mismatches):** `DELETE_TRIP_NAMES`, `DELETE_TRIP`, `DELETE_NAME` (caller: `UserServiceImpl.delete`; no authorized identity-delete method, and the three-statement/cascade transaction contract is absent); `SELECT_NAME_TRIP_REGISTERED` (caller: `MailServiceImpl.getRegisteredUsers`; existing `RegisteredUsers` returns `Trip,Name` and orders by `name DESC`, while Saturn reads `Name,Trip` and orders by `trip DESC`); `SELECT_LAST_N_MESSAGES` (caller: `UserServiceImpl.lastMessages`; existing `LastMessages` returns richer `model.Message`, filters PUBLIC, and adds `id DESC` tie-breaking). Group B requires an explicit interface/result/transaction decision and task-owned tests before implementation; no silent coercion.
- **Group C (17 blocked):** `INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES`, `GET_TRIP_BY_NICK_REGISTERED_OR_TRIP`, `SELECT_MAIL_BY_NICK_OR_TRIP`, and `UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER` (mail service path); `INSERT_INTO_BANNED_USERS_TRIP_NAME_HASH_REASON_CREATED_ON_VALUES`, `DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH`, `SELECT_BANNED_USERS`, and `DELETE_FROM_BANNED_USERS` (moderation path); `SELECT_LOUNGE_TRIPS` (listener-only `UserJoinedListenerImpl.getWhitelistedTrips`); `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`, `SELECT_NOTES_BY_TRIP`, and `DELETE_FROM_NOTES_WHERE_TRIP` (notes path); `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_TRIP` and `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_HASH` (uncalled unsafe SQL-concatenation fragments); `SELECT_LAST_SEEN`, `SELECT_SEEN_RECENTLY_AS`, and `SELECT_SESSION_JOINED` (history/session methods with no exact authorized target repository contract).

Direct Saturn caller evidence for the blocked service paths is in `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java`, `ModServiceImpl.java`, and `NoteServiceImpl.java`; the user history callers are in `UserServiceImpl.java`. Group C requires the owning subsystem/contract and scope authorization before any method, SQL, adapter, or test is created.

## Why the prior implementation could not safely proceed

1. The prior worker performed broad inspection and then stopped without creating the required task-owned ledger, test file, or source changes. The only claimed artifact was a handoff document.
2. The earlier implementation handoff says “task-owned tests ... if present” and reports existing tests as evidence. The checkout has no `internal/repository/h2/sql_util_row324_test.go`; the tests found are pre-existing baseline files. Thus the required ownership gate was not met.
3. The earlier handoff's supported list mixed exact Group A items with two non-exact adaptations: `SELECT_NAME_TRIP_REGISTERED` and `SELECT_LAST_N_MESSAGES`. The corrective architecture explicitly requires Group B for result/order/shape mismatch. Treating them as Group A would silently accept changed semantics.
4. The earlier material relied on schema similarity and overlapping behavior while the architecture requires exact contract checks for parameters, generated keys, nullability, cardinality, ordering, transaction boundaries, and failure semantics.
5. Several Saturn constants have callers in mail, moderation, notes, listener, or history/session code for which the authorized Zenbot repository interface has no owner. Proceeding would require speculative methods or widening excluded scope. The two uncalled concatenation fragments are additionally unsafe to parameterize under this row without a caller/contract decision.
6. No all-31 mapping ledger with one-and-only-one A/B/C disposition and task-owned evidence was delivered. Consequently no safe acceptance or “row accepted” claim was possible, even though focused existing H2 tests may pass.

## Bounded implementation recommendation

Implement only the nine Group A constants above, reusing the existing methods in `authorization.go`, `identity.go`, `audit.go`, and `user_queries.go`. First add the task-owned real-H2 tests listed above; then run them and the repository gates. Do not add a SqlUtil catalog, copy Saturn SQL into production, change `internal/agent/sql`, or touch Group B/C callers. Keep a 31-entry source-transcribed ledger in the implementation handoff, with the five Group B and 17 Group C dispositions retained. Re-run independent QA against the task-owned tests and ledger. The row remains unaccepted until those tests and the A/B/C documentation gates pass.

## Verification performed for this diagnostic

- Direct Saturn constant count: **31**.
- Saturn working tree status checked: pre-existing Saturn modifications were present; this task made no Saturn changes. No claim of a clean Saturn tree is made.
- Existing focused H2 package tests were inspected as baseline evidence; they do not substitute for the required task-owned row-#324 tests.
- This file was written as the only artifact by this diagnostic; application source and Saturn were not edited.
