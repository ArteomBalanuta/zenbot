# DBZ stats strict tracer-TDD recovery specification

**Artifact purpose:** controlled recovery of strict RED→GREEN evidence for the existing `dbzstats` read vertical. This is a developer handoff, not evidence that strict TDD already occurred and not authorization to broaden DBZ work.

## 1. Decision and non-negotiable constraints

[OBSERVED] `dbzstats` production and DBZ-specific tests first appear together in commit `48777d8029be8741d430b07559b37352177d793a`; no retained history records an antecedent failing end-to-end tracer followed by a minimal production GREEN. See `.hermes/handoffs/dbzstats-tdd-forensic.md`.

[RECOMMENDED] Recover evidence prospectively by **temporarily and manually isolating exactly one DBZ-stats-owned behavior at a time**, adding/running one tracer, observing its required behavioral failure, restoring only that behavior, and immediately rerunning that one tracer. A currently green test is explicitly not a RED result. A test-harness compilation/setup failure is not a valid production RED.

### Dirty-worktree protocol

[RECOMMENDED]

1. Work only from `/Users/ab/workspace/go-projects/zenbot`.
2. Before every recovery unit, record `git status --short`, `git diff --check`, and `git diff --cached --check`; retain the output with the unit’s RED/GREEN transcript.
3. Manually edit only the pre-RED recovery boundary named below plus the single new test under development. Do **not** use `git reset`, `git checkout`, `git restore`, `git clean`, `git stash`, `git add`, `git commit`, patch-wide formatting, or automated bulk reverts.
4. Before altering a shared dirty file, inspect its current hunk and establish that the exact DBZ token/branch is unchanged from the source described here. If the required token is inside a conflicting or otherwise owned dirty hunk, stop and obtain the owner’s coordination; do not overwrite, reorder, or reconstruct that hunk.
5. Restoration is a manual semantic restoration of only the pre-RED behavior, not a checkout of a file. Compare the resulting local DBZ fragment to the observed contract below; never claim unrelated hunks are unchanged merely because a command exits zero.
6. Keep a unit’s test and temporary production isolation in the worktree until its GREEN is recorded. Then remove only the temporary isolation by restoring the minimal behavior. Do not start the next RED while any prior tracer is red.
7. The only artifact created by this architecture task is this handoff. No application source or tests were changed.

[OBSERVED] The checkout is heavily dirty in shared command, factory, listener, core, repository, and service files. The forensic report identified no current DBZ-specific unstaged/staged production or test hunk. `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/command/registry.go`, `internal/factory/engine_factory.go`, `internal/listener/message/handlers.go`, and `internal/service/services.go` are shared files and must be treated as coordination boundaries, not as a DBZ-owned-file permit.

## 2. Current source-grounded vertical

[OBSERVED] Current path, including shared infrastructure:

```text
inbound ChatMessage("!ds", Name="goku", whisper)
  -> message.ResolveUserMetadata (sets Context.Author from active users)
  -> message.DispatchUserCommand.Handle
      -> common.BuildCommand("ds", engine, message)
      -> Engine.IsUserAuthorized(Author, command role REGULAR)
      -> command.legacyAdapter.ExecuteContext
      -> command.dbzCommand.Execute(canonical="dbzstats")
      -> service.DBZService.StatsText(ctx, message.Name)
      -> repository.DBZRepository.Stats(ctx, "goku")
      -> h2.Database.Stats SELECT join with bound $1
      -> command.reply -> Engine.SendChatMessage("goku", text, inbound-whisper)
```

