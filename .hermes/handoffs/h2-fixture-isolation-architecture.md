# H2 fixture isolation architecture

**Status:** proposed test-harness correction; no application or test source is changed by this document.

**Objective:** make the repository's H2-backed tests independently owned and concurrently safe so that `go test ./...` is a trustworthy migration gate.

## 1. Evidence and boundary

### Observed

- `internal/repository/h2/database.go` constructs an H2 `processServer`. `processServer.Start` treats a successful TCP dial of `Host:Port` as a server it may use, without proving process ownership or `BaseDir` identity. `Open` then reduces `Config.DatabaseStem` to `filepath.Base(...)` before connecting.
- Consequently, two fixtures with distinct `t.TempDir()` values but a common port and database basename connect to the first listener's database. `Database.Close` (`internal/repository/h2/audit.go`) closes its SQL pool then calls `Server.Stop`; an adopting instance has no `cmd` to stop, while the real owner can stop the process still used by adopters.
- `internal/repository/h2/audit_test.go:openTestDB` uses a per-test directory but fixed `55436` and stem `db`. It is called 31 times across the H2 package's test files.
- Fixed ports also exist in H2-backed tests outside the H2 package: `55435` in `internal/repository/h2/database_test.go`, `55437` in `internal/command/users_nicks_integration_test.go`, and `55438` in `internal/command/note_notes_parity_test.go`. `internal/command/mail_group_c_parity_test.go` and `internal/service/mail_group_c_parity_test.go` use the close-a-`net.Listen("127.0.0.1:0")` pattern, which has a release-to-bind race.
- H2 2.3.232 supports `-pgPort 0`. An executed local probe reported `PG server running at pg://localhost:62884 (only local connections)`, demonstrating that H2 chooses an ephemeral port and reports it on process output.

The confirmed incident and detailed failure sequence are in `.hermes/handoffs/identity-slice-h2-broad-gate-diagnostic.md`.

### Scope rule

This correction is **test-harness-only by use and ownership**:

1. Add an opt-in server-launch capability to the H2 adapter solely to support an owned ephemeral endpoint. It must not alter the current behavior of existing non-test `Config` callers.
2. Put all fixture construction in one internal test utility and import it only from `*_test.go` files.
3. Convert every current real-H2 test fixture listed above to that utility, including all `openTestDB` consumers indirectly. Do not change repository query behavior, schema SQL, production startup defaults, migration behavior, or protected migration handoffs.

The small production-package change is limited to a backwards-compatible configuration bit and process lifecycle plumbing; it is required because a test helper cannot safely retain ownership of a Java process created behind `h2.Open` otherwise.

## 2. Candidate designs

| Design | Assessment |
|---|---|
| Fixed port per package/test (for example, derive from PID) | Reject. It can collide with another test binary, stale Java process, CI job, or a concurrent invocation; it retains the unsafe “any listener is compatible” adoption path. |
| Reserve a free Go TCP port, close it, then pass it to H2 | Reject. This is only a TOCTOU reduction, not ownership: another process can bind between `Close` and Java/H2 bind. Existing Mail Group C helpers use this pattern. |
| One package-level shared H2 server, unique database per test | Viable but not minimal. It requires a synchronized manager, reference counting, bootstrap serialization, cleanup coordination, and a clear solution for cross-package test binaries. |
| **H2-owned OS-assigned port per fixture, centrally constructed** | **Recommended.** H2 itself binds port zero, announces the actual port, and the exact child process is retained by the `Database` instance. Each fixture has its own `t.TempDir`, server base directory, database stem, endpoint, SQL pool, schema bootstrap, and cleanup. No server adoption occurs in fixture mode. |

## 3. Recommended design

### 3.1 New opt-in configuration and server semantics

Add one explicit field to `internal/repository/h2/database.go`:

```go
// AutoPort asks an owned processServer to let H2 select an ephemeral PG port.
// It is intended for isolated test fixtures. Existing Port == 0 behavior is unchanged.
AutoPort bool
```

