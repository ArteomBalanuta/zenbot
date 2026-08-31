# Rapid agent parity after mandatory fresh history: bounded public `run_command`

## Decision

**Select Saturn's `run_command` agent tool, limited initially to its source-defined informational commands, plus the narrow engine-command gateway prerequisite.** This is the next highest-value unported live-agent vertical: it lets a public agent turn execute an already-existing Zenbot command through the same command boundary, have that command's actual room/whisper output delivered once, and give the model bounded execution evidence for its one allowed synthesis.

The accepted chain is retained: direct `l`; mention/ambient/AGENT relay; public H2 conversation context; one bounded tool call and at most two provider completions; durable turns; schema-validated durable read evidence; and mandatory fresh public history. This vertical adds one fixed **action** tool; it does not relaunch either accepted read tool or alter mandatory freshness.

### Why this is next

- [OBSERVED] Saturn explicitly exposes `RunCommandTool` as the bridge from the agent to selected Saturn commands; it is an ordered action because even weather/time can send a room message (`src/main/java/org/saturn/app/agent/tool/RunCommandTool.java:21-27`). Its baseline allowlist is the informational aliases `help,h,list,users,info,whois,lastseen,ping,p,weather,w,time,t,version,v` (lines 28-44).
- [OBSERVED] Zenbot already has concrete, registered public command paths for the useful overlap: `help`, `list`, `users`, `info`, `ping`, `weather`, `time`, and `version` are selected in `internal/command/handlers.go:newCommand` and registered from `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent`. A model cannot invoke them today because live composition freezes only `user_message_history` and `room_users` (`cmd/zenbot/main.go:newAgentToolLoop`; `internal/agent/live/tool_loop.go:NewBoundedToolLoop`).
- [OBSERVED] Saturn's engine gateway creates a synthetic command message using the trusted agent nick/trip/hash/whisper context, uses the normal user-command boundary, returns false for rejected/exceptional execution, and captures actual emitted chat for model data (`src/main/java/org/saturn/app/agent/tool/EngineSaturnCommandGateway.java:35-75`). Target inbound command dispatch currently has no callable result boundary: `internal/listener/message/handlers.go:DispatchUserCommand.Handle` authorizes then calls the legacy `cmd.Execute()` fire-and-forget path.
- [RECOMMENDED] Do this before dynamic database tools, per-command reflected tools, moderation, or corrective prose routing. It reuses registered behavior and exposes a narrowly bounded user-visible capability without SQL/schema work or a generic router.

### Rejected candidates

- `database_query`, `database_schema`, and `database_sql` need repository/schema execution composition plus H2 visibility/policy review; target only has `internal/agent/sql/policy.go` foundation.
- `SaturnCommandTool`/`SaturnCommandToolCatalog` require catalog-wide command parity and role/effect profiles. Saturn itself has that separate reflection-backed path (`src/main/java/org/saturn/app/agent/tool/SaturnCommandToolCatalog.java:22-45`); do not conflate it with the smaller generic `run_command` vertical.
- Response sanitizer/corrector and command-prose correction are separate router behavior and may add correction completions. They do not make command execution live.
- Moderation and permanent-ban command exposure are deliberately excluded even though Saturn's generic tool conditionally permits them (`RunCommandTool.java:45-47,190-209`).

## Evidence map

