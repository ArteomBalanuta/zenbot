# Agent turn state, budgets, evidence, freshness, and finalization

**Status:** architecture handoff only; no application code implemented.

**Source revision scope:** Saturn checkout at `/Users/ab/workspace/projects/saturn` (read-only), branch `develop` as named by `MIGRATION_PLAN.md`. **Target:** this Zenbot checkout. All source references below are repository-relative to the relevant checkout; line numbers are intentionally omitted where symbols are more stable.

## 1. Decision and bounded slice

This slice covers Saturn audit rows **#128–#143** in `.hermes/migration-audit.md`: `AgentExecutionState`, `AgentFreshDataCoordinator`, `AgentFreshDataFinalValidator`, `AgentFreshDataPolicy`, `AgentFreshDataTurnPolicy`, `AgentFreshnessPolicy`, `AgentMessageHistory`, `AgentNickNormalizer`, `AgentToolEvidence`, `AgentTurnMemory`, `AgentTurnPolicy`, `AgentTurnPolicyChain`, `AgentTurnPolicyInput`, `AgentTurnPolicyResult`, `AgentTurnState`, and `AgentUnverifiedActionPolicy`.

It also consumes, but does not reimplement, the already accepted Stage A/B tool contract/execution foundation. In particular, Saturn `AgentToolBudgetPolicy`, `AgentToolResultCoordinator`, `AgentToolBatchContext`, `CancellationToken`, and the accepted tool executor semantics are dependencies of this slice, not a reason to duplicate tool validation, registry, scheduling, or result rendering.

**Recommended bounded outcome:** implement request-local turn coordination as pure/in-memory components plus adapters around the existing Go runtime, LLM, assembler, and tool executor. It may be tested with fake LLM, fake runner, fake memory, and fake sink. It must not be wired to persistence, live provider configuration, moderation, listener ordering, or live chat/router dispatch in this slice.

This is deliberately not a claim of full agent migration. `MIGRATION_PLAN.md` says agent integration is Slice 7 and that the private agent runtime remains unwired from live command dispatch. The frozen audit marks every row #128–#143 `needs implementation` even though the target has related foundations.

## 2. Evidence map and observed behavior

### Saturn turn state and evidence

* `[OBSERVED]` `saturn/src/main/java/org/saturn/app/agent/turn/AgentTurnState.java` owns one `AgentExecutionState`, correction flags, tool enablement, command/tool success sets, successful result list, and attempted/success/failure counters. Its class comment explicitly says state is owned by exactly one router turn and is intentionally not thread-safe; the owning session lock is the concurrency boundary.
* `[OBSERVED]` `AgentExecutionState.advanceStep()` rejects once `steps >= limits.maxSteps()`. `reserveToolCalls(n)` rejects negative requests and rejects a request that would exceed `maxToolCallsPerTurn`; it increments only on success. Reservation is therefore a pre-execution operation, not a post-hoc accounting event.
* `[OBSERVED][TEST-BACKED]` `saturn/src/test/java/org/saturn/app/agent/turn/AgentTurnStateTest.java` proves: a one-step budget permits exactly one step; a two-call budget permits exactly two reserved calls; correction flags can be set independently; tools can be disabled idempotently; successful/failed command and tool sets are idempotent; outcome snapshots are immutable; and successful+failed tool outcomes cannot exceed attempted count.
* `[OBSERVED]` `AgentToolEvidence` (`.../AgentToolEvidence.java`) is an immutable value with invariant `successfulCount + failedCount == attemptedCount` and `attempted == attemptedCount > 0`. `none()` is the zero value.
* `[OBSERVED]` `AgentToolBudgetPolicy` (`.../tool/execution/AgentToolBudgetPolicy.java`) throws for non-positive requested calls, reserves through `AgentTurnState`, returns `(executeTools=true, finalizeWithoutTools=false)` on success, and disables tools plus returns `(false,true)` if reservation fails. The Go migration should preserve this transition, not silently truncate or execute an over-budget batch.

### Freshness and prerequisite semantics