| Boundary | Source-grounded implementation | Recovery ownership / risk |
|---|---|---|
| Catalog role/aliases | `internal/command/registry.go`, `RegisterAll`, `def("dbzstats", []string{"dbzstats", "dstats", "dstat", "ds"}, model.REGULAR)` | DBZ-owned metadata; low code complexity, high authorization risk. |
| Conditional exposure | `internal/command/dispatch_adapter.go`, `RegisterUserUtilitiesWithDirectAgent`; appends DBZ canonicals only when `bundle(e).DBZ != nil` | DBZ registration fragment in a dirty shared file; low complexity, high merge/coordination risk. |
| Command construction | `internal/command/handlers.go`, `newCommand`, DBZ canonical case returns `*dbzCommand`; `internal/command/dbz.go`, `dbzCommand.Execute` | DBZ branch is owned but construction file is shared/dirty; medium integration risk. |
| Authorization and metadata resolution | `internal/listener/message/handlers.go`, `ResolveUserMetadata` and `DispatchUserCommand.Handle` | Shared security boundary; do not recover by editing it. `DispatchUserCommand` authorizes before execution. |
| DBZ read/render | `internal/service/dbz.go`, `(*DBZService).StatsText` | DBZ-owned; low complexity, medium contract risk (exact text). |
| Persistence seam | `internal/repository/dbz.go`, `DBZRepository.Stats(context.Context, string) (DBZStats, bool, error)` | Stable interface; change neither interface nor unrelated mutation methods. |
| H2 read | `internal/repository/h2/dbz.go`, `(*Database).Stats` | DBZ-owned; low complexity, medium SQL/missing-row risk. |
| Reply transport | `internal/command/handlers.go`, `reply` | Shared helper, currently preserves `IsWhisper || Whisper || Type == "whisper"`; not a DBZ-owned recovery edit. |
| Composition | `internal/factory/engine_factory.go`, `NewEngineWithOptions`; installs `Bundle.DBZ` only when repository implements `DBZRepository` | Shared/dirty composition file. Full-path test may use a narrow test engine and real H2 without changing factory. |

[OBSERVED] Saturn’s source contract is `Role.REGULAR`, aliases `dbzstats`, `dstats`, `dstat`, `ds`, caller-derived name, seven-line row rendering, and `No stats found for character: <name>` when no row exists: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZStatsCommandImpl.java`, `DBZImpl.getStats`, and `DBZUtil.SELECT_STATS`.

## 3. Test seam and fixture specification

[RECOMMENDED] Add a focused test file, `internal/listener/message/dbz_dispatch_test.go`, rather than expanding generic authorization tests. It must use:

- the real isolated H2 fixture pattern from `internal/repository/h2/audit_test.go: openTestDB(t)`;
- a real `*service.DBZService{Repo: db}` in a `*service.Bundle`;
- normal `command.RegisterUserUtilitiesWithDirectAgent(engine, nil)` registration, not direct `dbzCommand.Execute` construction;
- a narrow local recording engine that implements `common.Engine`, `ServiceBundle() *service.Bundle`, command registration/lookup, active-user lookup, role-aware `IsUserAuthorized`, and `SendChatMessage` recording `(recipient, text, whisper)`;
- `DispatchUserCommand{}.Handle` directly. The test may call `ResolveUserMetadata{}.Handle` first only when it is explicitly testing metadata resolution; otherwise it supplies `Context.Author` to isolate dispatch behavior;
- H2 setup that directly inserts a unique `goku` character/stat row (level 2, free 5, str 3, agi 4, vit 5, ene 6). Do not call DBZ registration/mutation methods in a read tracer; that would make the read fixture dependent on separate DBZ behavior.

The expected row payload is exactly:

```text
character: goku
level: 2
free stats: 5
str: 3
agi: 4
vit: 5
ene: 6
```

with a trailing newline. Each successful full-path tracer asserts exactly one recorded reply, recipient `goku`, and `whisper == true` for an inbound `IsWhisper: true` request.

## 4. Strict sequential recovery ledger

**Sequence rule:** run unit N’s RED, make only unit N’s minimal GREEN, and run unit N’s GREEN before creating or running unit N+1. Never create a batch of failing tests.

### Unit 1 — conditional registration makes the real-H2 alias reachable

- **Test name:** `TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply`
- **Test location:** `internal/listener/message/dbz_dispatch_test.go`.
- **Pre-RED recovery boundary:** In `internal/command/dispatch_adapter.go`, manually remove only the literal `"dbzstats"` from the DBZ canonical append list inside `if b := bundle(e); b != nil && b.DBZ != nil`. Leave the other DBZ canonicals and every non-DBZ conditional registration untouched. This is a valid production isolation: the normal registry cannot expose `ds`/`dbzstats`, so the dispatcher cannot produce the intended reply.
- **RED command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply$'
  ```
