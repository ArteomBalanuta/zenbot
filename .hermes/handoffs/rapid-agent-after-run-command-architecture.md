# Rapid agent parity after public `run_command`: bounded command-prose channel

## Decision

Select **Saturn’s `AgentCommandProseGuard` + `AgentCommandChannelPolicy` behavior, limited to the already accepted fixed public `run_command` tool**.

The vertical prevents a public agent from presenting an approved command as Markdown/code prose (for example, `` `weather Tokyo` ``) as though it had run. It forces the already-fixed `run_command` protocol or one tightly validated non-command fallback, without adding commands, tools, authority, persistence, SQL, or a third completion.

This is the next slice because public command delivery is now live but the target’s `ToolLoop` accepts a no-tool completion #1 and sends it directly to the finalizer (`internal/agent/live/tool_loop.go:99-102`). The source explicitly treats rendered Markdown command text as a channel violation and corrects it into either a structured `run_command` call or a non-command response (`src/main/java/org/saturn/app/agent/routing/AgentCommandChannelPolicy.java:96-123`). Thus this closes the immediately adjacent observable integrity gap before broader command exposure, database tools, moderation, or generic command catalog work.

### Why it beats alternatives

- **Not database tools:** `database_schema` is gated by `DYNAMIC_SQL` (`src/main/java/org/saturn/app/agent/tool/DatabaseSchemaTool.java:68-96`) and needs a read-only H2 metadata seam; `database_query` adds five query contracts and repository scope (`DatabaseQueryTool.java:94-147`). Both exceed this no-broad-SQL vertical.
- **Not reflected/per-command command tools:** Saturn’s `SaturnCommandToolCatalog` scans command packages and creates one tool per annotated handler (`src/main/java/org/saturn/app/agent/tool/SaturnCommandToolCatalog.java:52-105`). That is explicitly out of scope: no catalog/reflection or new command surface.
- **Not privileged/moderation extension:** source `RunCommandTool` conditionally adds moderation/permanent-ban aliases (`RunCommandTool.java:45-47,190-209`); the accepted target action deliberately remains fixed public informational aliases (`internal/agent/tool/run_command.go:60-67`).
- **Not quiet/ambient work:** target already contains the source-shaped quiet registry and applies quiet before ambient submission (`internal/agent/participation/policies.go:73-120`, `invocation.go:118-132`), so it is not the next missing slice.

## Evidence map

| Evidence | Observed contract or target gap |
|---|---|
| `src/main/java/org/saturn/app/agent/routing/AgentCommandProseGuard.java:17-68,77-149,159-195` | Source derives permitted aliases from the exposed `run_command` definition; detects only inline/fenced Markdown commands; validates matching structured calls and rejects malformed shapes. |
| `src/test/java/org/saturn/app/agent/routing/AgentCommandProseGuardTest.java:19-135` | Source tests exact positive/negative detection, unauthorized `kick`, malformed definitions, exact structured-call match, and closed `{command,arguments}` behavior. |
| `src/main/java/org/saturn/app/agent/routing/AgentCommandChannelPolicy.java:87-123,134-170,219-240` | Source replaces rendered command prose with exactly one matching `run_command` call or closed `{response:string}` non-command fallback; it rejects repeated/invalid correction output. |
| `src/test/java/org/saturn/app/agent/routing/AgentCommandChannelPolicyTest.java:29-102,125-191,194-295` | Test-backed source behavior for correction, fallback shape, mismatched/multiple tool calls, repeated prose, and no tool re-offer after a command state. |
| `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java:21-26,159-181` | A command is an ordered room-delivery action; capability/allowlist failure does not execute the gateway. |
| `internal/agent/tool/run_command.go:17-67` | Accepted target fixed public alias set, closed schema, action/room-delivery descriptor, 10-second timeout, and gateway-only execution. |
| `internal/agent/live/tool_loop.go:96-166` | Current public state machine: mandatory fresh history has precedence; ordinary provider-selected action gets one tool call then a tools-disabled completion #2; `run_command` failure has no synthesis/retry. |
| `internal/agent/live/tool_loop.go:246-280` | Composition is intentionally frozen to concrete history, room-users, and `run_command` tools, blocking substituted same-name tools. |
| `internal/agent/turn/policy.go:12-147` and `internal/agent/turn/state.go:21-110` | Target already has unused policy/state vocabulary for a guard and unverified action correction, but no concrete guard/corrector or `ToolLoop` integration. Treat it as foundation, not delivered behavior. |
| `internal/agent/live/runner.go:64-86,100-113`; `internal/agent/runtime/runtime.go:122-145`; `internal/agent/live/direct.go:63-120` | Normal reply delivery precedes conversation/evidence persistence; direct `l` persists only after its caller successfully delivers the returned completion. |
| `internal/agent/assemble/assemble.go:347-365` | Target already retains Saturn’s intent filtering behavior for `saturn_*` definitions, but accepted composition advertises no such reflected definitions. No change is required here. |

