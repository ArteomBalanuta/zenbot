# Current migration plan vs. dirty worktree — architecture status comparison

**Repository:** `/Users/ab/workspace/go-projects/zenbot`  
**Assessment type:** read-only source/worktree comparison; no Go source was edited.  
**Plan authority:** `MIGRATION_PLAN.md`; the frozen ledger exists at `.hermes/migration-audit.md`.  
**Overall verdict:** **NOT COMPLETE.** The dirty worktree contains several independently QA-backed command/moderation verticals, but it does not satisfy the plan’s all-row closure criteria and semantic MODERATION ingress remains fail-closed.

## Evidence basis and current gate snapshot

### [OBSERVED] Worktree baseline

At assessment start, `git status --short` reported **22 modified tracked Go files** and **61 untracked files** (including handoffs, implementation, and tests). The tracked diff was **475 insertions / 86 deletions**. This assessment adds only this handoff; it does not claim ownership of the pre-existing application changes.

### [TEST-BACKED] Gates run against the current worktree

| Gate | Current result | Scope/limitation |
|---|---|---|
| `go test ./internal/command ./internal/core ./internal/repository/h2 ./internal/service ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -run 'Test(Prefix|Activity|LastOnline|LastSeen|ShadowBan|UnshadowBan|Mute|Unmute|Color|Flair|ManualSimpleModeration|AdminModeratorCatalog|Semantic|Moderation|LiveAgent)' -count=1` | PASS | `internal/agent/live` and `cmd/zenbot` reported **no tests to run** for that combined regexp; do not treat this invocation as focused live-agent proof. |
| `go test ./... -count=1` | PASS | Includes command, core, service, agent, and real-H2 packages; baseline health only, not migration closure. |
| `go build ./...` | PASS | Compilation only. |
| `git diff --check` | PASS | No whitespace error. |
| `go vet ./...` | FAIL | Sole output: `internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`. Historic vertical QA records the same diagnostic at line 95/96; the line drift makes the older location stale, but the blocker remains. |

The plan requires focused parity evidence, relevant package gates, and final `go test ./...` per rapid batch (`MIGRATION_PLAN.md:13-25`), while full closure additionally requires race, vet, format, H2/security, and ledger evidence (`MIGRATION_PLAN.md:213-258`). Therefore the current full-test/build passes do not close the migration.

## Accepted/completed bounded verticals

The labels below mean **accepted only within the stated bounded scope**, not that the corresponding frozen audit rows are closed. `.hermes/migration-audit.md` still marks the cited rows `needs implementation`, so its status text is stale relative to the newer handoffs but remains the plan-designated frozen inventory.

