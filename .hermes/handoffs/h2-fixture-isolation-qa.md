# H2 AutoPort fixture-isolation QA

**Verdict: ACCEPT** — independent source audit and every required repeat, broad, race, vet, build, formatting, integrity, and process-cleanup gate passed. No commit or push was made.

## Requirements audit

- `h2OpenTestHooks` and `Config.testHooks` are private, per-`Config`, and have no package-global/public configuration path.
- Default identity reading remains `db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)` through `readH2Version`; query failures retain `H2 identity check failed: %w`.
- The hook reader is selected only after a successful real `db.PingContext(ctx)`; `onStarted` runs only after successful `s.Start(ctx)` and receives that exact local `*processServer`.
- The empty-version path preserves `db.Close()` followed by `_ = s.Stop(context.Background())`, then returns exactly `H2 identity check returned empty version`.
- `TestOpenAutoPortEmptyH2VersionClosesOwnedChild` is same-package, real AutoPort/Java/PGX-ping coverage. It uses neither aliases, fakes, a fixed listener, nor test cleanup to establish termination. Before `Open` returns it proves the observed owned command exited and `waitDone` is closed; the later `Stop` checks only idempotence.

## Verification results

| Command | Result |
|---|---|
| `go test ./internal/repository/h2 -run '^TestOpenAutoPortEmptyH2VersionClosesOwnedChild$' -count=25 -parallel=8 -timeout=2m` | `ok zenbot/internal/repository/h2 16.915s` (exit 0) |
| `go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m` | `ok zenbot/internal/repository/h2 243.847s` (exit 0) |
| `go test ./... -count=1 -parallel=8` | all packages passed; H2 `ok ... 26.014s` (exit 0) |
| `go test -race ./... -parallel=8` | all packages passed; H2 `ok ... 25.606s` (exit 0) |
| `go vet ./...` | exit 0, no output |
| `go build ./...` | exit 0, no output |
| `gofmt -d internal/repository/h2/database.go internal/repository/h2/database_empty_version_cleanup_test.go` | empty output; then `gofmt -w` completed |
| `git diff --check` | exit 0, no output |

The race command emitted one macOS linker warning while linking `zenbot/internal/agent/sql.test` about malformed `LC_DYSYMTAB`; it exited 0 and every race test package passed. This is a host linker warning, not a Go test/race failure.

## Integrity and cleanup

Protected SHA-256 values matched exactly:

```text
bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0  MIGRATION_PLAN.md
75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18  .hermes/migration-audit.md
```

After all gates, `pgrep -fl 'org\.h2\.tools\.Server|h2-2\.3\.232\.jar'` returned no H2 server process. `lsof -nP -iTCP -sTCP:LISTEN -c java` returned no Java listeners (only pre-existing non-Java application listeners were listed by the command).

## QA changes

- Created this report only: `.hermes/handoffs/h2-fixture-isolation-qa.md`.
- No H2 source or test logic was changed during QA; formatting was applied to the two audited files and was already clean.
- The worktree contains unrelated pre-existing dirty files and broader uncommitted H2 fixture-isolation changes; none were committed, reset, or otherwise altered beyond the scoped gofmt invocation above.
