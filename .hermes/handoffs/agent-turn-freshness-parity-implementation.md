# Bounded turn/freshness parity implementation

## Outcome

Implemented the bounded pure/injected parity slice in `internal/agent/turn/`, following the architecture and diagnostic. The implementation uses the existing `llm.LlmResponse` value type and does not modify the LLM API or Saturn checkout.

## Exact task-owned files

- `internal/agent/turn/policy.go`
  - Added the full eight-field policy input shape: response, messages, definitions, command prose guard, request-local state, prompt, correlation ID, and optional required fresh tool.
  - Added validated construction and defensive message/definition/optional-string snapshots.
  - Changed policy result helpers to use `llm.LlmResponse`.
  - Added copied policy-chain construction, full-field propagation, response carry-forward, correction OR aggregation, stop short-circuit, and context checks.
  - Added injected `CommandProseGuard`/`ResponseCorrector` and bounded `UnverifiedActionPolicy`; correction messages are carried to subsequent policies.
- `internal/agent/turn/coordinator.go`
  - Added injected definition-selection and result-rendering seams.
  - Added exact-one-call correction completion for non-history tools, strict JSON-object/exact-name validation, one freshness correction limit, and fail-closed invalid correction behavior.
  - Preserved targeted history synthetic-call flow, reserve-before-execute ordering, successful result recording, and renderer use.
  - Added synthesis validation and one bounded synthesis correction with revalidation.
  - Final validation now requires an actual successful exact-tool result with nonblank content, nonblank synthesis, no final tool calls, and no repeated latest conversation assistant.
- `internal/agent/turn/state.go`
  - Added request-local `HasAnySuccessfulCommand` helper.
- `internal/agent/turn/state_results.go`
  - Successful result recording rejects error, missing-tool, and blank-content results.
- `internal/agent/turn/memory.go`
  - Added `AppendEvidenceFor`/`EvidenceFor` context-key-specific APIs.
  - Added narrow injected `TurnMemory` facade with stable sentinel translation, filtering, defensive loads, and prevalidated evidence batches.
  - Preserved all-or-nothing in-memory batch append behavior and redacted sentinel errors.
- `internal/agent/turn/turn_test.go`
  - Adapted existing policy tests to the actual `llm.LlmResponse` boundary.
- `internal/agent/turn/coordinator_test.go`
  - Updated the budget rejection expectation to match fail-closed freshness behavior.
- `internal/agent/turn/parity_red_test.go`
  - Added RED-first parity coverage for policy snapshots/propagation, unverified correction/reset, result-backed synthesis validation, and context-specific evidence buckets.

## Source references

- Target: `internal/agent/turn/policy.go`, `coordinator.go`, `freshness.go`, `state.go`, `state_results.go`, `memory.go`.
- Existing seams reused: `internal/agent/llm/client.go`, `internal/agent/tool/contract/definition.go`, `internal/agent/tool/execution/execution.go`, `internal/agent/api/api.go`.
- Saturn references from the architecture/diagnostic: `AgentTurnPolicyInput`, `AgentTurnPolicyResult`, `AgentTurnPolicyChain`, `AgentUnverifiedActionPolicy`, `AgentFreshDataCoordinator`, `AgentFreshDataPolicy`, `AgentFreshDataFinalValidator`, and `AgentTurnMemory`.

## TDD evidence

A RED test run was captured before implementation:

- `go test ./internal/agent/turn` — FAIL, with expected missing `NewPolicyInput`, `NewPolicyChain`, `NewUnverifiedActionPolicy`, `EvidenceFor`, `AppendEvidenceFor`, and `ValidateWithHistory` contracts, plus the old string response mismatch.

After the implementation and test adaptation:

- `go test -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — PASS
- `go test -race -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — PASS
- `go test -count=1 ./...` — PASS
- `go test -race ./...` — PASS
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `gofmt -d internal/agent/turn/*.go` — no output / PASS
- `git diff --check` — PASS

## Scope verification

The task-owned application changes are confined to `internal/agent/turn/` plus this handoff. No Saturn files were modified. No listener, router, provider, command wiring, H2/persistence registration, moderation, or sanitizer integration was added.

## Limitations and exclusions

- This is a pure/injected bounded seam; no live provider correction prompt/catalog, router/listener registration, production memory registration, or persistence implementation is claimed.
- Accepted executor validation/authorization/ledger behavior remains the source of truth; this slice does not reimplement that system.
- `State` remains request-local/owner-controlled and is not made generally concurrent-safe.
- The Go policy input is defensively snapshotted at construction and at chain boundaries, but its exported slice fields remain Go-visible fields; callers should use the validated constructor and treat values as immutable snapshots.
- The narrow `TurnMemory` facade is fake/in-memory contract coverage only and does not close production Saturn memory parity.
- No claim is made that audit rows #128–#143 or full production turn/router migration are closed.
