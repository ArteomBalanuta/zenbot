# Moderation and identity command audit — 2026-09-11

Historical baseline report: references to open repairs or missing reproductions
describe that audit phase. Current scope, fixes and verification are recorded in
the [consolidated Teardown & Rebuild report](2026-09-11-command-audit-coverage.md).

Scope: all 23 registered moderator commands plus legacy `unlock`. Inspection covered concrete handlers, catalog contracts and aliases, legacy wrappers where present, typed engine operations, relevant service/repository methods, temporary-room operations, cancellation and failure outputs. Systematic debugging was used: inspect call chains, compare working paths, form specific hypotheses, then run isolated offline reproductions. No production code, live moderation state, or external service data was changed.

Latest scope overrides this historical report: the user excluded automove for
now (and DBZ elsewhere). M4 and the automove row below are deferred, not remaining
repair obligations. New automove regressions live only in ignored deferred
scratch files; production automove remains unchanged.

The pre-audit complete suite passed according to the coordinating task. Targeted existing tests also passed locally. Several existing compatibility tests explicitly require behavior that is incorrect for the current agent contract; those tests are evidence of inherited behavior, not proof of correctness.

## Reproduced findings

### M1 — P1: `nuke` bypasses the permanent-ban capability and reviewed-author restriction

Repair status: fixed in `86f02e5`, independently approved. The current catalog requires `AgentPermanentBan`, describes permanent roster bans followed by room locking, and enforces mute-only reviewed-author scope across discovery and execution. The trace below records the audited baseline; snapshot cancellation and execution receipts remain separate open repairs.

`internal/command/catalog/catalog.go:59` declares `nuke` as `AgentModerator`, describes removal, and omits `targetedCommand`. `internal/listener/snapshot/nuke_operation.go:28` iterates every snapshot user and sends `ban`, then `lockroom` at line 43. This is permanent room-wide banning, not ordinary kicking.

`internal/agent/participation/invocation.go:40` grants normal moderators `ModerationCommands`; only creator DIRECT invocations receive `PermanentBan` at line 44. Autonomous review contexts also receive only `ModerationCommands` with a single reviewed author at line 69. Both can nevertheless pass `AgentCommandAuthorized` for `nuke`. Because this entry is not marked as targeting a user, the gateway's first-argument reviewed-author check does not apply. An offline gateway reproduction using precisely the autonomous capability/target shape successfully emitted `ban innocent` followed by `lockroom` in another room.

Reproducer: `TestAuditModerationNukeRequiresPermanentBanCapability` in `internal/command/audit_moderation_repro_test.go`. Actual result: `SUCCEEDED`, two raw effects, no error. A safe correction must require the permanent-ban capability and constrain room-wide operations in reviewed-author contexts. Describe the permanent bans and lock explicitly. This changes capability enforcement, not LLM reasoning ownership.

### M2 — P1: removing one alias also deletes another registered identity's link and shared trip

Repair status: fixed in `71524ef`, independently approved. The selector is resolved inside the transaction, only the selected relationship is deleted, and parents survive while referenced by unrelated links. Exact-trip, ambiguous-selector and rollback regressions passed.

`internal/repository/h2/sql_util_group_b.go:50` resolves only rows directly matching the selector, then line 71 passes the resulting name/trip to a destructive helper. The helper at lines 91–105 deletes **all** links using either that name or that trip and deletes both parents. The uniqueness check does not inspect relationships through the selected parent.

Reproduction with isolated real H2: register `alice/shared-trip`; attach `bob` to `shared-trip`; call `DeleteIdentityAuthorized(ctx, "alice")`. The selector sees one `(alice, shared-trip)` row and proceeds, but it deletes two links plus the shared trip. Bob's name remains orphaned; his directory/mail relationship is lost despite not matching the selector. Actual result: `{TripNamesRows:2 TripRows:1 NameRows:1}`, Bob's surviving registration-link count is zero. The symmetric shared-name/multiple-trip shape has the corresponding risk.

Reproducer: `TestAuditModerationDeleteAliasPreservesOtherRegisteredIdentity` in `internal/repository/h2/audit_moderation_repro_test.go`. Either reject shared-parent deletion before mutation, or define narrower selector-specific deletion while preserving unrelated links/parents. Resolve and validate within the transaction so a concurrent association cannot expand the scope between validation and deletion.

### M3 — P2: successful `unshadowban -all` returns failure

`internal/command/unshadow_ban.go:33` deletes all records successfully, line 39 sends the success output, and line 40 returns `FAILED`. The agent gateway therefore reports rejection after completion. The current existing test explicitly asserts this failure status.

Reproducer: `TestAuditModerationUnshadowAllReportsSuccessfulDeletion`. Actual result: `deleted=1 status=FAILED err=<nil>`. Return success after successful deletion; retain an accurate already-empty outcome. The generic effect receipt problem for repository mutations is separately owned by the coordinating audit.

