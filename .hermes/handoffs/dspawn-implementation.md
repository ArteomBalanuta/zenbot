# `dspawn` source-exact transient DBZ parity — implementation

## Scope delivered

Implemented only the `dspawn` command-visible source parity vertical:

- DBZ-state-gated registration remains in the existing DBZ conditional list.
- Missing enemy returns `FAILED`, makes no spawn mutation, and replies with exact prefix-aware `Example: <prefix>dspawn enemy`, preserving addressed/whisper behavior.
- Success appends the parsed first token, permits ordered duplicates, and emits the source-shaped unaddressed/public `spawned enemy: <enemy>` acknowledgement.
- DBZ enemy state remains append-only and instance-local; no repository/schema/persistence/fight/RNG/auth/transport refactor change was made.

## Strict sequential TDD transcript

1. **`TestRegisterUserUtilitiesRegistersDSpawnOnlyWhenDBZStateExists`**
   - Test added in `internal/command/dbz_test.go`.
   - Temporary DBZ-local isolation removed only the `"dspawn"` literal from the existing DBZ conditional list in `internal/command/dispatch_adapter.go`.
   - RED: `go test -count=1 ./internal/command -run '^TestRegisterUserUtilitiesRegistersDSpawnOnlyWhenDBZStateExists$'` exited 1 with `dspawn was not registered with DBZ state`.
   - GREEN restored only that literal immediately; same command passed (`ok zenbot/internal/command`).

2. **`TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate`**
   - Test added in `internal/command/dbz_test.go`.
   - RED: `go test -count=1 ./internal/command -run '^TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate$'` exited 1 with `chats=["goku|dspawn enemy|true"]`.
   - GREEN changed only the missing-argument reply payload to `Example: ` plus configured prefix plus `dspawn enemy`; same command passed (`ok zenbot/internal/command`).

3. **`TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement`**
   - Test added in `internal/command/dbz_test.go`.
   - RED: `go test -count=1 ./internal/command -run '^TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement$'` exited 1 with `chats=["goku|spawned enemy: frieza|true" "goku|spawned enemy: frieza|false"]`.
   - GREEN retained `SpawnEnemy(enemy)` and changed only success delivery to `SendChatMessage("", "spawned enemy: "+enemy, false)`; same command passed (`ok zenbot/internal/command`).

4. **`TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder`**
   - Test added in `internal/service/dbz_test.go`.
   - Because the pre-existing append implementation was green, temporary permitted isolation replaced only `SpawnEnemy`'s append with a mutex-protected no-op.
   - RED: `go test -count=1 ./internal/service -run '^TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder$'` exited 1 with `first enemies=[] want=["a" "a" "b"]`.
   - GREEN restored the exact append immediately; same command passed (`ok zenbot/internal/service`). No service production diff remains from this tracer.

## Touched paths

Final intentional implementation/test changes:

- `internal/command/dbz.go`
- `internal/command/dbz_test.go`
- `internal/service/dbz_test.go`

Temporary-only, restored byte-for-byte to its preexisting dirty state:

- `internal/command/dispatch_adapter.go`
- `internal/service/dbz.go`

This handoff was added:

- `.hermes/handoffs/dspawn-implementation.md`

## Focused final gate

Executed:

```sh
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
git diff --check && git diff --cached --check
git status --short
```

Result: all four packages passed (`internal/command` 10.549s, `internal/listener/message` 5.189s, `internal/service` 2.791s, `internal/repository/h2` 38.761s); both diff checks exited 0. The worktree remains heavily dirty with the pre-existing unrelated paths; this vertical added only the scoped test/command changes above and this handoff.
