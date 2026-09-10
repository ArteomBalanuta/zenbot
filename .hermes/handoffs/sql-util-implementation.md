# Row #324 — SqlUtil implementation handoff

## Gate and attribution

- **Target:** `/Users/ab/workspace/go-projects/zenbot`
- **Saturn source (read-only):** `src/main/java/org/saturn/app/util/SqlUtil.java`
- **Audit row:** #324, `org.saturn.app.util.SqlUtil`
- **Direct source verification:** PASS. The source file was read directly before any target edit; a regex count found exactly **31** `public static final String` declarations. The transcription below is the complete source file, preserving literal SQL, Java text blocks, semicolons, placeholders, and concatenation fragments.
- **Saturn unchanged:** verified with `git -C /Users/ab/workspace/projects/saturn status --short` (no Saturn edits made by this task).
- **Scope:** only existing Zenbot H2/repository contracts were considered. No SqlUtil catalog was added. `internal/agent/sql`, row #325 `Util`, Saturn, and excluded service/listener/command/agent/router/provider/transport/remote-room/Whiskey paths were not modified.

## Exact 31-constant inventory and caller mapping

| # | Exact public constant | Exact caller / Zenbot disposition |
|---:|---|---|
| 1 | `INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES` | `AuthorizationServiceImpl.insert → h2.Database.GrantTrip (insert branch)` |
| 2 | `UPDATE_TRIPS_SET_TYPE_WHERE_TRIP` | `AuthorizationServiceImpl.update → h2.Database.GrantTrip (update branch)` |
| 3 | `DELETE_TRIP_NAMES` | `UserServiceImpl.delete → h2.Database.Register (transactional identity delete; exact target delete path is not exposed)` |
| 4 | `DELETE_TRIP` | `UserServiceImpl.delete → h2.Database.Register (same affected identity path; delete method absent)` |
| 5 | `DELETE_NAME` | `UserServiceImpl.delete → h2.Database.Register (same affected identity path; delete method absent)` |
| 6 | `INSERT_NAMES` | `UserServiceImpl.register → h2.Database.Register` |
| 7 | `INSERT_TRIPS` | `UserServiceImpl.register → h2.Database.Register` |
| 8 | `INSERT_TRIP_NAME` | `UserServiceImpl.register → h2.Database.Register` |
| 9 | `INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES` | `LogRepositoryImpl.logCommand → h2.Database.CommandAudit` |
| 10 | `INSERT_INTO_MESSAGES` | `LogRepositoryImpl.logMessage → h2.Database.MessageAudit` |
| 11 | `INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES` | `MailServiceImpl.orderMessageDelivery → no existing repository method (blocked)` |
| 12 | `GET_TRIP_BY_NICK_REGISTERED_OR_TRIP` | `MailServiceImpl.getTripsByNickOrTrip → no existing repository method (blocked)` |
| 13 | `GET_NICKS_BY_TRIP` | `UserServiceImpl.getNicksByTrip → h2.Database.NicksByTrip` |
| 14 | `SELECT_NAME_TRIP_REGISTERED` | `MailServiceImpl.getRegisteredUsers → h2.Database.RegisteredUsers (shape/order differs: target returns Trip,Name)` |
| 15 | `SELECT_MAIL_BY_NICK_OR_TRIP` | `MailServiceImpl.getMailByTrip → no existing repository method (blocked)` |
| 16 | `UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER` | `MailServiceImpl.updateMailStatus → no existing repository method (blocked)` |
| 17 | `INSERT_INTO_BANNED_USERS_TRIP_NAME_HASH_REASON_CREATED_ON_VALUES` | `ModServiceImpl.shadowBan → no existing repository method (blocked)` |
| 18 | `DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH` | `ModServiceImpl.unshadowBan → no existing repository method (blocked)` |
| 19 | `SELECT_BANNED_USERS` | `ModServiceImpl.getBannedUsers → no existing repository method (blocked)` |
| 20 | `SELECT_ROLE_BY_TRIP` | `AuthorizationServiceImpl.findRoleByTrip → h2.Database.ResolveRole` |
| 21 | `SELECT_LOUNGE_TRIPS` | `UserJoinedListenerImpl.getWhitelistedTrips → no existing repository method (blocked; listener expansion excluded)` |
| 22 | `DELETE_FROM_BANNED_USERS` | `ModServiceImpl.unshadowbanAll → no existing repository method (blocked)` |
| 23 | `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES` | `NoteServiceImpl.save → no existing repository method (blocked)` |
| 24 | `SELECT_NOTES_BY_TRIP` | `NoteServiceImpl.getNotesByTrip → no existing repository method (blocked)` |
| 25 | `DELETE_FROM_NOTES_WHERE_TRIP` | `NoteServiceImpl.clearNotesByTrip → no existing repository method (blocked)` |
| 26 | `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_TRIP` | `no Saturn caller found (blocked; unsafe concatenation fragment)` |
| 27 | `SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_HASH` | `no Saturn caller found (blocked; unsafe concatenation fragment)` |
| 28 | `SELECT_LAST_SEEN` | `UserServiceImpl.lastOnline → no exact existing repository method (blocked; h2.LastMessages is a different result contract)` |
| 29 | `SELECT_SEEN_RECENTLY_AS` | `UserServiceImpl.isSeenRecently → no existing repository method (blocked)` |
| 30 | `SELECT_LAST_N_MESSAGES` | `UserServiceImpl.lastMessages → h2.Database.LastMessages (supported only as existing adapted path; visibility/id ordering differ from exact Saturn SQL)` |
| 31 | `SELECT_SESSION_JOINED` | `UserServiceImpl.setSessionDurationAndJoinedDateTime → no existing repository method (blocked)` |

