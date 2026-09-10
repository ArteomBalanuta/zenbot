# SeparatorFormatter QA Handoff

## Verdict

**PASS for the bounded row #323 slice.** The task-owned Go implementation and focused tests match the confirmed Saturn source semantics within the documented `[]*string` adaptation. No production defect was found, so no task-owned source/test fix was made during QA. This verdict does **not** claim overall migration completion.

## Scope and source evidence

- Audit row: `.hermes/migration-audit.md`, row **#323** (source line 346), `org.saturn.app.util.SeparatorFormatter`.
- Saturn production source inspected read-only: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SeparatorFormatter.java`.
- Saturn focused test inspected read-only: `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/SeparatorFormatterTest.java`.
- Go task-owned implementation: `internal/model/separator_formatter.go`.
- Go task-owned tests: `internal/model/separator_formatter_test.go`.
- Architecture and implementation evidence: `.hermes/handoffs/separator-formatter-architecture.md` and `.hermes/handoffs/separator-formatter-implementation.md`.

The Saturn fixture is exactly `[null, "test", "test2", "test3", "test4", null]`; the expected mutation is `[null, "test,", "test2,", "test3,", "test4", null]`. The Saturn source confirms `getLast` scans backward while `index > 0`, never inspecting index 0, and `addSeparator` stops on Java reference identity (`e == last`) while replacing `list.indexOf(e)`.

## Semantic QA findings

All requested behaviors were inspected against the source and implementation/tests:

- **In-place mutation and return identity:** `AddSeparator` changes element slots in the supplied slice and returns that same slice header/backing sequence; the focused test checks the original element address.
- **Size 0/1:** early return leaves values unchanged and preserves identity.
- **Nil elements:** nils are retained and skipped during formatting; the stop condition is represented by the last non-nil pointer, matching the Java loop for the supported representation.
- **GetFirst:** returns the first non-nil value and `(string, bool)` distinguishes absent from a present empty string.
- **GetLast:** backward scan excludes index 0; empty, one-element, and index-0-only cases return `("", false)`.
- **Separator behavior:** the implementation uses `string(separator)` from the Go `rune` boundary, with focused comma and pipe cases. No invented error/null-separator behavior exists.
- **Java reference identity adaptation:** `[]*string` preserves pointer identity for the `e == last` stop condition while replacement remains value-based, matching Java `List.indexOf` for ordinary strings. `TestSeparatorFormatterGoReferenceIdentityAdaptation` covers equal-valued distinct pointers.
- **Java-null list limitation:** the no-error Go signature cannot reproduce Java's `NullPointerException` for a null list; this is documented rather than silently claimed as parity. Nil list elements remain representable.
- **Join/pure-formatter check:** no `strings.Join` or equivalent pure-join implementation is present. The API mutates/returns the slice and preserves null slots.
- **Aliasing check:** no input slice copy or replacement slice is created. Pointer-identity stop and value-based first-match replacement are explicit and covered.

No genuine bounded defect was found. No source or test fix was warranted.

## Verification commands and results

Focused checks:

```text
go test ./internal/model -run TestSeparatorFormatter -count=1   PASS
 go test -race ./internal/model -run TestSeparatorFormatter -count=1   PASS
 gofmt -d internal/model/separator_formatter.go internal/model/separator_formatter_test.go   clean
```

Repository checks, run after focused QA:

```text
go test -count=1 ./...   PASS
go test -race ./...      PASS
 go vet ./...            PASS
 go build ./...          PASS
 gofmt check             PASS
git diff --check         PASS
```

The race suite emitted one non-fatal macOS linker warning for `internal/agent/sql.test` about malformed `LC_DYSYMTAB`; the command exited successfully and all packages passed.

## Changed-file attribution and preservation

QA did not modify the implementation or focused test. The task-owned files remain:

- `internal/model/separator_formatter.go` — pre-existing task implementation, inspected only.
- `internal/model/separator_formatter_test.go` — pre-existing task tests, inspected only.
- `.hermes/handoffs/separator-formatter-qa.md` — created by this QA pass.

Existing dirty model work was inspected and preserved:

- `internal/model/chat_message.go` remains modified.
- `internal/model/records.go` remains added.
- `internal/model/status.go` remains added.

The pre-existing `internal/util/` work and unrelated dirty/untracked files were not edited. No callers, wiring, `internal/util`, Saturn files, SQL paths, or existing model files were changed by this QA pass. The Saturn production/test source hashes observed during QA were:

```text
609768dd5a8510f9025d7136a1ad6d7815f71808  SeparatorFormatter.java
62bbd7bff7bfe93efceeb42994c6666bbe92093a  SeparatorFormatterTest.java
```

## Limitations and exclusions

- QA did not modify or execute the Saturn repository; parity evidence is source/test inspection plus Go focused tests.
- The Go API intentionally uses `[]*string`, `(string, bool)`, and `rune`; Java object identity beyond pointer-shaped string adaptation is not claimed.
- A nil Go slice does not raise Java's null-list exception under the required no-error signature.
- Empty, Unicode, and broader aliasing cases are not claimed as Saturn-focused-test requirements; the implementation's rune conversion is directly inspected and ASCII separator behavior is tested.
- No caller migration, SQL/persistence work, or overall migration assessment was performed.