## Source contract and bounded target adaptation

### Observed source contract

1. The guard obtains allowed command aliases only from the caller-visible `run_command` definition, not a hard-coded secondary catalog (`AgentCommandProseGuard.java:45-68`).
2. It recognizes command text only in an inline-code span or a complete backtick/tilde fenced block. Plain prose that happens to mention weather is not a command (`AgentCommandProseGuardTest.java:19-33`).
3. The command token is the first whitespace-delimited token after optional command prefix; it must be an exposed alias (`AgentCommandProseGuard.java:176-195`).
4. If the first response renders an unexecuted command, source creates a correction request. The only valid response is exactly one matching `run_command` call or `respond_without_command` with exactly one string `response` property (`AgentCommandChannelPolicy.java:134-170,190-240`).
5. Source’s general policy can use additional provider completions after this correction to resolve a tool result. That broader state machine has higher budgets than the accepted target (`AgentCommandChannelPolicyTest.java:29-52`).

### Target gap

The accepted target limits each ordinary public turn to **one call and two completions** (`ToolLoopLimits`, `internal/agent/live/tool_loop.go:33`). Its successful normal `run_command` sequence already consumes both completions: completion #1 tool call → action → completion #2 tools-disabled synthesis. A no-tool command-prose completion #1 currently bypasses all channel enforcement and can be delivered/persisted as false execution.

### Required bounded adaptation

Preserve the source’s channel rule while preserving the accepted target ceiling:

```text
normal first response with Markdown command prose
  -> correction completion #2 with exactly:
       A. a structured matching run_command call, OR
       B. respond_without_command({"response": string})

A -> execute run_command once; command itself delivers; return a local no-reply completion
B -> return the validated response as the ordinary final agent response
```

This is intentionally narrower than Saturn’s general post-correction cycle. **No completion #3 is added.** For branch A, command delivery is the only visible response; a local no-reply completion is not provider output, not a retry, and not durable evidence. For B, the model has explicitly supplied a non-command reply in the closed fallback envelope.

If the structured command action fails, times out, is canceled, or returns an invalid result, terminate exactly as accepted `run_command` failure behavior does: no fallback completion, no retry, no fabricated command acknowledgement. This is a bounded incompatibility with source’s later corrective prose handling, required by the accepted fixed two-completion contract; record it in implementation/QA rather than silently claiming full source-loop equivalence.

## Target design and owned file plan

### A. Concrete command-prose guard — standard implementation

**Create `internal/agent/turn/command_prose_guard.go` and `internal/agent/turn/command_prose_guard_test.go`.**

Implement a concrete `turn.CommandProseGuard` that is constructed from the **actual frozen `run_command` contract definition**, not a second alias list. It must:

- parse the definition’s JSON schema and collect only string values in `properties.command.enum` for the definition named `run_command`;
- expose `FindCommand(content string) (string, bool)` using source-equivalent inline/backtick/tilde fenced recognition and first-token normalization;
- expose internal helpers for the channel coordinator only: exact matching of a `run_command` LLM call against detected lower-case alias and closed argument shape; and detection of recognized command prose in a final response;
- reject malformed tool definition/schema/call input by returning no match, never by granting execution;
- never recognize aliases absent from the advertised definition. In this slice that resolves to the accepted 13 aliases, not source-only `whois`/`lastseen` and never moderator aliases.

Do not use `turn.UnverifiedActionPolicy` as-is: it invokes a generic `ResponseCorrector` and does not itself enforce the accepted completion ceiling. Keep that dormant foundation unchanged unless a small interface extraction is required by the focused tests.

### B. Fixed correction channel — senior review required

**Create `internal/agent/live/command_channel.go` and `internal/agent/live/command_channel_test.go`; modify `internal/agent/live/tool_loop.go` and `internal/agent/live/tool_loop_test.go`.**

Add a private `commandChannel` owned by `live`, with no public registry/factory/configuration. Its inputs are the loop’s provider, current prepared messages, frozen run-command provider definition, concrete guard, invocation context, and one `turn.State`.

When completion #1 has no tool calls and the guard detects a command:

1. append the original assistant response plus a fixed correction user message that says the command was rendered, was not executed, and requires exactly one of the two declared structured outputs;
2. make completion #2 with only two definitions: the already advertised fixed `run_command` definition and a private `respond_without_command` definition with closed schema `{"response": string}`;
3. reject blank, truncated, multiple-call, unrelated-call, malformed JSON, fallback extra-property, blank fallback, and fallback that itself contains recognized Markdown command prose;
4. for matching `run_command`, execute through the existing `execution.Executor` and existing frozen registry/ledger. It must reserve the same single call, retain descriptor timeout/cancellation/schema/result validation, and record success/failure once;
5. on successful action, return a `Completion` representation that explicitly suppresses ordinary final delivery. It must carry no durable evidence. The command gateway’s request-scoped capture has already delivered output exactly once;
6. for `respond_without_command`, convert only its validated string into a no-tool `llm.LlmResponse`; normal finalization/delivery/persistence then continues;
7. for all invalid/error cases, return an error. The runtime’s existing failure delivery applies only to reply-required public/direct modes; do not send a partial agent response from this coordinator.

Minimal internal completion change: add an explicit `SuppressReply bool` to `live.Completion` (rather than smuggling a marker string through provider content). Update `Runner.Run` to translate that result into `runtime.NewResultWithEvidence(..., "", false, nil)` and update `DirectInvoker.InvokeCompletion` to return an empty `runtime.DirectCompletion` without calling the finalizer. This gives public relay/mention and `l` the same no-duplicate semantic.

### C. Integration ordering

Modify `ToolLoop.CompleteWithEvidenceAndHistorical` only at the ordinary no-tool branch:

```text
assemble completion #1
  -> required fresh history? existing completeRequiredHistory unchanged
  -> tool call? existing one-call path unchanged
  -> no calls + no Markdown command: existing final response unchanged
  -> no calls + Markdown command: bounded commandChannel correction path
```

The correction path is unavailable to whispers because they already receive no definitions and a provider tool call is rejected (`tool_loop.go:74-77,103-105`). For a whisper’s plain Markdown command prose, reject/suppress it locally rather than exposing command execution or correction tools; do not add a whisper exception.

After ordinary `run_command` succeeds and completion #2 is received, run the concrete guard on that tools-disabled final content. If it renders a recognized Markdown command, suppress the final agent response locally; do not call the provider again and do not repeat command delivery. This source-shaped no-duplicate guard is bounded to an action that has already succeeded. A normal text summary such as “I ran the weather command” remains valid because it is not an inline/fenced command form.

Do not change `completeRequiredHistory`: required fresh history remains router-owned and has precedence over any provider command prose or tool request.

### D. No composition expansion

`cmd/zenbot/main.go:newAgentToolLoop` must remain exactly the three accepted tools (`user_message_history`, `room_users`, `run_command`; lines 59-75). No tool/config additions, no `saturn_*` definitions, and no changes to `assemble.filterTools` are part of this vertical.

