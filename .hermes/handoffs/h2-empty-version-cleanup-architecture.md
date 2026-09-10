# H2 AutoPort empty-version cleanup: implementation specification

**Status:** approved, minimal testability correction. This specification adds an internal observation seam and one real-H2 regression only; it does not alter H2 behavior for callers.

## 1. Evidence and decision

[OBSERVED] `internal/repository/h2/database.go:Open` creates one local `processServer`, successfully starts it, opens a PGX-backed `*sql.DB` to `s.Addr()`, then performs the real wire query `SELECT H2VERSION()`.

[OBSERVED] On `version == ""`, `Open` currently executes `db.Close()`, then `_ = s.Stop(context.Background())`, and returns `errors.New("H2 identity check returned empty version")` (`database.go:289-292`). `processServer.Stop` delegates to `stopOwned`, which waits for the child completion channel (`waitDone`) after interrupting it; timeout handling kills and then waits (`database.go:202-242`). No `*Database` is returned on this branch, so its locally owned `processServer` is otherwise unobservable by a caller.

[TEST-BACKED] AutoPort tests currently prove fixture isolation, sibling survival, and no adoption of an occupied listener in `internal/repository/h2/database_autoport_regression_test.go`. They do not cause the identity query to return an empty value. Normal real H2 is asserted nonempty by `TestRealH2PostgresWire` in `internal/repository/h2/database_test.go`.

[EXPERIMENT] With the pinned H2 2.3.232 jar, the following real Shell command succeeded and returned one empty result row:

```sql
SET BUILTIN_ALIAS_OVERRIDE TRUE;
CREATE ALIAS H2VERSION AS $$String h2version() { return ""; }$$;
SELECT H2VERSION();
```

**Decision:** add an unexported, per-`Config` callback that observes the exact started `processServer`; add one same-package regression that seeds the persisted H2 alias before calling the normal AutoPort `Open` path. This reaches the existing cleanup branch through a real H2 PostgreSQL-wire query and proves the exact owned child has exited before `Open` returns.

## 2. Module and file map

| Path | Responsibility | Required change |
|---|---|---|
| `internal/repository/h2/database.go` | Defines `Config`, starts owned AutoPort Java children, and owns every early-return cleanup in `Open`. | Add the unexported observation field and invoke it once after successful `s.Start(ctx)`. Do not change the existing empty-version cleanup statements. |
| `internal/repository/h2/database_empty_version_cleanup_test.go` **(new)** | Same-package (`package h2`) end-to-end regression; it may name `processServer` and the unexported `Config` field. | Seed the persisted alias, call `Open`, and assert the captured owned child is already exited/complete on the expected error. |
| `internal/repository/h2/database_autoport_regression_test.go` | External-package AutoPort ownership/isolation tests. | No change. Its `h2_test` package intentionally cannot use the internal callback. |
| `internal/repository/h2/database_autoport_test.go` | Public AutoPort configuration compatibility test. | No change. |
| `internal/testutil/h2fixture/h2fixture.go` | General successful real-H2 fixture helper. | No change. The regression requires a deliberately malformed persisted database and must not make the shared successful-fixture helper support that exceptional state. |
| `internal/repository/h2/audit.go` | `Database.Close` ordering: SQL pool close before server stop. | No change; the failure branch mirrors this ordering because no `Database` exists to close. |

## 3. Exact internal interface and callback invariants

Add to `Config` in `internal/repository/h2/database.go`:

```go
// onStarted is a same-package test observation hook for the successfully
// started processServer. It is nil in normal use.
onStarted func(*processServer)
```

Immediately after the successful `s.Start(ctx)` call in `Open`, add:

```go
if c.onStarted != nil {
    c.onStarted(s)
}
```

The callback contract is deliberately narrow:

1. **Unexported, per-call state:** `onStarted` is lowercase and belongs to the individual `Config`; it creates no public API or package-global mutable state.
2. **Success-only:** it is not invoked if `s.Start(ctx)` returns an error, including Java/jar prerequisite, endpoint-discovery, or readiness failures.
3. **Exact ownership identity:** the pointer argument is the same `s` created at `Open` and later passed to the empty-version `_ = s.Stop(context.Background())` call; it is not an adapter, clone, or mock.
4. **Ordering:** invoke it synchronously after `Start` has returned `nil` and before DSN parsing, `sql.OpenDB`, ping, identity query, bootstrap, or any later early-return cleanup. At callback time, successful AutoPort start has recorded `cmd`, `waitDone`, and `addr` (`processServer.startAutoPort` / `watch`).
5. **Nil compatibility:** a nil callback is a strict no-op. Existing external callers cannot set this field, and no default production behavior changes.
6. **Observation only:** the regression callback only stores its argument. It must not call `Stop`, mutate fields, block, launch a replacement server, or use a global synchronization mechanism. `Open` remains the only lifecycle owner on the expected failure path.
7. **No error channel:** do not add callback return/error semantics, factories, injectable SQL/query functions, a `Server` fake, or public test API. Such seams either widen production scope or bypass the real `H2VERSION()` query.

## 4. Real-H2 regression algorithm

Create `TestOpenAutoPortEmptyH2VersionClosesOwnedChild` in the new same-package test file.

### A. Seed only the persisted database state

