# SqlUtil Corrective Architecture Handoff

- **Row:** #324
- **Status:** corrective architecture
- **Complexity:** HIGH
- **Scope:** H2/repository/persistence work on affected existing Zenbot paths only
- **Acceptance:** the row remains **unaccepted** until every supported mapping has task-owned tests and every unsupported mapping is documented.

## Source-of-truth rule

Saturn has 31 exact public constants. During implementation, transcribe every constant directly from the Saturn source into a task-owned mapping ledger. Do **not** invent, normalize, rename, or reconstruct constant names or SQL in this handoff. The ledger must contain exactly 31 source-transcribed entries, one and only one of Groups A, B, or C, with source location, caller, target contract, and test/documentation disposition.

## Required partition

### Group A — supported by existing target contracts

Assign a source constant to Group A only when its exact behavior is already represented by the target contract and can reuse the existing Zenbot H2 implementation:

- role/trip authorization;
- identity registration, including the represented registration-by-trip behavior;
- command audit;
- message audit;
- nicks queries;
- registered-user queries;
- last-message queries, only where the exact result and ordering behavior are already represented.

Reuse-first target files:

- `identity.go` — registration paths;
- `authorization.go` — trip/role authorization;
- `audit.go` — command/message audit;
- `user_queries.go` — nicks, registered users, and exact last-message query behavior;
- related existing tests, extended with task-owned coverage rather than duplicating production logic.

A Group A mapping is not accepted merely because the SQL or schema looks similar: its method contract, parameters, generated-key behavior, nullability, ordering, transaction boundary, and failure behavior must match the target contract.

### Group B — shape-mismatched; explicit interface and test changes required

Assign a source constant to Group B when the operation is in an affected existing path but its interface, parameters, result shape, cardinality, null semantics, ordering, or lifecycle does not match the target contract. The implementation must first transcribe the exact Saturn source and then propose/implement explicit target interface and test changes for the affected shape. This includes, as applicable:

- mail operations;
- banned-user operations;
- notes operations;
- lounge operations;
- role/query result variants whose target result shape is not exact;
- any other affected repository operation that fails the Group A exact-contract test.

Do not silently coerce a Group B result into an existing type, discard fields, change cardinality, or substitute a nearby query. Each such mapping requires an explicit interface decision, task-owned tests for the new contract, and documentation of compatibility impact.

### Group C — blocked / no target contract

Assign a source constant to Group C when its caller is excluded from this task or no target contract exists. This includes listener-only and service-only paths, including excluded Saturn callers such as listener/service paths that are not covered by the authorized affected-existing-path scope. A Group C entry must preserve the direct Saturn source transcription and document:

- the caller/path that blocks migration;
- why no authorized target contract exists;
- the exact follow-up boundary or owner needed before work can resume.

Do not create speculative methods, SQL, adapters, or tests for Group C. A constant must not be forced into Group A or B solely to make the count close.

## Persistence and contract requirements

For every Group A mapping, and every Group B mapping after its explicit interface decision, add exact-H2 persistence tests against the matching Zenbot schema. Tests must verify, as applicable:

- transaction boundaries and commit visibility;
- rollback on statement or mapping failure, including absence of partial writes;
- generated-key retrieval and propagation;
- placeholder count, order, and bound values;
- null inputs, nullable columns, and null result behavior;
- row cardinality and duplicate handling;
- deterministic result ordering where the contract promises ordering;
- empty-result behavior;
- authorization/audit side effects and failure semantics;
- exact returned shape rather than only successful execution.

Use direct source transcription to derive the SQL/parameter mapping during implementation. Saturn and Zenbot H2 schemas are treated as matching, but schema similarity does not waive contract or persistence assertions.

## Delivery gates

1. **TDD gate:** write failing task-owned tests first for each proposed Group A mapping and each approved Group B interface change; implement only enough to pass; then refactor without weakening assertions.
2. **Mapping gate:** verify the ledger has exactly 31 entries, every entry is source-transcribed, and every entry has exactly one group and a disposition.
3. **Persistence gate:** run the exact-H2 tests, including transaction, rollback, generated-key, placeholder, null, and order cases relevant to each mapping.
4. **Independent QA gate:** an independent reviewer checks the ledger against the direct Saturn source, verifies target-contract reuse, inspects test ownership/coverage, and confirms no Group C path was implemented accidentally.
5. **Acceptance gate:** keep row #324 unaccepted until all Group A mappings have task-owned tests and all Group B/C mappings have explicit unsupported/shape-mismatch documentation.

## Explicit exclusions

- Row #325.
- Unrelated production registration work.
- Listener/service-only paths and other callers excluded from the authorized affected-existing-path scope.
- New speculative target contracts for constants with no target equivalent.
- Invented constant names, invented SQL, silent result-shape coercion, and schema-only claims of compatibility.

The implementation handoff is therefore: source-transcribe all 31 constants, partition them without inference into A/B/C using the criteria above, reuse existing target files first, make contract changes only for documented Group B mismatches, leave Group C blocked, and do not accept the row until the stated gates pass.
