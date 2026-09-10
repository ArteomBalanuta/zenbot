# H2 AutoPort empty-version cleanup: replacement architecture

**Status:** proposed implementation specification. The persisted-H2 `H2VERSION` override is conclusively rejected for pinned H2 2.3.232. The replacement is a minimal, per-call, same-package identity-reader seam that keeps the Java H2 child, AutoPort discovery, PGX connection, `PingContext`, SQL pool, and lifecycle cleanup real.

## 1. Evidence and decision

[OBSERVED] `internal/repository/h2/database.go:Open` creates one local `*processServer`, calls `s.Start(ctx)`, builds a PGX-backed `*sql.DB` against `s.Addr()`, clears PGX runtime parameters, and runs `db.PingContext(ctx)` before reading H2 identity. The present identity code is:

```go
var version string
if err = db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version); err != nil {
    db.Close()
    _ = s.Stop(context.Background())
    return nil, fmt.Errorf("H2 identity check failed: %w", err)
}
if version == "" {
    db.Close()
    _ = s.Stop(context.Background())
    return nil, errors.New("H2 identity check returned empty version")
}
```

[OBSERVED] In AutoPort mode, `processServer.startAutoPort` starts `java -cp <jar> org.h2.tools.Server -pg -pgPort 0 -ifNotExists -baseDir <dir>`, parses the announced loopback endpoint, and calls `watch`. `watch` retains the owned `*exec.Cmd`, creates `waitDone`, waits once, stores `waitErr`, then closes `waitDone` (`internal/repository/h2/database.go:117-200`). `Stop` is idempotent via `stopOnce`; its owned-child path interrupts, waits for `waitDone`, or kills and waits after deadline expiry (`database.go:202-242`).

[TEST-BACKED] `TestRealH2PostgresWire` uses a real H2 PG-wire fixture, verifies a non-empty `SELECT H2VERSION()`, and performs a real insert/query (`internal/repository/h2/database_test.go`). Existing external-package AutoPort tests verify independent fixtures, sibling survival, and avoidance of an occupied legacy listener (`internal/repository/h2/database_autoport_regression_test.go`). They cannot name unexported package details.

[LIMITATION] The forensic investigation recorded in `.hermes/handoffs/h2-empty-version-cleanup-alternative-diagnostic.md` found that every tested persisted override candidate fails on fresh H2 2.3.232 PG reopen with metadata collision or PG EOF. It cannot deterministically reach `version == ""` through a new PG connection. Do not reintroduce a persisted alias, `BUILTIN_ALIAS_OVERRIDE`, `FORCE` alias, alternate schema, or schema-search-path setup.

[RECOMMENDED] Add only an unexported per-`Config` hook bundle. It captures the exact server after successful startup and replaces only the identity-result boundary. Its nil/default path retains the literal production query and existing error wrapping. This produces a deterministic test of the existing empty-version cleanup while retaining real H2 startup, AutoPort selection, PGX `PingContext`, and process termination.

## 2. Exact file and interface map

### Files changed

| Path | Required change | Reason |
|---|---|---|
| `internal/repository/h2/database.go` | Add `h2OpenTestHooks`, unexported `Config.testHooks`, default `readH2Version`, one post-start observation call, and default-or-hook identity-reader selection. | `Open` owns both the real child and the otherwise unreachable empty-version branch. |
| `internal/repository/h2/database_empty_version_cleanup_test.go` (new, `package h2`) | Add one real AutoPort regression: `TestOpenAutoPortEmptyH2VersionClosesOwnedChild`. | Same-package access is required to set `testHooks` and assert the captured `processServer`'s owned command/wait channel. |

### Files deliberately unchanged

| Path | Why it stays unchanged |
|---|---|
| `internal/repository/h2/database_test.go` | `TestRealH2PostgresWire` remains the normal-path proof that real H2 identity over PG wire is nonempty. |
| `internal/repository/h2/database_autoport_test.go` | Public `AutoPort` compatibility coverage is unrelated. |
| `internal/repository/h2/database_autoport_regression_test.go` | Its `package h2_test` boundary intentionally prevents use of the seam; isolation coverage remains independent. |
| `internal/testutil/h2fixture/h2fixture.go` | The successful-fixture helper must not acquire exceptional-state support. It establishes the current `H2_JAR`-then-pinned-path convention. |
| `internal/repository/h2/audit.go` | `Database.Close` already closes DB then stops `Server`; no returned `Database` exists on this failure branch. |

