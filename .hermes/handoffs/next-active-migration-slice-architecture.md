# Next Active Migration Slice — Notes Group C Parity

## Decision and bounded scope

This is an implementation-ready technical specification for one pending, source-owned subset of Saturn audit row #324 (`SqlUtil`): Notes persistence and the `note`/`save` and `notes` command paths.

The selected Saturn SQL constants are:

1. `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`
2. `SELECT_NOTES_BY_TRIP`
3. `DELETE_FROM_NOTES_WHERE_TRIP`

This specification does not close full row #324, any other Group C constant, row #325 (`Util`), or the overall migration.

Complexity/risk: Standard. Route implementation to `@developer`. Escalate to `@senior-developer` if focused parity tests reveal that preserving Saturn error-silencing requires an incompatible change to the existing engine-wide service-error contract.

## Source authority and traced behavior

### Persistence owner

Saturn source: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/NoteServiceImpl.java`.

- `save(trip, note)` prepares `SqlUtil.INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`, binds trip, note, and `DateUtil.getTimestampNow()`, executes the insert, and catches/logs `SQLException` without returning it (`NoteServiceImpl.java:37-52`).
- `getNotesByTrip(trip)` prepares `SqlUtil.SELECT_NOTES_BY_TRIP`, binds trip exactly, reads each `note` value, applies `StringEscapeUtils.escapeJson`, and returns a `List<String>`. SQL failure is logged and converted to an empty/partial list (`NoteServiceImpl.java:54-73`).
- `clearNotesByTrip(trip)` prepares `SqlUtil.DELETE_FROM_NOTES_WHERE_TRIP`, binds trip exactly, executes, and catches/logs SQL failure (`NoteServiceImpl.java:75-88`).
- `executeListNotes(author, trip)` emits exactly `"'s notes: \\n ```Text \\n" + notes.toString() + "\\n```" to the author (`NoteServiceImpl.java:31-35`).
- `executeNotesPurge(author, trip)` clears then emits exactly `"'s notes has been deleted"` (`NoteServiceImpl.java:26-29`).

Saturn command callers:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/NoteUserCommandImpl.java` aliases `note`, `save`, requires REGULAR role, rejects no arguments with `Example: <prefix>note Jedi am I?!`, conditionally saves only when trip is non-null, then emits `note successfully saved!` and returns success (`lines 16-42`). Its argument join uses `Util.listToString`, which appends a trailing space (`Util.java:145-149`).
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/NotesUserCommandImpl.java` aliases only `notes`, requires a non-null trip (otherwise emits `\\n Set your trip first. Example: <prefix>notes` and fails), lists with zero arguments, purges only when its first argument exactly equals lowercase `purge` or `clear`, and fails silently for all other arguments (`lines 15-58`).
- Focused Saturn tests are `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/command/impl/user/NoteUserCommandImplTest.java` (no-argument failure) and `NotesUserCommandImplTest.java` (no-trip failure and exact queued response).

## Existing Zenbot owner trace

- `internal/service/services.go:188-217` owns `NoteService.Save`, `List`, and `Clear`; it already uses parameterized H2 SQL against the three selected source operations.
- `internal/command/mail_notes.go:52-109` owns `noteCommand` and `notesCommand`; it already uses the source response text, no-trip response text, exact lowercase purge/clear checks, and silent failure for other `notes` arguments.
- `internal/factory/engine_factory.go:66` attaches `&service.NoteService{DB: db}` to database-backed engines.
- `internal/repository/h2/schema-h2.sql:21-25` defines the `notes` table and `schema-h2.sql:71` defines `idx_notes_trip_created_on`; both source-of-truth schema copies already exist.
- `internal/repository/h2/mail_notes_test.go` is the existing real-H2 baseline test but is not targeted to all three Group C constants or command output contracts.

## Required parity work

Start test-first. This slice should not introduce a `SqlUtil` catalog, new generic repository interface, schema change, service injection, or second notes abstraction.

### New focused tests

Create `internal/repository/h2/sql_util_row324_group_c_notes_test.go` using `openTestDB(t)` and `service.NoteService{DB: d.DB}`. It must prove:

1. Save persists the trip, exact note payload, and non-zero creation timestamp through real H2.
2. List selects only the exact trip, returns no records for a different trip, and produces JSON-escaped note text for quote/newline/backslash content.
3. Clear deletes only the selected trip; another trip's notes remain.
4. The selected behavior is parameterized: quote-bearing trip/note values cannot alter selection or deletion scope.
5. The source query has no `ORDER BY`; do not claim an ordering guarantee beyond the existing Zenbot behavior. If the test establishes the current `ORDER BY id` contract, record it as Zenbot compatibility evidence rather than claiming literal Saturn SQL ordering.

