# DBZ Migration Slice — Architecture Specification

**Scope:** Saturn DBZ service, DBZ commands, DBZ persistence, and dispatch integration only. Architecture phase; no application code was changed. Source is read-only Saturn `develop`; target is Zenbot `master` with intentional dirty/untracked work preserved.

## 1. Evidence and inspected paths

### Saturn source (observed)

- `src/main/java/org/saturn/app/service/DBZService.java` — service contract.
- `src/main/java/org/saturn/app/service/impl/DBZImpl.java` — JDBC implementation and in-memory enemy list.
- `src/main/java/org/saturn/app/command/impl/dbz/DBZAddStrCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/dbz/DBZFightCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/dbz/DBZHelpCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/dbz/DBZRegisterCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/dbz/DBZSpawnEnemyCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/dbz/DBZStatsCommandImpl.java`
- `src/main/java/org/saturn/app/util/DBZUtil.java` — all DBZ SQL constants.
- `src/main/java/org/saturn/app/command/UserCommandBaseImpl.java` — command parsing, usage failure, role/authorized-trip plumbing.
- `src/main/java/org/saturn/app/command/UserCommand.java` — command contract (located while tracing).
- `src/main/java/org/saturn/app/command/factory/CommandFactory.java` — ClassGraph discovery, alias/anagram matching and duplicate validation.
- `src/main/java/org/saturn/app/facade/Base.java` — `DBZImpl` construction and service wiring.
- `src/main/java/org/saturn/app/model/Role.java`, `Status.java` — source authorization/status enums.
- `src/main/java/org/saturn/app/util/Util.java` — `getAdminAndUserTrips`, lower-casing and anagram behavior.
- `src/main/resources/schema-h2.sql` — source DBZ tables.
- `src/test/java/org/saturn/app/agent/routing/AgentRequestAssemblerTest.java` and `src/test/java/org/saturn/app/agent/tool/SaturnCommandToolCatalogTest.java` — only DBZ-related test references found; no focused DBZ command/service tests were found.
- `AGENTS.md` — source command and H2 testing guidance.

### Zenbot target (observed)

- `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql` — currently identical canonical schemas; DBZ tables exist.
- `internal/repository/h2/database.go` — real H2 PostgreSQL-wire open/bootstrap, `SELECT H2VERSION()`, embedded schema.
- `internal/repository/h2/identity.go` — representative transaction/repository style.
- `internal/repository/repository.go` — current repository interfaces.
- `internal/common/command_registry.go` — context-aware catalog contract and alias/anagram validation.
- `internal/common/command.go` — legacy inbound command contract.
- `internal/command/registry.go` — 64-entry reviewed catalog; DBZ entries currently generic placeholders.
- `internal/command/dispatch_adapter.go` — legacy adapter and `RegisterUserUtilities`; DBZ is intentionally not runtime-registered.
- `internal/command/handlers.go`, `internal/command/handlers_test.go` — existing command implementations/stubs and test engine.
- `internal/listener/user_chat_listener.go`, `internal/listener/message/handlers.go` — inbound parsing, chain ordering, authorization, command dispatch.
- `internal/core/engine_impl.go`, `internal/common/engine.go` — output, active-user, service-bundle and registration seams.
- `internal/service/services.go`, `internal/service/security_service.go` — service bundle, output interface and authorization semantics.
- `internal/model/role.go`, `status.go`, `chat_message.go`, `user.go`, `records.go`, `db.go` — target types.
- `internal/command/catalog_test.go`, `dispatch_integration_test.go`, `handlers_test.go`, `internal/listener/message/dispatch_authorization_test.go` — catalog/dispatch patterns and current negative DBZ coverage.
- `MIGRATION_PLAN.md`, `.hermes/migration-audit.md` — frozen migration scope and DBZ rows/SQL ledger.

## 2. Source behavior contract

### Service interface and state

`DBZService` exposes exactly:

```text
void register(String name)
void lvlUp(String name)
int addStr(String name, int str)
int addAgi(String name, int agi)
int addVit(String name, int vit)
int addEne(String name, int ene)
String getStats(String name)
int getFreeStats(String name)
void fight(String name)
void spawnEnemy(String name)
```

`DBZImpl(Connection, BlockingQueue<String>)` extends `OutService`, stores the JDBC `Connection`, and owns a process-local `List<String> enemies`. The queue is not used by DBZ service methods. Enemy state is not persisted, has no random generation, and has no validation: spawn appends; fight removes the first matching name if present and otherwise does nothing.

### Registration and level-up

- `register(name)` inserts `dbz_characters(name, level=1, created_on=DateUtil.getTimestampNow())`, selects the character id by exact `name`, then inserts `dbz_stats(char_id, free_stats=0, str=1, agi=1, vit=1, ene=1, created_on=DateUtil.getTimestampNow())`.
- It prints `char id registered: <id>` to stdout on successful id lookup and logs success. SQL errors are caught, logged, and swallowed; the method has no error return.
- The source explicitly has `// TODO: check if exists.` There is no duplicate precheck, no explicit transaction, and the two inserts are not atomic. A duplicate name can therefore create a second character (unless an external schema constraint prevents it); a failure after character insertion can leave an orphan character.
- `lvlUp(name)` executes `UPDATE dbz_characters SET level=level+1 WHERE name=?`, then `UPDATE dbz_stats SET free_stats=free_stats+5 WHERE char_id=(SELECT id ... WHERE name=?)`. Both update counts are ignored. SQL exceptions are logged/swallowed.

### Stat mutation quirks (must preserve unless an explicit parity decision changes them)

- `addStr(name, str)` binds `str` twice and runs `str=str+?` and `free_stats=free_stats-?`; it returns `-1` unconditionally, including after success/failure.
- `addAgi`, `addVit`, `addEne` each increment only their attribute; despite command/service parameters and logs saying “increased by 1”, the bound value is the caller-provided integer. They do **not** decrement `free_stats`. Each returns `-1` unconditionally.
- Commands guard only `addStr` with `getFreeStats() > 0`; they do not cap the requested amount against available points and do not inspect mutation return values.

### Reads and formatting

`getStats(name)` performs the joined `dbz_stats`/`dbz_characters` query and renders exactly:

```text
character: <name>\n
level: <level>\n
free stats: <free_stats>\n
str: <str>\n
agi: <agi>\n
vit: <vit>\n
ene: <ene>\n
```

For no row it returns `No stats found for character: <name>`. SQL failure returns whatever has been accumulated (normally empty string), after logging. `getFreeStats(name)` returns the first `free_stats`, or `-1` for no row/error. The source returns from inside the result-set branch before closing `ResultSet`/statement; this resource leak is observed and should not be silently generalized into a different externally visible error contract, though the Go implementation must close rows safely.

### Commands: aliases, role, arguments, outputs, transitions

All six DBZ command constructors call `super(message, engine, getAdminAndUserTrips(engine))`, then set discovered aliases. All return `Role.REGULAR` and `Status.SUCCESSFUL`/`FAILED` wrapped in `Optional`.

