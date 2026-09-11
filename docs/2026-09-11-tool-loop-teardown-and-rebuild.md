# Tool harness: teardown and rebuild

## Outcome and scope

Rebuilt the SDK/tool interface and production execution loop around one model-owned conversation. The LLM chooses actions, dependencies, conditions, recovery and completion. The SDK validates executable contracts and resource limits, executes calls, and returns factual outcomes. It does not infer user obligations or implement a semantic planner.

The starting revision was `6dfe93c` on `master`. That revision had already removed an earlier task planner; this redesign also removes the separate mandatory completion judge, phase/recovery choreography, command-prose correction, and dormant execution paths. This report distinguishes implemented mechanical guarantees from empirical model behavior.

No real chat actions were used for model evaluation. No deployment, restart, push, database migration, or unrelated security hardening was performed.

## Findings and implemented fixes

### 1. Competing completion authorities — High

**Trace:** A worker produces a normal answer → a second model with a smaller evidence projection judges it → the judge can reject useful partial work or request another cycle → phase/recovery rules can disable the capabilities needed for that correction. Even ordinary conversation incurs an extra provider call.

**Rebuild:** `live/tool_loop.go` and `live/turn_engine.go` now contain one native tool loop. A valid no-tool response is the model's final answer. The SDK checks only protocol/format validity and delivery receipts for explicit silence. Removed the semantic completion gate, phase machine, recovery policies, interrupt/resume scaffold, command-prose parser and duplicate bounded loop.

**Evidence:** Ordinary-answer regression requires one model call. The actual provider evaluation also used one call. Multi-round tests exercise one continuous transcript rather than planner/judge handoffs.

### 2. Model-hostile contracts and false strictness — High

**Trace:** The local descriptor knows purpose, avoid-guidance and prerequisites → abbreviated provider projection omits useful distinctions → the model guesses between adjacent operations or calls a dependent tool too early. Root union schemas and unconditional strict declarations can disagree with the endpoint's accepted parameter shape.

**Rebuild:** `tool/contract/manifest.go` projects concise ordinary prose including purpose, use/avoid, one example, prerequisites and output behavior. Every parameter schema has an object root. Auto-move/database-query variants are flat objects with local cross-field checks. `SupportsStrictParameters` declares strict mode only for compatible closed, fully required schemas. Optional fields remain optional.

**Evidence:** Catalog-wide valid examples/encoder checks, strict-schema regressions, and production inventory tests. Full creator invocation is 65 tools and 33,534 serialized schema bytes; the paging tool accounts for the difference from the 64-tool base manifest.

**Boundary:** Exact primary-intent uniqueness detects declared duplicates, not every possible natural-language overlap. `room_users` means the current room; `saturn_list` means another room; `saturn_ping` has the fixed hack.chat target. `run_command` remains a tested legacy adapter, not a production-visible choice. Kick stays the single-field nickname contract.

### 3. Malformed argument destroys valid siblings — High

**Trace:** A model emits two calls → one argument string is invalid JSON → provider decoding rejects the whole response → the valid sibling never becomes an executable call or useful result, and the model gets no per-call correction opportunity.

**Rebuild:** Preserve provider raw arguments in `llm/openai/client.go`. The local executor rejects only the malformed call with `INVALID_ARGUMENTS`, while valid siblings can succeed. Protocol replay uses the original argument string, avoiding decoded `null` and large-number precision loss.

**Evidence:** `TestMalformedArgumentsRemainToolObservationsInsteadOfRejectingResponse` and `TestTurnEngineMixedMalformedArgumentsPreserveSuccessfulSiblingForRepair` verify real parsing, mixed results, retained successes, and a subsequent corrected call.

### 4. Poisoned retry keys and global degradation — High

**Trace:** A call fails validation, misses a prerequisite or encounters a transient read error → its argument key is recorded as already used → a legitimate corrected-context retry is rejected → recovery disables useful tools or loops through apologies.

**Rebuild:** `execution/Ledger` separates admitted, running and finished attempts from action keys. Reads can refresh and retry. Validation/prerequisite failures consume bounded attempts but not execution-failure allowance. A known noncommitted action can be retried; committed/partial identical actions cannot. Errors stay available as feedback; the model chooses any retry or alternative.

**Evidence:** Tests for retrying failed reads, refreshing successful reads, satisfying prerequisites then retrying, per-tool limits and repeated effects. Provider-backed NOT_FOUND recovery revised the nickname and completed; the no-thinking baseline also exposed one wasteful retry, documented rather than hidden.

### 5. Error status erases effect certainty — Critical

**Trace:** A command changes state or sends a message → later formatting/delivery fails or panics → a generic error replaces the successful receipt → the caller may treat it as safely retryable or claim no action happened. Cancellation after dispatch creates the same ambiguity.

