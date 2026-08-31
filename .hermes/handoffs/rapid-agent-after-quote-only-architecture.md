# Rapid live-agent parity after verified quote-only: fail-closed internal tool-evidence delivery guard

## Decision

**Select one bounded live capability: reject a final assistant response containing Saturn’s internal tool-evidence sentinel before it can be delivered or persisted.**

This is the smallest highest-value remaining observable agent guard after verified quote-only delivery. Quote-only already replaces public **no-tool** prose, but deliberately exempts actual tool attempts so fresh `user_message_history`/`room_users` answers can be useful. That exemption leaves tool-backed synthesis as the remaining path where a provider can expose its model-only envelope or historical evidence framing to a user.

The selected guard is deterministic and fail-closed. It does not port Saturn’s provider correction/retry machinery, add a tool, widen history, change memory/evidence storage, or alter the accepted three-tool/one-call/two-completion protocol.

## Evidence map

### [OBSERVED] Saturn contract

| Evidence | Verified behavior |
|---|---|
| `src/main/java/org/saturn/app/agent/routing/AgentResponseCorrector.java`, `correctInternalToolEvidence` | Saturn identifies a final no-tool response containing the exact case-sensitive marker `[Internal tool evidence from `, requests one user-facing correction with tools omitted and prompt cache bypassed, then throws if the correction still contains the marker or calls a tool. |
| Same source, `containsInternalToolEvidence` | The source sentinel is a literal substring check. It does not parse arbitrary JSON, tool names, command prose, or user wording; a response with tool calls is not a final-leak candidate. |
| `src/test/java/org/saturn/app/agent/routing/AgentResponseCorrectorTest.java`, `rejectsRepeatedInternalEvidenceAfterCorrection` and `leavesVerifiedAndOrdinaryResponsesUnchanged` | Repeated internal evidence is never delivered; ordinary user-facing text remains unchanged. |
| `src/main/resources/agent/correction/router-internal-evidence-correction.txt` | Source uses an LLM correction instruction, which is generic provider work rather than an immutable local transform. |

### [OBSERVED] accepted Zenbot state and precise gap

| Evidence | Verified behavior / gap |
|---|---|
| `.hermes/handoffs/rapid-agent-verified-quote-only-qa.md` | Quote-only finalization accepts only trusted candidate kind and actual `ToolAttempted` metadata. Public eligible no-tool prose fails closed to a verified catalog line; tool-attempted answers are correctly exempt. Existing sanitizer/marker/rune bound run at final delivery and persistence remains after successful visible delivery. |
| `internal/agent/live/tool_loop.go`, `Completion` and `CompleteWithEvidenceAndHistorical` | The frozen public inventory is exactly `user_message_history`, `room_users`, and `run_command`; each turn permits at most one actual call and two provider completions. `ToolAttempted` derives only from request-local `turn.State.Evidence().Attempted`, including failed attempted reads. |
| `internal/agent/live/tool_loop.go`, normal tool follow-up and `completeRequiredHistory` | The provider receives model-visible tool envelopes only in the assistant/tool protocol messages. Final synthesis is tools-disabled, must contain no calls, and can carry persisted read evidence after successful delivery. The loop has no final sentinel check. |
| `internal/agent/live/runner.go`, `OutputFinalizer.FinalizeWithContext` | Finalizer sanitizes, processes no-reply semantics, applies quote-only selection, removes embedded markers, and rune-bounds output. Since tool attempts are quote-exempt, it can currently deliver a response containing `[Internal tool evidence from `. |
| `internal/agent/live/runner.go`, `AfterDelivery`; `internal/agent/live/direct.go`, `PersistDelivery`; `internal/agent/runtime/runtime.go`, `execute` | Conversation and eligible evidence writes occur only after a successful visible sink/direct send. Preventing a visible result therefore prevents both durable paths. |
| `internal/agent/assemble/assemble.go`, `RenderWithHistoricalEvidence` | Historical evidence is deliberately injected as untrusted system-prompt data. It is not approved for verbatim user delivery; a sentinel leak would violate this boundary. |
| `internal/agent/turn/policy.go`, `UnverifiedActionPolicy` | Existing generic correction foundation is not part of the live loop. It must not be reused because it requires a `ResponseCorrector` and could add completion work. |