| Vertical | Worktree/source evidence | Acceptance evidence and gates | Status boundary |
|---|---|---|---|
| S2 manual simple moderator operations | Concrete typed operation/command paths in `internal/common/moderation_operations.go`, `internal/core/moderation_operations.go`, `internal/command/moderation_simple.go`, `ban.go`, `unban.go`, `unbanAll.go`, `lock.go`, `handlers.go`, and `dispatch_adapter.go`. | `.hermes/handoffs/admin-moderator-s2-qa.md:3-24` records focused command/core, command race, catalog, `go vet ./internal/command`, and diff-check PASS; the historic broad-suite failure was the then-stale S4 guard. | Accepted for `captcha`, `authorize/auth`, `deauthorize/deauth`, `lock/lockroom`, `overflow` aliases, `ban`, `unban`, and `unbanall/pardonall` only. |
| S3 active-target moderation | Concrete `mute`/`unmute`/`color`/`flair` cases exist in `internal/command/handlers.go`; implementation/test files are `internal/command/mute.go` and `mute_color_flair_test.go`. | `.hermes/handoffs/admin-moderator-s3-qa.md:3-40` records focused/race S3 and retained S2 gates PASS. | Accepted for manual command behavior only; it deliberately does not apply automatic semantic protected-principal policy. |
| S4 public shadow-ban | Concrete `shadowbanlist`, `shadowban`, and `unshadowban` cases are present in `internal/command/handlers.go`; vertical spans `internal/command/shadow_ban.go`, `internal/service/shadow_ban.go`, `internal/repository/h2/shadow_ban.go`, and their tests. | `.hermes/handoffs/admin-moderator-s4-qa.md:30-65` records focused command/race, real-H2, command package, full suite, command vet, and diff-check PASS. The current full suite also passes. | Accepted for list/single/offline/contains/all-delete command scope and source-shaped H2 behavior; not a semantic agent activation. |
| S5 moderator activity | Files exist as untracked worktree artifacts: `internal/command/activity.go`, `internal/service/activity.go`, `internal/repository/activity.go`, `internal/repository/h2/activity.go`, and tests; narrow wiring is in `handlers.go`, `dispatch_adapter.go`, `services.go`, and `engine_factory.go`. | `.hermes/handoffs/moderator-activity-qa.md:3-36` says **ACCEPTED** after focused command/service/real-H2/race/package/full suite and diff gates; current full suite passes. | Accepted only for `active/activity`, source-shaped query/rendering, and its explicit target cancellation guard. |
| ADMIN prefix | Concrete files are `internal/common/prefix.go`, `internal/core/prefix_state.go`, `internal/command/prefix.go`, and tests; `internal/command/admin_moderator_catalog_guard_test.go:117-121` no longer allowlists `prefix`. | `.hermes/handoffs/admin-prefix-qa.md:3-65` records focused/race core+command, full core/command race, full suite, command vet, and diff checks PASS. | Accepted only for volatile host + current managed-replica fan-out. No persistence, future replica inheritance, listener reordering, or lifecycle work. |
| User last-online | Dirty files include `internal/command/last_online.go`, its tests, H2/user query changes, service files, and listener test adjustment. | `.hermes/handoffs/last-online-qa.md:3-51` records focused H2/service/command/listener, package/full suite, build, and diff checks PASS. | Accepted for `lastonline/seen/last/online/lastseen`; persistence/rendering source evidence is derived where Saturn lacks focused tests (`last-online-qa.md:53-55`). |
| Semantic moderation Stage A and typed reversals | Candidate/scaffolding is in `internal/agent/participation/semantic_moderation.go`, invocation/live adapter changes, and `cmd/zenbot/main.go`; typed reversals are in `internal/core/moderation_target.go`, `moderation_reversal.go`, and H2 shadow-ban repository. | Stage-A QA is PASS/fail-closed (`.hermes/handoffs/rapid-agent-semantic-moderation-qa.md:1-95`). Reversal QA is PASS (`.hermes/handoffs/rapid-agent-moderation-reversals-qa.md:1-77`) with focused/race/full/build/diff gates. | Accepted only as disabled scaffolding plus typed `unmute`/`unshadowban` primitives. It is **not** live semantic ingress or literal generic `RunCommandTool` parity. |

## In-progress / still-pending verticals

### [OBSERVED] Admin/moderator command inventory

`internal/command/admin_moderator_catalog_guard_test.go:21-57` verifies the **11 admin + 23 moderator = 34** source catalog rows and exact aliases/roles. Its transition allowlist at `:113-121` proves eleven scoped canonicals remain generic:

- Admin: `mine`, `replica`, `replicaoff`, `replicastatus`, `restart`, `shutdown`, `sql`, `whiskey`.
- Moderator: `automove`, `nuke`, `resurrect`.

`internal/command/handlers.go:399` remains the generic `saturnCommand` fallback. Thus catalog parity and `go test ./...` do **not** establish behavioral parity for those eleven command verticals. The governing architecture maps them to S6–S10 in `.hermes/handoffs/admin-moderator-full-migration-architecture.md:105-121`.

### [OBSERVED] Semantic MODERATION activation remains disabled

- `internal/agent/participation/semantic_moderation.go:62-72` retains the six source aliases but returns `false` from `SemanticModerationIngressReady()`.
- `internal/agent/live/participation.go:16-30` still transports a string target from `c.Author.Name`, rather than the immutable typed `core.ModerationTarget` activation architecture requires.
- `.hermes/handoffs/rapid-agent-semantic-moderation-activation-architecture.md:5-9,99-117,159-184` requires a separate MODERATION-only closed action gateway/tool loop, typed identity capture, silent/no-persistence lifecycle, one-action bound, and an explicit Saturn nick/hash disposition before composition.

