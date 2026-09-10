# Next Migration Slice Diagnostic 2 — Pure Utility Contracts

**Date:** 2026-08-30  
**Target:** `/Users/ab/workspace/go-projects/zenbot`  
**Source:** `/Users/ab/workspace/projects/saturn` (`develop`, read-only)  
**Decision:** Select the bounded **date/time, nick identity, and JSON payload utility contract** slice: Saturn audit rows **#320–#322** (`DateUtil`, `IdentityUtil`, `JsonPayloads`). This is a small, provider-neutral slice with direct Saturn tests and no need to activate live transport, listeners, commands, H2 persistence, tools, or agent routing.

> This diagnostic is a selection and implementation handoff, not a migration-closure claim. The frozen audit and migration plan remain **NOT COMPLETE**.

## 1. Why this is the next slice

**[OBSERVED]** The plan requires all utility rows to be behaviorally migrated, and the audit still marks rows #320–#322 as `needs implementation`. The accepted identity, DBZ, transport/lifecycle/replica, agent-contract/routing/tool/turn, and SQL-policy slices cover adjacent runtime foundations but do not provide a shared utility owner or parity tests for these three contracts.

This slice is preferred over the remaining broad listener/router/command wiring because it is:

- pure and request-local (no network, goroutines, H2, or external provider);
- backed by direct Saturn source and focused tests;
- small enough to implement and review without touching DBZ/shared integration files;
- useful to existing target paths that currently duplicate nick normalization and hand-build JSON payloads;
- independently verifiable with deterministic vectors, except for explicitly time-dependent `now` helpers.

The slice deliberately excludes `Util`, `Constants`, `DBZUtil`, `SeparatorFormatter`, and `SqlUtil`; those either have weak/no direct tests, cross-cutting command/DBZ coupling, or SQL/persistence implications and should be selected separately.

## 2. Exact pending audit rows

From `.hermes/migration-audit.md` rows **#320–#322** (the utility mapping is in the exhaustive Java table):

| Audit row | Saturn unit | Saturn evidence | Frozen status | Proposed target owner |
|---:|---|---|---|---|
| #320 | `DateUtil` | `src/main/java/org/saturn/app/util/DateUtil.java` | **needs implementation** | new `internal/util/date.go` (or the repository’s agreed utility package) |
| #321 | `IdentityUtil` | `src/main/java/org/saturn/app/util/IdentityUtil.java` | **needs implementation** | new `internal/util/identity.go`, with callers migrated only in a later coordinated pass |
| #322 | `JsonPayloads` | `src/main/java/org/saturn/app/util/JsonPayloads.java` | **needs implementation** | new `internal/util/json_payloads.go` |

The audit’s focused-verification column requires a parity test for each unit; current target package layout has `internal/model/**` and command/core code, but no dedicated `internal/util/**` owner or equivalent complete utility contract. Existing `internal/core/engine_impl.go` has a private `escapeJSON` helper and existing command code normalizes `@` targets inline; these are partial implementations, not closure evidence for the audited units.

## 3. Saturn source and test evidence

### `DateUtil`

Source: `src/main/java/org/saturn/app/util/DateUtil.java`.

Observed public behavior:

- `getTimestampNow()` returns epoch milliseconds.
- `getUtcNow()` returns `OffsetDateTime` in UTC using its ISO string form.
- `tsToSec8601(timestamp, zoneId)` returns `null` for a null timestamp or parse failure; parses `yyyy-MM-dd'T'HH:mm` in the supplied zone, defaulting a null zone to UTC, and returns epoch seconds.
- `getDifference(first, second)` computes `Duration.between(second, first)` and renders signed day/hour/minute/second components as `"%d days, %d hours, %d minutes, %d seconds"`.
- `formatZoneUTC` and `toZoneDateTimeUTC` interpret epoch milliseconds in UTC.
- `formatTime` renders `yyyy-MM-dd'T'HH:mm:ss z`.
- `formatRfc1123` accepts seconds or milliseconds, uses the supplied zone, and throws for other time units.

