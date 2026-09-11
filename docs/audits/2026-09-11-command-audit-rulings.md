# Command audit rulings — 2026-09-11

This is the exhaustive chronological record of substantive rulings from the audit ledger. The ruling text is preserved verbatim, including repeated, superseded, and procedural decisions.

Current scope excludes DBZ and automove. Entries below that mention DBZ or automove are retained as historical rulings only and do not authorize new work in those excluded areas.

Eight original rulings did not state a cost. Their text remains unmodified;
the controller supplies these explicit retrospective risk annotations for handoff:

- #1: Excluded command defects remain deferred; reopening them requires a later scoped task.
- #11: An undiscovered external consumer of a removed legacy class would require restoring an adapter from Git; registered aliases remain supported.
- #12: Incorrect recorder placement could miscount committed effects, requiring source-boundary adjustments; it must never infer task completion.
- #13: A mechanical fixture update could mask an excluded-family dispatch regression; behavior/state assertions were retained.
- #14: Legacy public audit logging remains weaker than typed storage; an incorrectly permitted fallback could lose attribution, so private/unknown visibility and real typed errors fail closed.
- #15: Internal delete callers must migrate to affected-row results; a driver that cannot establish the count yields uncertainty, not invented mutation evidence.
- #18: An undiscovered trailing-space dependency would require a formatting compatibility adjustment, not a replica-lifetime change.
- #20: A supported late coordinator-installation path would need a fresh binding to expose remote capability; construction-time installation was verified.

## 1. Prior DBZ scope

> Prior DBZ scope ruling: Exclude dbzstr, dfight,
> dbzhelp, dbzregister, dspawn, dbzstats and all aliases from remaining audit,
> fixes and receipt integration. Keep existing DBZ commits unchanged; no revert
> was requested. Remaining scope: 58 catalog identities plus legacy unlock,
> including 53 catalog agent tools. This update overrides older DBZ obligations
> elsewhere in this historical ledger and reports. All active workers notified.

## 2. Master-checkout workflow

> Ruling: continue the user's existing master-checkout workflow — prior explicit master integration requests and autonomous audit authority support in-place work; no push or deployment — cost if wrong is moving the resulting local commits to an isolated branch.

## 3. Compatibility tests

> Ruling: replace compatibility tests that require silent failure or disclosure — correctness and privacy are explicit requirements — cost if wrong is changed legacy output, documented per fix.

## 4. Concurrent implementers

> Ruling: use file-owned independent implementers concurrently — developer instructions explicitly require useful parallelism — cost if wrong is integration conflict; root owns shared interfaces and reviews each unit.

## 5. Task 2 reviewer reassignment

> Ruling: resume the available reviewer as Task 2 fix implementer after the original worker resume again failed with an authoritative thread-limit error — the existing reviewer already traced the copy boundary and can write its regression — cost if wrong is additional independent re-review, which is mandatory and will use another agent. No self-review approval substitutes for that gate.

## 6. Registration availability

> Ruling: registration checks actual configured controllers through optional common.CommandAvailability, not just EngineImpl method presence — production replicas otherwise advertise unwired replica/relay/automove methods — cost if wrong is a hidden command until its real capability is configured. Local working prefix remains supported. Task 7 owns the minimal common/registration changes; later gateway availability must reuse the owner contract.

## 7. Transaction commit uncertainty

> Ruling: transaction commit errors use generic repository.ErrCommitOutcomeUnknown in the existing H2 WithTx rather than a DBZ-only helper — the same uncertainty matters to all mutation receipts — cost if wrong is conservative non-replayable classification for a commit error that actually rolled back. Definite write/closure failures remain unchanged; uncertain game rewards cannot grant a second reward by retrying the same enemy.

## 8. Mail attempt ownership