## Alternatives and rationale

| Candidate | Decision | Evidence-based reason |
|---|---|---|
| **Fail-closed internal tool-evidence guard** | **Selected** | Applies exactly to the remaining tool-attempted final-response path that quote-only intentionally does not replace. It is a pure finalization check with no new capability surface. |
| Port `correctInternalToolEvidence` unchanged | Reject | Saturn makes a provider correction request with cache bypass and can change completion count. The accepted target is frozen at one call/two completions and explicitly excludes generic correction/retries. |
| Deterministic action-claim corrector | Defer | Quote-only already prevents public no-tool action prose; it has no effect on the residual tool-backed path selected here. |
| Database/schema/SQL tools | Defer | Needs capability, visibility, H2/repository, and dynamic-result policy work. |
| Reflected command catalog, privileged tools, or moderation | Defer | Broadens authority, authorization, and observable side effects. |
| Listener/relay/runtime/session-lock rewrite | Reject | Current direct/mention/ambient/relay admission and same-memory-key ordering already provide the needed finalization seam. |

## Exact Saturn contract and bounded Zenbot adaptation

### Observed source behavior

Saturn detects the exact marker in a no-tool final response, asks the model to produce a corrected non-tool user-facing response, and fails if the corrected result still leaks or includes a tool call.

### [RECOMMENDED] target contract

At the existing finalizer boundary, after sanitizer and exact no-reply-marker handling but **before** quote selection, embedded-marker removal, rune bounding, result construction, sink delivery, or persistence:

```text
if strings.Contains(content, "[Internal tool evidence from "):
    return error("agent response exposed internal tool evidence")
```

The match is literal and case-sensitive, exactly mirroring Saturn. It must be applied to all finalizer inputs, not only `ToolAttempted==true`: this keeps the policy simple, blocks a future metadata regression, and remains safe for all modes. It must run before quote-only selection so the catalog cannot mask a source-shaped evidence leak and turn a protocol breach into an apparently valid response.

The error is deliberately stable and does not include provider content, a tool name, H2/SQL detail, room/nick/trip/hash, prompt, resource, or internal envelope. Existing behavior then applies:

- **reply-required direct/mention/relay:** existing direct error or runtime failure-sink path; no ordinary agent text is delivered;
- **ambient:** runtime preserves its accepted error/silent behavior and does not emit a fixed reply;
- **any mode:** no `AfterDelivery`/`PersistDelivery`, conversation append, or durable evidence append.

This is a deliberate source adaptation:

```text
Saturn: leak -> one tools-disabled provider correction -> reject repeated leak
Zenbot: leak -> deterministic finalization error
```

It retains Saturn’s safety invariant—internal evidence is never visible—without a provider retry, correction prompt, cache bypass, response format, or third completion. Do not claim correction-content parity.

## File, interface, and composition plan

### Stage A — pure sentinel guard

**Create:**

- `internal/agent/live/internal_evidence_guard.go`
- `internal/agent/live/internal_evidence_guard_test.go`

Recommended private shape:

```go
const internalToolEvidenceMarker = "[Internal tool evidence from "

func containsInternalToolEvidence(content string) bool {
    return strings.Contains(content, internalToolEvidenceMarker)
}
```

Keep this in `live`, next to final-delivery policy. It must have no config, resource, mutable state, provider client, tool registry, command gateway, repository, clock, goroutine, or runtime dependency. Do not load/use `router-internal-evidence-correction.txt`: that resource exists for the explicitly excluded source correction flow.

Tests must assert literal/case-sensitive semantics, empty/nonmatching content, marker embedded amid ordinary prose, and no broad matching of user text, JSON, or tool results that lack the exact sentinel.

### Stage B — one finalizer check

**Modify:**

- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `internal/agent/live/direct_test.go`
- `cmd/zenbot/live_agent_test.go` only if package-level composition cannot establish both entry points

Do not add a `FinalizationContext` field. The finalizer already owns raw final content and determines whether it is safe to deliver. Insert the guard in `OutputFinalizer.FinalizeWithContext` in this exact order:

```text
responseSanitizer.sanitize(raw)
-> sanitized empty error (existing)
-> exact no-reply marker handling (existing)
-> internal-tool-evidence sentinel error (new)
-> quote-only catalog selection when eligible (accepted)
-> embedded no-reply-marker removal / ASCII-control trim (existing)
-> post-removal empty error / rune cap (existing)
```

Do not change `NewOutputFinalizer`, the verified quote catalog, config, `Finalizer` interface, `MarkerFinalizer` compatibility wrapper, `ToolLoop`, command channel, runtime, direct construction, or resource loading. This is intentionally a one-function pure integration after Stage A.

## Data flow and boundaries

```text
public MENTION / AMBIENT / relay AGENT / direct l
  -> existing provider-only or bounded ToolLoop completion
  -> existing OutputFinalizer
       sanitize / empty / exact marker
       -> internal evidence sentinel?
            yes: stable error; stop
            no: accepted quote-only / marker removal / rune bound
  -> existing result and one sink/send
  -> only after successful visible delivery: existing conversation/evidence persistence
```

| Concern | Required semantics |
|---|---|
| Visibility | Inspect final assistant content only. Do not inspect or surface prompt, room context, durable memory/evidence, tool envelopes, tool name, identity, capability, database, or transport state. |
| Authorization | No tool call, command dispatch, capability evaluation, privilege, moderation, or authorization behavior is added. |
| Cancellation / timeout | The check is synchronous and CPU-local; no timer, context replacement, retry, network, provider request, cache bypass, or background worker is added. Existing cancellation before finalization wins. |
| Delivery | A flagged response reaches neither normal sink nor direct sender. It does not create a substitute reply, command output, duplicate failure message, or new delivery path. |
| Persistence | Since no visible result exists, `Runner.AfterDelivery` and `DirectInvoker.PersistDelivery` cannot append conversation or durable tool evidence. Existing successful tool data remains request-local only. |
| Protocol | No change to frozen definitions, one-call ledger, two-completion limit, required fresh-history synthetic assistant/tool ID pairing, tools-disabled synthesis, `run_command` suppression, or provider message sequence. |
| Concurrency | The guard is a stateless function. Existing per-memory-key runtime serialization and ambient latest-wins semantics remain unchanged. |
| Source adaptation | The deterministic error replaces Saturn’s correction request. It intentionally does not try to remove only the sentinel and deliver potentially unsafe surrounding content. |

## RED → GREEN tests

1. **Guard RED — `internal/agent/live/internal_evidence_guard_test.go`:** add source-shaped table cases: exact marker at start/middle/end is true; empty, case variants, `[Internal tool evidence]`, and arbitrary JSON/tool-result strings without the exact marker are false. Run `go test ./internal/agent/live -run TestInternalToolEvidenceGuard -count=1` and observe failure before implementation.
2. **Finalizer RED — `internal/agent/live/runner_test.go`:** `OutputFinalizer` must return the stable error for marker-bearing content in public no-tool, tool-attempted, and direct-command-origin contexts. Assert the raw marker/content is not returned. Assert ordinary content and accepted exact catalog quote behavior remain unchanged.
3. **Ordering RED — same package:** exact ambient no-reply marker is still silent, required marker still errors, sanitizer-to-empty still errors, and a sentinel-bearing raw response errors **before** quote fallback/embedded marker/rune-cap. Include a deliberately overlong marker-bearing string to prove truncation cannot hide it.
4. **Tool-loop/lifecycle RED — `internal/agent/live/tool_loop_test.go`, `runner_test.go`, `direct_test.go`:** scripted `user_message_history` and `room_users` follow-up syntheses containing the sentinel yield no visible `runtime.Result`/`DirectCompletion`, no third completion, and no command execution. Confirm failed attempted history remains a normal finalizer input but a sentinel still fails closed. Confirm `run_command` `SuppressReply` returns before the finalizer and remains no-persistence.
5. **Delivery/persistence RED — runner/direct tests:** prove a simulated sink/send failure and a sentinel-finalizer error both append neither exchange nor evidence; a successful ordinary tool-backed response retains existing post-success persistence. Ambient sentinel failure must not call reply-required `FailureSink`.
6. **Composition regression — `cmd/zenbot/live_agent_test.go`:** prove enabled live/direct use `OutputFinalizer` through their current shared policy construction and disabled configuration still constructs no agent client/runtime/tool loop. No new constructor/config field is expected.
7. **GREEN:** implement only `internal_evidence_guard.go`, then the finalizer check. Re-run each focused test before moving to lifecycle regression. Do not add a provider-correction fixture because the acceptance condition is zero added completions.