### M4 — P2: structured automove configuration can execute enable/disable instead

`internal/command/catalog/agent_contract.go:345–353` accepts a configure source named `on` or `OFF` and serializes `source destination`. `internal/command/automove.go:19–41` handles those tokens before checking arity, ignores the destination, and changes enabled state. Thus `{operation:"configure",source:"on",destination:"destination"}` executes enable on the existing configuration; an `OFF` source disables it.

Reproducer: `TestAuditModerationAutomoveConfigureDoesNotToggle`, both subcases fail: `configure=0 enable=1` and `configure=0 disable=1`. Reject unrepresentable source tokens in the structured contract, or make the command syntax distinguish configuration from lifecycle operations. Existing direct-command tests intentionally allow trailing arguments on toggles, so this is specifically a structured-contract ambiguity that must be resolved deliberately.

### M5 — P2: registration continues mutating after request cancellation

Repair status: fixed in `452531f`, independently approved. Context now reaches
the repository and registration rechecks cancellation after lookups. Root also
reran the original command regression successfully on the integrated checkout.

`internal/command/identity_commands.go:25` checks context only on entry. After both identity queries, lines 47–59 invoke context-free mutations without rechecking. The UserService identity interface remains context-free; `internal/repository/h2/identity.go:35`, `:58`, and `:79` create transactions using `context.Background()`.

Reproducer: cancel context during `IsTripRegistered` and return a successful false lookup; the command then registers `alice:trip` and returns `SUCCESSFUL` with nil error. Test: `TestAuditModerationRegisterDoesNotMutateAfterLookupCancellation`. A pre-mutation check fixes the demonstrated boundary; context-aware repository methods are necessary to cancel work during an actual query/transaction and prevent the agent timeout from abandoning a still-running registration.

## Additional verified call-chain findings

### M6 — P2: activity query failures become successful statistics

`internal/service/activity.go:18–20` converts a repository error to `(err.Error(), nil)`. `internal/command/activity.go:29–37` therefore replies `Stats: <database error>` and returns success. This can expose database details and makes the agent treat a failed read as completed data. Entry/after-query cancellation checks do work. Existing tests `TestActivityServiceReturnsRepositoryErrorTextAsSourceResult` and `TestActivityCommandRepliesWithRepositoryErrorTextAsSourceResult` reproduce and explicitly assert the bad status behavior. Both were run successfully during this audit. Preserve user-facing compatibility only if necessary, but propagate the operational error/status separately.

### M7 — P1: executing a temporary-room nuke is not cancellable or bounded by the workflow timeout

`internal/listener/snapshot/coordinator.go:254–266` marks the workflow completed, removes it from active workflows, and stops its timer **before** calling `Operation.Apply`. The operation has no request context. `internal/listener/snapshot/nuke_operation.go:28–43` sends every ban and sleeps 200 ms after each. A sufficiently large snapshot exceeds the agent tool's 10-second deadline while continuing permanent bans and eventually locking the room. Once Apply starts, coordinator Cancel no longer finds the workflow. Similar stale completion/flush handling affects resurrect and other snapshot commands and is assigned to the coordinating audit.

Suggested deterministic test: start Apply with a sender that blocks after the first ban; cancel the workflow/request; release sender and require that no remaining bans or final lock occur. Use controllable waits instead of real long sleeps. Also assert active state remains running until the operation and flush complete.

### M8 — P2: all resurrect temporary connections get the same nick

`internal/core/room_snapshot.go:58–66` builds the temporary nick from the first eight workflow-ID characters. `resurrectWorkflowID` emits `resurrect-<random hex>`, so every resurrect temporary session uses `msg_resurrec`. Two simultaneous moves from the same unserved room can therefore collide at the server despite distinct workflow IDs. Nuke IDs keep only three random hex characters, reducing uniqueness to 4096 names.

Suggested unit test: generate two distinct resurrect IDs and assert derived temporary names differ; then use the offline replica websocket fixture to submit overlapping same-room operations. Build the nick from entropy in the ID or a safe hash of the full ID. This is shared snapshot infrastructure owned by the coordinating audit.

### M9 — P1: contains-mode moderation iterates the live active-user map unsafely

Repair status: fixed in `5a343b0` and `cbab13c`, independently approved. All active/AFK ownership boundaries now copy records and nested mutable JSON flair under consistent synchronization; readers cannot mutate or iterate the engine's owned map concurrently.

`internal/core/engine_impl.go:681` returns `&e.ActiveUsers` without a read lock or copy. `kick -c` in `internal/command/handlers.go` and `shadowban -c` in `internal/command/shadow_ban.go:56` iterate that map while online events mutate it under `usersMu`. A lock held only by the writer does not protect these iterations. This can race or panic during ordinary join/leave traffic. `GetActiveUserByName` is locked, but it returns a mutable pointer and does not repair the map iteration issue. A snapshot API should copy the map and user identity fields under the lock. Run a race test with concurrent join/remove and contains commands. Root owns the shared snapshot correction.

