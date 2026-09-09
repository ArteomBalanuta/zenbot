# DBZ command/persistence parity: smallest safe vertical

## Decision

[RECOMMENDED] Select **`dbzstats` (`dbzstats` / `dstats` / `dstat` / `ds`) as the first safe DBZ command/persistence vertical**. It is a regular-user, read-only path from authenticated inbound chat through the existing command adapter and service bundle to one H2 join query, then one reply. It needs no new identity inference, no mutable enemy state, no random source, and no multi-statement write transaction.

Do **not** start with `dbzregister`, `dbzstr`, `dfight`, or `dspawn`:

- `dbzregister` is a two-statement/three-step write and Saturn explicitly has no existence check; matching its duplicate-name and non-atomic `SELECT id WHERE name=?` behavior creates a concurrent attribution risk.
- `dbzstr` has a check-then-update race and source semantics permit spending more than the available free stats; the current Go strength path preserves that quirk.
- `dfight` mutates two independent state domains, removes any matching in-memory enemy without proving a kill, then levels up regardless; it has no source randomness despite the game wording.
- `dspawn` is memory-only, not a persistence vertical, and its process-local enemy list is lost on restart.

The selected slice is already present in the dirty checkout. This handoff is documentation only; it recommends the first independent RED test needed to turn the existing slice into a stronger parity boundary rather than claiming an implementation task remains unstarted.

## Evidence map

### Saturn source of truth

| Concern | Source evidence |
|---|---|
| `dbzstats` aliases and role | `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZStatsCommandImpl.java` — `@CommandAliases({"dbzstats", "dstats", "dstat", "ds"})`, `getAuthorizedRole() -> Role.REGULAR`. |
| Stats command behavior | `DBZStatsCommandImpl.execute()` derives `author` from `chatMessage.getNick()`, calls `engine.dbzService.getStats(author)`, enqueues the returned text, and returns success. |
| Source DBZ service | `../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java`; `DBZImpl.getStats(String)` in `service/impl/DBZImpl.java`. |
| Source state/query | `DBZImpl.getStats` prepares `DBZUtil.SELECT_STATS`, formats seven exact lines when a row exists, otherwise `No stats found for character: <name>`. `DBZUtil.SELECT_STATS` joins `dbz_stats` to `dbz_characters` on `char_id` and filters `c.name = ?`. |
| Source schema | `../../projects/saturn/src/main/resources/schema-h2.sql`: nullable duplicate `dbz_characters.name`/`level`; `dbz_stats.char_id NOT NULL` with an FK; no uniqueness constraint or DBZ indexes. |
| Saturn tests | Search of `../../projects/saturn/src/test/java` found no DBZ command/service/H2 behavioral test. The only DBZ-related tests concern agent tool catalog/routing, not command behavior. This is a source-test limitation, not evidence of output semantics. |

### Zenbot current path

| Layer | Observed implementation |
|---|---|
| Catalog | `internal/command/registry.go`: explicit regular definitions for all six DBZ canonicals and source aliases. `newCommand` maps those canonicals to `*dbzCommand`. |
| Conditional registration | `internal/command/dispatch_adapter.go: RegisterUserUtilitiesWithDirectAgent` registers the DBZ family only if `bundle(e).DBZ != nil`. |
| Authorization/dispatch | `internal/listener/message/handlers.go: DispatchUserCommand.Handle` resolves the registered command and authorizes `c.Author` through `Engine.IsUserAuthorized` before executing it. `internal/core/engine_impl.go: IsUserAuthorized` fails closed when `SecurityService` is nil. No DBZ-specific identity lookup is made. |
| Parsing and reply transport | `internal/command/handlers.go: args` drops the command token; `internal/command/dbz.go` uses only `args[0]` for `dbzstr`, `dfight`, and `dspawn`. `reply` calls `SendChatMessage(message.Name, text, message.IsWhisper || message.Whisper || message.Type == "whisper")`, so Zenbot preserves inbound whisper state. Saturn DBZ commands enqueue output rather than explicitly preserving whispers; this is a transport adaptation, not source-proven equality. |
| Service bundle | `internal/factory/engine_factory.go` assigns `&service.DBZService{Repo: dbz}` only when the repository implements `repository.DBZRepository`; `internal/service/services.go` holds it in `Bundle.DBZ`. |
| Read service | `internal/service/dbz.go: StatsText` calls `Repo.Stats`; it returns the seven-line source-shaped string or the source-shaped missing-character message. |
| H2 repository | `internal/repository/h2/dbz.go: Stats` uses a bound `$1` name parameter, joins `dbz_stats` and `dbz_characters`, maps `sql.ErrNoRows` to `(zero, false, nil)`. |
| Schema/bootstrap | `internal/repository/h2/schema-h2.sql` contains the two DBZ tables and foreign key. `internal/repository/h2/database.go: bootstrap` executes the embedded schema in a transaction before returning a database. `resources/schema-h2.sql` mirrors the DBZ table definitions. |
| Existing tests | `internal/command/dbz_test.go` checks aliases/role/concrete construction and malformed strength usage. `internal/service/dbz_test.go` locks the existing stats text. `internal/repository/h2/dbz_test.go` exercises real-H2 registration/mutations and missing reads. None currently traces authorized inbound `!dbzstats` through dispatch to a real H2-backed reply. |

