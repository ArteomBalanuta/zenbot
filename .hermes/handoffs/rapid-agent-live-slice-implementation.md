# Rapid agent live slice — implementation handoff

## Delivered

- Added the narrow `command.DirectAgentInvoker` boundary and a concrete direct `l` command. It trims and forwards the post-command prompt, sends successful assistant text using the existing command reply path, and returns invoker failures without a success reply.
- Added `RegisterUserUtilitiesWithDirectAgent`; legacy `RegisterUserUtilities` keeps its previous behavior and does not expose `l` without an invoker.
- Added `internal/agent/live.DirectInvoker`, which builds a `runtime.DIRECT` invocation, uses the existing prompt/assemble foundation, and calls the existing OpenAI-compatible LLM adapter synchronously for the command response.
- Extended the application TOML model with `[agent]` (`config.AgentConfig`) and wired `cmd/zenbot/main.go` to resolve it, construct the OpenAI client/prompt catalog/assembler/direct invoker, and register `l` only after successful composition.
- Startup now fails clearly when `agent.enabled` is not true, instead of registering a nil/no-op `l` command. Enabled configuration still uses existing `AgentConfig` validation/defaults and `SATURN_AGENT_API_KEY` resolution.
- Left `RelayAgentMessage` and `AgentParticipation` unchanged no-ops. No persistence, tools, routing, remote-room, ambient, or moderation behavior was added.

## Focused baseline test

`TestDirectLCommandForwardsResponseAndInvokerFailure` in `internal/command/handlers_test.go` uses one table to verify:

1. `!l hello world` invokes the injected boundary exactly once with `hello world` and returns its text through normal chat response behavior.
2. An invoker error yields failed command status, preserves the error, and sends no success response.

The test was run red first and failed as expected because `directLDefinition` did not exist:

```text
internal/command/handlers_test.go:182:13: undefined: directLDefinition
FAIL    zenbot/internal/command [build failed]
```

## Files changed

- `cmd/zenbot/main.go`
- `internal/config/config.go`
- `internal/agent/live/direct.go` (new)
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/handlers_test.go`

## Verification

```text
gofmt -w cmd/zenbot/main.go internal/config/config.go internal/agent/live/direct.go internal/command/handlers.go internal/command/handlers_test.go internal/command/dispatch_adapter.go
go test ./internal/command -run '^TestDirectLCommandForwardsResponseAndInvokerFailure$' -count=1
ok      zenbot/internal/command  0.504s

go test ./internal/command -count=1
ok      zenbot/internal/command  5.467s

go test ./...
PASS: all listed packages; `internal/repository/h2` completed in 25.046s

git diff --check
PASS: no output, exit 0
```

## Configuration note

The repository's checked-in `config.toml` has no `[agent]` table, so running the application with it now intentionally exits before database/transport startup with `agent.enabled must be true to register the l command`. To enable the live path, deployment configuration must supply the pre-existing agent fields, for example `[agent] enabled = true`, a valid `endpoint`, `model`, and the configured API-key environment variable (default `SATURN_AGENT_API_KEY`) when its provider requires one. No secret/default provider behavior was invented.
