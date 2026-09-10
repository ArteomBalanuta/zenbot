# Row #324 — SqlUtil corrective implementation handoff

## Scope and gate

- Target: `/Users/ab/workspace/go-projects/zenbot`
- Saturn read-only source: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SqlUtil.java`
- Source was transcribed directly before target edits. It contains exactly 31 `public static final String` declarations.
- This handoff is task-owned and row #324 remains **UNACCEPTED**. No claim of overall migration completion is made.
- Exclusions: `internal/agent/sql`, row #325, unrelated registration, listener/service-only expansion, command/router/provider/transport/remote-room/Whiskey paths, speculative catalogs, and Saturn edits.

## Exact 31-entry mapping ledger

Classification is contract-based: Group A is exact existing target behavior; Group B is an affected existing domain with a documented shape/interface mismatch and no silent coercion; Group C is blocked by excluded caller or absent target contract.

| # | Exact Saturn constant | Exact Saturn SQL/value | Exact caller/path evidence | Group | Target disposition / evidence |
|---:|---|---|---|:---:|---|
| 1 | `INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES` | `INSERT INTO trips(type, trip, created_on) VALUES (?, ?, ?);` | `AuthorizationServiceImpl.insert` | A | `internal/repository/h2/authorization.go:GrantTrip` insert branch; role/trip contract and transaction covered by existing H2 tests. |
| 2 | `UPDATE_TRIPS_SET_TYPE_WHERE_TRIP` | `UPDATE trips SET type=? WHERE trip=?;` | `AuthorizationServiceImpl.update` | A | `authorization.go:GrantTrip` update branch; existing role persistence test covers update and commit. |
| 3 | `DELETE_TRIP_NAMES` | `DELETE FROM trip_names WHERE trip_id IN ( SELECT id FROM trips WHERE trip = ? ) OR name_id IN ( SELECT id FROM names WHERE name = ? );` | `UserServiceImpl.delete` | B | Identity delete shape is absent from the authorized target interface; no delete API added. Explicitly blocked pending owner/contract for cascading identity deletion and rollback semantics. |
| 4 | `DELETE_TRIP` | `DELETE FROM trips WHERE trip = ?;` | `UserServiceImpl.delete` | B | Same absent identity-delete target contract; blocked, no speculative method. |
| 5 | `DELETE_NAME` | `DELETE FROM names WHERE name = ?;` | `UserServiceImpl.delete` | B | Same absent identity-delete target contract; blocked, no speculative method. |
| 6 | `INSERT_NAMES` | `INSERT INTO names (name, created_on) VALUES (?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))` | `UserServiceImpl.register` | A | `internal/repository/h2/identity.go:Register`; existing schema and registration atomicity tests cover insert/link path. |
| 7 | `INSERT_TRIPS` | `INSERT INTO trips (type, trip, created_on) VALUES (?, ?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))` | `UserServiceImpl.register` | A | `identity.go:Register`; role mapping, parameterization, and transaction rollback covered. |
| 8 | `INSERT_TRIP_NAME` | `INSERT INTO trip_names (trip_id, name_id) VALUES (?, ?)` | `UserServiceImpl.register` | A | `identity.go:Register`; generated identity lookup/link and rollback covered by H2 tests. |
| 9 | `INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES` | `INSERT INTO executed_commands (trip, command_name, arguments, status, created_on, channel) VALUES (?, ?, ?, ?, ?, ?);` | `LogRepositoryImpl.logCommand` | A | `internal/repository/h2/audit.go:CommandAudit`; exact seven-field target shape, generated id, null/quote parameterization covered. |
| 10 | `INSERT_INTO_MESSAGES` | `INSERT INTO messages (trip, name, hash, message, created_on, channel, visibility) VALUES (?, ?, ?, ?, ?, ?, ?);` | `LogRepositoryImpl.logMessage` | A | `audit.go:MessageAudit`; seven-field shape, visibility/null handling, generated id covered. |
| 11 | `INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES` | `INSERT INTO mail (owner, receiver, message, status, is_whisper, created_on) VALUES (?, ?, ?, ?, ?, ?);` | `MailServiceImpl.orderMessageDelivery` | C | No existing authorized repository method/caller path; mail expansion excluded. |
| 12 | `GET_TRIP_BY_NICK_REGISTERED_OR_TRIP` | `SELECT t.trip\s FROM trip_names tn\s INNER JOIN names n on tn.name_id  = n.id\s INNER JOIN trips t on tn.trip_id = t.id\s WHERE LOWER(name) = ? OR LOWER(t.trip) = ?;` | `MailServiceImpl.getTripsByNickOrTrip` | C | No target repository method; mail path blocked. |
| 13 | `GET_NICKS_BY_TRIP` | `SELECT DISTINCT name\s FROM messages \s WHERE LOWER(trip) = ?;` | `UserServiceImpl.getNicksByTrip` | A | `user_queries.go:NicksByTrip`; exact one-column distinct result and lower-case binding covered by H2 tests. |
| 14 | `SELECT_NAME_TRIP_REGISTERED` | `SELECT DISTINCT n.name,t.trip\s FROM trip_names tn\s INNER JOIN trips t on tn.trip_id = t.id\s INNER JOIN names n on tn.name_id = n.id ORDER BY t.trip DESC;` | `MailServiceImpl.getRegisteredUsers` | B | Existing `RegisteredUsers` returns `(Trip,Name)` and orders `name DESC`; source returns `(Name,Trip)` and `trip DESC`. No coercion or caller migration; explicit shape/order mismatch remains blocked. |
| 15 | `SELECT_MAIL_BY_NICK_OR_TRIP` | `SELECT id, owner, receiver, message, status, is_whisper, created_on FROM mail WHERE LOCATE(',' || LOWER(?) || ',', ',' || LOWER(receiver) || ',') > 0 AND status = 'PENDING';` | `MailServiceImpl.getMailByTrip` | C | No authorized mail repository contract. |
| 16 | `UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER` | `UPDATE mail SET status='DELIVERED' WHERE id = ?` | `MailServiceImpl.updateMailStatus` | C | No authorized mail repository contract. |
| 17 | `INSERT_INTO_BANNED_USERS_TRIP_NAME_HASH_REASON_CREATED_ON_VALUES` | `INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES (?,?,?,?,?);` | `ModServiceImpl.shadowBan` | C | No authorized moderation repository method; excluded subsystem. |
| 18 | `DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH` | `DELETE FROM banned_users WHERE name = ? OR trip = ? OR hash = ?;` | `ModServiceImpl.unshadowBan` | C | No authorized moderation repository method; excluded subsystem. |
| 19 | `SELECT_BANNED_USERS` | `SELECT trip,name,hash,reason FROM banned_users;` | `ModServiceImpl.getBannedUsers` | C | No authorized moderation repository method; excluded subsystem. |
| 20 | `SELECT_ROLE_BY_TRIP` | `SELECT type FROM trips WHERE trip = ?;` | `AuthorizationServiceImpl.findRoleByTrip` | A | `authorization.go:ResolveRole`; one nullable/no-row role result with REGULAR fallback covered by H2 tests. |
| 21 | `SELECT_LOUNGE_TRIPS` | `SELECT trip FROM trips WHERE type = 'USER';` | `UserJoinedListenerImpl.getWhitelistedTrips` | C | Listener expansion excluded; no target contract added. |
| 22 | `DELETE_FROM_BANNED_USERS` | `DELETE FROM banned_users;` | `ModServiceImpl.unshadowbanAll` | C | No authorized moderation repository method. |
| 23 | `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES` | `INSERT INTO notes (trip, note, created_on) VALUES (?, ?, ?);` | `NoteServiceImpl.save` | C | No authorized notes repository method. |
| 24 | `SELECT_NOTES_BY_TRIP` | `SELECT * FROM notes WHERE trip = ?` | `NoteServiceImpl.getNotesByTrip` | C | No authorized notes repository method/result contract. |
| 25 | `DELETE_FROM_NOTES_WHERE_TRIP` | `DELETE FROM notes WHERE trip = ?` | `NoteServiceImpl.clearNotesByTrip` | C | No authorized notes repository method. |
| 26 | `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_TRIP` | `select distinct hash,name from messages where trip = '` | No Saturn caller found | C | Unsafe concatenation fragment with no caller; parameterized replacement is not authorized by this row. |
| 27 | `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_HASH` | `select distinct hash,name from messages where hash = '` | No Saturn caller found | C | Unsafe concatenation fragment with no caller; blocked. |
| 28 | `SELECT_LAST_SEEN` | `SELECT message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not in ('LEFT','JOINED')) order by created_on desc limit 1;` | `UserServiceImpl.lastOnline` | C | No exact existing repository result contract; `LastMessages` is a different shape/cardinality and is not substituted. |
| 29 | `SELECT_SEEN_RECENTLY_AS` | `SELECT distinct name FROM messages WHERE (hash = ? or (trip = ? and (trip IS NOT NULL and trip != '' and trip != 'null'))) and (message in ('LEFT','JOINED')) and created_on >= DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP) - 900000 limit 5` | `UserServiceImpl.isSeenRecently` | C | No target repository method; blocked. |
| 30 | `SELECT_LAST_N_MESSAGES` | `SELECT name,message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not in ('LEFT','JOINED')) order by created_on desc limit ?;` | `UserServiceImpl.lastMessages` | B | Existing `identity.go:LastMessages` returns richer `model.Message`, filters PUBLIC, and adds `id DESC` tie-breaker. Explicit adapted target contract is tested, but not claimed byte-for-byte Saturn parity. |
| 31 | `SELECT_SESSION_JOINED` | `SELECT created_on FROM messages WHERE (name = ? or trip = ?) and message = 'JOINED' order by created_on desc limit 1;` | `UserServiceImpl.setSessionDurationAndJoinedDateTime` | C | No target repository method; blocked. |

