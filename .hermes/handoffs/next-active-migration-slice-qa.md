# Notes Group C Parity — Independent QA Handoff

## Bounded verdict: PASS

This PASS covers only the Saturn audit row #324 Notes Group C slice:

- `INSERT_INTO_NOTES_TRIP_NOTE_CREATED_ON_VALUES`
- `SELECT_NOTES_BY_TRIP`
- `DELETE_FROM_NOTES_WHERE_TRIP`
- observable `note` / `save` / `notes` behavior described in the architecture handoff.

It does **not** accept full row #324, other Group C work, or the overall migration.

## Independent scope review

The two focused tests assert real source contracts rather than merely compiling:

- real-H2 save persists the exact raw note payload and a positive `created_on` value;
- list is exact-trip scoped, returns no records for another trip, JSON-escapes quote/newline/backslash payloads, and handles a quote-bearing trip selector without widening selection;
- clear deletes only the selected trip and handles a quote-bearing selector without deleting another trip;
- `note` and `save` share behavior; no arguments, no-trip acknowledgement/no-save, outputs, statuses, list envelope, lowercase `purge`/`clear`, and invalid/uppercase silent failures are asserted.

No production source was modified for this slice. The pre-existing dirty worktree includes staged/unstaged production changes outside this task; inspection found no Notes Group C owner edit attributable to this slice. QA changed only the two task-owned tests: it added missing exact-persistence/different-trip/quote-selector coverage and moved the command parity test from port 55437 to dedicated port 55438, avoiding the existing command integration test's H2 port.

## Gates

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

```text
go test ./internal/repository/h2 -run 'Test.*GroupC.*Notes|TestMailAndNotesPersistenceParity' -count=1
PASS  ok zenbot/internal/repository/h2

go test ./internal/command -run 'Test.*Note|Test.*Notes|TestMailAndNotesAliasesDispatchThroughListener' -count=1
PASS  ok zenbot/internal/command

go test ./internal/command ./internal/service ./internal/repository/h2 -count=1
PASS  all three packages

go test -race ./internal/command ./internal/service ./internal/repository/h2 -count=1
PASS  all three packages

go test ./... -count=1
PASS  all packages

go test -race ./... -count=1
PASS  all packages

go vet ./...
PASS

go build ./...
PASS

gofmt -l internal/repository/h2/sql_util_row324_group_c_notes_test.go internal/command/note_notes_parity_test.go
PASS  no output

git diff --check
PASS
```

`go test -race ./...` emitted one non-failing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`); the command exited 0 and every package passed.

## Protected documents

Verified unchanged against implementation-handoff hashes:

- `MIGRATION_PLAN.md`: `bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0`
- `.hermes/migration-audit.md`: `75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18`

## Files attributable to this QA slice

- `internal/repository/h2/sql_util_row324_group_c_notes_test.go`
- `internal/command/note_notes_parity_test.go`
- `.hermes/handoffs/next-active-migration-slice-implementation.md` (pre-existing implementation handoff)
- `.hermes/handoffs/next-active-migration-slice-qa.md`