The six primitives now exist, but no reviewed `moderation_action` tool, private moderation gateway, moderation tool loop, runner selection, or typed activation transport appears in the dirty worktree. This is a **blocked, unactivated next vertical**, not an implementation failure of the accepted reversal slice.

### [OBSERVED] Migration-wide plan obligations remain pending

`MIGRATION_PLAN.md:249-258` requires all 325 units, 12 tables/18 indexes, 197 SQL occurrences, 88 methods, all command/listener/agent integration, H2-only closure, visibility security proof, and all gates. `.hermes/migration-audit.md:59-64` (agent moderation rows 36–41), `:174` (prefix row 151), `:188` (activity row 165), `:206/:210` (shadow-ban rows 183/187), and `:218` (last-online row 195) remain `needs implementation`. The plan says any status change requires architecture, implementation, QA, and acceptance evidence (`MIGRATION_PLAN.md:58-82`); these newer handoffs supply partial architecture/implementation/QA evidence, but no matching audit reconciliation/acceptance record changes those frozen rows.

## Explicit exclusions preserved by the worktree

- No Go source was modified by this assessment; no reset, clean, checkout, staging, commit, or Saturn source edit occurred.
- The accepted manual moderation verticals do not import automatic semantic-monitor protected-principal policy. This separation is required by `.hermes/handoffs/admin-moderator-full-migration-architecture.md:97-103`.
- Prefix is intentionally volatile: no config/schema persistence or future-replica propagation (`.hermes/handoffs/after-prefix-next-parity-architecture.md:103-108`; `.hermes/handoffs/admin-prefix-qa.md:80-85`).
- Semantic work excludes public `run_command`, `DispatchUserCommand`, human authorization, `Ban`/`Unban`, raw agent protocol construction, arbitrary arguments, and permanent bans (`rapid-agent-semantic-moderation-activation-architecture.md:199-204`).
- The rapid phase explicitly defers exhaustive SQL mapping/index/migration hardening and broad SQLite cleanup until command/agent live integration (`MIGRATION_PLAN.md:23-25`). Those deferrals are not closure waivers.

## Blockers

1. **Current repository-wide vet gate fails.** `go vet ./...` exits 1 with the `NewEngineImpl` copylock diagnostic cited above. Historic documents call it out-of-scope, but `MIGRATION_PLAN.md:213-235` makes vet a final mandatory gate. It blocks full migration acceptance, even though it did not block the bounded rapid vertical QA.
2. **Semantic activation requires an explicit source-owner/senior decision for Saturn’s nick/hash contradiction.** The activation architecture documents three allowed dispositions and mandates fail-closed behavior until selected (`rapid-agent-semantic-moderation-activation-architecture.md:99-117`).
3. **Lifecycle/admin SQL/Whiskey decisions remain unresolved.** The full admin/moderator architecture lists lifecycle D1, admin SQL D2, prefix/automove durability D3, manual protected-user D4, and Whiskey proxy D5 (`admin-moderator-full-migration-architecture.md:135-141`). Prefix’s bounded volatile decision is implemented, but `restart/shutdown/sql/whiskey` still cannot claim parity without the relevant decisions.
4. **The frozen audit has not been reconciled to bounded evidence.** It is intentionally still `NOT COMPLETE`, but rows now have newer bounded QA handoffs. Until an evidence-ledger reconciliation is performed, status counts cannot represent completed work.

## Conflicts and stale claims

### [OBSERVED]

