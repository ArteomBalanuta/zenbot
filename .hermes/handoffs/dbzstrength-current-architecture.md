# DBZ strength (`dbzstr`) strict-parity architecture

**Purpose:** bounded implementation handoff for Saturn’s DBZ strength-spend vertical. This artifact authorizes neither a DBZ redesign nor any change to identity, authorization, schema, or unrelated dirty work.

## 1. Decision and acceptance boundary

[RECOMMENDED] Implement/recover only **`dbzstr` / `dstr` / `daddstr`**. It is a self-only, `REGULAR`-authorized stat mutation using the inbound message author—not an argument target. Preserve Saturn’s deliberately weak accounting semantics:

1. validate only that the first argument is a positive Java/Go signed `int`;
2. read current `free_stats` separately;
3. reject only `free_stats <= 0`;
4. update `str += requestedAmount` and `free_stats -= requestedAmount` without bounding the amount to available stats;
5. use no transaction, lock, retry, conditional predicate, compensating write, or duplicate-name repair.

This means a character with one free stat may run `!daddstr 2`, gain two strength, and retain `free_stats = -1`. That is an intentional source quirk, not a bug to “fix” within this vertical.

[OBSERVED] The dirty Zenbot checkout already has a partial implementation in `internal/command/dbz.go`, `internal/service/dbz.go`, and `internal/repository/h2/dbz.go`; current tests cover malformed input and repository mutation but do not cover the strength route end-to-end or its output/overspend/error contract.

## 2. Saturn source contract

### Surface, role, subject, and arguments

| Item | Source-grounded contract |
|---|---|
| Command class | `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZAddStrCommandImpl.java` |
| Exact aliases | `dbzstr`, `dstr`, `daddstr` (`@CommandAliases`, line 17) |
| Role | `Role.REGULAR` (`getAuthorizedRole`, lines 24–27) |
| Subject | `chatMessage.getNick()` only (`execute`, line 31); no target argument, trip lookup, DBZ-name ownership lookup, or identity inference |
| Required argument | First parsed trailing token (`requiredIntArgument(0, "daddstr amount", value -> value > 0)`) |
| Accepted values | `Integer.parseInt(trimmedToken)` values strictly greater than zero; signed 32-bit range only |
| Ignored input | Any tokens after the first valid amount are not read by `DBZAddStrCommandImpl` |
| Invalid/missing/zero/negative/out-of-range | `UserCommandBaseImpl.requiredIntArgument` calls `failWithUsage("daddstr amount")`: enqueue addressed reply `Example: <prefix>daddstr amount`, preserving inbound whisper state; return `Status.FAILED` |

[OBSERVED] `UserCommandBaseImpl.java:154-180,191-202` trims the selected token before Java integer parsing. Thus whitespace around a sole token is accepted only insofar as the shared parser supplies it as an argument; blank/missing values, `0`, negative values, non-integers, and overflow are all the same exact usage branch.

### Exact observable output

| Condition | Saturn output and status |
|---|---|
| Invalid amount | Addressed to author, inbound whisper state, exact `Example: <prefix>daddstr amount`; `FAILED` |
| `getFreeStats(author) <= 0` | Public/unaddressed queue payload, exact `You don't have free stats. Level up!`; `SUCCESSFUL` |
| Positive amount and free stats > 0 | Public/unaddressed queue payload containing exactly the decimal requested amount (for example `2`); `SUCCESSFUL` |

`DBZAddStrCommandImpl.java:37-48` invokes the one-argument `OutService.enqueueMessageForSending(String)` for both successful branches. `OutService.java:35-43` normalizes/enqueues that raw public payload; it is not addressed and does not inspect inbound whisper state. The invalid-input helper instead calls the three-argument addressed overload through `replyToAuthor`.

### Persistence and free-stat behavior

[OBSERVED] Saturn uses these methods and SQL:

```text
DBZAddStrCommandImpl.execute
  -> DBZImpl.getFreeStats(author)
       DBZUtil.FREE_STATS:
       SELECT free_stats from dbz_stats
       WHERE char_id = (SELECT id from dbz_characters WHERE name = ?)
  -> only if returned freeStats > 0:
       DBZImpl.addStr(author, amount)
       DBZUtil.UPDATE_STR_BY_NAME:
       UPDATE dbz_stats
       SET str = str + ?, free_stats = free_stats - ?
       WHERE char_id = (SELECT id FROM dbz_characters WHERE name = ?)
```

Sources: `DBZImpl.java:101-118,207-228`; `DBZUtil.java:15-24,69-72`; `DBZAddStrCommandImpl.java:33-45`.

Important source facts:

- The precheck tests only the old value `> 0`. It does **not** require `free_stats >= amount`.
- The update has no `free_stats > 0` or `free_stats >= ?` predicate. It subtracts the full requested amount.
- Only strength consumes free stats. Saturn’s adjacent `addAgi`, `addVit`, and `addEne` updates do not decrement them; they are out of this scope.
- Both subqueries select by non-unique `dbz_characters.name`, without `ORDER BY`. Saturn’s schema allows duplicate character names and has no DBZ name index/uniqueness constraint (`../../projects/saturn/src/main/resources/schema-h2.sql`). Do not add ordering, generated-key logic, constraints, or an ownership rule here.
- `DBZImpl.getFreeStats` catches `SQLException` and returns `-1`; `DBZImpl.addStr` catches/logs SQL errors and returns `-1`, which the command ignores. A source DB failure therefore falls into the public no-free-stat success branch if it occurs during the read, or still emits the requested amount after a write failure if the read succeeded.

## 3. Zenbot current map and identified parity gaps

| Layer | Current source | Observed behavior / implementation disposition |
|---|---|---|
| Catalog | `internal/command/registry.go:RegisterAll` | Exact three aliases and `model.REGULAR` already declared. Keep unchanged. |
| Conditional registration | `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` | Registers DBZ strength only when `bundle(e).DBZ != nil`. Keep this fail-closed composition gate. |
| Construction | `internal/command/handlers.go:newCommand` | DBZ canonical returns `*dbzCommand`. Keep unchanged. |
| Shared gate | `internal/listener/message/handlers.go:DispatchUserCommand.Handle` | Resolves alias then requires non-nil resolved active author and `IsUserAuthorized(author, REGULAR)` before executing. Keep unchanged. |
| Command | `internal/command/dbz.go:dbzCommand.Execute`, `case "dbzstr"` | Parses `args[0]` through `strconv.Atoi(strings.TrimSpace(...))`, requires `n > 0`, reads free stats, calls `AddStrength`, currently discards write errors. It is correctly caller-name-only and permits overspend. |
| Service | `internal/service/dbz.go:DBZService.FreeStats/AddStrength` | Narrow repository delegation. Current `FreeStats` maps missing row to `-1`, but returns actual repository errors. |
| H2 | `internal/repository/h2/dbz.go:FreeStats/AddStrength` | Bound lookup join for read; strength update increments `str` and decrements `free_stats` in one statement, with no availability predicate. It uses the requested amount for both terms. |
| Schema | `internal/repository/h2/schema-h2.sql:41-50` and `resources/schema-h2.sql` | Compatible loose DBZ table shape; no schema edit required. |
| Reply helper | `internal/command/handlers.go:reply` | Addresses `message.Name` and preserves inbound whisper. Correct for source usage failures, but **not** for either valid DBZ-strength success branch. |

### Required minimal command correction

[RECOMMENDED] In the `dbzstr` branch only:

- retain `reply(&c.commandBase, "Example: "+prefix+"daddstr amount")` for invalid/missing input;
- replace `reply` with a checked `SendChatMessage("", text, false)` for both `free <= 0` and the decimal amount acknowledgement;
- return `FAILED` only if that target transport call errors, after retaining any prior DB effect. This is the existing Go output-adapter convention already documented for DBZ registration; Saturn’s queue API does not expose the same checked return path.

Do not edit the shared `reply` helper to manufacture source public delivery: that would alter unrelated commands and dirty shared infrastructure.