## Complete per-command coverage

### Additional credential audit evidence

**M10 — High: configured credentials are case-folded into elevated authority.**

Repair status: fixed in `452531f`, independently approved. Exact credentials,
REGULAR fallback, atomic access grants and persisted-demotion behavior are
covered by source-boundary regressions. The trace below is historical; shared
committed-mutation receipt integration remains a separate outstanding task.
`SecurityService` configured administrator, user-command and lifecycle checks,
plus H2 `IsTripAuthorized`, use case-insensitive credential comparisons. A
caller with `abc123` therefore passes a configured `AbC123` administrator check.
H2 registration lookup and alias insertion have the same defect: requesting
`new-alias/abc123` can attach the alias to the different stored `AbC123` trip.
The no-repository service fallback additionally treats an unknown caller as
TRUSTED without any grant. These are source authorization checks, not LLM routing.

Execution trace: configure/store an exact mixed-case trip; submit its lowercase
variant; the case-folded comparison returns true or selects the original row;
the command obtains authority or persists an association for the wrong
credential. Controlled H2/service tests reproduce all these results, with
exact-match success controls. Repair is assigned to Task 12: exact trip
comparisons, REGULAR fallback, caller-context propagation and atomic role grants.
This repair is not yet marked complete.

Root evidence: `go test ./internal/service ./internal/repository/h2 -run
'^(TestIdentityAudit|TestSecurityIdentityAudit)' -count=1 -v` failed as expected.
See `internal/repository/h2/identity_context_audit_test.go` and
`internal/service/security_identity_audit_test.go`.

All listed canonicals are concrete via `newCommand`; none of the assigned registered commands reaches the generic fallback. Catalog aliases and moderator roles are covered by existing catalog guard tests. `authorizeCommand` and older ban/unban/lock helper types still exist but are bypassed by current canonical constructors; defects in these dead helpers are not counted as production findings.

