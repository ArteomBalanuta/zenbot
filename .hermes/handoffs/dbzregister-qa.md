# `dbzregister` strict-parity QA

## Verdict: ACCEPT

Independent source comparison found no scope-owned defect. The existing registration delivery correction and its tests satisfy the bounded persistence contract. No production or test source was changed by this QA pass; this file is the only artifact created.

## Source-to-target findings

- **Surface and gate:** Saturn `DBZRegisterCommandImpl` declares exactly `dbzregister`, `dreg`, `dr` and `REGULAR`. Zenbot `internal/command/registry.go` matches those aliases/role; `RegisterUserUtilitiesWithDirectAgent` registers `dbzregister` only when `bundle(e).DBZ != nil`.
- **Identity/input:** Saturn ignores parsed arguments and registers `chatMessage.getNick()`. Zenbot persists only `message.Name`; the real-H2 dispatch tracer drives whispered `!dreg ignored tokens`, persists `goku`, and proves no `ignored` character.
- **Authorization/cancellation:** `DispatchUserCommand` resolves the active author and checks the resolved user against the command role before execution. Direct DBZ-register tests prove denied `REGULAR` causes no repository call/write and the shared unauthorized whisper; a pre-cancelled context causes no write and no output.
- **Output:** Saturn calls one-argument `OutService.enqueueMessageForSending`, which is public/unaddressed. Zenbot sends `SendChatMessage("", "Successfully registered character: "+author, false)`, including for a whispered inbound call. Output-send failure returns `FAILED` only after the registration call.
- **Persistence:** Saturn `DBZImpl.register` inserts a level-1 character, selects the first unordered matching name, then inserts stats `(free_stats,str,agi,vit,ene)=(0,1,1,1,1)`, swallowing SQL errors. Zenbot `h2.Database.RegisterCharacter` has the same three statement sequence, no `ORDER BY`, generated-key API, transaction, rollback, retry, lock, or compensation. Its command boundary discards the returned persistence error and still sends the success acknowledgement.
- **Schema:** Both Zenbot H2 schema copies match Saturn: nullable/non-unique character name, nullable level/default-stat columns, FK-only stats relationship, no DBZ indexes or unique/cardinality constraints. Focused diff inspection found no schema or `h2/dbz.go` production drift.
- **Duplicates:** Real-H2 coverage admits two same-name character rows and two stats rows; no test or code asserts that stats attach to the newest duplicate. This intentionally retains loose, non-transactional selection.

## Recorded tracer verification

Passed individually:

```text
go test -count=1 ./internal/listener/message -run '^(TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess|TestDispatchUserCommandDBZRegisterDeniedRegularDoesNotPersistAndUsesSharedUnauthorizedReply|TestDispatchUserCommandDBZRegisterPreCancelledDoesNotPersistOrReply)$'
ok  zenbot/internal/listener/message

go test -count=1 ./internal/command -run '^(TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails|TestDBZRegisterReturnsFailedAfterPersistenceWhenPublicSendFails|TestDBZAliasesAreRegularAndConcrete)$'
ok  zenbot/internal/command

go test -count=1 ./internal/repository/h2 -run '^(TestDBZRealH2RegisterAndQuirkyMutations|TestDBZRegistrationDuplicateAndMissingReadSemantics)$'
ok  zenbot/internal/repository/h2
```

All required final gates passed:

```text
go test -race -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
ok  zenbot/internal/command
ok  zenbot/internal/listener/message
ok  zenbot/internal/service
ok  zenbot/internal/repository/h2

go test -count=1 ./...
PASS (all packages; only expected [no test files] entries)

go vet ./...
PASS

go build ./...
PASS

git diff --check && git diff --cached --check
PASS (no output)
```

## Dirty-tree boundary

The checkout remains heavily dirty from concurrent work. Relevant pre-existing/owned worktree paths remain unstaged: `internal/command/dbz.go`, `internal/command/dbz_test.go`, `internal/command/dispatch_adapter.go`, `internal/command/registry.go`, `internal/listener/message/handlers.go`, `internal/repository/h2/dbz_test.go`, and untracked `internal/listener/message/dbz_dispatch_test.go`. No stage, reset, clean, commit, schema alteration, auth-policy change, or transaction/uniqueness policy was performed here.
