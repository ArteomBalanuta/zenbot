# Utility Contract Architecture — Saturn Rows #320–#322

**Status:** Architecture handoff only; no application code or Saturn source was modified.
**Scope:** `DateUtil`, `IdentityUtil`, and `JsonPayloads`.
**Target:** Zenbot (`module zenbot`, Go 1.24) under `/Users/ab/workspace/go-projects/zenbot`.
**Source:** Saturn `develop`, read-only, under `/Users/ab/workspace/projects/saturn`.

## 1. Evidence and decision

The frozen audit (`.hermes/migration-audit.md`, rows 320–322) marks all three units **needs implementation**. The diagnostic (`.hermes/handoffs/next-migration-slice-diagnostic-2.md`) selects this as a pure, provider-neutral slice. The authoritative migration plan remains **NOT COMPLETE**.

| Audit row | Saturn unit | Proposed Zenbot owner |
|---:|---|---|
| #320 | `DateUtil` | `internal/util/date.go` |
| #321 | `IdentityUtil` | `internal/util/identity.go` |
| #322 | `JsonPayloads` | `internal/util/json_payloads.go` |

**[OBSERVED]** Saturn source and direct tests inspected:

- `src/main/java/org/saturn/app/util/DateUtil.java`
- `src/test/java/org/saturn/app/util/DateUtilTest.java`
- `src/main/java/org/saturn/app/util/IdentityUtil.java`
- `src/test/java/org/saturn/app/util/IdentityUtilTest.java`
- `src/main/java/org/saturn/app/util/JsonPayloads.java`
- `src/test/java/org/saturn/app/util/JsonPayloadsTest.java`
- `pom.xml` (Java 23; `commons-lang3` is declared, and the utility imports Apache Commons Text `StringEscapeUtils`; the exact `commons-text` transitive version must be pinned/verified by the source build if an oracle is run).

**[OBSERVED]** Target conventions and partial helpers inspected:

- `go.mod`: module `zenbot`, Go `1.24.0`; standard library is sufficient.
- `internal/model/*.go`: exported value types and table-driven tests; `internal/model/records.go` already contains a different `IdentityKey` used for trip/hash/nick precedence.
- `internal/agent/api/identity.go`: a separate source-qualified `UserIdentity`; do not conflate it with nick-target normalization.
- `internal/core/engine_impl.go:564-575`: private `escapeJSON`, used only by existing chat output; it uses `encoding/json` then restores selected whitespace escapes and must remain untouched in this slice.
- `internal/command/handlers.go:90-103`: inline one-marker stripping and `strings.EqualFold` in `info`; `internal/listener/user_joined_listener.go:41-47` uses inline case-insensitive trip matching.
- Existing focused tests use ordinary Go `testing`, table-style cases, and exact string assertions.

**[RECOMMENDED]** Create a new provider-neutral package, not `internal/model` or `internal/core`:

```text
internal/util/date.go
internal/util/identity.go
internal/util/json_payloads.go
internal/util/date_test.go
internal/util/identity_test.go
internal/util/json_payloads_test.go
```

No callers, registrations, transport, listeners, commands, or engine output should change in this slice. Caller migration is a later coordinated pass.

## 2. Package/file map and exact Go API

Package declaration: `package util`; import path `zenbot/internal/util`.

### Date owner — `internal/util/date.go`

Use `time`, `time/tzdata` only if the application requires embedded zone data, and `errors`/`fmt` as needed. Suggested public API, with Java nullability made explicit:

```go
type TimestampUnit uint8

const (
    UnitSeconds TimestampUnit = iota
    UnitMilliseconds
)

func TimestampNowMillis() int64
func UTCNow() string
func Timestamp8601ToSeconds(timestamp *string, zoneID *string) (*int64, error)
func Difference(first, second time.Time) string
func FormatZoneUTC(timestampMillis int64) string
func ToZoneDateTimeUTC(timestampMillis int64) time.Time
func FormatTime(value time.Time) string
func FormatRFC1123(epochTimestamp int64, unit TimestampUnit, zoneID string) (string, error)
```

Adaptation notes:

- `TimestampNowMillis` corresponds to Java `getTimestampNow()` and returns epoch **milliseconds**.
- `UTCNow` corresponds to `getUtcNow()` and returns `time.Now().UTC().Format(time.RFC3339Nano)`-equivalent ISO output. The exact Java `OffsetDateTime.toString()` shape (fraction omission/precision) must be preserved by the implementation and tested as a shape/range contract, not asserted against a fixed clock.
- `Timestamp8601ToSeconds` uses `*string` to represent Java `null` input and `*int64` to represent Java `Long` null. `nil timestamp` returns `(nil, nil)`. Parse failure returns `(nil, nil)`, matching the Java catch of `ParseException`; callers must not receive a fabricated zero.
- A nil `zoneID` defaults to `UTC`. A non-nil invalid IANA/Java zone is an error: Java `ZoneId.of` can throw before the `ParseException` catch, so do not silently fall back to UTC.
- The source parser is `SimpleDateFormat("yyyy-MM-dd'T'HH:mm")`, configured with the selected zone. It is lenient and the direct test intentionally accepts `2023-08-27T14:5` (UTC) as `1693145100`. A strict Go parser is not an acceptable unrecorded behavior change. Implement an explicit compatibility parser/normalization or make the deviation an approved decision with a failing parity test; also test source behavior for malformed/overflow dates before choosing the final adapter.
- `Difference` corresponds to `Duration.between(second, first)`, then Java `toDays`, `minusDays(...).toHours`, `minusHours(...).toMinutes`, and remaining `getSeconds`. Render exactly:
  `"%d days, %d hours, %d minutes, %d seconds"`.
  Do not use an absolute duration or a normalized sign. For negative values, Java's integer component operations truncate toward zero at each stage; therefore a negative interval can render mixed signed components (for example, a -90061-second interval is `-1 days, -1 hours, -1 minutes, -1 seconds`, while -1 second is `0 days, 0 hours, 0 minutes, -1 seconds`). Verify these examples against a Saturn/JVM oracle during implementation.
- `FormatZoneUTC` and `ToZoneDateTimeUTC` interpret the argument as epoch milliseconds, never seconds.
- `FormatTime` uses Java pattern `yyyy-MM-dd'T'HH:mm:ss z`; zone text is observable.
- `FormatRFC1123` accepts only seconds or milliseconds and applies the supplied zone. Unsupported units must return an error equivalent in meaning to Java `RuntimeException("Timestamp should be in seconds or milliseconds.")`; invalid `zoneID` must also return an error. The Java method does not default a null zone here, so the Go API deliberately takes a non-pointer zone string and should reject empty/invalid values unless source-oracle evidence says otherwise.
- Java `DateTimeFormatter.RFC_1123_DATE_TIME` has observable day-padding and zone-text behavior (`Thu, 1 Jan 1970 00:00:00 GMT` is the direct golden). Go's standard layouts commonly produce `UTC`, not `GMT`, and may differ for named/DST zones; an adapter must explicitly match Java output rather than relying on `time.RFC1123`.

### Identity owner — `internal/util/identity.go`

```go
var ErrBlankNickTarget = errors.New("Nick target cannot be blank")

func NormalizeNickTarget(raw *string) (string, error)
func CanonicalNick(raw *string) (string, error)
func SameNick(left, right *string) bool
```

`*string` makes Java null explicit. An alternative value-string API is acceptable only if null is represented by a documented separate helper; do not make null equal to empty.

Exact behavior:

1. `nil` input is invalid and must produce an error whose message is exactly `Nick target cannot be blank` (typed sentinel plus wrapping is acceptable if `errors.Is` and the message remain stable).
2. Trim surrounding whitespace.
3. Remove **exactly one** leading ASCII `@`, then trim again.
4. If empty after that, return the same error. Thus blank and bare `@` reject; `@@alice` becomes `@alice`.
5. `CanonicalNick` lowercases the normalized value with locale-root semantics. Go's Unicode `strings.ToLower` is the intended locale-independent analogue; do not use locale-sensitive behavior or ASCII-only folding.
6. `SameNick` returns false if either input is nil; otherwise normalize/canonicalize both and compare. Invalid non-nil values must not panic; they cannot compare equal.

This contract is distinct from `model.IdentityKey` (trip/hash/nick precedence) and `agent/api.UserIdentity` (source-qualified identity). Neither should be reused as a substitute.

### JSON owner — `internal/util/json_payloads.go`