* `[OBSERVED]` `AgentFreshnessPolicy` (`.../turn/AgentFreshnessPolicy.java`) currently defines one mandatory freshness capability, `USER_MESSAGE_HISTORY = "user_message_history"`. It recognizes explicit/profile/history/speech forms and bounded follow-ups based on the latest user message. It extracts a nick, normalizes mentions, accepts offline targets, and uses trusted room-user context for ambiguity handling. Moderation is not a freshness lookup in the Saturn router path.
* `[OBSERVED]` `AgentFreshDataCoordinator` (`.../turn/AgentFreshDataCoordinator.java`) checks `requiredFreshTool` against `turnState.hasSuccessfulTool`. If missing, a targeted history request is executed directly with a synthetic call after reserving one call. A failed required lookup is a routing failure; a successful result is counted, stored in model-visible message form, and followed by a provider synthesis request. For non-history required tools, one bounded exact-tool-call correction is allowed; a second malformed/missing call fails closed. If a required tool is unavailable after budget exhaustion, it fails closed.
* `[OBSERVED]` Fresh history requires an exact tool name and, where a target exists, a JSON `nick` matching case-insensitively after trimming. `AgentFreshDataPolicy.matchesTarget` rejects malformed JSON and wrong targets. `satisfiesProfileContract` requires a successful `user_message_history` result and nonblank response content.
* `[OBSERVED]` `AgentFreshDataTurnPolicy` stops the policy chain while a required tool has not succeeded. Once the named successful tool is recorded, it allows later policies. `AgentFreshDataFinalValidator` rejects a final response that still lacks the required successful history evidence/content contract.
* `[OBSERVED]` `AgentFreshDataCoordinator` can request a fresh synthesis correction if the response repeats the previous assistant message or fails the profile contract. It permits one bounded synthesis correction via turn state and rejects a second incomplete synthesis.

### Policy chain and unverified actions

* `[OBSERVED]` `AgentTurnPolicyInput` is immutable and carries the current response, projected messages, tool definitions, command prose guard, mutable request-local state, prompt, correlation ID, and optional required fresh tool. Its canonical constructor null-checks all components.
* `[OBSERVED]` `AgentTurnPolicyResult` carries the replacement response, correction-used flag, and continuation flag. `stop(response)` stops later policy evaluation without changing the response.
* `[OBSERVED][TEST-BACKED]` `AgentTurnPolicyChain` applies policies in list order, carries each result response to the next policy, ORs correction-used flags, and breaks on `continuePolicyEvaluation=false`. `AgentTurnPolicyChainTest` proves both ordered response propagation and stop-before-later behavior.
* `[OBSERVED]` `AgentUnverifiedActionPolicy` runs at most once until reset. When there are no successful commands or the response does not contain a recognized command, it delegates to `AgentResponseCorrector.correctUnverifiedActionClaim`, marks the check, and returns the corrected response. Tool-result coordination resets this check after a tool batch, because a new batch changes the evidence basis.

### History and memory

* `[OBSERVED]` `AgentMessageHistory` (`.../AgentMessageHistory.java`) searches backwards for the latest role content, distinguishes ordinary assistant content from the exact `[Internal tool evidence from ...]\n` prefix, and exposes the evidence name. This prevents internal evidence rows from being treated as the conversation's latest assistant answer.
* `[OBSERVED]` `AgentTurnMemory` (`.../AgentTurnMemory.java`) owns memory I/O and error translation. It filters legacy persona turns on load via `AgentResponseSanitizer.excludeLegacyPersonaTurns`; it rejects null history; and translates storage failures to public messages `Agent memory load failed` / `Agent memory persistence failed` while retaining the cause for diagnostics. Tool evidence is prevalidated for nulls before any append, then appended in input order.
* `[TEST-BACKED]` `AgentTurnMemoryTest` proves legacy filtering leaves only the relevant conversation messages, internal persistence details do not appear in public error text, empty evidence is a no-op, evidence order is retained, and a null in the batch causes failure before partial persistence.

## 3. Current Zenbot foundations and gap

### Existing target seams

