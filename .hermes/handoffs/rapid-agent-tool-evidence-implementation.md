# Durable tool-evidence implementation — complete

## Completed contract
- Durable H2 repository remains the shared source for exact-memory-key, TTL-bound evidence (`repository.AgentToolEvidenceRepository`, `agent_tool_memory`).
- `turn.TurnMemory.LoadHistoricalEvidenceContext` loads structured, defensive-copied `HistoricalEvidence` only for public requests; it honors context cancellation, skips malformed rows, and never loads whispers.
- `assemble.AssembleWithHistoricalEvidence` and `SystemPrompt.RenderWithHistoricalEvidence` render bounded, valid JSON evidence only in `HISTORICAL_TOOL_EVIDENCE_UNTRUSTED_DATA`, positioned after trusted runtime metadata and before recent-room context. The section labels evidence stale/untrusted data, never instructions/policy/fresh data. It never uses the legacy `[Internal tool evidence from ...]` prefix. Empty, malformed, over-32KB, and whisper inputs emit nothing.
- `Runner` and `DirectInvoker` load historical evidence once before the initial completion. `ToolLoop.CompleteWithEvidenceAndHistorical` injects it only into completion #1; completion #2 retains the existing assistant/tool pair and does not reload or reinject it.
- Added immutable `runtime.DirectCompletion` carrying text plus defensively-copied `turn.PersistableEvidence`. `live.DirectInvoker.InvokeCompletion` returns that artifact; it retains no mutable per-invoker request state.
- `directLCommand` sends the artifact text first, then invokes `PersistDelivery` only after successful visible `SendChatMessage`. `DirectInvoker.PersistDelivery` appends the normal exchange and eligible evidence from the same artifact, skips whisper evidence, and returns post-delivery errors for logging only. Silent/blank/error/cancel/send-failure paths do not reach persistence.
- Normalized nil room-user slices at the `apiContext` bridge so direct persistence uses the correct immutable memory key.

## Test evidence / RED-GREEN
- RED projection: `go test ./internal/agent/assemble -run TestAssembleProjectsHistoricalToolEvidenceOnlyIntoTaggedUntrustedSection -count=1` failed with missing `turn.HistoricalEvidence` and `Assembler.AssembleWithHistoricalEvidence`.
- GREEN projection: same test passed after the structured prompt projection implementation.
- RED direct delivery: `go test ./internal/command -run TestDirectLCommandPersistsDirectDeliveryArtifactAfterVisibleSend -count=1` failed with missing `runtime.DirectCompletion` / `NewDirectCompletion`.
- GREEN direct delivery: same test passed after the immutable artifact and post-send command seam.
- Added coverage in `internal/agent/assemble/assemble_test.go`, `internal/command/handlers_test.go`, and `internal/agent/live/direct_test.go` for tagged projection, whisper suppression, post-visible-send artifact persistence, and public-versus-whisper durable append.

## Final gates
- PASS: `go test ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/assemble ./internal/agent/runtime ./internal/command ./cmd/zenbot -count=1`
- PASS: `go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/runtime -count=1`
- PASS: `go test ./... -count=1`
- PASS: `go build ./...`
- PASS: `git diff --check`
- `go vet ./...` remains blocked only by the known unrelated `internal/core/engine_impl.go:95:22` copylocks warning (`NewEngineImpl passes lock by value`).

## Scope retained
No schema changes, new provider-visible tools, tool registry changes, generalized routing, moderation changes, database SQL expansion, mutable pending-evidence state, commits, or pushes.
