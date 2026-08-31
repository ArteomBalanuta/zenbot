# Saturn Migration Unblock Analysis

## Decision statement

[OBSERVED] The authoritative migration status remains **NOT COMPLETE** in `MIGRATION_PLAN.md` and `.hermes/migration-audit.md`. The accepted baseline already covers identity, DBZ, transport/lifecycle/replica/shared integration, agent API/contracts/LLM/OpenAI, routing/assembly, tools, turn/freshness, SQL policy #86–#90, utilities #320–#323, and row #324 Group A.

[OBSERVED] Row #324 Group B is blocked by an explicit decision to preserve Zenbot contracts. Group C and row #325 are excluded. Row #56 `AgentMentionParser` is stale rather than an automatically actionable pending slice.

[RECOMMENDED] The real unblocker is an **authoritative bounded-slice decision plus prerequisite contract ownership**—not another generic audit scan. A scan can rediscover status, but it cannot decide whether the preserved Zenbot contract may be reopened, identify the exact target files, or assign the owner accountable for the contract and its tests.

## Evidence and limitations

[TEST-BACKED] Saturn has focused tests for `AgentMentionParserTest.java`, `AgentGapContractRedTest.java`, `OutServiceTest.java`, `DateUtilTest.java`, `IdentityUtilTest.java`, `JsonPayloadsTest.java`, and `LastMessagesCommandImplTest.java`, in addition to broader subsystem tests. These test names establish candidate verification surfaces; they do not by themselves authorize a migration slice or prove Zenbot parity.

[OBSERVED] Saturn `PingService`/`OutService` are coupled to sockets, queues, and command/output paths. Zenbot exposes `common.Engine` methods `SendRawMessage`, `SendChatMessage`, `SendWhisperMessage`, and `SendAddressedMessage`, plus `internal/command` `commandBase`/handlers and core engine integration.

[LIMITATION] No proven isolated Zenbot Ping/Out owner is established by the supplied evidence. Existing target-file lookup for `service_commands.go` failed, so it is not an established target and must not be cited as one. `Constants.java` was found, but no `ConstantsTest.java` was found; without an exact audit row, target, and test evidence, Constants is not selectable.

[LIMITATION] Saturn's working tree had unrelated pre-existing modifications, and the target is dirty with unrelated changes. This analysis therefore does not claim a clean baseline, implementation, or completed verification.

## Candidate paths

### 1. Dedicated agent-room contract slice — strongest candidate, conditional

[RECOMMENDED] Select this only if the user explicitly authorizes reopening the stale #56 boundary for `AgentMentionParser`. The authorization must name the exact target files, the preserved Zenbot contract to be honored, and the focused test ownership. The slice must remain contract-focused: **no full wiring**.

[TEST-BACKED] The relevant Saturn-focused evidence is `AgentMentionParserTest.java` and `AgentGapContractRedTest.java`. They are useful starting points for defining the contract and its negative/gap behavior, but the migration cannot be inferred from test names alone.

[EXPLICIT EXCLUSION] Do not silently treat stale row #56 as current approval, and do not expand this slice into routing, full engine wiring, or unrelated accepted areas.

### 2. Ping/Out — broader slice, unsafe in current scope

[OBSERVED] `PingService`/`OutService` cross sockets, queues, and command/output paths. The corresponding Zenbot surface spans `common.Engine`, `internal/command` `commandBase`/handlers, and core engine integration.

[RECOMMENDED] Treat Ping/Out only as a deliberately broader command/output/network slice with an explicitly named owner, contract boundary, target files, and integration-test plan. It is **not safe in the current scope** because no isolated Zenbot owner is proven and the dependency surface is wider than a bounded audit-row fix.

[EXPLICIT EXCLUSION] Do not select Ping/Out merely because `OutServiceTest.java` exists; a focused Saturn test does not remove the cross-layer dependency risk.

### 3. Constants — not selectable on current evidence

[LIMITATION] Constants has no selectable migration basis in the supplied evidence: `Constants.java` exists, but no `ConstantsTest.java` exists, and no exact audit row/target/test mapping is established.

[RECOMMENDED] Keep Constants out of the pending-slice choice until an authoritative audit row identifies the target, contract owner, and test evidence.

## Dependency graph

