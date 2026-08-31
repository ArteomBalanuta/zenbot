# Migration Plan Consolidation and Architecture Report

## Executive status

The migration is **NOT COMPLETE**. `MIGRATION_PLAN.md` remains the main strict-parity plan. `.hermes/migration-audit.md` contains 325 rows and currently reports 2 implemented, 10 intentional adaptations, and 313 needing implementation; that accounting predates and does not fully reflect accepted work.

Two accepted slices are source- and gate-backed:

- **Row #56 — `AgentMentionParser`**: implementation exists in `internal/agent/participation/policies.go` with tests; the corresponding Saturn implementation and tests are in `src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`; focused and full tests passed; an acceptance handoff exists.
- **SQL Group B**: repository and runtime parity handoffs exist. Implementation includes `internal/command/dispatch_adapter.go`, `handlers.go`, `identity_commands.go`, `mail_notes.go`, `users_nicks.go`, `remove.go`, `runtime_parity_red_test.go`, `internal/repository/sql_util_group_b.go`, `internal/repository/h2/sql_util_group_b.go`, `internal/repository/h2/sql_util_row324_group_b_test.go`, and `internal/service/services.go`. Focused, full, race, vet, build, and Make gates passed, and a mail-escaping regression was fixed.

Group C, row #325, and most migration rows remain pending. The accepted slices must be reconciled into the planning and audit views without weakening strict parity or treating an accepted handoff as completion of unrelated rows.

## OBSERVED

1. `MIGRATION_PLAN.md` is the authoritative strict-parity planning surface and is protected during this report phase.
2. `.hermes/migration-audit.md` is the row-level audit surface with 325 rows and a stale/incomplete aggregate relative to accepted work.
3. Row #56 has implementation, tests, focused/full verification, a Saturn counterpart, and an acceptance handoff.
4. SQL Group B has implementation across command dispatch/handlers, identity and mail/nick/remove command paths, repository SQL helpers including H2 coverage, and service wiring; repository and runtime parity handoffs exist.
5. SQL Group B verification includes focused/full/race/vet/build/Make gates, with mail escaping regression coverage/fix.
6. The protected plan and audit files are not to be modified in this report phase. Saturn remains read-only.

## TEST-BACKED

The accepted evidence supports the following status claims:

- **AgentMentionParser / row #56**: focused tests and the full test suite passed; acceptance handoff recorded.
- **SQL Group B**: focused tests, full tests, race tests, `go vet`, build, and Make gates passed; H2 row-324 coverage is present; repository/runtime parity handoffs recorded; mail escaping regression was fixed.

These results establish acceptance for the named slices only. They do not establish migration-wide completion, Group C completion, row #325 completion, or completion of the remaining subsystems.

## RECOMMENDED precise changes to `MIGRATION_PLAN.md`

Apply the following changes in a subsequent controlled plan-edit phase; do not edit the protected file during this report phase:

1. Add a dated **Consolidation / accepted work** status block near the plan's migration-status summary.
2. Mark **row #56 `AgentMentionParser`** as accepted/implemented only where the plan row explicitly corresponds to that scope, linking the implementation/test paths, Saturn read-only reference, focused/full test result, and acceptance handoff.
3. Mark **SQL Group B** as accepted only for its enumerated command, repository/H2, and service-runtime scope, linking both parity handoffs and the complete implementation-file list.
4. Record the verification gates for SQL Group B exactly: focused, full, race, vet, build, and Make; retain the mail-escaping regression/fix note.
5. Keep **Group C**, **row #325**, and all unrelated rows pending unless separately supported by equivalent evidence.
6. Preserve strict-parity semantics: acceptance of an adaptation or slice must not silently close its Saturn counterpart, neighboring rows, or unverified behavior.
7. Add explicit dependencies and next-slice ordering for the remaining subsystems listed below.
8. Add a reconciliation note stating that aggregate counts must be recomputed from row-level evidence rather than hand-edited.

Every row update requires source-grounded **architecture, implementation, QA, and acceptance evidence**. A plan edit should identify each evidence artifact and the exact scope it proves.

## RECOMMENDED safe audit reconciliation

Apply this only in a later audit-reconciliation phase; `.hermes/migration-audit.md` is protected during the current report phase:

1. Snapshot the current audit and plan before editing.
2. Reconcile row-by-row, not by changing the aggregate totals directly.
3. For row #56, attach the implementation paths, tests, Saturn reference, focused/full results, and acceptance handoff; change its classification only after verifying that the row's defined scope matches the evidence.
4. For SQL Group B, reconcile only the rows covered by the enumerated files and handoffs. Preserve separate status for repository parity, runtime wiring, H2 behavior, command behavior, and regression coverage where the audit models them separately.
5. Do not infer completion for Group C, row #325, or neighboring/unlisted rows.
6. Recompute totals from the reconciled row classifications and record the prior aggregate (2 / 10 / 313) as historical context, noting that it predates/does not fully reflect accepted work.
7. Validate that every changed row has architecture, implementation, QA, and acceptance citations, and that no Saturn source was modified.
8. Run the applicable focused/full/race/vet/build/Make gates for any newly changed implementation; record actual outcomes, not expected outcomes.
9. Review the resulting diff for accidental changes outside the audit rows and evidence links.

## Current-state matrix

| Area | Current status | Evidence / boundary |
|---|---|---|
| Main plan | Authoritative, protected in this phase | `MIGRATION_PLAN.md`; strict-parity plan |
| Audit | 325 rows; aggregate stale/incomplete | `.hermes/migration-audit.md`; 2 implemented, 10 intentional adaptations, 313 needing implementation; historical count |
| AgentMentionParser / row #56 | Accepted slice | Go implementation/tests, Saturn reference/tests, focused/full tests, acceptance handoff |
| SQL Group B | Accepted slice | Repository/runtime handoffs; enumerated command/repository/H2/service files; focused/full/race/vet/build/Make gates; mail escaping fix |
| SQL Group C | Pending | Not covered by the accepted Group B evidence |
| Row #325 | Pending | No accepted evidence supplied here |
| Overall migration | Not complete | Most rows and multiple subsystems remain pending |
| Saturn source | Read-only | No Saturn modifications in this phase |

## Remaining work by subsystems

### Agent API, config, memory, and routing

Complete and verify parity for the remaining agent-facing APIs, configuration contracts, memory behavior, routing rules, lifecycle semantics, error handling, and integration boundaries. Separate intentional adaptations from missing implementations, and preserve evidence for both architecture and runtime behavior.

### Commands

Continue beyond the accepted SQL Group B command scope. Reconcile remaining command families, dispatch paths, identity and mail/nick/remove behavior not covered by Group B, validation, help/usage, errors, and parity-sensitive side effects.

### Persistence / H2

Complete SQL Group C and remaining persistence rows, including schema/query behavior, transactions, null and ordering semantics, migrations, H2 compatibility, and repository/service integration. Row #325 remains pending until independently evidenced.

### Providers, listeners, and transport

Implement and verify outstanding provider adapters, listener lifecycles, event delivery, transport boundaries, retries/timeouts, shutdown behavior, and parity of externally visible behavior. Include runtime and integration evidence, not only compile-time coverage.

### Resources

Reconcile remaining resource files and loading behavior, including classpath/layout parity, defaults, templates, localization or message resources, and failure behavior when resources are absent or malformed.

### Tests and docs

Add focused tests for each slice, full-suite coverage where behavior crosses boundaries, race checks for concurrent code, and vet/build/Make gates as applicable. Update architecture and acceptance documentation with source-grounded links. Keep the plan and audit synchronized only through reviewed row-level reconciliation.

## Prioritized next slices

1. **Reconcile accepted work safely**: prepare reviewed plan/audit edits for row #56 and SQL Group B, without modifying them in this report phase.
2. **SQL Group C**: map its rows to architecture, implementation, QA, and acceptance evidence; implement the smallest dependency-complete slice.
3. **Row #325**: investigate and close only after full source-grounded evidence and applicable gates exist.
4. **Persistence/H2 follow-through**: finish dependencies exposed by Group B and Group C, including runtime wiring and regression coverage.
5. **Agent API/config/memory/routing**: prioritize contracts that unblock commands, providers, and listeners.
6. **Providers/listeners/transport**: implement boundary behavior with integration and concurrency verification.
7. **Resources, tests, and docs**: close cross-cutting parity gaps and preserve reproducible acceptance evidence.

## Limitations

This report is limited to the verified facts supplied for the accepted slices and the stated current counts. It does not claim a fresh row-by-row inspection, does not recalculate the audit totals, and does not declare any unlisted row complete. `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` must remain unmodified during this report phase. Saturn is read-only. All future status changes require source-grounded architecture, implementation, QA, and acceptance evidence for the exact row scope being changed.
