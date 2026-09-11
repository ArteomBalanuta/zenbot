# Teardown & Rebuild: reference-informed tool loop

## Scope and source provenance

The question is whether other implementations improve Zenbot's model-owned execution,
not whether Zenbot can resemble them. Streaming and unrelated security mechanisms are
excluded. Reference repositories were read-only; no branch checkout, source modification,
upload, deployment or reference-repository tests were performed.

| Source | Revision analyzed | Important distinction |
| --- | --- | --- |
| Zenbot before changes | `20bf0dd1c6fb27f85423f980fc1b3740ed741793` | Clean master; single native model/tool loop. |
| ACE Kernel main | `bf25f426ddf85bc161fd4f5295a418b4fca08716` | May 26 main, read through Git; newer checked-out feature branch was not substituted. |
| ACE first pilot DSL main | `bfeb44d714f89848986f7b4ff48fb96b72ff18ca` | Application DSL, not a self-contained model runtime. |
| Saturn develop | `10a1ea3c2daecba168deac36f0e9a52a74cb19e5` | Committed implementation; dirty weather work excluded. |
| pastor | Current five Markdown documents | Kernel guides describe `1d2cab893a6c8a8a805e28726999fa027407e3c3`; Saturn guides describe the requested develop revision. |

Recursive Markdown discovery under pastor found exactly AGENT_IMPLEMENTATION_COMPARISON.md,
KERNEL_AGENT.md, KERNEL_AGENT_CLEAN.md, SATURN_AGENT.md and SATURN_AGENT_CLEAN.md.
Relevant runtime, contract, context, recovery, memory, history and completion sections
were read. Diagram/source indexes were used as navigation, not accepted as proof of code.
Streaming-specific mechanisms and unrelated room automation were deliberately excluded.

## Original Zenbot loop

`ToolLoop.CompleteWithEvidenceAndHistorical` creates a request-local registry, observation
store, retrieval tool, execution ledger and transcript. The assembler supplies current
policy, caller capabilities, original request and bounded historical context. The first
model response enters `TurnEngine.Complete`. Every valid native batch crosses descriptor,
argument, availability, prerequisite and execution checks. Compatible reads may overlap;
actions execute synchronously in model order. One correlated observation is returned for
every call. The model observes those results and chooses another batch or final prose.

Completion is structural, not semantic: no calls plus usable final content ends the loop.
A bounded protocol repair handles malformed/empty responses. Resource exhaustion reserves
a tools-disabled final synthesis. The ledger distinguishes known non-execution, committed,
partial and unknown effects, and blocks replay after uncertainty. Full observations stay
outside the disposable provider transcript and can be paged by call ID. The original
request and an execution receipt index are required context. Later provider/finalization
failures retain an `IncompleteTurnError` and an interruption record in memory.

This is already stronger than the references in several important areas. It does not
discard a legitimate new call merely because the model also wrote prose, does not force
history retrieval from keywords, does not cache away transient read retries, and does
not turn post-dispatch ambiguity into ordinary retry advice.

## What the references actually implement

### ACE Kernel main

Source anchors below are relative to Kernel at the pinned main revision.

- `internal/application/interpreter/agent_llm.go:154` selects an ordinary native tool
  loop or optional TCL execution. There is no newer native SDK planner on this main.
- `agent_tools.go:102` is a repeated model/tool loop, but `:145–157` discards new calls
  whenever prior cached results exist and the next response contains nonempty prose.
  “I found the record; now updating it” can therefore terminate before the update.
- `agent_tools.go:786–846` caches errors as well as successes and rejects identical
  arguments; `:180–207` forces completion on duplicate/limit-only batches. This obstructs
  legitimate refreshes and model-selected recovery. One ceiling mixes iterations/calls.
- `agent_llm_request.go:330–339` derives return-shape descriptions from declared schemas.
  This is a useful contract projection: the model learns what fields a tool can return.
- TCL accepts model-written bindings, projections, conditions and iteration; execution is
  deterministic interpretation of a model-authored program, not a rule-based planner.
  However `agent_tcl.go:251–269` can repair/replay a program after earlier effects without
  a structured committed/partial/unknown ledger. Successful programs return formatted
  output at `:272–289` without a fresh model assessment.