| Evidence | Verified observation / use in this vertical |
|---|---|
| `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java:21-222` | Source name, closed `{command,arguments}` input, lower-case allowlist check, ordered action metadata, gateway delegation, error on rejection, and source capability expansion. |
| `src/main/java/org/saturn/app/agent/tool/SaturnCommandGateway.java:5-44` | Gateway contract returns whether an already-validated command executed; default model data says exactly one command executed. |
| `src/main/java/org/saturn/app/agent/tool/EngineSaturnCommandGateway.java:35-75` | Trusted synthetic message + normal command boundary; capture sent messages only after accepted execution. |
| `src/main/java/org/saturn/app/agent/tool/execution/AgentToolExecutionPolicy.java:5-28` | Every non-read-only tool is a sequential action barrier, not an eligible parallel read. |
| `src/test/java/org/saturn/app/agent/tool/SaturnCommandToolTest.java:18-104` | Test-backed analogous command-tool requirements: non-read-only ordered contract, validated dispatch, capability denial, and rejected gateway error. |
| `internal/common/command_registry.go:11-89` | Target typed command definition/registry contract; aliases are lower-cased lookup keys. |
| `internal/command/handlers.go:22-76,311-387` | Target direct `l` delivery/persistence pattern and the concrete command factory used by the gateway. |
| `internal/command/dispatch_adapter.go:37-80` | Current registration limits which command definitions are concrete; gateway must not expose catalog placeholders. |
| `internal/listener/message/handlers.go:167-199` | Existing inbound ordering/authorization/dispatch; no suitable return/output-capture API exists yet. |
| `internal/agent/tool/contract/definition.go:11-140` | Target descriptor supports `Action`, `RoomDelivery`, capability requirements, timeout, schema validation, and resource declarations. |
| `internal/agent/tool/execution/execution.go:125-203` | Executor provides schema validation, caller capabilities, timeout/cancellation, ledger, and result-schema validation. |
| `internal/agent/live/tool_loop.go:22-306` | Current one-call/two-completion state machine; fresh-history branch is router-owned and must retain precedence. |
| `internal/agent/participation/invocation.go:32-55` | Room turns carry trusted caller identity/capabilities; direct invocation construction currently needs the same trusted context capability preservation as a bounded prerequisite. |
| `cmd/zenbot/main.go:58-73,81-156,163-209` | Shared live/direct composition currently creates exactly the two read tools and has the target engine/repository/directory dependencies. |
| `internal/agent/live/runner.go:39-113`, `internal/agent/live/direct.go:24-120`, `internal/agent/runtime/runtime.go:109-145` | Delivery precedes memory/evidence persistence; runtime serializes same memory key and honors shutdown cancellation. |

## Observed Saturn contract

### Tool contract

[OBSERVED] `run_command` is named exactly `run_command`; schema is a closed object with required string `command` and optional string `arguments` (`RunCommandTool.java:65-99,140-156`). The command is normalized with `Locale.ROOT` lower case; absent arguments become `""` after trimming (`RunCommandTool.java:161-181`). A command outside the context-specific allowlist returns an error and must not call the gateway.

[OBSERVED] The result mode is `ROOM_DELIVERY_AND_MODEL_DATA`, it is non-idempotent, and its effect is room message unless the broader source moderation capabilities are present (`RunCommandTool.java:107-131`). Source execution records successful gateway output as tool success; rejected execution becomes an error. The source gateway captures output emitted by the command and falls back to an explicit one-command execution statement only when there was no captured chat (`EngineSaturnCommandGateway.java:61-75`).

[OBSERVED] Source scheduler semantics are sequential: all actions are barriers, while only independent idempotent reads can run concurrently (`AgentToolExecutionPolicy.java:18-28`). This target already limits a live turn to a single call, so no parallelism/DAG design is required.

### Intentional rapid target adaptation

[RECOMMENDED] Start with the **concrete target overlap only**:

```text
help, h, list, users, info, ping, p, weather, w, time, t, version, v
```

Do not advertise `whois` or `lastseen`: those source aliases have no verified concrete Zenbot command in the cited registration/factory path. Preserve input aliases as source names but resolve them to a single verified Zenbot `CommandDefinition`; do not turn an unregistered catalog definition or a `saturnCommand` placeholder into a successful action.

[RECOMMENDED] The target descriptor is fixed public-user access in this vertical (no required capability), `Action`, `RoomDelivery`, non-idempotent, one positive bounded timeout (use 10 seconds to match Saturn catalog command timeout in `SaturnCommandToolCatalog.java:28-29`), a closed command/arguments schema, result schema `{messages: [string], deliveredCount: integer}`, resource writes `commands` and `room_delivery`, and explicit negative-use guidance. It must not be classified `Safe` by `execution.Safe`.

This is intentionally narrower than Saturn's conditional moderator/creator expansion, but is not blocked by that missing prerequisite: public informational command execution is useful and source-supported, while privileged action exposure remains a later high-risk vertical.

## Target gap