Direct Saturn test: `src/test/java/org/saturn/app/util/DateUtilTest.java` covers parsing `2023-08-27T14:5` as UTC epoch seconds and RFC-1123 formatting of the Unix epoch. The implementation must add additional vectors for null/invalid input, zones, negative durations, milliseconds versus seconds, and unsupported units; do not silently normalize behavior beyond the source contract.

### `IdentityUtil`

Source: `src/main/java/org/saturn/app/util/IdentityUtil.java`.

Observed public behavior:

- `normalizeNickTarget(null)` and blank/bare-`@` values throw `IllegalArgumentException("Nick target cannot be blank")`.
- It trims, removes exactly one leading `@`, trims again, and preserves a second `@` (for example `@@alice` becomes `@alice`).
- `canonicalNick` lowercases the normalized value using locale-root semantics.
- `sameNick` returns false for either null and otherwise compares canonical values case-insensitively.

Direct Saturn test: `src/test/java/org/saturn/app/util/IdentityUtilTest.java` covers trimming/one-marker removal, blank rejection, and case-insensitive comparison.

Target evidence: `internal/command/handlers.go` performs inline `@` stripping and `strings.EqualFold`; `internal/listener/user_joined_listener.go` also performs matching inline. These are behavior fragments, not a reusable audited owner. Do not broaden this slice into a command/listener refactor; first establish the utility contract and tests.

### `JsonPayloads`

Source: `src/main/java/org/saturn/app/util/JsonPayloads.java`.

Observed public behavior:

- Three overloads build exact command JSON strings for command-only, one key/value, and two key/value payloads.
- Command, key, and value strings are JSON-escaped with Apache Commons `escapeJson`.
- Exact spacing is observable in the source/tests, including the two-key overload’s space before `}`.

Direct Saturn test: `src/test/java/org/saturn/app/util/JsonPayloadsTest.java` covers quote and backslash escaping for one- and two-value commands. Add newline, carriage-return, control-character, empty-string, and key-escaping vectors while preserving exact output shape.

Target evidence: `internal/core/engine_impl.go` currently builds chat JSON with a private `escapeJSON` helper and `fmt.Sprintf`; `internal/command/handlers.go` marshals raw values separately. This slice should add a provider-neutral builder, but must not wire or rewrite engine output in the same change.

## 4. Current Zenbot gap

**[OBSERVED]** There is no dedicated `internal/util` package implementing these three audited units. The target has:

- partial inline nick normalization/comparison in command/listener code;
- a private JSON string-escaping helper and manually interpolated payloads in `internal/core/engine_impl.go`;
- no DateUtil equivalent with the Saturn method set, time-unit behavior, or focused parity tests.

Therefore, passing existing command/core tests cannot mark rows #320–#322 complete. A future implementation must name owners and add focused tests before any caller migration or audit update.

## 5. Scope and non-scope

### In scope

- Add the smallest provider-neutral utility package/owners for `DateUtil`, `IdentityUtil`, and `JsonPayloads`.
- Preserve Saturn’s null/error behavior, one-leading-marker rule, locale-root lowercasing, epoch units, timezone handling, duration text, RFC-1123 output, exact JSON escaping, and exact payload spacing.
- Add table-driven Go tests corresponding to the direct Saturn vectors plus edge cases required to expose unit/zone/escaping regressions.
- Keep APIs deterministic where possible; isolate `now` functions so tests can assert shape/range without introducing clock-dependent flakes.
- Document any deliberate Go adaptation (for example, typed errors or `time.Time` return types) without changing observable semantics.

### Explicitly out of scope

- Migrating callers in `internal/core`, `internal/command`, `internal/listener`, or `cmd/zenbot/main.go`.
- Reworking transport/raw/chat/update-message behavior, listener ordering, command registration, agent routing, tool execution, or provider integration.
- `Util`, `Constants`, `DBZUtil`, `SeparatorFormatter`, or `SqlUtil`; no SQL occurrence or DBZ behavior is owned here.
- H2 schema/repository/service changes, persistence registration, message visibility policy, moderation actions, remote-room operations, Whiskey proxy behavior, or Saturn edits.
- Adding new product behavior or claiming rows outside #320–#322 are complete.

## 6. Risk assessment