```text
Authoritative bounded-slice decision
        |
        +--> explicit boundary reopening (only for stale #56, if chosen)
        |        |
        |        +--> exact target files
        |        +--> preserved Zenbot contract
        |        +--> contract/test owner
        |                 |
        |                 +--> architecture
        |                          |
        |                          +--> implementation
        |                                   |
        |                                   +--> focused tests + independent QA
        |
        +--> broader Ping/Out decision (separate option)
                 |
                 +--> sockets + queues + command/output paths
                          |
                          +--> named owner and integration scope
```

[OBSERVED] The accepted baseline is a prerequisite input to every branch. No candidate should reopen accepted slices, excluded Group C/row #325, or the preserved-contract decision without a new authoritative decision.

## Staged unblock plan

1. **Freeze the accepted baseline.**
   [RECOMMENDED] Record the accepted slices as fixed: identity, DBZ, transport/lifecycle/replica/shared integration, agent API/contracts/LLM/OpenAI, routing/assembly, tools, turn/freshness, SQL policy #86–#90, utilities #320–#323, and row #324 Group A. Mark Group B as blocked, Group C and row #325 as excluded, and #56 as stale pending explicit authorization.

2. **Choose one pending row and its target owner.**
   [RECOMMENDED] The decision-maker must choose either a newly authorized dedicated agent-room contract slice or a separately authorized broader Ping/Out slice. For the chosen row, name the exact target files, contract owner, test owner, and acceptance boundary. Do not choose Constants absent exact audit row/target/test evidence.

3. **Approve architecture before implementation.**
   [RECOMMENDED] For agent-room, define the narrow contract and gap behavior using the relevant focused Saturn tests as evidence, while explicitly excluding full wiring. For Ping/Out, map the sockets, queues, command/output, and core engine dependencies before any code change. The architecture approval must state what remains untouched.

4. **Implement only the approved bounded slice.**
   [RECOMMENDED] Keep changes limited to the named target files and contract. Do not use the task to reconcile unrelated dirty-tree changes or to advance excluded/accepted areas.

5. **Run independent QA.**
   [RECOMMENDED] Have an independent reviewer verify contract preservation, exact scope, focused tests, and regressions against the frozen baseline. A passing focused test is necessary but not sufficient for broader Ping/Out coupling; independent QA must check the declared dependency boundary.

6. **Update authoritative status only after evidence.**
   [RECOMMENDED] Reconcile `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` only after the chosen slice has an owner, implementation evidence, and independent QA. Until then, status remains NOT COMPLETE.

## Minimum authorization required

[RECOMMENDED] Before work begins, obtain one explicit decision containing:

- the single selected row/slice;
- whether the stale #56 boundary is being reopened;
- exact target files (not inferred filenames);
- the Zenbot contract to preserve and its owner;
- the test/QA owner and acceptance evidence;
- the explicit scope limit, including whether full wiring is forbidden.

For the dedicated agent-room option, the minimum is explicit authorization to reopen stale #56, exact target files, and a no-full-wiring constraint. For Ping/Out, the minimum additionally includes authorization for the broader command/output/network dependency surface and its integration ownership.

## Explicit exclusions

[EXPLICIT EXCLUSION] No generic audit rescan as a substitute for the decision.

[EXPLICIT EXCLUSION] No implementation claim or completion claim in this handoff.

[EXPLICIT EXCLUSION] No reopening of row #324 Group B's preserved Zenbot-contract decision without explicit authorization.

[EXPLICIT EXCLUSION] No Group C or row #325 work.

[EXPLICIT EXCLUSION] No automatic selection of stale #56, Ping/Out, or Constants based only on names of existing tests/files.

[EXPLICIT EXCLUSION] No citation of `service_commands.go` as an existing target.

## Unblock conclusion

[RECOMMENDED] Stop scanning for a generic next task. Freeze the accepted baseline, obtain the authoritative bounded-slice decision, assign prerequisite contract ownership, and only then proceed through architecture, implementation, and independent QA. The dedicated agent-room contract slice is the strongest candidate **only with explicit reopening of stale #56, exact target files, and no full wiring**. Ping/Out remains a broader command/output/network slice and is unsafe in the current scope. Constants is not selectable without exact audit row/target/test evidence. The migration remains NOT COMPLETE; this document does not claim completion.
