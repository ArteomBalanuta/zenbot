# Agent Mention Parser Reconciliation Architecture

## Scope and ownership

- **Target owner:** `internal/agent/participation`.
- **Implementation owner:** `@developer`, for an isolated contract fix only.
- **Likely files, conditional on proven parity gaps:**
  - `internal/agent/participation/invocation.go` — adjust the existing `MentionParser` contract/implementation only if comparison identifies a gap.
  - `internal/agent/participation/policies.go` — touch only if the parser contract is exposed or consumed there and the proven fix requires it.
  - `internal/agent/participation/policies_test.go` — add or amend focused regression cases for each proven gap.
- Do not claim parity, implementation, or passing tests until the comparison and execution gates below are complete.

## First implementation step (mandatory)

Transcribe the **exact Saturn regex and prompt-cleanup semantics** from:

- `src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`
- `src/test/java/org/saturn/app/agent/room/AgentMentionParserTest.java`
- `src/test/java/org/saturn/app/agent/routing/AgentGapContractRedTest.java`

into the task ledger, including the literal matching boundaries, case behavior, capture/retained text, and cleanup ordering. Then compare that ledger against the existing Go `MentionParser` before editing any Go source.

## Required Saturn-vs-Go comparison

Produce a behavior table, not an intuition-based equivalence claim, covering:

1. **Recognition:** Saturn regex shape, exact bot-mention requirement, and case-insensitive behavior versus Go matching behavior.
2. **Boundaries:** text immediately before/after a candidate mention, including the boundary cases represented by `TestMentionParserBoundariesAndCleanup`; record the expected result directly from Saturn tests/source.
3. **Cleanup:** whether and how the mention is removed, whitespace/prompt normalization, retained content, and operation ordering. Do not infer normalization rules not shown by source/tests.
4. **Non-matches and multiple candidates:** test only the cases explicitly evidenced by Saturn source/tests or already represented in the Go focused tests; mark any unverified behavior as an open question rather than defining it.
5. **Integration contract:** preserve existing accepted routing, participation, and assembly behavior; the parser reconciliation must not change unrelated policy decisions.

A parity gap is actionable only when a row in this table has an evidence-backed Saturn expectation that differs from the current Go behavior.

## Exact edge-case test set

The focused test set must be derived from the two Saturn test classes and the existing Go test `TestMentionParserBoundariesAndCleanup`. At minimum, retain coverage for the evidenced contract of:

- a case-insensitive exact bot mention;
- mention-boundary inputs covered by the Saturn and Go focused tests;
- prompt cleanup inputs covered by those tests, asserting both recognition and resulting prompt text;
- the routing/participation/assembly behavior already accepted by the repository, as a regression guard.

Do **not** add speculative cases or assert unspecified behavior. If the source does not settle a case, document it as unresolved and leave it out of the contract change unless the task ledger is subsequently updated with source evidence.

## Delivery gates

### RED — establish the gap

- Add or amend a focused Go regression test that encodes the exact Saturn-observed expectation.
- Run the focused test and record the failing assertion as evidence of the gap.
- If no focused test can be made to fail from verified Saturn semantics, stop: no implementation edit is justified.

### GREEN — smallest contract fix

- Change only the smallest existing implementation surface needed to satisfy the RED test.
- Prefer the existing `MentionParser`; do not create a parser package or replace the participation pipeline.
- Run focused tests, then the relevant `internal/agent/participation` test suite.
- Confirm existing routing/participation/assembly tests remain green.

### QA — reconciliation and scope check

- Re-run the Saturn-vs-Go behavior table against the final implementation.
- Verify every changed assertion is source-backed and no unspecified semantics were introduced.
- Review the diff for scope: only the conditional files above, with no transport/provider/listener/remote-room changes.
- Report tests actually run and their results; do not report parity unless every compared row is verified.

## Complexity and risk

- **Expected complexity:** low; a bounded comparison plus a minimal parser-contract correction, if needed.
- **Primary risk:** changing mention boundaries or cleanup in a way that alters accepted downstream participation behavior.
- **Mitigation:** RED-first focused regression, GREEN focused-plus-package tests, and QA comparison against exact Saturn semantics.

## Explicit exclusions

- No new parser package or parallel parser abstraction.
- No full routing/engine wiring.
- No provider, listener, transport, remote-room, or Whiskey expansion.
- No row `#324` B/C, row `#325`, or `internal/agent/sql` work.
- No broad refactor of participation policies or invocation assembly.

**Current status:** architecture and reconciliation procedure only. Saturn-vs-Go parity remains unverified, and no implementation completion is claimed.
