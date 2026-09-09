# Local `msgchannel` / `msgroom` parity implementation

## Scope and source authority

Implemented only the bounded local-room branch selected by `.hermes/handoffs/next-core-command-after-support-relay-architecture.md`, grounded in:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java`
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/command/impl/user/MsgChannelCommandImplTest.java`
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`

No remote session, credentials, temporary nick, snapshot, proxy/Whiskey, transport, persistence, agent, lifecycle, or authorization-policy work was added.

## Delivered behavior

- `msgchannel` and `msgroom` remain aliases of canonical `msgchannel`, `USER` role, now live-registered as a concrete `msgChannelCommand`.
- Normal listener authorization remains the existing `DispatchUserCommand` gate; command-local authorization was not introduced.
- Local target parsing removes every `?`, trims the room and rendered joined message, and requires exact case-sensitive equality with `Engine.GetChannel()`.
- Local delivery calls `Engine.SendChatMessage(message.Name+" ", body, message.IsWhisper)` once. It preserves the source body variants and recipient trailing-space contract.
- Missing room/body sends ` Example: <prefix>msgroom your-room your message`; all-question-mark room sends `Room name cannot be blank.`; both return `FAILED`.
- Remote target returns `FAILED` and `remote room delivery is not configured`, with no outbound chat or remote workflow.
- Pre-cancelled context and send errors return `FAILED`, with no fabricated reply or retry.
- Removed only the old generic `saturnCommand` `msgchannel` case after concrete coverage existed. `ParseMsgChannel` and its existing parsing tests were deliberately left unchanged.

## TDD tracer record

1. Registration/authorization tracer
   - RED: `go test ./internal/command -run '^TestMsgChannel(Registration|DispatchAuthorization)' -count=1`
   - Result: failed to compile with `undefined: msgChannelCommand`, proving the concrete command/routing did not exist.
   - GREEN: added `msgChannelCommand`, canonical construction switch, and concrete user registration.
   - GREEN rerun: `ok zenbot/internal/command 0.939s`.

2. Local parse/render/delivery tracer
   - RED: `go test ./internal/command -run '^TestMsgChannelLocalDelivery$' -count=1`
   - Result: failed all source-shaped cases with unformatted `alice||...` sends rather than anonymous-mail bodies.
   - GREEN: added source-shaped local normalization, rendering, recipient spacing, and whisper propagation.
   - GREEN rerun: `ok zenbot/internal/command 0.468s`.

3. Failure/exclusion tracer
   - RED: `go test ./internal/command -run '^TestMsgChannel(FailuresAndRemoteExclusion|CancellationAndSendError)$' -count=1`
   - Result: missing-body branch panicked at `arguments[0]`, demonstrating missing source validation.
   - GREEN: added source usage/blank replies, controlled remote error/no-send branch, and propagated send errors; removed generic fallback case.
   - GREEN rerun: `ok zenbot/internal/command 0.568s`.

4. Listener integration tracer
   - `TestMsgChannelListenerIntegration` was added after the preceding direct-command tracer had already implemented the complete listener-visible local behavior. Its first isolated execution was green (`ok zenbot/internal/command 0.928s`), requiring no listener/adapter change. This is an honest exception to an otherwise RED→GREEN sequence; it verifies the existing `NewUserChatListener` normalization/authorization seam rather than claiming a new implementation transition.

## Files touched

- `internal/command/msg_channel.go` — new concrete bounded local handler.
- `internal/command/msg_channel_test.go` — registration, authorization, source local vectors, errors, remote exclusion, and listener tests.
- `internal/command/handlers.go` — one canonical `msgchannel` constructor case.
- `internal/command/dispatch_adapter.go` — one canonical concrete-user registration entry.
- `internal/command/registry.go` — removed only generic `msgchannel` fallback case.

The repository was already extensively dirty. The three existing production files above contain unrelated pre-existing hunks; this pass changed only the listed local-msgchannel hunks and did not reset, clean, restore, stage, or commit anything.

## Verification

All executed from `/Users/ab/workspace/go-projects/zenbot`:

```text
$ go test ./internal/command -run 'TestMsgChannel|TestMsgAndWhiskeyParsing|TestSupportRelay|TestReplica' -count=1
ok  zenbot/internal/command  2.415s

$ go test ./internal/listener/message ./internal/listener -run 'Test.*(Dispatch|Authorization|UserChat)' -count=1
ok  zenbot/internal/listener/message  16.472s
ok  zenbot/internal/listener          0.367s

$ go test ./internal/command ./internal/listener/message ./internal/listener -count=1
ok  zenbot/internal/command           12.902s
ok  zenbot/internal/listener/message  15.674s
ok  zenbot/internal/listener          0.260s

$ go test ./...
PASS: all listed packages; internal/command 12.710s; internal/listener/message 15.327s

$ go vet ./... && go build ./...
exit 0; no output

$ git diff --check && git diff --cached --check
exit 0; no output
```

`ws`/`wsa` support relay code was not changed by this vertical. Remote `msgchannel` parity remains intentionally incomplete and requires a separate architecture/implementation/QA vertical.
