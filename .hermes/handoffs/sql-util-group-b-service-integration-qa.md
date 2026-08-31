# Row #324 Split A — Independent Service Integration QA

## Verdict

**PASS — Split A bounded service integration.**

I inspected the actual worktree files, architecture and implementation handoffs, Saturn source under the verified `src/main/java` paths, existing Zenbot callers, and dirty-tree attribution. The implementation satisfies the authorized scope: typed, service-only, unwired compatibility reads for Saturn registered users and Saturn last messages. No genuine Split A defect was found, so no source or test fix was made by this QA.

**Delete integration remains incomplete. Full row #324 remains incomplete. The overall Saturn-to-Zenbot migration remains incomplete.**

## Scope and inspected files

Authorized Split A source/test files inspected:

- `internal/service/services.go`
- `internal/service/group_b_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/group_b_test.go`

The existing `internal/factory/engine_factory_test.go` was inspected as a protected pre-task baseline. No worktree edit was present, and its worktree bytes compare identical to the staged baseline snapshot (`cmp` exit 0). The staged file is an existing added file in this broadly dirty worktree; the index-vs-HEAD comparison is not meaningful because that path is absent from HEAD.

Verified Saturn reference paths (read-only):

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/MailServiceImpl.java`
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SqlUtil.java`

## Semantic verification

### Typed delegation and result preservation

- `UserService.SaturnLastMessages(context.Context, *string, string, int)` returns `[]repository.SaturnLastMessage` directly from `GroupB.SaturnLastMessages`.
- `MailService.SaturnRegisteredUsers(context.Context)` returns `[]repository.SaturnRegisteredUser` directly from `GroupB.SaturnRegisteredUsers`.
- No conversion to `model.Message`, legacy `RegisteredUser`, formatted strings, or other result shapes occurs.
- Tests verify exact typed rows and error propagation with `errors.Is`.

### Saturn `lastMessages` input semantics

The service passes the nullable `name` pointer, `trip`, and `count` unchanged. Tests cover a non-nil name, `nil` name, and non-positive count (`0`), and assert the fake repository received the exact values. The repository remains responsible for Saturn-compatible `count <= 0` defaulting and SQL behavior; the service does not add the command's max-30 clamp or a PUBLIC filter. This agrees with the inspected `UserServiceImpl.lastMessages` contract and preserves the sensitive Saturn-shaped read as a separately named API.

### Registered-user semantics

The service preserves the typed `Name,Trip` rows and ordering returned by the repository and propagates repository errors rather than converting failures to an empty successful result. The pre-existing `MailService.RegisteredUsers() string` remains unchanged, including its legacy formatted directory behavior. This matches Saturn `MailServiceImpl.getRegisteredUsers` as a source reference without accidentally changing the existing Zenbot command contract.

### Explicit unavailable behavior

Both new methods fail closed when `GroupB == nil` with the explicit error text `group B repository unavailable`; they do not panic. Existing zero-value/legacy service construction remains safe. The focused service tests cover nil Group B separation, while factory tests cover legacy-only construction behavior.

### Factory injection and compatibility

`NewEngineWithOptions` type-asserts `repository.SqlUtilGroupBRepository` only at the existing database-backed service construction point, then injects the same optional implementation into both `MailService.GroupB` and `UserService.GroupB`. A repository without the optional interface receives a nil Group B field; the existing legacy assertions and service construction remain otherwise unchanged. A repository without `SQLDB` follows the pre-existing no-database path and leaves `Services` nil, without introducing a Group B requirement or panic.

### Existing callers/contracts unchanged

Search and source inspection confirmed existing callers remain on legacy methods:

- `internal/command/users_nicks.go` calls `UserService.RegisteredUsers` and keeps the legacy `Trip,Name` formatter.
- `internal/command/mail_notes.go` calls `MailService.RegisteredUsers()` for the existing not-registered help payload.
- `internal/command/identity_commands.go` calls `UserService.LastMessages` and keeps the legacy public-only `[]model.Message` formatting path.

