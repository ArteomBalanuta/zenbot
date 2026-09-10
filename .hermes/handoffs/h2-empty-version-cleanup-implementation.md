# H2 empty-version AutoPort cleanup: implementation-stage escalation

**Status:** blocked; do not implement the approved seam/regression as specified until the persisted real-H2 override mechanism is corrected.

## Scope outcome

No `Config.onStarted` production seam or `database_empty_version_cleanup_test.go` regression remains in the worktree from this implementation attempt. The temporary test and the two proposed production additions were reverted after the required real-H2 reopen-over-PG validation disproved the setup premise. Existing unrelated worktree changes were preserved.

## Strict TDD evidence

### RED — test first

A new same-package `internal/repository/h2/database_empty_version_cleanup_test.go` was created first, naming the required unexported `Config.onStarted` field. The specified focused command produced the intended compile RED:

```text
$ go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=1 -timeout=30s
# zenbot/internal/repository/h2 [zenbot/internal/repository/h2.test]
internal/repository/h2/database_empty_version_cleanup_test.go:49:3: unknown field onStarted in struct literal of type Config
FAIL    zenbot/internal/repository/h2 [build failed]
FAIL
```

### Reopen-over-PG validation — failed premise

The proposed three-line seam was then temporarily added and the same real-H2 test was run. It did **not** reach the empty-version identity branch:

```text
--- FAIL: TestOpenAutoPortEmptyH2VersionClosesOwnedChild (1.22s)
    database_empty_version_cleanup_test.go:57: Open error = H2 PostgreSQL connection unavailable: failed to connect to `user=sa database=empty-version`:
        127.0.0.1:52005 (localhost): failed to receive message: unexpected EOF
        [::1]:52005 (localhost): failed to receive message: unexpected EOF, want empty H2 version error
FAIL
FAIL    zenbot/internal/repository/h2 1.729s
FAIL
```

Independent manual validation against the pinned `/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar` confirmed the cause. Seeding succeeds in its one Shell session:

```sql
SET BUILTIN_ALIAS_OVERRIDE TRUE;
CREATE ALIAS H2VERSION AS $$String h2version() { return ""; }$$;
SELECT H2VERSION();
```

but opening that persisted database through `org.h2.tools.Server -pg` then connecting over the PostgreSQL wire exits the PG server thread with:

```text
org.h2.message.DbException: Function alias "H2VERSION" already exists [90076-232]
Caused by: org.h2.jdbc.JdbcSQLSyntaxErrorException: Function alias "H2VERSION" already exists; SQL statement:
CREATE FORCE ALIAS "PUBLIC"."H2VERSION" AS 'String h2version() { return ""; }' [90076-232]
```

The client sees `failed to receive message: unexpected EOF`. This is an H2 reopen-time metadata conflict, not an AutoPort cleanup failure; therefore it cannot satisfy the required assertion that `Open` returns `H2 identity check returned empty version`.

## Verification and protected files

```text
$ git diff --check
(exit 0)

$ shasum -a 256 MIGRATION_PLAN.md .hermes/migration-audit.md
bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0  MIGRATION_PLAN.md
75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18  .hermes/migration-audit.md
```

The requested focused repeat and package-repeat gates were intentionally not run: their acceptance condition depends on an invalid persisted-alias setup, and the user directed diagnostic escalation rather than blind implementation.

## Required next decision

Find and validate a real-H2 2.3.232 persisted override syntax/configuration that survives a fresh PG-server database open **before** reintroducing the observation seam. It must allow normal `Open(AutoPort:true)` to connect and return the exact empty-version identity error; do not substitute an injected query result, fake server, or host-wide process assertion.