| Command | Aliases | Parsing and behavior | Exact output |
|---|---|---|---|
| `DBZRegisterCommandImpl` | `dbzregister`, `dreg`, `dr` | Uses author nick; no arguments. Calls `register(author)` then reports success regardless of swallowed service failure. | `Successfully registered character: <author>` |
| `DBZStatsCommandImpl` | `dbzstats`, `dstats`, `dstat`, `ds` | Uses author nick; calls `getStats(author)`. | service result verbatim |
| `DBZAddStrCommandImpl` | `dbzstr`, `dstr`, `daddstr` | Requires argument 0; trims it, parses integer, requires `> 0`; invalid/missing emits `Example: <prefix>daddstr amount` and returns FAILED. If free stats `<=0`, emits `You don't have free stats. Level up!` and returns SUCCESSFUL. Otherwise calls `addStr(author,value)` and emits the decimal value only. | as shown; successful allocation output is `<value>` |
| `DBZFightCommandImpl` | `dfight`, `df` | Requires trimmed argument 0 (`dfight enemy` usage on failure). Calls `fight(enemy)` then `lvlUp(author)`; no check that enemy existed or author is registered. | `Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!` |
| `DBZHelpCommandImpl` | `dbzhelp`, `dbz`, `dhelp` | Ignores trip and arguments. Uses `StringEscapeUtils.escapeJava` before output. | Multi-line DBZ help: `This is a DBZ universe text based game.`; `/train`, `/fight <nick>`, `/claim`; thin-space `\u2009`; `/stats`; `/strength <int>`; `/agility <int>`; `/vitality <int>`; `/energy <int>`. Preserve escaped Java newline/separator behavior through target output normalization. |
| `DBZSpawnEnemyCommandImpl` | `dspawn` | Requires trimmed argument 0 (`dspawn enemy` usage on failure); appends enemy string. | `spawned enemy: <enemy>` |

The source command parser removes the configured prefix, trims the command text, identifies the first whitespace boundary, and splits the remainder on `\\s+`; required arguments are trimmed. Authorization checks command role and authorized trips before execute. `getAdminAndUserTrips` is the comma-split concatenation of configured admin and user trips, without additional trimming. Factory discovery sorts command classes by class name and matches aliases by anagram, not simple exact equality; the target catalog intentionally uses explicit canonical/alias registration and rejects exact/anagram collisions.

## 3. Target mapping and required interfaces

### Recommended target shape

Add a DBZ service to `internal/service`, backed by a narrow repository contract in `internal/repository` and H2 methods in `internal/repository/h2`. Do not embed SQL in command handlers. Preserve context-aware Go errors internally, while the compatibility command boundary decides which errors become source-equivalent output/status.

Proposed contracts (signatures are recommendations for implementation, not existing APIs):

```go
type DBZRepository interface {
    Register(ctx context.Context, name string, now func() int64) (int64, error)
    LevelUp(ctx context.Context, name string) error
    AddStrength(ctx context.Context, name string, amount int) error
    AddAgility(ctx context.Context, name string, amount int) error
    AddVitality(ctx context.Context, name string, amount int) error
    AddEnergy(ctx context.Context, name string, amount int) error
    Stats(ctx context.Context, name string) (DBZStats, bool, error)
    FreeStats(ctx context.Context, name string) (int, bool, error)
}
type DBZService struct { Repo DBZRepository; Enemies ... }
```

The service should expose methods equivalent to the source (`Register`, `LevelUp`, `AddStrength`, `AddAgility`, `AddVitality`, `AddEnergy`, `StatsText`, `FreeStats`, `Fight`, `SpawnEnemy`) and an injectable clock for deterministic tests. `Fight`/`SpawnEnemy` should be concurrency-safe because the source list is mutable shared state; no persistence should be invented for enemies.

Use `model.REGULAR` for all DBZ definitions, not the current placeholder `model.USER`: source explicitly returns `Role.REGULAR`; target role constants use iota and therefore are not numerically source-compatible. Verify authorization through the existing `SecurityService`/`DispatchUserCommand` path rather than bypassing it.

Implement concrete `SaturnCommand` handlers in `internal/command` (one file or a clearly bounded DBZ file) with `Execute(context.Context) (model.Status, error)`, `Role() model.Role`, `Aliases() []string`, and `NewInstance(common.Engine,*model.ChatMessage) common.SaturnCommand`. Register those constructors in `commandDefinitionFor`/`newCommand` and expose them from `RegisterUserUtilities` only when the required DBZ service is present. Do not leave the generic `saturnCommand` placeholder reachable for DBZ.

Because the existing `common.Engine` has no DBZ method, obtain it via a small optional interface or extend the service accessor seam (`ServiceBundle`); prefer adding `DBZ *DBZService` to `service.Bundle` and retrieving it using the existing `serviceBundle` pattern. Output through `common.Engine.SendChatMessage`/the target output seam so whisper routing and JSON escaping remain centralized.