Do **not** overload `Port == 0`: today it means the compatibility default `5435`; retaining that meaning avoids changing application behavior.

When `Config.AutoPort` is false, preserve current `processServer.Start` behavior exactly (including the existing externally-managed-server adoption behavior). Fixture code must never select this path.

When `Config.AutoPort` is true:

1. Require a loopback host (`127.0.0.1` is what the fixture utility supplies); do not expose an auto-selected test listener beyond loopback.
2. Do not pre-dial and do not adopt an existing listener.
3. invoke H2 with `-pg -pgPort 0 -ifNotExists -baseDir <fixture-dir>`;
4. capture the child process's combined startup output and parse exactly the H2 startup record `PG server running at pg://<host>:<decimal-port> ...`; validate a nonzero decimal port and normalize it with `net.JoinHostPort`;
5. set `processServer.addr` to that parsed endpoint before returning success, then dial **that endpoint** until reachable or `StartupTimeout` expires;
6. if output ends, parsing fails, the process exits, context expires, or readiness times out: kill/wait for this child and return an error that identifies startup/endpoint discovery. It must never fall back to port `5435`, a caller-provided fixed port, or adoption;
7. have `Open` build its DSN from `s.Addr()` rather than from the original `Config.Port`, so auto-port connections use the discovered endpoint.

The output reader must drain through child exit (not stop after the first line), so a future H2 log write cannot block the Java child on a full pipe. Startup parsing should deliver one validated port through a channel; process-exit and context cancellation must also unblock the wait. A `sync.Once`/single completion path is sufficient to prevent duplicate success/error delivery.

`processServer.Stop` remains the only process owner’s stop path in auto-port mode. It must:

1. be idempotent;
2. send interrupt, wait for the exact `exec.Cmd`, and kill then wait when its stop context expires;
3. never signal a process when `cmd == nil` (the legacy adoption case); and
4. return only after the owned child has exited, so `t.TempDir` cleanup cannot race its `-baseDir` files.

`Database.Close` already has the required ordering—close `DB` before `Server.Stop`—and should retain it. Fixture cleanup invokes `Close` exactly once; cleanup errors must be surfaced through the test utility rather than discarded.

### 3.2 Central fixture utility

[RECOMMENDED] Create `internal/testutil/h2fixture/h2fixture.go` (a proposed new path; a normal Go file by necessity, but with a package comment declaring it test support only). Recommended API:

```go
package h2fixture

func Open(t testing.TB, stem string) *h2.Database
```

Behavior:

- call `t.Helper()`;
- resolve H2 jar from `H2_JAR`, retaining the repository's existing pinned `2.3.232` fallback only if that remains the current test prerequisite convention;
- allocate `dir := t.TempDir()`;
- validate `stem` is a simple nonempty filename (no path separators, no `.db`/`.mv.db` required); use `filepath.Join(dir, stem)` for `DatabaseStem`;
- call `h2.Open(context.Background(), h2.Config{BaseDir: dir, DatabaseStem: ..., H2Jar: jar, Host: "127.0.0.1", AutoPort: true, StartupTimeout: 5 * time.Second})`;
- call `t.Fatal` on open error;
- register `t.Cleanup` immediately after a successful open. Cleanup calls `d.Close()` and reports a non-nil close error with `t.Errorf` (not `_ = d.Close()`);
- return the database.

The fixture utility has no package-global server, port, database name, mutable state, lock, reference counter, or cached client. Its lifecycle is per call. `t.TempDir` gives a distinct base directory; the caller-provided stems below should be unique-by-purpose but are no longer required to be globally unique because each server has its own base directory.

### 3.3 Required call-site conversion