- Memory “extractive/recursive” summarization in `agent_memory.go:503–565` is character
  truncation with role prefixes. Context limits in `agent_llm_request.go:398–449` are
  initial construction work, not Zenbot's projection before every subsequent model call.

### ACE first pilot DSL main

- `agents/myagent.ace:19–35` configures a model and TCL mode; `functions/agent.ace:1114–1127`
  sends message, SDK payload and visible application state to the external Kernel.
- Action inventories ground IDs, options and parameter semantics in current application
  state (`functions/agent.ace:586–622`). Chat/UI actions share business implementations,
  e.g. title changes (`surfaces/chat/actions-title.ace:3–20`) and inventory adjustments
  (`surfaces/ccr-chat/ccr-chat.ace:639–647`). This is a sound producer-owned contract idea.
- It is not fully model-driven: `functions/agent.ace:1224–1252` intercepts help, work-order,
  material and visible-line prompts using application-specific predicates before the LLM.
- The CCR transcript workaround supplies eight recent turns truncated to their first
  200 bytes (`surfaces/ccr-chat/ccr-chat.ace:178–218`). Its tests at `:3410–3461` describe a
  real follow-up bug where changing type repeated a prior title action. The problem is
  valuable evidence; a second tiny transcript is not a stronger solution than canonical
  receipts and the existing observation store.
- Reply dedup uses live UI messages (`functions/agent.ace:1040–1095`), not verified effect
  receipts. Partial rejection handling avoids full replay (`ccr-chat.ace:896–919`), but
  the exception handler still tells users to try again (`:924–931`) without effect facts.

### Saturn develop

- `src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java:188–293` runs a native
  tool loop and preserves provider call/result order. Safe read fan-out is descriptor-led
  in `tool/execution/AgentToolCallScheduler.java:112–168` and `AgentToolExecutionPolicy`.
- An ordered freshness/unverified-action/command-channel policy chain surrounds that loop.
  `AgentCommandIntentPolicy.java:24–46` hides concrete tools unless the prompt starts with
  a command alias or run/execute. `AgentFreshDataCoordinator.java:193` creates a history
  tool call from a regex-extracted nick. These are deterministic task decisions.
- `tool/RoomUsersTool.java:117–126` returns source-derived `{room,users,count}` for a
  managed room. Zenbot already does this for its current-room tool; the useful extension
  is typed remote-list results through the existing list command, not another room route.
- `CommandOutputCapture.java` captures synchronous text with ThreadLocal storage; the
  gateway joins strings or manufactures generic delivery copy. `SaturnCommandTool.java:151`
  accepts result type any. This is weaker than Zenbot's verified command receipts.
- `AgentToolInvoker.java:37–46` cancels a future on timeout without proving effects stopped;
  failure removes the in-flight key (`AgentToolExecutionLedger.java:82`). An action may
  become eligible again after an ambiguous timeout. Keep Zenbot's stricter effect model.
- `AgentMessageProjector.java:64–101` evicts whole native protocol units but lacks a full
  observation store and can lose the original request after tool/correction messages.
  Strict finalization failures can hide all completed work behind a generic failure.

### pastor design/reference documentation

The comparison recommends evidence-preserving final recovery, ambiguous-effect handling,
executed-action memory, a unified correction ladder and provider abstraction only when
needed. These are suggestions, not benchmarks or mandatory architecture.

The useful underlying principle is that successful or uncertain effects remain facts even
when presentation fails. Zenbot already retained such facts internally; its user-facing
failure path did not expose their operational status. The guides also highlight short
follow-up referents and keeping the current request salient. Zenbot's canonical context
is preferable to a second planner envelope or a fixed-size competing transcript.

## Documentation versus implementation discrepancies

