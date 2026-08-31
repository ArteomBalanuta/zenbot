# SqlUtil QA Handoff

## Verdict

**PARTIAL/FAIL — row #324 is not accepted.**

This is a QA handoff only. It does **not** establish completion of the Saturn `SqlUtil` migration and must not be reported as an overall migration completion.

## Scope and exact evidence

- Audit row **#324** is Saturn `SqlUtil`, with **31 public constants** in `src/main/java/org/saturn/app/util/SqlUtil.java`.
- Existing Zenbot H2/repository behavior covers some overlapping contracts and was reused for verification.
- Saturn and Zenbot H2 schemas were verified equal.
- Existing H2 tests were verified passing.
- Existing `gofmt`, `go vet`, and build checks were verified passing.
- Source-count evidence confirms the Saturn inventory contains 31 public constants.
- Existing repository observations include parameterized SQL usage and transaction handling in the overlapping H2/repository paths; these observations do not prove that every `SqlUtil` constant has the required equivalent call path or semantics.

## Acceptance failure

Row #324 is not accepted because no task-owned implementation or task-owned tests establish all **31 exact constants and their call paths**. Several constants remain blocked or shape-mismatched, so passing existing H2/repository tests and equal schemas are insufficient evidence of contract-complete migration.

In particular, the current evidence does not demonstrate, for every constant:

1. exact SQL/value parity with Saturn;
2. an equivalent supported Zenbot call path;
3. correct parameterization and transaction behavior for that path; and
4. task-owned regression coverage.

## Remaining blockers

- No complete task-owned `SqlUtil` catalog was added.
- No all-31 contract test suite was added.
- Several constants remain blocked by callers that are unsupported, unavailable, or otherwise require an explicit scope decision.
- Several remaining constants are shape-mismatched with the existing Zenbot H2/repository interfaces and cannot be accepted merely because overlapping behavior passes.
- Existing dirty target files and the Saturn source remain unchanged; their presence is not evidence of this row's closure.

## Changed-file attribution

- **QA handoff only:** `.hermes/handoffs/sql-util-qa.md`.
- No application code, Saturn source, registration work, or row #325 work was changed by this QA pass.
- The implementation handoff reports no production/test files changed; existing H2/repository code and tests were reused rather than supplemented with a complete `SqlUtil` implementation and catalog tests.

## Required next step

Perform a new bounded architecture/implementation pass for the still-supported constants, including exact mappings, supported call paths, and task-owned contract tests. For blocked callers, make an explicit scope decision before claiming coverage or closure. Re-run QA against that bounded scope and retain the **PARTIAL/FAIL** verdict until the required evidence exists.
