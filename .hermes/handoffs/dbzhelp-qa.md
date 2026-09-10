# DBZ help source-exact parity: independent QA

## Verdict

**PASS — no DBZ-help production defect found.** The implementation matches the bounded Saturn `DBZHelpCommandImpl` / `OutService` contract. One focused **post-implementation QA regression** was added for the only previously untested exceptional behavior: output-send error propagation.

## Independent source audit

Read:

- `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZHelpCommandImpl.java`
- `../../projects/saturn/src/main/java/org/saturn/app/service/impl/OutService.java`
- target Go sources, registry/adapter, listener dispatch, and focused test patterns.

Confirmed source parity:

- `dbzhelp`, `dbz`, and `dhelp` are `REGULAR` aliases.
- Help alone has no DBZ service dependency: `dbzCommand.Execute` handles canonical `dbzhelp` before `bundle(e).DBZ` acquisition; registration includes only `dbzhelp` unconditionally.
- Without DBZ state, mutable/read DBZ aliases (checked `dbzstats`) remain absent.
- Arguments and inbound whisper state are ignored by the help branch.
- Exact payload uses physical LF bytes, contains literal ASCII `\\u2009` rather than U+2009, and ends in LF. This follows Saturn's `escapeJava` then `normalizeForChatPayload` behavior, which converts `\\n` only.
- Delivery is unaddressed/public: `SendChatMessage("", payload, false)`.
- Send failure returns `FAILED` with the original error; successful send returns `SUCCESSFUL`.
- Listener authorization remains before execution: `DispatchUserCommand.Handle` resolves the command, checks `REGULAR`, and only then calls `ExecuteContext`/`Execute`. Existing authorization tests passed; no identity or policy code changed.

## Claimed RED audit and TDD limitation

The implementation handoff records two sequential REDs with expected, behavior-specific failures:

1. Registration tracer: `dbzhelp was not registered` before the canonical-list move.
2. Exact-output tracer: `status=FAILED err=DBZ state unavailable` before the pre-bundle help branch.

Both failure modes agree with the pre-change diff and source control flow. Their historical timing cannot be independently reproduced now without reverting dirty work, which was expressly prohibited. Therefore this QA can audit the transcript's internal consistency and verify the green behavior, but cannot independently attest to temporal TDD order.

The newly added send-error test is explicitly labelled as a post-implementation QA regression and passed on its first execution; it is not claimed as a RED/GREEN proof.

## Changes made by QA

- `internal/command/dbz_test.go`
  - Added `TestDBZHelpReturnsFailedWhenPublicSendFails` using a local erroring output seam.
  - Added only `errors` import and that test seam.
- `.hermes/handoffs/dbzhelp-qa.md`
  - This audit record.

No DBZ-help production source required alteration. No unrelated file was edited. No reset, checkout, restore, clean, stash, stage, or commit was performed.

## Executed evidence

```text
$ go test -count=1 ./internal/command -run '^TestDBZHelpReturnsFailedWhenPublicSendFails$'
ok  zenbot/internal/command  0.514s

$ go test -count=1 ./internal/command -run '^(TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState|TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload|TestDBZHelpReturnsFailedWhenPublicSendFails)$'
ok  zenbot/internal/command  0.466s

$ go test -count=1 ./internal/command ./internal/listener/message
ok  zenbot/internal/command           10.033s
ok  zenbot/internal/listener/message   3.092s

$ go test -race -count=1 ./internal/command ./internal/listener/message
ok  zenbot/internal/command           45.052s
ok  zenbot/internal/listener/message   7.259s

$ go test -count=1 ./...
PASS: all packages; slowest printed package was zenbot/internal/repository/h2 (39.024s).

$ go vet ./...
exit 0; no output

$ go build ./...
exit 0; no output

$ git diff --check && git diff --cached --check
exit 0; no output
```

## Scope and dirty-worktree boundary

The worktree was already broadly dirty. Final checks show the DBZ-help slice remains limited to existing implementation files:

- `internal/command/dbz.go`
- `internal/command/dbz_test.go`
- `internal/command/dispatch_adapter.go` (shared file; this QA made no edit)
- `internal/command/dispatch_integration_test.go`

This QA additionally created this handoff. The pre-existing broad dirty state was preserved. No policy-deferred DBZ mechanics, schema/repository/service work, authorization changes, or transport/helper refactor was introduced.
