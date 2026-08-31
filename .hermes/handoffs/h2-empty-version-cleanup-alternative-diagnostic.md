# H2 AutoPort empty-version cleanup: replacement diagnostic

**Status:** persisted-database override rejected for H2 2.3.232; use a narrow per-call identity-reader seam plus observation of the real owned child. This diagnostic made no Go source or test changes.

## Definitive recommendation

Do **not** attempt another persisted `H2VERSION` alias or database-setting strategy. No tested/documented candidate produces `SELECT H2VERSION()` returning `""` after a fresh `org.h2.tools.Server -pg` open without a metadata collision/PG EOF.

The smallest valid deterministic regression is a same-package test that:

1. starts the real Java H2 AutoPort child through normal `Open` and proves the real PGX `PingContext` succeeds;
2. captures that exact `*processServer` with an unexported per-call observation hook; and
3. injects only the identity-result/query boundary to return `""`, then asserts the existing empty-version branch stops and waits for that exact child before `Open` returns.

This is not a fake server, fake `sql.DB`, fixed-port listener, host-wide process scan, or cleanup spy. H2 startup, auto-port discovery, TCP/PG wire connection, `PingContext`, SQL pool, and child termination remain real. The single injected boundary is necessary because real H2 cannot deterministically supply the exceptional identity value over a fresh PG connection.

## Code path and current cleanup behavior

[OBSERVED] `internal/repository/h2/database.go:258-299` creates a private `processServer`, calls `s.Start(ctx)`, opens a PGX-backed `*sql.DB` against `s.Addr()`, removes PGX runtime parameters, pings the real server, and then calls:

```go
 db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)
```

[OBSERVED] On `version == ""` (`database.go:289-292`), the current worktree calls `db.Close()`, calls `s.Stop(context.Background())`, and returns exactly `H2 identity check returned empty version`.

[OBSERVED] AutoPort startup launches `java -cp <H2Jar> org.h2.tools.Server -pg -pgPort 0 -ifNotExists -baseDir <dir>`, parses H2's announced loopback endpoint, and retains the owned `*exec.Cmd`, `waitDone`, and `waitErr`. `stopOwned` sends interrupt, waits for `waitDone`, and kill-plus-waits on timeout (`database.go:117-242`). Therefore `ProcessState.Exited()` and a closed `waitDone` are deterministic, per-fixture proof of cleanup after `Open` returns.

[TEST-BACKED] `go test ./internal/repository/h2 -run '^TestRealH2PostgresWire$' -count=1 -timeout=45s` passed in 1.037 s using the pinned H2 2.3.232 fixture. Its post-command scan found no `org.h2.tools.Server`/H2-jar process and no Java TCP listener.

## Bounded persisted-state investigation

### Documentation constraint

H2's command reference describes `SET BUILTIN_ALIAS_OVERRIDE` as allowing overrides of **builtin system date/time functions for unit testing**. It does not document a persistent override mechanism for `H2VERSION`. H2's `SET SCHEMA_SEARCH_PATH` documentation also says it changes the **current connection** and that the default schema for new connections is `PUBLIC`; a Shell session's search path cannot configure the later PG client connection.