### Proposed internal API

Add near `Config` in `internal/repository/h2/database.go`:

```go
type h2OpenTestHooks struct {
    // onStarted observes the exact successfully started owned server.
    onStarted func(*processServer)
    // h2Version replaces only Open's H2 identity reader in same-package tests.
    h2Version func(context.Context, *sql.DB) (string, error)
}

// Config describes the externally managed H2 PostgreSQL server.
type Config struct {
    BaseDir, DatabaseStem, Host string
    Port                        int
    AutoPort                    bool
    H2Jar, Java                 string
    StartupTimeout              time.Duration

    // testHooks is nil in production and cannot be set outside package h2.
    testHooks *h2OpenTestHooks
}

func readH2Version(ctx context.Context, db *sql.DB) (string, error) {
    var version string
    err := db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)
    return version, err
}
```

`readH2Version` is not a new abstraction for callers. It is the exact existing query and scan, factored only so the nil/default path remains explicit and reviewable.

## 3. Hook ordering and default invariants

Modify `Open` in this order, without moving the existing cleanup branches:

```go
s := &processServer{cfg: c}
if err := s.Start(ctx); err != nil {
    return nil, err
}
if c.testHooks != nil && c.testHooks.onStarted != nil {
    c.testHooks.onStarted(s)
}

// Existing DSN parsing, sql.OpenDB configuration, and PingContext remain here.
...
if err = db.PingContext(ctx); err != nil { ... }

readVersion := readH2Version
if c.testHooks != nil && c.testHooks.h2Version != nil {
    readVersion = c.testHooks.h2Version
}
version, err := readVersion(ctx, db)
if err != nil {
    db.Close()
    _ = s.Stop(context.Background())
    return nil, fmt.Errorf("H2 identity check failed: %w", err)
}
if version == "" {
    db.Close()
    _ = s.Stop(context.Background())
    return nil, errors.New("H2 identity check returned empty version")
}
```

Required invariants:

1. **Per-call and unexported:** `testHooks` and `h2OpenTestHooks` are lowercase and live only in one `Config` value. No package globals, exported factory, mutable registry, or public API is added.
2. **Success-only observation:** `onStarted` runs exactly once only after `s.Start(ctx)` returned `nil`. It does not run for Java/jar prerequisites, endpoint discovery, or readiness failures.
3. **Exact object identity:** the callback receives the same local `s` later stopped by the empty-version branch—not a `Server` interface, clone, wrapper, or fake.
4. **Ordering protects the real wire path:** the callback executes before DSN/PGX work solely so the exact future child can be observed. The reader is selected and invoked only after the unmodified successful `db.PingContext(ctx)`. Therefore an expected empty-version result is impossible if real PGX ping fails.
5. **Strict default preservation:** when `testHooks` is nil, or its `h2Version` is nil, `Open` calls `readH2Version`, which executes exactly `db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)`. Query-error wrapping remains exactly `H2 identity check failed: %w`.
6. **Existing cleanup preservation:** the empty value still executes `db.Close()`, then `_ = s.Stop(context.Background())`, then returns exactly `H2 identity check returned empty version`. Do not change error handling, stop timeout policy, bootstrap, or `Database.Close`.
7. **Observation-only callback:** `onStarted` must only save its pointer in the test. It must not block, mutate `s`, call `Stop`, start another child, or create lifecycle synchronization. `h2Version` returns only the test identity value/error; it must not replace the DB, Ping, server, or cleanup.

## 4. Regression test algorithm

Create `internal/repository/h2/database_empty_version_cleanup_test.go` with `package h2`.

1. Resolve the already-established real-H2 prerequisite: `jar := os.Getenv("H2_JAR")`, falling back to `/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar`; require `os.Stat(jar)` succeeds. This is the convention in `internal/testutil/h2fixture/h2fixture.go`.
2. Set a whole-test deadline: `ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)` and `defer cancel()`. Use `dir := t.TempDir()` and `stem := filepath.Join(dir, "empty-version")`.
3. Declare `var started *processServer` and `identityRead := false`. Configure only the proposed hooks:

   ```go
   hooks := &h2OpenTestHooks{
       onStarted: func(s *processServer) { started = s },
       h2Version: func(context.Context, *sql.DB) (string, error) {
           identityRead = true
           return "", nil
       },
   }
   ```

   These assignments occur synchronously in `Open`; no mutex, channel, fake database, or cleanup spy is needed.
