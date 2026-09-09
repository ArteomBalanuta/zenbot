# `dfight` source-exact parity: bounded mixed transient/H2 vertical

## Decision

[RECOMMENDED] **Proceed as a bounded strict-parity vertical.** `dfight` is not combat: it accepts one required enemy token, removes the first equal entry from a process-local enemy list if present, then unconditionally attempts to level up the invoking author and add five free stats, publishes a fixed public acknowledgement, and returns success. There is no enemy-existence gate, kill validation, enemy stat lookup, damage calculation, RNG, ownership rule, persistence of enemies, rollback, or transaction in Saturn.

The Zenbot checkout already has the required command/service/repository surface and mostly matches source sequencing. Two command-visible differences block an exact claim:

1. missing input currently emits `dfight enemy`; Saturn emits addressed, whisper-preserving `Example: <prefix>dfight enemy`;
2. successful output currently uses addressed/whisper-preserving `reply`; Saturn uses the one-argument `OutService` overload, hence unaddressed/public output regardless of inbound whisper.

No new prerequisite or product decision blocks the bounded vertical. Do **not** turn source silence about combat, missing enemies, or concurrency into a new gameplay design.

## Source contract

### Command surface and parser

[OBSERVED] `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZFightCommandImpl.java`:

- declares exactly `dfight` and `df` (line 16) with `Role.REGULAR` (lines 23–26);
- binds the level-up subject only from `chatMessage.getNick()` (line 30); the enemy argument is never an author/character/identity target;
- obtains `requiredArgument(0, "dfight enemy")` (line 31), then returns `FAILED` without state mutation when absent/blank (line 32);
- calls `fight(enemy)` first, then `lvlUp(author)` second (lines 34–35), queues one fixed message (lines 37–38), and returns `SUCCESSFUL` (line 41).

[OBSERVED] `UserCommandBaseImpl.requiredArgument` in `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:154-164` rejects absent, null, and trimmed-blank first arguments and calls `failWithUsage`. `failWithUsage` at lines 191-202 sends exactly `Example: <engine-prefix>dfight enemy` to the author while preserving `chatMessage.isWhisper()`, then returns `FAILED`.

[OBSERVED] The shared parser strips prefix, trims, and splits trailing arguments on `\\s+` (`UserCommandBaseImpl.java:51-71`). Thus `!dfight   frieza ignored` acts on `frieza` only; later tokens do not matter. Preserve the target's existing first-token `args` path. Java and Go whitespace classification need not be broadened for exotic code points without a source fixture.

### Enemy existence, consumption, duplicates, and lifecycle

[OBSERVED] `DBZImpl` owns `private final List<String> enemies = new ArrayList<>()` (`../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java:16-24`). Its complete fight implementation is `this.enemies.remove(name)` (lines 230-233). Java `List.remove(Object)` removes the first equal entry and silently returns `false` when no equal entry exists; the return is ignored.

Consequences that must remain explicit:

| Incoming enemy token | Enemy-list effect | Level/stat effect | Source output/status |
|---|---|---|---|
| absent/blank | none | none | addressed `Example: <prefix>dfight enemy`, whisper follows input; `FAILED` |
| first equal entry exists | remove exactly the first equal entry | invoke level-up attempt | public fixed payload; `SUCCESSFUL` |
| duplicate equal entries | remove one earliest equal entry; retain later duplicates | invoke level-up attempt | public fixed payload; `SUCCESSFUL` |
| no equal entry exists | no list change | **still invoke level-up attempt** | same public fixed payload; `SUCCESSFUL` |

[OBSERVED] `spawnEnemy` is only `enemies.add(name)` (`DBZImpl.java:235-238`). `Base` constructs one `DBZImpl` for each base/engine instance (`../../projects/saturn/src/main/java/org/saturn/app/facade/Base.java:77-91`). Enemy entries are therefore append-only, insertion-ordered, instance-local, and lost with that DBZ service/engine instance. There is no H2 table, recovery, expiry, cap, deduplication, listing command, or cross-engine sharing.

