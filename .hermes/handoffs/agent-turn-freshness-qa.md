# Independent QA: turn/freshness Stages 1–4

**Verdict: FAIL (focused implementation is test/build green, but not full Saturn parity).**

## Scope and source grounding

Reviewed the target `internal/agent/turn/` implementation and tests against Saturn read-only sources under `src/main/java/org/saturn/app/agent/turn/`, `src/main/java/org/saturn/app/agent/tool/execution/AgentToolBudgetPolicy.java`, and corresponding focused tests, especially `AgentTurnStateTest`, `AgentFreshnessPolicyTest`, `AgentFreshDataCoordinatorTest`, `AgentFreshDataPolicyCorrectionTest`, `AgentFreshDataFinalValidatorTest`, `AgentMessageHistoryTest`, `AgentNickNormalizerTest`, `AgentTurnMemoryTest`, `AgentTurnPolicyChainTest`, and `AgentUnverifiedActionPolicyTest`.

No Saturn files were edited. Existing unrelated dirty/untracked changes in both worktrees were preserved.

## Findings and scoped fixes

### Fixed

1. **Freshness recognition was materially narrower than Saturn.** The Go policy only recognized a subset of profile/history phrases and only three exact follow-ups. It missed possessive forms, speech/history forms, Unicode/general bounded forms, and Saturn's non-nick exclusions. Added bounded Unicode-aware patterns for profile, possessive, `who is`, speech, history/activity, and follow-ups, with legacy/non-nick term exclusions and escaped-underscore normalization.
2. **Fresh required-tool coordination accepted any matching call among a multi-call response.** Saturn requires exactly one required call for correction validation. The coordinator now requires exactly one response tool call before execution.
3. **Successful fresh results were not recorded in the successful-result snapshot.** The coordinator now records the successful `contract.Result` after balanced evidence accounting.
4. **No-required-tool calls unnecessarily required client/executor dependencies.** Dependency validation is now conditional on a required tool, matching pass-through behavior.
5. **In-memory legacy filtering only recognized two exact marker strings.** Filtering now covers Saturn-style persona markers (`[sips tea`, `the archives reveal`, `carpe diem`, and `your history shows:`) case-insensitively while continuing to remove internal evidence.
6. Added focused regression coverage for Unicode/possessive/speech/follow-up freshness recognition; focused normal and race tests pass.

### Remaining parity failures / explicit limitations

1. **No Go equivalent of Saturn `AgentUnverifiedActionPolicy` exists.** State has mark/reset flags, but no command-prose guard/corrector policy that performs the at-most-once correction under Saturn's conditions. This is a real Stage 2 parity gap.
2. **No Go equivalents of Saturn `AgentTurnPolicyInput` / `AgentTurnPolicyResult` carrying the full immutable contract exist.** Go `PolicyInput` is only response/messages/state, and slices are not defensively retained by the policy contract. The ordered chain/response propagation/stop behavior is present and tested, but the exact contract is not.
3. **Fresh coordinator is a bounded seam, not Saturn's full correction path.** Missing/wrong/malformed required calls set a correction flag and return a marker result, but do not issue Saturn's correction prompt/LLM exact-call validation. Non-history required-tool correction is therefore not production-parity complete.
4. **Freshness final validation is weaker than Saturn.** Go checks successful-tool presence and non-empty history content, but does not validate the full successful evidence/result contract or repeated/stale synthesis behavior from `AgentFreshDataPolicy`.
5. **Memory facade is intentionally an in-memory test seam, not `AgentTurnMemory` over the production memory API.** Its evidence inspection is tied to the empty-context test bucket; no production registration/persistence adapter is claimed.
6. No live router/provider/listener/H2/moderation/final-sanitizer integration was assessed or changed.

## Invariants checked

- Step boundary and incremental reservation: present; reservations occur before fresh executor call.
- Budget exhaustion: `ReserveToolBatch` disables tools and selects finalize-only; fresh lookup rejects exhausted budget before execution.
- Permanent disablement: `DisableTools` is idempotent and budget adapter prevents later tool batches.
- Correction flags: independent state flags and unverified reset/mark operations present; full unverified policy missing as noted above.
- Idempotent command/tool sets: present; returned key slices and successful-result slices are defensive copies.
- Evidence balance: success/failure recording rejects over-recording; successful-result snapshots are copied; fresh coordinator now records successful results.
- Policy chain: ordered response carry-forward, correction OR, and stop short-circuit are present and tested.
- Internal evidence/latest assistant: internal prefix filtering and backward latest assistant selection are present (target assembler uses trimmed-prefix behavior; Go helper keeps exact Saturn prefix semantics).
- Unicode nick normalization: trim, escaped underscore, optional mention removal, Unicode letter/number bounds covered.
- Freshness: exact tool name, exact-one-call requirement, target matching, malformed/wrong target fail-closed before executor; one correction flag; lookup reservation/execution and successful evidence recording.
- Memory: per-`MemoryKey` buckets, load filtering, batch prevalidation, order preservation, and stable redacted sentinel errors present; production `AgentTurnMemory` parity remains excluded.

## Verification commands and actual results

- `go test -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — **PASS**
- `go test -race -count=1 ./internal/agent/turn ./internal/agent/tool/execution ./internal/agent/assemble` — **PASS**
- `go test -count=1 ./...` — **PASS**
- `go test -race ./...` — **PASS**
- `go vet ./...` — **PASS**
- `go build ./...` — **PASS**
- `gofmt -w internal/agent/turn/*.go` — **PASS**
- `git diff --check` — **PASS**

The prior implementation handoff records the initial contract RED (`go test ./internal/agent/turn` failed with undefined turn contracts before implementation) and focused GREEN. For the scoped parity fixes in this QA pass, no defect-specific RED run was captured before editing; therefore no stronger RED/GREEN claim is made for those fixes.

## Files changed by this QA pass

- `internal/agent/turn/freshness.go`
- `internal/agent/turn/coordinator.go`
- `internal/agent/turn/memory.go`
- `internal/agent/turn/turn_test.go`
- This handoff: `.hermes/handoffs/agent-turn-freshness-qa.md`

The target already contained extensive unrelated modified/untracked migration files; this pass did not revert or edit them. Saturn already contained unrelated pre-existing dirty/untracked files; this pass did not edit them.

## Explicit exclusions

No claim of full turn/router migration, audit rows #128–#143 closure, live provider behavior, listener/command registration, H2/persistence correctness, moderation safety, final output sanitizer parity, production memory registration, or end-to-end chat delivery.