### Explicit source/target error-semantics decision

[OBSERVED] Zenbot currently differs from Saturn when `DBZRepository.FreeStats` returns an error: `dbzCommand` returns `FAILED` before output, whereas Saturn converts its read failure to `-1` and publicly emits the no-free-stat message with success. Zenbot already discards `AddStrength` errors, matching the source’s post-read visible success shape more closely.

[RECOMMENDED] Treat this as a required **explicit maintainer decision before GREEN**, not an accidental cleanup:

- **Strict Saturn-observable option (default for this parity vertical):** `DBZService.FreeStats` converts repository read errors to `-1, nil` (or the command intentionally treats its error as `-1`) so the command emits the public no-free-stat success reply. Preserve diagnostic logging at the repository/adapter boundary if already available; do not add retries.
- **Go operational-safety exception:** retain the current error propagation only if the responsible maintainer explicitly records this as an approved target divergence. In that case the handoff/test must label the vertical **not source-exact for read failure**, rather than claiming full strict parity.

Neither option changes authorization, identity, schema, or who may mutate a character. Do not silently make the update transactional, bounded, or conditional to hide storage problems.

## 4. Transaction, concurrency, cancellation, and error matrix

| Scenario | Source semantics | Required Zenbot disposition |
|---|---|---|
| Pre-cancelled Go context | No Saturn context equivalent | Keep existing Go adaptation: `dbzCommand.Execute` returns `FAILED` with `ctx.Err()` before lookup, mutation, or output. No new cancellation policy after SQL begins. |
| Missing stats/character | `getFreeStats` yields `-1` after no row; public no-free message, `SUCCESSFUL` | Existing H2/service missing mapping naturally yields `-1`; assert exact public reply. |
| Amount greater than available | One positive free stat is enough to reach update; requested amount is subtracted | Preserve; assert an overspend example (`free=1`, amount `2` -> `str +2`, `free=-1`, public `2`). |
| Two simultaneous requests | Separate check then update; both may see positive free stats and both decrement | Do not serialize command/repository, add locking, use a transaction, or change SQL to conditional update. Test only an invariant tolerant of scheduling: negative free stats and/or both requested increments are source-permitted. |
| Duplicate same-name DBZ rows | Scalar `name` subquery has no order and no ownership binding; database behavior/error is source-shaped, not deterministic | Do not introduce unique/index/order/target selection repair. A duplicate-row concurrency test must not assert a chosen character ID. |
| Free-stat read SQL error | Caught, logged, `-1`, then public no-free success | Follow the explicit decision in §3; strict option converts to the source result. |
| Strength update SQL error | Caught/logged; command ignores return and public amount success remains | Preserve current command-side ignored `AddStrength` error; no rollback/retry/compensation. |
| Public output adapter fails | Saturn queue behavior is unchecked at command boundary | Target adaptation: return `FAILED` with send error after completed/no-op DB work; no retry or rollback. |
| DBZ bundle absent | Not a source command execution condition | Keep Zenbot registration gate and current `DBZ state unavailable` failure; do not register strength without a DBZ service. |
| Denied/unresolved author | Shared pre-command authorization boundary | No DBZ repository call and existing shared unauthorized handling. Do not derive authorization from nick, trip, character row, or command argument. |

## 5. Strict sequential TDD tracer plan

**Dirty-worktree protocol:** work only in `/Users/ab/workspace/go-projects/zenbot`. Before each unit capture `git status --short`, run `git diff --check && git diff --cached --check`, and inspect the immediate hunk in every shared dirty file. Do not use reset/restore/clean/stash/stage/commit, broad formatting, or any checkout operation. `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/command/registry.go`, `internal/command/dispatch_adapter.go`, and `internal/repository/h2/dbz_test.go` are already dirty; coordinate rather than overwrite an owned hunk.

A valid RED is a focused behavioral failure caused by temporarily isolating the named DBZ-owned behavior. A compile, H2-fixture, or generic harness error is not valid RED. Complete one RED→minimal GREEN before adding the next test.