* `[OBSERVED]` `internal/agent/runtime/contracts.go` already provides immutable `api.Context`/`runtime.Invocation`, four closed modes, capability membership, public-vs-whisper `MemoryKey`, a cancellation-aware `Runner`, and a `Result` with `ShouldReply`. `internal/agent/api/api.go` provides stricter public JSON contracts and validation.
* `[OBSERVED][TEST-BACKED]` `internal/agent/runtime/runtime.go` admits bounded work without blocking, serializes execution by `Context.MemoryKey()` using one room mutex, permits different keys to run concurrently, and cancels runners during `Close`. `internal/agent/runtime/runtime_test.go` proves queue rejection, per-room serialization, cross-room concurrency, delivery, and shutdown cancellation.
* `[OBSERVED]` `internal/agent/tool/execution/execution.go` already has a mutex-protected ledger, duplicate and per-tool reservations, prerequisite checks, failure disablement, context cancellation/deadline translation, result validation, and ordered contiguous read-only parallelism with action barriers. `execution_test.go` proves ordered parallel batches/barriers, duplicate/limit/failure behavior, authorization, timeout, and cancellation. This is the accepted execution contract and must be reused.
* `[OBSERVED]` `internal/agent/assemble/assemble.go` builds an immutable `PreparedRequest`, filters internal evidence from history, computes required fresh tool/nick, carries projection/fingerprint accounting, and renders trusted runtime metadata. `assemble_test.go` proves freshness extraction, Unicode bounds, cancellation, context/history/tool defensive copying, and internal-evidence filtering.
* `[OBSERVED]` `internal/agent/llm/client.go` provides immutable provider-neutral messages, tool calls, requests, responses, and `LlmClient.Complete(context.Context, LlmRequest)`. `internal/agent/llm/openai/client.go` plus tests provide an OpenAI-compatible adapter with cancellation, retry, malformed-response, and secret-redaction behavior.
* `[OBSERVED]` `internal/config/agent_config.go` already carries timeout, max tokens, max steps, max tools, and SQL settings with bounded defaults/validation. The config's `MaxTools` is the source of the per-turn reservation limit; a separate policy should not invent a second default.
* `[OBSERVED]` `internal/agent/participation/policies.go` has a minimal `ToolEvidence{Attempted bool}` and classification/finalization seam. It is useful as an adapter point but is not yet equivalent to Saturn's four-field evidence or turn policy chain.

### Gaps to fill without integration

`internal/agent/turn/` should be the target home for request-local state, evidence, freshness detection/coordinator/validator, history helpers, memory facade, and policy chain. `internal/agent/tool/execution/` should receive only the budget/evidence adapter needed to connect the accepted executor to the turn state; do not reimplement `Executor`, `Ledger`, contract validation, or scheduling. `internal/agent/runtime/` should receive a bounded runner/orchestration adapter only after the pure turn package has tests. `internal/agent/assemble/` should consume an evidence value at the provider-final prompt replacement point; current initial assembly's zero evidence is correct for a candidate request, but final prompt rendering must be supplied the live counters.

No persistence adapter is required to validate this slice: use a narrow in-memory `MemoryStore` fake matching Saturn's load/append/append-tool-evidence behavior. No provider integration is required: use a scripted `LlmClient`. No listener/moderation/live-router integration is required: invoke the turn coordinator directly from tests.

## 4. Proposed interfaces and ownership

The following are target interfaces, marked `[RECOMMENDED]`, not observed Go APIs:

```go
// internal/agent/turn/state.go
 type ExecutionLimits struct { MaxSteps, MaxToolCalls int; ToolTimeout time.Duration }
 type State struct { /* owner-goroutine/request-local; no exported mutable fields */ }
 func NewState(ExecutionLimits) *State
 func (s *State) AdvanceStep() bool
 func (s *State) ReserveToolCalls(n int) bool
 func (s *State) DisableTools()
 func (s *State) ToolEvidence() Evidence

// internal/agent/turn/policy.go
 type Policy interface { Apply(context.Context, PolicyInput) (PolicyResult, error) }
 type PolicyChain struct { ... }

// internal/agent/turn/freshness.go
 type FreshnessPolicy interface { RequiredTool(prompt string, history []llm.LlmMessage, users []string) (tool, nick string, ok bool) }
 type FreshDataCoordinator interface { Process(context.Context, ProcessInput) (ProcessResult, error) }

// internal/agent/turn/memory.go
 type MemoryStore interface {
   Load(api.Context) ([]llm.LlmMessage, error)
   Append(api.Context, string, string) error
   AppendToolEvidence(api.Context, string, string) error
 }
```

`State` is the sole owner of counters and correction flags. The runtime/session owner is the sole caller that mutates it. The executor owns tool-contract/ledger state; the turn state owns turn-wide budget/evidence. The coordinator owns freshness sequencing but not the main router loop. The policy chain owns deterministic policy ordering but not tool execution. The finalizer owns correction, freshness validation, sanitization, reply/silent decision, and output bounds, while memory owns only memory I/O/error translation.

## 5. State transitions and sequencing

### Proposed end-to-end sequence (based on observed Saturn `DefaultAgentRouter.routeInSession`)