### Level/stat effects and failure shape

[OBSERVED] `DBZFightCommandImpl` always invokes `engine.dbzService.lvlUp(author)` after `fight`, independent of whether an enemy was removed. `DBZImpl.lvlUp` executes, in one `try` block and with no explicit transaction:

1. `UPDATE dbz_characters SET level=level+1 WHERE name=?`; then
2. `UPDATE dbz_stats SET free_stats=free_stats+5 WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?)`.

Sources: `DBZImpl.java:78-99`; `DBZUtil.UPDATE_LEVEL_BY_NAME` and `UPDATE_ADD_STATS_BY_NAME` in `../../projects/saturn/src/main/java/org/saturn/app/util/DBZUtil.java:9-13`.

The scalar name subquery has no `ORDER BY`; Saturn's DBZ schema permits duplicate character names. Do not add a unique constraint, selection rule, ownership binding, retry, conditional update, or transaction. A matched list enemy need not correspond to a DBZ row, and a DBZ row need not have an enemy-list entry.

[OBSERVED] `lvlUp` catches/logs `SQLException` and returns `void`; `DBZFightCommandImpl` continues to its fixed output/status. If the first update fails, the second is not attempted because both statements share the same `try`; if the second fails, the level change may remain. No rollback or compensating write occurs.

### Exact successful output and no combat/RNG contract

[OBSERVED] The successful command calls `OutService.enqueueMessageForSending(String)` with exactly:

```text
Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!
```

`OutService.java:35-43` rejects blank payloads, normalizes line endings/literal `\\n`, and queues an unaddressed message. It does not use the command author or whisper flag. The payload is intentionally emitted even when no enemy existed and when `lvlUp` swallowed an SQL exception.

[OBSERVED] None of `DBZFightCommandImpl`, `DBZImpl`, `DBZUtil`, or `DBZService` contains a random source, enemy attributes, combat outcome, damage calculation, or kill proof. The text “Enemy has been slain” is not evidence of a combat subsystem. No RNG/design work is in scope.

[LIMITATION] Search of `../../projects/saturn/src/test/java` found no `dfight`, `DBZFight`, `fight`, or DBZ enemy behavioral test. The contract above is production-source/SQL/output-service grounded, not source-test backed.

## Zenbot mapping and bounded delta

| Concern | Observed target | Parity disposition |
|---|---|---|
| aliases/role/construction | `internal/command/registry.go` declares `dfight`/`df`, `REGULAR`; `internal/command/handlers.go` constructs `*dbzCommand` | already matches; do not alter catalog/shared handler |
| capability registration | `internal/command/dispatch_adapter.go:102-104` registers `dfight` only when `bundle(e).DBZ != nil` | preserve fail-closed composition gate |
| identity/authorization | `internal/listener/message/handlers.go:ResolveUserMetadata` binds active user by name; `DispatchUserCommand` authorizes `REGULAR` before execution | preserve; no nick/trip/DBZ-row identity inference |
| target parsing | `internal/command/handlers.go:args`; `internal/command/dbz.go` uses only `a[0]` and trims it | matches ordinary first-token source behavior |
| missing input | `internal/command/dbz.go:80-84` returns `FAILED` but replies `dfight enemy` | **correct only payload** to `Example: <prefix>dfight enemy`; retain addressed/whisper-preserving reply and no mutation |
| transient consumption | `internal/service/dbz.go:68-77` removes first equal entry and otherwise no-ops | sequential behavior matches Java `remove(Object)` |
| level-up bridge | `DBZService.LevelUp` delegates to `repository.DBZRepository.LevelUp`; `internal/repository/h2/dbz.go:25-30` uses source-ordered two statements and no transaction | matches normal/partial mutation ordering; command must continue to ignore its returned error |
| success output | `internal/command/dbz.go:85-87` currently uses `reply` | **replace only this success delivery** with public/unaddressed `SendChatMessage("", fixedPayload, false)` |
| H2/schema | DBZ tables already exist in `internal/repository/h2/schema-h2.sql` and mirrored `resources/schema-h2.sql` | no schema/repository change needed |
| tests | `internal/command/dbz_test.go` covers aliases but no fight behavior; `internal/service/dbz_test.go` has generic concurrent list exercise; untracked `internal/listener/message/dbz_dispatch_test.go` covers stats/register/strength but no fight | add focused dfight tracer(s) only; do not rewrite other DBZ coverage |

