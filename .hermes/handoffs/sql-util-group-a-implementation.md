# SqlUtil Row #324 Group A Implementation Handoff

## Scope and status

This handoff covers only the nine recovery-diagnostic Group A contracts, reusing existing Zenbot H2/repository methods. Row #324 remains **unaccepted** and overall migration completion is not claimed.

## Task-owned implementation ledger / test plan

Exact Saturn source: `src/main/java/org/saturn/app/util/SqlUtil.java` in the read-only Saturn checkout.

| Group A Saturn constant (exact source name) | Exact Saturn SQL (transcribed) | Saturn caller/method | Existing Zenbot target | Task-owned coverage |
|---|---|---|---|---|
| `INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES` | `INSERT INTO trips(type, trip, created_on) VALUES (?, ?, ?);` | `AuthorizationServiceImpl.insert` | `h2.Database.GrantTrip` insert branch | `TestGroupA_TripRoleContracts` |
| `UPDATE_TRIPS_SET_TYPE_WHERE_TRIP` | `UPDATE trips SET type=? WHERE trip=?;` | `AuthorizationServiceImpl.update` | `h2.Database.GrantTrip` update branch | `TestGroupA_TripRoleContracts` |
| `INSERT_NAMES` | `INSERT INTO names (name, created_on) VALUES (?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))` | `UserServiceImpl.registerNameByTrip` / `register` | `h2.Database.Register`, `RegisterNameByTrip` | `TestGroupA_IdentityRegistrationContracts` |
| `INSERT_TRIPS` | `INSERT INTO trips (type, trip, created_on) VALUES (?, ?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))` | `UserServiceImpl.registerTripByName` / `register` | `h2.Database.Register`, `RegisterTripByName` | `TestGroupA_IdentityRegistrationContracts` |
| `INSERT_TRIP_NAME` | `INSERT INTO trip_names (trip_id, name_id) VALUES (?, ?)` | `UserServiceImpl.registerNameByTrip` / `registerTripByName` | existing atomic link steps in identity methods | `TestGroupA_IdentityRegistrationContracts` |
| `INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES` | `INSERT INTO executed_commands (trip, command_name, arguments, status, created_on, channel) VALUES (?, ?, ?, ?, ?, ?);` | `LogRepositoryImpl.logCommand` | `h2.Database.CommandAudit` | `TestGroupA_CommandAuditContract` |
| `INSERT_INTO_MESSAGES` | `INSERT INTO messages (trip, name, hash, message, created_on, channel, visibility) VALUES (?, ?, ?, ?, ?, ?, ?);` | `LogRepositoryImpl.logMessage` | `h2.Database.MessageAudit` | `TestGroupA_MessageAuditContract` |
| `GET_NICKS_BY_TRIP` | `SELECT DISTINCT name FROM messages WHERE LOWER(trip) = ?;` | `UserServiceImpl.getNicksByTrip` | `h2.Database.NicksByTrip` | `TestGroupA_NicksByTripContract` |
| `SELECT_ROLE_BY_TRIP` | `SELECT type FROM trips WHERE trip = ?;` | `AuthorizationServiceImpl.findRoleByTrip`, via `grant`/`resolveRole` | `h2.Database.ResolveRole` | `TestGroupA_SelectRoleByTripContract` |

### Test plan

- Role insert/update/resolve: every `model.Role`, invalid role, blank trip, commit visibility, rollback/no partial row, blank/unknown resolve fallback, invalid persisted role.
- Identity: generated IDs and links for registration, registration-by-trip and registration-by-name, trimming/normalization, quote values, duplicate atomicity, blank rejection.
- Command audit: all six bound values including channel, generated ID, quote-containing arguments, nullable empty fields, failed write behavior.
- Message audit: all seven fields, generated ID, default PUBLIC, WHISPER, invalid visibility, nullable/empty optional fields, quote-containing text, failed write behavior.
- Nicks: DISTINCT, case-insensitive trip binding, empty results, blank input behavior, query error propagation.
- Role query: round-trip, blank/unknown REGULAR fallback, invalid persisted role, query error propagation.
- Transaction helper: commit visibility and rollback where applicable.

## RED evidence

The first focused run was executed before production edits with:

`go test ./internal/repository/h2 -run '^TestGroupA_' -count=1`

It failed in the new task-owned tests because `RegisterNameByTrip` and `RegisterTripByName` accepted blank inputs (genuine missing validation). Two initial assertions were corrected before implementation because they over-specified existing contracts: whitespace trimming was not part of `NicksByTrip`, and Go's string-only audit records represent nullable columns as empty strings rather than SQL NULL. The corrected RED test retained the genuine blank-identity failure and passed only after the minimal production change.

## GREEN evidence and production changes

Production change was limited to `internal/repository/h2/identity.go`: `RegisterNameByTrip` and `RegisterTripByName` now reject blank name/trip inputs before opening a transaction, matching `Register` and preventing invalid/partial identity writes. No other production file changed.

Task-owned test file:

- `TestGroupA_TripRoleContracts`
- `TestGroupA_IdentityRegistrationContracts`
- `TestGroupA_CommandAuditContract`
- `TestGroupA_MessageAuditContract`
- `TestGroupA_NicksByTripContract`
- `TestGroupA_SelectRoleByTripContract`

GREEN result: `go test ./internal/repository/h2 -run '^TestGroupA_' -count=1` passed.
Focused race result: `go test -race ./internal/repository/h2 -run '^TestGroupA_' -count=1` passed.
Full normal result: `go test ./... -count=1` passed.
Full race result: `go test -race ./... -count=1` passed; macOS linker emitted a warning for `internal/agent/sql.test`, but the command exited 0 and all packages passed.
Additional gates: `go vet ./...`, `go build ./...`, and `git diff --check` all passed.
Saturn verification: `git -C /Users/ab/workspace/projects/saturn status --short -- src/main/java/org/saturn/app/util/SqlUtil.java` was empty; Saturn stayed read-only.

## Remaining 22 constants explicitly blocked

### Group B (5)

- `DELETE_TRIP_NAMES`
- `DELETE_TRIP`
- `DELETE_NAME`
- `SELECT_NAME_TRIP_REGISTERED`
- `SELECT_LAST_N_MESSAGES`

Blocked because deletion has no authorized target method/transaction contract, and the registered-user and last-message target result shape/order differs from Saturn.

### Group C (17)

- `INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES`
- `GET_TRIP_BY_NICK_REGISTERED_OR_TRIP`
- `SELECT_MAIL_BY_NICK_OR_TRIP`
- `UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER`
- `INSERT_INTO_BANNED_USERS_TRIP_NAME_HASH_REASON_CREATED_ON_VALUES`
- `DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH`
- `SELECT_BANNED_USERS`
- `DELETE_FROM_BANNED_USERS`
- `SELECT_LOUNGE_TRIPS`
- `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`
- `SELECT_NOTES_BY_TRIP`
- `DELETE_FROM_NOTES_WHERE_TRIP`
- `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_TRIP`
- `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_HASH`
- `SELECT_LAST_SEEN`
- `SELECT_SEEN_RECENTLY_AS`
- `SELECT_SESSION_JOINED`

Blocked by excluded mail/moderation/lounge/notes/listener paths, unsafe uncalled SQL fragments, or absent exact authorized history/session target contracts.

## Explicit exclusions

No SqlUtil catalog, no duplicated Saturn SQL in production, no Group B/C methods/tests, no row #325, no `internal/agent/sql`, and no unrelated services/listeners/commands/agent/router/provider/transport/remote-room/Whiskey changes. Saturn remains read-only.
