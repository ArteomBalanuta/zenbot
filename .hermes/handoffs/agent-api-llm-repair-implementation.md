# Agent API / LLM bounded repair implementation handoff

## Outcome
Implemented the five bounded repairs from the verified diagnostic with regression coverage. No Saturn files were modified. The reduced Go API DTOs were intentionally not rewritten; exact Saturn API DTO/serialization parity is deferred to a separate follow-on contract slice.

## Repairs
- OpenAI `Retry-After` now accepts nonnegative integer seconds and valid HTTP-date values relative to injected `Config.Now`; malformed, negative, past, and overflow values use bounded exponential fallback, while `0` remains immediate.
- Added real cancellation regression tests for an in-flight context-bound transport and a real timed retry backoff. Both assert prompt completion, no later attempt, typed `llm.LlmError` code, and `errors.Is(err, context.Canceled)` identity.
- Successful OpenAI response diagnostics are retained from top-level provider JSON fields other than `choices` and `usage`. Diagnostics and accessors are defensively copied.
- Centralized recursive JSON-compatible copying in `internal/agent/llm`, preserving typed maps/slices/arrays/pointers/raw byte slices and nested values. OpenAI options use the shared copier.
- Configuration scalar reads now return actionable errors for malformed bool/int values and integer overflow, while blank/absent values retain fallback semantics. Runtime > environment > file precedence remains intact. Absent `APIKeyEnv` resolves `SATURN_AGENT_API_KEY`; explicit names still override it.

## Exact files changed by this repair slice
- `internal/agent/llm/client.go`
- `internal/agent/llm/client_test.go`
- `internal/agent/llm/openai/client.go`
- `internal/agent/llm/openai/client_test.go`
- `internal/config/value_reader.go`
- `internal/config/agent_config.go`
- `internal/config/agent_config_test.go`
- `.hermes/handoffs/agent-api-llm-architecture.md` — explicitly labels current API DTOs as reduced internal compatibility DTOs and defers exact parity.
- `.hermes/handoffs/agent-api-llm-implementation.md` — removes the prior overstatement of no remaining gaps and records deferred API parity.
- `.hermes/handoffs/agent-api-llm-repair-implementation.md` — this handoff.

Pre-existing unrelated dirty and untracked work was preserved. The existing `internal/config/config.go` modification and unrelated application files were not changed by this slice.

## Parity decisions and deferred scope
- Preserved existing retry classification (408, 429, and 500–599), retry bounds, cancellation wrapper identity, nullable content behavior, and existing reduced DTO names/call sites.
- Provider diagnostics are represented as a deep-cloned JSON-compatible map of successful response fields not otherwise modeled; HTTP error diagnostics remain bounded and token-safe.
- Exact Saturn `AgentInvocation`/`AgentContext`/`AgentResult`/`AgentUserIdentity` field shape, constructors, null/omitted semantics, identity derivation, and caller migration remain deferred. This slice does not claim exact API DTO parity and does not rewrite those types.
- Routing, invocation assembly, tools/execution, turns, persistence, moderation, listeners, commands, and Saturn source remain excluded.

## Verification (actual results)
All commands exited 0:
- `gofmt -w internal/agent/llm/client.go internal/agent/llm/client_test.go internal/agent/llm/openai/client.go internal/agent/llm/openai/client_test.go internal/config/value_reader.go internal/config/agent_config.go internal/config/agent_config_test.go`
- `go test -count=1 ./internal/agent/... ./internal/config/...`
- `go test -race ./internal/agent/... ./internal/config/...`
- `go test -count=1 ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

The full test and race runs passed all packages, including the agent LLM/OpenAI/config regression tests.
