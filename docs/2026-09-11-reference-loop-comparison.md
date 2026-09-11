# Teardown & Rebuild: reference-informed tool loop

## Scope and source provenance

This report covers Zenbot's model-owned execution loop and its Saturn reference.
Streaming and unrelated security mechanisms are excluded. Reference code was read-only;
no branch checkout, source modification, upload, deployment or reference-repository
tests were performed.

| Source | Revision analyzed | Important distinction |
| --- | --- | --- |
| Zenbot before changes | `20bf0dd1c6fb27f85423f980fc1b3740ed741793` | Clean master; single native model/tool loop. |
| Saturn develop | `10a1ea3c2daecba168deac36f0e9a52a74cb19e5` | Committed implementation; dirty weather work excluded. |

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

The original loop already preserves several important guarantees. It does not
discard a legitimate new call merely because the model also wrote prose, does not force
history retrieval from keywords, does not cache away transient read retries, and does
not turn post-dispatch ambiguity into ordinary retry advice.

## Reference implementation

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

## Documentation versus implementation discrepancies

| Claim/source | Actual evidence | Decision |
| --- | --- | --- |
| Saturn “strict schemas” | Provider definition has no strict flag; result validator checks root type/required presence, not full nested types | Preserve Zenbot's stronger actual validator. |
| Saturn model controls intent decomposition | Regex routing and router-created mandatory history calls also choose work | Reject those policies under the user's LLM-agency requirement. |

## Pattern decision matrix

Every row distinguishes the problem, implemented/documented mechanism, ownership, current
coverage, incremental benefit and cost. A rejection is deliberate, not an unexamined omission.

| Pattern and source | Problem/mechanism/ownership | Zenbot coverage and relative merit | Decision, gain and cost |
| --- | --- | --- | --- |
| Schema-derived return-shape hints | Expose declared output fields before invocation; SDK describes facts, model decides use | Existing result schema validated locally but hidden in provider projection | Adopt bounded generated hints; output fields become visible at a measured +22.39% base manifest cost. No second schema owner. |
| Typed domain observations, Saturn roster (implemented) | Return actual source data separately from presentation | Current-room tool already stronger; remote list loses structure | Extend existing list operation/gateway only; exact counts and subject, extra optional payload field. |
| Shared chat/tool business operations, Zenbot (implemented) | One canonical validator/persistence path | Existing SaturnCommand already delegates to command runtime | Preserve; do not fork weather/list/moderation services for agent use. |
| Evidence-preserving failure output | Completed effects remain facts if final prose fails | IncompleteTurnError persists receipts, but failure sink discards their user-facing value | Adopt receipt-only incomplete notice, not deterministic answer synthesis. Bounded output and no raw result disclosure. |
| Protected correction and bounded value index (original adaptation) | Context pruning removes repair instructions or all factual values before terminal no-tools answer | Original request/receipts retained; data only in optional protocol transcript | Required current feedback plus bounded data/error previews. Extra context cost buys usable facts; full payloads stay retrievable during active turns. |
| Safe read concurrency, Saturn (implemented) | Overlap independent reads without reordering actions | Already implemented with effects/resources and source-order observations | Preserve; no new scheduler or global parallel switch. |
| Structured coded errors, Saturn (implemented) | Let model distinguish failures and revise work | Zenbot already has richer receipt/error/related-call metadata | Preserve; no retry strategy chosen by application. |
| Unknown-effect handling, Zenbot (implemented) | Avoid duplicate side effects after uncertain dispatch | Synchronous actions and an unknown-outcome latch already protect the request | Preserve non-retryable unknown; do not copy timeout-as-retryable behavior. |
| Deterministic task planning | Reduce round trips by plan/validate/dispatch | Conflicts with iterative model revision and user constraints | Reject; no new lanes or frozen obligations. |
| Freshness regex/command-intent filter, Saturn (implemented) | Force recognized task routes | Narrow English/task shapes override model selection | Reject; contracts and model-visible current observations remain the mechanism. |
| Dynamic registry cache, Saturn (implemented; history 90edd05) | Runtime tool replacement and definition reuse | Static request-scoped inventory does not need it | Defer; consistency/cache lifecycle adds cost without demonstrated gain. |

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

## Implemented rebuild and independent review

All four selected changes are implemented, not recommendations:

- Source-owned list data flows through snapshot completion, gateway and SDK payload with
  copied JSON ownership. Source identity deduplication, stable roster ordering, empty zero
  counts and the existing room message are preserved. Only list advertises roster fields.
- Provider definitions derive sorted, optionality-aware shallow return hints from the
  authoritative result schema, bounded to 512 UTF-8-safe bytes. Callable schemas are unchanged.
- Current repair feedback is explicitly required runtime context, with a 1,536-byte
  aggregate rejected-batch diagnostic. The required factual index carries bounded data/error
  previews; generic recursive object projection preserves small nested values beside large
  fields. Preview budgets shrink before receipts; full retained results remain unchanged.
- Failure delivery now unwraps `IncompleteTurnError` and renders a receipt-only incomplete
  notice bounded to 2,000 runes. Tool outcome and effect state are independent, so
  `rejected + partial` and `failed + committed` remain visible. No raw results/arguments,
  inferred answer, semantic success claim or full-replay recommendation is emitted.

Independent review covered each task and the final integration. It and root inspection
found and corrected two integration defects before handoff: the initial shared result schema
advertised list fields for unrelated commands, and the first failure renderer conflated
command rejection with absence of effects. Both have explicit regression coverage. No
unresolved implementation findings remained in the reviewed code.

Two extra freshness-guidance experiments were deliberately removed after provider trials
did not demonstrate a benefit. The final change does not accumulate prompts or error
instructions merely because they sound plausible. The observed model failures remain
explicit limitations, not justification for a hidden deterministic planner.

## Validation and before/after assessment

Baseline `go test ./...` passed. New RED tests reproduced missing source data, mutable
callback data aliasing, misleading per-command result schemas, evicted current feedback,
lost terminal count/error facts, loss of nested counts beside a large roster, and masked
partial/unknown/committed failure effects. Corresponding GREEN tests exercise the actual
operation/gateway, contract, projector, loop and production failure-sink seams.

Final verification included the full uncached Go suite, `go vet ./...`, affected agent,
command and snapshot race tests, and a fresh race run for final live/tool/composition
changes. All passed. macOS emitted non-fatal LC_DYSYMTAB linker warnings during race builds.
One pre-existing coordinator timing assertion failed during an intermediate worker run:
it observes TIMED_OUT before session Close; 30 isolated repeats and subsequent full/race
runs passed. That existing state-ordering implementation was not changed. An intermediate
integration run also overlapped a worker's RED renderer fixture; the corrected final
tree passed both fresh package verification and the complete suite.

| Question | Evidence-backed assessment |
| --- | --- |
| Clearer contracts/arguments? | Return fields are now visible and scoped to the actual tool. Input schemas remain flat and unchanged; no added parameter nesting. |
| More usable multi-tool facts? | Remote counts no longer require presentation parsing. Forced-pruning tests retain zero, positive counts and errors through 3+ calls, including nested wrappers and tools-disabled synthesis. |
| Better correction context? | Current bounded repair instruction survives competing observations. Rejected calls remain non-execution data. |
| Correct partial failures? | Outcome and effect facts survive separately in context and in user-facing incomplete notices, including wrapped errors. |
| Less context waste? | Full payloads remain outside the disposable transcript; bounded hints/previews add useful redundancy but increase input cost. Base manifest grew 32,513 to 39,793 bytes; full live inventory is 40,852 bytes. |
| Better measured model accuracy? | Not established. Retained-candidate run passed 7/8; unchanged recovery repeats passed 3/3. Experiments also exposed stale-read and speculative-ordering failures. All traces retained. |
| Preserved LLM agency? | No chosen next tool, task obligation reducer, inferred progress, condition evaluator, retry workflow, new phase machine or semantic finalizer was introduced. |

See [the complete provider evaluation](evals/2026-09-11-reference-loop-provider.md) for
settings, every trial and the precise failure traces. Tests establish information and
execution invariants, not universal semantic correctness.

The original implementation's strongest parts remain: one native iterative loop,
compatible read concurrency, synchronous source-ordered actions, non-retryable unknown
effects, one canonical command implementation, flat arguments, exact current-request
retention, atomic protocol pruning, full-result paging, and transactional interruption
evidence. Bounded previews can still omit needed detail; model reasoning can still drift;
historical receipts are not cross-turn idempotency. No deployments, bot restarts, remote
writes, reference-repository modifications, streaming changes or unrelated security work
were performed.