- **Required RED outcome:** FAIL because the recording output has zero replies (and/or `ds` is absent from enabled commands), where the test requires exactly one `goku` whispered seven-line reply. A compile failure, H2 startup failure, or generic fixture failure is invalid; repair only test setup until this behavioral failure is observed.
- **Minimal GREEN implementation:** Restore only `"dbzstats"` to that existing DBZ conditional canonical list. Do not modify the registry aliases, dispatcher, factory, H2 SQL, service format, or shared reply helper.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply$'
  ```
- **Required GREEN outcome:** PASS. This records registration→BuildCommand→authorization→adapter→DBZ command→real H2→one whisper reply.

### Unit 2 — DBZ command delegates caller name and emits the service text

- **Test name:** `TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText`
- **Test location:** `internal/listener/message/dbz_dispatch_test.go`.
- **Pre-RED recovery boundary:** In `internal/command/dbz.go`, replace only the `case "dbzstats"` behavior with a no-output successful branch (`return model.SUCCESSFUL, nil`). Preserve the initial context/bundle guard and every non-`dbzstats` case. This removes the DBZ-stats command behavior while retaining registration and all shared dispatch mechanics.
- **RED command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText$'
  ```
- **Required RED outcome:** FAIL because a canonical inbound `!dbzstats` produces zero replies rather than exactly one `goku` whispered expected payload. The fixture must seed `goku` but invoke no explicit character argument, proving the command uses `message.Name`.
- **Minimal GREEN implementation:** Rebuild only the observed `dbzstats` branch in `dbzCommand.Execute`: call `b.DBZ.StatsText(ctx, author)`; return `model.FAILED` on its error; otherwise `reply(&c.commandBase, v)`. Do not add parsing, an identity lookup, an argument override, a direct repository call, or a DBZ-specific transport implementation.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText$'
  ```
- **Required GREEN outcome:** PASS.

### Unit 3 — independently evidenced H2 row read

- **Test name:** `TestDBZStatsRealH2ReadsJoinedSnapshotByName`
- **Test location:** `internal/repository/h2/dbz_test.go`.
- **Classification:** component recovery, not a substitute for Units 1–2’s complete dispatch tracer. It establishes the persistence component independently and is intentionally run after the first complete tracer is green.
- **Pre-RED recovery boundary:** In `internal/repository/h2/dbz.go`, manually replace only `(*Database).Stats` with `return repository.DBZStats{}, false, nil`. Do not touch `FreeStats`, registration, schema, generic database bootstrap, or the `DBZRepository` interface.
- **RED command:**
  ```sh
  go test -count=1 ./internal/repository/h2 -run '^TestDBZStatsRealH2ReadsJoinedSnapshotByName$'
  ```
- **Required RED outcome:** FAIL because `ok` is false/returned snapshot is zero despite the test’s directly seeded H2 row. A SQL fixture error is not accepted as RED.
- **Minimal GREEN implementation:** Restore only `Stats`: query the `dbz_stats`/`dbz_characters` inner join filtered by a bound `$1` name; scan all seven fields; translate `sql.ErrNoRows` to `(zero, false, nil)`; return `(snapshot, err == nil, err)` otherwise.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/repository/h2 -run '^TestDBZStatsRealH2ReadsJoinedSnapshotByName$'
  ```
- **Required GREEN outcome:** PASS with `goku`, 2/5/3/4/5/6 exactly.

### Unit 4 — independently evidenced service rendering contract

- **Test name:** `TestDBZStatsTextRendersExactSevenLineSnapshot`
- **Test location:** `internal/service/dbz_test.go`.
- **Classification:** component recovery. It is required for a precise formatter contract but does not replace a dispatcher/H2 tracer.
- **Pre-RED recovery boundary:** In `internal/service/dbz.go`, make only the successful `StatsText` return value empty after the existing successful repository read (for example, return `"", nil` in place of the format expression). Preserve error propagation and the `!ok` branch for this unit.
- **RED command:**
  ```sh
  go test -count=1 ./internal/service -run '^TestDBZStatsTextRendersExactSevenLineSnapshot$'
  ```