Create `internal/command/note_notes_parity_test.go`. Use the existing command/engine test conventions and assert:

1. `note` and `save` route to the same concrete behavior; no arguments returns FAILED and emits `Example: <prefix>note Jedi am I?!`.
2. `note` with a trip saves the arguments with Zenbot's existing string join semantics and emits `note successfully saved!`.
3. `note` without a trip does not save yet still emits `note successfully saved!` and returns SUCCESSFUL, matching Saturn's conditional save and unconditional acknowledgement.
4. `notes` without a trip fails and emits the exact source-compatible newline/prefix response.
5. `notes` with no arguments emits the exact formatted list envelope; escaped note payload remains represented as source-style list content.
6. Exact lowercase `purge` and `clear` delete notes and emit the exact purge acknowledgement. `PURGE` and any other argument fail without output or deletion.
7. Exercise listener dispatch only through the existing `internal/command/dispatch_integration_test.go` machinery if it can cover aliases without modifying listeners. Do not add listener production code.

### Production-code decision rule

Do not alter `internal/service/services.go` or `internal/command/mail_notes.go` merely to make code look more Java-like. The current command behavior is already aligned on the observable source paths above.

If tests expose a real observable mismatch, make the narrowest change in one of these existing owners only:

- `internal/service/services.go` for parameter binding, selection/deletion scope, escaping, or save/list/clear error conversion;
- `internal/command/mail_notes.go` for command alias, trip-null, argument, status, or exact output behavior.

A source change that converts Zenbot service errors to Saturn's logged-and-silent SQL failure behavior is high-risk because `noteCommand` and `notesCommand` currently propagate service errors. Do not make that conversion without a test that establishes the exact expected observable command behavior under a controlled H2 failure and an explicit implementation handoff explaining the target-wide contract impact.

## Explicit exclusions

- Mail constants/callers, mail delivery, and Group B reads.
- Moderation/ban constants and commands.
- `SELECT_LOUNGE_TRIPS` and all listener lifecycle work.
- History/session constants and unsafe unused SQL fragments.
- Row #325 `Util`, schema redesign, H2 startup, transport, agent, remote-room, Whiskey, prefix, DBZ, and any protected document.
- Changes to Saturn or unrelated target worktree files.

## Implementation order

1. Capture baseline focused tests for the existing NoteService and command paths.
2. Add the two focused test files above, initially without production edits.
3. Run the focused tests against real H2; inspect any mismatch against the cited Saturn methods before editing code.
4. If necessary, make only the minimal source-owner change described by the production-code decision rule and rerun focused tests.
5. Produce an implementation handoff with source constants, exact changed paths, test output, known source/target adaptations, and protected-document hashes.

## QA and acceptance gates

The independent tester must run:

```text
go test ./internal/repository/h2 -run 'Test.*GroupC.*Notes|TestMailAndNotesPersistenceParity' -count=1
go test ./internal/command -run 'Test.*Note|Test.*Notes|TestMailAndNotesAliasesDispatchThroughListener' -count=1
go test ./internal/command ./internal/service ./internal/repository/h2 -count=1
go test -race ./internal/command ./internal/service ./internal/repository/h2 -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
gofmt -l <task-owned Go files>
git diff --check
```

QA must verify that `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` remain unchanged, and must attribute only the expected task files.

## Gate-ready checklist

- [ ] Direct Saturn behavior and tests cited above are preserved in the implementation handoff.
- [ ] All three Note Group C constants have real-H2 behavioral evidence.
- [ ] `note`/`save` and `notes` exact output/status/trip/argument boundaries have focused tests.
- [ ] No broad Group C abstraction, schema change, or excluded subsystem was added.
- [ ] Focused, full, race, vet, build, formatting, and diff-check gates pass.
- [ ] Independent QA confirms protected documents and unrelated worktree state are preserved.

Expected task-owned paths:

- `internal/repository/h2/sql_util_row324_group_c_notes_test.go` (new)
- `internal/command/note_notes_parity_test.go` (new)
- `internal/service/services.go` (only if a focused mismatch proves a necessary owner-level change)
- `internal/command/mail_notes.go` (only if a focused mismatch proves a necessary owner-level change)
- `.hermes/handoffs/next-active-migration-slice-implementation.md` (future developer artifact)
- `.hermes/handoffs/next-active-migration-slice-qa.md` (future tester artifact)
