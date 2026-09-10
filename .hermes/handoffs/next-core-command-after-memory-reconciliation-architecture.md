# Next core command after memory reconciliation: no eligible unblocked command vertical

## Decision

**Do not select a new core-command implementation vertical.** After removing already accepted `memory` / `mem` / `memstats`, the current catalog leaves no command that is both genuinely unimplemented and inside the stated unblocked scope.

This replaces the stale memory selection without relabelling a generic fallback, a bounded acceptance, or an explicitly deferred remote/policy capability as new work. The smallest eligible vertical is therefore **none** until a deferral is explicitly lifted or a demonstrated defect invalidates an accepted slice.

`MIGRATION_PLAN.md` requires concrete command behavior and focused evidence, not catalog registration, acknowledgement, or no-op behavior. It also says the rapid phase should defer speculative work unless a concrete failure blocks the active capability (`MIGRATION_PLAN.md`, “Execution-priority override — rapid command and agent parity”, items 1, 4, and 5).

## Reconciliation authority

1. **Memory is ineligible.** `.hermes/handoffs/memory-command-current-state-reconciliation.md`, “Verdict” and “Historical acceptance and clean ownership”, establishes that `f9079ca` added the concrete route, registration, implementation, and test; current focused evidence accepts the ADMIN command. No replacement implementation or manufactured RED is valid.
2. **The scoped source catalog is fully accounted for.** `internal/command/admin_moderator_catalog_guard_test.go`, `saturnAdminModeratorCatalog` and `TestAdminModeratorCatalogMatchesSaturnSource`, declares 11 ADMIN plus 23 MODERATOR source rows and checks exact canonical, role, and ordered aliases against `RegisterAll`.
3. **Exactly five scoped canonicals remain generic.** `internal/command/admin_moderator_catalog_guard_test.go`, `allowedScopedGenericFallbacks` and `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded`, fail unless generic construction is exactly `mine`, `restart`, `shutdown`, `sql`, or `whiskey`. `internal/command/handlers.go`, `newCommand`, routes every other scoped canonical to a concrete command.
4. **Normal reachability does not turn catalog rows into parity.** `internal/command/dispatch_adapter.go`, `RegisterUserUtilitiesWithDirectAgent`, registers only concrete/capability-proven canonicals; `internal/listener/message/handlers.go`, `DispatchUserCommand.Handle`, authorizes a resolved caller before invoking the registered command.

## Candidate ranking

| Rank | Candidate | Current status | Decision | Evidence / reason |
|---:|---|---|---|---|
| — | **No eligible command** | The only remaining generic scoped routes are all explicitly deferred below. All other inspected core rows are either accepted or have a concrete reachable route with accepted bounded QA. | **Select no implementation.** | Guard-derived inventory above; this is the only result compatible with the requested exclusions. |
| 1 | `mine` | Generic fallback; Saturn is a scheduled credential-bearing temporary `LIST_CMD` workflow with optional proxy use. | **Defer.** | `internal/command/admin_moderator_catalog_guard_test.go`, `allowedScopedGenericFallbacks`; Saturn `src/main/java/org/saturn/app/command/impl/admin/MineTripCommandImpl.java`, `startMining`, `joinChannel`; requested deferral of mine and credentialed temporary sessions. |
| 2 | remote `msgchannel` / `msgroom` | Local branch accepted; remote delivery is intentionally unavailable. | **Defer.** | `.hermes/handoffs/local-msgchannel-parity-qa.md`, “Verdict” and “Scope and worktree record”; requested deferral of credentialed temporary sessions. |
| 3 | `whiskey` | Generic fallback; source creates AGENT replicas and conditionally tests/provisions proxies. | **Defer.** | Guard fallback list; Saturn `src/main/java/org/saturn/app/command/impl/admin/WhiskeyReplicaCommandImpl.java`, `initializeProxyMapping`, `registerReplica`, and `startReplicaWithProxyTesting`; requested proxy/Whiskey deferral. |
| 4 | `sql` | Generic fallback; source forwards arbitrary text to an ADMIN SQL service and returns its result. | **Defer.** | Guard fallback list; Saturn `src/main/java/org/saturn/app/command/impl/admin/SqlUserCommandImpl.java`, `execute`; requested raw-SQL deferral. |
| 5 | `restart` / `shutdown` | Generic fallback; source invokes singleton application lifecycle operations. | **Defer.** | Guard fallback list; Saturn `RestartCommandImpl.java` and `ShutdownCommandImpl.java`, `execute`; requested lifecycle deferral. |
| 6 | `replica` / `replicaoff` / `replicastatus` | Concrete and accepted. | **Exclude; do not reopen.** | `.hermes/handoffs/replica-qa.md`, “Verdict” and “Verification”, accepts the bounded lifecycle slice and confirms the fallback guard now contains only the five deferred names. |
| 7 | `prefix`, `automove`, `nuke`, `resurrect` | Concrete and accepted in their stated scopes. | **Exclude; do not reopen.** | `.hermes/handoffs/admin-prefix-qa.md`, “Acceptance verdict”; `.hermes/handoffs/automove-final-qa.md`, “Verdict”; `.hermes/handoffs/nuke-qa.md`, “Verdict”; `.hermes/handoffs/resurrect-qa.md`, “Verdict”. |
| 8 | `ws` / `wsay`, `wsa` / `wsayanon` / `anonsay` | Concrete bounded support-relay vertical accepted. | **Exclude.** | `.hermes/handoffs/support-relay-command-parity-qa.md`, “Outcome” and “Source and contract audit”. |
| 9 | local `msgchannel` / `msgroom` | Concrete bounded local-room vertical accepted. | **Exclude.** | `.hermes/handoffs/local-msgchannel-parity-qa.md`, “Verdict”. |
| 10 | `access` / `grant`, DBZ, and `lastonline` | Already accepted bounded verticals. | **Exclude.** | `.hermes/handoffs/access-command-parity-qa.md`, “Result”; `.hermes/handoffs/dbzstrength-qa.md`, “Result”; `.hermes/handoffs/last-online-qa.md`, “Result”. |