```go
func Command(cmd string) string
func CommandWithValue(cmd, key, value string) string
func CommandWithValues(cmd, firstKey, firstValue, secondKey, secondValue string) string
```

If overload-like naming is preferred, use one variadic/internal builder only behind these three explicit functions; Go has no Java overloads. Inputs are strings, including empty strings. The builder must preserve input values and return a string; it has no transport side effects and no JSON marshal error path for ordinary strings.

Exact output templates from `JsonPayloads.java`:

```text
Command:            { "cmd": "<escaped-cmd>"}
CommandWithValue:   { "cmd": "<escaped-cmd>", "<escaped-key>": "<escaped-value>"}
CommandWithValues:  { "cmd": "<escaped-cmd>", "<escaped-key1>": "<escaped-value1>", "<escaped-key2>": "<escaped-value2>" }
```

The final space before `}` exists **only** in the two-value form. The no-value and one-value forms have no space before `}`. Keys and values, including `cmd`, all use the same Apache Commons Text `StringEscapeUtils.escapeJson` behavior.

**[RECOMMENDED]** Implement a small JSON-string escaper rather than directly using `encoding/json` for the fragment: Go's encoder HTML-escapes `<`, `>`, and `&` by default and may emit output different from Apache Commons Text. Required escape mapping is: `"` for quote, `\\` for backslash, `\b`, `\t`, `\n`, `\f`, and `\r` for the corresponding controls, and `\u00XX`/four-hex `\uXXXX` for remaining U+0000–U+001F controls. Do not escape `/` or ordinary non-ASCII characters unless the pinned Apache Commons Text oracle demonstrates that version does so. Test bytes, not merely JSON parseability.

## 3. Golden/test matrix

Focused tests must be table-driven and independent of callers. Each row below is a required parity vector or an explicit required decision.

### Date matrix (`date_test.go`)

| Case | Input/expected contract |
|---|---|
| now millis | `TimestampNowMillis()` is within a bounded range around `time.Now().UnixMilli()` |
| UTC now | non-empty UTC ISO string; parseable and UTC; no fixed-clock assertion |
| direct source vector | `2023-08-27T14:5`, UTC => `1693145100` |
| null timestamp | `nil` => nil result, nil error |
| invalid timestamp | parse failure => nil result, nil error |
| nil zone | same valid input with nil zone uses UTC |
| explicit zone | fixed timestamp in e.g. `America/New_York` proves zone affects epoch seconds |
| malformed/lenient dates | oracle-backed cases for strict-vs-lenient `SimpleDateFormat` behavior; no silent Go tightening |
| UTC millis conversion | epoch 0 => `1970-01-01T00:00:00 UTC`-equivalent `FormatTime` output and UTC `time.Time` |
| difference zero/positive | exact four-component text |
| difference negative | -1 second and multi-component negative interval; verify Java truncation semantics |
| RFC seconds | epoch second 0, `UnitSeconds`, UTC => `Thu, 1 Jan 1970 00:00:00 GMT` |
| RFC milliseconds | epoch millis 0, `UnitMilliseconds`, UTC => same golden |
| RFC non-UTC | fixed seconds/millis with at least one named zone; compare JVM oracle zone text and local time |
| unsupported unit | error with source-equivalent message; no output |
| invalid zone | error; no UTC fallback |

### Identity matrix (`identity_test.go`)

| Input | Expected |
|---|---|
| `nil` | error `Nick target cannot be blank` |
| `"  "` | same error |
| `" @ "` | same error |
| `"  @alice  "` | `alice` |
| `"@@alice"` | `@alice` (one marker only) |
| Unicode/mixed case | root/locale-independent canonical lowercasing |
| `SameNick("@Alice", "alice")` | true |
| different names | false |
| either nil in `SameNick` | false |
| invalid non-nil comparison | false, no panic |

### JSON matrix (`json_payloads_test.go`)

