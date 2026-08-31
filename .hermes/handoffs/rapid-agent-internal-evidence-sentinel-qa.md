# Rapid agent internal-evidence sentinel QA

## Verdict

**PASS — fail-closed final-delivery sentinel guard is correctly bounded.**

The finalizer detects only the exact case-sensitive literal `[Internal tool evidence from ` after sanitization and exact no-reply handling, but before quote selection, embedded-marker removal, and rune capping. It returns the stable content-free error `agent response exposed internal tool evidence`.

## Observed implementation and boundary evidence

- `internal/agent/live/internal_evidence_guard.go` is a stateless `strings.Contains` predicate over precisely that literal. Tests cover start/middle/end, empty, case variant, incomplete marker, and unrelated JSON tool-result text.
- `OutputFinalizer.FinalizeWithContext` has the required order: sanitize → sanitized-empty error → exact no-reply behavior → sentinel error → quote selection → embedded marker removal/trim → post-removal empty error → rune cap.
- Finalizer coverage confirms source-shaped sentinel content fails for public no-tool, actual tool-attempted, and direct command-originated contexts, including a prefix longer than the configured cap. The error does not contain provider content.
- `Runner.Run` and `DirectInvoker.InvokeCompletion` return before an artifact on finalizer error. The runner tool-backed regression has exactly two provider completions, no third correction/retry, and no leaked raw secret. The direct regression has one completion and no completion artifact.
- Added QA regression `TestRuntimeSentinelFailureSkipsNormalDeliveryAndAfterDelivery` exercises actual `live.Runner` through `runtime.Runtime` for reply-required and ambient input. It proves zero normal sink calls and zero `AfterDelivery` calls; reply-required retains the existing one failure-sink call, while ambient calls no failure sink.
- `runtime.execute` invokes `AfterDelivery` only after a successful normal sink delivery. `Runner.AfterDelivery` and `DirectInvoker.PersistDelivery` consume only visible result/completion artifacts. The blocked paths have no such artifact, so conversation/evidence persistence cannot occur. Existing direct command handling similarly calls `PersistDelivery` only after a successful `SendChatMessage`.
- Existing `SuppressReply` returns before finalization in both Runner and DirectInvoker. Existing tool-loop tests retain `run_command` suppression, one-call/two-completion limits, no tools on synthesis, and fresh-history synthetic assistant/tool ID pairing. Existing ordinary-output and verified-quote tests remain green.

## QA repair

No production defect found. Added one test-only regression in `internal/agent/live/runner_test.go` to make the runtime-level no-normal-delivery/no-`AfterDelivery` and ambient-no-`FailureSink` guarantees explicit.

## Gates

All from `/Users/ab/workspace/go-projects/zenbot`:

- PASS: focused `go test ./internal/agent/live -run 'Test(InternalToolEvidence|OutputFinalizer|VerifiedQuote|QuoteOnly|ToolLoop.*(History|RoomUsers|Command|Evidence)|Runner.*(Evidence|Quote)|Direct.*Evidence|RuntimeSentinel)' -count=1`
- PASS: `go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1`
- PASS: `go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1`
- PASS: `go test ./... -count=1`
- PASS: `go build ./...`
- PASS: `git diff --check`
- PASS: optional `go test -race ./internal/agent/live -count=1`
- INFORMATIONAL: `go vet ./...` still reports the known unrelated core warning: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.

## Exclusions confirmed

No provider correction/retry, third completion, tool call, command execution, quote catalog/resource, command channel/gateway, runtime/listener/transport/config, SQL/persistence schema, or Saturn source was changed. No commit or push was made.
