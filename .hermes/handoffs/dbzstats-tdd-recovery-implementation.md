# DBZ stats strict tracer-TDD recovery implementation

## Scope and outcome

Implemented the prescribed DBZ read recovery tests only. Every temporary DBZ production isolation was manually restored immediately after its behavioral RED; no DBZ production fragment remains changed by this recovery.

New/modified recovery tests:

- `internal/listener/message/dbz_dispatch_test.go` (new): real isolated H2, real `service.DBZService`, normal `command.RegisterUserUtilitiesWithDirectAgent`, `DispatchUserCommand`, role-aware recording engine, and read-counting repository wrapper.
- `internal/repository/h2/dbz_test.go`: joined real-H2 `Stats` tracer.
- `internal/service/dbz_test.go`: exact seven-line formatter tracer.

No shared listener/authorization/reply/factory code was edited. Temporary edits were restricted to the specified DBZ registration, command, H2 read, service formatter/missing-row, and catalog-role fragments, then restored to their pre-RED semantics.

## Recovery ledger

Before each unit, the target fragment was inspected. `git diff --check` and `git diff --cached --check` passed each time. `dispatch_adapter.go` and `registry.go` were already dirty shared files, but the exact DBZ branches were unchanged pre-RED; only the specified literal/role was temporarily altered and restored. Full initial status was captured; subsequent target-status captures confirmed that the DBZ production targets had no pre-existing DBZ diff.

### Unit 1 — conditional registration

Test: `TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply`

- RED isolation: removed only `"dbzstats"` from the DBZ canonical append list in `internal/command/dispatch_adapter.go`.
- RED command:
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply$'
  ```
- RED result: exit 1; behavioral failure after `Unknown command`: `replies=0, want exactly one`.
- GREEN: restored only `"dbzstats"` to the existing DBZ append list.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/listener/message`.

### Unit 2 — command delegation

Test: `TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText`

- RED isolation: changed only `case "dbzstats"` in `internal/command/dbz.go` to `return model.SUCCESSFUL, nil`.
- RED command:
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText$'
  ```
- RED result: exit 1; command was built and produced `replies=0, want exactly one`.
- GREEN: restored the `StatsText(ctx, author)` call, failed-status error path, and shared `reply` call.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/listener/message`.

### Unit 3 — real-H2 joined read

Test: `TestDBZStatsRealH2ReadsJoinedSnapshotByName`

- RED isolation: replaced only `(*Database).Stats` behavior in `internal/repository/h2/dbz.go` with `(zero, false, nil)`; a no-op reference retained the pre-existing `database/sql` import so this was not a compile-only RED.
- RED command:
  ```sh
  go test -count=1 ./internal/repository/h2 -run '^TestDBZStatsRealH2ReadsJoinedSnapshotByName$'
  ```
- RED result: exit 1; real seeded H2 row returned zero snapshot and `ok=false` rather than `goku, 2/5/3/4/5/6`.
- GREEN: restored only the bound `$1` join query, seven-field scan, `sql.ErrNoRows` translation, and normal return contract.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/repository/h2`.

### Unit 4 — exact service rendering

Test: `TestDBZStatsTextRendersExactSevenLineSnapshot`

- RED isolation: made only the successful `StatsText` result empty. The now-unused formatter import/value binding were minimally removed to keep the production isolation compilable.
- RED command:
  ```sh
  go test -count=1 ./internal/service -run '^TestDBZStatsTextRendersExactSevenLineSnapshot$'
  ```
- RED result: exit 1; `StatsText()=""` mismatched the required trailing-newline seven-line payload.
- GREEN: restored the formatter import, returned snapshot binding, and the original `fmt.Sprintf` expression.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/service`.

### Unit 5 — missing character through dispatch

Test: `TestDispatchUserCommandDBZStatsMissingCharacterRepliesExactText`

- RED isolation: removed only the `!ok` missing-row branch in `StatsText`; the temporary binding ignored `ok` solely to retain compilation.
- RED command:
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsMissingCharacterRepliesExactText$'
  ```
- RED result: exit 1; real empty H2 read sent the zero snapshot (`character:`, all numeric values `0`) instead of `No stats found for character: missing`.
- GREEN: restored only `if !ok { return "No stats found for character: " + name, nil }` and its `ok` binding.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/listener/message`.

### Unit 6 — REGULAR authorization before read

Test: `TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply`

- RED isolation: changed only the `dbzstats` catalog definition role in `internal/command/registry.go` from `model.REGULAR` to `model.USER`.
- RED command:
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply$'
  ```
- RED result: exit 1; resolved `goku` reached the DBZ path and the real-H2 wrapper recorded `Stats reads=1`, where the tracer requires zero.
- GREEN: restored only `model.REGULAR` on `dbzstats`.
- GREEN command: same exact command.
- GREEN result: exit 0, `ok zenbot/internal/listener/message`.

## Recorded behavior

- Authorized `!ds` and `!dbzstats` use caller `goku`, query the directly seeded real H2 snapshot, and produce exactly one whispered seven-line reply.
- Missing `!dbzstats` produces exactly one whispered `No stats found for character: missing` reply from a real empty H2 read.
- An active-user-resolved caller denied `REGULAR` receives exactly the shared unauthorized reply for `!ds`; the wrapper observed zero `Stats` calls.
- Component coverage independently confirms the H2 seven-field join snapshot and service trailing-newline renderer.

## Final verification

Executed exactly:

```sh
go test -count=1 ./internal/listener/message ./internal/command ./internal/service ./internal/repository/h2 && git diff --check && git diff --cached --check
```

Output:

```text
ok   zenbot/internal/listener/message    3.385s
ok   zenbot/internal/command             10.391s
ok   zenbot/internal/service             2.742s
ok   zenbot/internal/repository/h2       42.084s
```

Both diff checks exited 0 with no output. A final fragment check found zero diff lines for `internal/command/dbz.go`, `internal/repository/h2/dbz.go`, and `internal/service/dbz.go`.

Final `git status --short` contains 190 entries in the already heavily dirty shared worktree. Recovery-specific paths are:

```text
 M internal/repository/h2/dbz_test.go
 M internal/service/dbz_test.go
?? internal/listener/message/dbz_dispatch_test.go
?? .hermes/handoffs/dbzstats-tdd-recovery-implementation.md
```

The first two test additions are 33 lines total; the listener tracer is a new 227-line test fixture/tracer file. Existing unrelated dirty paths remain outside this recovery scope.

## Acceptance labels and tradeoff

- **Strict recovery evidence:** six named behavioral REDs, each immediately followed by the required minimal GREEN and passing rerun.
- **Full-path acceptance:** Units 1, 2, 5, and 6 pass with normal conditional registration, concrete command dispatch, real H2, and recorded transport.
- **Component acceptance:** Units 3 and 4 pass.
- **Tradeoff:** shared `dispatch_adapter.go` and `registry.go` already carried unrelated dirty hunks. Their DBZ fragments were verified unchanged before isolation and restored semantically; coordinate with their existing owners before staging or merging any worktree changes.
