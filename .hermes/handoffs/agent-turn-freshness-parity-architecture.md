# Next bounded turn/freshness parity slice

**Status:** implementation specification only. This document is source-grounded; it does not modify application code or the Saturn checkout.

**Scope:** close the feasible pure/injected parity gaps identified by `agent-turn-freshness-diagnostic.md`: the complete immutable policy value contract and propagation; injected unverified-action correction; bounded freshness correction and final synthesis validation; and context/key-specific in-memory evidence inspection plus a narrow memory facade. Live provider/router/listener/persistence wiring remains excluded.

## 1. Evidence and constraints

### Target evidence (`zenbot`)

- `internal/agent/turn/policy.go`: current `PolicyInput` has only `Response`, `Messages`, and `State`; `PolicyChain` exposes a mutable `Policies` slice and reconstructs only those three fields.
- `internal/agent/turn/state.go` and `state_results.go`: request-local counters/flags, successful-tool names, and successful `contract.Result` snapshots already exist. State is owner-goroutine/request-local, not a general concurrent object.
- `internal/agent/turn/coordinator.go`: `FreshDataCoordinator` already has injected `llm.LlmClient` and `FreshExecutor`, exact-one-call gating, reservation-before-fresh-executor invocation, result recording, and history message rendering. Its missing-call branch currently marks correction and returns the original response; it does not complete/revalidate a correction.
- `internal/agent/turn/freshness.go`: `MatchesFreshCall` requires the exact tool name and, for `user_message_history`, a JSON object with a matching normalized `nick`; `LatestConversationAssistant` excludes `[Internal tool evidence from ...]` rows.
- `internal/agent/turn/memory.go`: `InMemoryStore` is keyed internally by `api.Context.MemoryKey()`, but `AppendEvidence` and `Evidence()` use `api.Context{}` and therefore cannot inspect a caller-selected bucket.
- `internal/agent/llm/client.go`: provider-neutral immutable-ish values and `LlmClient.Complete(context.Context, LlmRequest)` are the completion seam. `LlmRequest`/messages/tool calls defensively clone their nested values.
- `internal/agent/tool/contract/definition.go`: `Definition` contains `json.RawMessage`; accessors clone it. `Result` carries `CallID`, exact `ToolName`, `Content`, `ErrorCode`, and `IsError`.
- `internal/agent/tool/execution/execution.go`: `Executor.Execute` owns authorization, argument/result schema validation, prerequisites, ledger reservation, timeout/cancellation, and failure accounting. Turn code must adapt to it rather than duplicate it.
- `internal/agent/api/api.go`: `api.Context.MemoryKey()` separates public and whisper buckets; accessors defensively copy context collections. `runtime/contracts.go` and `runtime/runtime.go` provide the existing cancellation, owner, and per-memory-key serialization seams.
- Existing focused tests: `internal/agent/turn/turn_test.go` and `coordinator_test.go`; the QA record reports the current focused/full `go test`, race, vet, build, format, and diff checks green despite parity gaps.

### Saturn evidence (read-only reference)