[OBSERVED] Zenbot's `DBZService` uses `sync.Mutex` around individual `SpawnEnemy`, `Fight`, and `Enemies` operations (`internal/service/dbz.go:14,63-81`). This is a target safety improvement over Saturn's unsynchronized `ArrayList`. It preserves sequential first-match semantics but does **not** make `Fight` plus H2 `LevelUp` atomic, does not create a defined multi-command ordering contract, and must not be represented as source concurrency parity.

### Required target-only error adaptation

[RECOMMENDED] Follow the existing DBZ public-output adapter convention used by `dspawn`/`dbzstr`: after `Fight` and ignored `LevelUp` error, call checked `SendChatMessage("", fixedPayload, false)`; if the send fails, return `FAILED` with that Go transport error **after** the transient/persistent effects. Saturn's queue API can throw unchecked and does not expose that status mapping, so label this a target transport adaptation, not source evidence. Do not retry or roll back.

The legacy registration bridge calls `ExecuteContext` from inbound message dispatch but `legacyAdapter.Execute()` substitutes `context.Background()` and logs/discards returned errors (`internal/command/dispatch_adapter.go:12-38`). Direct tracer tests can prove pre-cancel behavior only through the context-aware dispatch route; they cannot prove a caller-observable status/error from the legacy no-context bridge. This is an existing platform limitation, not a dfight-specific reason to redesign dispatch.

## Execution sequence

```text
resolved active REGULAR user sends !df[ight] <enemy>
  -> DispatchUserCommand authorizes REGULAR
  -> legacyAdapter.ExecuteContext
  -> dbzCommand.Execute
      -> if no first token: addressed/whisper source usage, FAILED
      -> DBZService.Fight(first token): remove first equality match or no-op
      -> DBZService.LevelUp(author name)
          -> H2 UPDATE character level
          -> H2 UPDATE stats free_stats + 5
      -> public/unaddressed fixed acknowledgement
      -> SUCCESSFUL
```

A missing enemy follows the same path after the list no-op. A failed first H2 statement prevents the second statement; any repository error is ignored by the command, after which the public send is attempted. No branch may query whether an enemy existed before levelling.

## Strict sequential TDD tracer plan

**Dirty-worktree protocol:** operate only in `/Users/ab/workspace/go-projects/zenbot`. Before each tracer capture `git status --short`, run `git diff --check && git diff --cached --check`, and inspect the immediate hunks in `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/service/dbz.go`, `internal/service/dbz_test.go`, `internal/repository/h2/dbz.go`, and the existing untracked `internal/listener/message/dbz_dispatch_test.go`. Do not reset, restore, clean, stash, stage, commit, bulk-format, or overwrite another owner's hunk.

Write and run exactly one focused behavioral RED, make its minimal GREEN, rerun it, then move on. A compilation, fixture, or H2 bootstrap error is not a RED. Existing current-green regressions must be labelled as such rather than fabricated as historical TDD evidence.

### Tracer 1 — real-H2 alias, first-match consumption, unconditional level-up, public acknowledgement

