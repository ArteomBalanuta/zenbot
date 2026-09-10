# Live-agent composition and conditional `l` registration — tracer 5

## Scope

Completed only the final live-agent composition/registration tracer. Existing dirty unrelated main/command/worktree changes were neither reverted nor extended.

## RED

After adding `TestNewLiveAgentSharesRuntimeWithDirectSubmitter` to `cmd/zenbot/live_agent_test.go`, before editing production composition:

```text
$ go test ./cmd/zenbot -run '^TestNewLiveAgentSharesRuntimeWithDirectSubmitter$' -count=1
# zenbot/cmd/zenbot [zenbot/cmd/zenbot.test]
cmd/zenbot/main.go:198:150: undefined: command.DirectAgentInvoker
cmd/zenbot/live_agent_test.go:51:13: enabled.Service undefined (type *liveAgent has no field or method Service)
cmd/zenbot/live_agent_test.go:51:65: enabled.DirectSubmitter undefined (type *liveAgent has no field or method DirectSubmitter)
cmd/zenbot/live_agent_test.go:54:25: enabled.Service undefined (type *liveAgent has no field or method Service)
cmd/zenbot/live_agent_test.go:56:66: enabled.Service undefined (type *liveAgent has no field or method Service)
cmd/zenbot/live_agent_test.go:58:24: enabled.DirectSubmitter undefined (type *liveAgent has no field or method DirectSubmitter)
cmd/zenbot/live_agent_test.go:59:38: enabled.Service undefined (type *liveAgent has no field or method Service)
cmd/zenbot/live_agent_test.go:60:75: enabled.DirectSubmitter undefined (type *liveAgent has no field or method DirectSubmitter)
cmd/zenbot/live_agent_test.go:63:86: enabled.DirectSubmitter undefined (type *liveAgent has no field or method DirectSubmitter)
cmd/zenbot/live_agent_test.go:74:41: disabled.Service undefined (type *liveAgent has no field or method Service)
cmd/zenbot/live_agent_test.go:74:41: too many errors
FAIL    zenbot/cmd/zenbot [build failed]
FAIL
(exit 1)
```

This is the expected missing-composition failure. `DirectAgentInvoker` was already removed by the preceding command tracer while `main.go` still retained its obsolete duplicate composition reference.

## GREEN

Implemented the smallest composition-root change:

- `liveAgent` now contains the shared `live.AgentService`, room participation, and `command.DirectAgentSubmitter`.
- Enabled `newLiveAgent` creates exactly one runtime, wraps it once in `live.RuntimeService`, uses that service for room participation and `live.DirectSubmissionAdapter`, and returns both.
- Disabled composition returns pass participation with nil service/submitter before provider/runtime construction.
- Removed the duplicate `directAgentInvoker` production composition.
- Main conditionally supplies `roomAgent.DirectSubmitter` to `RegisterUserUtilitiesWithDirectAgent`; it closes the same composed service through `roomAgent.Close()` before lifecycle/replica/DB teardown.

```text
$ gofmt -w cmd/zenbot/main.go cmd/zenbot/live_agent_test.go && go test ./cmd/zenbot -run '^TestNewLiveAgentSharesRuntimeWithDirectSubmitter$' -count=1
ok      zenbot/cmd/zenbot  0.633s
```

The focused test proves enabled shared service/runtime identity, enabled `l` registration, disabled pass-through/no service/no submitter/no `l`, while the existing ambient test retains `SemanticCandidate == nil` and semantic readiness false.

## Regression and scope verification

```text
$ go test ./internal/command -run 'TestDirectLCommand' -count=1 && go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Delivery)' -count=1
ok      zenbot/internal/command  0.326s
ok      zenbot/internal/agent/live  0.550s
ok      zenbot/internal/agent/runtime  0.410s

$ go test ./internal/listener/message ./internal/listener -run 'Test.*(Chain|Participation|Joined)' -count=1 && go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
ok      zenbot/internal/listener/message  0.390s
ok      zenbot/internal/listener  0.659s
ok      zenbot/cmd/zenbot  0.442s

$ go test ./internal/core -run 'Test.*Lifecycle' -count=1
ok      zenbot/internal/core  0.387s

$ go test -race ./internal/command ./internal/agent/live ./internal/agent/runtime ./internal/listener/message ./cmd/zenbot -count=1
ok      zenbot/internal/command  42.873s
ok      zenbot/internal/agent/live  1.866s
ok      zenbot/internal/agent/runtime  2.115s
ok      zenbot/internal/listener/message  2.349s
ok      zenbot/cmd/zenbot  5.112s

$ go test ./... -count=1
PASS: all packages (including repository/h2, 39.158s)

$ go vet ./...
(exit 0; no diagnostics)

$ go build ./...
(exit 0; no diagnostics)

$ git diff --check
(exit 0; no diagnostics)
```

Read-back checks:

- Production `cmd/zenbot` has zero `directAgentInvoker`/`DirectAgentInvoker` matches.
- Main has one `runtime.NewWithFailureSink` and one `RuntimeService{Runtime: rt}` composition; runtime close occurs only in `live.RuntimeService.Close`, called from main through `roomAgent.Close()`.
- `DefaultChainWithParticipation` remains exactly `ResolveUserMetadata, AuditChatMessage, IgnoreBotMessage, RelayAgentMessage, LogChatMessage, DeliverPendingMail, UpdateAfkState, YoutubePreview, CernEasterEgg, AgentParticipation, DispatchUserCommand`.
- `SemanticModerationIngressReady()` remains literal `false`.
- No new moderation, DynamicSQL, raw-send, proxy/Whiskey, provider-secret, SQL, or public tool reachability was introduced. The existing fixed tool loop/catalog composition remains unchanged.