### Supported disposition

The existing target already contains the smallest reuse-first contracts for the following source operations: trip role lookup/insert/update (`ResolveRole`/`GrantTrip`), identity registration inserts and link creation (`Register`), command/message audit inserts (`CommandAudit`/`MessageAudit`), nick lookup (`NicksByTrip`), registered-user query (`RegisteredUsers`, with an explicitly documented target shape/order difference), and last-N message access (`LastMessages`, with target visibility and deterministic ID tie-breaker semantics). These are existing affected paths, not a new SQL catalog.

`SELECT_LAST_N_MESSAGES` is not claimed as byte-for-byte Saturn parity: the target method deliberately filters public messages and adds `id DESC` tie-breaking. It is recorded as an adapted supported path only because the existing Zenbot repository contract and tests already own that behavior. `SELECT_NAME_TRIP_REGISTERED` similarly has target field order `Trip,Name`; no caller migration was added.

### Blocked disposition

The remaining constants are blocked where there is no existing H2/repository owner, where the exact result/ordering/nullability contract is not represented, where the source caller is outside the authorized existing repository path, or where the source constant is an unsafe SQL-concatenation fragment. No guessed method, alias, or caller was created. In particular, mail, moderation, notes, lounge-listener, recent-seen, last-seen, session-joined, and the two uncalled concatenation fragments remain blocked.

## Exact source transcription

