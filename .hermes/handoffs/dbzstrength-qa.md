# DBZ strength strict-parity QA

## Disposition

**ACCEPTED, with one QA-only correction.** The implementation matches the approved strict Saturn DBZ-strength contract. No production DBZ behavior was changed by QA.

## Independently verified behavior

- Catalog declares exactly `dbzstr`, `dstr`, and `daddstr` at `REGULAR`; registration remains fail-closed when `Bundle.DBZ` is unavailable.
- Dispatch retains the shared resolved-author `REGULAR` authorization boundary. The denied strength route performs no free-stat read or strength update.
- The command uses only `message.Name` for the DBZ subject. Extra tokens are ignored.
- `strconv.ParseInt(..., 10, 32)` supplies Java signed-32-bit parsing; missing, zero, negative, non-integer, and `2147483648` produce the addressed inbound-whisper-preserving exact usage payload `Example: !daddstr amount` without mutation.
- Valid successes are public/unaddressed and non-whisper: exact no-free payload for free stats `<= 0`, or the requested decimal amount after a positive precheck.
- Real H2 overspend is preserved: free `1`, strength `3`, amount `2` becomes free `-1`, strength `5` and publicly emits `2`.
- The H2 strength mutation remains one unguarded update with no transaction, availability predicate, lock, retry, unique/index, or schema change.
- Repository free-stat errors are converted by `DBZService.FreeStats` to `-1, nil`, producing source-equivalent public no-free success. Strength write errors are ignored at the command boundary and still produce the public amount acknowledgement. Pre-cancelled execution performs no DBZ call or output.

## QA correction

`go vet ./...` initially failed because the newly added `failingFreeStatsDBZRepo` test fake embedded a `sync.Mutex` and implemented `FreeStats` with a value receiver, copying that lock. Changed only that test fake to a pointer receiver in `internal/service/dbz_test.go`.

Added `TestDBZStrengthAcknowledgesPublicAmountWhenWriteFails` in `internal/command/dbz_test.go`; it directly proves the required source-shaped write-error success payload. This is post-implementation QA coverage, so it does not establish a historical RED phase. The supplied implementation handoff's claimed prior RED/GREEN evidence was reviewed but cannot be independently reconstructed from the final dirty worktree.

## Executed verification

All passed:

```text
go test -count=1 ./internal/listener/message -run '^(TestDispatchUserCommandDBZStrengthAliasOverspendsAndPublishesPublicAmount|TestDispatchUserCommandDBZStrengthInvalidAmountRepliesUsageWithoutMutation|TestDispatchUserCommandDBZStrengthNoFreeStatsPublishesPublicMessageWithoutMutation|TestDispatchUserCommandDBZStrengthDeniedRegularDoesNotReadOrWrite|TestDispatchUserCommandDBZStrengthPreCancelledDoesNotReadWriteOrReply)$'
go test -count=1 ./internal/command -run '^(TestDBZMalformedStrengthUsesSourceUsage|TestDBZStrengthReadErrorPublishesSourceEquivalentPublicNoFreeSuccess)$'
go test -count=1 ./internal/service -run '^TestDBZFreeStatsConvertsRepositoryReadErrorToSourceEquivalentNoFreeValue$'
go test -count=1 ./internal/repository/h2 -run '^TestDBZStrengthRealH2AddsRequestedAmountAndAllowsNegativeFreeStats$'
go test -race -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
go test -race -count=1 ./internal/command -run '^TestDBZStrengthAcknowledgesPublicAmountWhenWriteFails$'
go test -race -count=1 ./internal/service -run '^TestDBZFreeStatsConvertsRepositoryReadErrorToSourceEquivalentNoFreeValue$'
go test -count=1 ./...
go vet ./...
go build ./...
git diff --check && git diff --cached --check
```

No command staged, committed, reset, cleaned, or restored the dirty worktree.

## Files touched by QA

- `internal/service/dbz_test.go` — pointer receiver on the failing read fake to satisfy `go vet`.
- `internal/command/dbz_test.go` — write-error public-success QA regression.
- `.hermes/handoffs/dbzstrength-qa.md` — this report.