- **Test home/name:** extend the existing untracked `internal/listener/message/dbz_dispatch_test.go` with `TestDispatchUserCommandDBZFightAliasConsumesFirstMatchLevelsWithoutEnemyProofAndPublishes`.
- **Arrange:** open its existing isolated H2 fixture; seed `goku` at `level=2`, `free_stats=5`; construct the normal `service.DBZService`/bundle and register via `command.RegisterUserUtilitiesWithDirectAgent`; prepopulate its in-memory list with `frieza`, `frieza`, `cell`; authorize resolved active `goku`; dispatch whispered `!df frieza ignored`.
- **Assert:** exactly one output `{recipient:"", text:"Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!", whisper:false}`; enemy snapshot is `["frieza", "cell"]`; goku is `level=3`, `free_stats=10`; no second `frieza` was consumed and ignored token did not alter the chosen enemy.
- **RED command:** `go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFightAliasConsumesFirstMatchLevelsWithoutEnemyProofAndPublishes$'`
- **Required RED now:** current `dfight` sends an addressed whisper reply; state mutation can already succeed. It must fail on recipient/whisper delivery, not fixture setup.
- **Minimal GREEN:** change only successful `dfight` delivery in `internal/command/dbz.go` to checked `SendChatMessage("", fixedPayload, false)`. Preserve the order `Fight` then ignored `LevelUp`, payload bytes, and success result.

### Tracer 2 — nonexistent enemy is not a rejection or a no-level branch

- **Test home/name:** `TestDispatchUserCommandDBZFightMissingEnemyStillLevelsAndPublishes` in the same listener test file.
- **Arrange/assert:** real H2 goku at `level=2/free_stats=5`, fresh empty `DBZService`, authorized inbound whisper `!dfight absent`. Assert same sole public payload, empty enemies, and final `level=3/free_stats=10`.
- **RED boundary:** after Tracer 1 green, this may be green against existing source-shaped state sequencing. It is a regression/provenance test, not a valid fresh RED. Do not make `Fight` conditional, invent a combat-result seam, or temporarily break broad shared code merely to manufacture a failure.

### Tracer 3 — missing/blank argument exact usage and no mutation

- **Test home/name:** `TestDispatchUserCommandDBZFightMissingEnemyArgumentUsesSourceUsageWithoutMutation`.
- **Arrange/assert:** seeded enemy list and H2 character, authorized whisper `!dfight   `. Assert `FAILED` at direct command layer (or no success effects through dispatcher), exactly one `{recipient:"goku", text:"Example: !dfight enemy", whisper:true}`, unchanged list and DB row.
- **RED command:** `go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFightMissingEnemyArgumentUsesSourceUsageWithoutMutation$'`
- **Required RED now:** current payload is `dfight enemy`, not source exact usage.
- **Minimal GREEN:** change only the missing-argument literal in the `dfight` case to `"Example: "+c.engine.GetPrefix()+"dfight enemy"`; retain `reply`, `FAILED`, and early return.

### Tracer 4 — level-up error is swallowed; public send still determines Go result

- **Test home/name:** a command-layer test in `internal/command/dbz_test.go`, e.g. `TestDBZFightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy`.
- **Arrange/assert:** DBZ repo fake returns a sentinel from `LevelUp`; `DBZService` has one matching enemy; normal send succeeds. Assert `SUCCESSFUL`, nil command error, matching enemy consumed, exactly one public fixed payload, and one level-up attempt.
- **Boundary:** this should pass after existing command behavior is proven; record it as post-GREEN regression unless a legitimate current defect exists. It guards the source requirement that storage failure is not converted to no-reply/failed command before output.
- **Separate target adaptation test:** make `SendChatMessage` return a sentinel; assert enemy consumption and level-up attempt precede `FAILED`/send error. This is target-only transport behavior, not Saturn parity.

### Tracer 5 — registration/authorization/cancellation boundaries remain shared

- Use the existing `dbzDispatchEngine` recording pattern, without editing shared listener logic:
  - no DBZ bundle: `dfight` must not register;
  - denied resolved `REGULAR`: no `Fight`/`LevelUp` side effects and existing shared unauthorized response;
  - pre-cancelled context passed to `DispatchUserCommand`: no enemy/H2/output side effect.
