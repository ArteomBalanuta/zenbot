# Mandatory current public history lookup — implementation

## Delivered

Implemented the source-shaped mandatory fresh `user_message_history` vertical without expanding the frozen one-tool/two-completion contract.

### Files modified

- `internal/agent/turn/freshness.go`
  - Canonicalized recognized profile/history classification under `turn.FreshnessPolicy`.
  - Added source-shaped explicit named-user, profile, possessive, speech, history, escaped-underscore, and latest-user-prompt follow-up recognition.
  - Preserved false-positive exclusions and added the canonical exported `IsValidNick` (Unicode letter/number/underscore/dash, 1–100 runes).
- `internal/agent/turn/turn_test.go`
  - Added positive, false-positive, escaped underscore, and follow-up policy coverage.
- `internal/agent/assemble/assemble.go`
  - Removed duplicate permissive assembler parser; assembly delegates once to `turn.FreshnessPolicy`.
  - Retained moderation clearing and existing whisper tool advertisement behavior.
- `internal/agent/assemble/assemble_test.go`
  - Added assertions that `who is president`, room presence, and Java do not produce a fresh requirement.
- `internal/agent/live/tool_loop.go`
  - Added a private required-history branch after completion #1 and before normal model-selected calls.
  - Ignores all first-response calls/prose, builds canonical JSON `{"nick":"<trusted nick>"}`, creates `fresh-history-<request-id>`, and executes only trusted current-room history through the existing executor/ledger.
  - Validates registry/descriptor safety, nick, tool result, cancellation, and final synthesis. Failure, truncation, blank/tool-call/repeated final synthesis fail closed with no second/third fallback where applicable.
  - Emits exactly one synthetic assistant/tool pair with matching generated ID and no dangling provider calls; success produces the existing request-local durable evidence candidate.
- `internal/agent/live/tool_loop_test.go`
  - Added automatic lookup, hostile initial tool diversion, repository failure, and repeated-synthesis fail-closed coverage.

## TDD record

RED observed:

- `go test ./internal/agent/turn -run TestFreshnessPolicyRecognizesOnlySourceShapedPublicHistoryRequests -count=1` initially failed for the missing named-user form.
- `go test ./internal/agent/assemble -run TestTruncateFreshnessBoundsAndCancellation -count=1` initially failed because the duplicate parser classified `who is president`.
- `go test ./internal/agent/live -run TestToolLoopForcesTrustedHistoryForRecognizedPublicRequest -count=1` initially failed because the first response was returned without repository access.

GREEN/verification:

- Focused turn/assemble/live gates passed.
- `go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1` passed.
- `go test -race ./internal/agent/turn ./internal/agent/assemble ./internal/agent/live ./internal/agent/runtime -count=1` passed.
- `go test ./... -count=1` passed.
- `go build ./...` passed.
- `git diff --check` passed.

## Security, cancellation, durability

- Router-owned arguments contain only trusted extracted nick; room remains exclusively derived by `UserMessageHistory` from invocation context.
- Whisper and moderation remain excluded from mandatory public history execution; normal whisper attempted-call rejection remains unchanged.
- Cancellation is checked before provider/tool follow-up; executor receives the request context and no completion #2 occurs after lookup cancellation/error.
- Evidence is created only after validated successful history execution and remains request-local; existing Runner/DirectInvoker post-delivery persistence remains unchanged, so failures, silence, and invalid final synthesis do not append conversation/evidence.

## Gates and exclusions

No tool/schema/SQL/repository query/config/moderation/command gateway/router expansion, third completion, retry, commit, push, or protected-document edit was made. Existing direct/live composition continues to share the frozen loop.

`go vet ./...` still reports the known unrelated dirty-tree warning:

```
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```
