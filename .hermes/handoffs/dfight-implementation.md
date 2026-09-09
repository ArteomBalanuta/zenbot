# `dfight` strict-parity implementation

## Delivered delta

`internal/command/dbz.go` now preserves the existing sequential DBZ path and corrects only the two command-visible Saturn parity gaps:

1. Missing input replies (addressed and whisper-preserving) with exactly `Example: <prefix>dfight enemy`, then returns `FAILED` before `Fight` or `LevelUp`.
2. A present enemy token calls `Fight(first-token)`, ignores the following `LevelUp` error, then sends exactly `Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!` through checked `SendChatMessage("", payload, false)`.

The checked output error is the specified Zenbot transport adaptation: it returns `FAILED` only after list/H2 effects. No combat/RNG/existence check, enemy persistence, transaction, lock, schema, identity, or authorization behavior was added.

## Strict TDD evidence

### Tracer 1: real H2 alias / first matching duplicate / H2 effects / public acknowledgement

Added `TestDispatchUserCommandDBZFightAliasConsumesFirstMatchLevelsWithoutEnemyProofAndPublishes` in the existing untracked listener tracer file.

- RED observed with the exact old result: `recipient:"goku", whisper:true`; expected `recipient:"", whisper:false`.
- Minimal GREEN changed only the successful `dfight` delivery to checked public `SendChatMessage`.
- Green command: `go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFightAliasConsumesFirstMatchLevelsWithoutEnemyProofAndPublishes$'` → `ok`.
- The real-H2 assertion verifies `goku` level `2→3`, free stats `5→10`, and enemy list `["frieza", "frieza", "cell"]→["frieza", "cell"]` for whispered `!df frieza ignored`.

### Tracer 2: nonexistent enemy does not reject or suppress levelling

Added `TestDispatchUserCommandDBZFightMissingEnemyStillLevelsAndPublishes`.

- This was intentionally a post-GREEN regression/provenance test: its first run was green because existing `Fight` already does a no-op on no match.
- Command: `go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFightMissingEnemyStillLevelsAndPublishes$'` → `ok`.
- It proves no enemy mutation, public fixed acknowledgement, and real-H2 level/free-stat increase.

### Tracer 3: missing argument exact usage / no mutation

Added `TestDispatchUserCommandDBZFightMissingEnemyArgumentUsesSourceUsageWithoutMutation`.

- RED observed the old exact payload `dfight enemy`; expected `Example: !dfight enemy`.
- Minimal GREEN changed only that missing-argument literal.
- Green command: `go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFightMissingEnemyArgumentUsesSourceUsageWithoutMutation$'` → `ok`.
- It verifies the enemy list and real-H2 row remain unchanged.

### Tracer 4: swallowed level-up error and checked-send adaptation

Added command-layer tests:

- `TestDBZFightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy`: regression/provenance test, green on first execution because the command already discards `LevelUp` errors. It verifies a matching enemy is consumed, one level-up attempt occurs, and the public fixed acknowledgement still succeeds.
- `TestDBZFightReturnsFailedAfterEffectsWhenPublicSendFails`: verifies the target-only send-error mapping. Enemy consumption and one level-up attempt occur before `FAILED` with the exact send error.

Both focused commands passed.

### Shared boundaries

Added focused guards proving the existing shared boundaries remain untouched:

- no DBZ bundle registers neither `dfight` nor `df`;
- denied `REGULAR` authorization causes neither `Fight` nor `LevelUp` effect and retains the shared unauthorized whisper;
- pre-cancelled context causes no enemy/list/H2/output effect.

## Final gates

Passed:

```text
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok  zenbot/internal/command
ok  zenbot/internal/listener/message
ok  zenbot/internal/service
ok  zenbot/internal/repository/h2

git diff --check && git diff --cached --check
exit 0
```

`gofmt` ran on the three touched Go files. The repository was already extensively dirty; no files were staged, reset, restored, cleaned, stashed, or committed.

## Touched files

- `internal/command/dbz.go` — two bounded `dfight` delivery/usage corrections.
- `internal/command/dbz_test.go` — focused swallowed-level-up-error and send-after-effects tracers plus no-bundle registration guard.
- `internal/listener/message/dbz_dispatch_test.go` (pre-existing untracked DBZ tracer) — real-H2 strict behavioral tracers and authorization/cancellation guards.
- `.hermes/handoffs/dfight-implementation.md` — this evidence record.
