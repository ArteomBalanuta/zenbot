# Turn/Freshness Stages 1–4 Forensic Diagnostic

**Scope:** read-only root-cause analysis of the five remaining independent-QA failures. Target source is `internal/agent/turn/`; Saturn reference is `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/agent/turn/` and its focused tests. No application source or Saturn source was modified.

**Boundary:** recommendations below stay within Stages 1–4 and use pure/injected seams. They do not wire live provider, persistence, listener, router, moderation, or production agent registration.

## Executive diagnosis

The focused Go package is build/test green, but it is not yet behaviorally equivalent to the Saturn turn contract. Four gaps are implementable in the bounded package with injected fakes; one (production parity of memory) is intentionally blocked by the excluded persistence integration. The common pattern is that the Go implementation has a narrow state/marker seam where Saturn has a complete policy or value contract.

| # | QA failure | Root cause | Stage-1–4 disposition |
|---|---|---|---|
| 1 | Missing `AgentUnverifiedActionPolicy` behavior | `policy.go` exposes only generic policies and state mark/reset methods; no corrector dependency or command-evidence decision path exists | Implementable in Stage 2 with injected corrector/guard |
| 2 | Incomplete `PolicyInput`/`PolicyResult` contract | `PolicyInput` contains only response/messages/state; it omits definitions, prose guard, prompt, correlation ID, and optional required fresh tool; caller-owned slices and policy list are retained/exposed | Implementable in Stage 2; immutable snapshots require a deliberate copy boundary |
| 3 | Coordinator is only a marker seam | `coordinator.go` marks `FreshnessCorrectionUsed` and returns the same response; it does not append correction messages, call the LLM, enforce exact corrected call, or perform synthesis correction | Pure/injected implementation is possible in Stage 3; production provider parity remains excluded |
| 4 | Weak final freshness validation | `FinalValidator.Validate` checks only successful-tool-name membership and non-empty content. It does not inspect a valid successful result, result payload, prior assistant repetition, tool calls in synthesis, or one-use synthesis correction | Implementable in Stage 3 with state/result inputs; full router finalization remains excluded |
| 5 | Memory facade/test-only and wrong evidence bucket | `InMemoryStore.Evidence()` always reads `api.Context{}` rather than a requested `MemoryKey`; there is no `AgentTurnMemory`-equivalent production adapter because Go persistence API/registration is outside this slice | Bucket bug is implementable in Stage 4; production parity is genuinely blocked/deferred |

## 1. Missing unverified-action policy

### Observed Saturn behavior

`AgentUnverifiedActionPolicy.apply` (`saturn/.../AgentUnverifiedActionPolicy.java:28–41`) is an ordered `AgentTurnPolicy`. It returns the response unchanged once `turnState.unverifiedActionChecked()` is true. Otherwise it corrects when either no successful command exists or `commandProseGuard.findCommand(response.content())` is empty. Correction is delegated to `AgentResponseCorrector.correctUnverifiedActionClaim(response, messages, definitions, correlationId)`, then the state flag is marked. Saturn's `AgentUnverifiedActionPolicyTest` proves correction of a prose action claim and at-most-once pass-through after the check; the test also observes correction messages being added.

The corrector itself (`saturn/.../routing/AgentResponseCorrector.java:312–343`) is not a simple string rewrite: it rejects/short-circuits already tool-called or non-claim responses, appends assistant plus correction user messages, calls the injected LLM, allows a bounded second correction, and returns a capability limitation after repeated unverified claims.

### Go data flow and root cause

Current `internal/agent/turn/state.go:107–109` has `UnverifiedActionChecked`, `MarkUnverifiedActionChecked`, and `ResetUnverifiedActionCheck`, but those flags are inert. `internal/agent/turn/policy.go:24–29` defines only a generic `Policy`/`PolicyFunc`; there is no `UnverifiedActionPolicy`, no command-prose guard interface, and no response-corrector interface. The existing `PolicyChain` can carry response strings, but nothing invokes a corrector or marks the flag conditionally. Therefore a response can narrate an action without successful command evidence and proceed unchanged.

This is not a provider limitation: the policy decision and at-most-once state transition are pure, and the correction call can be injected. The only excluded part is wiring a live provider/correction catalog into production.

### Smallest recommended correction