| Claim/source | Actual evidence | Decision |
| --- | --- | --- |
| pastor Kernel guides describe native planner, deterministic SDK lanes and recent action ledger | Guides pin a newer feature SHA, not requested main | Separate branch evidence; do not credit main with later mechanisms. |
| Kernel TCL spec diagrams result returning to model (`docs/tcl-specification.md:58–69,1213–1220`) | Successful integration directly formats/returns at `agent_tcl.go:272–289` | Do not confuse program termination with model-verified task completion. |
| TCL spec requires identical-call caching (`:1232`) | TCL executor directly dispatches at `runtime/executor.go:538–609` | Do not assume native-loop cache protects program replay. |
| TCL return types are critical to field routing (`:1096–1101`) | Integration never assigns TDL ReturnType (`agent_tcl.go:297–331`) | Prefer the actual native return-schema projection mechanism. |
| DSL language docs show token/tool/parallel limits and summarization | App main has max_messages 20 and chat timeout 60s; later size/budget commits a6b3df9 and 9d427f6 are not main ancestors | Separate language capabilities, configured app behavior and unmerged history. |
| DSL prompt grounding removes possibility of ungrounded output | Prompt instructions are not enforcement or empirical proof | Test model behavior; do not claim guarantees. |
| Saturn “strict schemas” | Provider definition has no strict flag; result validator checks root type/required presence, not full nested types | Preserve Zenbot's stronger actual validator. |
| Saturn model controls intent decomposition | Regex routing and router-created mandatory history calls also choose work | Reject those policies under the user's LLM-agency requirement. |
| pastor favors Saturn overall | Qualitative weights, different Kernel branch, not measured Zenbot comparison | Use individual mechanisms, not the verdict. |

## Pattern decision matrix

Every row distinguishes the problem, implemented/documented mechanism, ownership, current
coverage, incremental benefit and cost. A rejection is deliberate, not an unexamined omission.

| Pattern and source | Problem/mechanism/ownership | Zenbot coverage and relative merit | Decision, gain and cost |
| --- | --- | --- | --- |
| Return-shape hints, Kernel native (implemented) | Expose declared output fields before invocation; SDK describes facts, model decides use | Existing result schema validated locally but hidden in provider projection | Adopt bounded generated hints; clearer follow-up reasoning at modest manifest cost. No second schema owner. |
| Typed domain observations, Saturn roster/DSL action contracts (implemented) | Return actual source data separately from presentation | Current-room tool already stronger; remote list loses structure | Extend existing list operation/gateway only; exact counts and subject, extra optional payload field. |
| Shared UI/chat business operations, DSL (implemented) | One canonical validator/persistence path | Existing SaturnCommand already delegates to command runtime | Preserve; do not fork weather/list/moderation services for agent use. |
| Evidence-preserving failure output, pastor (proposal), DSL partial failure (implemented weaker) | Completed effects remain facts if final prose fails | IncompleteTurnError persists receipts, but failure sink discards their user-facing value | Adopt receipt-only incomplete notice, not deterministic answer synthesis. Bounded output and no raw result disclosure. |
| Protected correction and bounded value index (original adaptation) | Context pruning removes repair instructions or all factual values before terminal no-tools answer | Original request/receipts retained; data only in optional protocol transcript | Required current feedback plus bounded data/error previews. Extra context cost buys usable facts; full payloads stay retrievable during active turns. |
| Safe read concurrency, Saturn (implemented) | Overlap independent reads without reordering actions | Already implemented with effects/resources and source-order observations | Preserve; no new scheduler or global parallel switch. |
| Structured coded errors, Saturn/TCL (implemented) | Let model distinguish failures and revise work | Zenbot already has richer receipt/error/related-call metadata | Preserve; no retry strategy chosen by application. |
| Unknown-effect handling, newer Kernel/pastor (implemented on other branch/documented) | Avoid duplicate side effects after uncertain dispatch | Already stronger than requested main and Saturn; synchronous actions and unknown latch | Preserve non-retryable unknown; do not copy timeout-as-retryable behavior. |
| Native planner/deterministic lanes, newer Kernel/pastor (not main) | Reduce round trips by plan/validate/dispatch | Conflicts with iterative model revision and user constraints | Reject; no new lanes or frozen obligations. |
| Prose-plus-calls early stop, Kernel main (implemented) | Suppress repetitive preambles | Can suppress legitimate dependent action | Reject; only structural completion remains. |
| TCL, Kernel main (implemented) | Model-authored composition passes bindings directly | Existing result handles solve raw-context pressure; no measured need for new language | Defer; parser/runtime/prompt and partial-replay costs unjustified. |
| Suspend/resume, Kernel TCL (implemented) | Resume user-interaction programs | Zenbot has no durable program workflow requirement | Defer; needs versioned durable state and replay semantics, not a small loop enhancement. |
| Duplicate-read caching/forced completion, Kernel (implemented) | Bound repeated work | Can suppress valid refresh or recovery; Zenbot separates action replay from reads | Reject broad caching. Keep configured resource bounds. |
| Freshness regex/command-intent filter, Saturn/DSL (implemented) | Force recognized task routes | Narrow English/task shapes override model selection | Reject; contracts and model-visible current observations remain the mechanism. |
| Dynamic registry cache, Saturn (implemented; history 90edd05) | Runtime tool replacement and definition reuse | Static request-scoped inventory does not need it | Defer; consistency/cache lifecycle adds cost without demonstrated gain. |
| Tiny transcript/synthetic summarization, DSL/Kernel (implemented) | Fit history and preserve short follow-ups | Prefix truncation can remove referents; canonical atomic context already better | Reject a second transcript. Keep exact current request and factual history. |
| Auto knowledge graph extraction, Kernel TCL (implemented) | Infer entities from results using field-name rules | Domain-specific reasoning outside model | Reject; factual generic index is simpler and respects agency. |
| Provider factory/new provider, Kernel/pastor | Backend reach/fallback | Existing provider-neutral client is sufficient for one configured endpoint | Defer until a real provider requirement; no proven accuracy gain here. |