- `src/main/java/org/saturn/app/agent/turn/AgentTurnPolicyInput.java`: immutable eight-field value: response, messages, definitions, command prose guard, mutable request-local state, original prompt, correlation ID, and optional required fresh tool. Canonical constructor null-checks every component; seven-argument overload defaults `Optional.empty()`.
- `AgentTurnPolicyResult.java`: response, correction-used, continuation; response is non-null; two-argument form defaults continuation true; `stop(response)` sets continuation false.
- `AgentTurnPolicyChain.java`: copies/non-nulls the policy list, applies in order, carries response forward, ORs correction flags, reconstructs the full input, and stops before later policies.
- `AgentUnverifiedActionPolicy.java`: if already checked, pass through; otherwise correct when no successful command exists **or** prose has no recognized command; mark checked after correction and return the replacement response.
- `AgentFreshDataCoordinator.java`: one exact-call correction for non-history required tools; targeted history reserves before synthetic execution, records outcomes/results, renders assistant/tool messages, and completes synthesis; synthesis can receive one bounded correction and is revalidated.
- `AgentFreshDataPolicy.java` and `AgentFreshDataFinalValidator.java`: exact one-call/target validation; successful fresh result and nonblank content contract; fresh synthesis rejects tool calls and repetition of the latest conversation assistant; final validation remains required even for failure placeholders.
- `AgentTurnMemory.java`: load filtering, null-history rejection, stable public load/persistence errors with causes retained, ordered evidence append, and prevalidation of the whole evidence batch before any write.
- Focused Saturn tests read: `AgentUnverifiedActionPolicyTest`, `AgentTurnPolicyChainTest`, `AgentFreshDataCoordinatorTest`, `AgentFreshDataPolicyTest`, `AgentFreshDataPolicyCorrectionTest`, `AgentFreshDataFinalValidatorTest`, and `AgentTurnMemoryTest`.

`MIGRATION_PLAN.md` and `.hermes/migration-audit.md` establish that agent integration is later work; the current target worktree is already dirty/untracked. Preserve unrelated changes.

## 2. Module/file map

### Modify only the bounded turn package and its tests

| File | Responsibility in this slice |
|---|---|
| `internal/agent/turn/policy.go` | Full immutable `PolicyInput`/`PolicyResult`, injected guard/corrector interfaces, private policy-chain storage, complete-field propagation, cancellation/error behavior. |
| `internal/agent/turn/coordinator.go` | Injected correction completion, definition selection, exact-call revalidation, targeted-history synthesis sequencing, bounded synthesis correction, and final evidence validation. |
| `internal/agent/turn/freshness.go` | Keep freshness extraction and target matching; add pure helpers only where needed for synthesis/repetition and strict JSON call validation. |
| `internal/agent/turn/state_results.go` | Expose defensive successful-result snapshots suitable for final validation; do not replace existing turn counters/ledger semantics. |
| `internal/agent/turn/memory.go` | Key-specific evidence inspection and narrow injected memory facade. Keep all-or-nothing validation and stable sentinels. Do not add persistence. |
| `internal/agent/turn/turn_test.go` | Policy contract, guard/corrector, chain propagation, and memory bucket tests. |
| `internal/agent/turn/coordinator_test.go` | Scripted LLM/executor tests for correction limits, exact calls, targeted lookup, synthesis, and final validation. |
| Optional new `internal/agent/turn/policy_test.go`, `freshness_policy_test.go`, `memory_test.go` | Use only if splitting the currently compact tests improves isolation; no production dependency is implied. |

### Reuse without modification

- `internal/agent/llm/client.go` and its OpenAI adapter/tests for request/response values and completion cancellation.
- `internal/agent/tool/contract` and `internal/agent/tool/execution` for definitions, argument/result validation, authorization, prerequisites, ledger, and cancellation codes.
- `internal/agent/api.Context` for exact `MemoryKey()` isolation.
- `internal/agent/assemble` for required-tool extraction and internal-evidence filtering.
- `internal/agent/runtime` for owner/lifecycle and same-key serialization tests.

No listener, `cmd/zenbot/main.go`, factory, provider configuration, H2/repository, or production memory-registration file is part of this change.

## 3. Immutable value contracts

### `PolicyInput`

Recommended Go boundary (names may remain idiomatic, but all fields are required unless noted):

```go
type PolicyInput struct {
    Response           llm.LlmResponse
    Messages           []llm.LlmMessage
    Definitions        []contract.Definition
    CommandProseGuard  CommandProseGuard
    State              *State
    Prompt             string
    CorrelationID      string
    RequiredFreshTool  *string // nil is Optional.empty()
}
```

