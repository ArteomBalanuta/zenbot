# `dspawn` source-exact parity: bounded transient-state vertical

## Decision

[RECOMMENDED] **Proceed with `dspawn` only as a small, source-exact transient-state vertical**, after the DBZ/help dirty hunks are coordinated. It is not a persistence vertical: Saturn appends one string to an engine-local list and emits one public acknowledgement. It neither reads nor writes DBZ tables, character state, combat state, or RNG.

It is safe *only* with these limits:

- retain the existing `*service.DBZService` composition/registration prerequisite; do not expose `dspawn` without a DBZ service merely because this method itself does not use `Repo`;
- repair the two current command-visible mismatches: missing-argument usage text, and success delivery (source is unaddressed/public, not a reply/whisper);
- preserve append-only duplicates and process-local lifetime; do not add an enemy table, uniqueness, expiry, cross-engine sharing, or a combat rule;
- do not change `dfight`, although its existing `Fight` method consumes the first matching list entry and makes duplicate spawn state observable later.

This is a **current-green/recovery** plan, not proof that `dspawn` was developed test-first. The checkout already has a `dspawn` branch in `internal/command/dbz.go` and service methods. A new test that is green immediately is not a valid RED result. The implementation owner must use the named, temporary, DBZ-local isolation only where the plan says so, observe the behavioral RED, then restore the minimum behavior immediately.

## Source contract

### Command metadata and authorization

[OBSERVED] Saturn `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZSpawnEnemyCommandImpl.java`:

- declares exactly one alias, `dspawn` (line 16), and `Role.REGULAR` (lines 23–26);
- obtains `author = chatMessage.getNick()` only for logging (line 30 and 38); the author is not part of spawn state;
- calls `requiredArgument(0, "dspawn enemy")` (line 31);
- returns `Status.FAILED` on an absent/blank first argument (line 32), otherwise invokes `engine.dbzService.spawnEnemy(enemyName)` (line 35), queues output, and returns `Status.SUCCESSFUL` (lines 36–40).

[OBSERVED] `UserCommandBaseImpl.requiredArgument` at `../../projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:154-164` rejects absent, null, or trimmed-blank input; it calls `failWithUsage`. `failWithUsage` at lines 195–202 sends exactly `Example: <engine-prefix>dspawn enemy` to the author, preserving `chatMessage.isWhisper()` through `replyToAuthor` (lines 191–202). Therefore the normal missing-argument contract is:

```text
status: FAILED
payload: Example: <configured-prefix>dspawn enemy
recipient: invoking author
whisper: inbound whisper flag
mutation: none
```

[OBSERVED] Saturn parses by stripping the prefix then `trim()`ing and splitting remaining arguments by `\\s+` (`UserCommandBaseImpl.java:51-71`). `requiredArgument` trims the selected first token. Thus leading/trailing whitespace and extra argument tokens do not form an enemy name: the first nonblank parsed token is used and all later tokens are ignored.

[OBSERVED] Successful Saturn output calls the one-argument `OutService.enqueueMessageForSending("spawned enemy: " + enemyName)` (`DBZSpawnEnemyCommandImpl.java:35-40`). The overload at `../../projects/saturn/src/main/java/org/saturn/app/service/impl/OutService.java:35-43` is unaddressed and has no whisper parameter. It queues exactly:

```text
spawned enemy: <first-token>
```

with `SUCCESSFUL`, public/unaddressed delivery, even if the input command arrived as a whisper. This is deliberately different from the missing-argument path.

### State, duplicates, lifecycle, and concurrency

[OBSERVED] `DBZService` declares `void spawnEnemy(String name)` (`../../projects/saturn/src/main/java/org/saturn/app/service/DBZService.java:20-22`). `DBZImpl` owns `private final List<String> enemies = new ArrayList<>()` and implements `spawnEnemy` as only `this.enemies.add(name)` (`../../projects/saturn/src/main/java/org/saturn/app/service/impl/DBZImpl.java:16-24, 231-238`). There is no database method, SQL, validation, de-duplication, normalization, max size, expiry, or output from the service.

[OBSERVED] Saturn creates one `DBZImpl(connection, outgoingMessageQueue)` as part of every `Base` construction (`../../projects/saturn/src/main/java/org/saturn/app/facade/Base.java:77-91`). Enemy state therefore begins empty for a new engine/DBZ-service instance and disappears when that instance is discarded/restarted. A live instance retains each append in insertion order. Repeated `dspawn frieza` calls create repeated entries.