**Rebuild:** Results carry `NOT_STARTED`, `NOT_COMMITTED`, `COMMITTED`, `PARTIAL` or `UNKNOWN`, separately from success/error. Action/delivery counts and related IDs survive errors and schema rejection. The gateway captures successful typed mutations, including prefix/auto-move/replica operations. Unknown outcomes use non-retryable `ACTION_OUTCOME_UNKNOWN`, block further actions for the invocation and retain read-only verification tools. Actions remain synchronous.

**Evidence:** Execution tests cover committed invalid results, partial receipts, post-action errors, unknown outcomes, duplicate effects, and cancellation. Gateway tests cover rejection after delivery, panic after delivery, and a prefix mutation followed by send failure.

**Boundary:** Cooperative cancellation cannot forcibly stop an implementation that ignores its context. Action deduplication and uncertainty barriers are request-local, not durable cross-process idempotency.

### 6. Concurrency-dependent admission and bypassed action barriers — High

**Trace:** Worker goroutines race to consume a per-tool budget → a later call can be admitted ahead of an earlier call. Separately, removing a manifest-rejected action before the batch executor runs lets a following action bypass the failed-action barrier.

**Rebuild:** Calls are admitted in source order before concurrency. Safe reads can overlap; actions execute synchronously in provider order. An invocation-specific exposed-tool set is validated inside preparation, preserving the rejected action's effect classification in the batch. A failed action yields `ACTION_NOT_EXECUTED` for later actions, with the failed call ID; independent reads still finish.

**Evidence:** Ordered-admission, read fan-out, satisfied-prerequisite parallelism and failed-action tests. `TestTurnEngineManifestRejectionPreservesFailedActionBarrier` verifies a rejected action, skipped following action, and successful independent read.

### 7. Unreachable evidence after truncation and pruning — High

**Trace:** JSON is wrapped as an escaped summary string → oversized fields consume the view and hide later facts → early tool-call/result groups are pruned after more calls → an opaque continuation marker has no callable retrieval path. The final model cannot reconstruct the work it is expected to summarize.

**Rebuild:** `assemble/observations.go` emits structured `data`, explicit status/error/effect metadata, counts, truncation and omitted fields. Full results and original arguments remain in the request-local store. `read_tool_result` retrieves bounded Unicode pages without rerunning the original action. The required result index preserves every receipt through pruning; argument previews shrink first. Exact original request and current runtime status are mandatory; native call/result pairs remain atomic.

**Evidence:** Three-dependent-round tests preserve the exact request and earlier IDs; forced-pruning tests retain all receipts. Paging tests reconstruct Unicode content and original arguments exactly, enforce the byte limit, and reject nonexistent/out-of-range requests.

**Boundary:** Keeping evidence reachable proves information availability, not that every model uses it correctly. Retrieval consumes calls, and the context estimator is character-based rather than the provider's exact tokenizer.

### 8. Projection failure drops later committed results — High

**Trace:** A batch fully executes → its first observation has metadata too large for the view, such as a provider-supplied oversized call ID → protocol projection returns immediately → later committed action receipts never enter the observation store → the interrupted-turn record falsely appears to contain only the first call.

**Rebuild:** `appendRegistryProtocol` retains the complete executed batch before returning any projection error. It appends the assistant/call/result protocol atomically only after projection succeeds, leaving the prior valid conversation untouched on failure.

**Evidence:** `TestTurnEngineRetainsWholeExecutedBatchWhenObservationMetadataCannotFit` reproduced the loss, then verifies both results, the committed action receipt and the intact prior transcript after failure.

### 9. Context budget measures a different request than the wire — High

**Trace:** A malformed 20-KiB argument decodes to `nil` → the projector sizes `"null"` → the provider adapter correctly replays the original 20 KiB → the supposedly bounded correction request exceeds its context budget. Whitespace-heavy valid JSON produces the same undercount.

**Rebuild:** The assembler's wire representation and integration assertions use `RawArguments`, exactly like provider serialization.

**Evidence:** `TestProjectionBudgetsRawArgumentsActuallySentToProvider` tests malformed and whitespace-heavy 20-KiB arguments and verifies atomic pruning using the actual representation.

### 10. Business rejection labeled successful — High

**Trace:** Mail has no matching recipient, or note storage lacks the caller's required identity → a legacy handler reports a successful status with no completed business operation → downstream code can present it as a success or verified room answer.

**Rebuild:** Command handlers report missing/invalid prerequisites as failure outcomes. Typed gateway status, committed effects, positive action receipts and room-delivery receipts are validated separately. An empty lookup result can still be valid read data; it is not automatically action success.

**Evidence:** Mail/note parity tests now require truthful failure, gateway NOT_FOUND tests preserve missing-target behavior, and executor tests reject unverified actions/delivery. Kick to an absent current user sends no action and reports NOT_FOUND.

### 11. Failed continuation loses session evidence — High