```text
caller/listener (future, excluded) -> Runtime.Submit
  -> per-MemoryKey owner lock
  -> load memory; exclude legacy persona/internal evidence
  -> assemble candidate request + requiredFreshTool/nick
  -> LLM candidate response
  -> State.AdvanceStep (reject at max)
  -> FreshDataCoordinator.Process
       missing required data? reserve 1 before execution
         -> accepted Executor.Execute / ExecuteAll
         -> record attempted, success/failure, evidence, model-visible tool message
         -> LLM synthesis; restart loop
       missing/incorrect structured call? one bounded correction; second failure
       fresh evidence present but stale/repeated/incomplete synthesis? one correction; validate
  -> ordered PolicyChain (freshness gate, unverified-action, command channel)
       stop means do not run later policies
  -> response has tool calls?
       reject if disabled
       BudgetPolicy.reserve(call count) BEFORE Executor.ExecuteAll
       failure disables tools and invokes no tools; finalize-only provider request
       success -> append assistant tool-call message
              -> accepted ExecuteAll (prerequisites/order/deadline/cancellation)
              -> ResultCoordinator records every result and renders messages
              -> reset unverified-action check; refresh final system evidence; next LLM response
  -> finalizer: provider/failure/internal-evidence correction, fresh validator,
       sanitize, moderation silent/no-reply decision, output bound
  -> if reply: append conversation + allowed model-data evidence (future persistence)
  -> return result; sink delivery (future listener/router integration)
```

### Required transition invariants

1. `[RECOMMENDED]` A turn begins with zero counters, tools enabled, no corrections consumed, and empty evidence. State is discarded after the request; it is not shared across rooms or turns.
2. `[TEST-BACKED Saturn]` Each successful `AdvanceStep` consumes one step; the boundary is inclusive at `maxSteps`, then false.
3. `[TEST-BACKED Saturn]` Each accepted reservation increments the turn-wide tool count before any execution. A rejected reservation disables tools and causes no call execution. Never reserve after execution.
4. `[TEST-BACKED Saturn]` Tool evidence is counted by result outcome, must balance, and successful-result snapshots are immutable. Null results, mismatched call/result cardinality, or over-recording are errors.
5. `[RECOMMENDED]` Prerequisite checks happen before reservation/execution at the executor ledger boundary; a missing prerequisite is a coded result and does not count as successful evidence.
6. `[OBSERVED]` Same `MemoryKey` turns are serialized by the runtime room lock; different keys may run concurrently. The state itself need not be thread-safe, but an implementation must not let parallel tool workers mutate it directly; aggregate results in provider order under the turn owner.
7. `[RECOMMENDED]` Explicit cancellation and deadline cancellation prevent admission to new work, cancel outstanding execution contexts, and finalize with a coded failure/silent result. They must not be converted into successful evidence or a normal reply.
8. `[OBSERVED]` Finalization rejects null/empty output, truncates to configured max output, suppresses ambient no-reply, makes moderation silent, and rejects a no-reply marker for a required-reply invocation.

## 6. Cancellation, deadline, errors, and finalization

* `[OBSERVED]` Saturn `AgentToolBatchContext` carries an `Instant` deadline plus monotonic `CancellationToken`; deadline and explicit cancellation have distinct reasons/codes. Scheduler checks cancellation/expiry before submit, cancels futures on timeout/interruption, preserves input order, and returns `TOOL_BATCH_DEADLINE`, `TOOL_BATCH_CANCELLED`, or `TOOL_INTERRUPTED` as applicable.
* `[OBSERVED]` Target `execution.Cancellation` wraps context cancellation/deadline, and `Executor` maps already-cancelled contexts to batch codes and per-tool timeout to `TOOL_TIMEOUT`. Reuse these target semantics; do not create a competing token unless an adapter is needed.
* `[RECOMMENDED]` The whole turn should derive a child context from the runtime/request deadline. Every LLM completion, memory call, freshness lookup, and tool execution receives it. Before each state mutation or provider retry, check `ctx.Err()`; after cancellation, do not deliver a sink reply.
* `[OBSERVED]` Saturn catches provider `LlmException` at router boundary and exposes `AgentRoutingException("Agent provider failed: ...")`; memory hides storage details behind stable public messages. Go should retain typed errors/codes internally but avoid provider tokens, SQL text, database details, or raw tool arguments in user-visible final content/log fields.
* `[RECOMMENDED]` A failed tool may be rendered to the model as a coded, bounded tool result, but a failed mandatory fresh-data tool must terminate the fresh-data path rather than permitting an unsupported synthesis. A provider response with tool calls after tools are disabled is a routing failure, not an opportunity to execute them.
* `[OBSERVED]` Finalization order is material: failure-placeholder correction, internal-evidence leak correction, fresh-data final validation, sanitization, empty check, moderation/no-reply decision, quote-only correction where applicable, marker handling, and final output truncation. Preserve this order in a Go finalizer test matrix.