```java
package org.saturn.app.util;

public final class SqlUtil {
  public static final String INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES =
      "INSERT INTO trips(type, trip, created_on) VALUES (?, ?, ?);";
  public static final String UPDATE_TRIPS_SET_TYPE_WHERE_TRIP =
      "UPDATE trips SET type=? WHERE trip=?;";

  public static final String DELETE_TRIP_NAMES =
      """
      DELETE FROM trip_names WHERE trip_id IN (
              SELECT id FROM trips WHERE trip = ?
      ) OR name_id IN (
      SELECT id FROM names WHERE name = ?
      );
      """;

  public static final String DELETE_TRIP =
      """
      DELETE FROM trips WHERE trip = ?;
      """;

  public static final String DELETE_NAME =
      """
      DELETE FROM names WHERE name = ?;
      """;

  public static final String INSERT_NAMES =
      "INSERT INTO names (name, created_on) VALUES (?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))";
  public static final String INSERT_TRIPS =
      "INSERT INTO trips (type, trip, created_on) VALUES (?, ?, DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP))";
  public static final String INSERT_TRIP_NAME =
      "INSERT INTO trip_names (trip_id, name_id) VALUES (?, ?)";
  public static final String
      INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES =
          "INSERT INTO executed_commands"
              + " (trip, command_name, arguments, status, created_on, channel) VALUES (?, ?,"
              + " ?, ?, ?, ?);";
  public static final String INSERT_INTO_MESSAGES =
      "INSERT INTO messages "
          + "(trip, name, hash, message, created_on, channel, visibility) "
          + "VALUES (?, ?, ?, ?, ?, ?, ?);";
  public static final String
      INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES =
          "INSERT INTO mail (owner, receiver, message, status, is_whisper, created_on)"
              + " VALUES (?, ?, ?, ?, ?, ?);";

  public static final String GET_TRIP_BY_NICK_REGISTERED_OR_TRIP =
      """
      SELECT t.trip\s
      FROM trip_names tn\s
      INNER JOIN names n on tn.name_id  = n.id\s
      INNER JOIN trips t on tn.trip_id = t.id\s
      WHERE LOWER(name) = ? OR LOWER(t.trip) = ?;""";

  public static final String GET_NICKS_BY_TRIP =
      """
      SELECT DISTINCT name\s
      FROM messages \s
      WHERE LOWER(trip) = ?;""";

  public static final String SELECT_NAME_TRIP_REGISTERED =
      """
SELECT DISTINCT n.name,t.trip\s
FROM trip_names tn\s
INNER JOIN trips t on tn.trip_id = t.id\s
INNER JOIN names n on tn.name_id = n.id ORDER BY t.trip DESC;
""";
  public static final String SELECT_MAIL_BY_NICK_OR_TRIP =
      "SELECT id, owner, receiver, message, status, is_whisper, created_on FROM mail "
          + "WHERE LOCATE(',' || LOWER(?) || ',', ',' || LOWER(receiver) || ',') > 0 "
          + "AND status = 'PENDING';";
  public static final String UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER =
      "UPDATE mail SET status='DELIVERED' WHERE id = ?";
  public static final String INSERT_INTO_BANNED_USERS_TRIP_NAME_HASH_REASON_CREATED_ON_VALUES =
      "INSERT INTO banned_users(trip,name,hash,reason,created_on) VALUES (?,?,?,?,?);";
  public static final String DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH =
      "DELETE FROM banned_users WHERE name = ? OR trip = ? OR hash = ?;";
  public static final String SELECT_BANNED_USERS =
      "SELECT trip,name,hash,reason FROM banned_users;";
  public static final String SELECT_ROLE_BY_TRIP = "SELECT type FROM trips WHERE trip = ?;";

  /* For now using USER role per every whitelisted ?lounge user */
  public static final String SELECT_LOUNGE_TRIPS = "SELECT trip FROM trips WHERE type = 'USER';";
  public static final String DELETE_FROM_BANNED_USERS = "DELETE FROM banned_users;";
  public static final String INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES =
      "INSERT INTO notes (trip, note, created_on) VALUES (?, ?, ?);";
  public static final String SELECT_NOTES_BY_TRIP = "SELECT * FROM notes WHERE trip = ?";
  public static final String DELETE_FROM_NOTES_WHERE_TRIP = "DELETE FROM notes WHERE trip = ?";
  public static final String SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_TRIP =
      "select distinct hash,name from messages where trip = '";
  public static final String SELECT_DISTINCT_HASH_NAME_FROM_MESSAGES_WHERE_HASH =
      "select distinct hash,name from messages where hash = '";
  public static final String SELECT_LAST_SEEN =
      "SELECT message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not in"
          + " ('LEFT','JOINED')) order by created_on desc limit 1;";

  /* The message timestamp is stored in milliseconds since the Unix epoch. */
  public static final String SELECT_SEEN_RECENTLY_AS =
      "SELECT distinct name FROM messages WHERE (hash = ? or (trip = ? and (trip IS NOT NULL and"
          + " trip != '' and trip != 'null'))) and (message in ('LEFT','JOINED')) and created_on >="
          + " DATEDIFF(MILLISECOND, DATE '1970-01-01', CURRENT_TIMESTAMP) - 900000 limit 5";

  public static final String SELECT_LAST_N_MESSAGES =
      "SELECT name,message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not"
          + " in ('LEFT','JOINED')) order by created_on desc limit ?;";
  public static final String SELECT_SESSION_JOINED =
      "SELECT created_on FROM messages WHERE (name = ? or trip = ?) and message = 'JOINED' order by"
          + " created_on desc limit 1;";
}
```

## Tests and implementation result

- Existing real-H2 repository tests were the RED/GREEN baseline for the supported contracts. Before task-owned edits, `go test ./internal/repository/h2` returned `ok`; no new production implementation was necessary without widening into blocked callers or inventing a repository API.
- The existing tests cover real H2 schema bootstrap, generated identity visibility through the current audit contract, transaction rollback, role insert/update/resolve, registration atomicity, nicks/registered-user result shapes, last-message ordering/limits/visibility, and parameterized quote-containing values.
- **No fabricated RED result is claimed:** no new test was presented as RED when the existing target contract already passed. Adding tests for blocked constants would require inventing unsupported target contracts, contrary to the source-verification gate.

## Changed files

- `.hermes/handoffs/sql-util-implementation.md` (this handoff only).
- No production files were changed for row #324; the existing H2/repository files remain the reuse anchors documented above.

## Required verification commands

The focused H2 baseline was run successfully. Full required commands were run after handoff authoring where feasible; their exact exit status/output is recorded below rather than inferred.
