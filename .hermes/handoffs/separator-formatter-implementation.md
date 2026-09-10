# SeparatorFormatter Implementation Handoff

## Outcome

Implemented audit row **#323** (`SeparatorFormatter`) in the approved `internal/model` package only. This is a bounded implementation slice; it does **not** claim overall migration completion.

## Exact task-owned files

Created:

- `internal/model/separator_formatter.go`
- `internal/model/separator_formatter_test.go`
- `.hermes/handoffs/separator-formatter-implementation.md` (this handoff)

No callers were migrated. No existing model source was intentionally edited.

## Cleanup

The previously reported erroneous artifact `internal/util/separator_formatter_test.go` was checked before implementation and was **absent**, so no deletion was performed. No other `internal/util` file was removed or edited. The pre-existing untracked `internal/util/` directory was preserved.

Pre-existing model worktree state was preserved:

- `internal/model/chat_message.go` — already modified (`M`)
- `internal/model/records.go` — already added (`A`)
- `internal/model/status.go` — already added (`A`)

The task-owned files are the only new model files: `separator_formatter.go` and `separator_formatter_test.go`.

## API and behavior

`internal/model/separator_formatter.go` exposes exactly:

```go
func AddSeparator(values []*string, separator rune) []*string
func GetFirst(values []*string) (string, bool)
func GetLast(values []*string) (string, bool)
```

The implementation follows the confirmed Saturn fixture and semantics:

- `AddSeparator` mutates and returns the original slice.
- Nil elements are preserved and skipped unless the Saturn identity stop condition reaches a nil element.
- Lists of length 0 or 1 return unchanged.
- `GetFirst` returns the first non-nil value.
- `GetLast` scans backward only while the index is greater than zero, therefore excluding index 0.
- The replacement search is value-based, matching Java `List.indexOf` for ordinary strings.
- No `strings.Join` is used.

### Go adaptation documented by tests

Java reference identity (`e == last`) cannot be represented by returned `string` values alone. The Go boundary therefore uses `[]*string`: pointer identity preserves the stop-condition distinction for duplicate string values, while the public lookup methods return `(string, bool)` as required. The focused regression test `TestSeparatorFormatterGoReferenceIdentityAdaptation` documents this deliberate adaptation and the retained value-based first-match replacement behavior. This is not presented as unqualified exact parity for every possible Java object aliasing case.

## RED evidence

After creating the new test file and before creating the implementation, the focused command was run:

```text
$ go test ./internal/model -run TestSeparatorFormatter -count=1
# zenbot/internal/model [zenbot/internal/model.test]
internal/model/separator_formatter_test.go:11:15: undefined: GetFirst
internal/model/separator_formatter_test.go:15:14: undefined: GetLast
internal/model/separator_formatter_test.go:20:12: undefined: AddSeparator
internal/model/separator_formatter_test.go:31:18: undefined: GetLast
internal/model/separator_formatter_test.go:37:14: undefined: GetLast
internal/model/separator_formatter_test.go:46:13: undefined: AddSeparator
internal/model/separator_formatter_test.go:61:12: undefined: AddSeparator
internal/model/separator_formatter_test.go:72:12: undefined: AddSeparator
FAIL	zenbot/internal/model [build failed]
FAIL
```

This failed because the production API did not yet exist, as intended.

## GREEN and verification evidence

All requested verification commands passed in the final run:

```text
$ go test ./internal/model -run TestSeparatorFormatter -count=1
ok  	zenbot/internal/model	0.225s
$ go test -race ./internal/model -run TestSeparatorFormatter -count=1
ok  	zenbot/internal/model	1.348s
$ gofmt -d internal/model/separator_formatter.go internal/model/separator_formatter_test.go
gofmt clean
$ go vet ./...
$ go test -count=1 ./...
... all packages passed, including zenbot/internal/model and zenbot/internal/util ...
$ go test -race ./...
... all packages passed; macOS linker emitted one non-fatal LC_DYSYMTAB warning for internal/agent/sql.test ...
$ go build ./...
$ git diff --check
all requested verification commands passed
```

The full normal and race suites both completed successfully. The race run's warning was:

```text
ld: warning: '.../go-link-3075251313/000081.o' has malformed LC_DYSYMTAB, expected 98 undefined symbols to start at index 1626, found 95 undefined symbols starting at index 1626
```

It did not change the successful exit status.

## Source references

Confirmed architecture and audit linkage:

- `.hermes/handoffs/separator-formatter-architecture.md`
- `.hermes/migration-audit.md`, row `#323` (source line 346)

Read-only Saturn references:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SeparatorFormatter.java`
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/SeparatorFormatterTest.java`

The Saturn fixture was `[null, "test", "test2", "test3", "test4", null]`, with expected mutation `[null, "test,", "test2,", "test3,", "test4", null]`.

## Limitations and exclusions

- No Saturn files were modified.
- No caller migration was attempted.
- No `internal/util` implementation was added.
- No `strings.Join`, normalization, escaping, trimming, sorting, or error behavior was introduced.
- Empty separators, Unicode separators, and broader duplicate/aliasing behavior are not claimed as independently Saturn-proven requirements beyond the documented Go-rune/pointer adaptation.
- A nil Go slice cannot reproduce Java's null-list exception with the required no-error Go signatures; no invented error contract was added.
- Existing dirty/untracked repository state outside the task-owned files was preserved.
