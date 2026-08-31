# Next rapid live-agent vertical: mandatory fresh public user-history evidence

## Decision

**Select mandatory fresh-data enforcement for the already-live `user_message_history` tool.** For a narrow, source-recognized user-profile/history request, Zenbot must obtain a new public, current-room history result in the current invocation before it can deliver a synthesis. This is the next highest-value live parity vertical after durable evidence reuse.

This is a live correctness vertical, not another tool or persistence slice:

- The two public read-only tools and one-call/two-completion loop are live; durable evidence is now safely reusable but explicitly stale/untrusted (`internal/agent/live/tool_loop.go: CompleteWithEvidenceAndHistorical`; `internal/agent/assemble/assemble.go: RenderWithHistoricalEvidence`). Without this slice, a model can answer a fresh-profile request from durable evidence, recent context, or its first completion without invoking the live history tool.
- The target already computes a required fresh tool/nick during request assembly, but the live loop never consumes `PreparedRequest.RequiredFreshTool()` or `RequiredFreshNick()` (`internal/agent/assemble/assemble.go: AssembleWithHistoricalEvidence`, `internal/agent/live/tool_loop.go: CompleteWithEvidenceAndHistorical`). The target therefore has an inert parity seam, not a delivered behavior.
- Saturn makes this a router-owned invariant: `DefaultAgentRouter.routeInSession` obtains `requiredFreshTool`/`requiredFreshNick` from its assembler and calls `AgentFreshDataCoordinator.process` before normal response policies or delivery (`src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java:163-318`). For `user_message_history`, `loadRequiredHistory` constructs trusted arguments and executes the lookup itself; it does not depend on the model electing to call it (`AgentFreshDataCoordinator.java:85-122,178-218`).
- This adds no provider-visible tool, schema/query, command action, capability, moderation behavior, or new configuration. It reuses the accepted public/current-room history repository, tool descriptor, executor timeout/cancellation, finalizer, memory, durable-evidence post-delivery lifecycle, and direct/live composition.

### Rejected candidates

- **Response corrector/sanitizer:** important but presentation-only; it does not make a previously unenforced fresh-data guarantee live.
- **`database_query`, `database_schema`, or `database_sql`:** require H2 query/schema execution seams, policy-to-tool composition, capability/visibility review, and dynamic-data bounds. `internal/agent/sql/policy.go` is only policy foundation today.
- **Command gateway / `run_command`:** requires command catalog, authorization, output capture, room delivery, and side-effect audit; it is not a small safe vertical.
- **Moderation:** requires protected-principal and action-executor wiring, authorization, audit, and public safety semantics.
- **Agent config/persistence expansion:** durable memory/evidence are already live; no additional persistence is prerequisite for mandatory fresh read evidence.

## Source-grounded contract

### Saturn evidence

| Source owner | Observed contract to retain |
|---|---|
| `src/main/java/org/saturn/app/agent/turn/AgentFreshnessPolicy.java:98-141,151-278` | Identifies named-user profile/history/speech requests and explicit follow-ups; extracts one normalized nick. It does not turn general topics or room-presence questions into history reads. Source tests cover named forms, escaped underscores, offline targets/follow-ups, and false-positive exclusions (`src/test/java/org/saturn/app/agent/turn/AgentFreshnessPolicyTest.java:13-113`). |
| `src/main/java/org/saturn/app/agent/routing/AgentRequestAssembler.java` through `src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java:163-172` | Required tool/nick are decided from trusted invocation prompt/history before provider execution, not from model prose. |
| `src/main/java/org/saturn/app/agent/turn/AgentFreshDataCoordinator.java:85-122,178-218` | When the required tool is `user_message_history` and a nick is known, reserve one tool call, create a router-owned call with the extracted nick, execute it, append a valid assistant/tool pair, and request synthesis. A failed required lookup stops the route; it must not fall back to an old summary. |
| `src/main/java/org/saturn/app/agent/turn/AgentFreshDataPolicy.java:15-178` and `src/main/java/org/saturn/app/agent/turn/AgentFreshDataFinalValidator.java:30-39` | Fresh synthesis requires successful history evidence, nonblank content, no further tool calls, and must not simply repeat the immediately preceding assistant answer. |
| `src/test/java/org/saturn/app/agent/routing/DefaultAgentRouterTest.java:1333-1383,1421-1457,1880-1919` | Repeated old summaries are refreshed; a provider that does not call history still leads to one trusted history lookup and a second synthesis; lookup failure yields no stale fallback or memory append. |

