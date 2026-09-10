# Row #324 Group B — Service Integration Implementation

## Scope

Implemented Split A only: typed, service-only, unwired compatibility reads for Saturn registered users and Saturn last messages. No delete service or capability was added. Existing Zenbot `RegisteredUsers`, `LastMessages`, command wiring, output, visibility, schema, and authorization behavior were not changed.

## TDD evidence

### RED (before production edits)

Command:

```text
go test ./internal/service ./internal/factory -run 'Test(UserServiceSaturn|MailServiceSaturn|LegacyServiceMethods|NewEngineInjectsGroupB|NewEngineLegacyOnly)' -count=1
```

Actual result: failed to compile because the requested production seam was absent:

```text
internal/service/group_b_test.go:36:28: unknown field GroupB in struct literal of type UserService
internal/service/group_b_test.go:36:43: (&UserService{…}).SaturnLastMessages undefined
internal/service/group_b_test.go:67:28: unknown field GroupB in struct literal of type MailService
internal/service/group_b_test.go:67:43: (&MailService{…}).SaturnRegisteredUsers undefined
internal/factory/group_b_test.go:44:43: e.Services.Users.GroupB undefined
internal/factory/group_b_test.go:44:77: e.Services.Mail.GroupB undefined
FAIL
```

The complete compiler output also reported the corresponding missing symbols for the nullable-input and error-propagation tests. This was the expected RED failure.

### GREEN

After the minimal production edits:

```text
go test ./internal/service ./internal/factory -run 'Test(UserServiceSaturn|MailServiceSaturn|LegacyServiceMethods|NewEngineInjectsGroupB|NewEngineLegacyOnly)' -count=1
```

Actual result:

```text
ok  zenbot/internal/service  0.230s
ok  zenbot/internal/factory  0.343s
```

The later exact requested command also completed successfully:

```text
go test ./internal/service ./internal/factory -run GroupB -count=1
```

It reported `ok` for both packages (the service package had no tests matching the literal `GroupB` filter; the factory Group B tests ran).

## Implementation

- Added optional `GroupB repository.SqlUtilGroupBRepository` fields to `UserService` and `MailService`.
- Added `UserService.SaturnLastMessages(context.Context, *string, string, int)` returning `[]repository.SaturnLastMessage` unchanged.
- Added `MailService.SaturnRegisteredUsers(context.Context)` returning `[]repository.SaturnRegisteredUser` unchanged.
- Delegation preserves nullable names, count values including non-positive values, typed Saturn row shapes, and repository errors.
- Nil Group B fields return an explicit unavailable error rather than panicking.
- Factory type-asserts `repository.SqlUtilGroupBRepository` when building database-backed services and injects the same optional implementation into both owners.
- Legacy-only/no-database construction remains nil-safe and does not require Group B.

## Tests added

- `internal/service/group_b_test.go`: typed delegation, nullable name/count forwarding, error propagation, and separation from legacy service contracts.
- `internal/factory/group_b_test.go`: optional Group B injection and legacy-only construction safety.

`internal/factory/engine_factory_test.go` was restored byte-for-byte to its pre-task state and has no working-tree diff.

## Verification results

Passed:

```text
go test ./internal/service ./internal/factory ./internal/command -count=1
ok

go test -race ./internal/service ./internal/factory ./internal/command -count=1
ok

go test ./... -count=1
ok (all packages)

go test -race ./... -count=1
ok (all packages)
go vet ./...
(empty output, exit 0)
go build ./...
(empty output, exit 0)
```

The full race run emitted an existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exited 0 and all packages passed.

Formatting/whitespace checks:

```text
gofmt -l .
internal/factory/engine_factory_test.go
```

This is a pre-existing formatting issue in the protected, unchanged factory test; it was restored byte-for-byte as requested. The changed files were individually gofmt'd.

```text
git diff --check
```

This reports pre-existing trailing whitespace in already-staged unrelated handoff documents and the pre-existing blank line at EOF in `internal/factory/engine_factory_test.go`. No new whitespace issue was introduced by the implementation. `git diff --cached --check` reports the same pre-existing staged-document findings.

## Changed files attributable to this implementation

- `internal/service/services.go`
- `internal/service/group_b_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/group_b_test.go`
- `.hermes/handoffs/sql-util-group-b-service-integration-implementation.md` (this report)

No changes were made to `internal/factory/engine_factory_test.go`, `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, Saturn source, repository Group B seams, commands, or unrelated dirty files.

## Explicit exclusions and limitations

- Delete integration and any authorization/capability design remain excluded and unwired.
- This is not the full Row #324 migration and does not claim overall migration completion.
- The new methods are typed compatibility APIs only and remain unwired from commands, agent/sql, listeners, mail directory formatting, public history, and transport.
- No PUBLIC filtering was added to Saturn last-message compatibility reads.
- Saturn-shaped rows are not coerced into existing Zenbot types.
- Full-repository `gofmt -l .` and `git diff --check` are not clean solely because of pre-existing unrelated/staged content preserved by scope.
