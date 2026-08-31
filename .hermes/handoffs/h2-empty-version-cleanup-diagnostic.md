# H2 AutoPort empty-version cleanup diagnostic

**Status:** forensic testability/remediation handoff. No application source was changed for this diagnostic.

## Decision

Use a **real H2 persisted-database override plus a narrowly scoped server-observation hook**. It is the smallest deterministic approach that reaches the existing `Open` branch through the real PG wire query, starts a real AutoPort-owned Java child, and proves that exact child has exited before `Open` returns its empty-version error.

The recommended test-state seam is an unexported, per-call `Config` callback invoked immediately after successful `s.Start(ctx)`, for same-package H2 tests only. It is not a mock of cleanup or a replacement for the real process:

```go
// in Config; unexported so external callers cannot set it
onStarted func(*processServer)

// in Open, immediately after successful s.Start(ctx)
if c.onStarted != nil {
    c.onStarted(s)
}
```

This is a three-line behavioral observation seam (field plus invocation), carries no default behavior change, has no global mutable state, and exposes the actual `processServer` that `Open` later stops. Do **not** add a fake `Server`, fake `sql.DB`, global factory, or a test-only bypass of `H2VERSION()`; those would not prove owned AutoPort cleanup.

## Evidence map

| Classification | Evidence |
|---|---|
| [OBSERVED] | `internal/repository/h2/database.go`, `Open`, creates `s := &processServer{cfg: c}`, calls `s.Start(ctx)`, constructs a PGX-backed `*sql.DB`, pings it, then executes `db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)`. |
| [OBSERVED] | In the empty-string branch of `Open`, `db.Close()` is called, then `_ = s.Stop(context.Background())`, then `errors.New("H2 identity check returned empty version")` is returned. This cleanup was added in the uncommitted AutoPort diff. |
| [OBSERVED] | `processServer.startAutoPort` launches Java with `-pgPort 0`, parses the announced endpoint, stores the `*exec.Cmd`, and starts `watch`, whose `cmd.Wait()` completion closes `waitDone`. `Stop` uses `stopOnce` and waits on that same `waitDone`; it only no-ops when `cmd == nil`. |
| [OBSERVED] | `Database.Close` in `internal/repository/h2/audit.go` closes SQL first and calls `Server.Stop(context.Background())` second. The empty-version branch duplicates this ordering because no `*Database` is returned. |
| [TEST-BACKED] | `database_autoport_regression_test.go` exercises concurrent fixtures, sibling close behavior, and non-adoption of an occupied listener. It has no test that makes `Open` receive an empty identity value. `database_test.go` only asserts that a normal real H2 returns nonempty `H2VERSION()`. |
| [EXPERIMENT] | A local H2 2.3.232 Shell probe successfully executed `SET BUILTIN_ALIAS_OVERRIDE TRUE; CREATE ALIAS H2VERSION AS 'String h2version() { return ""; }'; SELECT H2VERSION();`. Its result column was `PUBLIC.H2VERSION()` with an empty value. This establishes an H2-native way to produce the needed value without altering application query code. |
| [LIMITATION] | The probe was one H2 Shell session. The proposed regression must create the persisted alias in the fixture database before `Open`; the regression itself is the required proof that the setting/alias survives the close/reopen boundary used by the real AutoPort server. |

## Root cause and ownership/data flow

The new failure path is inherently unreachable with the standard real-H2 fixture because normal H2 2.3.232 returns a nonempty version. `Open` owns the Java child only inside its local `processServer`; it returns no `Database` on the identity failure, so a caller cannot inspect `Database.Server`, its announced address, process handle, or completion channel after the branch runs.

```text
Config{AutoPort:true, BaseDir:D, DatabaseStem:D/db}
  -> Open normalizes stem to "db" and creates processServer S
  -> S.Start / S.startAutoPort
       -> Java H2 child C binds OS-selected loopback port P
       -> S records C, address P, and waitDone from C.Wait
  -> Open dials postgres://sa@P/db and runs SELECT H2VERSION()
  -> empty version
       -> db.Close()
       -> S.Stop(background) -- interrupt; wait C; kill+wait only on deadline
       -> Open returns (nil, "H2 identity check returned empty version")
```

The defect risk is a resource escape on a failure path after both resources are acquired: an open PGX pool can retain connections and an owned Java child can retain its listener/base-directory lock. The current code intends cleanup, but no test proves that the branch reaches it, that it targets the owned process rather than a legacy adopted listener, or that `Open` does not return before the child exits.

## Ranked strategies