1. Use `dir := t.TempDir()` and `stem := filepath.Join(dir, "empty-version")`.
2. Resolve the existing real-H2 test prerequisite convention: `H2_JAR` when set, otherwise the current pinned H2 2.3.232 jar path used by `internal/testutil/h2fixture/h2fixture.go`. Fail with `t.Fatal` if unavailable. Use `java` (or the test's established Java value) and run a bounded `exec.CommandContext` Shell child; do not start a PG listener for this setup.
3. Invoke `org.h2.tools.Shell` with URL `jdbc:h2:<stem>`, user `sa`, empty password, and this SQL:

   ```sql
   SET BUILTIN_ALIAS_OVERRIDE TRUE;
   CREATE ALIAS H2VERSION AS $$String h2version() { return ""; }$$;
   ```

   Capture combined output and fail setup on a nonzero exit. The Shell process must exit before `Open` starts. This is real persisted H2 state, not a mock/injected query result.
4. Use a whole-test context deadline (for example, 15 seconds) for both setup and `Open`, preventing an H2 startup or Shell regression from hanging the package.

### B. Drive `Open` and capture its actual child

```go
var started *processServer

db, err := Open(ctx, Config{
    BaseDir:        dir,
    DatabaseStem:   stem,
    H2Jar:          jar,
    Host:           "127.0.0.1",
    AutoPort:       true,
    StartupTimeout: 5 * time.Second,
    onStarted: func(s *processServer) {
        started = s
    },
})
```

`Java` may be omitted to preserve `Start`'s existing `java` default, or supplied from the same local prerequisite convention if the test already resolves it. Do not route this setup through `h2fixture.Open`: that helper calls `t.Fatal` on the intentionally expected `Open` error and hides the returned error from the regression.

### C. Assertions, in this order

1. `db` must be `nil`.
2. `err` must be non-nil and equal to, or contain, the stable text `H2 identity check returned empty version`. A normal version, query failure, timeout, or bootstrap failure is not an acceptable substitute.
3. `started`, `started.cmd`, and `started.cmd.Process` must be non-nil; `started.Addr()` must be nonempty. Together these show a real AutoPort-owned Java child was started before the expected identity failure.
4. Immediately after `Open` returns, require `started.cmd.ProcessState != nil && started.cmd.ProcessState.Exited()`. This proves `Open`'s own cleanup reached the exact owned `exec.Cmd` before returning; do not use a host-wide Java/listener scan.
5. Non-blockingly receive from `started.waitDone`; it must already be closed. For example:

   ```go
   select {
   case <-started.waitDone:
   default:
       t.Fatal("owned H2 child waitDone is not closed after Open returned")
   }
   ```

6. Call `started.Stop(context.Background())` only **after** the exited/closed assertions and require `nil`, proving existing stop idempotence without making cleanup pass. Do not register a cleanup that stops `started`; on this expected branch `Open` is the sole allowed stop owner.

Do not assert that the former random port is undialable: OS port reuse makes that flaky. Do not infer shutdown from H2 lock-file behavior: it is indirect and platform-sensitive. The captured exited `exec.Cmd` plus closed `waitDone` is the deterministic process-ownership proof.

### Proposed end-to-end sequence

```text
[PROPOSED regression]
Test Shell -> persisted <temp>/empty-version.mv.db: override H2VERSION() => ""
Shell exits
Test -> Open(Config{AutoPort:true, onStarted:capture})
     -> processServer.Start -> owned Java H2 child C on OS-selected port P
     -> callback captures C's exact processServer S
     -> PGX db.Ping; real PG wire: SELECT H2VERSION() => ""
     -> db.Close()
     -> S.Stop(background) -> interrupt/wait C -> close S.waitDone
     -> return nil, "H2 identity check returned empty version"
Test -> assert C.ProcessState.Exited and S.waitDone already closed
```

## 5. Errors and lifecycle expectations

- Preserve the existing order on this branch: close SQL before stopping the server. Ignore `db.Close`/`Stop` errors exactly as the current branch does; the regression's task is to prove the already-fixed cleanup executes, not redesign cleanup error propagation.
- The callback must not run if startup fails. Therefore a nil `started` is expected only in a startup-failure test, not in this regression after the specified error.
- If alias persistence/override stops working, the regression must fail visibly because `Open` returns a nonempty successful database, a different query error, or another non-expected error; do not add a fallback that forces `version == ""`.
- `Stop` is idempotent through `stopOnce`; the final test call observes that property after `Open` has already completed cleanup. It must not be used in `defer`/`t.Cleanup` before assertion.
- `t.TempDir`, a private stem, `AutoPort:true`, and no fixed/reserved TCP port keep the test parallel-safe. The H2 Shell is only a bounded setup child; the observed Java server is the system under test.

## 6. TDD commands and acceptance

Run from the repository root. Ensure `H2_JAR` points to H2 2.3.232 when the checked-in local fallback is unavailable.

```bash
# RED: add the new same-package regression first. Before onStarted exists,
# compilation must fail because Config has no such field.
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=1 -timeout=30s

# GREEN: callback plus regression. Repeat to expose lifecycle races.
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m

# Protect existing AutoPort semantics and full H2 behavior.
go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m
```

Acceptance is all repetitions green, with each focused run taking the exact empty-version error path and finding the observed owned child exited before `Open` returns. The focused test is not a reason to change the broader H2 isolation gates recorded in `.hermes/handoffs/h2-fixture-isolation-architecture.md`.

## 7. Scope exclusions and complexity

**Excluded:** public `Config` API additions; production configuration/default changes; query or schema changes; changes to `AutoPort`, output parsing, `processServer.Stop`, `Database.Close`, H2 fixture utility, existing AutoPort regressions, migrations, mocks/fakes, global hooks/factories, fixed-port adoption, host-wide process scans, lock-file assertions, and protected forensic handoffs.

**Complexity:** **small / low risk** — three production lines (one unexported field and a guarded callback invocation) plus one isolated integration regression and its local persisted-alias setup. The only environmental dependency is the already-pinned real H2 2.3.232 jar and Java runtime; the test must fail explicitly if either is unavailable.
