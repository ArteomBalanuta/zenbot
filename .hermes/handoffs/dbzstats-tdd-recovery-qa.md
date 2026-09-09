# DBZ stats TDD-recovery QA

## Verdict: ACCEPTED WITH PROVENANCE LIMITATION

The present recovery-owned implementation is source-grounded and acceptance-quality for the DBZ stats read path. The six recovery records are coherent with the inspected production seams, the resulting tracer tests exercise the intended real-H2/normal-registration/authorization/reply paths, and no temporary DBZ production isolation remains in the worktree.

Historical/temporal provenance remains necessarily limited: this QA pass can validate the recorded RED→GREEN ledger and current source/results, but cannot independently replay a past transient RED that has already been restored. Treat the six entries as the implementation handoff's retained recovery evidence, not newly observed QA RED evidence.

## Inspected paths

Handoffs:

- `.hermes/handoffs/dbzstats-tdd-forensic.md`
- `.hermes/handoffs/dbzstats-tdd-recovery-architecture.md`
- `.hermes/handoffs/dbzstats-tdd-recovery-implementation.md`

Recovery tests:

- `internal/listener/message/dbz_dispatch_test.go`
- `internal/repository/h2/dbz_test.go`
- `internal/service/dbz_test.go`

Production/parity seams:

- `internal/listener/message/handlers.go`
- `internal/common/command.go`, `internal/common/engine.go`
- `internal/command/dispatch_adapter.go`, `internal/command/dbz.go`, `internal/command/registry.go`
- `internal/service/dbz.go`
- `internal/repository/h2/dbz.go`, `internal/repository/h2/database.go`
- `internal/repository/h2/audit_test.go`
- `internal/testutil/h2fixture/h2fixture.go`

## Recovery-record audit

The six documented RED boundaries and corresponding restored GREEN behavior match current source:

1. DBZ conditional registration includes `dbzstats`, making `ds` reachable through normal `RegisterUserUtilitiesWithDirectAgent` registration.
2. `dbzCommand.Execute` routes canonical `dbzstats` to `StatsText(ctx, message.Name)` and shared `reply`.
3. H2 `Stats` performs the bound-name joined seven-field read and translates `sql.ErrNoRows` to `ok=false`.
4. `StatsText` renders the exact seven-line, trailing-newline payload.
5. `StatsText` returns exact missing-character text through the same dispatch/H2 path.
6. Catalog metadata holds `dbzstats` at `model.REGULAR`; dispatch authorizes before contextual execution/read.

The listener tests cover authorized alias `!ds`, canonical `!dbzstats`, caller-derived identity, real H2 seeding/read, exact reply text, recipient, inbound whisper preservation, missing-character behavior, active-user metadata resolution, denied `REGULAR` authorization, shared unauthorized output, and zero repository reads on denial. The H2 and service tests independently cover the joined snapshot and renderer.

## QA hardening performed

Modified only `internal/listener/message/dbz_dispatch_test.go`:

- Replaced its machine-specific inline `/Users/ab/...` H2 jar fixture with the existing test-only `h2fixture.Open(t, "dbz-dispatch")` convention. That fixture accepts `H2_JAR` and owns isolated temporary H2 lifecycle/auto-port setup.
- Removed embedded `common.Engine` from the DBZ recording engine. It now explicitly implements the required engine methods; every non-contract method panics. This prevents a nil embedded interface from silently satisfying unexpected dispatch/authorization calls and makes the tested surface explicit.
- Ran `gofmt -w` only on that tester-modified Go test file.

This was QA fixture hardening, not an added post-recovery regression behavior; no new DBZ behavioral test was introduced.

## Race/static assessment

The DBZ tracer tests do not call `t.Parallel`, and each creates its own engine, reply slice, read counter, temporary directory, and auto-port H2 process. The counter and reply slice are accessed serially inside each test. `go test -race` passed for the four relevant packages.

## Verification outcomes

All commands exited 0.

```text
go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStats'
ok   zenbot/internal/listener/message  3.447s

go test -count=1 ./internal/repository/h2 -run '^TestDBZStatsRealH2ReadsJoinedSnapshotByName$'
ok   zenbot/internal/repository/h2  0.921s

go test -count=1 ./internal/service -run '^TestDBZStatsTextRendersExactSevenLineSnapshot$'
ok   zenbot/internal/service  0.299s

go test -race -count=1 ./internal/listener/message ./internal/command ./internal/service ./internal/repository/h2
ok   zenbot/internal/listener/message  8.190s
ok   zenbot/internal/command           43.008s
ok   zenbot/internal/service           4.016s
ok   zenbot/internal/repository/h2     37.341s

go test -count=1 ./...
PASS: all listed packages; packages without tests reported `[no test files]`; H2 package passed in 38.829s.

go vet ./...
(no output)

go build ./...
(no output)

git diff --check && git diff --cached --check
(no output)
```

Final production-target diff check was empty for:

- `internal/command/dbz.go`
- `internal/repository/h2/dbz.go`
- `internal/service/dbz.go`

A structural check found neither `/Users/ab` nor `common.Engine` in `internal/listener/message/dbz_dispatch_test.go`.

## Files modified/created by this QA pass

- Modified: `internal/listener/message/dbz_dispatch_test.go` (test-only portability and explicit-interface hardening).
- Created: `.hermes/handoffs/dbzstats-tdd-recovery-qa.md`.

Pre-existing recovery test changes remain in `internal/repository/h2/dbz_test.go` and `internal/service/dbz_test.go`; they were inspected and not altered by QA. The checkout remains heavily dirty outside recovery scope; no staging, reset, checkout, restore, clean, stash, or commit was performed.
