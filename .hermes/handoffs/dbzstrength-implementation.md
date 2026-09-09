# DBZ strength (`dbzstr`) implementation

## Scope and decision

Implemented only the source-exact DBZ strength vertical (`dbzstr`, `dstr`, `daddstr`) from `dbzstrength-current-architecture.md`.

The chosen strict Saturn-observable free-stat read policy is implemented: `DBZService.FreeStats` converts repository read errors to `-1, nil`. Therefore command execution uses the public, successful exact payload `You don't have free stats. Level up!`, skips `AddStrength`, and does not propagate the read error. This mirrors Saturn's observable read-failure result. No retries, locks, transactions, conditional updates, schema/index changes, authorization/identity changes, or duplicate-name remediation were added.

## Delivered behavior

- Successful strength routes send public/non-whisper decimal amount acknowledgements using checked `SendChatMessage("", amount, false)`.
- No-free (including missing/read-error-converted `-1`) sends public/non-whisper exact source text and skips mutation.
- Invalid/missing/zero/negative/non-integer/out-of-signed-32-bit-range first amount uses the addressed inbound-whisper-preserving exact usage reply. Parsing now uses `strconv.ParseInt(..., 10, 32)` to preserve Java `Integer.parseInt` bounds on 64-bit Go hosts.
- Strength remains caller-name-only and consumes the full requested amount without availability checks. Real H2 coverage verifies `free=1`, amount `2` produces `str=5`, `free=-1`.
- Existing pre-cancel behavior and the shared `REGULAR` authorization gate remain outside the DBZ policy; strength-specific tests verify denied/cancelled paths perform no DBZ call/output as appropriate.

## Sequential tracer evidence

1. `TestDispatchUserCommandDBZStrengthAliasOverspendsAndPublishesPublicAmount`
   - RED: addressed whisper reply `{recipient:goku, text:2, whisper:true}` versus required public reply; mutation was already `str=5/free=-1`.
   - GREEN: public amount send passed.
2. `TestDispatchUserCommandDBZStrengthInvalidAmountRepliesUsageWithoutMutation`
   - RED: isolated usage delivery produced public output; the test also exposed host-width `Atoi` accepting `2147483648`.
   - GREEN: restored addressed usage and introduced signed-32-bit parsing; all five cases passed without mutation.
3. `TestDispatchUserCommandDBZStrengthNoFreeStatsPublishesPublicMessageWithoutMutation`
   - RED: isolated addressed reply mismatched required public/non-whisper delivery.
   - GREEN: public exact no-free message passed; row unchanged.
4. `TestDBZStrengthRealH2AddsRequestedAmountAndAllowsNegativeFreeStats`
   - RED: temporarily isolating subtraction left `free_stats=1` rather than `-1`.
   - GREEN: restored same-amount H2 subtraction; test passed.
5. `TestDBZFreeStatsConvertsRepositoryReadErrorToSourceEquivalentNoFreeValue` and `TestDBZStrengthReadErrorPublishesSourceEquivalentPublicNoFreeSuccess`
   - RED: repository sentinel read error propagated (`-1, error`; command `FAILED`).
   - GREEN: service conversion to `-1, nil` passed both service and public command outcome tests.
6. `TestDispatchUserCommandDBZStrengthDeniedRegularDoesNotReadOrWrite` and `TestDispatchUserCommandDBZStrengthPreCancelledDoesNotReadWriteOrReply`
   - Denied route was left under the existing shared authorization boundary; test passes with no `FreeStats`/`AddStrength` calls and the existing unauthorized reply.
   - Cancellation RED after temporarily isolating the DBZ command's existing context guard showed one `FreeStats` call and public no-free output; restored guard GREEN had no DBZ call or output.

All temporary RED isolations were restored.

## Final verification

Executed successfully:

```sh
gofmt -w internal/command/dbz.go internal/command/dbz_test.go internal/listener/message/dbz_dispatch_test.go internal/service/dbz.go internal/service/dbz_test.go internal/repository/h2/dbz_test.go
go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
# ok zenbot/internal/command
# ok zenbot/internal/listener/message
# ok zenbot/internal/service
# ok zenbot/internal/repository/h2
git diff --check && git diff --cached --check
# exit 0; no output
```

## Files changed by this vertical

- `internal/command/dbz.go`
- `internal/command/dbz_test.go`
- `internal/listener/message/dbz_dispatch_test.go` (pre-existing untracked test file)
- `internal/service/dbz.go`
- `internal/service/dbz_test.go`
- `internal/repository/h2/dbz_test.go`
- `.hermes/handoffs/dbzstrength-implementation.md`

The worktree was already extensively dirty. Existing unrelated DBZ help/register/spawn and stats test hunks in several listed files were preserved; no reset, checkout, restore, clean, stash, staging, or commit was performed.
