# `grant` / `access` functional-parity QA

## Verdict

**PASS — accepted as a user-approved functional parity baseline.** The current dirty worktree matches the approved `AccessUserCommandImpl` behavior at the command/persistence boundary and adds a safe writable-authorization registration gate.

This is **not historical strict-TDD proof** for the core command. Core tracers 1–5 are regression-tested functional evidence only; `.hermes/handoffs/access-command-parity-tdd-forensic.md` correctly records no historical sequential RED→GREEN evidence. The separate non-nil `Security.Authorization` capability gate (tracer 6) is strict-TDD-evidenced: its recorded RED failure preceded the minimal registration change.

## Independent source and target audit

| Area | Result | Evidence |
|---|---|---|
| Catalog / aliases / required role | PASS | Saturn `AccessUserCommandImpl.java` declares `grant`, `access`; inherited `UserCommandBaseImpl.getAuthorizedRole()` is `ADMIN`. Zenbot `internal/command/registry.go` maps canonical `access`, aliases `grant,access`, `model.ADMIN`; listener tracer proves the concrete alias is registered and shared dispatch calls authorization before mutation. |
| Existing shared authorization dispatch | PASS within approved baseline | `internal/listener/message/handlers.go:DispatchUserCommand` calls `IsUserAuthorized(c.Author, cmd.GetRole())` before contextual or ordinary execution. No command-local authorization was added. The known nickname-to-active-user identity limitation remains accepted scope, not a security proof. |
| Single target / aliases / output / whisper | PASS | Both aliases issue one `GrantTrip(target, ADMIN)`, send `\\n Granted new Role: ADMIN to trip: target` to the author, and preserve `type:"whisper"` through `UserChatListener` / `reply`. |
| Input and role parsing | PASS | Exactly two arguments plus nonblank invoker trip produce the exact source-shaped usage reply; only six uppercase role names parse; lowercase is failed with no write/reply. |
| Comma target quirk | PASS | `first,second, ADMIN` makes `USER` writes for `first` and `second`, drops the trailing empty component (Java `split` behavior), emits the requested role in the plural reply, and preserves the source best-effort ignored per-item errors. |
| H2 writes / errors | PASS | `Database.GrantTrip` performs select/update-or-insert transactionally and its real-H2 test covers all six persisted roles and update. A single-target mutation error yields `FAILED` and no success reply. Target propagation/transactionality deliberately differs from Saturn's logged/swallowed SQL failure behavior. |
| Writable-registration capability | PASS | `access` is registered only if `bundle`, `Users`, `GroupB`, `Security`, and non-nil `Security.Authorization` are present; neither alias dispatches without that writer. |
| Registration scope regression | FIXED | Initial guard accidentally also withheld `register` and `messages` without `Security.Authorization`. New test first failed (`reg must retain its Group B registration without Security.Authorization`); minimal fix retains their pre-existing Group-B + `Security` registration and gates only `access` on the writer. |
| Agent surface | PASS | No `grant` / `access` agent command/tool route was added; no agent files were changed by this slice. |

## Test evidence

Passed after the focused fix:

```text
go test ./internal/command -run '^TestAccessAuthorizationGateDoesNotSuppressOtherGroupBCommands$' -count=1 -v
go test ./internal/command -run '^TestAccess(ParityTracer|AuthorizationGate|Command)' -count=1 -v
go test ./internal/repository/h2 -run 'Test(GrantTripInsertsAndUpdatesAllPersistedRoles|AuthorizationConfiguredWildcardAndThreshold)' -count=1 -v
go test ./internal/service -run 'TestSecurity(UsesPersistedAuthorizationAndConfiguredWildcard|FailsClosedOnNilPrincipalAndRole|ServiceAuthorizeTripPropagatesRepositoryError)' -count=1 -v
go test ./internal/listener/message -run 'TestDispatchUserCommand' -count=1 -v
go test -race ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message -count=1
go test ./... -count=1
go vet ./...
go build ./cmd/zenbot
git diff --check
```

All commands exited zero. The build artifact was removed after verification. `git diff --check` remained clean.

## Files changed in this QA pass

- `internal/command/dispatch_adapter.go` — narrowed the non-nil authorization writer condition to `access` alone.
- `internal/command/access_command_parity_test.go` — added the RED→GREEN regression test proving `register` and `messages` remain exposed under their prior prerequisites.
- `.hermes/handoffs/access-command-parity-qa.md` — this QA record.

No staging, reset, restore, clean, checkout, stash, commit, or recovery-worktree access occurred.

## Remaining limitation

Do not re-label the core command as historically strict-TDD-proven. If that proof is required, use the isolated recovery plan in `.hermes/handoffs/access-command-parity-tdd-recovery-architecture.md`; it must run tracers 1–5 sequentially from the documented pre-foundation baseline. The approved functional parity baseline does not close the separate ingress authenticated-principal concern documented by the superseded/current architecture handoff.