| File | Replace with |
|---|---|
| `internal/repository/h2/audit_test.go` | Keep `openTestDB(t)` as a compatibility wrapper, but implement it as `return h2fixture.Open(t, "db")`. This mechanically fixes all 31 current callers in the H2 package without editing each test body. |
| `internal/repository/h2/database_test.go` | `h2fixture.Open(t, "zenbot")`; remove direct `Config`, fixed `55435`, direct jar lookup, and explicit/deferred close. |
| `internal/command/users_nicks_integration_test.go` | `h2fixture.Open(t, "dispatch")`; remove fixed `55437` and direct close. |
| `internal/command/note_notes_parity_test.go` | `h2fixture.Open(t, "notes")`; remove fixed `55438` and direct close. |
| `internal/command/mail_group_c_parity_test.go` | `h2fixture.Open(t, "mail-group-c-command")`; remove listener reservation/close and direct `h2.Open`. |
| `internal/service/mail_group_c_parity_test.go` | `h2fixture.Open(t, "mail-group-c")`, then return `.DB`; remove listener reservation/close and direct `h2.Open`. |

This list is task-owned and exhaustive for current real-H2 fixtures found through `h2.Open` and `openTestDB`. `internal/service/services_test.go` and `internal/command/service_commands_test.go` also reserve TCP ports, but they do not call H2 and are excluded from this H2 correction.

### 3.4 Proposed lifecycle sequence

```text
[PROPOSED: one fixture]
Test / subtest
  -> h2fixture.Open(t, "db")
     -> TempDir A; Config{BaseDir:A, stem:A/db, AutoPort:true}
     -> processServer.Start
        -> Java H2 binds its own loopback ephemeral port PA
        -> parse/validate startup endpoint PA; record owned exec.Cmd
     -> Open connects only to PA/db; bootstrap A/db
  -> test reads/writes A/db
  -> t.Cleanup: Database.Close
     -> SQL DB.Close
     -> interrupt/wait exact H2 child for PA
     -> t.TempDir removes A

Concurrent fixture B repeats with TempDir B, port PB, child B, database B/db.
PA != PB by OS allocation while both are live; B cannot reach A because it never dials PA.
```

## 4. Concurrency and shutdown invariants

The implementation and regression tests must demonstrate these invariants:

1. **Endpoint ownership:** every `AutoPort` fixture has a nonempty `Server.Addr()` that was announced by its own child; no pre-existing listener is consulted or reused.
2. **Concurrent separation:** two live fixtures have different addresses and different `BaseDir` values. A write on A is invisible on B and vice versa.
3. **Database identity:** DSNs use `Server.Addr()`, not `Config.Port`; each fixture's database stem is resolved under its own server's base directory.
4. **Bootstrap separation:** each fixture bootstraps only its own database. No fixture-level bootstrap mutex is needed because databases are not shared.
5. **Cleanup ownership:** only the fixture that started a Java process can signal/wait it. Closing A cannot stop B.
6. **Cleanup completion:** `Close` completes process termination before temp-directory cleanup; error and timeout paths kill/wait their own child before returning.
7. **No silent downgrade:** auto-port parse/start/readiness failure is a test failure, never a fallback to a fixed listener or legacy adoption.

## 5. Test plan

### Required regression tests

Add these focused tests in `internal/repository/h2` (names illustrative):

1. `TestAutoPortFixturesAreConcurrentAndIsolated`
   - create two parallel subtests using the central fixture utility;
   - have each fixture insert a distinct sentinel into `messages` while both are live; synchronize only after both opens/inserts;
   - assert `Server.Addr()` differs;
   - each fixture sees exactly its own sentinel and zero rows for the other sentinel;
   - this test must fail against the old `55436`/adoption fixture model.

   Avoid a parent/subtest `WaitGroup` deadlock: parallel subtests do not begin until their parent returns. Use two independently launched test workers with a barrier that closes when both are ready, or arrange the synchronization inside a single test after both fixtures have opened concurrently.

2. `TestAutoPortFixtureCloseDoesNotInterruptSibling`
   - open A and B concurrently;
   - close A explicitly;
   - verify B can ping and insert/query a sentinel after A closes;
   - close B and assert no close error. Do not rely only on `t.Cleanup` for this ownership assertion.

3. `TestAutoPortDoesNotAdoptOccupiedLegacyPort`
   - occupy a known loopback port with a test listener (or start a legacy H2 instance), then open an `AutoPort` fixture;
   - assert fixture address is not the occupied endpoint and its database is usable.

