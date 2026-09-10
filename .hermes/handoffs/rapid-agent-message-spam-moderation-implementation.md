# Deterministic public-message anti-spam moderation implementation

## Scope completed

Implemented the bounded Saturn-shaped public-message moderation path without provider, tool, runtime-result, memory, delivery-agent, user-command-dispatch, or ban fallback behavior.

- `MessageMonitor` is mutex-owned, has an injected clock and trusted protected predicate, normalizes literal `\n`/LF/Unicode whitespace and lower-cases content, uses inclusive windows, prunes all expired identity state, clears a threshold queue, and emits at most one decision.
- Identity is stable `hash`, then `trip`, then normalized nick; blank sender identity is a no-op.
- Escalation is `WARN → MUTE → KICK → SHADOWBAN → none`; repeated first breach is `MUTE`, source-style. No `BAN` symbol/action is added.
- Excluded disabled/protected/whisper/blank/malformed-identity input mutates no monitor state.
- Typed operations use `encoding/json` serialization at the core boundary: warning fixed text, `{"cmd":"mute","nick":...}`, `{"cmd":"kick","nick":...}`, and existing typed shadow-ban persistence. They honor cancellation, make one call, and have no retry/dispatcher/raw agent payload/ban fallback.
- `MessageAutomation` executes monitor decisions outside its state lock with a two-second child deadline, contains/logs executor errors, and cannot alter pipeline decisions/submission.
- Factory composition fails closed for disabled/incomplete config or missing typed shadow-ban persistence. `cmd/zenbot` installs its observer only if that composition is complete, before participation filtering.

## TDD evidence

RED outputs observed before their matching implementations:

- `internal/agent/moderation/message_monitor_test.go`: `undefined: NewMessageMonitor`, `undefined: MessageConfig`.
- `internal/config/agent_config_message_moderation_test.go`: missing `AgentConfig` message moderation fields.
- `internal/core/moderation_test.go`: missing `WarnFlood`, `MutePrincipal`, `KickPrincipal`.
- `internal/agent/moderation/message_executor_test.go`: missing `NewMessageActionExecutor`.
- `internal/agent/live/message_automation_test.go`: missing `MessageAutomation`.

Focused GREEN results:

```text
go test ./internal/agent/moderation -run 'Test(MessageMonitor|MessageActionExecutor|EngineActionExecutor)' -count=1
ok zenbot/internal/agent/moderation

go test ./internal/agent/live -run 'Test(RoomParticipation|MessageAutomation)' -count=1
ok zenbot/internal/agent/live

go test ./internal/core -run 'Test.*(Moderation|Mute|Kick|Captcha|ShadowBan)' -count=1
ok zenbot/internal/core

go test ./internal/config -run TestAgentModerationRequiresMessageDetectionSettings -count=1
ok zenbot/internal/config
```

Protocol evidence is `TestAuthoritativeMessageModerationUsesSafeFixedProtocol`: JSON output validates fixed warning text and safely escaped mute/kick payloads.

## Verification

```text
go test -race ./internal/agent/moderation ./internal/agent/participation ./internal/agent/live -count=1
ok all three packages

go test ./... -count=1
PASS (including cmd/zenbot and repository/h2)
```

```text
go build ./...
PASS

git diff --check
PASS

go vet ./...
FAIL only with pre-existing informational copylocks warning:
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files changed for this vertical

- `internal/agent/moderation/message_monitor.go`, `message_monitor_test.go`
- `internal/agent/moderation/message_executor.go`, `message_executor_test.go`
- `internal/agent/live/message_automation.go`, `message_automation_test.go`
- `internal/core/engine_impl.go`, `internal/core/moderation_test.go`
- `internal/config/agent_config.go`, `agent_config_message_moderation_test.go`
- `internal/factory/engine_factory.go`, `engine_factory_test.go`
- `cmd/zenbot/main.go`, `config.example.toml`

## Boundaries and exclusions

No changes were made to Saturn source, migration plans, existing handoffs, repository schema/SQL, semantic `MODERATION` provider routing, tools, memory/evidence persistence, command gateway/authorization, retry workers, background queues, or user-command `Kick`/`Ban` behavior. Existing join automation remains separate and unchanged.