Implement `NewPolicyInput(...) (PolicyInput, error)` or an equivalent validated constructor. Reject nil state, nil guard, nil message/definition slices if the Saturn contract treats them as required, and blank prompt/correlation ID if the selected Go boundary adopts Saturn's non-null-plus-validation rule. `RequiredFreshTool == nil` is valid. Reject a nil response only if the Go response type can represent nil; `llm.LlmResponse` is currently a value, so document that distinction.

At construction and on access, defensively snapshot:

- `Messages`, including nested tool calls/arguments (the LLM package already supplies cloning helpers through constructors/accessors).
- `Definitions`, including `Parameters` raw JSON; do not rely on a shallow slice copy.
- Any optional string pointer.

Do not expose mutable slices directly. A policy receives a value snapshot. `State` is intentionally mutable request-local state shared by policies, exactly as Saturn does; it is not made thread-safe by this change.

### `PolicyResult`

```go
type PolicyResult struct {
    Response       llm.LlmResponse
    CorrectionUsed bool
    Continue       bool
}
```

The response is a value. `Continue(response, correction)` returns continuation=true; `Stop(response)` returns continuation=false and correction=false. Policy implementations must return a valid result even when no correction occurred. A policy error aborts the chain and the caller must not treat the partial response as a successful final answer.

### Policy interfaces

```go
type Policy interface {
    Apply(context.Context, PolicyInput) (PolicyResult, error)
}

type CommandProseGuard interface {
    FindCommand(string) (command string, ok bool)
}

type ResponseCorrector interface {
    CorrectUnverifiedAction(context.Context, llm.LlmResponse,
        []llm.LlmMessage, []contract.Definition, string) (
        llm.LlmResponse, []llm.LlmMessage, error)
}
```

The corrector owns its own provider-response recovery policy if supplied by a later integration. This slice injects a scripted fake; it must not invent a sanitizer or silently implement an unbounded provider loop.

### `PolicyChain`

Store a private copied `[]Policy`, rejecting nil members. For each policy, check `ctx.Err()` before invocation; construct the next `PolicyInput` with **all eight fields**, changing only `Response`. OR `CorrectionUsed` across results. Stop immediately when `Continue` is false. Return the last response, aggregate correction flag, and final continuation state. A context cancellation or policy error is returned unchanged (or wrapped with `%w`) and no later policy executes.

## 4. Unverified-action policy and correction limits

Add `UnverifiedActionPolicy` in `policy.go` (or a dedicated turn file) with injected `CommandProseGuard` and `ResponseCorrector`.

Algorithm:

1. Validate input dependencies at construction/use.
2. If `State.UnverifiedActionChecked()` is true, return the original response, `CorrectionUsed=false`, `Continue=true`; do not call the corrector.
3. If `!State.HasSuccessfulCommand(any)` (a state-level “has any” helper may be needed) **or** `FindCommand(response.Content())` returns no command, invoke the corrector with the current response, a message snapshot, definitions, and correlation ID. Replace both response and messages in the chain-visible input and then mark `UnverifiedActionChecked`.
4. If successful command evidence exists and recognized command prose exists, pass through and mark the check only if matching Saturn’s actual state transition is intentionally selected; tests must pin the choice. The observed Saturn implementation marks only after correction, so default parity is to leave the flag unchanged on the pass-through branch.
5. Corrector errors propagate and must not mark the check or claim correction.
6. A new accepted tool batch resets only this check via `ResetUnverifiedActionCheck`; it must not reset freshness/synthesis flags or disablement.

The policy itself performs at most one check per reset. Any second provider correction attempt belongs inside an injected corrector with an explicit bounded limit; do not create a hidden loop in the policy.

## 5. Freshness coordinator contract and state machine

Retain the current injected seams:

```go
type FreshExecutor interface {
    Execute(context.Context, api.Context, execution.Call) contract.Result
}

type DefinitionProvider interface {
    DefinitionsFor([]contract.Definition, string) ([]contract.Definition, error)
}

type ToolResultRenderer interface {
    Render(api.Context, llm.LlmToolCall, contract.Result) string
}
```

