# Agent API/LLM implementation handoff

## Outcome
Implemented the bounded Saturn parity slice only in the agent API, provider-neutral LLM contracts, OpenAI-compatible client, and agent configuration reader/configuration.

## Exact files changed
- `internal/agent/api/api.go`
- `internal/agent/llm/client.go`
- `internal/agent/llm/openai/client.go`
- `internal/config/agent_config.go`
- `internal/config/value_reader.go`

Pre-existing unrelated dirty/untracked files were preserved. Final scoped status was:
?? internal/agent/api/api.go
?? internal/agent/llm/client.go
?? internal/agent/llm/openai/client.go
?? internal/config/agent_config.go
?? internal/config/agent_sql_config.go
?? internal/config/value_reader.go

## Implementation/parity decisions
- Preserved uppercase `DIRECT`, `MENTION`, `AMBIENT`, and `MODERATION`, with explicit unknown-mode validation.
- Added JSON tags and invocation validation while retaining existing Go call-site compatibility.
- LLM messages preserve nullable content through `ContentNullable`; constructors and accessors defensively copy nested maps/slices. Tool calls retain raw argument JSON and decoded object access.
- Added usage/provider-diagnostic response accessors and typed, unwrap-able, secret-safe `LlmError` fields.
- Configuration resolution uses runtime > environment > file values, applies Saturn defaults (localhost:16261, 30s, 1024 completion tokens, 5 steps, 4 tools), normalizes trailing slashes, resolves `APIKeyEnv` as a variable name, validates URL/positive-or-nonnegative constraints and bounded limits, and returns safe timeout duration.
- OpenAI client posts normalized `/v1/chat/completions`, sets JSON and bearer headers, supports injected HTTP client/context, optional temperature, tool calls/results, response format, transient transport/408/429/5xx retries, exponential retry delay and valid numeric `Retry-After`, cancellation during I/O/backoff, malformed response detection, and typed HTTP/provider diagnostics without putting provider body in the primary error string.

## Verification (actual commands, all exit 0)
- `gofmt -w internal/agent/api/api.go internal/agent/llm/client.go internal/agent/llm/openai/client.go internal/config/agent_config.go internal/config/value_reader.go`
- `go test -count=1 ./internal/agent/... ./internal/config/...`
- `go test -count=1 ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

## Remaining gaps
The five bounded repairs are covered by the focused regression tests and verification commands above. Exact Saturn API DTO shape/serialization parity remains intentionally deferred to a separately reviewed contract slice; this work did not rewrite `Invocation`, `Context`, `Result`, or `Identity`. Excluded routing, tool execution, turns, persistence, moderation, listeners, and command wiring were not modified.
