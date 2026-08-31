# Rapid agent verified quote-only delivery: implementation handoff

## Outcome

Implemented deterministic local verified quote-only finalization for eligible public, non-command no-tool turns. No provider correction, retry, response format, catalog mutation, tool inventory change, transport/listener change, SQL/config change, commit, or push was made.

## Touched paths

- `internal/agent/live/verified_quote_catalog.go` (new): validates and loads `resources/agent/verified-quotes.json` using the repository's filesystem resource convention; immutable catalog supports Java-whitespace exact lookup and first-entry fallback.
- `internal/agent/live/verified_quote_catalog_test.go` (new): catalog validation, canonical lookup, fallback, eligibility/finalizer behavior.
- `internal/agent/live/runner.go`: context-aware finalization after existing sanitizer/marker checks; public eligibility predicate and compatibility wrapper; Runner passes trusted completion metadata.
- `internal/agent/live/direct.go`: passes metadata through the shared finalizer path; direct invocations remain command-origin exempt.
- `internal/agent/live/tool_loop.go`: `Completion` now carries candidate request kind and actual loop-attempt evidence. Candidate is classified once from `inv.Prompt()`; attempt is read only from `turn.State.Evidence().Attempted` after execution state is known.
- `internal/agent/live/tool_loop_test.go`: public no-tool candidate/attempt provenance behavior.
- `cmd/zenbot/main.go`, `cmd/zenbot/live_agent_test.go`: production composition creates a catalog-aware finalizer and fails closed on invalid/missing catalog.

`cmd/zenbot/main.go` already had extensive unrelated dirty content before this slice; only the finalizer constructor/call sites were changed here. Existing resources/catalog content and protected migration/audit files were not edited.

## RED → GREEN evidence

Initial quote catalog/finalizer test run was RED as expected because `loadVerifiedQuoteCatalog`, `FinalizationContext`, `OutputFinalizer.Catalog`, and `FinalizeWithContext` did not exist:

```text
undefined: loadVerifiedQuoteCatalog
unknown field Catalog in struct literal of type OutputFinalizer
f.FinalizeWithContext undefined
undefined: FinalizationContext
```

After minimal implementation:

```text
go test ./internal/agent/live -run 'Test(VerifiedQuoteCatalog|OutputFinalizerUsesVerifiedQuote|OutputFinalizerDoesNotRequireQuotes|OutputFinalizer)' -count=1
ok zenbot/internal/agent/live
```

## Trusted metadata and delivery semantics

- Eligibility is `!whisper && !CommandOriginated && mode != MODERATION && (TALK || UNCLASSIFIED) && !ToolAttempted`.
- Candidate kind is computed once from the trusted invocation prompt, never model output.
- `ToolAttempted` is based only on actual `turn.State` execution state, not provider tool-call data, tool definitions, successful evidence, or model content. It remains true for a failed attempted read that reaches a response path.
- `SuppressReply` remains before finalization. Empty/no-reply marker behavior runs before catalog selection. Selection then canonicalizes an exact catalog line or chooses the first catalog line; final marker removal and rune bounds still run afterwards.
- Whispers, moderation, direct `l`, and attempted-tool results are excluded. No catalog selection happens for whispers. Invalid eligible public prose is a normal visible fallback, so existing post-success-only memory persistence remains intact; no evidence is fabricated.
- The frozen three-tool, one-call/two-completion protocol and command-prose behavior are unchanged.

## Verification

Passed:

```text
go test ./internal/agent/live -run 'Test(VerifiedQuote|OutputFinalizer|ToolLoop.*(Command|History|Carries)|Runner.*Quote|Direct.*Quote)' -count=1
go test ./internal/agent/participation -run TestClassifier -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent|Quote|OutputFinalizer)' -count=1
go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
go test -race ./internal/agent/live -count=1
```

All passed. `go vet ./...` remains informationally nonzero only for the pre-existing unrelated warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```
