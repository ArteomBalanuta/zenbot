# Next Migration Slice — Deep Diagnostic

## Verdict: BLOCKED — no genuinely safe pending row proven

The metadata scan is frozen at the user's direction. No application source or Saturn source was modified, and no migration-completion claim is made. This artifact records a blocker rather than selecting an unsupported implementation slice.

## Authoritative boundary applied

- Plan: `MIGRATION_PLAN.md` (Saturn source `/Users/ab/workspace/projects/saturn`, branch `develop`, read-only; target `/Users/ab/workspace/go-projects/zenbot`).
- Frozen inventory: `.hermes/migration-audit.md`, which records 325 Java source-unit rows and remains **NOT COMPLETE**.
- Already accepted and therefore not eligible for reopening: identity; DBZ; transport/lifecycle/replica/shared integration; agent API/config/LLM/OpenAI; exact agent contracts; routing/participation/assembly; Stage A/B tools; turn/freshness; SQL policy rows #86–#90; utility rows #320–#323; and row #324 Group A only.
- Explicitly unavailable: row #324 Groups B/C; row #325 `Util`; stale row #56 `AgentMentionParser`; internal/agent/sql policy; unrelated registration; broad command/listener/service/provider/transport/remote-room/Whiskey work.

## Candidate review outcome

No remaining row satisfies all required properties simultaneously:

1. still pending without reopening an accepted slice;
2. not #325, #56, or the blocked/excluded portions of #324;
3. narrow and isolated enough for a bounded migration slice;
4. grounded in an exact Saturn source path and a concrete Zenbot target package/path;
5. supported by exact focused behavioral evidence, preferably a Saturn test path; and
6. free of broad command, listener, service, persistence/SQL, provider, transport, remote-room, Whiskey, or agent-room integration.

The only obvious pure utility candidates are not available under the supplied boundary: rows #320–#323 are accepted, while #324/#325 are explicitly blocked or excluded. The remaining model/DTO, command, listener, service, facade, and agent rows either lack a sufficiently concrete bounded target under the frozen metadata or require one of the excluded/broad subsystem dependencies. The frozen audit's generic text (`Add/run parity test for <Class>`) is not exact focused behavioral evidence or a Saturn test path.

## Precise blocker

The frozen audit identifies pending rows and broad target alternatives, but it does not authorize a new eligible slice with the required combination of exact source evidence, exact focused test evidence, a concrete narrow target owner, and bounded prerequisites. Selecting any row now would either duplicate accepted work, broaden into an excluded subsystem, or infer a contract/target from insufficient metadata.

## Minimum authorization needed

Provide an authoritative re-selection (audit update or handoff) naming **one** still-pending eligible row with all of the following:

- exact audit row number and verified Saturn source path;
- exact Saturn focused test path and the behavioral cases it proves (or an explicitly verified equivalent test location);
- one concrete Zenbot target package and file/path, not an unbounded `internal/**`, `internal/agent/**`, `internal/model/**`, or `internal/service/**` glob;
- explicit slice scope, complexity, prerequisites, and exclusions;
- confirmation that implementation does not reopen accepted work or enter #324 Groups B/C, #325, #56, internal/agent/sql policy, or any broad/excluded subsystem.

Until that authorization exists, the safe next action is diagnostic/selection work only. No implementation should begin from the current frozen metadata.

## Preservation / verification

- Created only this handoff artifact: `.hermes/handoffs/next-migration-slice-deep-diagnostic.md`.
- Application source and Saturn source were left untouched.
- Migration remains **NOT COMPLETE**.