> Ruling: use durable per-recipient mail attempt ownership instead of only moving MarkDelivered between batch sends — batch ordering alone cannot prevent concurrent/crash replay or the second recipient's lost delivery — cost if wrong is conservative unresolved delivery after an interrupted attempt that never reached the server. No automatic retry/lease reclaim is safe without a server idempotency receipt. Existing source rows are preserved; uncertain attempts remain queryable and return safe operational errors.

## 9. Unknown identity authorization

> Ruling: unknown identities without a repository receive REGULAR, not the old TRUSTED fallback — no configured or persisted grant authorizes elevation — cost if wrong is that an adapter relying on implicit trust must now supply an explicit grant or repository. Exact configured credentials retain their grants; the explicit wildcard remains supported.

## 10. Task 10 capacity

> Task10 capacity ruling: fresh implementation spawn failed with thread-limit while a completed snapshot worker occupied the fourth slot. Resume that available worker for the separate bounded Task10 brief, rather than repeatedly retry creation or idle with independent work available. Cost is retained context; mitigate with explicit file ownership, new report, separate commit and independent reviewer. The worker was told Task8 is approved and not to edit snapshot lifecycle, kick or excluded commands. Root-owned shared parsing REDs transfer to Task10.

## 11. Task 10 scope

> Task10 scope ruling: retain canonical in subscription constructor and add sub/unsub canonical construction so their clones execute actual state changes. Root verified the constructor omission. Root also read say.go/afk.go and searched all Go source under internal/cmd: the legacy Say/Afk classes have no callers outside their own definitions; model.Afk is unrelated. Remove those two obsolete duplicate classes while preserving the registered canonical commands and aliases. Deletions will be recoverable from Git. Do not apply this ruling to other legacy classes without caller evidence.

## 12. Task 16 architecture

> Task16 architecture ruling: one context-scoped, data-free positive mutation recorder, consumed by the gateway and absent/no-op outside invocation. Source owners emit only confirmed commits, not model prose; a shared service may not retain per-request recorder state. Explicit reply-error propagation across in-scope handlers is preferred to a second command wrapper/hidden error accumulator. Snapshot/transport receipts stay separate and are merged without duplicates. Counts are execution evidence, never original-intent obligations or a completion judge. Excluded families stay untouched.

## 13. Task 15 compatibility

> Task15 compatibility ruling: only mechanical shared-dispatch error expectation changes are allowed in existing DBZ dispatch fixtures (ten checks plus errors import); no DBZ production, state assertions, game behavior, new targeted cases or targeted runs. Shared handlers_test.go may gain a typed audit stub and change precanceled DirectL authorization-call expectation to zero. Coordinate staging with Task16.

## 14. Task 15 production integration

> Task15 production integration ruling: EngineImpl advertises typed audit even when its actual repository is the legacy Dummy implementation. Allow explicit PUBLIC-only fallback inside LogMessageRecord, with cancellation checks; keep WHISPER/unknown fail-closed and never fallback after an actual typed storage error. Root inspected the method and real websocket composition; the worker's new controlled regression reproduces the failure. This corrects the production adapter rather than hiding it with a test-only repository.

## 15. Task 16 counted delete

> Task16 counted-delete ruling: a successful zero-row DELETE is not evidence that data changed. Existing shadow-ban repository removal methods may return affected counts, with mechanical caller/test adaptations, including the typed core reversal adapter. Record one logical mutation only for a positive known row count. Do not invent a parallel receipt repository interface or change selector grammar in this unit. A thin legacy reply helper may remain for excluded callers; in-scope handlers use the context-aware error-returning helper.

## 16. Task 18 dispatch architecture

> Task18 Ruling: use one error-capable BeginDispatch(ctx) and exported common.BeginCommandDispatch for future gateway reuse. Core-owned per-operation handles retain completion/error facts; error-only Request methods adapt to the same admission path. Coalescing shares the existing operation and must identify new versus coalesced admission, not fabricate a second effect. Owner retains only active/pending operations; handles settle once on result/cancellation/close/supersession. Production reporting runs outside locks. Cost if wrong is a small internal API migration; no command/tool contract changes or dispatcher waiting are authorized in this unit.

