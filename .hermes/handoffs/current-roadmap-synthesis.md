# Current Saturn → Zenbot roadmap synthesis

**Authority order:** `MIGRATION_PLAN.md` supplies the program contract and its rapid-priority override; `.hermes/migration-audit.md` remains the frozen scope ledger; newer QA handoffs supersede older checkpoint claims about current gates and delivered bounded slices. **Program verdict: NOT COMPLETE.**

## 1. Program goal

- **[OBSERVED / authoritative]** Finish the Saturn Java → Zenbot Go migration with strict observable parity, H2-only execution, and evidence for all **325** audited source units, **12** tables, **18** indexes, **197** SQL occurrences, **88** repository/service methods, commands, listeners, and agent behavior. Completion requires behavior evidence, not compilation/catalog presence.  
  **Citations:** `MIGRATION_PLAN.md:5,11,88-94,249-259`; `.hermes/migration-audit.md:4-18`.

## 2. Active priority ordering

1. **[OBSERVED / authoritative] Command parity first.** Deliver remaining real command behavior rather than aliases, acknowledgements, no-ops, or generic fallbacks; prioritize admin, moderator, remote-room/Whiskey, replica, DBZ, user, and `l` paths.  
   **Citation:** `MIGRATION_PLAN.md:13-21`.
2. **[OBSERVED / authoritative] Make the agent live second.** Runtime construction, participation/relay, `l`, room automation, routing, tools, memory, cancellation, and listener/command integration are still required. Private `internal/agent/**` foundations do not by themselves complete this work.  
   **Citation:** `MIGRATION_PLAN.md:17-20,31-35`.
3. **[OBSERVED / authoritative] Remote-room/session/replica/Whiskey and listener ordering third.** Reuse existing snapshot, replica, transport, and service foundations; do not redesign them.  
   **Citation:** `MIGRATION_PLAN.md:19-20,35`.
4. **[OBSERVED / authoritative] Rapid-phase execution policy.** Each capability gets focused evidence, relevant package tests, and final `go test ./...`; defer broad SQL inventory/index/migration hardening, SQLite cleanup, stress/race sweeps, and speculative cleanup unless they block active parity. Those deferrals are not closure waivers.  
   **Citations:** `MIGRATION_PLAN.md:20-25,69-82,213-258`.

## 3. Accepted completed slices — bounded, not migration closure

- **[OBSERVED / accepted] Earlier plan acceptance:** utility commands; mail/notes; weather/time; say/afk/list; safe moderation; info; users/nicks; help/howto; subscriptions; private snapshot coordinator; private agent runtime; prompts/resources; provider-neutral LLM/OpenAI-compatible adapter; request assembler. Also accepted: AgentMentionParser row #56 only, and bounded SQL Utility Group B only—not parent `SqlUtil` row #324 or row #325.  
  **Citation:** `MIGRATION_PLAN.md:31-32,42-67`.
- **[OBSERVED / accepted newer vertical QA]** Identity/history (`register/reg`, `authorize/auth`, `grant/access`, `messages/lastmessages`), manual simple moderation S2, active-target mute/unmute/color/flair S3, public shadow-ban S4, moderator activity S5, volatile prefix, and user last-online are accepted only within their stated command and persistence contracts.  
  **Citation:** `.hermes/handoffs/current-plan-vs-worktree-architecture.md:26-38`.
- **[OBSERVED / accepted-but-disabled]** Semantic Stage-A candidate scaffolding and typed `unmute`/`unshadowban` reversals are accepted as fail-closed foundations; they are not live semantic ingress or public command parity.  
  **Citations:** `.hermes/handoffs/current-plan-vs-worktree-architecture.md:38,51-57`; `.hermes/handoffs/copylock-semantic-recovery-qa.md:16-24,77-79`.
- **[OBSERVED / current QA]** Copylock recovery is complete: pointer-owned `NewEngineImpl` construction avoids copying synchronization state; focused, race, full test, `go vet ./...`, build, and diff-check gates passed.  
  **Citation:** `.hermes/handoffs/copylock-semantic-recovery-qa.md:9-14,26-58,77-79`.

## 4. Remaining immediate command slices

1. **[OBSERVED] Finish concrete catalog paths that remain generic:** admin `mine`, `replica`, `replicaoff`, `replicastatus`, `restart`, `shutdown`, `sql`, `whiskey`; moderator `automove`, `nuke`, `resurrect`. Catalog/alias registration is not behavior parity.  
   **Citation:** `.hermes/handoffs/current-plan-vs-worktree-architecture.md:42-49`.
2. **[RECOMMENDED / dependency order]** Execute remote snapshot commands (`mine`, `nuke`, `resurrect`) using temporary sessions with cleanup, then replica/Whiskey completion, then lifecycle/admin-SQL after explicit decisions, followed by shared registration/audit integration.  
   **Citation:** `.hermes/handoffs/admin-moderator-full-migration-architecture.md:117-121,123-133`.
3. **[RECOMMENDED]** Treat `automove` as the remaining local command-state slice and preserve the already accepted volatile prefix boundary unless an explicit durability decision changes it.  
   **Citations:** `.hermes/handoffs/admin-moderator-full-migration-architecture.md:117,135-141`; `.hermes/handoffs/current-plan-vs-worktree-architecture.md:63-69`.