## 7. Security and privacy boundaries

* `[OBSERVED]` `api.Context.MemoryKey` separates public history from whisper history and keys whisper sessions by trip, hash, or nick. Never merge public and whisper memory to simplify turn state.
* `[OBSERVED]` Assembler excludes internal tool-evidence messages from provider history; `AgentMessageHistory.latestConversationAssistant` excludes them from repetition checks. Keep this filter before freshness comparison and before user-visible output.
* `[OBSERVED]` Fresh history targeting must retain the requested nick and room/trip/hash scope. A successful lookup for another nick is not evidence for the requested target. Do not infer authorization from room-user presence; Saturn explicitly allows offline targets while preserving scope.
* `[OBSERVED]` Tool authorization/capability checks belong to accepted executor/descriptor paths. Turn orchestration must not bypass them for synthetic fresh calls, command corrections, or finalization.
* `[RECOMMENDED]` Persist only explicitly permitted model-data evidence; action/error/internal evidence remains non-persistent unless the accepted Saturn mapping says otherwise. Do not put API keys, raw provider diagnostics, SQL/database details, or private whisper content into correlation IDs, metrics, or public errors.
* `[OBSERVED]` Moderation is a finalization/router concern and is explicitly excluded from this bounded implementation. Do not claim that turn policy tests prove moderation safety.

## 8. Staged migration plan

### Stage 1 — pure values and state

Add Go `turn` values for limits/evidence/state, exact invariants, correction flags, immutable snapshots, and a budget policy adapter. Unit tests mirror all five Saturn `AgentTurnStateTest` cases plus invalid evidence and negative reservation cases. No imports from repository, listener, provider, or moderation.

### Stage 2 — policy chain and history

Implement policy input/result/chain, `AgentMessageHistory`, nick normalization, and unverified-action policy against existing Go response-correction seams. Test ordered response carry-forward, stop semantics, reset behavior, internal evidence exclusion, and no correction when command evidence is present.

### Stage 3 — freshness coordinator/validator

Implement the observed required-tool/nick patterns and follow-up behavior, using the existing assembler's freshness extraction as a compatibility reference. Inject `llm.LlmClient`, accepted execution interface, and renderer/definition provider. Test exact call enforcement, target mismatch, one correction limit, mandatory lookup reservation-before-execute, failed lookup, repeated synthesis, and final validator failure. Use scripted fakes only.

### Stage 4 — memory facade

Implement an in-memory test store adapter and `AgentTurnMemory`-equivalent error translation/filtering. Test load/append/evidence order, null prevalidation, and public error redaction. Leave H2/persistence wiring for the persistence slice.

### Stage 5 — isolated bounded runner integration

Create an unexported or private runner composition used only by package tests (or a feature-disabled factory seam). Connect runtime cancellation/deadline, assembler, LLM fake, accepted executor, policy chain, coordinator, and finalizer. Verify same-memory-key serialization with `go test -race`; do not register a listener, command, live router, or provider in `main`.

### Stage 6 — later integration gate (excluded here)

Only after persistence, provider configuration, moderation, listener ordering, and live routing are independently accepted should the runner be wired into the production agent factory and command/listener path. That later work must add real-H2 memory/evidence tests and end-to-end visibility/security tests.

## 9. Exact exclusions and unsupported claims

This handoff does **not** implement or claim:

* H2 schema/repository methods, `agent_memory`/`agent_tool_memory` persistence, transactions, or real-H2 evidence.
* OpenAI/provider wiring, provider retry policy beyond the existing adapter, credentials/config startup, or production LLM availability.
* Moderation actions, protected-principal enforcement, listener ordering, message auditing, room snapshots, or live chat delivery.
* Live router/command registration, `l` exposure, participation dispatch, or end-to-end command behavior.
* A general dependency DAG: accepted execution supports contiguous compatible read-only batches and sequential barriers, not arbitrary scheduling.
* Thread safety of the turn state itself: the owner lock/goroutine is required. Tool executor internal locking does not make arbitrary turn-state mutation safe.
* More than one fresh-data tool unless separately specified by the Saturn source. Current observed policy names `user_message_history`.
* Full closure of audit rows #128–#143. The audit remains `NOT COMPLETE` until focused parity evidence and all external integration gates pass.