[OBSERVED] Saturn `DBZImpl.fight(name)` later invokes `enemies.remove(name)` (`DBZImpl.java:230-233`); Java `List.remove(Object)` removes the first equal entry. This explains why spawn duplicates/order are meaningful state, but it is evidence for lifecycle only—not authorization to change `dfight` in this vertical.

[LIMITATION] Saturn uses an unsynchronized `ArrayList`; it has no defined safe concurrent-command behavior and no test located under `../../projects/saturn/src/test/java` for `dspawn`, `spawnEnemy`, or DBZ spawn behavior. Do not manufacture a concurrent ordering contract from that absence.

## Zenbot target map and current gaps

| Concern | Observed Zenbot location | Parity assessment / bounded action |
|---|---|---|
| Alias and role | `internal/command/registry.go`, `RegisterAll`, `def("dspawn", []string{"dspawn"}, model.REGULAR)` | Matches source exactly. No catalog change. |
| Concrete command construction | `internal/command/handlers.go`, `newCommand` maps `dspawn` to `*dbzCommand` | Matches required command type. No shared helper change. |
| Registration prerequisite | `internal/command/dispatch_adapter.go`, `RegisterUserUtilitiesWithDirectAgent`, DBZ conditional canonical list | `dspawn` registers only when `bundle(e).DBZ != nil`. Saturn always constructs a DBZ service with the engine, so preserve this fail-closed target composition gate. Do not move it into the unconditional no-DBZ `dbzhelp` list. |
| Dispatch/security | `internal/listener/message/handlers.go`, `ResolveUserMetadata` and `DispatchUserCommand.Handle` | Resolves active author then authorizes the catalog `REGULAR` role before executing. Preserve it; no nick/trip/DBZ-character inference. |
| Target parsing | `internal/model/chat_message.go:GetArguments` uses `strings.Fields`; `internal/command/handlers.go:args` drops the command token; `internal/command/dbz.go` takes and `strings.TrimSpace`s only `a[0]` | Matches ordinary whitespace/first-token/ignore-rest source behavior. Go and Java have non-identical Unicode-whitespace definitions; do not broaden parsing for exotic code points in this bounded slice without a source fixture. |
| Missing argument | `internal/command/dbz.go:81-85` currently sends `"dspawn enemy"` through `reply` then fails | **Mismatch.** Source payload is `Example: <prefix>dspawn enemy`. Keep `reply` for this branch so recipient and inbound-whisper behavior remain source-shaped. |
| Success delivery | `internal/command/dbz.go:86-88` currently `reply`s success | **Mismatch.** `reply` addresses the sender and preserves whisper (`internal/command/handlers.go:70-72`); source success is public/unaddressed. Use `SendChatMessage("", "spawned enemy: "+enemy, false)` and preserve successful normal status. |
| Transient store | `internal/service/dbz.go:11-16, 63-82` has `enemies []string`, `SpawnEnemy`, `Fight`, and a snapshot-copy `Enemies` seam | Sequential behavior matches source: append duplicates; `Fight` removes first equality match. `Enemies` and its copy are Go test observability, not a Saturn user API. |
| Concurrency difference | `internal/service/dbz.go:14, 63-81` uses `sync.Mutex` | Intentional target safety improvement over Saturn’s racy `ArrayList`. It protects individual spawn/fight/list operations but does **not** make a future `dfight` consume-plus-level-up workflow atomic. Do not claim concurrent ordering parity. |
| Factory/composition | `internal/factory/engine_factory.go:73-90` attaches DBZ only when the repository implements `repository.DBZRepository` | Existing application composition satisfies the registration prerequisite with real H2. No factory/repository/schema edit is necessary for spawn. |

### Current execution sequence

```text
inbound !dspawn <enemy> (public or whisper)
  -> ResolveUserMetadata: active user bound by name
  -> DispatchUserCommand: BuildCommand("dspawn")
  -> IsUserAuthorized(author, REGULAR)
  -> legacyAdapter.ExecuteContext
  -> dbzCommand.Execute
  -> DBZService.SpawnEnemy(first parsed token)
  -> source-required public/unaddressed acknowledgement
```

No repository call occurs after DBZ service composition. `repository.DBZRepository` (`internal/repository/dbz.go`) intentionally has no enemy method, and no H2 schema or SQL path belongs in this vertical.