## 4. File-level change map (implementation handoff)

**Create or extend only these task-owned areas:**

- `internal/repository/repository.go`: add the DBZ repository interface and DTO contract, if not placed in a dedicated repository interface file.
- `internal/repository/h2/dbz.go`: named prepared H2 methods for the nine DBZ SQL operations; close rows; return update counts/errors without ad hoc SQL in services.
- `internal/service/services.go` or `internal/service/dbz.go`: DBZ service, exact source formatting, error compatibility, clock injection, synchronized enemy list.
- `internal/service/dbz_test.go`: service behavior tests.
- `internal/command/dbz.go`: six concrete command handlers (the source has six command classes; `DBZService` is not a command). Preserve aliases, role and outputs.
- `internal/command/registry.go`: map six canonicals to concrete constructors and change their catalog role to `model.REGULAR`; retain all aliases exactly.
- `internal/command/dispatch_adapter.go`: register DBZ only when DBZ service is available; do not expose it in DBZ-less test engines.
- `internal/command/dbz_test.go`: metadata, parsing, output and state-transition tests.
- `internal/command/dispatch_integration_test.go` or a dedicated DBZ dispatch test: every alias through `listener.NewUserChatListener`.
- `internal/repository/h2/dbz_test.go`: real H2 persistence and rollback/ordering tests.
- `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql`: only if implementation reveals a required schema correction; currently DBZ tables are present and identical. Do not add unrelated schema objects.

Do not modify Saturn, identity slice files, or unrelated dirty paths.

## 5. Persistence and transaction semantics

The frozen audit maps DBZ SQL occurrences 61–80 to `DBZUtil.java` (the audit excerpt explicitly records INSERT character/stats, level/free-stats updates and subsequent stat/read queries). Named target methods must cover:

1. insert character;
2. select character id by exact name;
3. insert stats;
4. update level;
5. add five free stats;
6. add strength and decrement free stats;
7. add agility;
8. add vitality;
9. add energy;
10. joined stats rendering query;
11. free-stats query.

Use H2 PostgreSQL-wire placeholders (`$1` style consistent with target) or driver-compatible prepared statements. The source registration is non-transactional; strict parity would preserve that boundary, but atomic registration is a material security/data-integrity decision. The implementation handoff must explicitly choose and test either (a) source-compatible two-step behavior, or (b) a documented target adaptation using one transaction. No silent choice. In either case, duplicate behavior and update counts must be tested. Level-up is two separate source statements and can partially apply on failure; likewise stat mutation has no source-side free-point check in SQL. Preserve these quirks unless the migration owner approves a parity deviation.

Schema observations: `dbz_characters(id identity primary key, name VARCHAR, level INTEGER, created_on BIGINT NOT NULL)` and `dbz_stats(id identity primary key, char_id BIGINT NOT NULL, str/agi/vit/ene/free_stats INTEGER, created_on BIGINT NOT NULL, FK char_id)`. There is no uniqueness constraint on character name and no DBZ-specific index. Real-H2 metadata tests must assert these facts before changing schema.

## 6. Focused real-H2 test plan

Use the pinned H2 runtime through `h2.Open`; do not substitute SQLite or skip when H2 prerequisites are missing. Tests should:

1. Open a temporary H2 database and assert `SELECT H2VERSION()` succeeds.
2. Assert DBZ table columns, identity primary keys, nullability, types, and FK through H2 metadata.
3. Register a character and assert level 1, free stats 0, all four base stats 1, and two timestamps are populated.
4. Register duplicate names and capture actual source-compatible behavior (including whether both rows are allowed).
5. Level up and assert `level+1` and `free_stats+5`; test missing names and update counts.
6. Assert strength increments and decrements free stats; assert agility/vitality/energy increment without decrementing free stats (the intentional source quirk).
7. Assert missing-character reads return `No stats found...` and free stats `-1`; assert SQL/connection errors map to service errors without leaked rows.
8. Test registration failure between inserts and verify the selected transaction policy (partial visibility vs rollback).
9. Test concurrent `SpawnEnemy`/`Fight` access under `go test -race`; assert removing an absent enemy is a no-op and duplicate enemy entries remove one occurrence.
10. Test exact stats rendering, final newline, no-row message, and escaped help separators.
11. Test each canonical and every alias via the listener, including required/malformed/negative/non-integer arguments, whisper output, role rejection and authorized regular user dispatch.
12. Assert DBZ aliases are absent when the bundle has no DBZ service and no generic placeholder output occurs; assert catalog still has 64 definitions and no alias/anagram collision.

## 7. End-to-end dispatch sequence (target design)

```text
JSON chat -> UserChatListener.Notify
          -> ResolveUserMetadata (author/hash)
          -> AuditChatMessage / standard chain
          -> DispatchUserCommand
          -> BuildCommand(alias, explicit registry)
          -> SecurityService.IsAuthorized(author, REGULAR)
          -> concrete DBZ command.Execute(ctx)
          -> service.DBZ + H2 named repository methods
          -> Engine.SendChatMessage (central JSON/whisper normalization)
```

The listener currently invokes `cmd.Execute()` through the legacy `common.Command` seam; the adapter supplies `context.Background()` and logs returned errors. DBZ integration must preserve this existing dispatch behavior while ensuring the concrete handler, not `saturnCommand`, is selected.

## 8. Risks, limitations, and unresolved decisions

- **Known source defects:** no duplicate check; non-atomic registration; swallowed SQL errors; resource leak in `getFreeStats`; unconditional `-1` mutation returns; only strength consumes free stats; fight levels up regardless of enemy existence/registration; enemies are process-local.
- **Role mapping risk:** current target catalog labels DBZ as `model.USER`, while source classes require `Role.REGULAR`. This is a concrete parity defect, not a cosmetic label.
- **Alias semantics:** Saturn factory supports anagram aliases; target registry also validates anagrams but lookup is exact normalized alias. This is acceptable only if the migration contract intentionally uses target explicit dispatch; tests must document that anagram input is not accidentally exposed or must add compatibility deliberately.
- **Output risk:** source help uses Java escaping and literal `\\n` separators, while target output normalization converts escaped separators in centralized send paths. Golden tests must compare the actual queued/socket payload, not only an intermediate string.
- **SQL name matching:** source DBZ SQL uses exact case-sensitive `name=?`; do not import target identity's case-insensitive policy into DBZ without approval.
- **Transaction choice:** target repository idiom favors `WithTx`, but source DBZ does not define a transaction. This must be an explicit reviewed adaptation.
- **No source focused tests:** the source checkout contains no DBZ-specific service/command tests; behavior evidence is source inspection plus the migration audit, so target tests must be authored from the observed contract.

## 9. Complexity rating

**Medium-high (7/10).** The feature itself is small (six commands, one service and two tables), but strict parity requires preserving several counterintuitive defects, integrating two target command contracts, correcting role metadata, adding named H2 repository methods, deciding transaction/error adaptation, and proving real-H2 plus listener dispatch behavior without exposing placeholders or disturbing the identity slice.

## 10. Architecture completion checklist

- [ ] All six DBZ command classes have concrete target handlers and exact aliases.
- [ ] All DBZ handlers require `model.REGULAR`, not `model.USER`.
- [ ] DBZ is runtime-registered only with a DBZ-capable service bundle.
- [ ] Eleven DBZ SQL operations are named repository methods and tested on real H2.
- [ ] Source quirks and any approved deviations are recorded in tests.
- [ ] Stats/help output golden tests compare actual outbound messages.
- [ ] `go test -race`, `go vet`, `go build` and focused H2 tests pass after implementation.
- [ ] No unrelated or identity-slice files are changed.
