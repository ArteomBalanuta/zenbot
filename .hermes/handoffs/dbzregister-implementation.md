# `dbzregister` implementation and recovery evidence

## Scope completed

Only the source-exact `dbzregister` vertical was changed. Registration still uses the existing `REGULAR` catalog and shared dispatcher authorization, caller `message.Name`, ignored arguments, the fail-closed DBZ bundle gate, and the existing non-transactional H2 character insert → loose name select → stats insert. No schema, repository production behavior, factory, shared auth/identity, alias, order, uniqueness, transaction, retry, rollback, or DBZ help/spawn/stats/strength/fight behavior was changed by this vertical.

The registration acknowledgement is now the source-shaped checked public send:

```go
SendChatMessage("", "Successfully registered character: "+author, false)
```

A send error produces `FAILED` only after the DBZ call; database errors remain discarded at the command boundary and still receive the success acknowledgement.

## Strict tracer record

### Tracer 1: real H2 dispatch public delivery

Added `TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess` using normal `ResolveUserMetadata`, `DispatchUserCommand`, real H2, active authorized `goku`, whispered `!dreg ignored tokens`.

RED, before the production edit:

```text
$ go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess$'
--- FAIL: TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess (0.74s)
    dbz_dispatch_test.go:310: reply=message_test.recordedDBZReply{recipient:"goku", text:"Successfully registered character: goku", whisper:true}, want public exact registration acknowledgement
FAIL
FAIL    zenbot/internal/listener/message    1.254s
FAIL
```

Minimal GREEN changed only the `dbzregister` success delivery from `reply(...)` to the checked public `SendChatMessage` call.

GREEN:

```text
$ go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess$'
ok      zenbot/internal/listener/message    1.163s
```

The test asserts self-only persistence/default stats, no `ignored` character, and an empty recipient with `whisper=false`.

### Tracer 2: duplicate recovery protocol

The pre-existing `TestDBZRegistrationDuplicateAndMissingReadSemantics` already supplied the two-duplicate-character/two-stats regression property, so adding an equivalent test would have been immediate green rather than valid historical RED evidence.

For permitted recovery evidence only, the final stats `INSERT` in `RegisterCharacter` was temporarily isolated locally. No selection/order/transaction behavior was changed. The focused test then failed as expected:

```text
$ go test -count=1 ./internal/repository/h2 -run '^TestDBZRegistrationDuplicateAndMissingReadSemantics$'
--- FAIL: TestDBZRegistrationDuplicateAndMissingReadSemantics (0.65s)
    dbz_test.go:63: duplicate registration counts characters=2 stats=0
FAIL
FAIL    zenbot/internal/repository/h2    1.062s
FAIL
```

The exact final stats insert and return were immediately restored. No production isolation remains.

GREEN:

```text
$ go test -count=1 ./internal/repository/h2 -run '^TestDBZRegistrationDuplicateAndMissingReadSemantics$'
ok      zenbot/internal/repository/h2    1.068s
```

### Tracer 3: swallowed persistence error and checked transport error

Added direct command coverage with a narrow fake `repository.DBZRepository`:

- `TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails` records self name and returns a sentinel persistence error; it verifies `SUCCESSFUL`, nil command error, and exact public acknowledgement.
- `TestDBZRegisterReturnsFailedAfterPersistenceWhenPublicSendFails` verifies the DBZ call occurs before the checked public send error produces `FAILED`.

The persistence-error test was already green on its first run because the existing source-shaped command already discarded `DBZService.Register` errors. This is labelled regression/provenance evidence, not manufactured RED.

```text
$ go test -count=1 ./internal/command -run '^TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails$'
ok      zenbot/internal/command    0.532s

$ go test -count=1 ./internal/command -run '^TestDBZRegisterReturnsFailedAfterPersistenceWhenPublicSendFails$'
ok      zenbot/internal/command    0.338s
```

### Tracer 4: shared authorization and pre-cancellation boundaries

Added direct DBZ-register listener coverage:

- `TestDispatchUserCommandDBZRegisterDeniedRegularDoesNotPersistAndUsesSharedUnauthorizedReply` proves resolved active-user `REGULAR` denial stops before repository write and retains the shared whispered unauthorized response.
- `TestDispatchUserCommandDBZRegisterPreCancelledDoesNotPersistOrReply` proves the existing command pre-cancellation guard causes zero repository calls, zero H2 rows, and zero output. The legacy adapter logs the returned cancellation error; its void contract does not expose it to the dispatcher.

Both were first-run green regression/provenance evidence because shared authorization and the existing pre-persistence `ctx.Err()` guard were already present; no production code was altered to manufacture RED.

```text
$ go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZRegisterDeniedRegularDoesNotPersistAndUsesSharedUnauthorizedReply$'
ok      zenbot/internal/listener/message    1.247s

$ go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZRegisterPreCancelledDoesNotPersistOrReply$'
ok      zenbot/internal/listener/message    1.169s
```

## Owned changes

- `internal/command/dbz.go`: registration branch success transport only.
- `internal/command/dbz_test.go`: DBZ-register persistence-error and send-error regression coverage plus local fake seam.
- `internal/listener/message/dbz_dispatch_test.go`: real-H2 dispatch/public-delivery tracer and direct authorization/cancellation coverage plus local register-call recorder.
- `.hermes/handoffs/dbzregister-implementation.md`: this evidence record.

`internal/repository/h2/dbz.go` was temporarily edited only for recovery isolation and restored byte-for-byte to its original production behavior. Existing dirty DBZ help/spawn, H2 stats, and unrelated worktree changes are not owned by this vertical.

## Final gates

Executed after this handoff write:

```text
$ go test -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok      zenbot/internal/command             10.898s
ok      zenbot/internal/listener/message     5.676s
ok      zenbot/internal/service              2.318s
ok      zenbot/internal/repository/h2       37.950s

$ git diff --check && git diff --cached --check
exit 0; no output
```

`git status --short` remains heavily dirty from baseline shared work; this vertical added the three owned DBZ paths above and this untracked handoff, without staging, committing, or modifying the unrelated paths.