The older broad inventory in `.hermes/handoffs/admin-moderator-full-migration-architecture.md` is historical for dependency context only. Its rows that label current concrete/accepted slices as missing or partial are superseded by the newer named QA records above, as `.hermes/handoffs/current-roadmap-synthesis.md`, “Stale documents and superseded claims”, already notes.

## Why an implementation/TDD plan is intentionally absent

A strict TDD plan requires an implementable, approved behavior boundary. None exists in the candidate set:

- Writing a RED test for one of the five generic commands would necessarily encode an unapproved policy or activate explicitly deferred proxy, credential, raw-SQL, or process-control behavior.
- Writing a new RED test for `memory` or an accepted command would be dishonest unless it first exposes a real regression; deleting accepted code to recreate history is prohibited by `.hermes/handoffs/memory-command-current-state-reconciliation.md`, “Decision boundary”.
- Reopening accepted local relay, local-room, access, or DBZ commands violates the requested scope and does not advance an unimplemented vertical.

### Required de-block contract before any next command implementation

When the owner explicitly lifts one deferral, create a new source-specific architecture and execute these tracers **one at a time**. This is a gate, not authorization to implement now.

1. **RED — approved boundary and registration.** Add an exact source-derived test proving catalog aliases/role and that the formerly generic canonical is registered only when its newly approved capability exists. Run its named focused test and record the actual failure against the generic route.
2. **GREEN — minimal capability seam.** Add only the typed capability/configuration or lifecycle seam approved for that command. Rerun the same test; do not use `saturnCommand`, an unconditional registration, or a generic success response.
3. **RED — source behavior.** Add deterministic tests for source parsing, exact reply/raw output, authorization position, side effects, cancellation, and error/no-success behavior. Use fakes for process/proxy/session boundaries; do not use live credentials or real process termination.
4. **GREEN — concrete route.** Implement the smallest command owner plus its necessary typed boundary. Assert `newCommand` produces the concrete type and remove that canonical from `allowedScopedGenericFallbacks` only after its behavior test is green.
5. **Regression and acceptance.** Run focused package tests, the relevant listener/transport or H2 tests, `go test ./...`, `go vet ./...`, `go build ./...`, `git diff --check`, and `git diff --cached --check`. Record actual RED→GREEN evidence and preserve unrelated dirty work.

The exact behavior suite must be chosen after the applicable decision: mine needs an approved credential/session/scheduler ownership model; Whiskey needs proxy configuration and AGENT-replica semantics; SQL needs an admin execution/disclosure policy; lifecycle needs supervisor authority and reply/teardown semantics. No common implementation design can safely substitute for those decisions.

## Hard exclusions and invariants

- Do not implement remote `msgchannel` / `msgroom`, proxy/Whiskey activation, raw SQL, `mine`, `restart`, or `shutdown`.
- Do not modify or reopen accepted `memory`, `ws`/`wsay`, `wsa`/`wsayanon`/`anonsay`, bounded local `msgchannel`/`msgroom`, access/grant, DBZ, replica, prefix, automove, nuke, resurrect, or last-online scopes.
- Do not introduce credential files, temporary-session activation, proxy configuration, process-control APIs, raw-SQL policy, identity/policy work, schema changes, agent behavior, or listener-order changes.
- Do not stage, reset, clean, restore, checkout, commit, or change Saturn source.
- The migration remains **NOT COMPLETE**. A no-selection result is not a migration-closure claim.

## Current validation record

This architecture pass writes only this handoff. It must be validated after write by resolving every cited source/handoff path, checking the generic fallback set and concrete routes, running the focused catalog baseline, and running diff hygiene. No application or test source belongs to this pass.