### Intentional bounded Zenbot adaptation

Saturn has a general router with up to its configured multi-step budget and optional correction completions (`DefaultAgentRouter.java:188-297`). Zenbot’s accepted live contract is intentionally frozen at **one tool execution and at most two provider completions** (`internal/agent/live/tool_loop.go:33,78-156`). Preserve the substantive invariant while retaining that bound:

1. The first completion is still made with the existing frozen public definitions.
2. For a required public `user_message_history` lookup, Zenbot ignores any model-selected first-call choice and executes the one router-owned history call from the trusted required nick.
3. Zenbot makes exactly one tools-disabled synthesis completion with that fresh result.
4. If that synthesis is blank, truncated, contains calls, or exactly repeats the immediately preceding first-assistant content, fail closed. **Do not add Saturn’s third correction completion** in this vertical.

This is stricter than relying on a correction prompt and avoids allowing `room_users`, malformed arguments, an alternative nick, or a future action tool to consume the only fresh-data budget. It also stays within the accepted cost/cancellation boundary.

## Exact behavior

### Eligibility and trusted scope

A mandatory lookup applies only when all conditions hold:

1. `PreparedRequest.RequiredFreshTool()` is exactly `user_message_history` and `RequiredFreshNick()` is nonblank and valid after the source-shaped normalization.
2. The invocation is public (`!inv.Context().Whisper()`), its trusted room is nonblank, and the frozen registry contains/permits exactly the current `user_message_history` tool.
3. The existing tool descriptor remains `AccessUser`, `ReadOnly`, `ModelData`, idempotent, with no capability/prerequisite/writes, a positive timeout, and `ResourceReads: ["messages"]` (`internal/agent/tool/user_message_history.go:24-33`; `internal/agent/tool/execution/execution.go:146-200`).

The router-generated arguments are canonical JSON only:

```json
{"nick":"<RequiredFreshNick>"}
```

`room`, result limit, caller trip/hash, visibility, and any data-store identifier remain outside provider control. `UserMessageHistory.Execute` continues to derive room exclusively from trusted `api.Context`, uses its composition-set bound, and returns only public current-room rows (`internal/agent/tool/user_message_history.go:35-77`). The underlying repository remains the existing bound H2 query; this slice introduces no SQL or schema change.

### Freshness classification prerequisite

Replace the current duplicated/too-permissive parsing seam with one canonical source-shaped policy owner:

- Keep the public API `turn.FreshnessPolicy.Required(prompt, history, users) (tool, nick string, ok bool)`.
- Make `assemble.freshness` a thin delegation to it (or remove the duplicate helper and call it directly from `Assembler.AssembleWithHistoricalEvidence`). No second regex/parser is allowed.
- Bring `turn.FreshnessPolicy` to the observed Saturn cases: explicit `user named <nick>`, profile/describe/summarize/analyze, possessive message/history/profile, `who is` and speech/history forms only when their source conditions support a user target, Markdown-escaped underscores, one optional leading `@`, and explicit history follow-ups using the latest previous user prompt.
- Preserve the source false-positive boundary: general concepts such as Java/user experience/Rome/Shakespeare, generic status/build follow-ups, and `who is in <room>` do not require history. In particular, do not retain the current assembler shortcut that treats every token after `is` as a user (`internal/agent/assemble/assemble.go:400-425`).
- A classifier failure is **not** a reason to infer a nick from model output. It is normal non-fresh routing. A recognized fresh target with an invalid/blank nick is a construction error before the provider request, not a broad or guessed lookup.
- `MODERATION` remains excluded as current assembly already requires (`internal/agent/assemble/assemble.go:314-317`). Whisper invocations are excluded from mandatory lookup even if the pure classifier recognizes a phrase: current live policy intentionally advertises no tools to whispers and rejects attempted whisper calls (`internal/agent/live/tool_loop.go:70-73,96-100`). This is the required Zenbot privacy adaptation; do not use a public-history query to satisfy a whisper request.

### End-to-end route

```text
public direct l | public mention | public relay-backed room turn | ambient
  -> Runner / DirectInvoker load normal memory, historical durable evidence, current context
  -> ToolLoop.CompleteWithEvidenceAndHistorical
     -> Assembler returns trusted RequiredFreshTool/Nick plus initial provider request
     -> completion #1 with frozen [user_message_history, room_users] definitions
     -> required history?
          no: existing zero-or-one model-selected bounded call behavior
          yes:
            reject initial length response before any execution
            construct fresh-history-<request-id> call using RequiredFreshNick
            execute only user_message_history through existing Executor
            append assistant(first content + synthetic call) + tool(envelope, matching ID)
            -> completion #2 with tools=nil
            -> validate fresh evidence and fresh non-tool synthesis
  -> existing MarkerFinalizer
  -> visible delivery
  -> existing durable conversation append + eligible durable tool-evidence append
```

