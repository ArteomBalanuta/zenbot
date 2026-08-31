# Bounded public `run_command` QA

## Verdict

**Accepted with one hardening fix.** The action remains a fixed, public informational-command vertical: command identity derives only from the trusted invocation context, normal command authorization is repeated, command output is delegated once and captured only after successful `SendChatMessage`, and the bounded loop has one tool call and two completions at most.

## Defect found and fixed

`live.NewBoundedToolLoop` initially treated the three expected tool names as the full frozen-composition guarantee. A caller could provide a different `tool.Tool` implementation that reused `user_message_history`, `room_users`, or `run_command`; it would be advertised and executed despite the closed registry names.

The loop now accepts only the concrete `tool.UserMessageHistory`, `tool.RoomUsers`, and non-nil-gateway `tool.RunCommand` types with their expected names. Added a RED regression test (`TestNewBoundedToolLoopRejectsForgedNamedTool`), observed it fail before the fix, then pass after it.

Added QA coverage that confirms:

- a failed `run_command` produces no synthesis completion and no retry;
- required-fresh routing ignores a provider-forged `run_command`, invokes only router-owned `user_message_history`, and makes the tools-disabled synthesis;
- a normal command action still has one gateway call, exactly two provider requests, no tools in completion #2, and no durable evidence.

## Boundary assessment

- **Allowlist/schema:** fixed aliases only (`help,h,list,users,info,ping,p,weather,w,time,t,version,v`); closed arguments schema with bounded 4,000-character tail; gateway repeats the allowlist and rejects unavailable/non-concrete definitions.
- **Trusted authority:** no provider field supplies nick, trip, hash, room, whisper, role, or capabilities. The gateway reconstructs its synthetic message from `api.Context`, finds the active caller, and repeats `IsUserAuthorized` against the selected definition role.
- **Delivery capture:** the request-local decorator overrides only `common.Engine.SendChatMessage`; it delegates once, records text only after success, and leaves all other engine methods untouched. It neither replays captured output nor changes listener/transport behavior.
- **Loop/lifecycle:** `run_command` is non-idempotent `Action` + `RoomDelivery`, 10 seconds, excluded from `Safe`, frozen with exactly the two read tools, max one call/two completions, excluded from whisper definitions, and has no persistable evidence. Existing post-delivery memory behavior therefore persists only ordinary final agent output after its successful send.
- **Routing:** the required-fresh branch occurs before provider-selected calls and is the only action permitted for a fresh request. Public mention/direct/ambient all share the tool loop when already admitted; whisper does not.

## Deliberate exclusions / residual limits

- No privileged/moderation/catalog/dynamic command execution, SQL work, retries, third completion, global capture, listener reordering, or transport changes.
- This review exercised fixture-backed command/tool/loop/composition paths; it did not connect to a real chat transport or a live provider.
- `go vet ./...` remains nonzero only for the pre-existing unrelated warning: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.

## Verification

Passed from `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./internal/command -run '^TestAgentCommandGateway' -count=1
go test ./internal/agent/tool ./internal/agent/tool/execution -run 'Test(RunCommand|Executor)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*(RunCommand|Command|Fresh|Whisper)|NewBoundedToolLoop|Runner.*RunCommand|Direct.*RunCommand)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/command ./internal/agent/tool ./internal/agent/tool/execution ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

All listed commands passed. `go vet ./...` produced only the known warning above.