- Do not manufacture REDs by changing `ResolveUserMetadata`, `DispatchUserCommand`, `Engine.IsUserAuthorized`, registry metadata, factory composition, or schema.

### Verification after each GREEN

```sh
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

Acceptance requires the focused packages to pass, diff checks clean, temporary RED isolation (if any) restored, and a path-scoped final diff limited to the two bounded `dfight` command corrections and its focused tests. Existing dirty DBZ/listener files are baseline, not permission to modify them.

## Concurrency, transaction, lifecycle, and exclusions

| Area | Source fact | Required disposition |
|---|---|---|
| enemy/list race | Saturn `ArrayList` has no synchronization | retain existing Go mutex for individual list safety; do not claim ordering parity or add broad serialization |
| fight + level-up | distinct in-memory removal and two H2 statements, no transaction | preserve non-atomic sequence; no transaction/lock/rollback/retry |
| simultaneous fights | source offers no defined outcome; multiple calls can each level a user even for absent enemy | do not add an existence reservation or test one schedule as contractual |
| H2 failure | level-up catches errors and command still queues success | ignore `LevelUp` error in dfight; no compensating state restoration |
| restart | enemy list is new/empty per DBZ service instance | do not persist, recover, sync, or share enemies |
| duplicate names | name-based un-ordered scalar update | no uniqueness/index/ownership/select-order repair |
| cancellation | no Saturn context equivalent | retain existing pre-execution Go cancellation only; do not add mid-sequence cancellation policy |

**Explicit exclusions:** combat/RNG/damage/enemy stats; enemy existence validation; enemy persistence/listing/expiry/caps/deduplication; player-versus-player behavior inferred from help text; schema/index/migration; repository redesign; transaction/lock/retry/rollback; duplicate-name repair; identity/role/listener policy changes; `dspawn` changes; DBZ register/stats/strength/help changes; factory/main/transport refactors; staging, commit, cleanup, or unrelated edits.

## File map and developer tier

- Saturn command/parser/output: `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZFightCommandImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/service/impl/OutService.java`
- Saturn service/SQL/lifecycle: `../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java`, `../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/util/DBZUtil.java`, `../../projects/saturn/src/main/java/org/saturn/app/facade/Base.java`, `../../projects/saturn/src/main/resources/schema-h2.sql`
- Zenbot command/registration/dispatch: `internal/command/dbz.go`, `internal/command/registry.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/listener/message/handlers.go`
- Zenbot state/H2: `internal/service/dbz.go`, `internal/repository/dbz.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql`
- Zenbot tests: `internal/command/dbz_test.go`, `internal/service/dbz_test.go`, `internal/repository/h2/dbz_test.go`, untracked `internal/listener/message/dbz_dispatch_test.go`
- Related handoffs: `.hermes/handoffs/dbz-command-current-architecture.md`, `.hermes/handoffs/dspawn-current-architecture.md`, `.hermes/handoffs/dbzstrength-current-architecture.md`

**Developer tier:** Senior Go maintainer comfortable with H2 statement ordering and active dirty-worktree coordination. The implementation is tiny; the risk is introducing speculative game semantics or altering visible delivery/error behavior at a shared DBZ command boundary.

## Baseline and diff safety

[TEST-BACKED] Before writing this handoff, the dirty checkout passed:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok   zenbot/internal/command
ok   zenbot/internal/listener/message
ok   zenbot/internal/service
ok   zenbot/internal/repository/h2

git diff --check && git diff --cached --check
exit 0; no output
```

[OBSERVED] `git status --short` already contained extensive modified/untracked migration work, including DBZ command/service/repository tests and the untracked listener DBZ dispatch tracer. The scoped diff shows current `dfight` code as pre-existing dirty work alongside DBZ help/register/strength/spawn changes. This task authorizes no application/test/source/schema change and creates only this handoff.