The first provider response is never itself delivered on the required-fresh path. If it requested `room_users`, another nick, an unknown tool, a batch, or malformed arguments, Zenbot must neither execute nor surface that model request. It uses only the trusted fresh-history call. The assistant protocol message must contain only the synthetic call so it has exactly one matching tool result; do not carry the rejected provider calls forward as dangling protocol state.

## Target design and ownership

### Stage A — canonical, source-shaped classification

**Files:**

- Modify `internal/agent/turn/freshness.go`
- Modify `internal/agent/assemble/assemble.go`
- Extend `internal/agent/turn/turn_test.go`
- Extend `internal/agent/assemble/assemble_test.go`

**Design:**

1. Port only the source-supported detection/extraction contract from `AgentFreshnessPolicy`; do not import Java regex text mechanically if Go’s regexp syntax differs. Keep Unicode nick rules (`\p{L}`, `\p{N}`, `_`, `-`) and the 100-rune bound.
2. Ensure the public pure policy gets all necessary inputs. The latest user prompt is read from the provided LLM history; provider/tool messages and the legacy internal-evidence prefix cannot become a follow-up target.
3. `Assembler.AssembleWithHistoricalEvidence` invokes this policy once after context/memory construction and stores the result in `PreparedRequest`. It must use the same result for direct and runtime paths because both already call the same `ToolLoop`.
4. Do not alter prompt ordering, historical-evidence injection, current-room context query, normal tool definitions, or `filterTools` behavior outside removing the duplicated freshness parser.

### Stage B — required-history branch inside the existing frozen loop

**Files:**

- Modify `internal/agent/live/tool_loop.go`
- Add/extend `internal/agent/live/tool_loop_test.go`
- Optionally add narrowly scoped helpers in `internal/agent/turn/freshness.go` or `internal/agent/turn/coordinator.go`; do not expose a second router or use `FreshDataCoordinator.Process` as an unreviewed parallel loop.

**Internal helper shape (recommended):**

```go
func (l ToolLoop) completeRequiredHistory(
    ctx context.Context,
    inv runtime.Invocation,
    agent api.Context,
    prepared assemble.PreparedRequest,
    first llm.LlmResponse,
    state *turn.State,
) (Completion, error)
```

It is private to `live`; it receives already-prepared trusted state, not a raw prompt, repository, provider-selected tool name, or callback.

Required algorithm:

1. Assemble and call completion #1 exactly as today. Preserve initial cancellation checks and reject `FinishReason == "length"` before any tool access.
2. If public `prepared.RequiredFreshTool()` is empty, use the accepted current branch unchanged.
3. If it is nonempty but not `user_message_history`, return a stable internal configuration error before tool execution. This slice supports exactly the source/current live requirement, not a generic forced-tool framework.
4. Validate `RequiredFreshNick()` with the canonical policy’s nick validator. Verify `Registry.Lookup("user_message_history")`, `Registry.Allowed`, descriptor integrity, and the existing read-only/no-capability/no-write contract before execution. If unavailable/invalid, return an error and do not fall back to the first answer.
5. Advance and reserve exactly one existing `turn.State` tool call. Construct a nonblank deterministic-per-request ID such as `fresh-history-<RequestID>`; construct arguments through `encoding/json`, not concatenation.
6. Execute via the existing request-local `execution.Executor`, its registry, and `execution.NewLedger(map[string]int{"user_message_history": 1}, 1)`. It retains validation, descriptor timeout, context propagation, result schema validation, and stable envelopes. Call `RecordToolSuccess`, `RecordSuccessfulTool`, and `RecordSuccessfulToolResult` only after a non-error validated result. A tool error/cancellation/timeout returns an invocation error immediately—there is no synthesis completion and no ordinary model-tool fallback.
7. Append `assistant(first.ContentNullable(), []LlmToolCall{syntheticCall})`, then the one `tool` message containing `result.Envelope()` and the exact synthetic ID. Do not use original first-response calls. Advance to step 2 and call the provider once with tools omitted.
8. After completion #2: return an error if canceled, `length`, any tool calls, or blank content. Then require the recorded successful `user_message_history` result and reject an exact trimmed repetition of the first assistant content. Reuse/refactor `turn.FinalValidator.ValidateWithHistory` only if its inputs can express this protocol precisely; do not invoke `FreshDataCoordinator.Process`, which can issue correction completions and violates this vertical’s bound.
9. Build `Completion{Response: second, DurableEvidence: []turn.PersistableEvidence{candidate}}` using existing `turn.NewPersistableEvidence(descriptor, result)`. This preserves the accepted durable-evidence rule: it is only appended after successful visible delivery by `Runner.AfterDelivery` or `DirectInvoker.PersistDelivery`.