1. **Recommended: persisted H2 override + observed real `processServer`.** Pre-create the exact file database with H2 Shell/JDBC, set `BUILTIN_ALIAS_OVERRIDE`, and persist an `H2VERSION` alias that returns `""`. Then use `Open(AutoPort:true)` normally. A per-call callback retains the real `*processServer` only for assertions. This drives the actual SQL query and exact owned process lifecycle. Deterministic; no port reservation or external listener is involved.
2. **Acceptable but weaker: persisted H2 override + filesystem-only proof.** Same real empty query, but infer cleanup from removal/reacquisition of the H2 lock. Reject as the primary test: lock behavior is H2/platform-specific and does not directly identify the child/listener.
3. **Weaker unit/integration hybrid: inject an identity-query function or `sql.OpenDB` factory.** It can force `version == ""` while a real child starts, but it bypasses the actual `SELECT H2VERSION()` PG-wire behavior. Use only if H2 stops supporting the alias override; it is not the preferred regression.
4. **Reject: fake `Server`/spy `Stop`.** Verifies a helper call but does not launch or terminate a child/listener.
5. **Reject: fake TCP/PG server or fixed-port adoption.** Reintroduces the very ownership/adoption ambiguity AutoPort was designed to eliminate, and `Open` would not own the listener.
6. **Reject: inspect Java processes/listeners by name after return.** It is host-global, races parallel tests, and cannot attribute a process to this fixture.

## Exact minimal test outline

Add one same-package test, preferably in a new `internal/repository/h2/database_empty_version_cleanup_test.go`, so it can set the unexported per-call observation callback.

### Helper: preseed a deterministic empty-version database

1. Obtain the existing real-H2 test prerequisites (the pinned 2.3.232 jar and Java command used by `openTestDB`/`h2fixture`).
2. Allocate `dir := t.TempDir()` and `stem := filepath.Join(dir, "empty-version")`.
3. Run the H2 Shell against `jdbc:h2:<stem>` before `Open`:

   ```sql
   SET BUILTIN_ALIAS_OVERRIDE TRUE;
   CREATE ALIAS H2VERSION AS 'String h2version() { return ""; }';
   ```

   Fail the test on a nonzero Shell exit. This is setup of the database state, not a mock. The shell exits before AutoPort starts, so no listener or process is shared with the system under test.

### Test: `TestOpenAutoPortEmptyH2VersionClosesOwnedChild`

1. Declare `var started *processServer`.
2. Call `Open(context.Background(), Config{...})` with:
   - `BaseDir: dir`
   - `DatabaseStem: stem`
   - `H2Jar` and `Java` from the established test prerequisite helper
   - `Host: "127.0.0.1"`
   - `AutoPort: true`
   - `StartupTimeout: 5 * time.Second`
   - `onStarted: func(s *processServer) { started = s }`
3. Assert `db == nil` and `err` exactly matches/contains `H2 identity check returned empty version`. If the persisted alias does not survive into the PG server, the test fails before this assertion; it cannot pass spuriously by injecting the result.
4. Assert `started != nil`, `started.cmd != nil`, `started.cmd.Process != nil`, and `started.Addr() != ""`. These prove the test reached a real AutoPort child rather than failing in setup.
5. Assert `started.cmd.ProcessState != nil && started.cmd.ProcessState.Exited()` after `Open` returns. `Stop` waits on the child’s `waitDone`, so this is an exact process-ownership assertion, not an eventual leak scan.
6. Non-blockingly receive from `started.waitDone`; fail if it is not already closed. Assert a second `started.Stop(context.Background())` returns nil (idempotence after cleanup).
7. Do **not** call `started.Stop` to make the assertion pass and do not register a cleanup that masks the result. The only allowed cleanup owner in the test path is `Open`'s empty-version branch.

A context deadline around the whole test (for example `context.WithTimeout(context.Background(), 15*time.Second)`) bounds a startup regression. Do not assert that a random port cannot be dialed after shutdown: port reuse can make that check flaky. The exited exact `exec.Cmd` plus closed `waitDone` is the deterministic proof that the owned child—and therefore its listener—was terminated before return.

## Minimal remediation

The branch itself already contains the intended cleanup. Make only these changes:

1. `internal/repository/h2/database.go`: add the unexported per-instance `Config.onStarted func(*processServer)` and invoke it immediately after `s.Start(ctx)` succeeds in `Open`.
2. `internal/repository/h2/database_empty_version_cleanup_test.go` (new): H2 persisted-alias setup and the one end-to-end cleanup regression above.

No fixture helper conversion, public API, migration SQL, production configuration, or protected handoff needs modification for this testability correction. Keeping the callback unexported prevents external production callers from treating lifecycle observation as an API contract.

## Risks and controls

| Risk | Control |
|---|---|
| H2 changes alias-override syntax/behavior | Pin existing H2 2.3.232 test prerequisite; the regression fails visibly at the expected empty-version error rather than silently skipping the branch. |
| Callback adds a test-only production field | Keep it unexported, per-config, nil by default, synchronous, and invoke only after successful start. Do not use a package-global hook. |
| Test leaks a child if an assertion fails | `Open` must already have stopped it before returning the expected error. If setup fails before `Open`, Shell is a bounded child waited by `exec.CommandContext`. |
| Parallel-suite interference | Use `t.TempDir`, AutoPort, and no fixed/reserved port. The test owns its database file and server. |
| Cleanup assertion is accidentally satisfied by test code | The test never closes the observed server before assertions; it only checks state produced by `Open`. |

## Acceptance command

```bash
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m
```

Acceptance requires all 25 repetitions to pass, including the exact empty-version error and exited observed child on every iteration. Follow with the existing H2 package and repository gates already listed in `h2-fixture-isolation-architecture.md`.