**Risk: low-to-medium.** The footprint is small, but date/time and wire-format quirks are easy to alter accidentally, and identity normalization is security-adjacent when used for authorization or user lookup.

Primary risks:

1. Epoch seconds/milliseconds can be confused; `formatRfc1123` must preserve the explicit unit switch and failure for unsupported units.
2. Java `SimpleDateFormat` parsing is lenient and accepts the tested one-digit minute; a stricter Go parser could create an undocumented deviation. Decide and test the compatibility behavior before implementation.
3. `ZoneId.of` failures and null zone handling must not be silently converted into a different timezone.
4. `Duration` component rendering for negative intervals must be checked rather than inferred from positive examples.
5. Removing more than one leading `@`, using non-root case folding, or treating null as equal would change identity semantics.
6. Hand-written JSON interpolation can mishandle trailing backslashes, CRLF, literal backslash-n, or escaped keys. The builder must be tested as wire bytes and must not be confused with chat/raw engine integration.
7. Moving callers during this slice would create shared-file churn and make regressions difficult to attribute; keep the contract addition separate.

## 7. Staged architecture, implementation, and QA acceptance

### Architecture stage

1. Confirm audit rows #320–#322 and the Saturn source/test paths above against the frozen files and read-only checkout.
2. Choose stable Go signatures and return types, explicitly recording Java-to-Go adaptations for checked/unchecked errors, timezone identifiers, and time-unit enums.
3. Define exact JSON output golden strings and the null/invalid/error matrix before implementation.
4. Decide whether `internal/util` is the owner or whether existing repository conventions require `internal/model`; do not place generic helpers in `internal/core`.
5. Prove the slice has no callers or registration changes; caller migration is a later task.

### Implementation stage

1. Add task-owned utility files and focused tests only.
2. Implement DateUtil with explicit seconds/milliseconds handling, UTC conversion, zone parsing, and Saturn-compatible formatting/error behavior.
3. Implement IdentityUtil with exactly one leading-marker removal, trim order, root case folding, and null-safe comparison.
4. Implement JsonPayloads with structured escaping and exact Saturn spacing/overload output; preserve original input values.
5. Keep existing engine/command helpers untouched unless a separately approved compatibility shim is required; no broad refactor.

### QA stage

1. Run focused tests RED-first for the missing utility contracts, then GREEN after implementation.
2. Verify direct Saturn vectors and add negative/edge vectors: null/blank/bare marker, `@@`, Unicode case, invalid timestamp, null/default zone, explicit non-UTC zone, seconds/milliseconds, unsupported unit, negative duration, quotes, backslashes, CRLF, newline, control characters, empty keys/values, and exact payload spacing.
3. Inspect package callers to confirm no live engine/listener/command wiring was introduced.
4. Run task-owned formatting and focused tests, then the repository baseline gates required by the plan: `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
5. Independently inspect `git status --short` and confirm only task-owned utility files/tests plus this handoff changed; Saturn remains read-only and unrelated dirty work is preserved.

## 8. Slice acceptance criteria

The slice is accepted only when backed by actual test output and source inspection:

- [ ] Audit rows #320–#322 each have a named Go owner and focused parity tests.
- [ ] Date parsing, UTC/default-zone behavior, epoch units, duration rendering, UTC formatting, RFC-1123 formatting, and unsupported-unit failure match the documented Saturn contract.
- [ ] Identity normalization removes exactly one leading `@`, rejects null/blank/bare marker, uses root case folding, and is null-safe for comparisons.
- [ ] JSON payload overloads produce exact Saturn output and safely escape quotes, backslashes, newline forms, control characters, keys, and values.
- [ ] No command/listener/core caller migration, live wiring, H2/persistence, agent/tool/provider, moderation, remote-room, Whiskey, DBZ, or Saturn changes are introduced.
- [ ] Focused normal/race tests, full normal/race tests, vet, build, formatting, and diff checks pass.
- [ ] The overall migration remains explicitly reported as **NOT COMPLETE**.

## 9. Diagnostic verification

This artifact must be verified after writing: it must be non-empty, contain the required sections, and every cited existing Saturn and Zenbot path must resolve. This diagnostic pass must not modify application code or Saturn.
