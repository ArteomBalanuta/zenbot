# Utility Contracts Implementation — Saturn Rows #320–#322

**Status:** Implemented provider-neutral utility owners and focused parity tests; caller migration and overall migration remain **NOT COMPLETE**.
**Target:** Zenbot (`module zenbot`) at `/Users/ab/workspace/go-projects/zenbot`.
**Source reference:** Saturn `develop`, read-only, at `/Users/ab/workspace/projects/saturn`.
**Architecture:** `.hermes/handoffs/agent-utility-contracts-architecture.md`.
**Diagnostic:** `.hermes/handoffs/next-migration-slice-diagnostic-2.md`.

## Exact files attributable to this implementation

- `internal/util/date.go`
- `internal/util/date_test.go`
- `internal/util/identity.go`
- `internal/util/identity_test.go`
- `internal/util/json_payloads.go`
- `internal/util/json_payloads_test.go`
- `.hermes/handoffs/agent-utility-contracts-implementation.md`

No existing caller, registration, transport, listener, command, core helper, provider, persistence, DBZ, moderation, remote-room, Whiskey, or Saturn file was modified by this slice.

## RED/GREEN evidence

Strict TDD was used:

1. Focused tests were added before production utility files.
2. RED command: `go test -count=1 ./internal/util`
3. RED result: expected compilation failure with undefined `TimestampNowMillis`, `UTCNow`, `Timestamp8601ToSeconds`, `Difference`, `FormatZoneUTC`, `ToZoneDateTimeUTC`, `FormatTime`, `TimestampUnit`, `FormatRFC1123`, `NormalizeNickTarget`, `ErrBlankNickTarget`, `CanonicalNick`, `SameNick`, `Command`, `CommandWithValue`, and `CommandWithValues`.
4. GREEN implementation added the three provider-neutral owners, followed by `gofmt -w internal/util/*.go`.
5. Focused normal result: `go test -count=1 ./internal/util` — **PASS**.
6. Focused race result: `go test -race ./internal/util` — **PASS**.

## Implemented Saturn contracts and explicit Go adaptations

### DateUtil / `internal/util/date.go`

- `TimestampNowMillis` returns epoch milliseconds.
- `UTCNow` returns RFC3339Nano UTC text.
- `Timestamp8601ToSeconds` uses pointers to represent Java nullability: nil timestamp returns `(nil, nil)`, parse failure returns `(nil, nil)`, nil zone defaults to UTC, invalid zones return errors.
- The parser accepts one-digit date/time fields required by Saturn's `SimpleDateFormat` vector (`2023-08-27T14:5`) and uses `time.Date` overflow normalization to retain documented lenient-date behavior.
- `Difference` renders ordered signed components exactly as `"%d days, %d hours, %d minutes, %d seconds"`, including negative intervals.
- Epoch conversion APIs interpret input as milliseconds and use UTC.
- `FormatTime` uses the Saturn pattern `yyyy-MM-dd'T'HH:mm:ss z` adapted to Go's layout.
- `FormatRFC1123` has an explicit `TimestampUnit` enum, seconds/milliseconds conversion, invalid-zone errors, unsupported-unit error text, non-padded day output, and maps UTC's Go abbreviation to Saturn's `GMT` output.

### IdentityUtil / `internal/util/identity.go`

- `NormalizeNickTarget` uses `*string` for explicit Java null handling.
- It trims, removes exactly one leading ASCII `@`, trims again, and rejects nil, blank, and bare-marker inputs with the exact sentinel message `Nick target cannot be blank`.
- `CanonicalNick` uses locale-independent Go `strings.ToLower`.
- `SameNick` is nil-safe and returns false for invalid non-nil inputs.

### JsonPayloads / `internal/util/json_payloads.go`

- Three explicit Go functions replace Java overloads: `Command`, `CommandWithValue`, and `CommandWithValues`.
- Exact Saturn spacing is preserved: no space before `}` for zero/one-value forms, and one space before `}` for the two-value form.
- A provider-neutral escaper handles quotes, backslashes, `\\b`, `\\t`, `\\n`, `\\f`, `\\r`, and remaining U+0000–U+001F controls as four-hex uppercase Unicode escapes, matching the inspected Apache Commons Text behavior.
- Keys, values, and command strings are escaped identically; ordinary Unicode and slash remain unchanged.
- Tests assert exact bytes and JSON parseability, including empty strings and trailing backslashes.

## Saturn references inspected

- `src/main/java/org/saturn/app/util/DateUtil.java`
- `src/test/java/org/saturn/app/util/DateUtilTest.java`
- `src/main/java/org/saturn/app/util/IdentityUtil.java`
- `src/test/java/org/saturn/app/util/IdentityUtilTest.java`
- `src/main/java/org/saturn/app/util/JsonPayloads.java`
- `src/test/java/org/saturn/app/util/JsonPayloadsTest.java`

Saturn remained read-only. No runtime Saturn dependency or JVM oracle was added.

## Verification command results

- `gofmt -w internal/util/*.go` — **PASS**.
- `go test -count=1 ./internal/util` — **PASS**.
- `go test -race ./internal/util` — **PASS**.
- `go test -count=1 ./...` — **PASS**.
- `go test -race ./...` — **PASS**; emitted an existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exited successfully.
- `go vet ./...` — **PASS**.
- `go build ./...` — **PASS**.
- `git diff --check` — **PASS**.

## Scope verification and limitations

The checkout had extensive unrelated pre-existing staged, modified, and untracked migration work before this slice. The implementation changes attributable here are limited to the six `internal/util` files and this handoff. Existing application files and Saturn files were not touched. `git status --short -- internal/util .hermes/handoffs/agent-utility-contracts-implementation.md` reports only the new `internal/util/` directory because this handoff is itself untracked until the caller stages it.

This slice does **not** migrate callers, replace `internal/core/engine_impl.go`'s private `escapeJSON`, alter inline command/listener identity logic, update the frozen audit, or claim completion of rows outside #320–#322 or of the overall Saturn-to-Zenbot migration.