Add a Stage-2 policy with narrow injected interfaces, for example:

```go
type CommandProseGuard interface { FindCommand(string) (string, bool) }
type ResponseCorrector interface {
    CorrectUnverifiedAction(context.Context, string, []llm.LlmMessage,
        []contract.Definition, string) (string, []llm.LlmMessage, error)
}
type UnverifiedActionPolicy struct { Guard CommandProseGuard; Corrector ResponseCorrector }
```

Use the full `PolicyInput` below rather than adding ad-hoc parameters. Algorithm: if checked, `Continue(response,false)`; otherwise if `!State.HasSuccessfulCommands()` OR guard finds no command, call the injected corrector, replace response/messages, mark checked, and return continuation; otherwise mark checked without correction and pass through. Keep correction-loop policy inside the injected corrector, or explicitly document a bounded fake corrector for Stage 2 rather than silently pretending the LLM loop exists.

### Required tests

* Corrects an unverified prose response when there are no successful commands; verifies corrector call and state mark.
* Corrects when successful commands exist but prose contains no recognized command.
* Does not correct when state is already checked.
* Does not correct when successful command evidence and recognized command prose are both present.
* Resets after a new tool batch and permits one new check.
* Propagates corrector error and does not claim a successful correction.
* Verifies correction messages/response are carried to the next chain policy.

## 2. Incomplete and non-defensive policy value contract

### Saturn contract

`AgentTurnPolicyInput` (`AgentTurnPolicyInput.java:23–90`) is an immutable record containing response, projected messages, tool definitions, `AgentCommandProseGuard`, mutable request-local state, original prompt, correlation ID, and `Optional<String> requiredFreshTool`. Its canonical constructor null-checks all components. The seven-argument constructor defaults the optional to empty.

`AgentTurnPolicyResult` (`AgentTurnPolicyResult.java:13–46`) contains response, `correctionUsed`, and `continuePolicyEvaluation`; it null-checks response and supplies a two-argument constructor defaulting continuation to true. `AgentTurnPolicyChain` (`AgentTurnPolicyChain.java:16–50`) copies/null-checks the policy list, applies in order, carries each replacement response forward, ORs correction flags, and stops before later policies when continuation is false.

### Go data flow and root cause

`internal/agent/turn/policy.go:8–17` has only `Response`, `Messages`, and `State`. It cannot express the definitions, command guard, prompt, correlation ID, or required fresh tool that Saturn policies consume. It also accepts a nil state and nil messages without validation. The chain reconstructs only the reduced input (`policy.go:39`), so even if later fields were added elsewhere they would not propagate. `PolicyChain` stores an exported `Policies []Policy` directly, allowing caller mutation after construction. The existing LLM value types do defensively clone their own nested slices, but that does not make the policy input slices immutable when the input itself is retained.

`PolicyResult` uses a string response, so its response value is intrinsically copied, but the contract shape/constructor validation still differs (`Stop` and `Continue` are exported helpers rather than an equivalent validated value boundary).

### Smallest recommended correction

Expand `PolicyInput` to the Saturn fields, using Go-native equivalents:

```go
type PolicyInput struct {
    Response string
    Messages []llm.LlmMessage
    Definitions []contract.Definition // or the exact immutable definition type selected by turn API
    CommandProseGuard CommandProseGuard
    State *State
    Prompt string
    CorrelationID string
    RequiredFreshTool *string // nil means Optional.empty()
}
```

Validate required interface/pointer/string fields at construction (prefer `NewPolicyInput` returning an error, or an unexported validated representation). Clone messages, definitions, and nested definition data at the boundary; do not retain caller-owned slices. Make `PolicyChain` hold a private copied slice and expose no mutable field. Every chain transition must reconstruct the complete input, changing only `Response`; preserve messages/definitions/guard/state/prompt/correlation/required-tool. Add nil/context-error behavior explicitly rather than relying on a panic.

If `contract.Definition` is not suitable for the policy boundary because it contains mutable `json.RawMessage`, add a turn-local immutable snapshot type or clone on input/access; do not claim a shallow slice copy is defensive.

### Required tests