## Semantics matrix

| Concern | Required behavior |
|---|---|
| Public direct `l`, mention, relay, admitted ambient | Same frozen loop/guard. If correction selects a command, only the command’s actual room output is visible; no normal agent duplicate follows. Ambient still observes its existing no-reply semantics and must not gain extra scheduling. |
| Whisper | No correction definitions, no `run_command`, no command gateway/capture. Recognized command Markdown must not be delivered as claimed execution; return the normal error/suppression path without a second provider completion. |
| Authority | Guard derives aliases from provider-visible fixed contract only. It has no caller, role, capability, room, nick, trip, hash, or whisper JSON. Existing gateway remains the sole trusted identity and authorization boundary (`internal/command/agent_gateway.go:43-86`). |
| Visibility | No repository reads/writes and no tool-evidence storage are added. Existing command execution stays public/informational and its synthetic message retains trusted caller context. |
| Cancellation/timeout | Parent context flows to correction completion and executor. Cancellation before correction, during correction, or during command execution exits with no late send and no next completion. `run_command` retains its 10-second descriptor timeout. |
| Delivery | The source-like structured command branch sends only through the existing request-scoped gateway decorator; no replay from the captured result. Fallback branch sends exactly once through normal agent sink/direct command. Suppressed branch sends neither an agent result nor persistence. |
| Persistence | Structured command success has no `PersistableEvidence`; `SuppressReply` prevents conversation append. Valid fallback follows normal post-success delivery and may append conversation only after that delivery. Failure/cancel/invalid/prose-after-action appends neither conversation nor tool evidence. |
| One call/two completions | Normal action flow remains 1 call/2 completions. Command-prose correction uses 0 or 1 call and exactly 2 completions. Fresh history remains 1 router-owned call/2 completions. No branch adds a third completion, retries, batches, or parallel calls. |
| Concurrency/order | The existing runtime serializes same memory keys (`internal/agent/runtime/runtime.go:109-121`). Within a turn the correction is sequential; `run_command` remains a non-idempotent action barrier. No scheduler/DAG change. |

## RED → GREEN test plan

1. **RED guard contract.** Add `command_prose_guard_test.go` before implementation. Build a guard from the actual `tool.RunCommand{}.Descriptor(...)` definition. Prove inline/fenced backtick/tilde positive cases; plain prose, `List.of()`, source-only `whois`, moderator `kick`, malformed definition, missing enum, malformed JSON, uppercase/non-string command, extra property, and invalid `arguments` are rejected. Reproduce the focused source tests’ behavioral cases, not source implementation details.
2. **GREEN guard.** Implement only the concrete parser/validator. Run `go test ./internal/agent/turn -run 'Test.*CommandProseGuard' -count=1`.
3. **RED channel correction.** Script completion #1 as `` `weather Tokyo` `` and completion #2 as matching `run_command`. Assert exactly two provider requests; correction request advertises only `run_command` plus `respond_without_command`; one gateway call; one command send; no ordinary sink/direct send; no durable evidence; no third completion. Observe failure before coordinator integration.
4. **GREEN structured correction.** Add the smallest private channel coordinator and suppression completion plumbing. Re-run focused live tests.
5. **RED fallback and invalid protocol.** Cover valid closed `respond_without_command`; blank/object/null/extra response; unrelated tool; two calls; mismatched alias; raw invalid JSON; repeated Markdown command in fallback; provider truncation; and correction client error. Assert zero gateway/send for failures and exactly no extra completion.
6. **GREEN validation.** Implement closed fallback parser and guard-on-fallback. Re-run focused tests.
7. **RED existing-action final guard.** Normal completion #1 requests `run_command`, then completion #2 renders `` `weather Tokyo` ``. Assert command is delivered once, final agent output is suppressed, no persistence, and no completion #3. Also prove non-code acknowledgement remains deliverable/persistable under existing rules.
8. **GREEN final response guard.** Insert post-action check only after a successful `run_command`; preserve current behavior for history/room-users results and all non-command final content.
9. **RED mode/lifecycle matrix.** Add runner, direct, and main composition tests: public mention/direct correction branch; whisper sees no correction definitions/gateway; required-fresh first completion containing command prose still executes only forced history; cancellation before/during correction and during action has no extra send/persistence; failed action has no fallback/retry; disabled agent constructs no coordinator/provider/tool work.
10. **GREEN integration.** Wire the bounded coordinator into existing loop only and rerun the focused package tests.