## Teardown findings and rebuild boundaries

### 1. Presentation-only remote facts — High

Trace: model calls saturn_list for two rooms → source snapshot contains actual users →
ListRoomOperation renders a chat string → gateway retains only that string and receipts →
model must recount presentation rows, including formatting/identity subtleties → subsequent
calculation or condition can be wrong despite successful tool execution.

Fix: preserve typed roster, subject and exact count from the operation through the command
gateway and declared SDK result. Existing list command still sends its normal chat output.
No router decides which room to query or how to combine counts.

### 2. Locally enforced but model-invisible output contract — Medium

Trace: SDK knows tool result fields → provider projection omits them → model selects a tool
without knowing its result shape → dependent reasoning guesses field names/available facts.
Fix: generate a compact return-shape hint from the same schema, with explicit bounds and
optionality. This improves information, not semantic routing enforcement.

### 3. Evictable repair instruction — High

Trace: malformed native response → loop appends correction as ordinary user context →
higher-priority observation units consume remaining space → correction is removed → model
sees old observations and status but not why its last call was rejected → repeat failure can
exhaust the single structural correction. Fix: pass current bounded feedback as required
runtime context by identity, alongside current status; keep original request intact.

### 4. Receipt-only terminal evidence — High

Trace: three or more calls produce observations → atomic transcript units do not fit →
required index says each call succeeded but contains no values → tool budget becomes zero →
read_tool_result disappears → model cannot retrieve counts needed for the requested final
calculation. Fix: preserve bounded structured data and error previews in the factual index,
shrinking previews before receipts. Explicit truncation prevents false completeness.

### 5. Completed effects hidden behind generic failure — High

Trace: tool commits action → later provider/finalization fails → IncompleteTurnError keeps
receipt internally → production failure sink emits only generic failure → user cannot tell
whether retrying duplicates an action. Fix: bounded receipt-based incomplete-turn notice,
distinguishing committed/partial/unknown/not-started. It is not a successful answer or task
completion decision; normal synthesis stays with the LLM.

## Intent retention versus semantic correctness

The original-request and 3+ call projection tests prove exact request text and factual
execution evidence survive. They do not prove the model obeys every condition, computes
correctly or reports all obligations. Previous provider trials already showed a model can
execute an incorrect conditional action despite receiving the original request/results.
No replacement deterministic planner is introduced to hide that model limitation.

Final validation, actual changes and measured outcomes are recorded after implementation
in the companion provider evaluation and the completion section below.
