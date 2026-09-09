# Independent QA: shared-runtime `l` integration

## Verdict

**PASS.** The configured `l` route is conditionally registered, admission-only, factory-derived from trusted listener state, and uses the one live runtime/service that owns visible delivery and persistence ordering. No production defect was found.

## TDD / implementation evidence audit

Read the current architecture and all implementation, forensic/recovery, runtime-service, and composition handoffs.

- Tracers 1–5 each record a focused expected RED then GREEN. The recovery record identifies and resolves the original tracer-3 code-ahead-of-test breach before continuing.
- Current source matches the final checkpoint: command executes one `DirectAgentSubmitter.Submit`; `DirectSubmissionAdapter` validates then calls `InvocationFactory.Create(snapshot, *message, prompt, api.DIRECT, true)` once; `RuntimeService` forwards exactly once through `runtime.APIBridge`; composition returns one service for both participation and direct submission.
- Added an independent regression assertion, `TestDirectSubmissionAdapterDefersPersistenceToRuntimeDelivery`. It exercises the real direct adapter -> `RuntimeService` -> runtime path with a recording runtime runner/sink. It proves visible direct replies order `sink` before `AfterDelivery`, while sink failure and no-reply result in no post-delivery persistence. This was an audit regression test, not a defect remediation; it passed on its first execution because the runtime already owned the required behavior.

## Source/diff audit scope

Read and inspected:

- Composition/lifecycle: `cmd/zenbot/main.go`, `cmd/zenbot/live_agent_test.go`
- Command admission/conditional registration: `internal/command/handlers.go`, `handlers_test.go`, `dispatch_adapter.go`, `registry.go`
- Shared runtime/direct identity/delivery: `internal/agent/live/{direct_submission.go,direct_submission_test.go,service.go,service_test.go,participation.go,runner.go,tool_loop.go}`, `internal/agent/runtime/{runtime.go,runtime_test.go}`
- Protected listener/semantic paths: `internal/listener/message/handlers.go`, `internal/listener/user_joined_listener.go`, `internal/agent/participation/semantic_moderation.go`
- `git status --short`, full tracked `git diff --stat`/names, relevant diff content, untracked live files, and all live-agent handoffs.

Targeted source/diff searches covered `directAgentInvoker`, `DirectAgentInvoker`, `RuntimeService`, `DirectSubmissionAdapter`, `run_command`, `NewAgentCommandGateway`, `DispatchUserCommand`, moderation/DynamicSQL, raw send, SQL/H2, proxy/Whiskey, and credential-like literals.

Findings:

- `directAgentInvoker` / `DirectAgentInvoker`: zero production references.
- `l` is a `model.USER` command definition, but is registered only through `directLDefinition(submitter)`; nil submitter omits it.
- The direct command trims its prompt, calls the supplied submitter once, returns admission status, and has no send/raw/persistence/provider branch.
- The adapter does no nick/hash lookup or inference. It copies the trusted users slice and delegates capability/identity construction to `participation.InvocationFactory`.
- Enabled composition creates one runtime and one `RuntimeService`; participation `Submit` and `DirectSubmissionAdapter.Service` use it. Disabled composition returns pass participation with nil service/submitter before memory/provider/runtime construction.
- Shutdown calls `roomAgent.Close()` before host lifecycle stop, replica stop, and deferred DB/server teardown. Runtime close rejects late work and waits for cancellation.
- The fixed tool loop remains exactly `user_message_history`, `room_users`, `run_command`; semantic ingress remains literal `false`; default message chain and join ordering read back unchanged.
- Existing `NewAgentCommandGateway`, `SendRawMessage`, and H2 imports remain in broader pre-existing composition/command code, but the tracked diff adds no `run_command`, gateway, dispatch, DynamicSQL, raw-send, SQL/H2, proxy/Whiskey, or secret reachability for this vertical. No credential literal found.

## Verification commands and results

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

```text
$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterDefersPersistenceToRuntimeDelivery$' -count=1 -v
PASS: visible reply, sink failure, no reply

$ go test -race ./internal/agent/live -run '^TestDirectSubmissionAdapterDefersPersistenceToRuntimeDelivery$' -count=1
ok  zenbot/internal/agent/live  1.409s

$ go test ./internal/command -run 'TestDirectLCommand' -count=1
ok  zenbot/internal/command  0.439s

$ go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Delivery)' -count=1
ok  zenbot/internal/agent/live  0.266s
ok  zenbot/internal/agent/runtime  0.621s

$ go test ./internal/listener/message ./internal/listener -run 'Test.*(Chain|Participation|Joined)' -count=1
ok  zenbot/internal/listener/message  0.355s
ok  zenbot/internal/listener  0.665s

$ go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
ok  zenbot/cmd/zenbot  0.547s

$ go test -race ./internal/command ./internal/agent/live ./internal/agent/runtime ./internal/listener/message ./cmd/zenbot -count=1
PASS: all five packages (command 42.676s; live 1.526s; runtime 1.774s; listener/message 2.343s; cmd/zenbot 4.485s)

$ go test ./... -count=1
PASS: all packages, including internal/repository/h2 (40.961s)

$ go vet ./...
exit 0; no diagnostics

$ go build ./...
exit 0; no diagnostics

$ git diff --check
exit 0; no diagnostics
```

Read-back assertions also confirmed the exact default listener sequence, `SemanticModerationIngressReady() == false`, and active-user insertion before join automation.

## QA-owned touched paths

- `internal/agent/live/direct_submission_test.go` — added direct-path delivery/persistence-order regression coverage only; no production code changed.
- `.hermes/handoffs/live-agent-integration-qa.md` — this report.

No unrelated dirty file was reset, cleaned, checked out, staged, or committed.