## Senior vs standard routing

| Work | Route |
|---|---|
| Concrete Markdown guard/parser and its source-case tests | Standard agent/turn implementation review. |
| Private `respond_without_command` closed-schema parser and correction request construction | Standard implementation plus contract review; it must not escape into tool registry/configuration. |
| `ToolLoop` branch ordering, single-call/two-completion accounting, and final-action suppression | **Senior live-agent review required.** This is the only state-machine change and must replay fresh precedence, malformed provider outputs, and all completion counts. |
| `SuppressReply` propagation through `Runner`, runtime delivery, and `DirectInvoker` | **Senior command/runtime review required.** It affects exactly-once user-visible delivery and post-delivery persistence. |
| Command gateway, alias allowlist, authorization, engine decorator, listener ordering, transport | No changes; regression review only. |

## Rapid test gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only files intentionally changed by this slice.

```sh
gofmt -w \
  internal/agent/turn/command_prose_guard.go internal/agent/turn/command_prose_guard_test.go \
  internal/agent/live/command_channel.go internal/agent/live/command_channel_test.go \
  internal/agent/live/tool_loop.go internal/agent/live/tool_loop_test.go \
  internal/agent/live/runner.go internal/agent/live/runner_test.go \
  internal/agent/live/direct.go internal/agent/live/direct_test.go \
  cmd/zenbot/live_agent_test.go

go test ./internal/agent/turn -run 'Test.*(CommandProseGuard|Policy)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*(CommandProse|RunCommand|Fresh|Whisper)|CommandChannel|Runner.*CommandProse|Direct.*CommandProse)' -count=1
go test ./internal/agent/tool ./internal/agent/tool/execution ./internal/command -run 'Test(RunCommand|Executor|AgentCommandGateway)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/turn ./internal/agent/live ./internal/agent/tool ./internal/agent/tool/execution ./internal/command ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go vet ./...` as an informational gate. If it remains nonzero only for the pre-existing `internal/core/engine_impl.go:95:22` copylocks warning recorded by `rapid-agent-run-command-qa.md`, report it; do not repair it in this slice. Do not require race sweeps or broaden into SQL, schema, retries, transport, or catalog work unless a focused gate proves a direct blocker.

## Exclusions

- No dynamic database query/schema/SQL tools, H2 metadata/repository work, or SQL policy change.
- No privileged/admin/moderator/permanent-ban command exposure, moderation behavior, new capabilities, or provider-supplied authority.
- No reflected/generic command catalog, `saturn_*` tool, tool configurability, aliases outside accepted concrete public overlap, or command registration changes.
- No generic response sanitizer, broad prose classifier, provider fallback, retry, third completion, multi-tool/batch/parallel execution, or global correction framework.
- No gateway/engine capture rewrite, listener/relay/ambient ordering change, transport change, persistence schema change, or durable action evidence.
- No edit to `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, application source, or existing handoff during this architecture task.

## Artifact verification

- All material source references above resolve in the read-only Saturn checkout and target checkout at authoring time. Proposed Go files under `internal/agent/turn/` and `internal/agent/live/` are explicitly new target files, not claimed existing evidence.
- The only file created by this task is `.hermes/handoffs/rapid-agent-after-run-command-architecture.md`.
- The architecture’s deliberate source adaptation is explicit: source’s broader correction loop is not added because the accepted target invariant is one tool call and two completions maximum.