## 17. Task 18 cleanup

> Task18 cleanup ruling: final process teardown may retry idempotent cleanup of one explicitly retained owned host/candidate after a failed retirement; this is resource cleanup, not rerunning replacement to erase its error. Verify StopContext idempotence and document the injected retire callback contract. Preserve original operation error and join cleanup failure; do not rebuild/start automatically after failed retirement. Cost if wrong is a conservative unpublished host requiring cleanup, rather than a false healthy binding.

## 18. Task 18 integration fixture

> Task18 integration fixture ruling: root inspected cmd/zenbot/replica_composition_test.go:72; it expected an invented trailing space after input `!say hello room`. Task10 now preserves the body rather than constructing a trailing separator. Authorize only that expected string to become `hello room`, no replica production or adjacent excluded tests. The lifetime assertion is preserved; focused case passes. This is Task10 compatibility, not Task15.

## 19. Task 20 interface

> Task20 interface ruling: OperationResult.DataObserved set only by the list source after successful data construction; commandgateway carries the marker and Data. Error Result has optional ObservedData containing the existing declared command payload. Shared validation checks it without duplicating validators; malformed optional data is dropped while original error/effect/counts survive. read_tool_result may add observedData selector while preserving content/error-text semantics. Cost if wrong is an optional internal protocol extension; no arbitrary raw error data or success inference is allowed.

## 20. Task 19 producer capability

> Task19 producer-capability ruling: actual credentialedRoomSnapshotMaster advertises remote support with a nil coordinator. Root inspected room_snapshot.go and all binding/install call sites: installation is construction-time, factory installs before production binding/registration. Prefer returning the ordinary engine view from BindCredentialedRoomSnapshotMaster when coordinator is absent, exposing the decorator only for configured MASTER. This preserves manual local list/relay and avoids another optional availability seam. Worker must verify no supported late-install caller contradicts this; binding condition/tests authorized, workflow ownership unchanged.

## 21. Task 19 fallback source selection

> Task19 fix round1/5 dispatched to original implementer after Important configured/source mismatch review. Ruling: preserve usable fallback dependencies and make actual source selection share typed-nil configuration semantics with admission, authorizing bounded users/messages/lastonline source checks and a dependency-neutral helper if needed. No fallback after real query error or pre-work count warning on definite unavailability. Cost if wrong is a narrow source-selection compatibility migration; hiding a valid fallback would violate the working-minimal-engine requirement. Root verified the three actual source paths before dispatch.

## 22. Task 24 receipt counts

> Task24 receipt ruling: root's transferred lifecycle assertion omitted the acknowledgment from aggregate ActionCount. Existing capture.recordDelivery increments both deliveryCount and actionCount; accepted request+ack is2/1, coalesced+ack1/1. Source insertion count remains exactly1/0. Authorize fixture correction, preserve aggregate ledger semantics. Root checked agent_capture_engine.go:43-49; initial RED remains genuine because actual data/counts were all missing. Cost if wrong is count-only fixture repair, not production protocol expansion.

## 23. Task 22 mechanical adapters

> Task22 mechanical-adapter ruling: authorize only signature/return adjustment in listener/message/dbz_dispatch_test.go and inventory snapshot read in cmd/zenbot/automove_composition_test.go, preserving behavior/assertions and no targeted excluded testing. Private runtime inventory requires fixture literal removal or registered setup elsewhere. Cost if wrong is compile-only fixture churn, not excluded feature changes. Moderation_s2 signature adaptation waits for Task26 commit.

## 24. Task 24 cancellation

> Task24 cancellation ruling: gateway currently treats every cancellation after command entry as unknown, including structured lifecycle rejection before insertion. Authorize bounded source-owned typed non-admission signal and a small gateway mapping before generic ctx-unknown; preserve errors.Is and require no captured effects. Absence of observations/counts or canonical-name checks alone is not proof. Error-only compatibility controllers may act then return cancellation and must remain unknown. Worker must test that negative control and actual structured source rejection. Cost if wrong is an internal error-boundary adjustment; no generic replay/ledger policy or optimistic unknown downgrade is authorized.

