# H2 broad-gate forensic diagnostic — identity slice

**Scope:** investigation only; no application or test code was changed.

## Finding and attribution

The broad H2 failures are attributable to a **test-harness server lifecycle/isolation defect**, not to the current identity-slice behavior.

| Area | Attribution | Evidence |
|---|---|---|
| H2 startup, connection availability, schema bootstrap, retained rows | **Harness defect** | All package fixtures share TCP port `55436`; `Open` adopts any listener on that port without proving ownership or its base directory/database. The adopted server can therefore resolve the common database stem `db` from an earlier fixture/run. |
| `exclusive mode`, EOF, duplicate-index bootstrap failures | **Harness defect** | These happen at the process/connection/bootstrap boundary before repository query behavior is exercised. Concurrent/adopted bootstrap on the same H2 database accounts for the observed DDL and connection-lifecycle failures. |
| Current Group B identity/message-history behavior | **No regression found in isolated execution** | The changed H2 identity test passed alone against a fresh fixture. The three changed command tests also passed. The changed production code does not alter H2 server startup, port allocation, fixture lifecycle, or schema bootstrap. |

The broad `go test ./...` gate is therefore **not currently a valid acceptance signal for the slice**. It remains a blocking repository gate until the harness is isolated and the broad suite is rerun, but it is not evidence that the identity slice is defective.

## Root cause

`internal/repository/h2/audit_test.go:13-22` defines the package fixture:

```go
dir := t.TempDir()
d, err := Open(context.Background(), Config{
    BaseDir: dir,
    DatabaseStem: filepath.Join(dir, "db"),
    H2Jar: jar,
    Port: 55436,
    StartupTimeout: 5 * time.Second,
})
t.Cleanup(func() { _ = d.Close() })
```

Every H2 test using `openTestDB` gets a different temporary directory, but all use **the same fixed port** (`55436`) and the same logical database name (`db`).

`internal/repository/h2/database.go:73-77` implements startup ownership incorrectly for that fixture model:

```go
s.addr = net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
if c, err := net.DialTimeout("tcp", s.addr, 200*time.Millisecond); err == nil {
    c.Close()
    return nil
}
```

A reachable listener is treated as the requested H2 server. The code does **not** verify:

1. that it was started by this `Database` instance;
2. that it has the requested `BaseDir`;
3. that it serves this fixture's temporary database files; or
4. that another fixture is not bootstrapping the same logical database.

`Open` then discards the directory portion of the requested database stem (`database.go:115,122`) and connects using only its basename:

```go
c.DatabaseStem = filepath.Base(c.DatabaseStem) // "db"
dsn := ... /db?sslmode=disable
```

For an adopted server, `db` is resolved under **the already-running server's** `-baseDir`, not the new fixture's `t.TempDir()`. This makes data and DDL state survive into a nominally separate test fixture.

## Failure data flow

```text
H2 test A
  openTestDB -> TempDir A + stem "db" + fixed port 55436
  Open -> starts Java H2 with -baseDir TempDir A
  bootstrap -> schema/index DDL; test writes rows

H2 test B, another test binary/run, or a retry overlaps while A's server is live
  openTestDB -> TempDir B + stem "db" + the same fixed port 55436
  Open.Start -> TCP dial succeeds -> adopts A's server (no ownership/baseDir check)
  Open -> PostgreSQL DSN database name "db"
  H2 resolves it at TempDir A, not TempDir B
  bootstrap -> concurrent/repeated DDL against A/db; queries see A's rows

Possible races/results
  * simultaneous schema transactions -> `object already exists: idx_messages_trip_created_on`
    despite `CREATE INDEX IF NOT EXISTS`;
  * conflicting H2 open/server lifecycle -> `[90135-232] database is open in exclusive mode`;
  * one fixture stops the adopted/original server while another is connecting ->
    `unexpected EOF` wrapped as `H2 PostgreSQL connection unavailable`;
  * state is retained -> later tests assert counts/closed state against prior fixture data.
```

`Database.Close` (`internal/repository/h2/audit.go:25-39`) closes its SQL client and invokes `Server.Stop`. A fixture that merely adopted an existing listener has `processServer.cmd == nil`, so it cannot own or stop that listener. Conversely, a fixture that owns the process can stop it while adopters are still using it. This confirms the lifecycle ownership split responsible for the connection symptoms.

## Why the current identity slice is not the cause

The bounded modified paths are:

- `internal/command/identity_commands.go`
- `internal/command/identity_commands_test.go`
- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`

Their behavior changes are Group B result projection/filter/order (`name, trip, message, created_on`, public visibility, `id` tiebreak), returned row-trip display, and Saturn command parsing semantics. None changes:

- `h2.Config`;
- `processServer.Start` / `Stop`;
- port selection;
- `openTestDB`;
- H2 base-directory resolution; or
- schema/index bootstrap.

The changed Group B test does call the existing `openTestDB`, which means it can be a **victim** of the fixture defect, but it does not introduce it.

## Reproduction and observed results

### User-provided broad-gate baseline

After non-H2 packages passed, `go test ./...` failed in `internal/repository/h2` with failures spanning unrelated files: `audit_test.go`, `authorization_identity_test.go`, `basic_user_data_test.go`, `dbz_test.go`, `identity_test.go`, `mail_notes_test.go`, both SQL utility group tests, including the changed Group B test.

Representative errors:

```text
H2 schema bootstrap: ERROR: The database is open in exclusive mode; can not open additional connections [90135-232]
H2 PostgreSQL connection unavailable ... 127.0.0.1:55436 ... unexpected EOF
General error: RuntimeException: object already exists: idx_messages_trip_created_on
```

The cross-cutting failure distribution and bootstrap/connection failure stage are inconsistent with a defect confined to the new Group B query.

### Minimal isolated current-slice H2 test (executed)

```bash
go test ./internal/repository/h2 \
  -run '^TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies$' \
  -count=1 -v
```

Observed:

```text
=== RUN   TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies
--- PASS: TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies (0.65s)
PASS
ok      zenbot/internal/repository/h2    0.934s
```

### Isolated changed command tests (executed)

```bash
go test ./internal/command \
  -run 'Test(AccessCommandUsesSaturnRawCaseSensitiveRoleParsing|AccessCommandCommaTargetsUseUserAndJavaSplitSemantics|MessagesCommandRendersReturnedGroupBRowTrip)$' \
  -count=1 -v
```

Observed: all three tests passed; package result was `ok zenbot/internal/command 0.338s`.

### Environmental corroboration

Before the isolated H2 run, local inspection found a Java listener on `*:55436`. That is direct evidence that the fixed fixture port can already be occupied before a new fixture attempts startup. Because `Start` treats any successful TCP connection as reusable, this condition necessarily bypasses the new fixture's requested `BaseDir`.

## Impacted paths

### Root-cause paths

- `internal/repository/h2/audit_test.go:13-22` — package-wide fixed-port fixture (`55436`), temp directory, shared `db` stem.
- `internal/repository/h2/database.go:45-108` — process start/stop; a successful port probe adopts an unknown server.
- `internal/repository/h2/database.go:110-170` — removes database directory and connects by basename after server adoption.
- `internal/repository/h2/database.go:172-210` and `schema-h2.sql:61-78` — bootstrap executes shared schema/index DDL, exposing the collision.
- `internal/repository/h2/audit.go:25-39` — cleanup cannot stop an adopted server and can stop a server used by adopters.

### Affected test consumers

All tests that call `openTestDB`, including the reported audit, authorization, basic-user-data, DBZ, identity, mail/notes, Group A, and Group B tests. `database_test.go` is separately fixed to `55435` and has the same general fixed-port/adoption design risk, though it does not share `55436`.

## Safe resolution path (not implemented)

1. **Give every test fixture a unique server endpoint.** Prefer an OS-assigned free port reserved through startup, or a package-level single owned server with a unique database name per test. Do not use a globally fixed `55436` fixture port.
2. **Make server ownership explicit.** A successful TCP probe must not silently mean “this instance owns a compatible H2 server.” Either always launch an owned process for its unique port, or use an explicit shared-server manager with synchronization, reference counting, and a known base directory.
3. **Preserve per-test database identity at the server boundary.** Use a unique database name/stem for each test and ensure it is resolved by the server that owns the requested base directory. Avoid reducing an absolute fixture path to a generic `db` when adopting another server is possible.
4. **Serialize bootstrap for any intentionally shared database.** If a shared server/database is retained, guard schema bootstrap and wait for completion before clients open connections. This is secondary to fixing ownership and isolation.
5. **Guarantee cleanup and verify it.** Close SQL pools before terminating the owned Java process; wait for process exit; only delete a temp directory after its server is gone. Never let one fixture terminate a server that another fixture adopted.
6. **Add harness regressions.** At minimum, open two test databases with distinct temp directories concurrently and assert independent rows, then repeatedly run the H2 package and `go test ./...` under parallel package execution.

After this harness work, rerun the isolated Group B test, the H2 package (including repeated/stress runs), and `go test ./...`. The identity slice should not be accepted as fully green until that repository gate is demonstrated passing, but no query/command rollback is indicated by this investigation.
