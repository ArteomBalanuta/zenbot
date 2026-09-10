# `dspawn` parity QA

## Verdict

**ACCEPTED WITH ONE CORRECTION.** The `dspawn` vertical now matches the inspected Saturn source on alias/role, DBZ-state-gated registration, argument behavior, mutation order, delivery shape, duplicate/order/lifecycle behavior, and normal status. No persistence, schema, repository, fight, RNG, authorization, or factory change was introduced for this QA correction.

## Independently inspected source provenance

- Saturn `DBZSpawnEnemyCommandImpl.java:16,23-40`: sole alias `dspawn`, `Role.REGULAR`, `requiredArgument(0, "dspawn enemy")`, `spawnEnemy` before public enqueue, then `SUCCESSFUL`.
- Saturn `UserCommandBaseImpl.java:51-71,154-164,191-202`: first whitespace token; absent/trimmed-blank argument calls addressed/whisper-preserving `Example: <prefix>dspawn enemy` and fails.
- Saturn `OutService.java:35-43`: one-argument public enqueue validates/normalizes then calls `queue.add`; an enqueue failure escapes as an unchecked exception.
- Saturn `DBZImpl.java:19,230-238`: per-instance `ArrayList`, append-only spawn, first-equal removal by `fight`; no DB/repository enemy operation.

## Audit evidence

- `internal/command/registry.go`: `dspawn` is exactly alias `dspawn`, role `REGULAR`.
- `internal/command/dispatch_adapter.go`: `dspawn` is registered only when `bundle(e).DBZ != nil`; it is not in the unconditional DBZ-help list.
- `internal/listener/message/handlers.go` and existing `dbz_dispatch_test.go`: active-user resolution and `REGULAR` authorization occur before legacy command execution; shared denied-DBZ tracer verifies no DBZ work before denial.
- `internal/model/chat_message.go`, `handlers.go:63-69`, and `dbz.go`: `strings.Fields` / command-token drop makes `dspawn` use only the first parsed nonblank argument and ignore later tokens.
- `TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate`: exact `Example: !dspawn enemy`, addressed whisper reply, `FAILED`, no spawn.
- `TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement`: whispered `!dspawn   frieza ignored` and public `!dspawn frieza` produce ordered `["frieza", "frieza"]` and two exact unaddressed/public acknowledgements.
- `TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder`: one service retains `["a", "a", "b"]`; a new service is empty. The retained Go mutex is a safety difference, not a claimed Saturn concurrency-order contract.

## Correction: public-send failure

The prior implementation ignored the Go `SendChatMessage` error and returned `SUCCESSFUL`. That contradicted source control flow: Saturn invokes `queue.add` after mutation and does not catch its unchecked failure, so it cannot return `SUCCESSFUL` when enqueue throws. Zenbot has an explicit error return at this boundary.

A focused test was added first:

```text
go test -count=1 ./internal/command -run '^TestDSpawnReturnsFailedAfterSpawnWhenPublicQueueFails$'
FAIL: status=SUCCESSFUL err=<nil>
```

The minimal correction in `internal/command/dbz.go` now returns `FAILED, err` after `SpawnEnemy(enemy)` if the public enqueue fails. The test then passed. It also asserts source ordering: the spawn remains recorded when the later enqueue fails.

## Gates run after correction

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok zenbot/internal/command (10.012s)
ok zenbot/internal/listener/message (3.323s)
ok zenbot/internal/service (2.666s)
ok zenbot/internal/repository/h2 (38.118s)

go test -race -count=1 ./internal/command ./internal/listener/message ./internal/service
ok zenbot/internal/command (47.607s)
ok zenbot/internal/listener/message (7.471s)
ok zenbot/internal/service (4.162s)

go test ./...
PASS: all packages (including command, listener/message, repository/h2, service)

go vet ./...
exit 0

go build ./...
exit 0

git diff --check && git diff --cached --check
exit 0; no output
```

## Scope and limitation

QA-owned changes are the focused public-send error check in `internal/command/dbz.go`, its focused regression in `internal/command/dbz_test.go`, and this handoff. The checkout was already heavily dirty; the wider DBZ/help and registration hunks were inspected but not altered by this QA correction. The implementation handoff records earlier temporary-isolation RED→GREEN tracers; those historical runs were not independently replayed because replay would require deliberately breaking shared dirty code. This QA independently observed the new send-failure RED and GREEN and reran all listed final gates.
