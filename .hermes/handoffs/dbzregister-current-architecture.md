# `dbzregister` source-exact persistence vertical

## Decision

[RECOMMENDED] Implement/recover **only `dbzregister` / `dreg` / `dr`** as one bounded self-registration vertical. Preserve Saturn’s observable persistence quirk exactly: a character insert, a non-transactional name lookup, then a stats insert; duplicate names are allowed; there is no existence check, uniqueness constraint, deterministic duplicate selection, or rollback.

The broad authorization to continue the parity rewrite permits this source-exact quirk **only within this bounded command**. It does **not** authorize a new security policy, nick/trip identity inference, character ownership rule, deduplication, transaction, unique index, retry, or “repair” workflow. The existing shared `REGULAR` authorization gate remains authoritative. This is a documented decision rather than a silent safety change.

The current Go repository already deliberately implements the three database operations non-atomically, and the command currently deliberately drops its returned error before acknowledging success. The implementation vertical still needs source-exact public delivery coverage and an explicit failure/partial-state contract before any shared-file change is accepted.

## Source contract

### Command surface

[OBSERVED] `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZRegisterCommandImpl.java`:

- aliases are exactly `dbzregister`, `dreg`, `dr` (`@CommandAliases`, line 16);
- required role is exactly `Role.REGULAR` (lines 23–26);
- it takes no command argument: `execute()` assigns `author = chatMessage.getNick()` and calls `engine.dbzService.register(author)` (lines 29–33). Parsed trailing arguments are ignored;
- it unconditionally queues exactly `Successfully registered character: <author>` through the one-argument `OutService.enqueueMessageForSending` overload (line 34), then returns `Status.SUCCESSFUL` (line 37);
- it does not read the inbound whisper flag and does not target the author. Per Saturn `OutService`, that overload is public/unaddressed.

[OBSERVED] The shared Saturn user-command parser in `UserCommandBaseImpl.java:51-71` tokenizes trailing text, but this command does not inspect the resulting argument list. The source authorization dispatcher authorizes the resolved command before `execute()` (`UserCommandBaseImpl.java:83-102`); `DBZRegisterCommandImpl` itself adds no identity or authorization policy.

### Service and persistence state

[OBSERVED] `DBZService.register(String)` is a void method (`../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java:3-6`). `DBZImpl.register` (`service/impl/DBZImpl.java:26-76`) performs this sequence on one JDBC connection:

```text
1. INSERT INTO dbz_characters(name, level, created_on) VALUES(author, 1, now)
2. SELECT id FROM dbz_characters WHERE name = author
3. INSERT INTO dbz_stats(char_id, free_stats, str, agi, vit, ene, created_on)
   VALUES(selected-id, 0, 1, 1, 1, 1, now)
```

The source closes the first statement before the select. It assigns `charId = 0`, uses the first `ResultSet` row if present, and throws `SQLException("Character not found: " + name)` only if no row is returned. Every `SQLException` from any stage is caught, logged, and swallowed by `register`; the command then still publicly acknowledges success.

[OBSERVED] `DBZUtil.java:4-7,59-62` supplies those exact SQL shapes. No `ORDER BY`, duplicate check, generated-key API, savepoint, transaction demarcation, rollback, or retry appears in the command/service path.

[OBSERVED] Saturn schema `../../projects/saturn/src/main/resources/schema-h2.sql:41-50` defines:

```text
dbz_characters(id identity PK, name VARCHAR nullable/non-unique,
               level INTEGER nullable, created_on BIGINT NOT NULL)
dbz_stats(id identity PK, char_id BIGINT NOT NULL FK dbz_characters(id),
          str/agi/vit/ene/free_stats INTEGER nullable, created_on BIGINT NOT NULL)
```

There are no DBZ indexes and no cardinality constraint requiring one stats row per character or one character per name.

## Current Zenbot map

