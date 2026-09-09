# Local `msgchannel` / `msgroom` parity QA

## Verdict

**Accepted as a bounded local-room vertical.** The concrete `msgchannel` handler preserves the source-tested local behavior for both exact aliases and deliberately rejects remote targets without constructing a session or performing outbound delivery. This is not acceptance of Saturn's remote-room branch.

## Independent parity findings

- Catalog and registration: `msgchannel` and `msgroom` resolve to canonical `msgchannel` with `USER` authorization metadata. `RegisterUserUtilities` registers the concrete legacy adapter, whose definition creates `*msgChannelCommand`, rather than the generic `*saturnCommand` fallback.
- Authorization: the normal `DispatchUserCommand` authorization gate remains before execution. An allowed active user reaches the command; a denied resolved user receives only the existing addressed denial and causes no command delivery.
- Local predicate and rendering: the command removes every `?` from the first post-alias field, trims it, requires exact case-sensitive equality with `Engine.GetChannel()`, joins the remaining fields with one ASCII space, and trims the rendered message. It preserves the source ordinary and literal-`![](` formats, including the source literal `\\n` marker and the deliberate trailing space in `message.Name+" "`.
- Validation and errors: missing room/body routes the exact source-shaped ` Example: <prefix>msgroom your-room your message` reply; an all-question-mark room routes `Room name cannot be blank.`. Pre-cancelled execution sends nothing and returns `context.Canceled`. A local send error returns `FAILED` without reply or retry. A remote normalized target returns `FAILED` with `remote room delivery is not configured`, and produces no chat/raw side effect.
- Isolation: `msg_channel.go` imports only context/format/strings/model. It does not touch temporary sessions, snapshots, credentials/password, generated nicks, proxy, `LIST_CMD`, transports, persistence, replica lifecycle, or agent runtime. The generic `msgchannel` fallback was removed; `ParseMsgChannel` remains unchanged for its unrelated existing callers.

## Tracer-history audit

The first three implementation tracers have recorded antecedent RED failures and Green reruns. The final `TestMsgChannelListenerIntegration` tracer first ran green after direct command behavior already existed; it is **regression-only**, not a RED→GREEN implementation tracer. No history is represented otherwise.

## Verification executed

All commands ran from `/Users/ab/workspace/go-projects/zenbot` and passed:

```text
go test ./internal/command -run 'TestMsgChannel|TestMsgAndWhiskeyParsing|TestSupportRelay|TestReplica' -count=1
go test -race ./internal/command -run 'TestMsgChannel|TestSupportRelay|TestReplica' -count=1
go test ./internal/listener/message ./internal/listener -run 'Test.*(Dispatch|Authorization|UserChat)' -count=1
go test -race ./internal/command ./internal/listener/message ./internal/listener -run 'TestMsgChannel|Test.*(Dispatch|Authorization|UserChat)' -count=1
go test ./...
go vet ./...
go build ./...
git diff --check
git diff --cached --check
```

The final race gate passed for `internal/command` (23.541s), `internal/listener/message` (31.705s), and `internal/listener` (1.338s). `go test ./...`, vet, build, and both diff checks exited zero.

## Scope and worktree record

No scoped defect was found, so this QA pass made no production/test change and required no new RED/fix/GREEN cycle. The repository was already broadly dirty. This pass did not stage, reset, clean, restore, or commit anything. The only QA artifact is this handoff. Remote `msgchannel`/`msgroom` delivery remains intentionally unavailable and needs a separate architecture/implementation/QA vertical before broader parity can be claimed.