1. [OBSERVED] There is no `run_command` Go tool or `CommandGateway` interface under `internal/agent/tool/`.
2. [OBSERVED] `ToolLoop` accepts only the two read tools and lets regular model-selected calls use a generic single-call branch (`internal/agent/live/tool_loop.go:95-159`). It must recognize an action result as valid but must retain one call/two completions.
3. [OBSERVED] Target command registration and listener dispatch cannot expose status/error/output to the agent. `legacyAdapter` deliberately erases the command return in inbound handling (`internal/command/dispatch_adapter.go:12-35`).
4. [OBSERVED] Room participation can build trusted capabilities, but live conversion drops them: `live.Runner.apiContext` reconstructs `api.Context` with `api.NewContext` and direct construction uses `runtime.NewContext` (`internal/agent/live/runner.go:115-121`; `internal/agent/live/direct.go:41`). This does not block public commands, but the gateway must take trusted identity from invocation and preserve capability propagation as a contained prerequisite so a later privileged vertical cannot inherit an accidental empty/forged set.
5. [LIMITATION] Command output is currently delivered directly through `common.Engine.SendChatMessage`; no target equivalent of Saturn's `CommandOutputCapture` exists. A bounded request-local capture seam is required, not a transport rewrite.

## Proposed target design and file plan

### A. Bounded command execution seam (prerequisite, not a separate vertical)

**Create `internal/command/agent_gateway.go` and `internal/command/agent_gateway_test.go`.**

Define a narrow interface owned by `command`, not `live`:

```go
type AgentCommandGateway interface {
    Execute(ctx context.Context, caller agentapi.Context, command, arguments string) (CommandExecution, error)
}
type CommandExecution struct {
    Executed bool
    Messages []string // request-local copies of successfully accepted sends
}
```

Implement it with the existing `commandDefinitionFor`/`newCommand` construction, a synthetic `model.ChatMessage` populated solely from trusted `agentapi.Context` (room, nick, trip, hash, whisper), and the existing target `SaturnCommand.Execute(ctx)` contract. It must:

1. canonicalize only against the fixed overlap allowlist and resolve through `commandDefinitionFor`; reject unknown/unregistered/placeholder aliases before construction;
2. enforce the existing command role against `engine.GetActiveUserByName(caller.Nick())` and `engine.IsUserAuthorized`, matching `DispatchUserCommand.Handle` before `Execute`; never accept a provider-supplied role, nick, room, whisper flag, or prefix;
3. execute once with the invocation context; map `model.SUCCESSFUL` with no error to `Executed=true`; map denial, non-success status, error, or canceled context to a non-success result/error without fabricated model data;
4. use a **request-scoped engine decorator** that intercepts only `SendChatMessage` made during this command, delegates every successful send once to the real engine, and appends the sent text only after delegate success. It must not buffer, replay, reorder, suppress, or globally capture unrelated sends. All non-send `common.Engine` calls delegate unchanged.

The decorator is deliberately smaller than a transport change. Its captured messages form model data; they are not an extra user-visible agent response.

### B. `run_command` tool

**Create `internal/agent/tool/run_command.go` and `internal/agent/tool/run_command_test.go`.**

`RunCommand{Gateway command.AgentCommandGateway}` implements `tool.Tool`.

- Descriptor name/schema/effect/result metadata are exactly as the bounded adaptation above. `command` enum is the fixed sorted overlap; `arguments` is optional string with max length 4000 (the source per-command catalog documents that upper bound at `SaturnCommandToolCatalog.java:145-156`; use it here to bound command tail input).
- `Execute` parses through the existing executor-validated JSON, trims/lowercases command, verifies it remains in the tool's fixed allowlist, trims arguments, and calls the gateway once.
- Gateway `Executed=false` produces `COMMAND_REJECTED`; gateway context cancellation propagates as an execution failure so `Executor` produces its established cancellation/timeout envelope. A successful result is JSON matching `{messages,deliveredCount}`; if output is empty, use the source-shaped, bounded acknowledgement `Saturn command '<canonical>' executed; its output was sent to the room. No other Saturn command was executed.`
- Do not persist `run_command` as durable evidence: it is an action with request-specific delivered output, unlike the accepted durable read evidence.

### C. Frozen three-tool composition and single action turn

**Modify `internal/agent/live/tool_loop.go` and tests; modify `cmd/zenbot/main.go` and its focused test.**

Rename construction only if necessary for clarity, but keep the contract frozen: composition requires **exactly** `user_message_history`, `room_users`, and `run_command`, a fixed allowlist, `MaxSteps:2`, and `MaxToolCalls:1`. No external tool inventory/configuration or generic factory is introduced.

Regular public route:

```text
assemble -> completion #1 (three fixed public definitions)
  -> no call: existing finalization
  -> one model-selected call:
       schema/capability/ledger/timeout validation via Executor
       action run_command: real command sends once; capture model evidence
       append matching assistant/tool pair
       completion #2 (tools disabled)
  -> existing finalizer -> one ordinary agent delivery if non-marker
```

