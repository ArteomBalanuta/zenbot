# SQL Utility Group B Runtime-Parity Acceptance

## Verdict

**ACCEPTED: bounded SQL Utility Group B runtime parity only.**

This acceptance is limited to the implemented and independently QA-verified runtime slice over the already accepted Group B repository seam. It does not accept broader row #324 work, Group C, row #325, or the overall Saturn-to-Zenbot migration.

**Group C: NOT COMPLETE.**  
**Saturn audit row #325 (`Util`): NOT COMPLETE.**  
**Overall Saturn-to-Zenbot migration: NOT COMPLETE.**

## Evidence and exact changed files

The governing artifacts are:

- `.hermes/handoffs/sql-util-group-b-runtime-parity-architecture.md`
- `.hermes/handoffs/sql-util-group-b-runtime-parity-architecture-qa.md` — architecture QA **PASS / implementation gate open**
- `.hermes/handoffs/sql-util-group-b-runtime-parity-implementation.md`
- `.hermes/handoffs/sql-util-group-b-runtime-parity-qa.md` — independent QA **PASS for the owned Group B runtime-parity slice**
- This acceptance artifact: `.hermes/handoffs/sql-util-group-b-runtime-parity-acceptance.md`

The implementation handoff and independent QA agree on the exact task-owned application files. The nine modified files are:

- `internal/command/dispatch_adapter.go`
- `internal/command/handlers.go`
- `internal/command/identity_commands.go`
- `internal/command/mail_notes.go`
- `internal/command/users_nicks.go`
- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `internal/service/services.go`

The two new task-owned files are:

- `internal/command/remove.go`
- `internal/command/runtime_parity_red_test.go`

No change was made to `internal/factory/engine_factory.go` or `internal/command/registry.go`; their existing Group B injection and canonical `remove` catalog definition were inspected and preserved.

## Accepted runtime behavior

### Remove dispatch, aliases, and real execution

- `RegisterAll` retains one canonical moderator `remove` definition with aliases `del`, `delete`, and `remove`; no duplicate catalog entry was added.
- `RegisterUserUtilities` now registers canonical `remove` when the configured user service exposes the Group B capability. The existing adapter expands that single definition to all three aliases for the inbound listener.
- `handlers.go:newCommand` constructs the real `removeCommand`. The registered Group B path no longer reaches Saturn's `"remove accepted"` placeholder.
- `removeCommand` trims the first argument, rejects a blank or missing selector, invokes `UserService.DeleteIdentity(ctx, selector)`, and preserves the accepted Saturn-style responses:
  - success: `User has been removed successfully`
  - failure: `Something went wrong deleting the user`

### Selector resolution and no-delete boundaries

- The narrow exported capability is `repository.SaturnAuthorizedDeleteRepository.DeleteIdentityAuthorized(context.Context, string)`.
- H2 trims the single name-or-trip selector, rejects blank input, and performs parameterized case-insensitive matching over `trip_names JOIN names JOIN trips`.
- Deletion requires exactly one distinct stored `(name, trip)` pair. H2 resolves both stored values before calling the existing two-field delete transaction.
- Blank, missing, and ambiguous selectors return an error and a zero `DeleteResult`; the delete transaction is not invoked.
- The ordinary `DeleteIdentity(ctx, name, trip)` authorization gate remains intact. H2's authorization key, context constructor, predicate, and unauthorized error remain private; no command imports H2 and no private authorization context is exported.
- If the optional capability is unavailable, `UserService.DeleteIdentity` returns `authorized Group B delete unavailable` rather than manufacturing authorization or silently succeeding.

### Group B last-message routing

- With Group B present, `messagesCommand` calls `UserService.SaturnLastMessages(ctx, nil, trip, count)` and does not call the legacy `IdentityRepository.LastMessages` path.
- The typed `(Name, Message, CreatedOn)` result is explicitly adapted to the existing `name#trip: message` output using the command trip, because `SaturnLastMessage` intentionally has no trip field.
- Existing command behavior is preserved: requests above 30 are capped, non-positive counts are forwarded so the Group B repository applies Saturn's default of 5, and truncation/escaping remain active.
- Compatibility-only engines without Group B retain the legacy fallback; configured Group B engines take the Group B path.

### Registered-user routing

- `!users` prefers `UserService.SaturnRegisteredUsers(ctx)` when Group B is present, adapts the typed `Name,Trip` rows to the existing table formatter, and retains a legacy fallback only for engines without Group B.
- The mail unregistered-recipient branch calls `MailService.SaturnRegisteredUsers(ctx)` instead of the old direct-DB `MailService.RegisteredUsers()` formatter. The existing explanatory response is preserved.
- The separate `MailService.Queue` validation/queue path remains unchanged; this acceptance covers only the registered-user directory branch.

### Mail escaping regression fix

The mail directory formatter now emits Saturn's escaped newline convention (`name trip\\n`) from typed Group B rows. The runtime-parity regression test verifies the exact result `Merc trip\\n`. This prevents the directory response from reverting to the prior direct-DB formatting path while preserving the existing explanatory text.

## Actual passing gates

The following results were run from `/Users/ab/workspace/go-projects/zenbot` and are recorded by the implementation and independent QA handoffs:

```text
go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/factory -count=1  PASS
go test ./internal/command -run 'TestRuntimeParity' -count=1                 PASS
go test ./... -count=1                                                     PASS
go test -race ./... -count=1                                               PASS
go vet ./...                                                               PASS
go build ./...                                                             PASS
make test                                                                  PASS
make vet                                                                   PASS
make build                                                                 PASS
gofmt -l on task-owned Go files                                           clean
git diff --check                                                           PASS
```

The race run emitted a macOS linker warning about a malformed `LC_DYSYMTAB` in one test binary, but exited 0 and all tests passed. The runtime-parity tests cover alias registration and real execution, trimmed selector forwarding, invalid/no-delete behavior, Group B last-message routing with nil name and trip adaptation, `!users` Group B routing, and the escaped-newline mail regression. H2 tests cover unique case-insensitive resolution and ambiguous resolution without deletion.

## Preservation and attribution boundaries

- The accepted repository Group B seam remains `internal/repository/sql_util_group_b.go` plus `internal/repository/h2/sql_util_group_b.go`; no duplicate SQL abstraction or broad DI layer was introduced.
- Existing factory Group B injection was preserved: `NewEngineWithOptions` continues to inject the same Group B instance into both user and mail services.
- Protected `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` were not modified and have no task-owned worktree diff.
- Saturn at `/Users/ab/workspace/projects/saturn` was inspected read-only. No Saturn source was modified; its pre-existing dirty files were preserved.
- Pre-existing staged, dirty, and untracked repository work was preserved and is not attributed to this acceptance. No unrelated cleanup, normalization, reset, or revert was performed.
- No schema, transport, provider, listener, agent/sql policy, broad security redesign, or unrelated application migration was accepted.

## Explicit exclusions and final status

Not accepted by this document:

- **Group C** or any other migration group.
- **Saturn audit row #325 (`Util`)**.
- **Overall Saturn-to-Zenbot migration completion**, including remaining command, service, listener, agent, persistence, or SQLite-elimination work outside this slice.
- Full row #324 completion beyond the bounded Group B runtime-parity behavior described here.
- Changes to protected documents, Saturn, unrelated dirty files, schema, or broad security/architecture.

**FINAL: PASS — bounded SQL Utility Group B runtime-parity slice accepted only.**  
**Group C: NOT COMPLETE.**  
**Row #325: NOT COMPLETE.**  
**Overall Saturn-to-Zenbot migration: NOT COMPLETE.**
