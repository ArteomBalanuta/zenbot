# Agent API / LLM repair QA

## Verdict: PASS for the five bounded repairs

Fresh independent QA was performed against the final Go source/tests and the read-only Saturn references under `/Users/ab/workspace/projects/saturn`. All five requested repaired behaviors are proven by source inspection plus focused regression coverage. Exact Saturn API DTO parity remains explicitly deferred and is not counted as a blocker for this bounded slice.

## Behavior evidence

1. **Retry-After and injected clock — PASS**
   - `internal/agent/llm/openai/client.go` parses nonnegative integer seconds and valid HTTP-date values using injected `Config.Now`.
   - Invalid, negative, overflow, and past dates use exponential fallback; `Retry-After: 0` returns an immediate zero delay.
   - `TestRetryAfterHTTPDate` now exercises a real `httptest.Server`: first 429 with an HTTP-date, second successful response, fixed injected clock, and recorded 3-second sleep. It also asserts malformed/negative/overflow/past fallback and zero immediate behavior.
   - Retry classification remains bounded to 408, 429, and 500–599.

2. **Request/backoff cancellation and error identity — PASS**
   - `TestCompleteCancellationDuringRequest` uses a real context-bound blocking `RoundTripper`, verifies prompt return, one attempt, typed `*llm.LlmError` code `cancelled`, and `errors.Is(err, context.Canceled)`.
   - `TestCompleteCancellationDuringBackoff` uses a real transient 503 response and context-aware timed backoff, verifies prompt cancellation, one attempt/no later request, typed `cancelled` error, and `errors.Is(err, context.Canceled)`.
   - Production path attaches caller/operation context to HTTP requests and sleep; `LlmError.Unwrap` preserves identity.

3. **Provider diagnostics and bounded HTTP errors — PASS**
   - Successful response diagnostics are retained from top-level JSON fields other than `choices` and `usage`, including nested provider metadata; accessors clone the data.
   - `TestCompletePreservesProviderDiagnostics` and `TestResponseDiagnosticsAreDefensivelyCopied` verify retained values and accessor/input mutation safety.
   - HTTP errors retain bounded `Snippet`/typed provider fields while `Error()` excludes body snippets and tokens; `TestProviderErrorDoesNotLeakToken` covers this.

4. **Deep JSON-compatible defensive copying — PASS**
   - Shared `llm.CloneJSONValue` recursively handles typed maps, slices, arrays, pointers/interfaces, structs with exported JSON-compatible fields, and raw byte slices/`json.RawMessage`; LLM DTO constructors/accessors and OpenAI options use cloning.
   - `TestContractsCloneTypedJSONValuesAndRawBytes` covers nested typed maps/slices/raw bytes across request tools and response-format input/accessor mutation.
   - `TestNewClientClonesNestedOptions` covers typed nested OpenAI options input mutation after construction.
   - Existing nested DTO and diagnostics tests cover accessor mutation isolation.

5. **Strict config parsing, precedence, and API-key defaults — PASS**
   - `ValueReader.Bool`/`Int` return actionable errors for malformed values and integer overflow rather than silently falling back; absent/blank values retain fallback semantics.
   - Lookup order is runtime > environment > file > fallback. Existing precedence tests cover runtime and environment over lower sources; new `TestValueReaderRejectsMalformedScalarsAtEverySource` verifies strict failures at runtime, environment, and file sources.
   - `AgentConfig.Resolve` defaults absent `APIKeyEnv` to `SATURN_AGENT_API_KEY`; explicit names override it and secret lookup uses the selected name. Existing tests cover both default and explicit override.
   - Saturn comparison: `AgentConfigLoader` and `AgentConfigValueReader` confirm the same default variable name, strict scalar parsing, and overflow rejection semantics.

## Files changed by this QA pass

- `internal/agent/llm/openai/client_test.go` — added real HTTP-date retry integration/fallback assertions and nested options cloning coverage.
- `internal/config/agent_config_test.go` — added malformed bool/int tests for runtime, environment, and file sources.
- `.hermes/handoffs/agent-api-llm-repair-qa.md` — this report.

The production repair files were already present from the implementation slice and were not changed by this QA pass:
`internal/agent/llm/client.go`, `internal/agent/llm/openai/client.go`, `internal/config/value_reader.go`, and `internal/config/agent_config.go`.

No Saturn files were modified by this QA. The Saturn checkout had unrelated pre-existing dirty/untracked files; they were preserved. Unrelated target worktree changes were preserved.

## Actual verification results

All commands exited 0:

- `gofmt -w internal/agent/llm/client.go internal/agent/llm/client_test.go internal/agent/llm/openai/client.go internal/agent/llm/openai/client_test.go internal/config/value_reader.go internal/config/agent_config.go internal/config/agent_config_test.go`
- `go test -count=1 ./internal/agent/llm/... ./internal/config/...` — passed.
- `go test -count=1 ./internal/agent/... ./internal/config/...` — passed.
- `go test -race ./internal/agent/... ./internal/config/...` — passed.
- `go test -count=1 ./...` — passed all packages.
- `go test -race ./...` — passed all packages.
- `go vet ./...` — passed with no output.
- `go build ./...` — passed with no output.
- `git diff --check` — passed with no output.

## Deferred API-contract scope

The Go `internal/agent/api` DTOs remain reduced internal compatibility DTOs rather than exact Saturn `AgentInvocation`/`AgentContext`/`AgentResult`/`AgentUserIdentity` parity. Exact field shape, constructors/defaults, null/omitted semantics, identity derivation, and caller migration require a separate reviewed API-contract slice and were intentionally not evaluated as a blocker here. Routing, invocation assembly, tools/execution, turns, persistence, moderation, listeners, commands, and Saturn source remain excluded.