If definitions are passed directly in the existing Go API, a small provider adapter may filter to the required tool. The completion request for correction must contain only the required tool definition, mirroring Saturn; synthesis completion contains no tool definitions.

### Process transitions

**A. No required fresh tool / already satisfied:** return a defensive message snapshot, `Corrected=false`; do not require client/executor.

**B. Required non-history tool missing:**

- If tools are disabled or budget cannot permit the correction path, fail closed with a stable routing error; do not call executor.
- Validate current response as exactly one tool call: `len(ToolCalls()) == 1`, exact required name, and valid JSON object arguments. For `user_message_history`, require a string `nick` equal to the requested normalized target, case-insensitively. Reject null, array, scalar, malformed JSON, missing/wrong `nick`, wrong name, zero calls, and multiple calls.
- If invalid and freshness correction already used, return `required fresh-data tool call missing` (or the chosen stable equivalent). Do not call LLM again.
- Otherwise append the current assistant candidate and a bounded `router-fresh-tool-correction` user message to a copied message list, call `Client.Complete` exactly once with only the required definition, and revalidate the returned response using the same exact validator.
- Mark `FreshnessCorrectionUsed` only after the correction response was received and passed exact-call validation. Never mark a correction that was not requested or that returned an invalid response.
- Only a validated exact call can reach `FreshExecutor`; invalid corrected responses execute zero calls.

**C. Targeted `user_message_history` lookup:**

- Reserve one turn-wide tool call before executor invocation. On failure, disable tools and return the existing stable budget error; execute zero calls.
- Construct the synthetic call with a deterministic nonempty ID and `{"nick": requestedNick}`. Call the accepted `FreshExecutor`; do not bypass its argument/schema/authorization checks.
- After result: error result records failure and terminates the mandatory fresh path; success records balanced tool success, exact successful tool name, and a defensive successful `contract.Result` snapshot.
- Append model-visible assistant tool-call and tool-result messages in order. Render through an injected renderer; do not hand-format a competing result envelope.
- Complete synthesis with the injected `LlmClient` exactly once using the message snapshot and the original definitions. Return `Corrected=true`/restart indication only after successful completion.

**D. Fresh synthesis:**

After required evidence exists, validate the candidate before returning it:

- non-nil/nonblank content;
- no tool calls in a synthesis response;
- latest previous *conversation* assistant comparison uses `LatestConversationAssistant`, excluding internal evidence rows, and compares Saturn-equivalent trimmed content;
- required history evidence contract is satisfied by a non-error successful result whose `ToolName` is exactly `user_message_history` and whose content is nonblank/valid under the already accepted result contract.

If synthesis is invalid and `FreshSynthesisCorrectionUsed` is false, append candidate assistant plus bounded `router-fresh-synthesis-correction`, complete once with no tools, mark the flag only after receiving the correction, and revalidate all conditions. If still invalid, or the flag was already set, fail closed with `Agent did not produce a complete fresh history synthesis` (stable public text). A null LLM response is an error, not a successful correction.

## 6. Final evidence/synthesis validation

Replace the current marker-only `FinalValidator` check with a pure input that consumes state snapshots and prior history, for example:

```go
type FinalValidationInput struct {
    Response       llm.LlmResponse
    History        []llm.LlmMessage
    RequiredTool   string
    Successful     []contract.Result
}
```

Validation order:

1. Empty `RequiredTool`: pass through; final validation is not a freshness requirement.
2. Required tool: find at least one non-error result with exact `ToolName == RequiredTool`; reject nil/invalid entries and a successful-tool name marker with no matching result.
3. For `user_message_history`: require nonblank evidence content and nonblank final content. Do not duplicate executor schema validation; trust the executor's accepted result validator but verify the result is present, successful, exact-tool, and content-bearing.
4. Reject any tool calls in final synthesis.
5. Reject response content equal to the latest non-internal assistant content after trimming.
6. Keep the one-use synthesis correction in the coordinator/state, not inside a validator that has no side effects.