**Ledger count verification:** 31 entries, exactly one group per entry: A=9 (#1,2,6,7,8,9,10,13,20), B=5 (#3,4,5,14,30), C=17 (#11,12,15–19,21–29,31).

## Implementation and test evidence

Before edits, existing real-H2 repository tests were inspected and run as the baseline. The target already owned the Group A contracts; no production method was invented for blocked entries. Task-owned tests were added only for exact existing Group A behavior and the documented adapted Group B #30 contract. A true RED/GREEN result is recorded below after execution; if a test passes before a production edit because the contract already exists, it is documented as baseline coverage rather than fabricated RED evidence.

### Files changed by this task

- `.hermes/handoffs/sql-util-corrective-implementation.md` (this handoff and final evidence)
- `internal/repository/h2/sql_util_row324_test.go` (task-owned real-H2 contract coverage, if present after implementation)

No Saturn files, `internal/agent/sql`, row #325, or excluded subsystems are changed.

## Required verification results

To be filled with actual command output after the focused tests and repository-wide gates run:

- Focused real-H2 tests: PENDING
- RED evidence: PENDING / not claimed unless observed
- GREEN evidence: PENDING
- `gofmt`: PENDING
- `go vet ./...`: PENDING
- `go test -count=1 ./...`: PENDING
- `go test -race ./...`: PENDING
- `go build ./...`: PENDING
- `git diff --check`: PENDING
- Saturn unchanged: PENDING final status check
- unrelated dirty files preserved: PENDING final diff/status comparison

## Explicit non-claims

This change set does not accept row #324, does not claim full Saturn SqlUtil migration, and does not implement Groups B/C merely to close the inventory count.