- **Required RED outcome:** FAIL with empty/mismatched text versus the required exact seven lines including the trailing newline.
- **Minimal GREEN implementation:** Restore only `fmt.Sprintf("character: %s\nlevel: %d\nfree stats: %d\nstr: %d\nagi: %d\nvit: %d\nene: %d\n", ...)` using the returned `DBZStats` fields. Do not change the repository interface or output transport.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/service -run '^TestDBZStatsTextRendersExactSevenLineSnapshot$'
  ```
- **Required GREEN outcome:** PASS.

### Unit 5 — complete dispatch behavior for a missing DBZ character

- **Test name:** `TestDispatchUserCommandDBZStatsMissingCharacterRepliesExactText`
- **Test location:** `internal/listener/message/dbz_dispatch_test.go`.
- **Pre-RED recovery boundary:** In `internal/service/dbz.go`, remove only the `if !ok { return "No stats found for character: " + name, nil }` branch from `StatsText` while leaving the H2 missing-row translation unchanged. The surviving formatter will produce an incorrect zero-value snapshot, giving a behavioral RED through the full dispatch path.
- **RED command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsMissingCharacterRepliesExactText$'
  ```
- **Required RED outcome:** FAIL because inbound `!dbzstats` for authorized `missing` has a reply other than exactly `No stats found for character: missing` (or, if the temporary isolation instead yields none, zero replies). The test must use a real empty H2 DBZ read, not a stub.
- **Minimal GREEN implementation:** Restore only the observed `!ok` return branch in `StatsText`. Do not make DBZ names unique, add a schema migration, change H2 `Stats`, or introduce a separate command-level missing check.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsMissingCharacterRepliesExactText$'
  ```
- **Required GREEN outcome:** PASS with exactly one reply to `missing`; use an inbound whisper and assert it remains whispered.

### Unit 6 — DBZ stats keeps the REGULAR authorization gate closed before reads

- **Test name:** `TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply`
- **Test location:** `internal/listener/message/dbz_dispatch_test.go`.
- **Pre-RED recovery boundary:** In `internal/command/registry.go`, change only the `dbzstats` definition’s role from `model.REGULAR` to `model.USER`; do not alter the listener authorization code. The tracer’s engine permits `USER` but denies `REGULAR`, so the temporarily weakened DBZ metadata permits execution and makes the expected no-read/no-DBZ-reply assertion fail.
- **RED command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply$'
  ```
- **Required RED outcome:** FAIL because H2 read instrumentation observes a read and/or the DBZ stats output appears, when the test expects no DBZ read plus one shared unauthorized response for `!ds`. The test must not count the shared unauthorized response as a DBZ reply.
- **Minimal GREEN implementation:** Restore only `model.REGULAR` on the `dbzstats` definition. Do not change `ResolveUserMetadata`, `DispatchUserCommand.Handle`, `Engine.IsUserAuthorized`, or security-service policy.
- **GREEN command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply$'
  ```
- **Required GREEN outcome:** PASS: no H2 `Stats` invocation, no DBZ formatted/missing payload, and exactly the shared listener unauthorized text to the resolved author. This proves authorization ordering for the concrete DBZ command but does not claim to re-prove the shared authorization implementation itself.

## 5. Recovery boundaries that must not be used

[RECOMMENDED] Do not manufacture a DBZ RED by:

- deleting/altering `message.DispatchUserCommand.Handle`, `ResolveUserMetadata`, `common.BuildCommand`, `Engine.IsUserAuthorized`, or `reply`; these are shared and dirty infrastructure, and failures would not be DBZ-owned recovery;
- changing `internal/factory/engine_factory.go` merely to make a test fail; use the narrow real-H2 test engine instead;
- deleting tables, changing either schema copy, stopping H2, removing the H2 jar, or breaking test fixture setup; these are environmental/harness failures, not a missing DBZ behavior;
- replacing the repository with a stub in a full-path tracer; component tests may use a stub only for Unit 4;
- using a new test that passes before the named isolation as proof of RED→GREEN;
- combining the happy path, missing row, and authorization assertions in one initially failing test; that makes failure cause ambiguous and violates the sequential tracer rule.

## 6. Scope, complexity, and risk assessment

| Area | Complexity | Main risk | Decision |
|---|---:|---|---|
| Alias registration | Low | Shared dirty adapter hunk / lost aliases | Recover in Unit 1 only. |
| Command delegation | Low | Caller name accidentally replaced by argument; output swallowed | Recover in Unit 2 through real dispatch/H2. |
| H2 read | Low | Query/scan or `sql.ErrNoRows` contract drift | Independently recover in Unit 3. |
| Exact service format | Low | Whitespace/newline parity drift | Independently recover in Unit 4. |
| Missing row | Low | Zero-valued stats rendered as real state | Recover in Unit 5 through real dispatch/H2. |
| Authorization metadata | Low code / high security | Changing role makes protected data readable | Recover in Unit 6; no shared-security edits. |
| Whisper preservation | No DBZ-owned code | Shared `reply` helper changes or transport semantics | Assert on full-path units; do not manipulate shared helper to create RED. |
| Factory composition | Medium | Dirty shared factory and broad engine setup | Not required for primary tracer; cover separately only if a factory-specific regression is reported. |

## 7. Final verification and acceptance vocabulary

After all six GREEN outcomes, run only after every individual GREEN is recorded:

```sh
go test -count=1 ./internal/listener/message ./internal/command ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