4. Keep `TestRealH2PostgresWire`, now routed through the fixture utility, as the end-to-end PG-wire/schema smoke test.

Do not mark the existing suite globally parallel merely to exercise this. The new regression test deliberately introduces bounded overlap; it must use timeouts/deadlines so an H2 startup failure does not hang the package.

### Commands and acceptance gates

Run from `/Users/ab/workspace/go-projects/zenbot` with a valid `H2_JAR` when the local fallback is unavailable:

```bash
# New ownership/isolation regression, repeated to expose lifecycle races.
go test ./internal/repository/h2 -run 'TestAutoPort(FixturesAreConcurrentAndIsolated|FixtureCloseDoesNotInterruptSibling|DoesNotAdoptOccupiedLegacyPort)' -count=25 -parallel=8 -timeout=2m

# Full H2 repository package, repeated; includes all openTestDB consumers.
go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m

# H2-backed consumers that formerly selected/reused ports independently.
go test ./internal/command ./internal/service -count=5 -parallel=8 -timeout=5m

# Required migration acceptance gate, fresh and uncached.
go test ./... -count=1 -parallel=8 -timeout=10m
```

A passing focused test is not sufficient. The correction is accepted only when all commands pass, there is no listener/process leak attributed to the command run, and the final broad command is green. If a pre-existing unrelated H2/Java listener remains, the auto-port fixtures must still pass; its existence must not be used as a reason to waive the gate.

## 6. Complexity, risk, and mitigations

| Item | Assessment | Mitigation |
|---|---|---|
| Code size | Small-to-medium: one opt-in `Config` field, auto-port startup/output parsing, one fixture utility, six fixture-construction conversions, and focused regressions. | Preserve legacy `AutoPort:false` behavior; do not refactor repository SQL or schema. |
| Startup-output contract | Moderate. Auto-port discovery depends on H2's emitted PG startup URL. | Pin H2 `2.3.232`, parse a strict URL, test parse failures, and make a changed/unparseable line fail loudly. |
| Pipe/process handling | Moderate. Incorrect pipe draining or kill/wait can leak/hang Java. | Drain output through exit; single owned command; kill-and-wait every failed startup path; deadline-bound tests. |
| Existing production semantics | Low. | `AutoPort` is opt-in; preserve `Port == 0 => 5435` and legacy managed-listener behavior for existing callers. |
| Test speed | Moderate increase from one Java H2 process per fixture. | This is intentionally the smallest robust ownership model. Reconsider a synchronized shared manager only if measured suite time is unacceptable after correctness is established. |

## 7. Exclusions

- No migration query, command, schema, model, or production business-logic change.
- No change to `cmd/zenbot/main.go` configuration or its default `5435` behavior.
- No change to legacy non-test `h2.Open` adoption semantics in this minimal correction; a later hardening task may remove/validate that behavior separately.
- No package-level shared H2 manager, global bootstrap lock, port registry, arbitrary retry loop, or PID-derived port scheme.
- No modification of `.hermes/handoffs/identity-slice-h2-broad-gate-diagnostic.md` or other protected migration documents.
- No claims that a green isolated identity test alone validates the migration; only the post-fix broad gate does.

## 8. Completion checklist

- [ ] `Config.AutoPort` is opt-in and `Port == 0` legacy behavior remains unchanged.
- [ ] Auto-port H2 child endpoint is parsed, validated, recorded, and used for DSN construction.
- [ ] No auto-port fixture takes the TCP-probe/adoption branch.
- [ ] All six real-H2 fixture constructors use `h2fixture.Open`; `openTestDB` remains the sole H2-package wrapper.
- [ ] Cleanup reports errors and waits for the owned Java child before temp directory deletion.
- [ ] Concurrent-isolation, sibling-close, and occupied-legacy-port regressions pass repeatedly.
- [ ] Full H2, consumer, and `go test ./...` gates pass uncached.
- [ ] `git diff` contains only task-owned harness/test files; application logic and protected handoffs are untouched.