* Constructor rejects nil state/guard and missing required scalar fields, while optional fresh tool can be empty.
* Mutating original messages, definitions, and nested JSON after construction does not affect policy observations.
* Mutating returned/accessor slices does not affect subsequent observations.
* Chain's second policy sees the first response and all unchanged Saturn fields.
* Chain ORs correction flags and stops before later policies.
* Mutating the source policy slice after chain construction does not change execution order.

## 3. Fresh coordinator lacks correction prompt/LLM exact-call path

### Saturn data flow

`AgentFreshDataCoordinator.process` (`AgentFreshDataCoordinator.java:85–160`) first detects an unsatisfied required tool. For targeted `user_message_history`, it reserves before synthetic execution (`:178–217`), records attempted/outcome/successful tool/result, renders model-visible assistant/tool messages, and calls the LLM for synthesis. For other required tools, it validates an exact call; if invalid, it permits one correction by appending the assistant response and `router-fresh-tool-correction` user prompt, calls the LLM with only relevant definitions, validates the corrected response again, and marks the correction. A second invalid response fails closed. The Saturn correction tests cover disabled tools, exhausted budget, null correction responses, second missing correction, and synthesis failures.

### Go data flow and root cause

`internal/agent/turn/coordinator.go:34–84` has a useful bounded exact-one-call gate and targeted executor path. However, when the call is absent, wrong, malformed, or non-exact (`:46–51`), it only marks `FreshnessCorrectionUsed` and returns the original response with `Corrected: true`. No correction prompt is appended; `Client.Complete` is not invoked; no corrected response is validated; and no second correction result is fail-closed. Thus the flag records a hypothetical correction rather than an actual correction.

The targeted execution path also parses raw arguments after reservation/attempt marking (`:53–68`), whereas malformed input should fail closed before accepted execution and should not be counted as an executed successful/failed lookup. The accepted executor already owns argument validation; the coordinator should not bypass it or invent a competing partial parser.

### Smallest recommended correction

Keep the injected `llm.LlmClient` and `FreshExecutor`, but add injected prompt/definition/response-rendering seams rather than provider wiring:

* Build correction messages from the current message snapshot plus assistant candidate and a bounded fresh-tool correction user message.
* Call `Client.Complete` exactly once for the correction attempt, with only the required tool definition supplied by a `DefinitionProvider` or equivalent.
* Require exactly one corrected call via `MatchesFreshCall`; if invalid, return a stable routing error and never execute.
* Mark `FreshnessCorrectionUsed` only after the correction response has been received/validated (or preserve Saturn's exact marking point if tests require it, but never mark a correction that was not requested).
* For targeted history, create the synthetic call with the requested nick, reserve before executor invocation, execute through `FreshExecutor`, record attempted/result/evidence, append model-visible messages, then call the injected client once for synthesis.
* For synthesis, separate `repeatsPreviousAssistant` and profile-contract checks. Allow at most one injected synthesis correction and require the corrected response to pass the validator.

All of this is testable with a scripted client and counting executor. Live provider prompt catalog and production router loop remain excluded.

### Required tests

* Missing/wrong/malformed non-history call invokes the client once with correction messages and exact required-tool definition.
* Corrected response with zero, multiple, wrong-name, malformed, or wrong-target calls fails without executor invocation.
* A second invalid response fails closed and does not call the client again.
* Successful targeted lookup reserves before executor, records result, renders messages, and performs one synthesis completion.
* Failed lookup records failure and does not perform synthesis.
* Malformed arguments are rejected before executor/accounting where the accepted executor is responsible for validation.
* Cancellation/client error propagates without false success/correction state.

## 4. Final freshness validation is incomplete

### Saturn contract and tests

`AgentFreshDataPolicy.satisfiesProfileContract` (`AgentFreshDataPolicy.java:21–30`) requires a non-null/non-null-entry result list containing a successful `user_message_history` result and nonblank response content. `requiresFinalSynthesisValidation` (`:63–68`) applies this to final responses, including failure placeholders. `requireFreshSynthesis` (`:165–179`) rejects null responses, any tool calls, and synthesis repeating the previous assistant content. The coordinator additionally uses `repeatsPreviousAssistant` and a one-use `freshSynthesisCorrectionUsed` flag. `AgentFreshDataPolicyCorrectionTest`, `AgentFreshDataPolicyTest`, and `AgentFreshDataFinalValidatorTest` cover absent evidence, valid history evidence, failure placeholders, repeated synthesis, tool calls during synthesis, nulls, and second correction failure.

### Go data flow and root cause

`internal/agent/turn/coordinator.go:87–97` `FinalValidator` checks only `state.HasSuccessfulTool(requiredTool)` and non-empty response content for history. `RecordSuccessfulToolResult` exists (`state_results.go:8–13`) but its result is never consulted by `FinalValidator`; it only rejects `IsError`/empty tool name, not empty/invalid result content or the required tool identity. A state can therefore contain the successful-tool marker without a corresponding successful history result. A response containing fresh-tool calls can pass if content is non-empty. A repeated previous assistant synthesis is never compared in the Go coordinator. The synthesis correction flag exists in state but is not used by `FinalValidator` or the coordinator's completed-evidence branch.

The smallest correctness issue is not “add more strings” but make evidence and synthesis validation consume the same data that the coordinator records.

### Smallest recommended correction

Introduce a pure validator input containing response, prior messages/history, required tool, and defensive successful-result snapshot. Validate, in order:

1. No required tool: pass through.
2. Required tool: require at least one non-error successful result whose `ToolName` exactly equals the required tool and whose content is nonblank/valid under the selected result contract.
3. For history synthesis: require nonblank content, no tool calls, and no equality (after Saturn-equivalent trimming) with the latest non-internal assistant message.
4. If invalid and synthesis correction is unused, coordinator performs one injected correction completion and revalidates; otherwise fail closed.

Keep result-schema validation in accepted tool execution where available; the final validator should verify the presence/identity/content contract, not duplicate the executor's full schema engine. Ensure successful result snapshots are cloned and read from state, not from a test-only side channel.

### Required tests

* Successful-tool marker without a matching successful result fails.
* Error result, nil result entry, wrong tool result, and blank result content fail.
* Valid history result plus nonblank fresh synthesis passes.
* Empty/blank response, response with tool calls, repeated prior assistant content, and failure placeholder fail.
* One synthesis correction is attempted and a still-invalid correction fails; a second attempt is never made.
* Internal evidence assistant rows are ignored when selecting the previous assistant for repetition.

## 5. Memory facade and empty-context evidence inspection

### Saturn contract

`AgentTurnMemory` (`AgentTurnMemory.java:38–125`) is an adapter over `AgentMemoryStore` and `AgentResponseSanitizer`. It loads with legacy-persona filtering, rejects null history, translates storage failures to stable public messages while retaining the cause, appends conversation, prevalidates all tool evidence before any append, and preserves input order. `AgentTurnMemoryTest` covers all of these behaviors, including no partial persistence after a null evidence entry.

### Go data flow and root cause

`internal/agent/turn/memory.go` intentionally implements an in-memory `MemoryStore`, not the production memory facade. It does provide per-`MemoryKey` buckets for messages/evidence and stable sentinel errors. However, `AppendEvidence` calls `appendEvidence(api.Context{}, es)` (`:66–67`), and `Evidence()` also reads `m.bucket(api.Context{})` (`:85–88`). Evidence appended through `AppendEvidence` is therefore observable only in the empty context bucket; evidence appended through `AppendToolEvidence(ctx,...)` is not observable through `Evidence()` unless `ctx` itself is empty. This is a deterministic test seam defect, not a Saturn behavior difference.

The deeper “test-only” finding is a scope boundary, not an accidental omission: the target has no production-equivalent `AgentMemoryStore` adapter wired into this slice, and architecture explicitly excludes H2/persistence registration. There is no source-grounded basis to claim production `AgentTurnMemory` parity from `InMemoryStore` tests.

### Smallest recommended correction

Make evidence inspection context/key-specific, e.g. `Evidence(api.Context) []EvidenceEntry`, and have tests query the same context used for append. If retaining a convenience `Evidence()` method for tests, document that it means the empty bucket and add a separate `EvidenceFor(ctx)` method; do not silently conflate buckets. Continue prevalidating the complete batch before locking/appending and return defensive copies.

For Stage 4 parity, add a narrow `TurnMemory` facade over the injected `MemoryStore` interface with load filtering, null checks, stable load/persistence errors, and all-or-nothing evidence prevalidation. Do not add H2 or production registration.

### Required tests

* Two contexts/MemoryKeys retain isolated evidence; querying context A never returns context B.
* Evidence order is preserved per bucket and snapshots are defensive.
* Empty evidence is a no-op.
* A null/invalid batch causes zero writes.
* Load filters all supported legacy persona markers and internal evidence without mutating store data.
* Store failures expose stable public errors and preserve causes internally.

## Exact exclusions / blocked production parity

The following must not be presented as fixed by Stages 1–4 or by this diagnostic:

* Live LLM/provider correction prompts, provider credentials/retries, or production prompt catalog registration. Scripted `llm.LlmClient` injection is sufficient for pure coordinator tests.
* Listener ordering, live router/command registration, chat delivery, participation integration, moderation, final sanitizer, and end-to-end routing.
* H2 schema/repository/transaction/persistence correctness, real `AgentMemoryStore` registration, and production memory retention. The Stage-4 facade may be validated against fakes only.
* Reimplementation of accepted executor authorization, argument/result schema validation, prerequisite ledger, scheduling, or cancellation semantics. Turn code should call `internal/agent/tool/execution` through a narrow adapter.
* Thread safety of `State`; it remains owner-goroutine/request-local. Race tests must exercise the owner/runtime seam, not imply arbitrary concurrent state mutation is safe.
* Claiming that successful-tool names alone constitute successful fresh evidence.

## Acceptance criteria for the next correction pass

1. Saturn `AgentUnverifiedActionPolicyTest` scenarios have Go equivalents with injected guard/corrector, one-use state, reset behavior, and chain response propagation.
2. `PolicyInput` exposes the complete Saturn field set, validates required values, defensively snapshots mutable inputs, and chain reconstruction preserves every field. `PolicyResult` null/constructor semantics and stop behavior are explicit and tested.
3. Non-history fresh-tool correction performs exactly one injected correction completion, validates exactly one correct call, executes none on invalid output, and fails closed on the second invalid attempt. Targeted history still reserves before execution and records a successful result.
4. Final validation requires a matching non-error successful result, nonblank/valid evidence content, and a fresh non-repeated synthesis with no tool calls; one synthesis correction is bounded and revalidated.
5. Memory evidence tests are keyed by the supplied `api.Context`/`MemoryKey`; load/append/evidence snapshots are defensive and evidence batches are all-or-nothing.
6. Focused tests, race tests, full `go test ./...`, `go vet ./...`, and `go build ./...` remain green after implementation. Saturn remains read-only and no listener/router/provider/persistence files change.
7. The final implementation handoff explicitly reports which behaviors are pure/injected and which production integrations remain deferred; no audit rows #128–#143 are called fully closed solely from these package tests.

## Evidence files reviewed

### Target

* `.hermes/handoffs/agent-turn-freshness-qa.md`
* `.hermes/handoffs/agent-turn-freshness-architecture.md`
* `.hermes/handoffs/agent-turn-freshness-implementation.md`
* `internal/agent/turn/state.go`
* `internal/agent/turn/state_results.go`
* `internal/agent/turn/policy.go`
* `internal/agent/turn/freshness.go`
* `internal/agent/turn/coordinator.go`
* `internal/agent/turn/memory.go`
* `internal/agent/turn/turn_test.go`
* `internal/agent/turn/coordinator_test.go`
* `internal/agent/llm/client.go`
* `internal/agent/tool/contract/definition.go`
* `internal/agent/tool/execution/execution.go`

### Saturn

* `src/main/java/org/saturn/app/agent/turn/AgentUnverifiedActionPolicy.java`
* `AgentTurnPolicy.java`, `AgentTurnPolicyInput.java`, `AgentTurnPolicyResult.java`, `AgentTurnPolicyChain.java`
* `AgentFreshDataCoordinator.java`, `AgentFreshDataPolicy.java`, `AgentFreshDataFinalValidator.java`, `AgentFreshDataTurnPolicy.java`
* `AgentTurnMemory.java`
* Focused tests: `AgentUnverifiedActionPolicyTest`, `AgentTurnPolicyChainTest`, `AgentFreshDataCoordinatorTest`, `AgentFreshDataPolicyCorrectionTest`, `AgentFreshDataFinalValidatorTest`, `AgentFreshDataPolicyTest`, `AgentFreshDataTurnPolicyTest`, and `AgentTurnMemoryTest`.
