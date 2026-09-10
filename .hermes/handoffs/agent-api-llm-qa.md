# Agent API/config/LLM/OpenAI QA

## Verdict: FAIL (regressions fixed, parity still incomplete)

I independently read both handoffs, inspected the final Go source/tests, and compared the bounded behavior with the read-only Saturn source/tests under `/Users/ab/workspace/projects/saturn`. The implementation is buildable and all requested Go gates pass, but I cannot claim PASS because required parity/coverage gaps remain (listed below).

## QA files changed

- `internal/agent/api/api_test.go` — unknown mode, required invocation fields, uppercase enum JSON, empty Result JSON.
- `internal/config/agent_config_test.go` — precedence, API key indirection, defaults, invalid endpoint/limits.
- `internal/agent/llm/openai/qa_test.go` — null content/usage mapping, constructor validation, token-safe provider errors.
- `internal/agent/llm/client.go` — added metadata-preserving response constructor and strict content type check.
- `internal/agent/llm/openai/client.go` — preserved JSON null content, mapped usage, validated constructor values, bounded retryable status classification.
- `.hermes/handoffs/agent-api-llm-qa.md` — this report.

No Saturn files were edited by this QA. The Saturn worktree was already dirty before QA (its status contains unrelated pre-existing files); no Saturn path appears in the QA change list.

## Findings and fixes

1. **Fixed:** OpenAI request serialization converted assistant `nil` content to `"content":""`; Saturn emits JSON `null`. `messageJSON` now uses `ContentNullable()` and the regression test asserts decoded JSON null.
2. **Fixed:** OpenAI response `usage` was discarded. The response envelope now decodes usage and `LlmResponse` has a metadata constructor that defensively copies it.
3. **Fixed:** `decodeResponse` converted provider `content:null` to a present empty string. It now passes nil through, preserving nullable semantics.
4. **Fixed:** `New` accepted negative retries/timeouts/delays/token limits and out-of-range temperature. Constructor regression tests cover rejection.
5. **Fixed:** retry classification treated arbitrary status >=500 (including invalid HTTP status 600) as retryable; it is now restricted to 500–599, plus 408 and 429.

## Parity evidence

- Saturn `AgentConfigLoader` confirms defaults: endpoint `http://localhost:16261`, completion tokens 1024, steps 5, tool calls 4 (Go defaults match); Saturn normalizes trailing endpoint slashes.
- Saturn OpenAI tests/source confirm POST `/v1/chat/completions`, bearer auth, `Content-Type`, `stream:false`, `max_tokens`, optional tools/tool choice, structured response format, null assistant content, tool-call mapping, malformed choices rejection, transient retries, and no retry for ordinary 4xx. Go tests/source cover most of these.
- Saturn `AgentConfigValueReader` confirms environment-over-TOML precedence, strict env scalar parsing, integer overflow conversion, and API-key lookup by configured variable name. Go `ValueReader` covers runtime > environment > file lookup and API-key indirection, but does not yet provide equivalent strict parse errors.
- Saturn `AgentInvocation` rejects blank request ID/prompt and null context/mode. The Go API is a different reduced DTO and validates mode, room, and timestamp; exact field-level parity is therefore not established.

## Required command results

All commands exited 0:

- `gofmt -w internal/agent/api/api_test.go internal/agent/llm/client.go internal/agent/llm/openai/client.go internal/agent/llm/openai/qa_test.go internal/config/agent_config_test.go`
- `go test -count=1 ./internal/agent/... ./internal/config/...` — all packages passed.
- `go test -race ./internal/agent/... ./internal/config/...` — all packages passed.
- `go test -count=1 ./...` — all packages passed.
- `go test -race ./...` — all packages passed.
- `go vet ./...` — passed with no output.
- `go build ./...` — passed with no output.
- `git diff --check` — passed with no output.

## Remaining gaps / exclusions preventing PASS

- `Retry-After` only accepts numeric seconds. Standard valid HTTP-date form is not handled; this is explicitly required behavior.
- Cancellation/backoff has implementation support and existing focused coverage, but the QA additions do not independently exercise cancellation during a real request and during timed backoff with error identity assertions.
- Successful response provider diagnostics are not preserved (only usage is mapped); the required response metadata contract mentions provider diagnostics.
- `LlmRequest`/tool JSON defensive copying is incomplete for arbitrary nested typed maps/slices; cloning handles `map[string]any` and `[]any`, not all JSON-compatible Go representations.
- The Go config reader silently falls back for malformed boolean/integer values, unlike Saturn's strict environment readers, which return actionable errors. It also does not expose Saturn's loader-level default `SATURN_AGENT_API_KEY` for an absent `APIKeyEnv`.
- Exact Saturn API parity is not provable from the reduced Go API types: the Go `Invocation`, `Context`, `Memory`, `Result`, and `Identity` shape differs materially from Saturn's records/classes. The added tests verify the implemented reduced contract, not full Saturn serialization parity.
- No public JSON marshal/unmarshal contract exists for the provider-neutral LLM DTOs themselves because their fields are private; JSON is verified only at the OpenAI wire boundary.

The bounded slice is green under all requested Go quality gates, but the above required behaviors remain unverified or mismatched, so the correct QA verdict is FAIL rather than PASS.
