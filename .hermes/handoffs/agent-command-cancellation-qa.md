# Lifecycle-to-synchronous-command cancellation — independent QA

## Verdict

**PASS.** The inspected admission-only bridge satisfies the requested lifecycle-to-synchronous-command boundary. No production defect was found, so no production code was changed. Accepted `l` work remains runtime-owned after successful admission.

## Scope and diff audit

Inspected implementation boundary:

- `internal/core/engine_impl.go`
- `internal/listener/user_chat_listener.go`
- `internal/listener/message/handlers.go`
- `internal/command/dispatch_adapter.go`
- direct submission/runtime tests and implementation
- `agent-command-cancellation-current-architecture.md` and implementation handoff

Read-back findings:

1. `StartContext` passes its lifecycle context only to `DispatchMessageContext`; the compatibility `DispatchMessage` wrapper uses `context.Background()`.
2. Only `chat` uses a local optional `NotifyContext(context.Context, string)` assertion. Other `common.Listener` callers and public common contracts are unchanged.
3. `UserChatListener.Notify` calls `NotifyContext(context.Background(), ...)`. `NotifyContext` derives one cancelable child immediately around the single synchronous `Chain.Process` call and defers child cancellation.
4. Default handler ordering is unchanged and remains covered by `TestDefaultChainPlacesRelayBeforeAllDownstreamHandlers`.
5. `DispatchUserCommand` performs lookup and authorization before the optional contextual execution branch; non-context `common.Command` implementations retain `Execute()`.
6. `legacyAdapter.Execute` remains background-compatible. `ExecuteContext` shares the exact existing Saturn-command error logging path with a nil-context background fallback.
7. `DirectSubmissionAdapter` rejects canceled contexts before invocation creation/submission. `runtime.Runtime.execute` continues to pass only `rt.ctx` to its runner, so caller/message cancellation after successful admission does not cancel accepted work. `Runtime.Close` remains the cancellation owner.
8. `SemanticModerationIngressReady()` remains `false`. No QA change touched tools, command-gateway reachability, common contracts, moderation, SQL/H2, config, proxy/Whiskey, snapshot, or automove paths.

The dirty worktree contains extensive pre-existing unrelated tracked and untracked work. I did not reset, clean, checkout, stage, or commit anything. `git diff --check` passed.

## TDD evidence audit

The implementation handoff records the required tracer RED before production work:

```text
internal/command/handlers_test.go:270: NotifyContext undefined
FAIL zenbot/internal/command [build failed]
```

It then records the same tracer GREEN. Independent QA added only post-GREEN regression coverage because the behavior was already implemented; all added regressions passed on their first valid run. No root-cause fix cycle was needed.

## Independent QA coverage added

- `internal/core/engine_command_cancellation_test.go`
  - lifecycle `StartContext` cancellation reaches an in-flight synchronous contextual chat notifier and `StopContext` completes;
  - direct `DispatchMessageContext` delivers the supplied lifecycle context only to the optional chat notifier;
  - compatibility `DispatchMessage` remains background-derived;
  - legacy chat notifiers still receive `Notify`.
- `internal/listener/user_chat_listener_context_test.go`
  - chat child context is live during the synchronous chain and canceled on return;
  - compatibility `Notify` starts from background semantics.
- `internal/listener/message/dispatch_authorization_test.go`
  - authorization occurs before contextual execution and contextual commands receive the exact canceled context;
  - legacy non-context commands still execute via `Execute()`.
- `internal/agent/live/direct_submission_test.go`
  - accepted direct work remains live after caller cancellation and is canceled only by runtime close.

## Verification

All exited 0:

```sh
gofmt -w internal/core/engine_command_cancellation_test.go internal/listener/user_chat_listener_context_test.go internal/listener/message/dispatch_authorization_test.go internal/agent/live/direct_submission_test.go
go test ./internal/command -run 'Test(LiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission|DirectLCommand)' -count=1
go test ./internal/core ./internal/listener ./internal/listener/message -run 'Test(StartContextCancelsSynchronousChatDispatchOnLifecycleStop|UserChatListenerNotify|DispatchMessage|DispatchUserCommand)' -count=1
go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Cancellation)' -count=1
go test -race ./internal/command ./internal/core ./internal/listener/message ./internal/listener ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Observed race sweep: all six packages passed; full suite passed including `cmd/zenbot`, command/core/listener/runtime, agent, factory, transport, and H2 repository packages.

## Exact QA-touched paths

- `.hermes/handoffs/agent-command-cancellation-qa.md` (new)
- `internal/core/engine_command_cancellation_test.go` (new)
- `internal/listener/user_chat_listener_context_test.go` (new)
- `internal/listener/message/dispatch_authorization_test.go` (modified)
- `internal/agent/live/direct_submission_test.go` (augmented; the file was already untracked in the incoming dirty worktree)