| Case | Expected assertion |
|---|---|
| command only | exact template, no space before `}` |
| one value | exact Saturn test: `ban`, `nick`, `mer"c\` => `{ "cmd": "ban", "nick": "mer\\"c\\"}` as Go-quoted expected literal |
| two values | exact Saturn test: `kick`, `nick=me\`, `to=x"y` => `{ "cmd": "kick", "nick": "me\\", "to": "x\\"y" }` |
| escaped command/key | quotes and backslashes in `cmd` and key are escaped too |
| newline/CR/tab/backspace/form-feed | exact short escapes `\n`, `\r`, `\t`, `\b`, `\f` |
| remaining controls | exact four-hex Unicode escapes |
| empty strings | valid quoted empty command/key/value, exact spacing retained |
| ordinary Unicode and slash | unchanged unless Apache oracle says otherwise |
| trailing backslash | no dropped/duplicated slash |
| parseability | each output parses as JSON, but exact-byte assertion remains primary |

The expected strings should be written with Go raw literals where that improves readability, while comments identify the actual control bytes. Add a JVM oracle test/script only if needed to settle Commons Text or `SimpleDateFormat` edge behavior; do not make Saturn a runtime dependency.

## 4. Risks and explicit non-decisions

1. **Date parser leniency:** Go layouts are often stricter than Java `SimpleDateFormat`; one-digit minutes are already a required compatibility vector.
2. **Timezone data/text:** `time.LoadLocation` availability, DST transitions, Java zone names, and `GMT` versus `UTC` rendering can diverge. Use fixed oracle vectors and fail closed on invalid zones.
3. **Negative durations:** component extraction must follow Java's ordered truncation, not conventional absolute decomposition.
4. **Units:** millisecond/second confusion is likely because both APIs use `int64`; keep the unit type closed and test epoch zero plus a non-zero value.
5. **Identity security boundary:** changing trim order, stripping all `@` markers, ASCII-only lowercasing, or null handling can alter authorization/user lookup behavior. No callers are migrated here.
6. **JSON escaping:** `encoding/json` is not byte-equivalent by default due to HTML escaping and implementation details. Preserve exact spaces and escape keys as well as values.
7. **Existing partial helpers:** do not delete or rewrite `core.escapeJSON`, `model.IdentityKey`, command inline normalization, or listener matching in this architecture/implementation slice.
8. **Source dependency version:** `JsonPayloads` imports Apache Commons Text, while the inspected POM directly declares Commons Lang; resolve the actual transitive/pinned Commons Text implementation before claiming complete escape parity.

## 5. Exclusions

This handoff must not modify or wire:

- `internal/core`, `internal/command`, `internal/listener`, `cmd/zenbot/main.go`, transport, router, provider, or tool behavior;
- H2, persistence, DBZ, SQL policy, moderation, remote-room, Whiskey, replica/lifecycle, or agent routing;
- `Constants`, `Util`, `DBZUtil`, `SeparatorFormatter`, or `SqlUtil`;
- Saturn source, tests, build files, or generated artifacts;
- the frozen migration audit/plan verdict.

Do not claim caller migration or completion of rows outside #320–#322.

## 6. Acceptance gates

The implementation/QA handoff may accept this slice only when all are evidenced:

1. `internal/util/date.go`, `identity.go`, and `json_payloads.go` exist with focused tests and compile under `module zenbot`.
2. Every API and adaptation above is documented in code comments/tests; null, invalid-zone, parse-failure, and unsupported-unit semantics are covered.
3. Date tests pass all source vectors, including one-digit minute parsing, explicit/nil zones, seconds/milliseconds, exact RFC output, negative duration components, and invalid units/zones.
4. Identity tests prove exact one-marker removal, trim order, blank/bare-marker errors, root lowercasing, and null-safe comparison.
5. JSON tests prove exact byte shape, all three templates, key/value/command escaping, controls/newline forms, empty strings, trailing slash, and the two-value closing-space quirk.
6. No existing application file or Saturn file changed; no caller was migrated and no existing helper was silently replaced.
7. Run and record `gofmt`/`go fmt`, focused `go test -count=1 ./internal/util`, then repository gates from the plan: `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
8. Inspect `git status --short` and confirm unrelated dirty/untracked work is preserved; only task-owned utility files/tests and handoff artifacts may be newly attributable.
9. Update audit evidence only in a later acceptance/ledger step after real test output; the overall migration verdict remains **NOT COMPLETE**.

## 7. Architecture verdict

**[RECOMMENDED]** `internal/util` with three small files is the smallest stable owner and avoids generic helpers leaking into `internal/core` or domain models. This document is ready for an implementation agent, subject to the explicit JVM-oracle decisions for lenient date parsing, negative duration verification, timezone rendering, and Apache Commons Text escaping. 
