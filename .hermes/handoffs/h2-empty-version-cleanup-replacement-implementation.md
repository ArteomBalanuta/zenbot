# H2 AutoPort empty-version cleanup: replacement implementation evidence

**Status:** implemented and verified with the required real-H2 repeat gates. No commit or push was made.

## Scope

Changed only the requested implementation/test surfaces for this slice:

- `internal/repository/h2/database.go`
  - Added unexported, per-`Config` `h2OpenTestHooks` and `Config.testHooks`.
  - Added default `readH2Version`, preserving the literal production statement:
    ```go
    db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)
    ```
  - Calls `onStarted` only after `s.Start(ctx)` succeeds.
  - Selects the injected identity reader only after the existing successful `db.PingContext(ctx)` gate.
  - Retains the existing query wrapping and the empty-value cleanup sequence (`db.Close`, `s.Stop(context.Background())`, exact error).
- `internal/repository/h2/database_empty_version_cleanup_test.go` (new, `package h2`)
  - `TestOpenAutoPortEmptyH2VersionClosesOwnedChild` starts a real Java H2 AutoPort child, captures the exact `*processServer`, uses real PGX ping, injects only an empty identity value, and requires the exact error, nil database, reader invocation, exited owned child, already-closed `waitDone`, and post-proof idempotent `Stop`.

No alias, H2 setting, schema trick, package-global hook, fake server/database, fixed port, or protected-file change was introduced.

## Strict TDD evidence

### RED: test added before seam

Command:

```text
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=1 -timeout=30s
```

Exact output:

```text
# zenbot/internal/repository/h2 [zenbot/internal/repository/h2.test]
internal/repository/h2/database_empty_version_cleanup_test.go:30:12: undefined: h2OpenTestHooks
internal/repository/h2/database_empty_version_cleanup_test.go:45:3: unknown field testHooks in struct literal of type Config
FAIL	zenbot/internal/repository/h2 [build failed]
FAIL
```

Exit code: `1` (expected compilation RED: the new private seam did not exist).

### GREEN: minimal seam added

Command:

```text
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=1 -timeout=30s
```

Exact output:

```text
ok  	zenbot/internal/repository/h2	1.188s
```

Exit code: `0`.

## Required repeat evidence

Focused cleanup regression:

```text
go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m
```

Exact output:

```text
ok  	zenbot/internal/repository/h2	16.100s
```

Exit code: `0`.

H2 package regression gate:

```text
go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m
```

Exact output:

```text
ok  	zenbot/internal/repository/h2	244.645s
```

Exit code: `0`.

## Formatting and integrity

Command:

```text
gofmt -w internal/repository/h2/database.go internal/repository/h2/database_empty_version_cleanup_test.go && git diff --check
```

Exact output: empty. Exit code: `0`.

Protected handoff hashes were recorded before implementation and rechecked afterward; they are unchanged:

```text
046cb2e8e3007d25274ffb7e59f8b5c70ac48611211d91f5e8b37297c9b25203  .hermes/handoffs/h2-empty-version-cleanup-replacement-architecture.md
626efe759d40e661e5eca78ccb6f79b406a9016f820e4c55ddd7b35e525ce690  .hermes/handoffs/h2-empty-version-cleanup-alternative-diagnostic.md
```

A final `git diff --check` also exited `0`. The worktree retains pre-existing unrelated dirty identity/mail/notes/H2 changes; they were not altered for this implementation.
