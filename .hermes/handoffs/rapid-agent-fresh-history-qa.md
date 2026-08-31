# Mandatory fresh public history routing — independent QA

## ACCEPT

The bounded loop accepts the required-fresh route after independent review and verification. One hardening gap was found and repaired: public recognized fresh requests could bypass the bounded loop when a `Runner` or `DirectInvoker` was composed without `ToolLoop`, causing a provider-only response. Both paths now fail closed before their first provider request in that misconfiguration.

## Repair

- `internal/agent/live/runner.go`
  - If public assembly requires fresh history but `ToolLoop` is absent, return `required fresh history needs bounded tool loop` before `Client.Complete`.
- `internal/agent/live/direct.go`
  - Applies the same guard to the direct fallback path.
- `internal/agent/live/runner_test.go`
  - Adds a regression test proving no-loop public required-fresh runner calls neither provider nor fallback delivery.
- `internal/agent/live/direct_test.go`
  - Adds the matching direct-invoker regression test.

RED was observed before the repair:

```text
--- FAIL: TestDirectInvokerFailsClosedForRequiredFreshHistoryWithoutBoundedLoop
required fresh history fell back to a provider-only direct response
--- FAIL: TestRunnerFailsClosedForRequiredFreshHistoryWithoutBoundedLoop
required fresh history fell back to a provider-only response
```

Both tests passed after the repair.

## Independent findings

- Classification has one assembly owner: `assemble` delegates to `turn.FreshnessPolicy`; no second assembler parser remains. It normalizes escaped underscores, one leading `@`, Unicode letter/number nicks, and checks the 100-rune bound. Existing focused tests cover named user, possessive/history/speech forms, follow-up from latest user message, Unicode escaped underscore, and Java/Rome/Shakespeare/president/room/general-follow-up false positives.
- Mandatory execution is public-only in the loop: whisper requests receive no definitions and do not get automatic history execution; moderation clears required fields during assembly.
- For a public required request, completion #1 is never delivered. The loop ignores every first-provider call/prose, constructs canonical router-owned `{"nick":"<trusted nick>"}`, executes only current-room `user_message_history`, and supplies exactly one synthetic assistant/tool pair with matching `fresh-history-<request-id>` ID before a tools-disabled completion #2.
- The loop rejects first `length` before execution and fails closed on unavailable/unsafe descriptor, invalid nick/result, executor error/cancellation, second `length`, blank, tool call, or trimmed repetition. It creates durable evidence only after a validated successful history result. Runner/direct persistence remains post-visible-delivery only.
- Normal nonrequired model-selected one-call path is unchanged. The frozen limit remains two completions and one tool call.

## Verification

Passed:

```text
gofmt -w internal/agent/live/runner.go internal/agent/live/direct.go internal/agent/live/runner_test.go internal/agent/live/direct_test.go
go test ./internal/agent/live -run 'Test(Runner|DirectInvoker)FailsClosedForRequiredFreshHistoryWithoutBoundedLoop' -count=1 -v
go test ./internal/agent/live -run 'TestToolLoop(Forces|Ignores|Fails|Stops|Rejects)' -count=1 -v
go test ./internal/agent/turn -run 'Test(HistoryNickAndFreshness|FreshnessPolicy|ParityFinalValidator)' -count=1
go test ./internal/agent/assemble -run 'Test(TruncateFreshnessBoundsAndCancellation|.*Fresh)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/live -count=1
go test -race ./internal/agent/turn ./internal/agent/assemble ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` has only the known unrelated existing warning and exits nonzero:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions

No SQL/schema/query/config/moderation/command/router generalization, new tool, retry, third completion, commit, push, or unrelated dirty-file cleanup was performed. The repository was already heavily dirty; only the four live files above and this QA handoff were changed by this review.