The regular model-selected path must retain its frozen one-call invariant and all currently accepted behavior. Keep `NewBoundedToolLoop` frozen to exactly `user_message_history` and `room_users`; no generalized constructor or externally configurable mandatory-tool set belongs in this slice.

### Stage C — composition verification only

**Files:**

- Extend `internal/agent/live/runner_test.go`
- Extend `internal/agent/live/direct_test.go`
- Extend `cmd/zenbot/live_agent_test.go` only if a composition assertion is needed

No production composition rewrite is expected: `cmd/zenbot/main.go` already builds a single two-tool loop for both live and direct paths (`newAgentToolLoop:58-73`, `newLiveAgent:121-140`, `directAgentInvoker:204-208`). Verify that it remains shared and that disabled-agent construction remains before provider/tool-loop creation.

## Security, visibility, failure, output, and cancellation matrix

| Condition | Required behavior |
|---|---|
| Public recognized profile/history request | Exactly one trusted current-room public `user_message_history` execution; no final answer before successful fresh result. |
| First response has no tool call / claims it will use history | Ignore the prose, execute the trusted required lookup, make one tools-disabled synthesis completion. Saturn proves this (`DefaultAgentRouterTest.java:1421-1457`). |
| First response requests another tool, wrong nick, a batch, unknown tool, blank ID, or malformed args | Execute none of those calls. Execute only trusted required history once; do not expose model-selected scope/action. |
| Fresh tool success | Model sees a normal success envelope only in the matching tool message; JSON is never delivered directly to chat. Candidate evidence remains request-local until visible delivery. |
| Fresh history tool error, timeout, unavailable registry, invalid result, invalid required nick, or context cancellation | Fail closed before completion #2. Do not reuse first response, durable evidence, old memory, room snapshot, or synthetic empty success. No memory/evidence append. |
| Completion #2 is blank, truncated, contains a call, or repeats first answer | Fail closed; no third correction/completion. No delivery or post-delivery persistence. |
| Exact no-reply marker on valid fresh synthesis | Existing finalizer keeps it silent; therefore no conversation/evidence append. A required fresh lookup may have run, but it is never persisted because delivery did not occur. |
| Successful direct / mention / relay / visible ambient delivery | Existing delivery routes unchanged. Only after successful sink/send can existing memory and eligible fresh history evidence append. |
| Ambient error or required-fresh failure | Existing runtime behavior stays silent/log-only because ambient does not require a reply (`runtime.Mode.RequiresReply`, `internal/agent/runtime/runtime.go:122-145`). |
| Direct `l` required-fresh failure | Existing direct command error behavior; no send and no `PersistDelivery`. |
| Whisper | No definitions, no automatic or model-selected history lookup, no historical durable-tool evidence projection. A hallucinated call fails before repository access. |
| Runtime close / parent cancellation | Propagate one context into the executor and provider calls. Do not issue completion #2 after cancellation; runtime cancellation prevents late sink delivery. No goroutine, retry loop, queue, or background persistence is added. |

## TDD sequence

Work RED → GREEN in these increments; do not implement a broad router first.