## 10. Complexity and risk

| Area | Complexity | Main risk | Mitigation |
|---|---:|---|---|
| State/evidence/budget | Low–medium | counting after execution or mismatched outcomes | immutable evidence invariant; reservation tests; owner-only mutation |
| Policy chain | Low | wrong order or continuing after stop | table-driven order/stop tests |
| Freshness recognition | Medium | false positives/negative target extraction; stale synthesis | port exact patterns and test ambiguous terms, mentions, follow-ups, malformed JSON |
| Fresh coordinator | High | duplicate execution, bypassed authorization, correction loops | inject accepted executor; reserve first; one-use flags; fake call ledger |
| Cancellation/deadline | Medium–high | goroutine leaks or reply after shutdown | context propagation, race tests, cancellation-before-admission tests |
| Memory facade | Medium | leaking private/storage details; partial evidence append | prevalidate all results; stable public errors; persistence later |
| Finalization | Medium | accepting unsupported action/no-reply or leaking internal evidence | preserve observed order; dedicated matrix |
| Integration | High (deferred) | accidental live-router/provider/persistence scope expansion | keep package-private seams and feature-disabled factory |

## 11. Acceptance matrix

| Requirement | Evidence/test required | In-slice status/owner |
|---|---|---|
| Rows #128–#143 have target owner | file map above and package ownership review | planned; no row marked complete |
| step budget is bounded | max=1 permits one then rejects; max boundary tests | Stage 1 |
| tool budget reserves before execution | spy executor observes no call after failed reservation; count increments once | Stage 1/3 |
| tools disable on exhausted batch | subsequent tool call is rejected; finalization path is selected | Stage 1/5 |
| evidence invariant | zero/valid/invalid constructor and success+failure balancing tests | Stage 1 |
| defensive snapshots/idempotent sets | mutate returned slices/maps and repeat records | Stage 1 |
| one-owner concurrency | same memory key serializes; distinct keys overlap; race test | Stage 5; target runtime already proves base behavior |
| exact freshness tool/target | wrong name, wrong nick, malformed args, one correction, second failure | Stage 3 |
| prerequisite semantics | missing prerequisite rejected before accepted executor call; success unlocks dependent call | reuse execution tests + Stage 3 |
| fresh synthesis | repeated previous assistant rejected/corrected; evidence/content required; second failure rejected | Stage 3 |
| policy chain | deterministic order, response carry-forward, stop short-circuit | Stage 2 |
| unverified action | correction only under observed conditions; reset after new tool batch | Stage 2/5 |
| memory privacy/errors | legacy/internal evidence filtered; null batch prevalidated; stable redacted errors | Stage 4 |
| cancellation/deadline | LLM/tool contexts canceled, coded result, no sink delivery after cancellation | Stage 5 |
| finalization | correction/validation/sanitize/no-reply/moderation/output-bound matrix | Stage 5; moderation behavior itself deferred |
| no accepted execution rework | imports/adapters use `internal/agent/tool/execution` and existing contract tests remain green | all stages |
| no external integration claim | no changes to listener/main/provider/persistence registration | all stages |

## 12. Artifact checks and verification record

The handoff was written to the requested path:

`/Users/ab/workspace/go-projects/zenbot/.hermes/handoffs/agent-turn-freshness-architecture.md`

Checks performed for this architecture phase:

* `[PASS]` `MIGRATION_PLAN.md` and frozen `.hermes/migration-audit.md` were inspected before Saturn source analysis.
* `[PASS]` Saturn source files for all named turn classes and their four focused tests were located and read.
* `[PASS]` Saturn related router/finalizer/budget/result-coordinator/execution/cancellation sources were inspected.
* `[PASS]` Target runtime, tool execution, assembler, LLM, config, participation, and focused tests were inspected.
* `[PASS]` The target worktree was observed before writing; it already contained extensive unrelated modified/untracked migration work. This handoff did not revert or edit those application files.
* `[PASS]` The output is non-empty and contains selected audit rows, source citations, observed/test-backed versus recommended labels, a sequence diagram, transitions, ownership, cancellation, security, exclusions, risks, staged plan, and acceptance matrix.
* `[LIMITATION]` No application code or new tests were implemented, so this document does not provide new runtime parity evidence and cannot change the audit verdict.
* `[LIMITATION]` Because the target worktree was already dirty, final verification of unrelated-change preservation is based on the pre-write status observation and the handoff-only write, not on a clean diff.
