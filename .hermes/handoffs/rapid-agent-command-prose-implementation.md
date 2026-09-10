# Rapid agent command-prose implementation

## Delivered paths

- `internal/agent/turn/command_prose_guard.go`
- `internal/agent/turn/command_prose_guard_test.go`
- `internal/agent/live/command_channel.go`
- `internal/agent/live/tool_loop.go`
- `internal/agent/live/tool_loop_test.go`
- `internal/agent/live/runner.go`, `runner_test.go`
- `internal/agent/live/direct.go`, `direct_test.go`

The live package was already untracked in the caller's dirty workspace; this slice changed only the listed files within it and the two new turn guard files. No gateway, engine capture, listener, transport, command catalog, persistence schema, `MIGRATION_PLAN.md`, or `.hermes/migration-audit.md` was edited. No commit was made.

## TDD evidence

1. **Guard RED:** after creating `command_prose_guard_test.go`, `go test ./internal/agent/turn -run 'TestCommandProseGuard' -count=1` failed with `undefined: NewCommandProseGuard` (three references).
   **GREEN:** implemented the definition-derived parser/validator; the same test passed.
2. **Structured correction RED:** after adding `TestToolLoopCorrectsPublicCommandProseIntoOneStructuredCommand`, `go test ./internal/agent/live -run TestToolLoopCorrectsPublicCommandProseIntoOneStructuredCommand -count=1` failed with `completion.SuppressReply undefined`.
   **GREEN:** added private `commandChannel`, correction definitions, one-shot execution, and `Completion.SuppressReply`; the focused test passed.
3. **Post-action prose RED:** `TestToolLoopSuppressesCommandProseAfterSuccessfulRunCommand` failed with `SuppressReply:false` after exactly two requests and one command gateway call.
   **GREEN:** added only the successful `run_command` final-content guard; focused correction/fallback/invalid/final tests passed.
4. **Runner RED:** `TestRunnerSuppressesOrdinaryReplyForCorrectedCommandDelivery` failed with `finalize agent response: agent returned an empty response`.
   **GREEN:** runner maps `SuppressReply` to `runtime.NewResultWithEvidence(requestID, "", false, nil)` before finalization; test passed.
5. **Direct RED:** `TestDirectInvokerSuppressesOrdinaryReplyForCorrectedCommandDelivery` failed with `agent returned an empty response`.
   **GREEN:** direct invocation returns an empty `runtime.DirectCompletion` before finalization; test passed.

## Semantics and security gates

- Alias recognition is derived solely from the actual advertised frozen `run_command` descriptor schema (`properties.command.enum`); malformed schemas grant no aliases. It recognizes only inline code or complete backtick/tilde fenced code, never normal prose. It rejects unadvertised `whois`, moderator `kick`, malformed shapes, case-mismatched calls, non-string arguments, and extra keys.
- On a public no-call completion containing recognized Markdown command prose, fresh-history routing has already taken precedence. The correction request is completion #2 and advertises exactly `run_command` plus private closed `respond_without_command`. It accepts exactly one matching action or a closed nonblank fallback free of recognized command prose. Truncation, unrelated/multiple calls, invalid fallback, and repeated command prose fail closed with no third completion.
- Structured correction uses the existing registry, descriptor validation, ledger, executor, timeout, context, and gateway exactly once. Successful command delivery yields `SuppressReply`, no ordinary output, no replay, and no durable evidence/persistence. A command failure does not synthesize/retry.
- Valid fallback resumes ordinary finalizer/delivery and post-send persistence exactly once. Whispers receive no correction definitions and recognized command prose fails locally with no gateway call or second completion.
- After a normal successful model-selected `run_command`, only recognized Markdown command prose in its tools-disabled completion #2 is suppressed. Ordinary acknowledgements remain deliverable/persistable; no completion #3 is made.

## Bounded target adaptation

Saturn's broader command policy can continue after correction to resolve a tool result. This implementation intentionally does not: the accepted Zenbot ceiling remains at most two completions and one tool call. A corrected structured action returns a local typed no-ordinary-reply completion after the command's existing real room delivery. That is the deliberate source incompatibility; it avoids a third completion, synthesized acknowledgement, evidence, or persistence.

## Verification

Passed:

```text
go test ./internal/agent/turn -run 'Test.*(CommandProseGuard|Policy)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*(CommandProse|RunCommand|Fresh|Whisper)|CommandChannel|Runner.*CommandProse|RunnerSuppressesOrdinaryReplyForCorrectedCommandDelivery|Direct.*CommandProse|DirectInvokerSuppressesOrdinaryReplyForCorrectedCommandDelivery)' -count=1
go test ./internal/agent/live -count=1
go test ./internal/agent/tool ./internal/agent/tool/execution ./internal/command -run 'Test(RunCommand|Executor|AgentCommandGateway)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/turn ./internal/agent/live ./internal/agent/tool ./internal/agent/tool/execution ./internal/command ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
gofmt -d <all owned Go files>  # empty output
```

`go vet ./...` remains nonzero only for the known pre-existing copylocks warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

It was not changed in this slice.