| Concern | Current Zenbot evidence | Required vertical disposition |
|---|---|---|
| Catalog metadata | `internal/command/registry.go:RegisterAll` declares `dbzregister` aliases `dbzregister`, `dreg`, `dr`, role `model.REGULAR`. | Keep; no catalog edit. |
| Concrete construction | `internal/command/handlers.go:newCommand` maps `dbzregister` to `*dbzCommand`. | Keep; no generic fallback. |
| Public registration | `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` appends `dbzregister` only when `bundle(e).DBZ != nil`. | Keep this fail-closed composition gate. |
| Shared authorization | `internal/listener/message/handlers.go:DispatchUserCommand.Handle` authorizes the resolved active `Context.Author` for the command role before calling `legacyAdapter.ExecuteContext`. | Keep. Do not use a DBZ row, requested argument, nick string alone, trip, or repository lookup as a new identity authority. |
| Input subject | `internal/command/dbz.go:Execute` uses `c.message.Name` as `author`; it does not read `args` in the registration branch. | Matches command self-registration/ignored arguments after trusted dispatch binds the message. |
| Service seam | `internal/service/dbz.go:DBZService.Register` calls `Repo.RegisterCharacter(ctx, name, s.now)`. | Keep narrow DBZ seam; do not widen `Bundle` or expose `*sql.DB` to command code. |
| Repository seam | `internal/repository/dbz.go:DBZRepository.RegisterCharacter(context.Context, string, func() int64) (int64, error)`. | Existing return is a Go diagnostic seam; command must not turn it into a new user-visible failure policy without a separately approved semantics change. |
| H2 operations | `internal/repository/h2/dbz.go:RegisterCharacter` inserts character, `SELECT id ... WHERE name=$1`, then inserts stats; its comment explicitly records source non-atomicity. | Keep exact order and loose duplicate selection. |
| Schema | `internal/repository/h2/schema-h2.sql:41-50` is embedded at `h2.Database` bootstrap; `resources/schema-h2.sql:41-50` mirrors it. | No schema/index/constraint/migration change. |
| Factory | `internal/factory/engine_factory.go:73-90` supplies `&service.DBZService{Repo: dbz}` only where the repository implements both SQL DB access and `repository.DBZRepository`. | Keep; no main/factory modification required. |
| Current output mismatch | `internal/command/dbz.go:44-46` calls `reply`, which targets `message.Name` and preserves inbound whisper via `internal/command/handlers.go:70-72`. | Correct only this registration-success delivery to `SendChatMessage("", exactText, false)`; source is public/unaddressed. |

### Target sequence

```text
inbound !dbzregister / !dreg / !dr [ignored tokens]
  -> ResolveUserMetadata binds active author
  -> DispatchUserCommand resolves concrete registered alias
  -> IsUserAuthorized(resolved author, REGULAR)
  -> legacyAdapter.ExecuteContext
  -> dbzCommand.Execute(canonical=dbzregister)
  -> DBZService.Register(ctx, message.Name)
  -> H2 RegisterCharacter:
       INSERT character(level=1, timestamp #1)
       SELECT first id WHERE name=$1 (no ORDER BY)
       INSERT stats(free=0,str=1,agi=1,vit=1,ene=1,timestamp #2)
  -> discard returned Go error, as current source-shaped command does
  -> SendChatMessage("", "Successfully registered character: "+message.Name, false)
```

The command must retain the existing early `ctx.Err()` check before persistence. Saturn has no cancellable command context; this is a pre-existing Go runtime safety boundary. It must cause no write and no acknowledgement when already cancelled, but it must not be generalized into rollback/cancellation policy after a write has begun.

## Exact H2 semantics and failure branches

### Normal and duplicate calls

| Case | Database effect | Source/target reply and status |
|---|---|---|
| First registration, all three operations succeed | One character `(author,1,t1)` and one stats row linked to the selected ID `(0,1,1,1,1,t2)`. | Public/unaddressed exact success text; `SUCCESSFUL`. |
| Sequential duplicate same name | Inserts another character. Name select is not ordered, so it can select any matching ID; inserts one stats row for that selected character. It need not select the newly inserted ID. | Same public success; `SUCCESSFUL`. |
| Concurrent duplicate names | Each invocation independently inserts then independently selects an unspecified matching row. Stats rows can attach to an earlier or other concurrent same-name character; outcomes depend on database scheduling/selection. | Each normal completion acknowledges success. No atomicity, uniqueness, or “correct row” promise. |
| Same name already has zero or many stats | Registration does not inspect existing stats and appends a new character plus stats attempt. | Same public success if the command reaches output. |

