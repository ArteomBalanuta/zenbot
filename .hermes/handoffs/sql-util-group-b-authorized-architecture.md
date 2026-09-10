# Row #324 Group B — Authorized compatibility architecture

**Status:** Architecture only; no application source or Saturn source was modified.
**Scope:** The five named `SqlUtil` constants authorized as a bounded Group B compatibility slice.
**Target:** Zenbot (`/Users/ab/workspace/go-projects/zenbot`), H2 PostgreSQL-wire persistence.
**Reference:** Saturn develop, read-only (`/Users/ab/workspace/projects/saturn`).

## 1. Scope and explicit exclusions

In scope only:

- `DELETE_TRIP_NAMES`
- `DELETE_TRIP`
- `DELETE_NAME`
- `SELECT_NAME_TRIP_REGISTERED`
- `SELECT_LAST_N_MESSAGES`

Explicitly out of scope: Group C; row **#325** (`Util`); `internal/agent/sql` and agent/sql policy; unrelated production registration; new command/listener/provider/router/transport/remote-room/Whiskey work; broad service or command work; Saturn edits; and rewriting protected `MIGRATION_PLAN.md` or `.hermes/migration-audit.md`. Existing dirty/staged/untracked target work must be preserved.

This is not full row #324 acceptance. Group A remains bounded by `.hermes/handoffs/sql-util-group-a-qa.md`; the prior Group B decision is superseded only to the extent that this separately authorized architecture may proceed to contract/test design, not implementation.

## 2. Evidence inventory

### [OBSERVED] Saturn source and callers

`src/main/java/org/saturn/app/util/SqlUtil.java` contains the exact declarations:

```sql
DELETE FROM trip_names WHERE trip_id IN (
        SELECT id FROM trips WHERE trip = ?
) OR name_id IN (
SELECT id FROM names WHERE name = ?
);

DELETE FROM trips WHERE trip = ?;

DELETE FROM names WHERE name = ?;

SELECT DISTINCT n.name,t.trip
FROM trip_names tn
INNER JOIN trips t on tn.trip_id = t.id
INNER JOIN names n on tn.name_id = n.id ORDER BY t.trip DESC;

SELECT name,message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not
in ('LEFT','JOINED')) order by created_on desc limit ?;
```

The source uses Java text blocks for the first four declarations and a concatenated string for the last; the SQL above is a semantic transcription, not a license to normalize whitespace or change predicates.

Relevant callers are:

- `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`, `delete(String name, String trip)`: executes `DELETE_TRIP_NAMES` with `(trip,name)`, then `DELETE_TRIP` with `(trip)`, then `DELETE_NAME` with `(name)` inside `runInTransaction`; returns `0` on success and `1` on `SQLException`.
- The same class, `lastMessages(String name, String trip, int count)`: changes `count <= 0` to `5`, binds nullable name, trip, count, reads columns `name`, `message`, `created_on`, and excludes `LEFT`/`JOINED` through the SQL. Its Java `Message` is a three-value result (`mName`, the method argument `trip`, text, timestamp); SQL errors are logged and return the accumulated list.
- `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java`, `getRegisteredUsers()`: executes `SELECT_NAME_TRIP_REGISTERED`, reads named columns `name` then `trip`, and renders `name + " " + trip + "\\n"`; SQL errors return an empty string.
- `src/main/java/org/saturn/app/service/UserService.java`: declares `delete`, `lastMessages`, and registration-related methods. The Saturn service interface is not a target Go interface.

### [OBSERVED] Zenbot target surfaces

- `internal/repository/user_queries.go`: `UserQueryRepository.RegisteredUsers(context.Context) ([]repository.RegisteredUser,error)` and `RegisteredUser{Trip,Name}`.
- `internal/repository/repository.go`: `IdentityRepository` currently contains registration and `LastMessages(string,string,int) ([]model.Message,error)`, but no delete contract.
- `internal/repository/h2/user_queries.go`: `selectRegisteredUsers` is `select distinct t.trip,n.name ... order by n.name desc`; it scans `(Trip,Name)`. This intentionally differs from Saturn's `(Name,Trip)` projection and `ORDER BY t.trip DESC`.
- `internal/repository/h2/identity.go`: `LastMessages` currently selects `id,trip,name,hash,message,created_on,channel`, filters `visibility='PUBLIC'` and excludes `LEFT`/`JOINED`, orders `created_on DESC,id DESC`, defaults non-positive count to `5`, and returns rich `model.Message` values. It uses `fmt.Sprintf` only for the validated integer limit; data values remain parameters.
- `internal/service/services.go`: `UserService` delegates registered-user reads to `Queries`, and last-message reads to `Identity`. `MailService.RegisteredUsers` independently performs a target SQL query with `(name,trip)` and `ORDER BY t.trip DESC`, then renders the string.
- `internal/command/identity_commands.go`: `messagesCommand` requires a trip/count, caps count above 30, calls `LastMessages("", trip, n)`, and renders rich messages. No delete command or delete call path was found in the inspected target surfaces.
- `internal/repository/h2/schema-h2.sql`: `trips.trip` and `names.name` are unique; `trip_names` has foreign keys to both parents but no `ON DELETE CASCADE`. Therefore deleting a parent before all links are removed is unsafe, and the Saturn three-step order is materially relevant.
- `internal/repository/h2/database.go`: `WithTx`-style transaction primitives are used by identity mutations; `Open` proves H2 with `SELECT H2VERSION()` and bootstraps schema in a transaction. `internal/repository/h2/database_test.go` is the real-H2 PostgreSQL-wire gate.