`SuccessfulToolResults()` must return defensive copies, including `json.RawMessage`-like nested values if later added. A state marker alone never satisfies final evidence.

## 7. Context/key-specific memory inspection and narrow facade

### In-memory store correction

Preserve `api.Context.MemoryKey()` as the sole bucket key. Add an explicit inspection API such as:

```go
func (m *InMemoryStore) EvidenceFor(ctx api.Context) []EvidenceEntry
```

It must return a defensive copy from `m.bucket(ctx)`. Either change `Evidence()` to accept a context or retain it only as a documented empty-context convenience; the preferred API is context-specific and tests must never use the empty bucket accidentally. Add an explicit context-aware batch method, e.g. `AppendEvidenceFor(ctx, entries)`, while retaining compatibility only if it cannot reintroduce ambiguity.

Required isolation tests use two independently created contexts/keys, including public versus whisper, append evidence to each, and assert cross-key reads are empty. Preserve input order and no partial writes: validate every entry (`Tool` and `Content` nonblank) before locking/appending; empty input is a no-op.

### Narrow `TurnMemory` facade

If a facade is added, it wraps the injected `MemoryStore` only:

```go
type TurnMemory struct { store MemoryStore }
func (m TurnMemory) Load(ctx api.Context, correlationID string) ([]llm.LlmMessage, error)
func (m TurnMemory) Append(ctx api.Context, user, assistant, correlationID string) error
func (m TurnMemory) AppendToolEvidence(ctx api.Context, []EvidenceEntry, correlationID string) error
```

Load rejects a nil returned history, filters supported legacy persona markers and internal evidence without mutating stored messages, and returns `ErrMemoryLoad` while retaining the cause internally if the implementation wraps errors. Append paths return `ErrMemoryPersistence` with stable public text and preserve underlying causes for diagnostics. Evidence batches are prevalidated before the first delegate call and retain order. This is a fake/in-memory contract test only; it is not production `AgentTurnMemory` registration.

Do **not** add a response sanitizer. Use the existing narrow legacy/internal filtering behavior already present in `memory.go` and assembler/history helpers; full Saturn `AgentResponseSanitizer` and final output sanitization are excluded.

## 8. Error and cancellation behavior

- Context cancellation/deadline is checked before every correction completion, executor call, and state mutation where practical; propagate `ctx.Err()` or existing typed LLM/execution errors. Do not convert cancellation into successful evidence or a normal reply.
- Client errors propagate without marking correction-used. A nil/empty required correction response fails closed with a stable routing error.
- Invalid exact calls are routing failures; they never execute and never count as successful evidence. The coordinator must not parse/validate schema differently from the accepted executor.
- Required lookup error results record failure then terminate the mandatory freshness path. They cannot satisfy final validation.
- Budget rejection happens before fresh execution and disables tools where the current state/budget contract requires it. Never reserve after execution.
- Public error strings must not include provider diagnostics, credentials, SQL text, or raw arguments. Preserve causes for internal tests/diagnostics.
- State remains request-local and owner-controlled. Tool workers must not mutate turn state directly; aggregate under the owner before final validation.

## 9. Test matrix

### Policy/value tests

- Constructor rejects nil state/guard and invalid required scalars; nil required-tool is accepted.
- Mutating caller messages, definitions, nested definition JSON, or accessor results does not change later policy observations.
- Chain second policy sees first response plus unchanged definitions, guard, state, prompt, correlation ID, and required tool.
- Chain ORs correction flags, stops before later policies, checks context cancellation, rejects nil policy entries, and is unaffected by source-slice mutation after construction.
- Unverified prose with no successful command invokes corrector and carries corrected response/messages; checked state bypasses corrector; recognized command plus successful command passes through; corrector error leaves state unmarked; reset enables one new check.

### Freshness/coordinator tests

