# Group B — SQL utility contract decision record

**Decision:** Conservative default — preserve existing Zenbot contracts.

**Status:** Decision recorded; Group B remains blocked. No source changes were made for this decision record.

## Scope

This record covers only the five Group B Saturn `SqlUtil` constants:

- `DELETE_TRIP_NAMES`
- `DELETE_TRIP`
- `DELETE_NAME`
- `SELECT_NAME_TRIP_REGISTERED`
- `SELECT_LAST_N_MESSAGES`

Group C, row **#325**, `internal/agent/sql`, and unrelated subsystems remain excluded.

## Decision and implications

The user did not provide an alternative contract choice. Therefore, the conservative default applies: **existing Zenbot contracts are preserved**. Saturn semantics must not be silently adapted into existing interfaces, result shapes, ordering, filtering, visibility, or authorization behavior.

All five Group B constants remain **blocked**. No Saturn-compatible implementation is authorized by this record. Any implementation of Saturn-compatible delete behavior, result shapes, ordering, or filtering requires a **separately authorized compatibility slice** with an explicit target contract, exact SQL evidence, contract tests, real-H2 verification, and independent QA.

This record does **not** constitute full row #324 acceptance and does **not** claim migration completion. The previously recorded bounded Group A QA PASS remains bounded to Group A.

## Why each constant remains blocked

### `DELETE_TRIP_NAMES`

- Exact Saturn SQL still requires transcription from collected/source evidence; it must not be invented.
- Saturn deletes a trip/name identity scope, but Zenbot has no confirmed exact authorized identity-delete contract to map to.
- Scope, authorization and precondition behavior, affected-row semantics, and transaction boundaries are therefore unresolved.

### `DELETE_TRIP`

- Exact Saturn SQL still requires transcription from collected/source evidence; it must not be invented.
- Zenbot has no confirmed trip-scoped delete contract, including behavior for duplicate names, absent trips, or mixed ownership.
- The target scope, authorization, absent-row/no-op or error behavior, and atomicity requirements must be decided before implementation.

### `DELETE_NAME`

- Exact Saturn SQL still requires transcription from collected/source evidence; it must not be invented.
- Zenbot has no confirmed name-scoped delete contract.
- It is unresolved whether deletion is permitted, whether scope is global or trip-qualified, and how authorization, same-name collisions, absent rows, affected rows, and atomicity should behave.

### `SELECT_NAME_TRIP_REGISTERED`

- Exact Saturn SQL still requires transcription from collected/source evidence; it must not be invented.
- The known Saturn and Zenbot contracts differ: Saturn returns `Name,Trip` and orders by `trip DESC`, while Zenbot `RegisteredUsers` returns `Trip,Name` and orders by `name DESC`.
- Preserving Zenbot means no positional result swapping or silent Saturn ordering/projection substitution. A compatibility choice and caller/test impact assessment are required before any change.

### `SELECT_LAST_N_MESSAGES`

- Exact Saturn SQL still requires transcription from collected/source evidence; it must not be invented.
- Zenbot returns a richer `model.Message`, filters `PUBLIC`, and applies an `id DESC` tie-break; equivalence to Saturn's exact projection, visibility, limit, and ordering contract is unconfirmed.
- No silent filtering, enrichment, reordering, or result-shape adaptation is permitted. The approved contract must explicitly define visibility, projection, limit, primary ordering, and tie behavior.

## Next prerequisites

Before any Group B implementation is considered:

1. Authorize a separate, bounded compatibility slice for one or more named constants; this decision record alone is not implementation authorization.
2. Transcribe and preserve the exact Saturn SQL from reliable collected/source evidence for each authorized constant; do not guess SQL.
3. Record the target contract explicitly, including interface/result shape, scope, authorization, visibility/filtering, ordering and tie policy, limit, absent-row behavior, affected-row semantics, and transaction/atomicity requirements as applicable.
4. Write contract tests first (RED), including positive, empty, boundary, collision/scope, authorization, ordering/tie, visibility, and atomicity cases relevant to the approved contract.
5. Implement only the smallest explicitly approved mapping, without positional column swapping or silent semantic adaptation.
6. Run the required real-H2 verification; mocks alone are insufficient.
7. Obtain independent QA sign-off covering SQL mapping, authorization boundaries, projections/result shapes, ordering, filtering/visibility, limits, and real-H2 assertions.
8. Reassess row #324 only after all authorized slices pass their evidence, test, and QA gates; until then its status remains partial/not accepted.

**Recorded outcome:** Preserve existing Zenbot contracts; leave all five Group B constants blocked; make no source changes; defer any Saturn-compatible behavior to separately authorized work.