### Failure and partial-state matrix

| Failure point | Source behavior | Required Go behavior for parity vertical |
|---|---|---|
| Pre-cancelled context | Not represented by Saturn. | Existing target safety adaptation: return `FAILED` with `context.Canceled`; zero SQL and zero output. Test it; do not add cancellation mechanics beyond this check. |
| Character insert fails | `DBZImpl.register` catches/logs and returns; command still queues success. No later source statements. | H2 returns error; `DBZService.Register` returns it; `dbzCommand` intentionally discards it, sends public success, returns `SUCCESSFUL`. No retry. |
| Select fails or returns no row | Source catches/logs (`Character not found: <name>` for no row); no stats insert; command still success-acks. Prior character insert can remain committed. | H2 returns error and stops before stats insert; command discards it and success-acks. Preserve the orphan-character possibility. |
| Stats insert fails | Source catches/logs; inserted character and any selected row remain. | H2 returns error after preceding committed operations; command discards it and success-acks. No rollback or compensating delete. |
| H2/connection error while final public `SendChatMessage` | Saturn queue behavior is not expressed as a checked status in this command. | Existing Go transport API returns an error. Treat this as a narrow target adapter difference: return `FAILED` with that error **after** retaining completed DB effects; no retry/rollback. Add an explicit test only after the normal source tracer is green. |

`RegisterCharacter` returns the selected ID only for diagnostics/tests. Its value is not command output and must not be exposed to callers.

## Strict sequential tracer plan

**Dirty-worktree guard for every unit:** work only under `/Users/ab/workspace/go-projects/zenbot`; first capture `git status --short`, run `git diff --check && git diff --cached --check`, and inspect only local hunks in `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/dbz_test.go`, `internal/service/dbz.go`, and `internal/listener/message/dbz_dispatch_test.go`. All are already shared/dirty except some handoffs. Do not reset, restore, stash, stage, commit, bulk-format, or overwrite another owner’s hunk.

Every tracer is exactly **one RED immediately followed by its minimal GREEN**. Do not write the next test until the preceding one is green. A fixture/compiler error is not a behavioral RED.

### Tracer 1 — real H2 public registration, aliases, and ignored arguments

- **RED test:** add `TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess` in `internal/listener/message/dbz_dispatch_test.go` using its existing isolated H2 fixture and normal `RegisterUserUtilitiesWithDirectAgent` registration. Drive `!dreg ignored tokens` through `ResolveUserMetadata` and `DispatchUserCommand` as an authorized active `goku` from a whispered input.
- **Assertions:** precisely one `dbz_characters` row with `name=goku`, `level=1`; precisely one linked `dbz_stats` row with `free_stats=0,str=1,agi=1,vit=1,ene=1`; no character called `ignored`; one reply exactly `Successfully registered character: goku`, recipient `""`, whisper `false`.
- **RED command:**
  ```sh
  go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess$'
  ```
- **Expected current behavioral RED:** persistence passes, but output is targeted to `goku` and whispered because current code calls `reply`.
- **Minimal GREEN:** in `internal/command/dbz.go` registration branch only, replace `reply(...)` with checked `SendChatMessage("", "Successfully registered character: "+author, false)` and return `FAILED` only when that send returns an error. Retain `_ = b.DBZ.Register(ctx, author)`, status success after a successful send, aliases, role, registration gate, factory, schema, and all shared helpers unchanged.
- **GREEN command:** rerun the exact focused command; require PASS.

### Tracer 2 — source duplicate/non-transactional selection contract

- **RED test:** add a real-H2 repository test named `TestDBZRegisterDuplicateNameKeepsLooseNameSelectionAndAppendsStats` in `internal/repository/h2/dbz_test.go`. Seed a first character named `vegeta`, then use `RegisterCharacter` twice with deterministic timestamps; assert two character rows, two stats rows total, defaults and timestamps, and that each stats `char_id` references *a* `vegeta` row. Do **not** assert selected ID equals the newest character.
- **RED command:**
  ```sh
  go test -count=1 ./internal/repository/h2 -run '^TestDBZRegisterDuplicateNameKeepsLooseNameSelectionAndAppendsStats$'
  ```