- `internal/agent/participation/semantic_moderation.go:68-71` says `unmute` and `unshadowban` have no source-backed typed operations. That is stale: `internal/core/moderation_reversal.go` implements them and `.hermes/handoffs/rapid-agent-moderation-reversals-implementation.md:28-38` and its QA record their verified behavior. The function’s `return false` remains correct because gateway/composition gates are still missing.
- The older Stage-A implementation and QA make the same now-stale “missing unmute/unshadowban” explanation (`.hermes/handoffs/rapid-agent-semantic-moderation-implementation.md:35-36,120-122`; `.hermes/handoffs/rapid-agent-semantic-moderation-qa.md:20,93-95`). Their **disabled** conclusion remains correct.
- `.hermes/migration-audit.md` still calls bounded-delivered vertical rows `needs implementation`; this conflicts with handoff-level acceptance but not with the plan’s global `NOT COMPLETE` verdict. Treat the audit status text as unreconciled, not proof that the source changes are absent.
- Historic QA reports S2/S3 broad-suite failures from a stale S4 generic-fallback allowlist (`admin-moderator-s2-qa.md:23-24,52-62`; `admin-moderator-s3-qa.md:39-55`). S4 QA says it was reconciled (`admin-moderator-s4-qa.md:20-42`), and the present guard allowlist confirms the three S4 canonicals are absent; the current `go test ./... -count=1` passes.
- Older handoffs cite the vet diagnostic at `engine_impl.go:95`; current vet reports `:96`. The issue survives, but exact line citations need refresh.
- The full admin/moderator architecture says S0 should freeze `36/86` scoped catalog (`admin-moderator-full-migration-architecture.md:111`), while its own inventory/table and current source guard establish `34` command classes and **72 scoped aliases** (`:66`, `internal/command/admin_moderator_catalog_guard_test.go:81-85`). The 36/86 text is stale/incorrect and must not be used as an acceptance count.

## Smallest recommended next source-grounded slice

### [RECOMMENDED] Senior-owned semantic MODERATION activation gateway — construction-only, fail-closed until the nick/hash disposition is explicit

This is the smallest slice that advances a currently blocked, independently bounded vertical without widening human command behavior or disturbing accepted admin/moderator work. The prerequisite operations are already present and tested; the next gap is isolated control/identity transport, not more primitive protocol/H2 work.

**Exact proposed ownership (from `.hermes/handoffs/rapid-agent-semantic-moderation-activation-architecture.md:119-135`):**

1. Add a private closed `moderation_action` descriptor/enum in `internal/agent/tool/moderation_action.go`; exactly six aliases, only `{action}`, `additionalProperties: false`.
2. Add `internal/agent/commandgateway/moderation_gateway.go` to map the enum to the six reviewed typed core operations. It must reject non-MODERATION, non-bot/missing-capability, missing typed target, cancellation, unsupported values, and all model-supplied target-like data.
3. Change `internal/agent/live/participation.go` and `internal/agent/participation/invocation.go` to copy a listener-resolved `core.ModerationTarget`, never a nickname/model argument. Keep the public loop untouched.
4. Add an isolated one-action moderation loop and focused unit tests, but leave `SemanticModerationIngressReady()` false and do **not** compose it in `cmd/zenbot/main.go` until a senior/source owner selects and tests the nick/hash disposition.

**Why construction-only first:** it keeps the risk bounded and preserves fail-closed runtime state while establishing tests for identity provenance, closed schema, exact alias mapping, cancellation, one action, and no public-command reachability. Activation composition and readiness change are a subsequent tiny step only after the explicit decision and independent QA.

**Required gates before accepting this construction slice:** the architecture’s focused participation/live/tool/gateway/core/H2 commands and `go test -race` (`rapid-agent-semantic-moderation-activation-architecture.md:171-185`), plus `go test ./... -count=1`, `go build ./...`, and `git diff --check`. Record `go vet ./...` as failing on the existing copylock until separately fixed; do not call the overall migration accepted.

## Assessment conclusion

The worktree is materially ahead of the frozen audit for bounded S2–S5, prefix, last-online, semantic candidate scaffolding, and typed semantic reversals, with current full-test/build/diff evidence green. It remains a dirty, unreconciled, partial migration: eleven admin/moderator catalog rows are explicitly generic, semantic MODERATION is intentionally disabled, final vet fails, major plan inventories remain open, and no migration-wide acceptance is justified.
