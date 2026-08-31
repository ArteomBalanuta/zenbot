# Row #324 Group B — Split A Service Integration Acceptance

## Verdict

**PASS — Split A accepted as a bounded, typed, service-only compatibility slice.**

This acceptance covers only the unwired service integration for Saturn-shaped registered-user and last-message reads. It does not accept delete integration, a public caller, command rewiring, or any broader migration work.

**Delete integration: NOT COMPLETE.**

**Full row #324: NOT COMPLETE.**

**Overall Saturn-to-Zenbot migration: NOT COMPLETE.**

## Exact Split A target files

The application source/test files attributable to Split A are exactly:

- `internal/service/services.go`
- `internal/service/group_b_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/group_b_test.go`

The acceptance artifact is:

- `.hermes/handoffs/sql-util-group-b-service-integration-acceptance.md`

No other application source, schema, repository Group B implementation, command, listener, agent/sql policy, transport, provider, or production-registration file is part of this acceptance.

## Implementation evidence

The implementation handoff is:

- `.hermes/handoffs/sql-util-group-b-service-integration-implementation.md`

It records that Split A added:

- Optional `GroupB repository.SqlUtilGroupBRepository` fields to `UserService` and `MailService`.
- `UserService.SaturnLastMessages(context.Context, *string, string, int) ([]repository.SaturnLastMessage, error)`.
- `MailService.SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error)`.
- Direct delegation preserving nullable names, count values, typed Saturn row shapes, and repository errors.
- Explicit fail-closed `group B repository unavailable` behavior when the optional dependency is nil; no panic.
- Factory injection of the same optional Group B implementation into both service owners when the repository implements the interface.
- Nil-safe legacy-only/no-database construction.

The implementation handoff also records the TDD RED compile failure caused by the absent seam, followed by GREEN focused service/factory tests after the minimal production edits.

## Independent QA evidence

The independent QA handoff is:

- `.hermes/handoffs/sql-util-group-b-service-integration-qa.md`

Its verdict is **PASS — Split A bounded service integration**. The QA inspected the actual worktree, the architecture and implementation handoffs, Saturn reference source, existing Zenbot callers, and dirty-tree attribution. It found no genuine Split A defect and made no source or test fix.

QA specifically verified:

- The two new methods return the repository's typed Saturn-shaped slices directly.
- No conversion occurs to `model.Message`, legacy `RegisteredUser`, formatted strings, or other existing result shapes.
- Nullable `name`, `trip`, and `count` are forwarded unchanged for `SaturnLastMessages`.
- Repository-owned non-positive count defaulting and SQL semantics are not overridden by the service.
- No PUBLIC filter or command max-30 clamp was introduced into the compatibility read.
- Existing formatted `MailService.RegisteredUsers()` behavior remains unchanged.
- Existing callers remain on legacy methods and contracts.
- Optional Group B injection and legacy-only construction remain compatible.
- No command, listener, agent/sql policy, schema, authorization policy, repository contract, or existing public method was changed by Split A.

## Actual passing gates

The following gates were run in `/Users/ab/workspace/go-projects/zenbot` and passed:

```text
go test ./internal/service ./internal/factory ./internal/command -count=1
PASS — all three packages green.

go test -race ./internal/service ./internal/factory ./internal/command -count=1
PASS — all three packages green.

go test ./... -count=1
PASS — all packages green.

go test -race ./... -count=1
PASS — all packages green.

go vet ./...
PASS — exit status 0, empty output.

go build ./...
PASS — exit status 0, empty output.
```

The full race run emitted the existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exited 0 and all packages passed.

The implementation/QA handoffs additionally record passing focused service/factory, command-focused, formatting, and task-scoped whitespace checks. The changed Split A files were individually gofmt'd. Repository-wide formatting/whitespace output contains only pre-existing unrelated/staged findings preserved by scope, including the pre-existing blank line in `internal/factory/engine_factory_test.go`.

## Typed result-shape boundaries

Split A intentionally introduces separately named typed compatibility APIs:

- Registered users: `[]repository.SaturnRegisteredUser`, preserving the repository's `Name,Trip` shape and ordering.
- Last messages: `[]repository.SaturnLastMessage`, preserving `(Name, Message, CreatedOn)`.

