# Rapid agent internal-evidence sentinel implementation

## Scope

Implemented the approved source-shaped, deterministic fail-closed guard at Zenbot's final delivery boundary.

- New private predicate: `internal/agent/live/internal_evidence_guard.go`
  - Exact, case-sensitive literal: `[Internal tool evidence from `.
  - Stateless `strings.Contains`; no provider, tool, command, runtime, persistence, config, or resource dependency.
- Integration: `internal/agent/live/runner.go`
  - `OutputFinalizer.FinalizeWithContext` now orders handling as:
    1. sanitize
    2. sanitized-empty rejection
    3. exact no-reply marker handling
    4. internal-evidence sentinel rejection
    5. quote-only selection
    6. embedded no-reply marker removal and ASCII trim
    7. post-removal empty rejection and rune cap
  - Sentinel error is exactly `agent response exposed internal tool evidence`; it contains no provider content.
  - `SuppressReply` still returns before the finalizer in both `Runner` and `DirectInvoker`.

## Saturn source and adaptation

Read-only Saturn source verified at:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/agent/routing/AgentResponseCorrector.java:389-408,447-450`

Saturn detects this same literal on no-tool final content and makes one tools-disabled correction completion before rejecting a repeated leak. Zenbot intentionally adapts this to a local finalization error: no correction prompt/resource, retry, cache bypass, tool call, command execution, or third completion is introduced. Unlike Saturn's candidate selection, Zenbot checks every finalizer context before quote fallback, including tool-attempted and direct-command-originated flows.

## Strict TDD evidence

### RED

1. Created `internal/agent/live/internal_evidence_guard_test.go` before the predicate.
   - `go test ./internal/agent/live -run TestInternalToolEvidenceGuard -count=1`
   - Failed as expected: `undefined: containsInternalToolEvidence`.
2. Added finalizer ordering/context test before the finalizer branch.
   - `go test ./internal/agent/live -run TestOutputFinalizerRejectsInternalToolEvidenceBeforeQuoteFallbackOrTruncation -count=1`
   - Failed as expected in public no-tool, tool-attempted, and direct-command-originated subtests with `error = <nil>`.

### GREEN

1. Added the private literal predicate, then the single finalizer branch.
2. `go test ./internal/agent/live -run 'Test(InternalToolEvidenceGuard|OutputFinalizerRejectsInternalToolEvidenceBeforeQuoteFallbackOrTruncation)' -count=1` passed.
3. Added lifecycle regressions after the green slice:
   - `TestRunnerRejectsToolBackedInternalEvidenceWithoutThirdCompletion`: scripted history tool + sentinel synthesis returns no result, makes exactly two completions, and leaks no raw secret in the error.
   - `TestDirectInvokerRejectsInternalEvidenceWithoutDeliveryArtifact`: direct invocation returns no completion artifact, makes exactly one completion, and leaks no raw secret in the error.

## Boundaries verified by tests and existing lifecycle

- Literal matching tests cover start/middle/end, empty content, case variant, incomplete marker, and ordinary JSON tool-result text.
- Finalizer test proves sentinel rejection precedes quote fallback and an output cap that would otherwise truncate the marker.
- Tool-backed runner test preserves the existing one-tool/two-completion protocol and proves zero provider correction completion.
- Because blocked `Runner.Run` and `DirectInvoker.InvokeCompletion` return an error/no artifact before runtime sink/send, the existing delivery-after-finalization control flow cannot call `AfterDelivery` or `PersistDelivery`; therefore it cannot append conversation or durable evidence. Existing runtime behavior also only calls `FailureSink` for reply-required modes, so ambient remains silent on this failure.
- Existing ordinary output, verified quote handling, command prose behavior, fresh-history flow, and run-command `SuppressReply` path were unchanged.

## Files owned by this slice

- `internal/agent/live/internal_evidence_guard.go` (new)
- `internal/agent/live/internal_evidence_guard_test.go` (new)
- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `internal/agent/live/direct_test.go`
- `.hermes/handoffs/rapid-agent-internal-evidence-sentinel-implementation.md` (new)

No protected migration files, quote resources/catalog, tool registry, command channel/gateway, runtime/listener/transport/config, SQL, or persistence schema were edited. No commit or push was made.

## Gates

All commands run from `/Users/ab/workspace/go-projects/zenbot` after formatting owned Go files:

- PASS: `go test ./internal/agent/live -run 'Test(InternalToolEvidence|OutputFinalizer|VerifiedQuote|QuoteOnly|ToolLoop.*(History|RoomUsers|Command|Evidence)|Runner.*(Evidence|Quote)|Direct.*Evidence)' -count=1`
- PASS: `go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1`
- PASS: `go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1`
- PASS: `go test ./... -count=1`
- PASS: `go build ./...`
- PASS: `git diff --check`
- Informational known-core failure: `go vet ./...` reports pre-existing `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.