## Focused, relevant, full, build, and diff gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only implementation-owned Go files.

```sh
gofmt -w \
  internal/agent/live/internal_evidence_guard.go \
  internal/agent/live/internal_evidence_guard_test.go \
  internal/agent/live/runner.go \
  internal/agent/live/runner_test.go \
  internal/agent/live/direct_test.go \
  internal/agent/live/tool_loop_test.go \
  cmd/zenbot/live_agent_test.go

go test ./internal/agent/live -run 'Test(InternalToolEvidence|OutputFinalizer|VerifiedQuote|QuoteOnly|ToolLoop.*(History|RoomUsers|Command|Evidence)|Runner.*(Evidence|Quote)|Direct.*Evidence)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

The focused test, relevant-package test, full suite, build, and diff hygiene are acceptance gates under `MIGRATION_PLAN.md` Priority 2. Run `go test -race ./internal/agent/live -count=1` only as optional confirmation; this design adds no mutable state. Observe `go vet ./...` separately: accepted QA documents a pre-existing unrelated copylocks warning at `internal/core/engine_impl.go:95:22`.

## Complexity routing

**Complexity: low.** The implementation is one literal predicate and one finalizer branch. Review-sensitive concerns are ordering before quote fallback, zero added provider work, and no persistence after a blocked final response.

- **Developer:** implement the private guard and focused finalizer tests.
- **Senior live/runtime reviewer:** verify sentinel ordering, tool-attempt/quote-only interaction, direct/ambient failure semantics, `run_command` suppression, and delivery-before-persistence proof.
- **Independent QA:** replay all sentinel cases across no-tool/tool-backed/direct paths; verify no provider retry, no tool/command/SQL/transport/config work, fixed protocol bounds, targeted/relevant/full/build/diff gates, and only slice-owned files changed.

## Explicit exclusions

- No `AgentResponseCorrector` port, correction request, correction resource use, provider retry, third completion, response format, cache bypass, or `turn.UnverifiedActionPolicy` live integration.
- No changes to quote catalog/resource, output sanitizer/rune bound, no-reply semantics, tool inventory, command catalog, `run_command` authorization/gateway, history/room-users behavior, durable memory/evidence schema, config, H2/SQL, moderation, listener/relay/transport, admission, session locks, or prompt assembly.
- No edits to Saturn source, protected `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, existing handoffs, or application source in this architecture task; no commit or push.

## Completion checklist

- [ ] Exactly one capability is implemented: fail-closed final delivery for Saturn’s internal-tool-evidence sentinel.
- [ ] All cited existing Saturn and Zenbot paths resolve; proposed guard files are marked new.
- [ ] Source provider correction and deterministic no-retry target adaptation are explicitly distinct.
- [ ] Marker, quote-only, tool-attempt, command suppression, cancellation, delivery, persistence, protocol, and concurrency boundaries have tests.
- [ ] Three-tool/one-call/two-completion and existing direct/mention/ambient/relay contracts are unchanged.
- [ ] Only listed implementation files change and focused/relevant/full/build/diff gates pass.
