# Notes Group C full-suite H2 failure diagnostic

## Verdict

**The reported failure is an existing H2 test-isolation/lifecycle defect, not a Notes Group C implementation defect.** It is a shared-server contamination/race caused by fixed TCP ports and a bootstrap routine that assumes any listener at that port is the current test's isolated server. The bounded Notes acceptance is **not blocked**: the task-owned focused repository Notes test passes in a clean run, and the failed statement concerns the pre-existing `messages` index, not `notes` behavior.

A full-suite green result remains blocked until the H2 test harness is repaired; that is a separate infrastructure defect and should be tracked independently.

## Failure attribution

Reported full-suite symptom:

```text
--- FAIL: TestGroupA_SelectRoleByTripContract
internal/repository/h2/sql_util_row324_group_a_test.go:183: H2 schema bootstrap: ERROR: General error: "java.lang.RuntimeException: object already exists: idx_messages_trip_created_on"; SQL statement: CREATE INDEX IF NOT EXISTS idx_messages_trip_created_on ON messages (trip, created_on DESC) [50000-232] (SQLSTATE HY000)
```

`TestGroupA_SelectRoleByTripContract` only reaches this at `openTestDB(t)` (`internal/repository/h2/sql_util_row324_group_a_test.go:181-184`). It has not executed any Group A assertion or written Notes data when it fails.

The failing DDL is a single declaration in both schema mirrors:

- `internal/repository/h2/schema-h2.sql:61` (the embedded runtime schema)
- `resources/schema-h2.sql:61` (the resource mirror)

Neither schema contains a duplicate declaration of `idx_messages_trip_created_on`. `internal/repository/h2/database.go:172-210` instead splits the embedded schema on semicolons and executes every DDL statement in one transaction. Thus the error is an H2 concurrent/repeated-bootstrap symptom, not two statements in a schema file.

### Mechanism

1. `openTestDB` gives each test a unique temporary `BaseDir`, but hard-codes **port 55436** and database name `db` (`internal/repository/h2/audit_test.go:13-22`).
2. `processServer.Start` checks only whether something is listening on that port. If so, it returns success without determining ownership, base directory, database state, or readiness for another bootstrap (`internal/repository/h2/database.go:73-77`).
3. An opener that reused an existing listener has `s.cmd == nil`; therefore its `Stop` is a no-op (`internal/repository/h2/database.go:94-108`). It cannot clean up the server it reused.
4. During server shutdown/startup overlap or another `go test` invocation, a test configured for a new temporary directory connects to the already-running server's `db` instead. Two bootstrap transactions can then issue `CREATE INDEX IF NOT EXISTS ...` against the same database. H2 2.3.232 can return `object already exists` in this race despite `IF NOT EXISTS`.
5. The same fixed-port lifecycle pattern exists in the Notes command helper, with port **55437** (`internal/command/note_notes_parity_test.go:15-31`), proving the defect is harness-wide rather than tied to Group A or Notes SQL.

The observed failure shape is therefore timing-dependent: it can surface as duplicate index, database exclusive mode, closed session, reset/EOF, or connection failure depending on the server lifecycle phase.

## Reproduction evidence (no source/test changes)

All commands were run from `/Users/ab/workspace/go-projects/zenbot` with `-count=1`.

### Clean isolated targets

```sh
go test ./internal/repository/h2 -run '^TestGroupA_SelectRoleByTripContract$' -count=1 -v -timeout=30s
# PASS

go test ./internal/repository/h2 -run '^TestSqlUtilRow324GroupCNotesParity$' -count=1 -v -timeout=30s
# PASS
```

This demonstrates the failing Group A test is not deterministically broken and that the task-owned H2 Notes test is independently green.

### Lifecycle contamination repro

```sh
go test ./internal/command \
  -run '^Test(NoteAndSaveParityAliasesAndTripBoundary|NotesParityListPurgeClearAndInvalidArguments)$' \
  -count=1 -v -timeout=30s
```

