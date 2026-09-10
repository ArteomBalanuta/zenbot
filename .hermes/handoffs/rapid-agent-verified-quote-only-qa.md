# Rapid agent verified quote-only delivery: independent QA

## Verdict

**PASS with hardening applied.** Eligible public no-tool outputs are now fail-closed when the catalog is unavailable, and catalog validation rejects CR/LF in every persisted source field. The existing implementation otherwise carries only trusted loop metadata to finalization, preserves the bounded completion protocol, and keeps direct `l` command-origin exempt.

## Source/provenance audit

- Saturn `VerifiedQuoteCatalog` loads the static `agent/verified-quotes.json`, validates nonempty required fields, duplicates, and source quote grammar, uses Java `strip()` for exact lookup, and falls back to the first entry.
- Zenbot uses the repository's existing filesystem resource convention (the same source-relative `resources/agent` pattern used by `internal/agent/prompt`), not hard-coded quote strings. Logical JSON comparison confirmed Zenbot's three catalog records exactly match Saturn's three records.
- `Completion.CandidateKind` is derived from `inv.Prompt()` by the existing classifier; in ToolLoop paths it is classified once at loop entry. Runner/Direct no longer pre-classify before calling ToolLoop.
- `Completion.ToolAttempted` is projected solely from `turn.State.Evidence().Attempted`, set immediately before executor invocation. Provider tool-call JSON, tool definitions, response content, and durable evidence cannot forge it. A failed `user_message_history` execution that reaches synthesis remains attempted.

## Defects found and fixed

1. **Fail-open catalog pointer:** `OutputFinalizer` previously passed raw eligible public provider text when `Catalog == nil`. It now returns `verified quote catalog is not initialized`, preventing invalid eligible text from reaching a sink.
2. **Incomplete CR/LF validation:** CR/LF was rejected only after rendering the visible quote line. It now rejects CR/LF in `id`, `quote`, `book`, `author`, and `reference`, preventing malformed hidden catalog metadata from being accepted.
3. **Duplicate classification:** Runner and Direct classified before invoking ToolLoop and then discarded that value. They now classify only in the provider-only compatibility branch; ToolLoop retains the single classification owner for bounded-loop invocations.

## Focused coverage added

- Eligible output with no initialized catalog fails closed.
- CR/LF in non-rendered catalog fields is rejected.
- A failed actual read-tool attempt remains `ToolAttempted=true` through the synthesis completion.
- Direct `l` retains ordinary command output rather than triggering quote-only fallback.

Existing coverage verifies sanitizer/empty/no-reply-marker handling occurs before selection, deterministic exact-line canonicalization/fallback, whisper/moderation/direct/attempt exclusions, command suppression before finalization, fixed three-tool/one-call/two-completion mechanics, ID pairing, and post-delivery persistence boundaries.

## Gate evidence

Passed:

```text
gofmt -w internal/agent/live/{verified_quote_catalog,verified_quote_catalog_test,runner,tool_loop_test,direct,direct_test}.go
go test ./internal/agent/live -run 'Test(VerifiedQuote|QuoteOnly|OutputFinalizer|ToolLoop.*(Quote|Command|History|Carries|KeepsAttempted)|Runner.*Quote|Direct.*Quote|DirectInvokerKeepsOrdinary)' -count=1
go test ./internal/agent/participation -run TestClassifier -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent|Quote|OutputFinalizer)' -count=1
go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
go test -race ./internal/agent/live -count=1
git diff --check
```

`go vet ./...` remains informationally nonzero only for the known unrelated core warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions checked

No Saturn files, catalog resources, protected migration/audit documents, tool inventory, listener/transport/gateway, SQL/schema/config, provider correction/response format, extra completion, commit, or push were changed by this QA hardening. Existing unrelated dirty work, including `cmd/zenbot/main.go`, was preserved.