## Observed state model and command map

[OBSERVED] Durable state is only the character/stat pair:

```text
 dbz_characters                         dbz_stats
 ┌─────────────────────────────┐        ┌──────────────────────────────────┐
 │ id (identity PK)            │<───────│ char_id (NOT NULL FK)            │
 │ name VARCHAR (nullable, non-unique) │ │ free_stats, str, agi, vit, ene  │
 │ level INTEGER (nullable)    │        │ created_on                       │
 │ created_on (NOT NULL)       │        └──────────────────────────────────┘
 └─────────────────────────────┘
```

[OBSERVED] The transient state is `DBZService.enemies []string`, guarded by `sync.Mutex` in Zenbot (`internal/service/dbz.go`). It is not stored in H2 and has no restart recovery or table prerequisite.

| Command | Source intent / current Zenbot behavior | State and safety classification |
|---|---|---|
| `dbzregister` | Inserts a level-1 character and level-1 stats, then always replies success. | Durable write; unsafe first slice due to non-atomic duplicate-name ID resolution. |
| `dbzstats` | Reads caller-name stats and replies exact formatted snapshot or missing text. | Durable read; **selected**. |
| `dbzstr` | Requires positive integer; checks free stats; writes strength and decrements free stats only. | Durable write; source check/update race and overspend quirk. |
| `dfight` | Requires an enemy token; removes matching transient enemy then invokes level-up and +5 free stats unconditionally. | Mixed transient/durable write; no deterministic combat calculation, no randomness, no proof enemy existed. |
| `dbzhelp` | Emits fixed help text. | No persistence; source escapes Java text including thin space, Zenbot currently uses literal `\\n` text. Not the requested persistence vertical. |
| `dspawn` | Requires enemy token, appends it to transient list, replies. | In-memory-only write; not a persistence vertical. |

## Selected vertical sequence

[OBSERVED] Current execution sequence (authorization is shared infrastructure, not a DBZ policy change):

```text
chat event
  -> message.ResolveUserMetadata (finds active user by name)
  -> message.DispatchUserCommand
      -> common.BuildCommand(alias, Engine, ChatMessage)
      -> Engine.IsUserAuthorized(author, REGULAR)
      -> legacyAdapter.ExecuteContext
      -> dbzCommand.Execute(canonical=dbzstats)
      -> Bundle.DBZ.StatsText(ctx, message.Name)
      -> DBZRepository.Stats(ctx, name)
      -> H2 SELECT join ($1 bound)
      -> reply(..., inbound-whisper flag)
```

[TEST-BACKED] `go test ./internal/command ./internal/service ./internal/repository/h2` passed on the inspected dirty tree. This proves the existing focused command/service/H2 package suites pass; it does not prove the above end-to-end dispatch sequence because no such DBZ tracer is present.

## Exact first safe RED tracer

[RECOMMENDED] Add one focused test in `internal/listener/message` (for example, `dbz_dispatch_test.go`) named:

`TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply`

Arrange only real production components where possible:

1. Start the existing isolated H2 fixture, seed `dbz_characters`/`dbz_stats` for `goku` with distinct values, and construct the normal `service.DBZService`/`Bundle` over the real `*h2.Database`.
2. Build an engine test double with the real command registry registration path, a resolved active `model.User{Name: "goku"}`, an authorization implementation that permits `REGULAR`, and a recording `SendChatMessage`.
3. Run `DispatchUserCommand.Handle(context.Background(), Context{Message: &model.ChatMessage{Name: "goku", Text: "!ds", IsWhisper: true}, Author: activeGoku, Engine: engine})`.
4. Assert one reply addressed to `goku`, whispered, with exactly:

```text
character: goku
level: 2
free stats: 5
str: 3
agi: 4
vit: 5
ene: 6
```

Before production edits, this test should be made to fail only if the existing behavior is absent from the branch being implemented (for example, by placing it before registration/composition is enabled). In this checkout it is expected to pass immediately because the slice is already implemented; do **not** call that a RED result. Instead, create a deliberately unimplemented acceptance detail first (such as real H2 dispatch composition in the test harness), observe that harness/behavior failure, then make the minimal test-only composition addition. No production-code change is warranted unless the new tracer exposes a defect.

## Strict sequential TDD map