- No required tool passes through without client/executor.
- Missing/wrong/malformed/multiple required calls invoke exactly one correction completion with only the required definition.
- Corrected zero, multiple, wrong-name, malformed, wrong-target, non-object, and missing-nick calls fail closed and execute zero calls.
- Second invalid response never invokes client again.
- Exact successful call reserves before executor; failed reservation executes zero calls; successful targeted history records exact result, renders messages, and performs one synthesis completion.
- Error result records failure and never synthesizes a supported fresh answer.
- Client cancellation/error and nil response do not mark correction or success.
- Final validation rejects marker-only success, error result, nil entry, wrong tool, blank result content, blank final content, final tool calls, repeated previous assistant, and failure placeholder; accepts exact successful history result plus fresh nonblank synthesis.
- Internal evidence assistant rows are ignored for repetition. One synthesis correction is attempted at most once and is revalidated.

### Memory tests

- Two `api.Context`/`MemoryKey` buckets remain isolated; public and whisper evidence never cross.
- Evidence order and defensive snapshots hold.
- Empty evidence is a no-op.
- Null/invalid batch causes zero delegate writes.
- Load filtering removes all currently supported legacy markers and internal evidence without mutating storage.
- Load/persistence failures expose stable sentinels and preserve causes internally.

## 10. Acceptance commands

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation:

```bash
go test -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble
go test -race -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble
go test -count=1 ./...
go test -race ./...
go vet ./...
go build ./...
gofmt -d internal/agent/turn/*.go
git diff --check
```

`gofmt -d` must produce no diff; use `gofmt -w` only on intentionally modified turn files. Verify application scope with:

```bash
git status --short
git diff --name-only -- internal/agent/turn
```

The final review must confirm no Saturn file changed and no listener/router/provider/persistence application file was added to this slice. Existing unrelated dirty/untracked files are not grounds for cleanup or reset.

## 11. Explicit exclusions: Saturn behavior requiring production wiring

The following must remain explicitly out of scope and must not be represented as closed by these package tests:

1. Live provider correction prompts, prompt catalog/resource registration, credentials, provider retries, and production LLM availability. Scripted `llm.LlmClient` injection proves only the seam.
2. Live router/listener/command registration, participation dispatch, chat delivery, listener ordering, moderation, protected-principal checks, and end-to-end routing.
3. H2 schema/repository/transactions, `AgentMemoryStore` production adapter/registration, retention, and real persistence evidence. The narrow facade is validated only against fakes/in-memory data.
4. Saturn `AgentResponseSanitizer`, response finalizer, quote/no-reply/marker handling, output truncation, and any invented Go response sanitizer.
5. Reimplementation of accepted executor authorization, argument/result schema validation, prerequisites, ledger scheduling, parallel read-only barriers, or cancellation semantics. Fresh orchestration calls `internal/agent/tool/execution` through `FreshExecutor`.
6. General DAG scheduling or arbitrary concurrent state mutation. Existing execution supports bounded ordered batches/barriers; `State` remains owner-goroutine/request-local.
7. Production registration of multiple freshness tools. The observed policy currently names `user_message_history`; additional capabilities require a separately specified integration.
8. Claiming audit rows #128–#143 complete. This slice can add pure/injected parity evidence only; production migration remains gated by the excluded integrations.

## 12. Definition of done

- Full eight-field policy input is validated, defensively snapshotted, and propagated unchanged through the chain except for response.
- Injected unverified-action guard/corrector has Saturn-equivalent conditions, message/response propagation, reset behavior, and bounded correction semantics.
- Fresh correction performs one completion, exact-call revalidation, zero execution on invalid output, and fail-closed second failure; targeted history reserves first and records an exact successful result.
- Final validator consumes real successful result snapshots, rejects stale/incomplete/tool-call synthesis, performs at most one correction, and revalidates.
- Memory inspection is explicitly keyed by supplied context; facade behavior is narrow, defensive, all-or-nothing, and stably error-translated.
- Focused, race, full, vet, build, formatting, and diff checks pass; no application code outside the bounded turn package/tests is modified; Saturn remains read-only.
