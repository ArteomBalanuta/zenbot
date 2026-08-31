# Migration Plan Consolidation QA

## Verdict

**PASS WITH CORRECTIONS REQUIRED — documentation update gate is HOLD.**

The consolidation report is grounded for the two bounded accepted slices and its cited current Zenbot symbols and focused/full test evidence resolve. It correctly preserves the overall migration verdict as **NOT COMPLETE**. However, the proposed documentation update must be narrowed before editing: it must not imply that the complete SQL Group B scope or full audit row #324 is accepted, and the audit procedure must explicitly deduplicate the 325 primary row IDs rather than count handoffs, files, tests, or SQL evidence as additional rows.

No changes were made to `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, application source, or Saturn by this QA pass.

## Evidence verification

### AgentMentionParser / row #56 — PASS, bounded only

The claim is supported for the bounded parser contract by:

- `internal/agent/participation/policies.go`: `MentionParser.Parse` and `mentionWordRune` implement literal-`@`, case-insensitive matching, Unicode boundary checks, all-match removal, and cleanup.
- `internal/agent/participation/policies_test.go`: `TestMentionParserSaturnParity` and `TestMentionParserBoundariesAndCleanup` cover the cited behavior, including Unicode-number boundaries, mixed case, literal nick characters, multiple mentions, and punctuation cleanup.
- `.hermes/handoffs/agent-mention-parser-acceptance.md` and `.hermes/handoffs/agent-mention-parser-qa.md`: explicitly limit acceptance to the bounded `AgentMentionParser` contract and state that migration closure and unrelated rows remain incomplete.
- Focused verification rerun in the current checkout: `go test ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1` — **PASS**.

Correction/boundary: the plan update should record **bounded acceptance for row #56's evidenced parser scope**, not general agent-room integration, routing, listener wiring, or migration completion. Preserve the acceptance handoff's limitation language.

### SQL Utility Group B — PASS, bounded repository/runtime scope only

The implementation and runtime claims resolve against current source:

- `internal/repository/sql_util_group_b.go`: typed Group B result shapes and `SqlUtilGroupBRepository`, plus the narrow optional `SaturnAuthorizedDeleteRepository` capability.
- `internal/repository/h2/sql_util_group_b.go`: five accepted SQL constants, selector resolution, private authorization context, atomic delete transaction, Saturn-shaped registered-user and last-message reads.
- `internal/command/dispatch_adapter.go`: conditional `remove` registration through one canonical catalog definition and its aliases.
- `internal/command/handlers.go`: `newCommand` constructs `removeCommand`.
- `internal/command/remove.go`: trimmed selector, service call, success/failure replies.
- `internal/command/identity_commands.go`: Group B last-message routing with nil name, count cap/default forwarding, and explicit trip adaptation.
- `internal/command/users_nicks.go`: Group B registered-user routing with existing table formatting.
- `internal/command/mail_notes.go`: Group B directory lookup only for the unregistered-recipient branch; queue validation remains separate.
- `internal/service/services.go`: Group B service delegators and the optional authorized-delete bridge.
- `internal/command/runtime_parity_red_test.go` and `internal/repository/h2/sql_util_row324_group_b_test.go`: focused runtime, selector, transaction, result-shape, ordering/filtering, and no-delete coverage.

The cited handoffs agree on the bounded scope and exclusions: `.hermes/handoffs/sql-util-group-b-acceptance.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-acceptance.md`, `.hermes/handoffs/sql-util-group-b-runtime-parity-qa.md`, and the implementation handoff. Current-checkout verification returned **PASS** for:

- `go test ./internal/repository/h2 -run 'TestGroupB' -count=1`
- `go test ./internal/command -run 'TestRuntimeParity' -count=1`
- `go test ./... -count=1`
- `go test -race ./... -count=1` (macOS `LC_DYSYMTAB` linker warning observed; exit status 0)
- `go vet ./...`
- `go build ./...`
- `make test`, `make vet`, and `make build`

Correction/boundary: these results accept only the enumerated Group B repository/runtime behavior. They do **not** accept all SQL Group B work, full row #324 (`SqlUtil`), Group C, row #325 (`Util`), or any unrelated command/service/listener/agent work. The plan must say “bounded Group B sub-scope accepted” and retain **row #324: NOT COMPLETE** unless a separately defined row-level scope and evidence model is approved. The mail handoff limitation (no isolated command-level test driving the real unregistered-recipient branch through `Queue`) should remain visible rather than being converted into an unqualified end-to-end claim.

## 325-row audit reconciliation QA

The current audit's exhaustive Java mapping contains exactly **325 unique, contiguous primary row IDs 1–325**. Its status counts are **2 implemented + 10 intentional target adaptations + 313 needs implementation = 325**. The `package-info` unit is explicitly row #42, so it is not an extra row and must not be counted twice.

The proposed procedure is directionally safe but incomplete for duplicate prevention. The documentation update must add these rules:

1. Treat the exhaustive Java mapping's row ID (`1..325`) as the sole primary key for the class/unit ledger.
2. Validate exactly 325 rows, uniqueness, and contiguity before reconciliation; fail closed on duplicates, gaps, or out-of-range IDs.
3. Attach evidence artifacts as many-to-many citations to a primary row; do not count each handoff, source file, test, SQL constant, repository method, or Go file as another Java row.
4. Keep separate ledgers and totals for SQL tables (12), indexes (18), SQL occurrences (197), repository/service methods (88), and Zenbot Go files (71). Never add those inventories to the 325 Java-row total.
5. Deduplicate accepted evidence by `(row_id, evidence_scope)` and merge overlapping handoff citations; do not increment a status count once per handoff or once per file.
6. For partial row coverage such as Group B's five `SqlUtil` constants, record a bounded sub-scope while leaving the parent row's full status pending unless the audit schema explicitly supports and reports sub-row status.
7. Recompute status totals from the deduplicated row ledger, and assert that the sum of status counts remains exactly 325. Keep `2/10/313` as historical pre-reconciliation context, not as a hand-edited replacement.
8. Require architecture, implementation, QA, and acceptance citations for every status change, and keep Group C and row #325 pending absent independent evidence.

## Corrections to the consolidation report

1. Replace any unqualified reading of “SQL Group B accepted” with “bounded SQL Utility Group B repository/runtime sub-scope accepted.”
2. Explicitly state that **full audit row #324 remains NOT COMPLETE**; the five constants and selected runtime paths do not close all `SqlUtil` behavior.
3. Clarify row #56 as bounded parser-contract acceptance and retain exclusions for broader agent integration.
4. Preserve the mail runtime test limitation from the independent QA handoff.
5. Expand the audit-reconciliation procedure with the primary-key, separate-ledger, many-to-many evidence, and deduplication rules above.
6. Do not publish new aggregate counts until the row-level reconciliation is performed; if only row #56 changes status in a future approved ledger model, calculate the resulting total from that ledger and do not infer any change from Group B's sub-row evidence.

## Explicit documentation-update gate

**HOLD / NOT CLEARED.** `MIGRATION_PLAN.md` may be edited only after the editor:

- applies the six corrections above;
- labels both accepted areas as bounded sub-scopes;
- leaves Group C, row #324 full scope, row #325, and unrelated rows pending/not complete;
- adds the deduplicated 325-row reconciliation rules;
- cites the exact implementation, test, QA, and acceptance artifacts; and
- verifies the diff changes only the intended documentation, with Saturn and application source untouched.

After those checks, the controlled documentation edit may proceed. This QA report does not authorize migration closure.
