# SeparatorFormatter Architecture

## Audit linkage (confirmed)

- **Migration slice:** Saturn `SeparatorFormatter`.
- **Exact audit row:** **#323** in `.hermes/migration-audit.md` (the table's source line is 346): package `org.saturn.app.util`, class `SeparatorFormatter`, evidence `src/main/java/org/saturn/app/util/SeparatorFormatter.java`, status **needs implementation**, focused verification “Add/run parity test for `SeparatorFormatter` (real H2 where persistence is involved)”.
- **Saturn repository:** `/Users/ab/workspace/projects/saturn` (read-only, branch `develop` per the audit).
- **Exact production path:** `src/main/java/org/saturn/app/util/SeparatorFormatter.java`.
- **Exact focused test path:** `src/test/java/org/saturn/SeparatorFormatterTest.java`.
- **Target handoff:** this document only. No application code or Saturn file is changed by this architecture update.

The previously carried-forward claim that the row and paths were unconfirmed is superseded by the evidence above. Rows #320–#322 are `DateUtil`, `IdentityUtil`, and `JsonPayloads`; they do not cover this class.

## Confirmed Saturn API and method semantics

**[OBSERVED]** `SeparatorFormatter.java` declares a public class with three static methods:

```java
public static List<String> addSeparator(List<String> list, char separator)
public static Object getFirst(List<String> list)
public static Object getLast(List<String> list)
```

**[TEST-BACKED]** `SeparatorFormatterTest.java` supplies one list fixture:

```text
[null, "test", "test2", "test3", "test4", null]
```

It asserts:

- `getFirst(list)` returns `"test"` (first non-null element).
- `getLast(list)` returns `"test4"` (last non-null element found by the implementation).
- `addSeparator(list, ',')` returns the mutated list `[null, "test,", "test2,", "test3,", "test4", null]`.

**[OBSERVED]** The exact production behavior is not a pure join and must not be described as one:

1. `addSeparator` reads `list.size()`; a Java `null` list therefore fails immediately with `NullPointerException`.
2. If the size is `0` or `1`, it returns the same list unchanged.
3. Otherwise it calls `getLast(list)`. `getLast` scans backward starting at `size - 1` but stops when the index reaches `0`; index `0` is never inspected. It returns the last non-null element found at an index `>= 1`, or `null` if none is found.
4. `addSeparator` iterates the list in order and stops when an element is the **same object reference** as the value returned by `getLast` (`e == last`), not merely equal by string contents.
5. Before that stop, each non-null element is converted with `e.toString() + separator`; the code then replaces `list.indexOf(e)` with that string. This mutates the caller's list and returns the same list object. Null elements are skipped, not removed.
6. Because replacement uses `indexOf` and the stop test uses reference identity, duplicate strings and unusual aliasing can produce behavior that is not equivalent to a conventional separator join. Do not normalize this away without an explicit compatibility decision and test.
7. `getFirst` scans from the front, skips nulls, and returns the first non-null object; if none exists it returns `null`. A null list fails during iteration.
8. `getLast` returns `null` for an empty list and for a one-element list (its loop never examines index `0`); a null list fails during iteration. For larger lists it ignores index `0` and returns the last non-null value at an index greater than zero.
9. The separator is a primitive Java `char`; there is no null-separator or separator-error case in the source API. The focused test uses comma only. Empty separators, whitespace, punctuation, Unicode, duplicate values, and additional null layouts are not established by the focused test and must not be called parity requirements until separately approved and tested.

## Exact Go signature decision

**[RECOMMENDED / DECISION FOR IMPLEMENTATION]** Use a package-level utility under the audit-approved `internal/model/**` target (rather than inventing an `internal/util` target) with this public API:

```go
func AddSeparator(values []*string, separator rune) []*string
func GetFirst(values []*string) (string, bool)
func GetLast(values []*string) (string, bool)
```

Reasons for this exact boundary:

- `[]*string` preserves Java `null` list elements and permits the returned slice to be the same mutable sequence, matching the source's in-place behavior. A plain `[]string` cannot distinguish Java `null` from `""`.
- `rune` is the idiomatic Go representation of a character at this boundary; the implementation must document/test the deliberate Java-`char` to Go-rune adaptation if non-ASCII separators are added. The confirmed comma case is unchanged.
- `AddSeparator` returns the same slice after mutating its element slots; it must not be replaced with a pure `string`-returning formatter or an error-returning signature. There is no source-level error result.
- `(string, bool)` represents Java's nullable `Object` result for the two lookup methods without conflating “no value” with an empty string.

**[LIMITATION]** Go `string` values do not expose Java object-reference identity. The implementation must either (a) preserve the tested behavior for ordinary values and explicitly document duplicate/reference-identity adaptation, or (b) introduce a narrower internal representation whose tests demonstrate identity parity. It must not silently claim exact parity for untested duplicate aliasing. The implementation phase owns that focused compatibility test/decision; this architecture does not authorize changing Saturn.

## Implementation contract and scope

### In scope

1. Add the smallest implementation under the selected `internal/model/**` target of the three signatures above, after the identity-adaptation decision is captured in tests.
2. Port the three focused Saturn assertions one-for-one using the fixture and expected mutation shown above.
3. Add explicit tests for the confirmed size-0/size-1 early return, null elements, first/last lookup behavior, and in-place mutation/returned-slice identity.
4. Add duplicate/reference-identity cases only if the implementation makes a documented compatibility choice; label any Go adaptation rather than presenting it as observed Saturn behavior.
5. Keep the utility dependency-free and independent of services, logging, configuration, I/O, persistence, and global state.

### Explicitly out of scope

- Any modification to Saturn.
- `SqlUtil`, `Util`, SQL policy, application wiring, broad caller replacement, or unrelated refactoring.
- Converting this helper into a pure join/formatter, adding trimming/escaping/filtering/sorting, or inventing null/error behavior.
- Treating empty separators, Unicode separators, or duplicate values as Saturn-proven requirements without evidence beyond the two cited files.

## Acceptance gates

The slice is accepted only when every gate passes:

- [ ] Audit row **#323** is cited, with the exact production and test paths above.
- [ ] The implementation exposes exactly the decided Go signatures, or any change is separately justified and approved.
- [ ] `AddSeparator` is verified as in-place mutation returning the same slice, not as a pure join.
- [ ] The Saturn fixture and all three focused assertions pass in Go parity tests.
- [ ] Size `0`/`1`, null elements, `GetFirst`, and `GetLast` semantics match the observed implementation, including `GetLast`'s index-0 exclusion.
- [ ] Java-null versus Go representation is explicit; empty strings are not silently treated as null.
- [ ] The Java reference-identity limitation for duplicates is covered by a documented implementation decision and regression tests, or the slice is blocked as unresolved.
- [ ] No invented null-separator/error behavior is present; the source accepts primitive `char` and returns no error.
- [ ] Focused tests pass; required formatting/static checks and the relevant affected-package Go tests pass.
- [ ] Saturn remains unchanged, and no unrelated application files or pre-existing dirty/untracked files are altered.
- [ ] Final diff contains no caller-API changes, normalization, or scope expansion.

## Verification record

Evidence inspected for this update:

- `.hermes/migration-audit.md`, row #323.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SeparatorFormatter.java`.
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/SeparatorFormatterTest.java`.

This document is an architecture handoff, not an implementation. The target application and Saturn source/test remain untouched.