## 25. Task 28 help surface

> Remaining help findings were already identified in utility audit but had no repair unit: live help still advertises unsupported mine/whiskey routes, JVM labels, application/replica lifecycle scope and misclassified duplicate msgchannel. Root inspected actual help.go and its real-newline constants versus literal-backslash split. Ruling: bounded Task28 repairs existing static help and alignment after Tasks25/27; do not introduce a new help framework or production audit-exclusion rules. Preserve excluded rows' meaning, private delivery and observation semantics. Cost if wrong is reversible help text/formatting change; capability filtering remains intentionally outside this repair. Task28 preflight has one overlap with25/27 in help.go, ordered after both; no other source interface changes. Root initially included move among unsupported routes, then verified catalog.go:63: move is a legitimate resurrect alias. Corrected the plan before dispatch; do not remove or flag it.

## 26. Task 26 selected-source move

> Task26 fix round1/5 dispatched to original implementer: independent review confirmed raw resurrect selector passes the new canonical NickTarget type without source lookup. Root independently read command/core handoff, after discovering same path while checking help grammar. Ruling: resolve on selected serving host/replica and clarify existing LiveRoomMover raw-selector handoff (ordinary normalized string acceptable), no competing protocol API or excluded edits; missing user in an owned source is a known rejection, not a remote fallback. Cost if wrong is a bounded internal interface migration. Include host/replica command→core RED, canonical/marker/case/cancellation controls. Minor bare-marker tests may accompany fix; usage replies are allowed but moderation requests/mutations are not. Darwin warning remains accepted documented baseline.

## 27. Task 22 reviewer reassignment

> Task22 review found Important retained ghost resolver outside immutable catalog: handlers.go603-604 still fabricates unlock/unlockroom although Task21 removed the concrete handler/factory branch. Root verified source and no current lookup test callers; prior Task21 report and root coverage overstated definition-fallback removal. Task21's cleanup is otherwise retained; this exact remaining gap is fixed under22, not silently declared done. Original22 resume failed with agent-thread-limit. Ruling: reuse the available completed independent reviewer seat for the bounded two-file fix, with new tests and appended22report; a different worker must perform scoped re-review. Cost if wrong is loss of fresh-implementer context, mitigated by narrow supplied finding and independent re-review. No duplicate review or quota/reset action. Task22 remains unapproved pending fix1.

## 28. Task 29 roster identity

> Remaining utility-report roster finding confirmed in actual list_operation.go: orderedUniqueUsers uses model.IdentityKey, whose trip/hash-first identity collapses distinct observed nicknames sharing credentials/network hash. Existing list test labels a different same-trip nickname duplicate and asserts the undercount. Ruling: Task29 counts exact observed nicknames, not inferred humans/credential identities; preserve first exact duplicate and stable ordering. Do not change global IdentityKey, authorization, old Store or other action selection. Bots remain part of the observed roster; no guessed collector exclusion without source-certified metadata. Cost if wrong is a roster count compatibility change, covered by real Parse→producer and gateway error-observation tests. Task29 uses Task20 producer path and is independent of Task25 text observations/Task26 move; Task27 later comments only must preserve its data.

## 29. Task 28 and Task 27 concurrency

> Ruling: Task28 may run concurrently with Task27 after Task25 review — the completed Task27 source preflight assigns no help.go/help-test edits, eliminating the originally anticipated file overlap; both consume the already verified outbound-write boundary — cost if wrong is a reversible wording reconciliation at integration. Task28 owns only help content/alignment/tests; Task27 owns source acknowledgments/catalog/shared interpretation. Index commits remain serialized and independently reviewed. Updated plan and regenerated Task28 brief before dispatch.

## 30. AFK identity