4. **[OBSERVED]** Decisions still block exact parity for lifecycle (D1), admin SQL (D2), manual protected-user policy (D4), and Whiskey endpoint/proxy configuration (D5).  
   **Citation:** `.hermes/handoffs/admin-moderator-full-migration-architecture.md:135-141`.

## 5. Semantic activation gate and decision

- **[OBSERVED]** A private closed `ModerationAction`/`ModerationGateway` exists, has only the six source aliases, maps only to reviewed typed operations, and is not composed into public `run_command`, human dispatch, `main`, or the public runtime loop. `SemanticModerationIngressReady()` is literally `false`; listener target transport remains inert scaffolding.  
  **Citation:** `.hermes/handoffs/copylock-semantic-recovery-qa.md:16-24`.
- **[OBSERVED]** Semantic ingress must therefore remain disabled. The live path is not complete merely because the gateway/tool and six primitives exist.  
  **Citation:** `.hermes/handoffs/rapid-agent-semantic-moderation-activation-architecture.md:5-9,97,111-117,159-169,199-204`.
- **[RECOMMENDED / required decision]** A senior/source owner must explicitly select and test one disposition for Saturn’s semantic `unmute` nick/hash contradiction: (1) documented native-identity adaptation using captured raw hash, (2) strict compatibility that rejects unless nick equals hash, or (3) source correction/clarification first. Until then, keep construction/readiness fail-closed and do not compose ingress.  
  **Citation:** `.hermes/handoffs/rapid-agent-semantic-moderation-activation-architecture.md:99-117`.
- **[RECOMMENDED after decision]** Independently QA typed listener capture, private one-action moderation loop/runner selection, silent/no-persistence lifecycle, cancellation, and the zero-new-public-reachability constraint before changing readiness/composition.  
  **Citation:** `.hermes/handoffs/rapid-agent-semantic-moderation-activation-architecture.md:119-169`.

## 6. Deferred closure work

- **[OBSERVED / authoritative]** After command and live-agent integration: reconcile the 325-row ledger; complete all H2 schema/table/index/SQL/method obligations; prove visibility security; complete listener/transport/lifecycle integration; conduct SQLite-elimination acceptance; and run independent closure QA (format, vet, full/race/build, H2/security, audit evidence).  
  **Citations:** `MIGRATION_PLAN.md:23-25,69-82,114-131,199-211,213-259`.
- **[OBSERVED]** The frozen audit’s individual row status remains unreconciled even where bounded handoff QA accepts a vertical. Do not use that lag to erase delivered scoped evidence, and do not treat scoped evidence as global row or migration closure.  
  **Citation:** `.hermes/handoffs/current-plan-vs-worktree-architecture.md:26-28,59-61,76,84`.

## 7. Stale documents and superseded claims

- **[STALE]** `.hermes/handoffs/current-plan-vs-worktree-architecture.md` reports `go vet ./...` failing on `EngineImpl` copylock and says the private moderation gateway/tool loop does not appear in the worktree. Newer recovery QA verifies the copylock fix, `go vet ./...` exit 0, and the private gateway/action tool’s existence; its conclusion that semantic ingress is uncomposed remains current.  
  **Compare:** `current-plan-vs-worktree-architecture.md:22,51-57,71-86` with `copylock-semantic-recovery-qa.md:9-24,48-58,77-79`.
- **[STALE / historical snapshot]** `admin-moderator-full-migration-architecture.md` labels several now-accepted S2–S6 verticals as missing/partial and contains an invalid S0 count of 36 commands/86 aliases. Retain its dependency ordering and decision list, but use the newer accepted-slice record and the verified 34-command/72-alias catalog figure.  
  **Citations:** `admin-moderator-full-migration-architecture.md:31-66,109-121`; `current-plan-vs-worktree-architecture.md:26-38,78-87`.
- **[STALE / unreconciled ledger text]** `.hermes/migration-audit.md` still labels bounded-delivered rows as `needs implementation`; it is frozen inventory scope, not current bounded-slice status. Its overall **NOT COMPLETE** conclusion remains current.  
  **Citations:** `MIGRATION_PLAN.md:58-67,88-94`; `.hermes/handoffs/current-plan-vs-worktree-architecture.md:28,59-61,84`.
- **[STALE in part]** The semantic activation architecture’s proposed gateway/file-map statements are superseded by recovery QA only as to gateway/action-tool existence. Its activation decision, nick/hash gate, composition constraints, and no-public-reachability rules remain authoritative.  
  **Citations:** `rapid-agent-semantic-moderation-activation-architecture.md:68-117,119-135,159-169`; `copylock-semantic-recovery-qa.md:16-24`.

## 8. Worktree observation

- **[OBSERVED]** `git status --short` shows a deliberately dirty worktree: tracked command/core/agent/factory/listener/repository/service/main changes and untracked handoffs, tests, semantic gateway/action-tool files, and accepted vertical files. This synthesis changed no application source, staged nothing, and did not reset/clean/commit.
