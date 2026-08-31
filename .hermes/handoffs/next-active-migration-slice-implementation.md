# Notes Group C Parity — Implementation Handoff

## Scope

Implemented only the bounded Notes subset of Saturn audit row #324 Group C identified by the architecture handoff:

- `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`
- `SELECT_NOTES_BY_TRIP`
- `DELETE_FROM_NOTES_WHERE_TRIP`
- Saturn `note` / `save` / `notes` observable command behavior.

No production code changed. Existing Zenbot `NoteService` and command implementations satisfied the new focused parity checks.

## Task-owned files

- `internal/repository/h2/sql_util_row324_group_c_notes_test.go` (new)
- `internal/command/note_notes_parity_test.go` (new)
- This handoff.

## Behavior exercised

The real-H2 test covers save/list/clear behavior, exact trip isolation, quote/backslash/newline escaping, persisted positive `created_on` milliseconds, cross-trip deletion isolation, and a quote-bearing selector.

The command test covers `note` and `save` aliases, no-argument failure text, no-trip success-without-save behavior, `notes` no-trip failure text, list response envelope, exact lowercase `purge` and `clear`, and silent/no-delete behavior for invalid or uppercase arguments.

During RED/GREEN correction, test code was aligned with existing Zenbot contracts: `NoteService` methods are `Save`, `List`, and `Clear`; `List` returns `[]string`; H2 `created_on` scans as positive `int64` milliseconds. These were test-only corrections, not production adaptations.

## Verified focused gates

Run independently from `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./internal/repository/h2 -run TestSqlUtilRow324GroupCNotesParity -count=1
PASS: ok zenbot/internal/repository/h2

go test ./internal/command -run 'TestNoteAndSaveParityAliasesAndTripBoundary|TestNotesParityListPurgeClearAndInvalidArguments' -count=1
PASS: ok zenbot/internal/command
```

## Scope and preservation boundaries

No schema, mail, moderation, listener lifecycle, session/history, agent, transport, DBZ, prefix, remote-room, Whiskey, or row #325 work is included. Full row #324 and the overall migration remain NOT COMPLETE.

Protected documents were preserved:

- `MIGRATION_PLAN.md`: `bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0`
- `.hermes/migration-audit.md`: `75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18`

## QA handoff

Independent QA must inspect the two new tests, ensure no production source changed for this scope, rerun focused tests plus `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, formatting checks on task-owned Go files, and `git diff --check`; then record an explicit bounded acceptance or failure.