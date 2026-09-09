# QA: lifecycle concrete-factory missing-controller repair

## Verdict

**PASS for the missing-controller panic repair.** Direct concrete `restart` and
`shutdown` handlers built against a bare `commandEngineStub` now return
`model.FAILED` with exactly `host lifecycle controller is not configured`, emit
no chat/raw output, and do not panic. The 64-factory catalog traversal now
completes without a lifecycle nil-pointer panic.

## Independent coverage added

`internal/command/restart_shutdown_test.go` now includes
`TestRestartShutdownDirectExecutionPreservesControllerBehavior`, covering both
canonicals for:

- already-cancelled context: `FAILED`, `context.Canceled`, zero controller
  calls, and no output;
- controller success: one corresponding request, `SUCCESSFUL`, nil error, and
  no output;
- controller request failure: one corresponding request, logged/
  `SUCCESSFUL` bridge, nil returned error, and no output.

The existing direct missing-controller test verifies both canonicals separately.
The existing registration test verifies missing-capability registration exposes
none of `restart`, `reload`, `re`, `exit`, `quit`, or `shutdown`; a controller
backed engine preserves each alias, ADMIN role, and concrete command type.

## Commands witnessed

```text
go test ./internal/command -run '^(TestRestartShutdownDirectExecutionWithoutControllerFailsClosed|TestRestartShutdownDirectExecutionPreservesControllerBehavior|TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestSaturnCatalogHas64ConcreteFactories)$' -count=1 -v
PASS

go test -race ./internal/command -run '^(TestRestartShutdownDirectExecutionWithoutControllerFailsClosed|TestRestartShutdownDirectExecutionPreservesControllerBehavior|TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete|TestSaturnCatalogHas64ConcreteFactories)$' -count=1
PASS

go test ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1 -v
PASS

go test -race ./internal/listener -run '^TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies$' -count=1
PASS

go test ./internal/command -run '^TestSaturnCatalogHas64ConcreteFactories$' -count=1 -v
PASS

go test -run '^$' ./...
PASS (all packages compile with tests skipped)
```

## Baseline and scope

`go test ./internal/command -count=1` remains red only at
`TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded`:
`SqlUserCommandImpl (sql) generic fallback=false, allowed transitional
fallback=true`. This is the documented pre-tracer-11 SQL catalog mismatch; no
SQL/catalog change was made here and no lifecycle panic was observed.

`TestSQLRegistrationRequiresRawQueryCapability` is not present in this current
worktree (`go test ... -run '^TestSQLRegistrationRequiresRawQueryCapability$'`
reports `no tests to run`), so it cannot be used as a current regression gate.

`git diff --check` passed for tracked changes. The lifecycle source and tests
are untracked in this dirty worktree, so they were additionally formatted with
`gofmt`; untracked-file diff checks are recorded separately in the final QA
command output. No staging, reset, restore, clean, or commit was performed.