### [TEST-BACKED] Existing target behavior

- `internal/repository/h2/identity_test.go` verifies case-insensitive registration checks, atomic registration rollback, public-only rich message results, exclusion of whispers and presence events, and `id DESC` tie breaking.
- `internal/repository/h2/user_queries_test.go` verifies target `RegisteredUsers` values in `(Trip,Name)` order and case normalization for `NicksByTrip`.
- `internal/repository/h2/sql_util_row324_group_a_test.go` and `.hermes/handoffs/sql-util-group-a-qa.md` verify only the bounded Group A subset; they do not prove Group B.
- The current suite has no target delete interface/implementation contract and no focused test proving Saturn's three-delete sequence.

### [LIMITATION]

The inspected Saturn tests contain only a narrow `H2CommandPersistenceCompatibilityTest` assertion that `lastMessages(...,1)` returns one row. They do not establish delete affected-row semantics, collision behavior, rollback after an injected mid-sequence failure, or deterministic tie ordering. Saturn's `runInTransaction` implementation was observed through its call boundary, but its lower-level connection/rollback helper was not adopted as a target API.

## 3. Compatibility decision per constant

| Constant | Decision | Rationale and required contract |
|---|---|---|
| `DELETE_TRIP_NAMES` | **Requires an explicit compatibility API** | Exact SQL is available, but Zenbot has no delete interface and the predicate is broader than a single `(trip,name)` pair: it removes every link matching the trip OR the name. Expose only through a named, security-reviewed operation; do not add it to the generic `IdentityRepository` without an approved caller contract. |
| `DELETE_TRIP` | **Requires an explicit compatibility API** | Exact SQL can run against the unique `trips.trip`, but it is a parent delete whose correctness depends on the preceding link delete and on explicit scope/authorization. It cannot be silently added as a public one-step helper. |
| `DELETE_NAME` | **Requires an explicit compatibility API** | Exact SQL can run against unique `names.name`, but it is global by name and is not equivalent to deleting one trip/name association. Its collision and authorization semantics must be explicit. |
| `SELECT_NAME_TRIP_REGISTERED` | **Exact parity is possible, through a separate compatibility read** | The schema supports the exact projection/order and the Saturn caller reads named columns. A new compatibility method can return a Saturn-shaped `Name,Trip` record while preserving the existing `RegisteredUsers` `(Trip,Name)`/`name DESC` contract. No positional swapping or existing-interface rewrite is permitted. |
| `SELECT_LAST_N_MESSAGES` | **Requires an explicit compatibility API** | Exact SQL returns only `(name,message,created_on)`, excludes only `LEFT`/`JOINED`, has no visibility predicate or id tie-break, and permits `name IS NULL`. Zenbot's existing API returns rich `model.Message`, enforces `PUBLIC`, and has id tie ordering. A separate Saturn-compatibility read is required if exact parity is desired; otherwise this constant remains **blocked for implementation** until a result/visibility contract is signed. |

The last row's explicit API decision does not authorize removing Zenbot's public-only history boundary. Any compatibility API must be separately scoped and must not be reachable from agent, command, or general history paths without a security decision.

## 4. Proposed target interfaces and files

### [RECOMMENDED] New narrow repository contracts

Do not alter the existing contracts merely to make shapes appear compatible. Add a dedicated interface in `internal/repository/sql_util_group_b.go` (or an equivalently named bounded file) only after contract tests are approved:

```go
type SaturnRegisteredUser struct {
    Name string
    Trip string
}

type SaturnLastMessage struct {
    Name      string
    Message   string
    CreatedOn int64
}

type SqlUtilGroupBRepository interface {
    DeleteTripNames(ctx context.Context, trip, name string) (DeleteResult, error)
    DeleteTrip(ctx context.Context, trip string) (DeleteResult, error)
    DeleteName(ctx context.Context, name string) (DeleteResult, error)
    SaturnRegisteredUsers(ctx context.Context) ([]SaturnRegisteredUser, error)
    SaturnLastMessages(ctx context.Context, name *string, trip string, count int) ([]SaturnLastMessage, error)
}
```

`DeleteResult` should be a named result (at minimum `RowsAffected int64`; optionally a bounded per-statement breakdown) rather than an unexamined integer. The interface must document whether individual delete methods are primitives or whether only an atomic `DeleteIdentity(ctx, name, trip)` operation is exposed. The safer default is to expose the atomic composite operation to callers and keep the three SQL statements private, because Saturn invokes them as one transaction.

Exact files to inspect/modify in a later implementation slice:

- `internal/repository/sql_util_group_b.go` — new narrow types/interface, if approved.
- `internal/repository/h2/sql_util_group_b.go` — exact SQL constants and H2 implementation; use `QueryContext`/`ExecContext` and parameters, never concatenate identity values.
- `internal/repository/h2/sql_util_row324_group_b_test.go` — real-H2 contract and rollback tests.
- `internal/repository/user_queries.go` — only if the compatibility read must be exposed beside, but not replace, the existing query seam.
- `internal/repository/h2/user_queries.go` and `internal/service/services.go` — only for an explicitly approved compatibility caller; do not change existing `RegisteredUsers` or `LastMessages` behavior.
- `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql` — **no planned change**; inspect metadata and preserve current foreign-key semantics.

No command, listener, factory, provider, transport, agent, or production registration file is a target of this architecture.

Because this introduces a new repository interface, new H2 implementation/test file, and cross-file contract decisions, the implementation owner is **@senior-developer**.

## 5. Required semantics

### [RECOMMENDED] Deletes, transaction, and rollback

- The public compatibility operation must validate/authorize scope before mutation. SQL itself is not an authorization boundary.
- The composite delete must execute, in one `WithTx` transaction, exactly: `DELETE_TRIP_NAMES(trip,name)`, then `DELETE_TRIP(trip)`, then `DELETE_NAME(name)`. Commit only after all statements succeed.
- Roll back all prior statements on any error, including foreign-key or injected test failure. Never report success after partial mutation.
- Preserve Saturn's observed success/error convention only behind a typed Go error/result contract; do not return Saturn's `0/1` convention without documenting it.
- Record actual affected rows. The Saturn caller ignores counts and returns 0 on success, so count equality with Saturn is not test-backed; the target should not invent a misleading aggregate count.
- The SQL predicates are exact and case-sensitive as written. Any normalization, uniqueness assumption, absent-row no-op policy, or trip/name resolution must be explicit and tested. No global `DELETE_NAME` may be substituted for a trip-qualified delete.
- Since `trip_names` has no cascade, statement ordering is a correctness requirement. A parent delete that fails must leave the transaction unchanged.

### [RECOMMENDED] Registered-user read

- `SaturnRegisteredUsers` returns `Name,Trip` values, scans by column names/order explicitly, uses `DISTINCT`, and orders `Trip DESC` as Saturn does.
- Empty result is a non-error empty slice (or a documented nil-vs-empty choice); preserve it consistently.
- Existing `RegisteredUsers` remains `Trip,Name` and `Name DESC`. No caller receives a silently reordered slice.
- Saturn SQL has no tie-break beyond trip; do not add one under an “exact parity” label. If deterministic ties are required, that is a target-specific compatibility decision and must be named as such.

### [RECOMMENDED] Saturn last-message read

- The exact compatibility projection is `(name,message,created_on)` only.
- Bind nullable `name` as SQL NULL when requested; bind `trip` and integer limit as parameters or a driver-safe bounded integer expression. Do not interpolate user-controlled values.
- For exact Saturn parity, filter only `message NOT IN ('LEFT','JOINED')`, order only `created_on DESC`, and default `count <= 0` to 5 because that is observed in the caller. Do not add `PUBLIC` or `id DESC` in this API.
- The existing `LastMessages` remains public-only, rich, and `created_on DESC,id DESC`; no compatibility result may be coerced into `model.Message` or passed to the current `messagesCommand`.
- Limit policy must be explicit. The current command cap of 30 is command behavior, not a repository-wide Saturn constant contract; this slice must not broaden command work.

## 6. Security boundary and unsupported behavior

### [OBSERVED]

Saturn's SQL constants contain no authorization predicates. Zenbot's `messages.visibility` check is a target security boundary in the existing history path. The delete SQL is also broader than its two input parameters suggest because each delete predicate uses OR across a whole trip or whole name.

