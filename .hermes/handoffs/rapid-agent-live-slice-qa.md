# Rapid direct `l` live-agent slice — baseline QA

## Verdict: ACCEPT

The scoped migration slice satisfies the required direct-command baseline. No concrete baseline regression was found, so no production-code fix was made.

## Scope audited

- `cmd/zenbot/main.go`
- `internal/config/config.go`
- `internal/agent/live/direct.go`
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/handlers_test.go`

## Observed behavior

- Composition resolves the existing agent configuration and constructs the OpenAI client, prompt catalog, assembler, and `live.DirectInvoker` before registering `l`; invalid, absent, or disabled agent configuration stops startup rather than exposing a nil/no-op command.
- The injected direct `l` handler strips the command token, rejects an empty prompt, invokes once, replies only with a successful result, and returns invocation errors without emitting a fabricated success response.
- `live.DirectInvoker` assembles the DIRECT runtime request before calling `Client.Complete`; provider execution is not invoked if assembly fails.
- `RelayAgentMessage` and `AgentParticipation` remain deferred; no relay/participation bridge was added by this slice.

## Required verification

```text
gofmt -w cmd/zenbot/main.go internal/config/config.go internal/agent/live/direct.go internal/command/handlers.go internal/command/dispatch_adapter.go internal/command/handlers_test.go
# exit 0

go test ./internal/command -run '^TestDirectLCommandForwardsResponseAndInvokerFailure$' -count=1
ok      zenbot/internal/command  0.449s

go test ./internal/command -count=1
ok      zenbot/internal/command  5.436s

go test ./...
?       zenbot/cmd/zenbot [no test files]
ok      zenbot/internal/agent/api (cached)
ok      zenbot/internal/agent/assemble (cached)
?       zenbot/internal/agent/live [no test files]
ok      zenbot/internal/agent/llm (cached)
ok      zenbot/internal/agent/llm/openai (cached)
?       zenbot/internal/agent/moderation [no test files]
ok      zenbot/internal/agent/participation (cached)
?       zenbot/internal/agent/persistence [no test files]
ok      zenbot/internal/agent/prompt (cached)
?       zenbot/internal/agent/room [no test files]
?       zenbot/internal/agent/routing [no test files]
ok      zenbot/internal/agent/runtime (cached)
ok      zenbot/internal/agent/sql (cached)
?       zenbot/internal/agent/tool [no test files]
ok      zenbot/internal/agent/tool/contract (cached)
ok      zenbot/internal/agent/tool/execution (cached)
ok      zenbot/internal/agent/turn (cached)
ok      zenbot/internal/command (cached)
?       zenbot/internal/common [no test files]
ok      zenbot/internal/config (cached)
ok      zenbot/internal/core (cached)
ok      zenbot/internal/factory (cached)
ok      zenbot/internal/listener (cached)
ok      zenbot/internal/listener/info (cached)
ok      zenbot/internal/listener/message (cached)
ok      zenbot/internal/listener/snapshot (cached)
ok      zenbot/internal/model (cached)
?       zenbot/internal/repository [no test files]
ok      zenbot/internal/repository/h2 (cached)
ok      zenbot/internal/service (cached)
?       zenbot/internal/testutil/h2fixture [no test files]
ok      zenbot/internal/transport (cached)
ok      zenbot/internal/util (cached)

git diff --check
# exit 0; no output
```

## Configuration prerequisite

`config.toml` has no `[agent]` table. With the new intended live-agent composition, the checked-in configuration exits at startup with `agent.enabled must be true to register the l command`. This is a deliberate migration prerequisite, not a baseline regression: the architecture handoff explicitly requires startup to fail rather than register a deceptively live `l` command, and the rapid migration plan requires agent construction from runtime configuration. A deployment enabling `l` must provide `[agent] enabled = true`, a valid endpoint and model, plus the configured API-key environment variable where required (default `SATURN_AGENT_API_KEY`).

## Files changed by QA

- Added `.hermes/handoffs/rapid-agent-live-slice-qa.md`.
- No production or test behavior was changed by QA; required `gofmt` was run on the six scoped Go files.

## Deferred scope

- `RelayAgentMessage` and `AgentParticipation` wiring.
- Agent routing/participation lifecycle, ambient replies, tools, memory/persistence, remote-room behavior, moderation, and broader Saturn parity.
- No SQL/H2 work was performed.