**Trace:** A tool succeeds or has an unknown outcome → the next provider request fails → the runner returns a generic error and discards partial Completion → a follow-up sees no execution record and may repeat the work or deny the effect.

**Rebuild:** `IncompleteTurnError` retains request-local observations and validated read evidence. A bounded persistence attempt stores an explicit interrupted-turn receipt record, not a fabricated final answer. It reloads as context rather than prior assistant speech. Successful explicit silence stores actual delivered data, not the no-reply marker. History evidence schema accepts the timestamps actually returned by the history tool.

**Evidence:** Runner tests verify committed/unknown receipts, provider failure, cancellation, validated read evidence and absence of fake successful output. Real history-result persistence is tested. Production memory writes remain atomic.

**Boundary:** Persistence itself can fail; this error is preserved rather than swallowed. Interrupted records keep receipt metadata but not full action bodies, so they are not a permanent replay store.

### 12. Correct context, wrong conditional action — Critical model-quality finding

**Trace:** Real provider receives the exact request, lounge count 3 and programming count 4 → request authorizes kick only if the product exceeds 20 → the no-thinking model emits kick anyway → after a committed fixture receipt, its final answer denies the kick. This happened with complete, untruncated context, before and after a concise conditional-action policy refinement.

**Rebuild:** The system prompt explicitly requires evaluating conditions from returned evidence and truthfully reporting actual receipts. Controlled provider tests showed that prompt wording alone was insufficient. Enabled reasoning in the local ignored configuration and tracked examples; examples reserve 4096 completion tokens for models sharing reasoning/output budget. Explicit config controls remain available; no SDK condition evaluator or second completion judge was added.

**Evidence:** No-thinking baseline: 6/8 strict full-suite checks passed; false-condition checks failed repeatedly. An initial thinking-enabled run passed 8/8 full-suite checks and 6/6 additional repeated conditional trials. A later full run passed 7/8, with an intermittent compound-case failure; three focused reruns passed, leaving that failure's exact cause unproven. Earlier suite durations were 9.95 vs 32.00 seconds. Full prompts, actual calls, outputs, model identity, fixture correction, additional sampling checks and limitations are in [the provider evaluation](evals/2026-09-11-tool-loop-provider.md).

**Boundary:** This is an improvement in measured behavior, not a guarantee or proof that all conditional requests are safe. Model changes require re-evaluation. No production action was taken in these tests.

## Requirement-to-evidence map

| Requested capability | Implementation and validation |
|---|---|
| Accurate tool discrimination and arguments | Concrete catalog-owned tools, concise purpose/avoid/examples, flat schemas, strict compatibility, provider inventory checks and real kick/list/weather/ping calls |
| LLM-owned iterative planning and stopping | One model loop; no semantic task graph/reducer/judge; ordinary conversation uses one call |
| Sequential/dependent/conditional work | Native rounds preserve prior outcomes; scripted multi-round coverage plus real compound and conditional cases |
| Parallelizable calls | Model-selected native batch, safe-read fan-out tests, source-ordered admission and action barriers |
| Useful errors and model-driven correction | Per-call malformed-argument feedback, prerequisite repair, typed NOT_FOUND and effect certainty; provider alternative-target recovery |
| Partial success and uncertain actions | Receipt-preserving results, non-retryable unknown outcomes, retained executed batches and interrupted-turn persistence |
| Bounded loops without a rule planner | Aggregate/per-tool/model limits; useful failures remain visible; required terminal metadata and one reserved final synthesis |
| Context efficiency and state retention | Structured previews, required receipt index, exact request preservation, atomic groups and callable full-result paging |
| Natural coherent final response | Ordinary model text finalizes; combined summaries remain visible; model-selected silence requires delivery receipts |
| Integrated prompt/contract design | Current-room vs remote-room distinction, fixed ping target, weather command interface, dependencies/conditions and explicit receipt truth |
| Simplification | Removed duplicate loop, mandatory judge, phase/recovery/policy classes, command-prose heuristics and dormant moderation route |

## Validation and remaining limits

The full Go test suite, vet and focused race suite passed, alongside targeted red/green regressions. The opt-in provider evaluation has the mixed semantic results described above; it is not counted as an unqualified pass. Exact final commands and status are recorded in the implementation plan.

The model remains capable of semantic mistakes. Provider evaluation covers eight small English scenarios, not all tools, languages or long histories. Actual weather/network services and real moderation transport were not exercised by that evaluation. Commands still deliver intermediate chat messages; “one final combined summary” is supported, not “one total chat message.” Cooperative action cancellation, request-local deduplication, approximate token budgeting and bounded persistence remain explicit limits.

The local `config.toml` is ignored and is not part of the commit. Its reasoning setting was updated for the next service start; this does not change the already running bot. The SDK's zero-value/default compatibility behavior is not silently changed for other consumers.
