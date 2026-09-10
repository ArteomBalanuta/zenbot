# Next Migration Slice Diagnostic 3

## Decision

Select **SeparatorFormatter** as the next bounded pure-utility migration slice.

This is intentionally narrower than the combined `SeparatorFormatter` / `SqlUtil` / `Util` candidate set: migrate and contract-test separator formatting only, without broad utility cleanup, SQL policy changes, or application wiring.

## Audit linkage

- **Authoritative audit:** `.hermes/migration-audit.md`
- **Known candidate evidence:** the pending pure-utility candidates include `SeparatorFormatter`, `SqlUtil`, and `Util`.
- **Exact audit row(s):** not retained in the available selector evidence. The architecture phase must confirm the exact row number(s) in `.hermes/migration-audit.md`; no row number is asserted here.
- **Prior accepted boundary:** SQL policy rows `#86–#90` and utility-contract rows `#320–#322` are accepted work and are not part of this slice.

## Saturn evidence map

Saturn remains read-only at `/Users/ab/workspace/projects/saturn`.

- **Source symbol:** `SeparatorFormatter` (pure formatting utility).
- **Source path:** exact Saturn repository-relative path requires confirmation in the architecture phase from the known audit/source evidence. It is not reproduced here because the path was not retained in the current handoff context.
- **Test symbol:** the focused `SeparatorFormatter` contract tests, if present in the Saturn checkout.
- **Test path:** exact Saturn repository-relative path requires confirmation in the architecture phase from the known audit/source evidence. It is not reproduced here because the path was not retained in the current handoff context.
- **Evidence constraint:** do not infer package or directory names from the class name; resolve the source and focused test paths before implementation.

## Current Go gap

Zenbot has no accepted, bounded Go implementation/contract for the Saturn `SeparatorFormatter` behavior in the migration evidence currently available to this handoff. The gap is behavioral parity at the utility boundary: the Go side needs a small, directly testable formatter contract whose outputs match Saturn for ordinary separators and edge cases established by the Saturn source/tests.

The gap does **not** establish that no similarly named helper exists anywhere in the Go tree; confirming collisions and call sites belongs to architecture. This diagnostic therefore avoids claiming a missing symbol by filename alone.

## Scope

### In scope

1. Confirm the exact audit row(s), Saturn source path, test path, package, public API, and behavior.
2. Define one Go package-level pure utility for separator formatting, with the smallest API justified by Saturn.
3. Port only the behavior covered by the Saturn implementation and focused tests.
4. Add deterministic Go unit tests for normal, boundary, empty, and repeated-separator cases that are actually supported by Saturn evidence.
5. Record parity decisions for whitespace, null/empty input, and invalid arguments rather than silently choosing semantics.

### Explicitly out of scope

- `SqlUtil` and `Util` migration.
- SQL policy work or changes to the accepted rows `#86–#90`.
- Changes to utility contracts already accepted under `#320–#322`.
- Application wiring, handler/service refactors, broad call-site replacement, or package-wide renaming.
- Saturn source or test edits.
- Performance work, generalized string templating, or a new formatting framework.

## Risks and mitigations

- **Unconfirmed audit/path metadata:** row and filesystem identifiers could be wrong if reconstructed from memory. Mitigation: make architecture confirmation a hard gate and do not invent identifiers.
- **Semantic drift at edge cases:** empty input, whitespace, or repeated separators may differ between Java and Go. Mitigation: derive cases from Saturn source/tests and use table-driven parity tests.
- **Accidental broad wiring:** a utility can attract unrelated cleanup. Mitigation: require a changed-file and call-site allowlist in review.
- **Contract overlap:** nearby accepted utility work may already own part of the behavior. Mitigation: compare against rows `#320–#322` before implementation and split ownership explicitly.
- **False absence claim:** a Go helper may exist under another name. Mitigation: architecture confirms the current Go symbol/package and chooses add, adapt, or no-op accordingly.

## Staged execution

### 1. Architecture gate

- Confirm exact audit row number(s) in `.hermes/migration-audit.md`.
- Inspect Saturn `SeparatorFormatter` source and focused tests; record exact repository-relative paths, package, signature, examples, and error/edge semantics.
- Inspect the relevant Go package and call sites only to establish whether the gap is absent, partial, or already covered.
- Produce a one-page contract and an explicit changed-file/call-site allowlist.

### 2. Implementation gate

- Implement the smallest pure Go utility matching the confirmed Saturn contract.
- Keep the implementation independent of I/O, database state, configuration, and application services.
- Do not modify Saturn or touch `SqlUtil`/`Util`.

### 3. QA gate

- Run focused Go unit tests, including all Saturn-backed examples and edge cases.
- Run formatting/static checks required by the repository.
- Run the relevant broader Go test target to detect package integration regressions, without expanding the feature scope.
- Review the diff for the allowlist, accidental wiring, and unrelated dirty/untracked-file changes.

## Acceptance gates

The slice is accepted only when all gates pass:

1. Exact audit row(s), Saturn source path, and Saturn test path are confirmed and cited; unresolved identifiers block implementation.
2. The Go API and semantics are written as a small explicit contract, including edge behavior.
3. Focused Go tests demonstrate parity for every behavior supported by the confirmed Saturn evidence.
4. Tests and required checks pass with real command output recorded in the implementation handoff.
5. No Saturn files are modified.
6. No SQL-policy, `SqlUtil`, `Util`, broad wiring, or unrelated refactor changes appear in the diff.
7. Existing accepted work (`#86–#90`, `#320–#322`) remains untouched unless a separately documented contract conflict is proven.

## Recommendation

Proceed with `SeparatorFormatter` only after the architecture phase resolves the exact audit row and both Saturn paths. If the confirmed evidence shows that the formatter is already fully covered by accepted rows `#320–#322`, cancel this slice rather than duplicating the contract; otherwise keep the implementation and QA strictly utility-local.