> Ruling: AFK is caller-nickname presence, not a trip-wide operation — the existing catalog explicitly says mark the caller and forbids changing another user's presence; shared credentials can belong to multiple active nicknames — cost if wrong is changing legacy trip-wide AFK behavior to the narrower declared contract. Validate exact caller trip and active source identity; preserve storage and actual receipts. This is identity/grammar validation, not semantic task reasoning.

## 31. Utility room selectors

> Root additionally reproduced the previously recorded list/msgchannel room-selector inconsistency in utility_boundary_audit_test.go: msgchannel other?room and ?other?room submit otherroom; list ?lounge submits ?lounge. RED session30136 exit1/0.536s. Ruling: normalize one leading raw room-link marker for these utility selectors and preserve every remaining room byte — deleting interior/trailing characters silently changes the destination and can misdirect a message; source-owned channels stay literal — cost if wrong is a documented compatibility change for malformed-looking legacy operands previously stripped globally. Existing ?other? test encodes that old rewrite and must change. No claim about server acceptance of a particular room name is needed; transmission must preserve selected identity. Task30 bounded scope/brief expanded before dispatch; no root production edit.

## 32. Historical source facts

> Ruling: historical last-online returns timestamped source observations, not a computed ongoing session; nicks returns public historical aliases for an exact trip — persisted leaves, best-effort event capture and cross-room scope cannot establish uninterrupted/current activity, and registrations are a different source — cost if wrong is a documented history output/compatibility change. Unify primary/fallback fact records to prevent weaker fallback semantics; preserve privacy and observed-error retention.

## 33. History lookup collisions

> Ruling: preserve nickname-or-trip lookup but reject public cross-kind collisions whose matching row sets differ — merging separate identities would corrupt the independent fact report; arbitrary precedence would silently choose a subject — cost if wrong is a conservative ambiguity rejection requiring the caller to resolve an observed nickname first. Equal nickname-and-trip on the same matching row is not inherently ambiguous. No private evidence, extra tool schema, intent heuristic or selector workflow is introduced. Task31 includes actual-source controls for both cases.

## 34. History ambiguity outcome

> Ruling: give source-proven history ambiguity a safe semantic outcome in the existing gateway/tool error path — actual source inspection confirms generic errors currently become UNKNOWN, hiding the reason and blocking useful model correction — cost if wrong is one internal outcome enum/error mapping addition. Approved ErrAmbiguousHistory→OutcomeAmbiguous→AMBIGUOUS_TARGET with static guidance and native/compat coverage; cancellation precedence, positive receipts, observed facts and partial/unknown non-retry barriers must remain. No raw selector/SQL, zero-count inference, new schema/state-machine or unrelated classification changes. Updated Task31 plan/brief before worker continues this extension.

## 35. Concrete identity owners

> Ruling: extend exact nickname ownership through concrete current-room and AFK owners and all snapshot source boundaries — final composition review proves earlier command/remote-roster fixes left owner-level credential merging and repeated mention normalization intact — cost if wrong is a narrower identity compatibility change. Keep global IdentityKey unchanged, active-roster exact-name replacement, AFK same-name replacement and chat removal requiring exact name plus trip; source-owned names never lose a mention marker. This enforces source/target correctness, not model intent.

## 36. Bounded final-review repairs

> Ruling: address all six bounded Minor findings in the single final fix wave alongside the five Important defects — existing test hooks/setters/grammar suffice without new production machinery — cost if wrong is limited fixture/API/help churn and a bounded (not universal) negative callback observation window. Preserve independent-recipient and cancellation controls, and accepted non-failing Darwin tooling warnings.

## 37. Recoverable audit-workspace cleanup

> Ruling: after a clean final gate, remove this plan from active scratch by archiving its exact owned directory rather than irreversibly deleting it — three user-deferred automove reproductions and full verification evidence otherwise exist only there; higher-level guidance prefers recoverable cleanup — cost if wrong is retained local disk usage. Preserve sibling plans and all repository/user files; the tracked decision log and final report remain the user-facing record.