- **Expected RED rule:** only add this test if its specified observable property is absent. On the current dirty tree, the existing `TestDBZRegistrationDuplicateAndMissingReadSemantics` already proves the key two-character/two-stats property, so a new equivalent test will be green immediately and is **regression evidence, not valid RED**. If strict recovery evidence is required, temporarily isolate only the final stats insert in a local unstaged edit, observe the expected one-stats failure, then immediately restore it; never alter duplicate selection/order or make the repository transactional to manufacture RED.
- **Minimal GREEN:** restore/retain the three current statements in source order. Do not add `ORDER BY`, `RETURNING`, a generated-key query, `BEGIN`, `ROLLBACK`, a unique constraint, index, lock, retry, or compensating delete.
- **GREEN command:** same focused repository command; require PASS.

### Tracer 3 — partial failure remains success-visible and non-rollback

- **RED test:** introduce a narrow fake `repository.DBZRepository` in `internal/command/dbz_test.go` whose `RegisterCharacter` records the self name then returns a sentinel persistence error. Execute normal `dbzregister` with a whispered message and ignored trailing tokens.
- **Assertions:** repository receives only the message author; status is `SUCCESSFUL`, nil command error; exactly one public/unaddressed success acknowledgement. The fake models any swallowed Saturn SQL failure; the real-H2 partial-state shape remains covered by repository tests.
- **RED command:**
  ```sh
  go test -count=1 ./internal/command -run '^TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails$'
  ```
- **Expected RED:** valid only if command code starts propagating `Register` errors or suppressing output. Current code already discards the error; after Tracer 1 it should be green. Therefore this is a post-GREEN regression test in the current checkout, not historical RED evidence. Do not artificially change production error handling merely to produce RED; record the existing implementation provenance honestly.
- **Minimal GREEN if a defect is revealed:** discard only the `Register` error at the command boundary; do not change H2 error propagation, introduce a logger/retry, or alter service/repository interfaces.
- **GREEN command:** same focused command.

### Tracer 4 — shared authorization and cancellation boundaries

- **RED tests:** extend the listener tracer area with (a) denied `REGULAR` dispatch: no `RegisterCharacter` call/no DB rows and existing shared unauthorized reply; (b) pre-cancelled context: no repository call/no output and adapter logs the returned cancellation error.
- **RED commands:** run each named test alone in `./internal/listener/message`.
- **Minimal GREEN:** only repair the existing shared dispatch/context propagation if these tests expose a defect; do not alter DBZ authorization/identity, output policy, or persistence semantics. Current `dbzstats` dispatch tests already demonstrate the corresponding authorization shape, but `dbzregister` must get direct coverage before claiming it.
- **GREEN command:** rerun the exact test after each immediate repair.

### Final verification

Only after all preceding GREEN outcomes:

```sh
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

Then inspect the final diff by path. Acceptance is limited to the planned DBZ registration tests and the minimal command delivery edit; pre-existing dirty paths are baseline preservation, not owned changes.

## Complexity, risks, and QA

| Area | Complexity | Principal risk | Required control |
|---|---:|---|---|
| Public delivery | Low | Visible mismatch: target reply/whisper vs source public queue | Exact recipient/whisper/text assertion from whispered input. |
| Duplicate selection | Medium | “Safer” generated key, ordering, uniqueness, or transaction silently changes source behavior | Test loose duplicate linkage; prohibit ordering/constraints/rollback. |
| Partial failures | Medium | User-visible target error policy or accidental rollback differs from source success acknowledgement | Fake error command test; real-H2 failure-path design review; no retry. |
| Concurrency | High semantic risk, low code scope | Concurrent same-name stats can attach to an arbitrary same-name character | State non-determinism explicitly; use no race-sensitive ID assertion or new locking. |
| Security | No implementation complexity | Broadening registration/authorization or treating DBZ name as identity | Preserve `REGULAR`, resolved-active-user gate, and conditional DBZ service registration. |
| Dirty shared files | Medium merge risk | Clobbering active DBZ/dispatch work | Coordinate before `dbz.go` or DBZ tests; path-scoped diff review. |

### QA acceptance checklist

1. `dbzregister`, `dreg`, and `dr` remain the only aliases; role remains `REGULAR`.
2. Only caller `message.Name` is persisted; zero/one/many arguments do not alter the character name.
3. A normal real-H2 call creates the source defaults and sends exact public/unaddressed acknowledgement, even for whispered input.
4. Sequential duplicates remain admitted; no unique/index/order/transaction behavior is added.
5. A persistence error still produces source-shaped success acknowledgement; no retry or rollback is added.
6. Denied and pre-cancelled calls cause no write; this is shared authorization and Go cancellation safety, not a DBZ auth change.
7. Final focused packages pass and both staged/unstaged diff checks are clean.

## Scope exclusions and handoffs

### Explicit exclusions

No DBZ stats/read change, `dbzstr`, `dfight`, enemy persistence, spawn state, combat/RNG, character ownership/account binding, database migration, DBZ unique index, transaction, locking/retry/repair job, raw SQL command, role/access change, identity inference, factory/main refactor, transport abstraction, help payload rewrite, proxy/Whiskey, agent moderation, staging, commit, or cleanup is part of this vertical.

### DBZ recovery/help/spawn handoff boundaries

- `.hermes/handoffs/dbz-command-current-architecture.md` correctly identifies registration’s duplicate-name/non-atomic selection as the persistence risk; this document supplies the implementation decision and exact public-output correction.
- `.hermes/handoffs/dbzstats-tdd-recovery-architecture.md` / implementation / QA own the read-only stats route; do not revise stats rendering/query behavior here.
- `.hermes/handoffs/dbzhelp-current-architecture.md`, implementation, and QA own the independently public fixed help payload. Registration’s public output follows the same Saturn one-argument queue principle but does not authorize edits to help.
- `.hermes/handoffs/dspawn-current-architecture.md`, implementation, and QA own transient enemy append/delivery. `dspawn` has no H2 operation and is expressly excluded from this persistence vertical.
- `dfight` remains a later mixed state/persistence design: source removes an in-memory name then performs separate level/stat updates. Do not couple it to registration or use registration to infer enemy/character ownership.

## Module and file map

- **Saturn evidence:**
  - `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZRegisterCommandImpl.java`
  - `../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java`
  - `../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java`
  - `../../projects/saturn/src/main/java/org/saturn/app/util/DBZUtil.java`
  - `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`
  - `../../projects/saturn/src/main/resources/schema-h2.sql`
- **Zenbot production:**
  - `internal/command/registry.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/command/dbz.go`
  - `internal/listener/message/handlers.go`
  - `internal/service/dbz.go`, `internal/service/services.go`
  - `internal/repository/dbz.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/database.go`
  - `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql`
  - `internal/factory/engine_factory.go`
- **Zenbot test homes:**
  - `internal/command/dbz_test.go`
  - `internal/listener/message/dbz_dispatch_test.go`
  - `internal/repository/h2/dbz_test.go`
  - `internal/service/dbz_test.go`

## Recommended developer tier

**Senior Go maintainer with H2/JDBC semantics awareness and shared-worktree coordination.** The code delta is small, but the intentional duplicate/non-transactional behavior, partial-state/error contract, and exact public transport behavior make this unsuitable for unsupervised implementation.

## Baseline and limitations

[TEST-BACKED] This architecture pass ran successfully on the dirty checkout:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok   zenbot/internal/command
ok   zenbot/internal/listener/message
ok   zenbot/internal/service
ok   zenbot/internal/repository/h2

git diff --check && git diff --cached --check
exit 0; no output
```

[OBSERVED] The worktree is heavily dirty, including `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/command/registry.go`, `internal/factory/engine_factory.go`, `internal/repository/h2/dbz_test.go`, and `internal/service/dbz_test.go`; `internal/listener/message/dbz_dispatch_test.go` is untracked. This handoff modifies no application source, test, schema, staged content, or commit.

[LIMITATION] No Saturn DBZ command/service/H2 behavior test was located in the inspected Saturn source. Observable claims above are grounded in the production Java implementation and schema. Concurrent duplicate selection has intentionally unspecified row choice; it cannot be made deterministic without deviating from the traced source.
