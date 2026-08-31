# Identity command/history Saturn parity QA

## Verdict

**PASS — accepted.** The bounded `reg/register`, `authorize/auth`, `grant/access`, and `messages/lastmessages` slice passed focused real-H2 and command coverage plus the required repository-wide Go gates.

## QA hardening performed

A new red test exposed an attributable `grant/access` parity defect:

- Go `strings.Split` preserves trailing empty fields, while Saturn Java `targetTrip.split(",")` drops trailing empty fields.
- The target also trimmed comma-segment values before grants, unlike Saturn.

The command now drops trailing empty comma fields and grants each remaining segment as raw text with `USER`, preserving Saturn’s comma-target quirk and Java list-format reply.

### RED evidence

```text
$ go test ./internal/command -run '^TestAccessCommandCommaTargetsUseUserAndJavaSplitSemantics$' -count=1
--- FAIL: TestAccessCommandCommaTargetsUseUserAndJavaSplitSemantics (0.01s)
    identity_commands_test.go:168: status=SUCCESSFUL err=<nil> grants=[first:User second:User :User] chats=[mod|\\n Granted new Roles: ADMIN to trips: [first second ]|false]
FAIL
FAIL    zenbot/internal/command    0.526s
FAIL
```

### GREEN and focused real-H2 evidence

```text
$ go test ./internal/command -run '^TestAccessCommandCommaTargetsUseUserAndJavaSplitSemantics$' -count=1
ok      zenbot/internal/command    0.942s

$ go test ./internal/command -count=1
ok      zenbot/internal/command    5.553s

$ go test ./internal/repository/h2 -count=1
ok      zenbot/internal/repository/h2    20.677s
```

The Group-B H2 test exercises actual schema/database behavior: nullable name query scope, `(name OR trip)` scope, PUBLIC-only filtering, `LEFT` exclusion, row-trip mapping, identical-`created_on` ordering by `id DESC`, and limit behavior.

## Saturn/source comparison

Read-only Saturn sources inspected:

- `src/main/java/org/saturn/app/command/impl/moderator/RegisterUserCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/moderator/AuthorizeTripCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/admin/AccessUserCommandImpl.java`
- `src/main/java/org/saturn/app/command/impl/moderator/LastMessagesCommandImpl.java`
- `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`

Confirmed implemented parity points: case-sensitive `Role.valueOf` equivalent parsing; comma targets receive `USER`; row trip is rendered; Group-B history is PUBLIC-only, excludes `LEFT`/`JOINED`, keeps `(name OR trip)`, and orders `created_on DESC,id DESC`.

`internal/repository/h2/identity.go` was inspected. Its legacy `LastMessages` query already had the required PUBLIC/lifecycle/filter/order behavior. No task-owned change was made to it.

## Broad Go gates

```text
$ go test ./...
ok      zenbot/internal/repository/h2    (cached)
... all packages passed; exit 0

$ go test -race -p 1 ./...
ok      zenbot/internal/command          13.808s
ok      zenbot/internal/repository/h2    21.941s
... all packages passed; exit 0

$ go vet ./...
(exit 0; no output)

$ go build ./...
(exit 0; no output)

$ git diff --check
(exit 0; no output)
```

The first unsynchronized `go test ./...` attempt failed only in `internal/repository/h2` with fixed-port H2 lifecycle symptoms (`:55436`, exclusive database, duplicate existing index, EOF). This did not reproduce after confirming no listener on `:55436`; a subsequent literal `go test ./...` passed. The failure is test-harness/environment contention, not attributable to this slice. `go test -p 1 ./...` and `go test -race -p 1 ./...` also passed. The race link emitted one macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but the package and command completed successfully.

## Files modified / audited

QA direct changes:

- `internal/command/identity_commands.go` — exact comma split/grant parity fix.
- `internal/command/identity_commands_test.go` — trailing-comma raw-role and row-trip coverage.
- `.hermes/handoffs/identity-next-slice-qa.md` — this report.

Slice implementation/test changes audited:

- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `internal/repository/h2/identity.go` (inspection only; no diff)

No protected migration documents were modified, and no commit/push was performed.

## Protected hashes

```text
bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0  MIGRATION_PLAN.md
75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18  .hermes/migration-audit.md
```

## Evidence limitations

- Saturn was inspected as read-only source; Saturn’s unobserved `modService.auth` persistence/error internals were not asserted.
- Command tests use fakes at the command/service seam. History and existing identity/authorization tests provide real-H2 coverage.
- The unmodified H2 suite uses a fixed test port, so concurrent external test runs can interfere; final standalone and serialized gates passed.