### [RECOMMENDED]

- Keep compatibility methods repository-internal until an authorized service caller exists. Require an already-authorized caller/context and fail closed when it is absent.
- Do not expose raw delete primitives to agent/sql, SQL policy, commands, listeners, or generic database callers.
- Do not allow an arbitrary caller to use `DELETE_NAME(name)` as a synonym for deleting one identity association.
- Do not bypass `PUBLIC` filtering in the existing Zenbot history API. An exact Saturn last-message API, if approved, must have a clearly named restricted boundary and cannot be used for whisper/history disclosure.
- Use context-aware statements, parameter binding, bounded count validation, and no dynamic SQL from identity input.

Unsupported in this slice: authorization policy design, agent SQL policy changes, new production registrations, command behavior, visibility migration, schema changes, service-wide error mapping, and any Saturn behavior not evidenced by the cited constants/callers.

## 7. Real-H2 test plan and QA gates

### [RECOMMENDED] RED/GREEN contract tests

Add tests before implementation and run them against the existing real H2 PostgreSQL-wire fixture (`openTestDB`/`Open`, pinned H2 2.3.232):

1. Verify `SELECT H2VERSION()` and schema metadata; confirm unique parent keys and non-cascading `trip_names` foreign keys.
2. Seed multiple trips/names/links, including the same name linked to multiple trips where schema/test setup permits, then verify the exact OR semantics of `DELETE_TRIP_NAMES` and the exact parent scope of the two parent deletes.
3. Verify composite delete ordering and success: links disappear before parents, unrelated rows remain, and `RowsAffected` is documented.
4. Verify absent trip/name behavior, duplicate/collision scope, blank inputs, and authorization-denied calls without mutation.
5. Force a failure after the first or second delete (test seam or FK/injected error) and assert all tables are restored; no partial links, trip, or name remain.
6. Verify `SaturnRegisteredUsers` projection (`Name,Trip`), `DISTINCT`, descending trip order, empty result, and that the existing `RegisteredUsers` test/contract is unchanged.
7. Verify Saturn last-message projection, nullable name, trip matching, exclusion of `LEFT`/`JOINED`, count default/boundary, descending timestamp order, equal-timestamp behavior (document nondeterminism if exact parity), and no rich-field enrichment.
8. Verify security separation: whisper rows are not returned by existing `LastMessages`; no compatibility method is reachable through the current command/service path.

### QA gates

- Exact SQL in the implementation is source-transcribed and reviewed against `SqlUtil.java`; no guessed or reformatted semantic changes.
- Contract tests are RED before implementation and GREEN after it; tests use real H2, not mocks alone.
- Existing focused tests (`identity_test.go`, `user_queries_test.go`) continue to pass unchanged.
- Run `go fmt ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and `go build ./...`; record actual outputs in the implementation/QA handoffs.
- Independent QA checks result shapes by field, ordering/filtering/limits, transaction rollback, affected-row semantics, parameter safety, and that no out-of-scope file changed.
- Final slice acceptance requires a separate implementation handoff and independent QA PASS. It does not close Group C, row #325, or full row #324.

## 8. Complexity and recommendation

**Complexity:** Medium-high. The SQL itself is small, but deletes cross three FK-related tables, introduce a new repository seam, require explicit authorization and rollback semantics, and coexist with two intentionally different existing read contracts. The last-message compatibility API creates a second result shape and must not leak into the public-only rich history path.

**Owner:** **@senior-developer**, because interface/schema-boundary decisions and multi-file H2/repository/test changes are required. No schema modification is currently recommended; metadata verification is required before implementation.

**Recommendation:** Proceed only with a follow-on implementation authorization that signs the typed delete scope/result contract and decides whether the exact Saturn last-message read is allowed behind a restricted compatibility API. Implement the exact SQL in a dedicated bounded repository file, preserve existing Zenbot contracts, and keep all production callers untouched in this slice.

## 9. Architecture completeness check

- [x] Exact five Saturn constants transcribed from read-only source.
- [x] Saturn callers and relevant target interfaces/implementations/tests cited.
- [x] Observed, test-backed, limitations, and recommendations separated.
- [x] Per-constant parity/API/blocked decision recorded.
- [x] Transaction, rollback, ordering, filtering, limit, and result-shape semantics defined.
- [x] Real-H2 plan and QA gates defined.
- [x] Security boundaries and unsupported behavior defined.
- [x] Complexity and implementation owner recorded.
- [x] Group C, #325, agent/sql policy, unrelated registration, and broad service/command work explicitly excluded.
- [x] No application source or Saturn source changed by this architecture task.
