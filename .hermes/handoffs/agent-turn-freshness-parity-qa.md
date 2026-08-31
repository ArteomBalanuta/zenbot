# Independent QA: bounded turn/freshness parity

## Verdict

**PASS for the bounded pure/injected turn slice, with production exclusions below.**
The implementation and the scoped corrective changes were exercised against the target Go tests and compared with the cited Saturn sources/tests. No overall migration-completion claim is made.

## Source-grounded review

Reviewed target:

- `internal/agent/turn/policy.go`, `coordinator.go`, `freshness.go`, `state.go`, `state_results.go`, `memory.go`
- `internal/agent/turn/turn_test.go`, `coordinator_test.go`, `parity_red_test.go`
- `internal/agent/llm/client.go`, `internal/agent/tool/contract/definition.go`
- architecture, diagnostic, and implementation handoffs

Reviewed Saturn read-only sources:

- `src/main/java/org/saturn/app/agent/turn/AgentTurnPolicyInput.java`
- `AgentTurnPolicyResult.java`, `AgentTurnPolicyChain.java`, `AgentUnverifiedActionPolicy.java`
- `AgentFreshDataCoordinator.java`, `AgentFreshDataPolicy.java`, `AgentFreshDataTurnPolicy.java`
- `AgentTurnMemory.java`, `AgentResponseSanitizer.java`
- focused Saturn tests named by the architecture/diagnostic, including `AgentFreshDataCoordinatorTest`, `AgentFreshDataPolicyTest`, `AgentFreshDataPolicyCorrectionTest`, `AgentFreshDataFinalValidatorTest`, `AgentUnverifiedActionPolicyTest`, and `AgentTurnMemoryTest`

Confirmed behaviors in target include the full eight-field policy input boundary, defensive nested LLM/definition snapshots, all-field chain propagation, copied private chain storage, correction/error/stop aggregation, exact required-tool validation, bounded correction and synthesis paths, reserve-before-execute targeted history, real successful-result final validation, context-keyed evidence, and narrow memory facade behavior.

## Defects found and fixed in this QA pass

1. **Policy-chain mutability seam:** `PolicyChain` retained an exported mutable `Policies` slice and supported reconstruction from it. Removed the exported field; `NewPolicyChain` now owns the copied private slice. Updated the local construction test.
2. **Unverified correction result parity:** Saturn's `AgentUnverifiedActionPolicy` returns `correctionUsed=false`; the Go implementation incorrectly reported true. Corrected the result while retaining corrected response/message propagation and state marking.
3. **Disabled-tool correction:** non-history missing-tool correction could call the injected LLM while tools were disabled. Added fail-closed check before correction completion; added call-count regression coverage.
4. **Strict JSON call contract:** non-history exact correction accepted empty raw arguments. It now requires a non-empty JSON object for every required tool; added regression coverage.
5. **Correction message seam:** correction assistant message now carries no tool calls and the user message uses the Saturn-equivalent bounded fresh-tool correction text rather than a marker-only string.
6. **Legacy history filtering:** `TurnMemory.Load` now removes the preceding user turn when its legacy assistant turn is excluded, matching `AgentResponseSanitizer.excludeLegacyPersonaTurns`; added regression coverage.
7. **Memory validation/error surface:** evidence validation now rejects whitespace-only tool/content. Memory failures retain causes while exposing stable public sentinel text without leaking internal diagnostics; `errors.Is` remains supported.
8. **Policy constructor validation:** `NewPolicyInput` now rejects nil required message/definition slices in addition to nil state/guard and blank identity fields; optional required-fresh-tool remains valid as nil.

## Commands and actual results

All commands ran from `/Users/ab/workspace/go-projects/zenbot`:

- `gofmt -w internal/agent/turn/*.go` — PASS
- `go test -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — PASS
- `go test -race -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — PASS
- `go test -count=1 ./...` — PASS (all packages green; H2 tests included)
- `go test -race ./...` — PASS (all packages green)
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `gofmt -d internal/agent/turn/*.go` — PASS, no output
- `git diff --check` — PASS

## Files modified by this QA pass

- `internal/agent/turn/policy.go`
- `internal/agent/turn/coordinator.go`
- `internal/agent/turn/memory.go`
- `internal/agent/turn/parity_red_test.go`
- `internal/agent/turn/turn_test.go`
- this handoff: `.hermes/handoffs/agent-turn-freshness-parity-qa.md`

The target worktree contains many unrelated pre-existing staged/dirty/untracked migration files; they were preserved. The Saturn checkout was not modified by this QA pass (it had unrelated pre-existing dirty files when inspected).

## Limitations and explicit exclusions

- This validates only pure/injected seams and scripted LLM/executor behavior. It does not prove live provider correction prompts, provider retries/credentials, or production prompt registration.
- No live router/listener/command wiring, participation, delivery ordering, moderation, sanitizer/finalizer, or end-to-end routing was changed or claimed.
- `TurnMemory` and `InMemoryStore` are narrow/fake in-memory contracts; H2/persistence, production memory adapter/registration, retention, and transactions remain excluded.
- Accepted executor authorization, schema validation, prerequisites, ledger behavior, and cancellation implementation remain the source of truth and were not reimplemented.
- `State` remains request-local/owner-controlled, not generally concurrent-safe.
- Exported `PolicyInput` fields remain a Go-visible representation; callers must use the validated constructor and treat the resulting snapshot as immutable. No response sanitizer was added.
- Audit rows #128–#143 and full production migration are not closed by this report.
