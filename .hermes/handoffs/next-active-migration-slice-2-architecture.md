# Next Migration Slice 2 — Mail Group C Parity

## Bounded scope

Implement only Saturn MailService behavior backed by these pending row #324 Group C constants:

- `INSERT_INTO_MAIL_OWNER_RECEIVER_MESSAGE_STATUS_IS_WHISPER_CREATED_ON_VALUES`
- `GET_TRIP_BY_NICK_REGISTERED_OR_TRIP`
- `SELECT_MAIL_BY_NICK_OR_TRIP`
- `UPDATE_MAIL_SET_STATUS_DELIVERED_WHERE_RECEIVER`

Source: `src/main/java/org/saturn/app/service/impl/MailServiceImpl.java` and `command/impl/user/MailUserCommandImpl.java` in Saturn. Target owners: `internal/service/services.go` (`MailService.Queue`, `Pending`, `MarkDelivered`), `internal/command/mail_notes.go` (`mailCommand`), and the existing `DeliverPendingMail` listener path.

## Source contract

Saturn normalizes target names, finds all trips matching name or trip case-insensitively, joins recipient trips by comma, appends a trailing space to nonempty message bodies, JSON-escapes the persisted message, stores PENDING mail with string whisper flag and current milliseconds, and acknowledges delivery scheduling. Unknown recipients receive the registered-user directory; blank receivers receive an exact error reply.

Pending mail reads only PENDING rows where the recipient list contains the trip selector. It preserves id, owner, receiver, message, status, whisper flag, and timestamp. Delivery changes status to DELIVERED by mail id.

## Target assessment

Zenbot already owns comparable paths. Existing differences must be asserted before production changes:

- `Queue` currently returns errors; Saturn logs SQL failures and remains command-successful.
- `Queue` currently does not JSON-escape persisted mail payload.
- `Pending` accepts both receiver name and trip; source method is trip-only, while listener-level delivery behavior must be preserved separately.
- `MarkDelivered` already updates by id.

Do not change behavior based on these observations without focused failing parity tests.

## Implementation plan

Route to `@developer` (Standard complexity). Add test-first coverage in new task-owned files:

- `internal/service/mail_group_c_parity_test.go`
- `internal/command/mail_group_c_parity_test.go`
- optionally `internal/listener/message/mail_group_c_parity_test.go` only if existing listener tests cannot exercise Pending/MarkDelivered behavior.

Use real H2 and assert normalization, case-insensitive matching, multiple receiver-trip serialization, exact payload escaping/trailing-space semantics, unknown/blank response paths, PENDING filtering, trip-boundary safety, id-based delivery update, and failed-write/error behavior. Modify only `internal/service/services.go`, `internal/command/mail_notes.go`, or existing mail listener owner if a focused test proves observable mismatch.

## Explicit exclusions

No Notes reopening; no mail schema change; no broad listener ordering rewrite; no moderation, agent, transport, H2 lifecycle, Group C ban/history constants, row #325, or protected document change.

## Gates

Focused real-H2/service/command/listener tests; `go test ./...`; `go test -race ./...`; `go vet ./...`; `go build ./...`; `gofmt -l` on task-owned files; `git diff --check`; preserved protected-document hashes.

## Acceptance boundary

A passing implementation accepts only this Mail Group C sub-scope. Full row #324 and overall migration remain NOT COMPLETE.