Source: [H2 Commands — SET BUILTIN_ALIAS_OVERRIDE / SET SCHEMA_SEARCH_PATH](https://www.h2database.com/html/commands.html), retrieved 2026-08-31.

### Fresh-server experiments

All experiments used Java 23.0.1 and `/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar`. A temporary Go probe created an isolated temporary database per candidate with `org.h2.tools.Shell`, then started a fresh process with the exact AutoPort flags above, connected through PGX with `RuntimeParams = map[string]string{}`, ran `SELECT H2VERSION()`, and deferred interrupt/wait (kill/wait fallback after five seconds). The temporary probe directory was removed after execution.

| Candidate persisted Shell SQL | Fresh PG result | Conclusion |
|---|---|---|
| `SET BUILTIN_ALIAS_OVERRIDE TRUE; CREATE ALIAS H2VERSION AS $$String h2version() { return ""; }$$;` | PG client received unexpected EOF | Reproduces the disproven premise. The earlier direct validation captured H2's reopen failure: `Function alias "H2VERSION" already exists [90076-232]`, from `CREATE FORCE ALIAS "PUBLIC"."H2VERSION" ...`. |
| Same alias, followed by `SET BUILTIN_ALIAS_OVERRIDE FALSE` | PG client received unexpected EOF | Resetting the setting does not avoid the reopen-time alias conflict. |
| `CREATE FORCE ALIAS H2VERSION ...` under `BUILTIN_ALIAS_OVERRIDE TRUE` | PG client received unexpected EOF | `FORCE` does not make persisted metadata coexist with H2's reopen alias creation. |
| Alias in `ALT` schema plus `SET SCHEMA_SEARCH_PATH ALT, PUBLIC` | PG client received unexpected EOF | An alternate-schema alias still does not survive the PG reopen cleanly; independently, search path is connection-local and the new client defaults to `PUBLIC`. |
| `SET MODE PostgreSQL` control (no alias) | `VALUE "2.3.232"` | Fresh AutoPort PG connectivity and normal H2 identity work; PostgreSQL mode does not supply an empty version. |

The original implementation-stage validation independently produced the same canonical-alias failure and recorded the full Java stack trace in `.hermes/handoffs/h2-empty-version-cleanup-implementation.md:37-53`. This investigation expands it with the bounded `FALSE`, `FORCE`, and alternate-schema variants rather than treating the first syntax as conclusive.

### Cleanup evidence

Every temporary probe trial reported `server-stop=exit status 130`, meaning the probe's deferred interrupt/wait reaped its owned Java process. Immediately after the complete run:

```text
POST_PROBE_PROCESSES
<no matching org.h2.tools.Server or h2-2.3.232.jar process>
POST_PROBE_JAVA_LISTENERS
<no Java TCP listener>
```

The temporary probe was deleted. A subsequent real package test also passed and left the same process/listener checks empty. No Java/H2 process or listener remains from this diagnostic.

## Required future test seam (not implemented here)

Make the change only in these two files:

| File | Change |
|---|---|
| `internal/repository/h2/database.go` | Add an unexported, per-`Config` test-hooks field and use it only to observe a successful start and replace the `H2VERSION` reader. Default behavior remains the real query. |
| `internal/repository/h2/database_empty_version_cleanup_test.go` (new, `package h2`) | One end-to-end AutoPort cleanup regression described below. |

Suggested exact internal interface:

```go
type h2OpenTestHooks struct {
    // onStarted observes the exact successfully started server; it does not mutate it.
    onStarted func(*processServer)
    // h2Version replaces only the identity-query boundary in a same-package test.
    h2Version func(context.Context, *sql.DB) (string, error)
}

// Config: unexported field; nil in all production callers.
testHooks *h2OpenTestHooks
```

Use a default reader with the current production query:

```go
func readH2Version(ctx context.Context, db *sql.DB) (string, error) {
    var version string
    err := db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)
    return version, err
}
```

Immediately after successful `s.Start(ctx)`, invoke `testHooks.onStarted(s)` when non-nil. Immediately before the existing identity query, choose `readH2Version` unless `testHooks.h2Version` is non-nil. Preserve the current wrapping of a query error and the current empty-version cleanup statements exactly.

The field is unexported on an exported `Config`, so only same-package tests can set it. Avoid package globals, exported factories, fake `Server` implementations, or a generic database factory. The hook must not run before successful start, block, call `Stop`, mutate `s`, or expose lifecycle controls to external callers.

## Exact regression algorithm

Test name: `TestOpenAutoPortEmptyH2VersionClosesOwnedChild` in `package h2`.

1. Create `ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)` and `defer cancel()`; use `dir := t.TempDir()` and `stem := filepath.Join(dir, "empty-version")`.
2. Resolve the existing pinned jar convention (environment `H2_JAR`, otherwise `/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar`); use host `127.0.0.1`, `AutoPort: true`, and a five-second startup timeout.
3. Set hooks only as follows:

   ```go
   var started *processServer
   hooks := &h2OpenTestHooks{
       onStarted: func(s *processServer) { started = s },
       h2Version: func(context.Context, *sql.DB) (string, error) { return "", nil },
   }
   ```

4. Call normal `Open(ctx, Config{BaseDir: dir, DatabaseStem: stem, H2Jar: jar, Host: "127.0.0.1", AutoPort: true, StartupTimeout: 5*time.Second, testHooks: hooks})`.
5. Assert `db == nil` and `err.Error() == "H2 identity check returned empty version"`. A startup, ping, query, or bootstrap error is a failure, not an accepted alternative.
6. Assert `started`, `started.cmd`, and `started.cmd.Process` are non-nil and `started.Addr()` is nonempty. These facts establish that real AutoPort created an owned child before the injected identity result is used. The unmodified normal path already performs real PGX `PingContext` before the boundary.
7. Before any test cleanup, assert `started.cmd.ProcessState != nil && started.cmd.ProcessState.Exited()`, then non-blockingly receive from `started.waitDone`. Both must already be complete when `Open` returned.
8. Finally call `started.Stop(context.Background())` and require nil solely to confirm idempotence. Do not use this call, a `defer`, or `t.Cleanup` to make the ownership assertion pass. `Open` must be the only actor that stops the child on the expected branch.

Focused acceptance command after the future change:

```bash
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m
```

Then run the package's existing H2 suite. The focused test must pass every repetition with the exact error and already-exited captured child.

## Risks and controls

| Risk | Control |
|---|---|
| The injected reader no longer exercises the literal `SELECT H2VERSION()` statement on the exceptional branch. | Keep `readH2Version` as the nil/default production reader and retain `TestRealH2PostgresWire` as the real-wire normal-path check. The test exercises the branch that cannot be produced by H2 persistence. |
| Test accidentally fakes lifecycle behavior. | Hook only captures the real `processServer`; it does not replace `Start`, `Stop`, the SQL pool, or the PG server. Assert actual `exec.Cmd` exit and `waitDone` closure. |
| Parallel interference or accidental legacy-port adoption. | `t.TempDir`, unique stem, `AutoPort:true`, loopback host, and no fixed/reserved port. |
| A failed assertion leaks a child. | The expected branch must stop it before `Open` returns; the whole-test context bounds startup. The test does not use cleanup to mask a lifecycle failure. |
| Production API expands. | `testHooks` and its type are unexported, per call, nil by default, and confined to the h2 package. |

## Scope / repository state

This diagnostic created only this replacement report. It did not modify Go sources, Go tests, protected files, or the pre-existing unrelated dirty worktree changes. `git diff --check` had already returned exit 0 during investigation.