These results are not coerced into:

- Zenbot's legacy `RegisteredUser` shape (`Trip,Name`).
- The existing formatted `MailService.RegisteredUsers() string` directory.
- Zenbot's rich `[]model.Message` history.

The existing `UserService.RegisteredUsers`, `UserService.LastMessages`, mail-directory behavior, and their callers remain unchanged. The compatibility methods remain unwired from commands, listeners, agent/sql, mail formatting, public history, and transport.

## Security and visibility boundaries

- Split A adds no delete capability and no authorization bypass.
- The compatibility reads remain service-callable only through the explicitly added internal seam and have no public command caller.
- Saturn-shaped last-message rows intentionally preserve Saturn semantics, including the absence of Zenbot's PUBLIC predicate; therefore they must not be substituted into the existing public-only/moderator history output path.
- No service manufactures authorization evidence, uses `context.Background()` as proof, or exposes the repository's package-private delete capability.
- No broad directory projection or potentially sensitive history result is routed through an existing public formatter without a separate visibility and caller decision.
- Repository errors are propagated; read failures are not converted into successful empty results.

Delete authorization/capability design is explicitly outside Split A and remains unresolved. There is no accepted production delete caller in this slice.

## Optional factory injection behavior

`internal/factory/engine_factory.go` type-asserts `repository.SqlUtilGroupBRepository` at the existing database-backed service construction point and injects the same optional implementation into `MailService.GroupB` and `UserService.GroupB`.

The behavior is intentionally optional:

- A repository implementing Group B receives the dependency in both owners.
- A database-backed repository without Group B receives nil Group B fields while legacy services continue to construct.
- A legacy-only/no-database path remains nil-safe and does not acquire a Group B requirement.
- A nil Group B service method returns the explicit unavailable error rather than panicking.

No existing legacy repository assertion or service construction contract is replaced.

## Protected-document and Saturn preservation

The protected documents were not modified:

- `MIGRATION_PLAN.md` remained byte-identical to its staged baseline; observed SHA-256: `44df91cd91a9f9c7a74f1118529fbf816ec69798952d7c4a1c82ac8834a39a67`.
- `.hermes/migration-audit.md` remained byte-identical to its staged baseline; observed SHA-256: `75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18`.

Saturn was read-only for this task. No Saturn source was modified. Existing unrelated Saturn worktree dirt was preserved and is not attributed to Split A.

The accepted Saturn compatibility semantics are represented by the existing Group B repository seam; Split A does not alter that seam, its schema, or its SQL. It only adds typed, unwired service delegation over the two read operations.

## Changed-file scope verification

The independent QA attributed the task-owned application delta only to the four exact Split A files listed above. Its unstaged-diff check found only the expected implementation/test paths:

```text
internal/factory/engine_factory.go
internal/factory/group_b_test.go
internal/service/services.go
```

`internal/service/group_b_test.go` was already staged and had no unstaged delta at QA time. Pre-existing dirty, staged, and untracked files were preserved and are not attributed to this acceptance. `internal/factory/engine_factory_test.go` was restored/verified byte-identical to its staged pre-task baseline and was not modified by Split A.

No protected document, Saturn source, unrelated application source, command path, schema, or repository Group B source was modified by this acceptance task.

## Explicit exclusions

The following are explicitly excluded and remain unaccepted:

- Delete integration, delete authorization/capability design, and any destructive service or command path.
- Full row #324 beyond this bounded service-only Split A seam.
- Group C and row #325 (`Util`).
- Changes to existing legacy service contracts or callers.
- Command, listener, mail-directory, public-history, agent/sql policy, or broad production wiring.
- Schema or migration changes.
- New providers, transports, routers, remote-room, Whiskey work, or unrelated production registration.
- Conversion of Saturn-shaped rows into public Zenbot result types.
- Any claim that an authorized production service path exists.

## Final status

**PASS — Split A only: typed, service-only, unwired compatibility reads accepted.**

**Delete integration: NOT COMPLETE.**

**Full row #324: NOT COMPLETE.**

**Overall Saturn-to-Zenbot migration: NOT COMPLETE.**