Mandatory fresh history has precedence. If `PreparedRequest.RequiredFreshTool()` is nonempty, retain `completeRequiredHistory` exactly: ignore first-model calls, execute only router-owned current-room history, then make one tools-disabled synthesis. `run_command` cannot consume or bypass that call.

`newAgentToolLoop` receives a `command.AgentCommandGateway`; `newLiveAgent` constructs an engine-backed gateway for the host engine. `directAgentInvoker` also receives/builds the same gateway so direct and live use exactly the same fixed tool registry. Keep disabled-agent short circuit before client/tool/gateway construction.

### D. Trusted context preservation

**Modify only the capability-preserving conversion seams: `internal/agent/live/runner.go`, `internal/agent/live/direct.go`, and focused tests.**

Add an internal `apiContext(inv)`/runtime constructor path that copies `inv.Context().Capabilities()` and moderation target unchanged into `api.NewContextWithCapabilities`. The direct `l` constructor has no `TrustedSnapshot`; its public informational descriptor does not need capabilities, so preserve an empty trusted set there. Do not infer privileges from prompt/model data or add command-mode capability policy in this slice.

## Semantics matrix

| Concern | Required behavior |
|---|---|
| Public mention / relay / ambient | `run_command` is visible only on ordinary public tool-enabled requests. The existing participation and relay admission/order stay unchanged. Ambient may execute it only if existing ambient admission reaches the loop; it remains one sequential action and then one synthesis. |
| Direct `l` | Non-whisper direct `l` gets the same three fixed definitions and command gateway. The direct command itself sends the captured command output through the target engine; `directLCommand` then sends only the final agent synthesis if nonblank. |
| Whisper | No tool definitions, no `run_command` execution, and no capture. A provider call is rejected before gateway access as existing `ToolLoop` behavior requires (`internal/agent/live/tool_loop.go:99-103`). |
| Authorization | Tool JSON has no caller/room/role fields. Gateway identity comes exclusively from `api.Context`; it reuses current command role authorization. Fixed informational aliases are public in this slice; no moderator/admin/permanent-ban expansion. |
| Delivery | Gateway-decorated command output delegates exactly once to the real engine and is captured after success. Completion #2 is normal agent output and may be silent by marker. No duplicate command output is reconstructed from captured text. |
| Cancellation / timeout | One parent context flows provider → executor → tool → gateway → command. Executor applies the positive descriptor timeout; cancellation before/during dispatch makes the call fail and prohibits completion #2. No retries, goroutines, queues, or late delivery are added. |
| Failure | Invalid schema/alias, unauthorized caller, unavailable gateway, command failure, send failure, timeout, or cancellation produces a tool error; model receives its envelope only if the ordinary loop's established error path permits synthesis. The command is never retried and no success acknowledgement is fabricated. |
| Persistence | Command side effects are completed at command execution. Existing conversation append remains only after final visible agent delivery. `run_command` yields no `PersistableEvidence`; command output/effects are never placed in durable tool evidence. Failed/canceled/marker-silent final turns append neither conversation nor tool evidence. |
| Ordering / concurrency / budget | Exactly one provider-selected tool call and max two completions. `run_command` is `Action`, non-idempotent, and an ordering barrier. There are no parallel calls, no batch, no correction/third completion, and no interaction with the mandatory fresh router-owned call. |

## RED → GREEN focused TDD

1. **RED gateway authorization/output:** add `internal/command/agent_gateway_test.go` using a recording engine. Prove a trusted synthetic `weather Tokyo` calls the real concrete handler once, sends once, and returns captured output; unknown/placeholder alias and denied role do not execute/send; command error/non-success/send error returns failure; canceled context does not execute. Run only this test and observe the missing gateway failure.
2. **GREEN gateway:** add the smallest decorator/gateway implementation. Re-run the focused gateway test.
3. **RED tool contract:** add `internal/agent/tool/run_command_test.go`. Assert exact closed schema/enum, `Action`, non-idempotence, `RoomDelivery`, positive timeout, result shape, normalization, one gateway call, rejection, malformed/extra argument rejection through executor, and no call after canceled context.
4. **GREEN tool:** implement `RunCommand`; run its focused tests and `internal/agent/tool/execution` tests to prove the existing executor enforces descriptor behavior.
5. **RED loop:** extend `internal/agent/live/tool_loop_test.go`. Script completion #1 with `run_command weather Tokyo`, then a plain completion #2. Assert exactly two provider requests, one command gateway call, one command send, one matching assistant/tool ID, tools omitted in completion #2, and no durable evidence. Add cases for failed action/cancel/timeout and prove no completion #2/retry. Add a mandatory-fresh request whose first response asks `run_command`; prove it executes only forced history and never the action.
6. **GREEN composition:** extend frozen loop construction to exact three tools and compose the same gateway in direct/live roots. Keep fixed bounds. Re-run focused loop tests.
7. **RED delivery/lifecycle:** extend `runner_test.go`, `direct_test.go`, and `cmd/zenbot/live_agent_test.go`: public visible completion persists only normal conversation after successful final delivery; command result is not durable; final sink/send failure and marker silence do not persist; whisper exposes no command; disabled agent constructs no gateway/provider/tool work.
8. **GREEN lifecycle/context:** preserve trusted capability conversion and wire one shared gateway; rerun focused tests.

