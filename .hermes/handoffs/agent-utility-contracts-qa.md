# Utility Contracts QA — Saturn Rows #320–#322

**Verdict:** PASS for the bounded `internal/util` utility-contract slice. Overall Saturn-to-Zenbot migration remains **NOT COMPLETE**.

**Target:** `/Users/ab/workspace/go-projects/zenbot`  
**Saturn source:** `/Users/ab/workspace/projects/saturn` (read-only)  
**Implementation handoff:** `.hermes/handoffs/agent-utility-contracts-implementation.md`  
**Architecture handoff:** `.hermes/handoffs/agent-utility-contracts-architecture.md`  
**Rows:** #320 `DateUtil`, #321 `IdentityUtil`, #322 `JsonPayloads`

## Findings and fixes

Independent review found three bounded parity defects in the implementation, each reproduced by a regression test before the fix:

1. **Date parser was too strict.** The regex required four-digit date years, one-or-more fixed-width fields, and end-of-input. Saturn uses `SimpleDateFormat.parse(String)`, which accepts one-digit month/day/hour/minute fields, lenient overflow, and trailing text. The adapter now accepts the tested one-digit fields, preserves `time.Date` overflow normalization, and matches the observed prefix-parse behavior.
2. **Negative fractional durations differed.** Go duration division truncates toward zero, while Java `Duration.getSeconds()` floors a negative fractional second and the preceding component extraction truncates toward zero. `Difference` now floors only the total negative fractional seconds before ordered component extraction. Regression: `-1500ms` => `0 days, 0 hours, 0 minutes, -2 seconds`.
3. **Unsupported RFC unit error ordering differed.** Saturn switches on `TimeUnit` before attempting `ZoneId.of`. The Go implementation now rejects an unsupported unit with `Timestamp should be in seconds or milliseconds.` even when the zone is invalid.

No other genuine defects were found. `IdentityUtil` and `JsonPayloads` matched the reviewed contracts and tests.

## Source-grounded evidence

Reviewed exact Saturn sources/tests:

- `src/main/java/org/saturn/app/util/DateUtil.java` and `src/test/java/org/saturn/app/util/DateUtilTest.java`
- `src/main/java/org/saturn/app/util/IdentityUtil.java` and `src/test/java/org/saturn/app/util/IdentityUtilTest.java`
- `src/main/java/org/saturn/app/util/JsonPayloads.java` and `src/test/java/org/saturn/app/util/JsonPayloadsTest.java`

Observed Saturn behavior verified directly with JDK `jshell`: `2023-8-7T4:5` parses; `2023-02-30T14:05` and `2023-13-01T14:05` normalize leniently; `2023-08-27T14:05suffix` parses its valid prefix; and negative fractional `Duration` retains a floored `getSeconds()` value. Saturn’s RFC source performs the unit switch before zone resolution. The exact Unix epoch golden remains `Thu, 1 Jan 1970 00:00:00 GMT`.

Target owners and tests:

- `internal/util/date.go`, `internal/util/date_test.go`
- `internal/util/identity.go`, `internal/util/identity_test.go`
- `internal/util/json_payloads.go`, `internal/util/json_payloads_test.go`

Coverage inspected includes DateUtil null/invalid parsing, UTC default and explicit zones, lenient fields/overflow, epoch seconds vs milliseconds, UTC formatting, signed duration components, RFC1123 day/unit/zone/error behavior; IdentityUtil trim, exactly one leading ASCII `@`, blank/null errors, Unicode lowercase, root-style canonicalization, and nil-safe comparison; JsonPayloads exact overload spacing, command/key/value escaping, quote/backslash/newline/CR/tab/backspace/form-feed/control handling, Unicode/slash preservation, empty strings, trailing backslashes, and JSON parseability.

## Verification commands and results

All commands ran in `/Users/ab/workspace/go-projects/zenbot` after the fixes:

- `gofmt -w internal/util/*.go` — **PASS**
- `go test -count=1 ./internal/util` — **PASS**
- `go test -race ./internal/util` — **PASS**
- `go test -count=1 ./...` — **PASS**
- `go test -race ./...` — **PASS**; emitted the pre-existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exited 0
- `go vet ./...` — **PASS**
- `go build ./...` — **PASS**
- `git diff --check` — **PASS**

## Attribution, preservation, and exclusions

Task-attributable implementation/test changes are limited to the six files under `internal/util`; this QA handoff is the additional artifact. The implementation handoff and architecture/diagnostic handoffs were already present as untracked task artifacts. Existing unrelated staged/modified/untracked migration work was preserved. No caller migration or changes were made under `internal/core`, `internal/command`, `internal/listener`, `cmd`, transport/router/provider/tool, H2/persistence/DBZ/moderation/remote-room/Whiskey, or other excluded areas.

Saturn was not modified. Its status/diff-name snapshot before QA showed only pre-existing changes in `.saturn_*`/`.target3_*`, weather service/test files, and unrelated untracked diagnostic/spec/QA files; no Saturn utility source/test file was changed by this task. The Saturn checkout is not clean because of those pre-existing changes.

**Limitations:** No Saturn source/build files or runtime dependency were changed, and no JVM oracle was added to the Go test suite. Direct JDK probes were used for the specific parser and duration semantics noted above. This QA does not certify caller migration, frozen audit updates, rows outside #320–#322, or overall migration completion.