[RECOMMENDED] Record the exact command output and the before/after status snapshots. Compare the final status to the pre-recovery snapshot: the expected additions are only the explicitly approved tracer tests and their deliberate minimal DBZ implementation changes; this architecture handoff itself is already an untracked documentation artifact. Any unrelated changed/added/deleted path is a blocker for claiming a clean recovery result.

Use these labels precisely:

- **Strict recovery evidence:** every named unit has its behavioral RED transcript and immediately subsequent GREEN transcript.
- **Full-path acceptance:** Units 1, 2, 5, and 6 pass; these span registration/authorization/adapter/command/real H2/reply where applicable.
- **Component acceptance:** Units 3 and 4 pass; they support but cannot replace full-path acceptance.
- **Legacy current-green only:** any existing test pass without the documented pre-RED isolation and behavioral failure. This remains insufficient for strict provenance.

## 8. Developer checklist

- [ ] Read `.hermes/handoffs/dbzstats-tdd-forensic.md` and this specification; acknowledge that current green is not historical strict-TDD evidence.
- [ ] Capture initial `git status --short`; run both `git diff --check` commands; preserve output.
- [ ] Inspect each shared-file target immediately before its temporary edit and coordinate if its DBZ fragment is dirty/conflicted.
- [ ] Implement the real-H2 recording-engine fixture once, but add/run only Unit 1’s test first.
- [ ] Complete Unit 1 RED and GREEN exactly as specified and record both outputs.
- [ ] Complete Unit 2 RED and GREEN exactly as specified and record both outputs.
- [ ] Complete Unit 3 RED and GREEN exactly as specified and record both outputs.
- [ ] Complete Unit 4 RED and GREEN exactly as specified and record both outputs.
- [ ] Complete Unit 5 RED and GREEN exactly as specified and record both outputs.
- [ ] Complete Unit 6 RED and GREEN exactly as specified and record both outputs.
- [ ] Confirm all temporary isolations are restored and no shared behavior was changed to manufacture RED.
- [ ] Run the final focused package suite and diff/status checks.
- [ ] Do not stage, commit, reset, checkout, clean, or overwrite unrelated work.
- [ ] Report strict recovery evidence separately from current baseline health and from Saturn parity claims.

## 9. Current baseline captured for this handoff

[TEST-BACKED] On the dirty checkout inspected for this artifact:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok   zenbot/internal/command           10.771s
ok   zenbot/internal/listener/message   0.444s
ok   zenbot/internal/service            2.887s
ok   zenbot/internal/repository/h2     37.328s

git diff --check && git diff --cached --check
exit 0; no output
```

[LIMITATION] This baseline establishes focused package health and whitespace validity on the then-current dirty tree. It does not establish antecedent strict-TDD provenance or close the missing full-dispatch acceptance gaps without the recovery transcripts above.