1. **Classification RED — `internal/agent/turn/turn_test.go`:** table-drive all Saturn positive forms (explicit named, escaped underscore, possessive, speech/history, source-approved follow-up) and false positives (Java, user experience, Rome, Shakespeare, room presence, generic build/status follow-up). Assert exact tool/nick or no requirement.
2. **Assembly RED — `internal/agent/assemble/assemble_test.go`:** prove the assembled fresh fields come only from `turn.FreshnessPolicy`; `who is president` produces neither tool nor nick; a public recognized request does; moderation clears it; the same whisper request may remain classifiable internally but is not advertised/executed by the loop.
3. **Loop RED: automatic execution — `internal/agent/live/tool_loop_test.go`:** scripted completion #1 returns plain old summary/no calls. Assert exactly one history repository call using extracted trusted nick, exactly two provider calls, first request has the frozen definitions, second has no tools, and second message sequence has one synthetic assistant call and matching tool ID. Assert no `room_users` lookup.
4. **Loop RED: hostile model choice — same test:** completion #1 returns `room_users`, wrong history nick, batch, or malformed tool call. Assert zero execution of provider-selected calls, exactly one trusted history call, no dangling original call in synthesis messages, and one fresh synthesis only.
5. **Loop RED: fail closed — same test:** history repository error, descriptor/tool-result validation failure, deadline cancellation while blocked, missing registry tool, response #1 `length`, response #2 `length`, blank, a tool call, and repeated first content. Assert no final answer, no third provider call, and no room directory/alternate tool execution.
6. **Durability/lifecycle RED — `internal/agent/live/runner_test.go`, `internal/agent/live/direct_test.go`:** successful required-history synthesis produces exactly one persistable history candidate and it appends only after sink/send success. Send failure, marker silence, fresh failure, final invalidity, cancellation, and close append neither exchange nor tool evidence. Preserve the already accepted post-delivery contract.
7. **Composition RED — `cmd/zenbot/live_agent_test.go`:** enabled direct and live paths use the one existing two-tool loop and force history for a recognized public direct/room invocation; disabled configuration creates no provider, loop, or repository activity.

## Required verification

Run from `/Users/ab/workspace/go-projects/zenbot`. Format only slice-owned files and new/edited focused tests.

```sh
gofmt -w \
  internal/agent/turn/freshness.go internal/agent/turn/turn_test.go \
  internal/agent/assemble/assemble.go internal/agent/assemble/assemble_test.go \
  internal/agent/live/tool_loop.go internal/agent/live/tool_loop_test.go \
  internal/agent/live/runner_test.go internal/agent/live/direct_test.go \
  cmd/zenbot/live_agent_test.go

go test ./internal/agent/turn -run 'Test(HistoryNickAndFreshness|ParityFinalValidator)' -count=1
go test ./internal/agent/assemble -run 'Test(TruncateFreshnessBoundsAndCancellation|.*Fresh)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*Fresh|Runner.*Fresh|Direct.*Fresh)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test -race ./internal/agent/turn ./internal/agent/assemble ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Observe `go vet ./...` separately. The current dirty tree has a known unrelated copylocks warning in `internal/core/engine_impl.go`; do not weaken this slice or claim that existing warning is a freshness failure.

## Explicit exclusions

- No new tool, new tool definition, multi-tool/batch execution, parallel tool calls, retry, third completion, or general Saturn router.
- No database schema/query/SQL policy integration, H2 migration, command gateway, command execution, admin/moderation capability, protected-principal behavior, or agent moderation.
- No response-corrector/sanitizer/quote-only/truncation migration beyond the narrow required-fresh final validation specified here.
- No change to `room_users`, conversation context, durable evidence schema/storage/TTL, memory-key semantics, listener ordering, relay topology, ambient coalescing, or runtime admission.
- No config property or `MaxSteps`/`MaxTools` behavior expansion. The accepted live loop remains fixed at 2 steps/1 call regardless of broader source defaults.
- No protected-doc edits, commits, resets, or cleanup/reformatting of unrelated dirty files.

## Risks and routing

| Risk | Control |
|---|---|
| Stale hallucinated profile answer | Required route consumes only a successful current invocation history result; first answer is never delivered. |
| Cross-room/private leak | Router supplies only nick; tool retains trusted invocation room plus `PUBLIC` repository scope; whisper path is excluded. |
| Model attempts to divert to another tool/nick | Fresh branch ignores all model-selected initial calls and creates a single canonical trusted call. |
| Cost/recursion regression | Exactly one provider call before and one after one tool; no correction retry/third completion. |
| Invalid provider protocol | Synthetic assistant/tool pair has exactly one generated ID; final synthesis has no tools; test the pair explicitly. |
| Durable stale evidence is mistaken for fresh | Existing system prompt labels durable evidence stale/untrusted; fresh classification forces a new lookup and no successful delivery/persistence occurs on failure. |
| Dirty-tree damage | Restrict edits to the listed agent/test files plus this handoff; preserve all existing modifications. |

**Routing:** Stage A can be implemented by an agent-policy developer. Stage B requires senior agent/runtime review because it changes the shared live completion state machine and tool protocol pairing. Stage C requires an independent reviewer to replay the focused failure/cancellation and post-delivery persistence tests. Do not combine this vertical with database tools, command gateway, moderation, or general response correction.