## Exact behavior specification

| Input after a resolved/authorized `REGULAR` user | State mutation | Output | Status |
|---|---|---|---|
| `!dspawn` or whitespace-only argument text | none | addressed/whisper-preserving `Example: <prefix>dspawn enemy` | `FAILED` |
| `!dspawn   frieza` | append `"frieza"` | public/unaddressed `spawned enemy: frieza` | `SUCCESSFUL` |
| `!dspawn frieza ignored` | append only `"frieza"` | public/unaddressed `spawned enemy: frieza` | `SUCCESSFUL` |
| repeated successful `!dspawn frieza` | append one `"frieza"` per invocation, preserving duplicates | one public acknowledgement per invocation | `SUCCESSFUL` each |
| denied/missing active user | none | existing shared unauthorized behavior only where the shared dispatcher emits it | command not executed |
| a new DBZ-service/engine instance after restart | empty new enemy list | no restart recovery output | not an invocation |

[RECOMMENDED] A direct Go transport error is an adaptation boundary: Saturn’s queue overload may throw unchecked rather than return a command status. The normal success tracer must verify the ordinary no-error source contract. Do not add a new retry, queue abstraction, persistence fallback, or status policy while implementing `dspawn`.

## Strict sequential per-tracer RED → GREEN plan

### Preconditions for every unit

1. Work only in `/Users/ab/workspace/go-projects/zenbot`; record `git status --short`, `git diff --check`, `git diff --cached --check`, and the local hunks in `internal/command/dbz.go`, `internal/command/dispatch_adapter.go`, and DBZ tests.
2. The checkout is heavily dirty. `internal/command/dbz.go`, `dispatch_adapter.go`, `handlers.go`, `registry.go`, `service/dbz_test.go`, and the listener test area already have unrelated/in-progress changes. Coordinate ownership before a shared-file edit; never reset, checkout, restore, stash, stage, commit, bulk-format, or rewrite another hunk.
3. Write/run exactly one test, observe its required behavioral RED, make only its minimal GREEN, and rerun that one test before creating the next. A fixture/compile/H2 failure is not RED.
4. The temporary isolation below is only for prospective strict-TDD recovery on this already-implemented branch. Restore it immediately in the stated GREEN; do not leave a known broken command in the worktree.

### Tracer 1 — conditional registration with DBZ state

- **Test:** `TestRegisterUserUtilitiesRegistersDSpawnOnlyWhenDBZStateExists`
- **File:** `internal/command/dbz_test.go`
- **Arrange/assert:** register normal utilities on an engine with a nonnil `service.Bundle{DBZ: &service.DBZService{}}`; assert only alias `dspawn` resolves to canonical `dspawn`, role `REGULAR`, concrete legacy adapter. A second engine with no DBZ bundle must not register `dspawn`.
- **Pre-RED isolation:** manually remove only literal `"dspawn"` from the existing DBZ conditional canonical append in `internal/command/dispatch_adapter.go` after confirming the current hunk owner approves it.
- **RED command:** `go test -count=1 ./internal/command -run '^TestRegisterUserUtilitiesRegistersDSpawnOnlyWhenDBZStateExists$'`
- **Required RED:** DBZ-backed engine lacks `dspawn`; not a test setup failure.
- **Minimal GREEN:** restore only `"dspawn"` to that existing DBZ conditional list. Do not move it to the unconditional help list and do not alter catalog role/aliases, factory, security, or service.
- **GREEN command:** same command; require PASS.

### Tracer 2 — source-shaped missing argument

- **Test:** `TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate`
- **File:** `internal/command/dbz_test.go`
- **Arrange/assert:** construct normal `dspawn` with DBZ service state and `ChatMessage{Name:"goku", Text:"!dspawn   ", IsWhisper:true}`. Assert `FAILED`, no error, exactly one addressed whisper to `goku` with `Example: !dspawn enemy`, and empty `Enemies()`.
- **RED command:** `go test -count=1 ./internal/command -run '^TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate$'`
- **Required RED now:** payload is the current `dspawn enemy`, not source usage text.
- **Minimal GREEN:** change only the missing-argument `reply` payload in `internal/command/dbz.go` to `"Example: "+c.engine.GetPrefix()+"dspawn enemy"`. Keep `reply`, `FAILED`, and the no-spawn return path.
- **GREEN command:** same command; require PASS.

