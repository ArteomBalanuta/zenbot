# Rapid agent tool-loop history QA

## Verdict: ACCEPT

Independent QA of the bounded `user_message_history` slice found one bounded-loop defect and repaired it. The slice now satisfies the reviewed security and response-count boundary; no protected documentation, commits, pushes, Saturn sources, or unrelated work were changed.

## Repair made

`internal/agent/live/tool_loop.go` previously rejected `finish_reason == "length"` only when completion #1 had no tool calls. A truncated completion carrying a syntactically valid tool call could therefore execute the history query and start completion #2.

The loop now rejects a length finish reason immediately after completion #1, before reading or executing calls. The regression test `TestToolLoopRejectsLengthBeforeExecutingTool` was RED first (the old code consumed the only scripted response by issuing completion #2 and panicked), then GREEN after the one-branch repair. It asserts one provider request and zero repository calls.

QA also strengthened existing loop coverage to prove the retained assistant tool-call ID matches the appended `tool_call_id`, and added `TestToolLoopStopsAfterCancellationDuringHistoryExecution`, which proves cancellation during the repository call returns `context.Canceled` and starts no follow-up completion.

## Boundary evidence

- H2 query is fixed and parameterized; it binds trusted original room, case-insensitive nick, `visibility = 'PUBLIC'`, and the capped limit. It returns newest-window rows chronologically. Real-H2 tests cover public/whisper/legacy-null/other-room exclusion, invalid input, and injection-shaped/whitespace-shaped rooms.
- The sole descriptor is closed to `nick` (string, 1..100); model room and limit are absent. Tool normalization accepts one leading `@`, derives room only from invocation context, and returns JSON rows limited to name/message/createdOn/channel plus count. Repository errors are stable `TOOL_EXECUTION_FAILED` envelopes without driver text.
- The request-local loop permits one named, nonblank-ID history call, one matching assistant/tool pair, and exactly one no-tools follow-up. It rejects whisper calls, batch/unknown/invalid calls, length, blank final synthesis, and any second response tool call. Whisper completion #1 receives no definitions and cannot execute history.
- Live runner and direct invoker share the loop when composed. Direct responses still pass through `MarkerFinalizer`; room failure/silence handling was not changed by this QA repair.

## Verification executed

```text
$ go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessagesForNick' -count=1
ok  zenbot/internal/repository/h2

$ go test ./internal/agent/tool -run 'TestUserMessageHistory' -count=1
ok  zenbot/internal/agent/tool

$ go test ./internal/agent/live -run 'Test(ToolLoop|Runner.*Tool|Direct.*Tool|.*History)' -count=1
ok  zenbot/internal/agent/live

$ go test ./cmd/zenbot -run 'Test.*LiveAgent|Test.*DirectAgent' -count=1
ok  zenbot/cmd/zenbot

$ go test -race ./internal/agent/tool ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1
ok  zenbot/internal/agent/tool
ok  zenbot/internal/agent/live
ok  zenbot/internal/agent/runtime
ok  zenbot/internal/repository/h2

$ go test ./...
PASS: all packages

$ go build ./...
PASS

$ git diff --check
PASS
```

## Vet attribution and exclusions

`go vet ./...` still exits nonzero solely with the existing unrelated warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

It is outside this history-loop slice and was intentionally not changed. Excluded: other tools, dynamic SQL/schema, broad routing, memory, moderation, Saturn source changes, unrelated dirty work, commit, and push.