4. Invoke normal `Open`:

   ```go
   db, err := Open(ctx, Config{
       BaseDir:        dir,
       DatabaseStem:   stem,
       H2Jar:          jar,
       Host:           "127.0.0.1",
       AutoPort:       true,
       StartupTimeout: 5 * time.Second,
       testHooks:      hooks,
   })
   ```

5. Require `db == nil`, `identityRead == true`, and `err != nil` with `err.Error() == "H2 identity check returned empty version"`. Any startup, endpoint, PGX ping, query, or bootstrap error is a test failure rather than an alternative pass condition. Since `h2Version` is called only after `PingContext` returns nil, this confirms the normal real PGX ping gate was passed before the injected result.
6. Require `started != nil`, `started.cmd != nil`, `started.cmd.Process != nil`, and `started.Addr() != ""`. These establish that the hook captured a real successfully-started AutoPort server, with a real owned Java child and discovered endpoint.
7. Before any cleanup by the test, require `started.cmd.ProcessState != nil && started.cmd.ProcessState.Exited()`. Then non-blockingly require that `started.waitDone` is already closed:

   ```go
   select {
   case <-started.waitDone:
   default:
       t.Fatal("owned H2 child waitDone is not closed after Open returned")
   }
   ```

   `ProcessState.Exited` and closed `waitDone` are direct per-instance proof that the exact captured child was stopped and reaped by `Open` before it returned.
8. Finally call `started.Stop(context.Background())` and require nil only to observe existing idempotence after the proof. Do not use this call, `defer`, or `t.Cleanup` to satisfy the termination assertions.

### Proposed end-to-end sequence

```text
[PROPOSED regression]
Test -> Open(Config{AutoPort:true, testHooks})
  -> processServer.Start / startAutoPort
  -> Java H2 child C, OS-selected loopback PG port P, watched by S.waitDone
  -> hooks.onStarted(S) captures exact S/C
  -> PGX sql.DB connects to P; db.PingContext(ctx) succeeds
  -> hooks.h2Version(ctx, db) returns "" (only injected boundary)
  -> db.Close()
  -> S.Stop(background) -> interrupt/wait C -> S.waitDone closes
  -> return nil, "H2 identity check returned empty version"
Test -> assert C.ProcessState.Exited and S.waitDone already closed
```

## 5. TDD and acceptance commands

Run from repository root. The checked real-H2 prerequisite is currently available at the pinned path; CI or another machine must set `H2_JAR` if that local fallback does not exist.

```bash
# RED: add the same-package test first. It must fail to compile because
# Config.testHooks and h2OpenTestHooks do not exist yet.
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=1 -timeout=30s

# GREEN: add only the hook bundle, default reader, and ordered calls above.
# Repeat to expose child-cleanup races.
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m

# Regression gate: retain real-H2 normal-wire and AutoPort behavior.
go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m
```

Acceptance requires every focused repetition to return the exact empty-version error and find the captured owned child exited with `waitDone` already closed. The normal package gate must remain green, including `TestRealH2PostgresWire` and the AutoPort tests.

## 6. Scope exclusions, complexity, and regression prevention

**Scope exclusions:** no persisted H2 aliases/settings/schema tricks; no fake H2 server, fake `sql.DB`, fixed port, reserved listener, host-wide Java/listener scan, lock-file assertion, cleanup spy, package-global hook, exported hook/API, generic database/server factory, `Server` interface change, PGX configuration change, AutoPort/parser/lifecycle redesign, schema/bootstrap change, fixture-helper change, migration change, or protected handoff modification.

**Complexity and risk:** **low complexity / moderate integration risk.** Production scope is one private hook bundle, one default query helper, and two guarded selection/callback sites; test scope is one integration regression. The remaining risk is environmental (Java and pinned H2 availability) and child-process timing, controlled by existing AutoPort, loopback-only endpoint rules, `t.TempDir`, unique database stem, time-bounded context, repeated focused execution, and direct `exec.Cmd`/`waitDone` assertions.

**Regression prevention:** retain `readH2Version`'s literal query and error wrapping as review invariants; ensure all production `Config` instances leave `testHooks` nil; keep the test in `package h2` so external callers cannot configure the seam; require `identityRead`, exact error, exited command, and closed wait channel in the same test; and run the focused repeat plus package-repeat gates above. Any future attempt to broaden the hook beyond identity read or to substitute lifecycle/network components should be rejected as out of scope.