## Standard vs high-risk routing

| Work | Route |
|---|---|
| Fixed command allowlist, schema, `RunCommand.Execute`, unit contract tests | Standard agent/tool implementation. |
| Narrow command gateway plus request-local engine send-capture decorator | **High risk:** command/runtime owner review. It touches real user-visible delivery and must prove no duplicate send or authorization bypass. |
| Extending frozen loop inventory from two read tools to three fixed tools while retaining one-call/two-completion/fresh precedence | **High risk:** live-agent state-machine review and focused cancellation/protocol replay. |
| Composition in `cmd/zenbot/main.go` and direct capability copy | Standard implementation followed by independent integration review. |

## Exact rapid-parity baseline gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only slice-owned files.

```sh
gofmt -w \
  internal/command/agent_gateway.go internal/command/agent_gateway_test.go \
  internal/agent/tool/run_command.go internal/agent/tool/run_command_test.go \
  internal/agent/live/tool_loop.go internal/agent/live/tool_loop_test.go \
  internal/agent/live/runner.go internal/agent/live/runner_test.go \
  internal/agent/live/direct.go internal/agent/live/direct_test.go \
  cmd/zenbot/main.go cmd/zenbot/live_agent_test.go

go test ./internal/command -run 'TestAgentCommandGateway' -count=1
go test ./internal/agent/tool ./internal/agent/tool/execution -run 'Test(RunCommand|Executor)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*(RunCommand|Fresh)|Runner.*RunCommand|Direct.*RunCommand)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/command ./internal/agent/tool ./internal/agent/tool/execution ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go vet ./...` and report its known unrelated current `internal/core/engine_impl.go` copylocks warning separately if it remains nonzero; do not broaden this rapid slice to repair it. Do not require race sweeps, broad SQL/migration revisions, transport rewrites, or an exhaustive command matrix unless a focused gate exposes a direct blocker.

## Deliberate exclusions

- No new database/query/schema/SQL tool, SQL policy change, H2 migration, repository work, or dynamic SQL capability.
- No `SaturnCommandTool` catalog reflection, generic command-tool registry, aliases absent from the verified concrete overlap, or placeholder command invocation.
- No moderation, protected-principal, captcha, ban/unban, admin, creator, or permanent-ban execution; no capability expansion based on provider input.
- No response sanitizer/corrector/prose guard, retries, fallback provider, third completion, multi-tool/batch/parallel execution, generic router, or tool configurability.
- No transport rewrite, global output capture, listener reordering, relay alteration, ambient policy change, durable action-evidence storage, or fresh-history change.
- No changes to protected `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, application Go code, or existing handoffs in this architecture task.

## Verification of this architecture artifact

- [OBSERVED] All evidence citations are repository-relative and resolve in the read-only Saturn or target checkout at authoring time. The four non-resolving paths named in the file plan (`internal/command/agent_gateway.go`, `internal/command/agent_gateway_test.go`, `internal/agent/tool/run_command.go`, and `internal/agent/tool/run_command_test.go`) are explicitly proposed creates, not evidence citations.
- [OBSERVED] This handoff is the only file created by this task: `.hermes/handoffs/rapid-agent-after-fresh-history-architecture.md`.
- [OBSERVED] No application Go source, `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, or existing handoff was edited by this task.
- [RECOMMENDED] An independent implementation reviewer must replay the listed focused action, cancellation, authorization, duplicate-delivery, fresh-precedence, and post-delivery persistence tests before accepting the implementation slice.
