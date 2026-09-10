# Next Migration Slice Diagnostic 5

## Verdict: BLOCKED — no safe source-grounded pending row is available

This is a metadata-first diagnostic only. I read the authoritative `MIGRATION_PLAN.md`, the frozen `.hermes/migration-audit.md`, and the relevant accepted/blocked handoffs. No application or Saturn source was inspected or modified, and no migration-completion claim is made.

The accepted boundary supplied for this task already covers identity, DBZ, transport/lifecycle/replica/shared integration, agent contracts/LLM/OpenAI, routing/assembly, tools, turn/freshness, SQL-policy rows **#86–#90**, utility rows **#320–#323**, and only the bounded nine-constant Group A subset of row **#324**. The Group B decision explicitly blocks five further `SqlUtil` constants; Group C remains excluded. Row **#325** and stale row **#56** are excluded by instruction.

After applying those boundaries, no remaining audit row can be selected safely without either reopening accepted work, entering a blocked/excluded row, or broadening into an unverified subsystem. Therefore this artifact records an explicit blocker rather than guessing a next slice.

## Authoritative metadata reviewed

- Plan: `MIGRATION_PLAN.md`
  - Source is `/Users/ab/workspace/projects/saturn` (`develop`, read-only).
  - Target is `/Users/ab/workspace/go-projects/zenbot`.
  - The plan states the overall verdict is **NOT COMPLETE** and requires row-level evidence rather than filename/class-count parity.
- Frozen audit: `.hermes/migration-audit.md`
  - 325 Java source-unit rows; row-level source paths and target alternatives are authoritative.
  - The audit itself remains **NOT COMPLETE**.
- Relevant handoffs compared:
  - `.hermes/handoffs/final-migration-qa.md` — shared integration checkpoint PASS only; overall migration remains NOT COMPLETE.
  - `.hermes/handoffs/agent-utility-contracts-qa.md` — bounded rows #320–#322 PASS.
  - `.hermes/handoffs/separator-formatter-qa.md` — bounded row #323 PASS.
  - `.hermes/handoffs/sql-util-group-a-qa.md` — Group A subset PASS; full row #324 PARTIAL / NOT ACCEPTED.
  - `.hermes/handoffs/sql-util-group-b-decision.md` — Group B remains blocked; Group C and row #325 remain excluded.
  - `.hermes/handoffs/next-migration-slice-diagnostic-4.md` — prior metadata-only diagnostic also recorded no safe candidate under its acceptance boundary.

## Explicitly ruled-out rows

| Audit row | Saturn source path | Frozen target alternative | Why it cannot be the next slice |
|---:|---|---|---|
| **#324 `SqlUtil`** | `src/main/java/org/saturn/app/util/SqlUtil.java` | `internal/model/** or internal/service/** (new/extend)` | Full row is not accepted. Group A is bounded and already handled; Group B is explicitly blocked; Group C is excluded. The row owns SQL-string inventory and would broaden into repository/persistence/contract decisions. |
| **#325 `Util`** | `src/main/java/org/saturn/app/util/Util.java` | `internal/model/** or internal/service/** (new/extend)` | Explicitly excluded/ambiguous. Its broad utility scope and SQL-associated metadata do not define a small target package or contract. Selecting it would broaden scope without authoritative selection. |
| **#56 `AgentMentionParser`** | `src/main/java/org/saturn/app/agent/room/AgentMentionParser.java` | `internal/agent/** (new/extend)` | Historical direction is stale and invalid. It must not be resumed without a new authoritative re-selection; the broad agent-room boundary is also outside this metadata-only safe slice. |

## Remaining-row assessment

The frozen audit still labels many rows as `needs implementation`, but that status is not sufficient by itself to select work. The rows preceding the utility tail belong to agent API/config/moderation/persistence/room/routing/tool/turn, command, facade/transport, listener/snapshot, model/DTO, and service families. Under the supplied context, their corresponding bounded slices are accepted where stated, while the unresolved remainder requires broad subsystem implementation and focused source/test evidence. The audit's generic verification text does not, by itself, identify a new narrow pending contract, exact Saturn test path, or a non-broad target owner.

Consequently, no row can be promoted from the frozen inventory as the “next safe” row while also satisfying all of these constraints:

1. not row #324 blocked work;
2. not row #325;
3. not stale row #56;
4. not duplicating an accepted slice;
5. not broadening excluded command, listener, service, persistence/SQL, agent-room, provider, remote-room, or Whiskey subsystems; and
6. having authoritative source path, target package, bounded scope, and prerequisites grounded in the metadata/handoffs.

## Candidate-shape requirements for an unblocker

No candidate is selected, so there is no legitimate target package or complexity assignment for a new slice. An authoritative re-selection is required to supply all of the following before implementation planning:

- **Audit row number and Saturn source path:** exact row from `.hermes/migration-audit.md` and verified repository-relative source path.
- **Target package:** a concrete narrow package/path, not `internal/**`, `internal/agent/**`, `internal/model/**`, or `internal/service/**` as an unbounded glob.
- **Scope:** one cohesive contract or utility boundary, with explicit exclusions and no caller/wiring expansion.
- **Complexity:** low/medium/high with a reason tied to parsing, persistence, security, concurrency, external providers, or integration.
- **Prerequisites:** exact Saturn focused-test path or other authoritative behavioral evidence; target contract decision; and any accepted dependency gates.
- **Acceptance gates:** focused parity tests, required H2/security checks where applicable, independent QA, and dirty-worktree/Saturn preservation.

## Recommendation

**Do not start another implementation slice from the current metadata.** Preserve the current accepted boundary and leave the migration verdict **NOT COMPLETE**. Request an authoritative handoff or audit update that names one still-pending eligible row with an exact source path, concrete target package, bounded scope, complexity, prerequisites, and confirmation that it does not reopen #324 Group B/C, #325, stale #56, or any excluded subsystem.

If no such re-selection is provided, the next safe action remains **blocked diagnostic work only**, not application changes. Any future row selection must be re-evaluated against the frozen audit and the latest acceptance/blocked handoffs before implementation.

## Preservation and verification notes

- No application/Saturn files were modified by this diagnostic.
- No broad implementation code was inspected.
- No target gap, parity completion, or migration completion is asserted.
- The output path for this diagnostic is `/Users/ab/workspace/go-projects/zenbot/.hermes/handoffs/next-migration-slice-diagnostic-5.md`.
