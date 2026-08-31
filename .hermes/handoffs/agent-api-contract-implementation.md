# Agent API Contract Implementation Handoff

## Outcome
Implemented the Saturn-shaped agent API contract in `internal/agent/api` without modifying Saturn or unrelated target worktree changes. Existing runtime and assembler behavior remains intact; an explicit runtime compatibility adapter is provided rather than field guessing or JSON round-tripping.

## Files changed/created
- `internal/agent/api/api.go` — exact invocation mode, capability, context, invocation types; constructors, validation, accessors, memory-key derivation, defensive copies, and invocation JSON boundary.
- `internal/agent/api/result.go` — context JSON boundary plus exact three-component `Result`, reply/silent factories, validation, and JSON handling.
- `internal/agent/api/identity.go` — `UserIdentity` / `AgentUserIdentity`, normalized source precedence (`trip > hash > nick`), validation, and JSON handling.
- `internal/agent/api/api_test.go` — constructor/default, validation, defensive-copy, memory-key, identity, result, and JSON golden/missing/null/unknown-mode tests.
- `internal/agent/runtime/adapters.go` — explicit `ToAPIInvocation`, `FromAPIInvocation`, and `ToAPIResult` adapters for the existing runtime seam.
- `.hermes/handoffs/agent-api-contract-implementation.md` — this handoff.

## Design decisions
- Saturn field names and lower-camel JSON names are used: `requestId`, `context`, `prompt`, `mode`, `currentMessageText`, `commandOriginated`, `room`, `nick`, `trip`, `hash`, `whisper`, `roomUsers`, `capabilities`, `moderationTarget`, `correlationId`, `content`, `shouldReply`, and `value`.
- Nullable Saturn scalar fields are represented internally as pointers where null must survive JSON (`currentMessageText`, context trip/hash/moderation target). Required JSON fields use pointer-backed decode probes so omitted/null is rejected deterministically.
- Invocation generated-ID construction uses a UUID v4 from `crypto/rand`; default mode is `DIRECT`, current text is null, and command origin is false.
- Context copies input collections and returns copies. `MemoryKey` uses a Java-compatible UTF-16 room length and exact Saturn public/whisper precedence and formatting; no normalization is applied to key components.
- Identity normalization trims and lowercases the selected source, with trip/hash/nick precedence. Existing `internal/model.IdentityKey` was not changed.
- Saturn `Result` has no `errorCode`. `ToAPIResult` rejects runtime error envelopes with an error code rather than silently discarding it. The adapter documents the unavoidable runtime empty-string mapping for nullable current text and context nullable strings.
- Existing `internal/agent/runtime` and `internal/agent/assemble` implementation/callers were not behaviorally rewritten; the adapter is the explicit staged migration boundary.

## Verification (actual results)
All commands exited 0:
- `gofmt -w internal/agent/api internal/agent/runtime/adapters.go`
- `go test -count=1 ./internal/agent/api ./internal/agent/runtime ./internal/agent/assemble`
- `go test -race ./internal/agent/api ./internal/agent/runtime ./internal/agent/assemble`
- `go test -count=1 ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

Focused tests and the full repository suite passed. No Saturn files were touched. Existing unrelated dirty/untracked target files were preserved.

## Remaining gaps
- Saturn itself has no focused JSON golden tests or serializer annotations for these records, so JSON behavior is the explicit target policy proven by the new Go tests, not a claim of observed Saturn wire bytes.
- Existing assembler/runtime production signatures still use the private runtime seam; migration is available through explicit adapters and intentionally does not alter excluded execution behavior.
- Go cannot provide Java-style overloaded constructors; variadic constructors preserve the documented overload behaviors while returning Go errors for invalid input.
