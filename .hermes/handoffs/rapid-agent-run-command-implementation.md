# Bounded public `run_command` implementation

## Files

Created:
- `internal/agent/commandgateway/gateway.go`
- `internal/command/agent_gateway.go`
- `internal/command/agent_gateway_test.go`
- `internal/agent/tool/run_command.go`
- `internal/agent/tool/run_command_test.go`

Modified:
- `internal/agent/live/tool_loop.go`
- `internal/agent/live/tool_loop_test.go`
- `internal/agent/live/runner.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

## RED → GREEN evidence

Gateway RED:
```
# zenbot/internal/command [zenbot/internal/command.test]
internal/command/agent_gateway_test.go:32:17: undefined: NewAgentCommandGateway
internal/command/agent_gateway_test.go:48:19: undefined: NewAgentCommandGateway
internal/command/agent_gateway_test.go:59:17: undefined: NewAgentCommandGateway
FAIL zenbot/internal/command [build failed]
```
Green: `go test ./internal/command -run '^TestAgentCommandGateway' -count=1` passed.

Tool RED:
```
# zenbot/internal/agent/tool_test [zenbot/internal/agent/tool.test]
internal/agent/tool/run_command_test.go:21:23: undefined: agenttool.RunCommand
internal/agent/tool/run_command_test.go:31:28: undefined: agenttool.RunCommand
internal/agent/tool/run_command_test.go:34:27: undefined: agenttool.RunCommand
FAIL zenbot/internal/agent/tool [build failed]
```
Green: `go test ./internal/agent/tool -run '^TestRunCommand' -count=1` passed.

Additional loop test verifies one command call, a tools-disabled second completion, and zero durable evidence.

## Constraints preserved

- Fixed public aliases only: `help,h,list,users,info,ping,p,weather,w,time,t,version,v`.
- Tool JSON is closed and has only `command` and optional bounded `arguments`; trusted invocation context alone builds the synthetic message.
- Gateway reuses concrete command definitions and existing role authorization; aliases outside the fixed concrete overlap are rejected.
- Send capture is request-scoped. It delegates a real send once and records text only after that send succeeds. Failed sends are not captured.
- `run_command` is a non-idempotent `Action` with `RoomDelivery`, a 10-second timeout, one invocation path, and no durable evidence.
- Frozen public composition now has exactly history, room users, and run command. The fresh-history branch remains before provider-selected tool handling. Whisper definitions remain absent.
- Runtime capability/moderation-target conversion is preserved into agent API context.

## Verification

Passed:
- focused command/tool/cmd tests listed in the architecture handoff
- `go test ./... -count=1`
- `go build ./...`
- `git diff --check`

`go vet ./...` remains nonzero only for the existing unrelated copylock warning:
```
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions

No SQL/schema/config/migration/moderation/catalog reflection/generic router/transport rewrite/retry/multi-tool/third completion work. Protected `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` were not edited.