| Command | Handler, semantics, validation and output | Audit result |
| --- | --- | --- |
| active | `activity.go`; requires first trip token; context-aware ActivityService/H2 query; stats/no-activity output | M6; catalog calls input an identity selector although H2 filters trip only. Extra direct arguments ignored. |
| authorize | `moderationIdentityCommand`; first trip token; typed `authtrip`; confirmation only after send | No additional command-specific defect found. Current constructor correctly uses server authorization, not the unused local security helper. |
| automove | `automove.go`; on/off before arity; two args configure additively; otherwise three usage/status messages and FAILED | M4. Enable sets enabled before starting replicas; errors leave partial state. Disable removes every configured source replica, including pre-existing ones. These existing semantics need accurate partial receipts; root owns state integration. Join notice hardcodes lounge even for a configured different destination (`listener/automove_join.go:13`). |
| ban | `simpleBanCommand`; one `@` removed; typed `ban` nick; confirmation | Proper PermanentBan catalog gate. Does not require/check active membership although tool metadata says active; only transport success is known. Legacy `Ban.Execute` delegates typed helper with Background context and discards error. |
| captcha | `captcha.go`; absent/on enables, off disables, other input fails; typed payload; success confirmation | No additional command-specific defect found. Structured bool contract excludes invalid states; direct command ignores trailing tokens. |
| color | `profile.go`; requires target/value, resolves active canonical nick, typed forcecolor; intentionally no success chat | Shared silent-success tool contract mismatch, sent to root. Value is passed through without hex validation; server acceptance was not verified. |
| deauthorize | typed `moderationIdentityCommand` with deauthorize flag; exact trip; `deauthtrip` then confirmation | No additional command-specific defect found. |
| flair | shared `profile.go`; active nick plus one token value; typed forceflair then success reply | No additional command-specific defect found. Structured requiredToken means multiword flair is not supported; no clear existing promise of multiword support. |
| kick | `handlers.go` concrete command; normalized active target; `-m` skips absent users; `-c` case-sensitive substring; fail-fast sends; silent success | M9. Bare `-m` or `-c` succeeds without effects; duplicates in -m are sent twice. Legacy `Kick.Execute` still emits `Kick(target, randomRoom)` while registered handler emits plain typed kick. Legacy wrapper drops send errors and lacks canonical casing resolution. Current registration uses concrete handler, so legacy difference is recorded, not presumed live. |
| messages | `identity_commands.go`; trip/count; nonnumeric fails, >30 clamps; typed GroupB public-only history with stable ordering, fallback legacy reader; reply rows use stored trip | Direct nonpositive count becomes repository default 5, while structured contract accepts only 1–30. Message truncation uses 200 bytes and can split UTF-8 before Java-style quoting, corrupting output. No new high-severity history/privacy defect found; PUBLIC filter exists. |
| lock | `simpleLockCommand`; exact on/off required; typed lock/unlock and confirmation | No additional command-specific defect found. Legacy Lock delegates typed helper. |
| unlock (legacy) | separate alias resolution and `Unlock.Execute`/`unlockCommand`; no args, calls void `engine.Unlock`, always confirms | Not in current production catalog or agent allowlist. If explicitly legacy-registered, send error is discarded and success can be false; direct legacy wrapper uses only IsWhisper. Root should decide whether to retire or route it through typed unlock. |
| mute | `mute.go`; normalize/resolve active canonical nick/hash; typed mute; confirmation includes hash | No additional command-specific defect found apart from shared mutable user pointers/receipt handling. Missing user is FAILED with room message. |
| nuke | first room argument with all `@` removed; credentialed snapshot; bans every snapshot user then locks | M1, M7, M8 and shared silent success/capture issues. Snapshot includes no filtering for author, bot, or moderators; actual server protection behavior unverified. Blank/missing target and pre-cancel stop submission. |
| overflow | normalize first nick token; typed overflow; no active lookup; silent success | Shared silent-success mismatch. Metadata promises active nick but handler sends absent nick too. No additional local failure propagation defect found. |
| register | four existing-name/trip branches; new identities get REGULAR; transactional mutations; requires two args | M5. Both existing but not already associated is rejected rather than linked; this is existing explicitly tested behavior, but catalog's “associate” is broader. SQL parameters are bound. |
| remove | unique selector then typed DeleteIdentityAuthorized; confirmation only after successful transaction | M2. Blank/missing/ambiguous selectors fail. Transaction rollback protects multi-statement failures. |
| resurrect | exactly nick/source/destination; normalize nick; serving-room typed move first, else credentialed snapshot | M7/M8 shared workflow issues. Serving-room path sends without checking membership; temporary path checks case-insensitively but sends requested casing. Absent temporary target becomes Absent output, which generic gateway currently treats as succeeded; root owns outcome mapping. |
| shadowbanlist | typed repo list, base64 hashes decoded, placeholder trip for blank, empty list succeeds | No additional command-specific defect found. Output is unbounded for large ban tables; any malformed stored base64 hash aborts listing. |
| shadowban | normalized single name; active identity persisted then kicked; offline persists name-only; contains persists/kicks each current user; stop on error | M9. Contains detection accepts `-c` anywhere but uses arguments[1], so malformed `target -c fragment` matches literal `-c`; empty match set returns success. Persistence may commit before kick error/cancel; existing cancellation test proves that boundary. Root owns partial receipt correction. |
| unbanall | no parameters; typed unbanall then mercy confirmation | Proper PermanentBan capability; no additional command-specific defect found. Legacy wrapper delegates same typed helper using Background. |
| unban | exact first hash, typed unban then confirmation | Proper PermanentBan capability. Hash is correctly not normalized as nick. In reviewed-author context generic nickname target comparison rejects a correct hash; root owns target contract mapping. Legacy delegates typed helper. |
| unmute | exact first hash, typed unmute then confirmation | Same reviewed-author hash-vs-nickname mismatch as unban, assigned to root. No additional command-specific defect found. |
| unshadowban | exact selector matches name, trip, or encoded raw hash; `-all` anywhere clears records; confirmation | M3. `@nick` is not normalized, while shadowban accepts it, so undo-by-mention can report success without deleting. Delete-by-selector returns no affected count, so absent selectors also report success. Keep exact hash/trip handling when addressing nickname mentions. |

## Test evidence and boundaries

New temporary regression files, retained for the coordinating task to incorporate or remove:

- `internal/command/audit_moderation_repro_test.go`: five failing subcases across four tests (unshadow-all, automove on/OFF, nuke authorization, register cancellation).
- `internal/repository/h2/audit_moderation_repro_test.go`: one failing real-H2 shared-alias deletion test.

Command executed: `go test ./internal/command ./internal/repository/h2 -run TestAuditModeration -count=1 -v`. All intended regression checks failed for the concrete reasons above; compilation and fixture setup succeeded. No production fixes were applied, so the working suite is intentionally red while these audit regression files remain.

Existing targeted checks were run for activity error conversion, unshadowban deletion, normalized nuke protocol, automove configuration, and kick multi/contains behavior. They passed, including the tests that deliberately encode incorrect status conventions. No live server acknowledgment, real bans, real messages, or real room connections were performed. The H2 reproducer uses a temporary isolated database and local fixture process. Race and overlapping temporary-room-session reproductions remain suggested tests rather than observed runtime failures. Generic command dispatch, capture, tool result modes, snapshots and partial-state receipt work are consolidated by the coordinating audit.
