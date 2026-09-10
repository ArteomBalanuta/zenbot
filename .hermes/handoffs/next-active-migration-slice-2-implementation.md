# Mail Group C implementation handoff

## Scope

Completed the bounded Mail Group C parity correction:

- `MailService.Queue` now persists the JSON-escaped payload Saturn stores.
- The success-path command test now expects that escaped persisted payload.
- The unknown-receiver directory test now matches Saturn's literal `\\n` directory convention.

No command production code changed.

## Exact touched paths

- `internal/service/services.go`
- `internal/command/mail_group_c_parity_test.go`
- `.hermes/handoffs/next-active-migration-slice-2-implementation.md`

## Production edit

After preserving the established non-empty trailing-space behavior, `MailService.Queue` marshals the message with `encoding/json` and strips the surrounding JSON quotes before insertion. Thus quotes, backslashes, and newlines persist as JSON escapes while the trailing space remains intact.

## Test corrections

`internal/command/mail_group_c_parity_test.go` was aligned with already-proven behavior:

- unknown-recipient directory suffix expects the emitted literal `\\n`;
- queued quote-bearing payload expects `quote \\"x\\" line `, matching `MailService.Queue` serialization.

## Actual RED evidence

```text
go test ./internal/service -run TestMailGroupC -count=1
--- FAIL: TestMailGroupCQueueSerializesResolvedTripsAndEscapedPayload
    mail_group_c_parity_test.go:73: mail owner="alice#origin" receiver="trip-a,trip-b" message="quote \"x\"\nline " status="PENDING" whisper="true" createdOn=1788160289306
FAIL    zenbot/internal/service
```

```text
go test ./internal/command -run TestMailGroupC -count=1
--- FAIL: TestMailGroupCParityUnknownReceiverDirectory
    mail_group_c_parity_test.go:104: unknown-receiver chats=["alice|User you specified is not registered. Please use a name from provided list to send a message to respective trip. \\\\nMerc trip-a\\n|true"] want="alice|User you specified is not registered. Please use a name from provided list to send a message to respective trip. \\\\nMerc trip-a\n|true"
FAIL    zenbot/internal/command
```

After the production change, the command suite correctly exposed the now-stale raw-payload expectation before that test was updated.

## Verified focused gates

Run from `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./internal/service -run TestMailGroupC -count=1
ok      zenbot/internal/service    1.614s

go test ./internal/command -run TestMailGroupC -count=1
ok      zenbot/internal/command    2.428s

git diff --check
PASS (no output)
```

## Scope boundary

No schema, command production, listener, agent, transport, or protected-document changes were made. Full-suite validation remains outside this bounded task.