Observed:

```text
TestNoteAndSaveParityAliasesAndTripBoundary: PASS
TestNotesParityListPurgeClearAndInvalidArguments:
H2 schema bootstrap: ERROR: General error: "java.lang.RuntimeException:
object already exists: idx_messages_trip_created_on"; SQL statement:
CREATE INDEX IF NOT EXISTS idx_messages_trip_created_on ON messages (trip, created_on DESC)
```

The two tests use the same helper/port 55437 but distinct `t.TempDir()` directories. The second bootstrap can attach to the first server instead of the directory it requested, yielding the same duplicate-index signature.

Additional evidence: while another full-suite verifier was active, port 55436 was owned by an H2 server launched with the temporary base directory of `TestUserQueriesPreserveSaturnRowsAndTripNormalization`. Concurrent targeted Group A/Notes invocations then failed with H2 exclusive-mode, connection-reset, and closed-session errors. After that process exited, clean isolated target runs passed.

A package run with a residual 55436 listener failed early with exclusive-mode/reset/session errors, while later Group A and Group C tests passed once a fresh server became available. That confirms a shared process/state boundary, not a test-data or Notes-query defect.

## Scope and impacted paths

### Directly implicated

- `internal/repository/h2/database.go`
  - `processServer.Start`: treats any listener on the configured port as reusable.
  - `processServer.Stop`: cannot stop a reused listener.
  - `bootstrap`: repeatedly executes DDL without cross-client serialization.
- `internal/repository/h2/audit_test.go`
  - shared `openTestDB` fixture: per-test directory but static port 55436/database `db`.
- `internal/command/note_notes_parity_test.go`
  - analogous fixture: per-test directory but static port 55437/database `db`.
- `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql`
  - contain the affected index declaration once each; they are evidence of the DDL target, not the root cause.

### Not implicated / outside the bounded Notes implementation

- `internal/repository/h2/sql_util_row324_group_c_notes_test.go` is a task-owned focused acceptance test and passed independently.
- The failure is on `messages` / `idx_messages_trip_created_on`, before any Notes assertion.
- The stated Notes slice owns only `internal/repository/h2/sql_util_row324_group_c_notes_test.go` and `internal/command/note_notes_parity_test.go`; it does not alter H2 runtime source, bootstrap, or either schema.

## Safe resolution recommendation

Do **not** suppress this by removing `IF NOT EXISTS`, deleting indexes, weakening Group A, or changing Notes behavior.

Repair the H2 test harness in a separate, explicitly owned change:

1. Allocate a unique free port per test process/fixture (or run a deliberately shared suite server with unique database stems and controlled lifecycle); do not use a package-global fixed port.
2. Make server ownership explicit. `Start` must not silently reuse an arbitrary listener; either verify the owned process/expected base directory or fail with a clear collision error.
3. On stop, wait until the owned port is actually released before the next test can start. A reused server must not be presented as a newly isolated server.
4. Serialize schema bootstrap for any intentionally shared database, or bootstrap once before parallel clients connect. Keep index DDL idempotent as defense in depth, but do not rely on H2's `IF NOT EXISTS` to make concurrent DDL safe.
5. Add a regression test that opens two fixtures sequentially and concurrently with distinct temporary directories, verifies distinct data visibility, and verifies no H2 listener remains after cleanup.

Until then, run bounded H2 acceptance tests in an isolated process with no pre-existing listener and avoid overlapping `go test` invocations that use the fixed fixture ports.

## Acceptance decision

**Bounded Notes Group C acceptance: PASS / not blocked by this failure.**

The Notes repository target passed cleanly. The command Notes pair exposed the same pre-existing static-port lifecycle flaw on its second fixture, so its broad pair invocation is not reliable as a source-level Notes verdict. The full suite cannot presently be claimed green because of the separate H2 harness defect; retain that failure as an infrastructure follow-up rather than attributing it to Notes.
