# BLOCKED Diagnostic — Audit Row #324

**Status:** BLOCKED — validation-only work cannot be safely source-grounded.

## Audit row

- **Row:** #324
- **Source symbol:** `org.saturn.app.util.SqlUtil`
- **Source path:** `src/main/java/org/saturn/app/util/SqlUtil.java`
- **Proposed target area:** `internal/repository/h2/*.go`

## Blocking diagnosis

No safe validation-only subset can be derived from the available source evidence:

1. `SqlUtil` declares 31 public SQL string constants, and those constants are persistence SQL rather than an independently specified validation contract.
2. There is no focused Saturn test at `src/test/java/org/saturn/SqlUtilTest.java` that defines or verifies a portable contract for those constants.
3. `SqlUtil` is imported and used by multiple Saturn persistence/service implementations. Mapping the constants into Zenbot would therefore either invent a new API/contract or duplicate database/persistence contracts without source-grounded ownership.

Consequently, migrating or reproducing the constants is not validation-only work. It would be a persistence/database contract migration and must not be represented as a validation result.

## Important distinction

Existing Zenbot SQL policy validation in `internal/agent/sql/policy.go` is a validation-policy boundary. It is **not** an equivalent catalog of `SqlUtil` persistence constants and does not provide evidence that the 31 Saturn constants can be migrated or validated as the same API. The presence of agent SQL policy validation does not establish acceptance of audit row #324.

## Authorized scope

The user authorized **validation-only SQL contract work**. That authorization permits documenting source-grounded validation boundaries and blockers only; it does not authorize implementing or migrating persistence SQL.

## Explicitly excluded work

The following work is excluded and was not performed:

- SQL execution
- Persistence changes
- H2 changes
- Repository changes
- Database-tool registration
- Production wiring
- Broad service changes
- Editing Saturn source or tests
- Application-code implementation of `SqlUtil` constants

Unrelated dirty or untracked target files must remain preserved.

## Authorization required to proceed later

A broader, explicit authorization would be required to migrate or reproduce `SqlUtil`: authorization for persistence/database-contract changes in the target repository/H2 layer, any required production wiring and service integration, and permission to modify or extend the relevant Saturn-side/source-grounding tests and contracts. Validation-only authorization is insufficient.

## Required architecture and QA gates if scope expands

Before any expanded implementation is accepted, the work must at minimum:

- establish the owning target API and a source-grounded mapping for all 31 constants (or explicitly justify a narrower subset);
- document how each constant maps to repository/H2 responsibilities without duplicating or silently changing database contracts;
- define compatibility, transaction, error, parameterization, and lifecycle semantics at the architecture boundary;
- add focused tests for the chosen target contract and repository/H2 behavior, plus relevant integration/regression coverage;
- run the applicable Go and Saturn test suites and static/format checks;
- review production wiring, database-tool registration, and service impact if those become in scope; and
- obtain explicit review/acceptance of the architecture and QA evidence before claiming the audit row is complete.

## Disposition

**Do not claim audit row #324 accepted.** This artifact records a blocked diagnostic only. No application code was written, no Saturn files were edited, and no persistence migration was performed.