No command, listener, agent/sql policy, schema, authorization, repository contract, or existing public method was changed by Split A. No new command or wiring to the compatibility methods exists.

## Actual commands and results

All commands below were run in `/Users/ab/workspace/go-projects/zenbot` during this QA.

```text
gofmt -w internal/service/services.go internal/service/group_b_test.go internal/factory/engine_factory.go internal/factory/group_b_test.go
PASS — completed; no task-file formatting changes remained.

gofmt -l internal/service/services.go internal/service/group_b_test.go internal/factory/engine_factory.go internal/factory/group_b_test.go
PASS — empty output.

go test ./internal/service ./internal/factory ./internal/command -count=1
PASS — all three packages green.

go test ./internal/service ./internal/factory -run 'Test(UserServiceSaturn|MailServiceSaturn|LegacyServiceMethods|NewEngineInjectsGroupB|NewEngineLegacyOnly)' -count=1
PASS — service and factory focused tests green.

go test ./internal/command -run 'Test.*(Users|Mail|Message|Last|Command)' -count=1
PASS — command-focused tests green.

go test -race ./internal/service ./internal/factory ./internal/command -count=1
PASS — all three packages green.

go test ./... -count=1
PASS — all packages green.

go test -race ./... -count=1
PASS — all packages green; existing macOS linker warning for internal/agent/sql.test (`malformed LC_DYSYMTAB`) was emitted, exit status 0.

go vet ./...
PASS — empty output, exit status 0.

go build ./...
PASS — exit status 0.
```

## Dirty-tree attribution and preservation checks

The worktree was already extensively staged/modified/untracked before this QA. The unstaged delta after QA was limited to these three authorized paths:

- `internal/service/services.go`
- `internal/factory/engine_factory.go`
- `internal/factory/group_b_test.go`

`internal/service/group_b_test.go` was already staged and had no unstaged delta. All four files are authorized Split A task-owned files; no other application source was attributed to this slice. No command path appeared in the unstaged diff. The task-owned changes were limited to the expected Group B fields, methods, factory assertion/injection, and focused tests. No unrelated source was edited.

Checks:

```text
git diff --check -- internal/service/services.go internal/service/group_b_test.go internal/factory/engine_factory.go internal/factory/group_b_test.go
PASS — empty output.

git diff --name-only
PASS — only internal/factory/engine_factory.go, internal/factory/group_b_test.go, internal/service/services.go (the expected unstaged Split A implementation delta).

worktree_vs_index cmp for internal/factory/engine_factory_test.go
PASS — exit 0; byte-identical to the staged pre-task baseline snapshot.
```

The whole staged tree is not whitespace-clean because of pre-existing unrelated handoff whitespace and the pre-existing blank line at EOF in `internal/factory/engine_factory_test.go`; `git diff --cached --check` reported those findings. They were not altered. The task-file check above is clean.

Protected-document preservation:

- `MIGRATION_PLAN.md`: worktree compares byte-identical to its staged baseline (`cmp` exit 0); current SHA-256 `44df91cd91a9f9c7a74f1118529fbf816ec69798952d7c4a1c82ac8834a39a67`.
- `.hermes/migration-audit.md`: worktree compares byte-identical to its staged baseline (`cmp` exit 0); current SHA-256 `75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18`.
- Saturn service/util source status and diff checks were clean; no Saturn source was modified during QA.

## Limitations and residual boundaries

- This QA validates the service/factory Split A seam, not production authorization or a public caller. The compatibility reads remain intentionally unwired.
- The factory legacy-only test fixture does not implement `SQLDB`, so it verifies the existing no-database construction path. Source inspection additionally verifies that a database-backed repository lacking Group B gets a nil optional assertion result.
- Saturn’s service methods swallow SQL exceptions in their Java implementation; Zenbot’s new service methods correctly propagate repository errors as required by the architecture rather than copying that unsafe error-swallowing behavior.
- No delete service/capability was added or validated. Delete integration remains incomplete.
- This is not acceptance of full row #324 or the overall migration: **full row #324 and the overall Saturn-to-Zenbot migration remain incomplete**.
