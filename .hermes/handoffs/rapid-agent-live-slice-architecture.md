# Rapid agent live slice — architecture handoff

**Scope:** the smallest production-wired direct `l` invocation. This is intentionally a rapid baseline, not a Saturn-equivalence migration. The scope and deferrals follow `MIGRATION_PLAN.md:13-25`.

## Evidence and objective

| Evidence | Consequence for this slice |
|---|---|
| Saturn `src/main/java/org/saturn/app/command/impl/user/LUserCommandImpl.java` is the direct `l` command. | `l` is the one command to make live. |
| Saturn `src/main/java/org/saturn/app/listener/message/handler/RelayAgentMessageHandler.java` and `AgentParticipationHandler.java` are message-path agent bridges. | Preserve the participation bridge as the next wiring point; do not make relay-message handling part of the baseline. |
| Saturn `src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java` owns service-level agent behavior. | Reuse Zenbot's existing agent foundation rather than reproduce Saturn's service/persistence stack. |
| Zenbot `internal/command/handlers.go` contains no-op `RelayAgentMessage` and `AgentParticipation`; `l` is catalogued but has no dispatch case. | Replace only the `l` dispatch gap for the live baseline. Leave the two no-ops as an explicit, separately wired bridge. |
| Zenbot `cmd/zenbot/main.go` imports no agent packages. | Composition root wiring is required; command-only work is not live. |
| Zenbot already has private foundations in `internal/agent/runtime`, `assemble`, `prompt`, `llm/openai`, `participation`, and `routing`. | Compose these existing layers; do not introduce a second agent model, provider client, or routing system. |

## Proposed live path

```text
incoming direct `l <prompt>`
  -> internal/command/handlers.go (add the missing `l` case)
  -> injected/direct agent invocation boundary
  -> internal/agent/assemble
  -> internal/agent/prompt + internal/agent/runtime
  -> internal/agent/llm/openai
  -> command response transport
```

The command must pass the user's post-command text as the direct prompt and return the runtime's resulting assistant text through the normal command response path. It must not silently fall through, invoke the relay no-op, or manufacture an answer when configuration/provider execution fails.

## Exact target changes and boundaries

1. **`internal/command/handlers.go`**
   - Add the missing `l` dispatch case to the existing command switch/dispatch mechanism.
   - Replace the current no-op behavior for that direct command with an injected callable that invokes the assembled agent runtime and emits its response.
   - Keep `RelayAgentMessage` and `AgentParticipation` present but no-op in this slice. They are not substitutes for direct `l` invocation.
   - The command package depends on a narrow **direct-invocation interface** supplied by the composition root, not on OpenAI configuration/client types. The implementation must use the existing exported API at the `internal/agent/runtime` / `internal/agent/assemble` boundary; this handoff deliberately does not invent an interface signature because no signature was supplied as evidence.

2. **`cmd/zenbot/main.go`**
   - Import and construct the existing agent assembly/runtime stack.
   - Supply the resulting direct-invocation implementation to command-handler construction/registration.
   - Fail startup clearly if required agent/provider configuration is absent or invalid. Do not register a deceptively live `l` command backed by a nil/no-op invoker.

3. **Existing agent packages, reused without redesign**
   - `internal/agent/assemble`: composition of the runtime dependencies.
   - `internal/agent/prompt`: prompt construction used by the direct request.
   - `internal/agent/runtime`: execution boundary for the direct request.
   - `internal/agent/llm/openai`: configured provider adapter.
   - `internal/agent/participation` and `internal/agent/routing`: retain as the planned message/participation bridge dependencies; do not expand their behavior for this baseline.

No new public package, persistence model, tool registry, or alternate LLM abstraction is warranted for this slice.

## Direct source behavior to preserve

`LUserCommandImpl.java` is the source authority for direct `l`: explicit user command invocation reaches the agent service and returns the agent result to that command context. The Zenbot baseline preserves that observable shape: `l` is explicitly dispatchable, receives its direct argument as input, performs one runtime invocation, and returns either its result or an explicit failure. It does **not** claim parity for Saturn relay-driven agent triggering or participation lifecycle; those behaviors belong to `RelayAgentMessageHandler.java` and `AgentParticipationHandler.java`.

## Provider/configuration and failure behavior

**Prerequisites**
- The OpenAI-backed adapter in `internal/agent/llm/openai` must be constructible from the application's existing configuration.
- All runtime/assembly dependencies required by that adapter and the existing prompt/runtime layers must be available at startup.

**Failure contract**
- Missing or invalid provider configuration: fail composition/startup with a clear configuration error; do not leave `l` catalogued as a working command.
- Provider/runtime failure after startup: return the error through the existing command error/response convention; send no fabricated assistant text and do not panic the process.
- Empty/malformed direct argument: use the existing command parser/validation convention. If it has no defined convention, reject it before provider invocation with a clear usage error.

## Participation bridge (deferred but named)

The follow-on slice replaces the no-op `RelayAgentMessage` and `AgentParticipation` hooks in `internal/command/handlers.go` with an adapter that feeds inbound messages through `internal/agent/routing` and `internal/agent/participation`, then invokes the same assembled runtime used by `l`. That bridge must share the direct-invocation execution path, not create a second provider/runtime. It is excluded from this baseline because the requested rapid path is direct invocation, and Saturn identifies relay and participation as separate handlers.

## Excluded scope

- SQL/persistence and any historical/state migration.
- Agent tools and tool execution.
- Ambient/automatic agent replies.
- Moderation policy/guardrails.
- Remote-room behavior and relay transport parity.
- Exhaustive Saturn command/listener/service parity and hardening.

## Single baseline test plan

Add one focused test in the existing package that owns `internal/command/handlers.go` (use its established `*_test.go` location): construct handlers with a deterministic fake of the direct-invocation boundary, dispatch `l hello`, and assert exactly one invocation receives `hello` and its returned text reaches the command response. In the same test table, assert an invoker error is surfaced via the normal command error path and produces no success response. Then run the relevant package test and the required full baseline:

```sh
go test ./...
```

This test proves catalogue-to-dispatch-to-invoker behavior without a network provider. Provider configuration/startup behavior is integration wiring, not a reason to make unit tests call OpenAI.

## Complexity and acceptance

**Complexity:** small/low-risk wiring slice: one missing command case, one narrow injected boundary, composition-root construction, and one focused handler test. The principal risk is API mismatch with the existing private agent packages; resolve it by adapting at `cmd/zenbot/main.go`, not by duplicating their types.

**Done when:** `l` reaches the existing assembled runtime in a normal process; missing provider setup fails clearly; provider failures surface cleanly; the focused test passes; and `go test ./...` passes. Go sources, tests, and `MIGRATION_PLAN.md` are intentionally unchanged by this architecture handoff.