### Tracer 3 — first token, duplicate append, and public success output

- **Test:** `TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement`
- **File:** `internal/command/dbz_test.go`
- **Arrange/assert:** invoke the normal command twice on one DBZ service: first a whispered `!dspawn   frieza ignored`, then public `!dspawn frieza`. Assert two `Enemies()` entries in order `["frieza", "frieza"]`; assert each invocation returns `SUCCESSFUL`; assert its one acknowledgement is recipient `""`, whisper `false`, and exact `spawned enemy: frieza`. The first call proves extra-token ignoring and whisper-to-public delivery; the second proves duplicates.
- **RED command:** `go test -count=1 ./internal/command -run '^TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement$'`
- **Required RED now:** state append succeeds but current `reply` produces addressed output and preserves the first inbound whisper.
- **Minimal GREEN:** leave `SpawnEnemy(enemy)` intact; replace only the success `reply` with unaddressed/public `SendChatMessage("", "spawned enemy: "+enemy, false)`. Preserve the normal successful return and do not use `reply`, a repository call, a new output interface, or fight logic. Explicitly decide/error-test any Go transport-error handling only in a separately approved adapter follow-up.
- **GREEN command:** same command; require PASS.

### Tracer 4 — process-local lifecycle, not restart persistence

- **Test:** `TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder`
- **File:** `internal/service/dbz_test.go`
- **Arrange/assert:** call `SpawnEnemy("a")`, `SpawnEnemy("a")`, `SpawnEnemy("b")` on one service and observe `["a", "a", "b"]`; create a second `DBZService` and observe no enemies. Do not call a repository or `Fight`.
- **RED rule:** this test will pass against the current service and therefore is a post-GREEN regression, not valid historical RED. If strict prospective recovery is mandatory, temporarily replace only `SpawnEnemy`’s append with a no-op, observe the expected empty-list behavioral failure, then immediately restore the append. Do not alter the mutex, `Fight`, or `Enemies` snapshot behavior to manufacture RED.
- **Commands:** `go test -count=1 ./internal/service -run '^TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder$'` for required RED recovery then immediate GREEN.
- **GREEN criterion:** exact ordered duplicates in the first instance, empty second instance; no persistence seam introduced.

### Final focused verification

After every listed GREEN:

```sh
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

The focused listener package protects the shared authorization order; it is not a request to change listener code. Compare final status to the captured baseline and stop if an unrelated source/test path changed.

## Complexity, risks, and exclusions

| Area | Complexity | Risk | Control |
|---|---:|---|---|
| Command payload/delivery | Low | High observable parity: public source success versus target reply/whisper | Exact recipient/whisper/text tests. |
| Missing argument | Low | Medium exact text/prefix parity | Test exact `Example: <prefix>dspawn enemy`; retain shared `reply`. |
| Registration | Low | High merge risk because adapter is dirty/shared | One literal token, only under existing DBZ conditional, coordinated. |
| Lifecycle | Low | Medium future accidental persistence/deduplication | Instance-local duplicate/order regression. |
| Concurrency | No new implementation | Medium semantic overclaim | Retain mutex as Go safety; document source has no concurrent contract. |
| Authorization | No new implementation | High security risk | Preserve `REGULAR` and listener ordering; no identity/role changes. |

**Explicit exclusions:** `dfight`, DBZ register/stats/strength/level-up, character persistence, H2 schema/repository changes, enemy persistence/recovery, combat or RNG, enemy listing UI/API, expiry/caps/deduplication, access/grant, proxy/Whiskey, raw SQL, semantic moderation, factory/engine refactors, transport refactors/retries, staging, commits, and unrelated cleanup.

## Developer tier

**Senior Go maintainer with shared-worktree coordination.** The code delta is small, but delivery is externally visible and `dispatch_adapter.go` is a dirty shared boundary. A DB specialist is not needed; no SQL work is in scope.

## Baseline for this handoff

[TEST-BACKED] On the inspected dirty checkout:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok   zenbot/internal/command
ok   zenbot/internal/listener/message
ok   zenbot/internal/service
ok   zenbot/internal/repository/h2

git diff --check && git diff --cached --check
exit 0; no output
```

[OBSERVED] The only target DBZ diffs in the scoped `git diff` concern already-in-progress `dbzhelp` work and its registration move; they do not alter the current `dspawn` branch. This handoff creates no application/test modification.