### Tracer 1 — authorized alias, public overspend acknowledgement, and real H2 mutation

- **Test home:** extend `internal/listener/message/dbz_dispatch_test.go` with a real-H2 recording engine following its existing DBZ stats setup.
- **Name:** `TestDispatchUserCommandDBZStrengthAliasOverspendsAndPublishesPublicAmount`.
- **Arrange:** seed `goku` at `free_stats=1`, `str=3`; allow `REGULAR`; make inbound `!daddstr 2 ignored` a whisper; resolve active `goku`; register through `command.RegisterUserUtilitiesWithDirectAgent`.
- **Assert:** exactly one reply `{recipient:"", text:"2", whisper:false}`; final row `str=5`, `free_stats=-1`; no use of trailing token as target/input.
- **Pre-RED boundary:** in `internal/command/dbz.go`, temporarily make only successful `dbzstr` output use the existing addressed `reply` behavior (the current checkout already does this). The tracer must fail specifically on recipient/whisper mismatch while proving the existing mutation occurred.
- **Minimal GREEN:** change only the two successful DBZ-strength sends to checked `SendChatMessage("", payload, false)`. Do not alter parsing, query, synchronization, or shared reply.
- **Commands:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZStrengthAliasOverspendsAndPublishesPublicAmount$'
  ```
  Run once for behavioral RED and immediately again after the minimal GREEN.

### Tracer 2 — invalid amount uses source usage and no state mutation

- **Test home/name:** `internal/listener/message/dbz_dispatch_test.go`, `TestDispatchUserCommandDBZStrengthInvalidAmountRepliesUsageWithoutMutation`.
- **Cases:** missing, `0`, `-1`, `nope`, and one out-of-range decimal; execute one table row at a time if strict provenance is required. Each expects addressed author, inbound whisper retained, exact `Example: !daddstr amount`, `FAILED` at command component level, and unchanged seeded row.
- **Pre-RED boundary:** isolate only the `n <= 0 || Atoi error` usage branch in `internal/command/dbz.go`; do not change `args`, shared dispatch, or role metadata.
- **Minimal GREEN:** restore exactly the current first-token positive-int validation and usage reply. No clamping, alternate syntax, or target argument.

### Tracer 3 — no free stats is public success and skips update

- **Test home/name:** `internal/listener/message/dbz_dispatch_test.go`, `TestDispatchUserCommandDBZStrengthNoFreeStatsPublishesPublicMessageWithoutMutation`.
- **Arrange/assert:** real H2 `free_stats=0`, `str=3`, incoming whisper `!dstr 1`; one public/unaddressed non-whisper reply exactly `You don't have free stats. Level up!`; row unchanged.
- **Pre-RED boundary:** isolate only the `free <= 0` early-return/output branch. Do not break `FreeStats` SQL or the fixture.
- **Minimal GREEN:** restore early return before `AddStrength`, with the public source output. This establishes that a negative value also follows this same branch.

### Tracer 4 — H2 update preserves requested-subtraction quirk

- **Test home/name:** `internal/repository/h2/dbz_test.go`, `TestDBZStrengthRealH2AddsRequestedAmountAndAllowsNegativeFreeStats`.
- **Arrange/assert:** direct known character/stat row with `free=1`, `str=3`; call `AddStrength(ctx, "goku", 2)`; assert `str=5`, `free=-1`.
- **Pre-RED boundary:** isolate only the `free_stats=free_stats-CAST($2 AS INTEGER)` portion of `(*Database).AddStrength`; the test must fail on free-stat value, not on fixture/connection failure.
- **Minimal GREEN:** restore the bound same-amount subtraction. Do not add `WHERE free_stats >= ...`, an explicit transaction, or a unique/order constraint.

### Tracer 5 — free-stat read failure policy is recorded, not guessed

