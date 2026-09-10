# Rapid agent tool-loop history implementation

## Scope delivered

Implemented the bounded `user_message_history` vertical from `rapid-agent-tool-loop-history-architecture.md`.

### Touched files

- `internal/repository/agent_context.go` — added `AgentUserMessageHistoryRepository`.
- `internal/repository/h2/agent_context.go` — added fixed parameterized public/current-room/case-insensitive-nick query and interface assertions.
- `internal/repository/h2/agent_user_history_test.go` — real-H2 scope, visibility, ordering, limit, injection-shaped input tests.
- `internal/agent/tool/user_message_history.go` — one closed descriptor and restricted JSON history result.
- `internal/agent/tool/user_message_history_test.go` — descriptor, nick normalization, trusted scope, restricted output, and generic-error tests.
- `internal/agent/live/tool_loop.go` — request-local maximum-one-call / maximum-two-completion loop.
- `internal/agent/live/tool_loop_test.go` — exact assistant/tool pairing and whisper no-definition/no-execution tests.
- `internal/agent/live/runner.go` — optional loop delegation while retaining nil-loop legacy path.
- `internal/agent/live/direct.go` — optional loop delegation and marker finalization parity.
- `cmd/zenbot/main.go` — one frozen registry/loop built from the existing open DB and passed to both live and direct paths.
- `cmd/zenbot/live_agent_test.go` — narrow repository stub now satisfies history interface.

`cmd/zenbot/main.go`, live files, and the repository/context files were already uncommitted at task start; unrelated work was retained.

## Security and bounded-loop semantics

- The only function definition is `user_message_history`, with closed `{nick}` arguments, min 1/max 100, no model room or limit.
- Storage binds the original trusted room and nick as SQL parameters, uses case-insensitive equality, requires `visibility = 'PUBLIC'`, selects newest `(created_on DESC,id DESC)` then returns chronological `(created_on ASC,id ASC)`.
- Tool output contains only `name`, `message`, `createdOn`, `channel`, and `returnedCount`; no trip/hash/row IDs.
- The composition-held limit is `max(1, min(ContextMessageLimit, 60))`.
- Public requests receive one definition. Whisper requests receive none and a hallucinated call fails before repository access.
- First completion may make exactly one named call with a nonblank ID. An assistant tool-call message plus a matching-ID `tool` envelope are appended. Completion two receives no tools; tool calls, length, or blank output fail. No retry, batch, third completion, or parallel tools.
- Parent cancellation is checked before provider work, after the first completion, after execution, and after the follow-up; cancellation prevents the follow-up.
- Repository failures become a stable `TOOL_EXECUTION_FAILED` envelope without SQL/driver details.

## Observed RED → GREEN

### RED

```text
$ go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessagesForNick' -count=1
... db.RecentPublicRoomMessagesForNick undefined ...
FAIL zenbot/internal/repository/h2 [build failed]

$ go test ./internal/agent/tool -run 'TestUserMessageHistory' -count=1
... undefined: agenttool.UserMessageHistory
FAIL zenbot/internal/agent/tool [build failed]

$ go test ./internal/agent/live -run TestToolLoopMakesOneFollowUpWithMatchingToolID -count=1
... undefined: ToolLoop
... undefined: ToolLoopLimits
FAIL zenbot/internal/agent/live [build failed]
```

### GREEN / required gates

```text
$ go test ./internal/repository/h2 -run 'TestRecentPublicRoomMessagesForNick' -count=1
ok zenbot/internal/repository/h2

$ go test ./internal/agent/tool -run 'TestUserMessageHistory' -count=1
ok zenbot/internal/agent/tool

$ go test ./internal/agent/live -run 'Test(ToolLoop|Runner.*Tool|Direct.*Tool|.*History)' -count=1
ok zenbot/internal/agent/live

$ go test ./cmd/zenbot -run 'Test.*LiveAgent|Test.*DirectAgent' -count=1
ok zenbot/cmd/zenbot

$ go test -race ./internal/agent/tool ./internal/agent/live ./internal/agent/runtime ./internal/repository/h2 -count=1
ok zenbot/internal/agent/tool
ok zenbot/internal/agent/live
ok zenbot/internal/agent/runtime
ok zenbot/internal/repository/h2

$ go test ./...
PASS (all packages)

$ go build ./...
PASS

$ git diff --check
PASS
```

`go vet ./...` remains blocked only by the known pre-existing unrelated warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

No attempt was made to scope-creep into that core warning.
