# Agent command cancellation implementation

## RED

Added `TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission` in `internal/command/handlers_test.go`. It uses the real default user-chat listener chain, normal command registration/authorization, a real `legacyAdapter` built from `directLDefinition`, a cancelled parent context, and a recording direct submitter. It asserts authorization runs once, no submit occurs, and no command output is emitted.

Placement is `internal/command/handlers_test.go` as prescribed. This test package can import `internal/listener` without an import cycle (`listener` does not import `command`); it also reuses the existing command engine fixture and recording submitter.

```sh
go test ./internal/command -run '^TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission$' -count=1
```

Output:

```text
# zenbot/internal/command [zenbot/internal/command.test]
internal/command/handlers_test.go:270:39: listener.NewUserChatListener(engine).NotifyContext undefined (type *listener.UserChatListener has no field or method NotifyContext)
FAIL	zenbot/internal/command [build failed]
FAIL
```

This is the expected feature-missing RED; no production cancellation bridge existed.

## GREEN

Implemented only the admission-phase bridge:

- `EngineImpl.DispatchMessage` remains background-compatible and delegates to `DispatchMessageContext`; `StartContext` now uses the context-aware dispatch entry point and only chat uses a local optional `NotifyContext` assertion.
- `UserChatListener.Notify` remains background-compatible and delegates to `NotifyContext`; `NotifyContext` creates and defers cancellation of a child scope only around synchronous chain processing.
- `DispatchUserCommand` runs unchanged authorization before an optional internal `ExecuteContext(context.Context)` route.
- `legacyAdapter.Execute` remains background-compatible; `ExecuteContext` shares the existing execution/logging body with a nil-context fallback.
- No runtime, direct submitter/adapter, agent service, command interface, listener order, Saturn source, or other excluded paths changed.

```sh
go test ./internal/command -run '^TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission$' -count=1
```

Output:

```text
ok  	zenbot/internal/command	0.522s
```

## Verification

```sh
gofmt -w internal/core/engine_impl.go internal/listener/user_chat_listener.go internal/listener/message/handlers.go internal/command/dispatch_adapter.go internal/command/handlers_test.go
go test ./internal/command -run 'Test(LiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission|DirectLCommand)' -count=1
go test ./internal/listener/message ./internal/listener -run 'Test(DefaultChainPlacesRelayBeforeAllDownstreamHandlers|DispatchUserCommandRejectsUnauthorizedPrincipal|.*UserChat.*)' -count=1
go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Cancellation)' -count=1
go test -race ./internal/command ./internal/listener/message ./internal/listener ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

All commands exited 0. The focused outputs were respectively `ok zenbot/internal/command`, `ok zenbot/internal/listener/message` and `ok zenbot/internal/listener`, and `ok zenbot/internal/agent/live` and `ok zenbot/internal/agent/runtime`; the race run passed all five listed packages. The full test suite passed, including `cmd/zenbot`, `internal/core`, command/listener/runtime packages, and repository H2 tests; build and diff check were clean.

Read-back confirms the default chain remains `ResolveUserMetadata → AuditChatMessage → IgnoreBotMessage → RelayAgentMessage → LogChatMessage → DeliverPendingMail → UpdateAfkState → YoutubePreview → CernEasterEgg → AgentParticipation → DispatchUserCommand`; authorization remains before the optional context execution branch; and `SemanticModerationIngressReady()` remains `false`. The implementation adds no cancellation context beyond admission: `DirectSubmissionAdapter`, runtime execution, and runtime ownership are untouched.