- **Test home:** `internal/service/dbz_test.go` for strict-source conversion, plus `internal/command/dbz_test.go` or the listener tracer for the output outcome.
- **Strict-source test:** repository fake returns a sentinel error from `FreeStats`; expected service/command result is public exact no-free text and success, with no `AddStrength` call.
- **RED/GREEN boundary:** only after the §3 policy owner chooses strict source parity. Temporarily isolate the chosen conversion point, observe the behavioral failure, then restore it. If the owner chooses the Go exception, write the failing/parity test as skipped/rejected design evidence and label that exception explicitly; do not add a deceptive current-green test.

### Tracer 6 — authorization and pre-cancellation stay outside DBZ policy

- Add strength-specific dispatch tests using the existing recording repository pattern:
  - denied `REGULAR` author: no `FreeStats`/`AddStrength` call, no mutation, exactly existing shared unauthorized reply;
  - pre-cancelled context: no DBZ repository call and no output.
- Do not manufacture these REDs by editing `ResolveUserMetadata`, `DispatchUserCommand`, `Engine.IsUserAuthorized`, `reply`, factory composition, or schema. Those are shared security/infrastructure boundaries.

### Focused final verification

After every individual GREEN is recorded:

```sh
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

Acceptance requires all focused packages passing, both diff checks clean, temporary RED isolations restored, and a path-scoped final diff containing only approved DBZ strength tests plus the minimal DBZ-strength code changes. Existing dirty paths are baseline, not permission to alter them.

## 6. Explicit exclusions

No work here may add or change: DBZ registration/stats/help/agility/vitality/energy/fight/spawn behavior; combat/RNG; character ownership/account binding; nick/trip identity inference; role/access policy; listener authorization; schema/index/constraint/migration; transaction/lock/retry/reconciliation; duplicate-name repair; factory/main refactor; shared reply helper; agent/moderation/proxy work; staging; commits; cleanup; or unrelated test edits.

The adjacent stat commands are not interchangeable with strength: their Saturn SQL does not decrement free stats. Do not generalize strength behavior into a generic “spend stat” abstraction without a separately source-grounded design.

## 7. Evidence, limits, and developer tier

### Evidence map

- Saturn command: `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZAddStrCommandImpl.java`
- Saturn parser/output service: `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/service/impl/OutService.java`
- Saturn DBZ service/SQL: `../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java`, `../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/util/DBZUtil.java`
- Saturn schema: `../../projects/saturn/src/main/resources/schema-h2.sql`
- Zenbot command/registration/dispatch: `internal/command/dbz.go`, `internal/command/registry.go`, `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/listener/message/handlers.go`
- Zenbot service/repository/schema: `internal/service/dbz.go`, `internal/repository/dbz.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql`
- Zenbot current tests: `internal/command/dbz_test.go`, `internal/listener/message/dbz_dispatch_test.go`, `internal/service/dbz_test.go`, `internal/repository/h2/dbz_test.go`
- Accepted prerequisite handoffs: `.hermes/handoffs/dbz-command-current-architecture.md`, `.hermes/handoffs/dbzstats-tdd-recovery-architecture.md`, `.hermes/handoffs/dbzregister-current-architecture.md`

[LIMITATION] No Saturn DBZ strength command/service/H2 behavioral test was found under the inspected `../../projects/saturn/src/test/java`; claims are grounded in the Java production command, service, SQL constants, output service, and schema. The source duplicate-name scalar-subquery result is deliberately not deterministic and must not receive a fabricated test assertion.

**Recommended developer tier: Senior Go maintainer with H2/JDBC semantics knowledge and active dirty-worktree coordination.** The code change is small; preserving intentional overspend, public-vs-addressed output, swallowed-write failure visibility, and no-new-security-policy constraints is not.

## 8. Baseline captured for this handoff

[TEST-BACKED] On the inspected dirty tree:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok   zenbot/internal/command
ok   zenbot/internal/listener/message
ok   zenbot/internal/service
ok   zenbot/internal/repository/h2

git diff --check && git diff --cached --check
exit 0; no output
```

[OBSERVED] At baseline the worktree had extensive pre-existing modified/untracked source, tests, and handoffs, including DBZ-related shared files. This task created only this handoff and made no application source, test, schema, staging, or commit change.