1. **RED 1 — selected tracer:** write the authorized `!ds` real-H2 dispatch test above; run only it. Require one exact reply and whisper preservation.
2. **GREEN 1:** only if it fails due to DBZ production behavior, minimally repair the layer revealed by the failure; rerun the one test. If it fails only because the test lacks an engine/factory seam, add the minimum test composition, not a new command path.
3. **REFACTOR 1:** remove only test duplication; rerun `go test ./internal/listener/message ./internal/command ./internal/service ./internal/repository/h2`.
4. **RED 2 — missing state:** add one same-path `!dbzstats` test for a name with no H2 rows; expect exact `No stats found for character: <author>`, then GREEN/refactor.
5. **RED 3 — authorization boundary:** add one denied-regular-user dispatch test asserting no DBZ repository call and the existing shared unauthorized reply behavior; do not alter `ResolveUserMetadata` or role inference.
6. **Only after 1–5:** consider `dbzregister` as a separately reviewed persistence design. Its first RED must establish the intended duplicate-name and failure/transaction semantics before any implementation, rather than silently making it atomic and calling that Saturn parity.

## Dependency and priority decomposition

| Priority | Slice | Dependencies | Exit criterion |
|---|---|---|---|
| P0 | `dbzstats` read tracer | Existing registry, security service, `Bundle.DBZ`, H2 schema | Real-H2 authorized dispatch and missing-row tests pass. |
| P1 | Registration policy decision | P0; explicit decision whether exact Saturn duplicate/non-atomic behavior is acceptable | Written semantic decision plus transactional/error tests before code. |
| P2 | Strength spending | Registered character semantics; an explicit concurrency/overspend policy | Atomic update or explicitly isolated parity quirk, tested under concurrent callers. |
| P3 | Spawn/fight | P2; explicit lifecycle/persistence decision for enemies | Enemy existence/consumption semantics and level-up coupling are specified and race-tested. |
| P4 | Help transport text | Output escaping/newline compatibility decision | Exact public/whisper payload expectation covered separately. |

## Risks, blockers, and no-scope

### Policy/security blockers

- [RECOMMENDED] Preserve the current shared authorization gate and `REGULAR` role. `ResolveUserMetadata` must remain the authoritative active-user binding. Do not infer identity from a nick, argument, trip, or database DBZ row; DBZ character names are non-unique and not an identity authority.
- [OBSERVED] A missing `SecurityService` fails closed in `EngineImpl.IsUserAuthorized`; a missing DBZ service keeps DBZ commands out of the registration adapter. Retain both composition gates.
- [LIMITATION] Saturn swallows SQL errors in DBZ service methods and commands often still report success. Zenbot surfaces errors as command failure/log output. Do not erase Zenbot error propagation under the label of parity without an explicit security/operational decision.

### Concurrency/transaction blockers

- [OBSERVED] Saturn register performs character insert, name-based ID select, and stat insert without a transaction. Zenbot deliberately mirrors this (`internal/repository/h2/dbz.go` comment). With duplicate names, concurrent registration can attach stats to a different matching character.
- [OBSERVED] Saturn and Zenbot level-up each issue two independent updates without a transaction. `dbzstr` reads free stats before issuing a separate update; the amount is not bounded by available stats and only strength decrements free stats.
- [OBSERVED] Zenbot mutex-protects its transient enemies list, unlike Saturn's unguarded `ArrayList`; it makes list operations race-safe but does not make fight plus level-up atomic.
- [OBSERVED] There is no combat RNG, enemy stats, or deterministic damage model in the traced Saturn DBZ classes. Do not invent one for parity.

### Schema prerequisites

- [OBSERVED] Both Zenbot schema copies define DBZ tables matching Saturn's basic shape and the embedded `internal/repository/h2/schema-h2.sql` is what `h2.bootstrap` executes. Any future column/index/migration change must update the embedded schema and its mirrored resource copy deliberately.
- [LIMITATION] No DBZ-name index/unique constraint exists in either traced schema. Adding either changes duplicate/selection behavior and is not part of the selected read vertical.

### Explicit no-scope

No whiskey/proxy, mine, restart/shutdown, raw admin SQL, semantic moderation, role-policy broadening, identity-inference change, enemy persistence, combat design, RNG, schema migration, code/test edits, cleanup, staging, or commit is included in this handoff.

## File map

- Saturn command family: `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/{DBZRegisterCommandImpl,DBZStatsCommandImpl,DBZAddStrCommandImpl,DBZFightCommandImpl,DBZHelpCommandImpl,DBZSpawnEnemyCommandImpl}.java`
- Saturn service/query/schema: `../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java`, `../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java`, `../../projects/saturn/src/main/java/org/saturn/app/util/DBZUtil.java`, `../../projects/saturn/src/main/resources/schema-h2.sql`
- Zenbot registration/dispatch: `internal/command/registry.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/listener/message/handlers.go`
- Zenbot DBZ/service/repository/schema: `internal/command/dbz.go`, `internal/service/dbz.go`, `internal/repository/dbz.go`, `internal/repository/h2/dbz.go`, `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql`, `internal/repository/h2/database.go`
- Zenbot tests: `internal/command/dbz_test.go`, `internal/service/dbz_test.go`, `internal/repository/h2/dbz_test.go`, `internal/listener/message/dispatch_authorization_test.go`
