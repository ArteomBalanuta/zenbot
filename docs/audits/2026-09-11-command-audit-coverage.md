# Teardown & Rebuild: command harness audit

Audit completed on local `master`, baseline `3c11916` through code commit `6f830bc`. All confirmed in-scope findings are repaired and independently reviewed. The full uncached suite, full vet and affected race tests pass. DBZ and automove remain excluded as requested. The traces below document failures found during the audit, not defects claimed to remain in the final code.

## Coverage accounting

The catalog has 64 identities: 23 moderation/identity, 23 utility/messaging, 17 runtime/game, and `l`. Legacy `unlock` is an additional implemented route outside that catalog. `mine` and `whiskey` are catalog placeholders, not registered production commands; `ws` and `wsa` are manual-only. The other 59 catalog identities are exposed as agent tools when capability policy permits. Capability composition further limits the manual registry.

The user subsequently excluded DBZ-related commands and then automove for now.
Remaining audit scope is 57 catalog identities plus legacy unlock; 52 of those
catalog identities have agent tools. The seven excluded identities and every
alias are listed separately below. Existing DBZ commits are retained. Automove
production code is unchanged; its new audit regressions are deferred outside
the active suite. Neither excluded family requires further targeted work.

| Family | Exact identities | Evidence |
|---|---|---|
| Moderation / identity (22) | active, authorize, ban, captcha, color, deauthorize, flair, kick, messages, lock, mute, nuke, overflow, register, remove, resurrect, shadowbanlist, shadowban, unbanall, unban, unmute, unshadowban | [Command-by-command audit](2026-09-11-moderation-command-audit.md) |
| Automove (1), deferred by user | automove | No further configuration, join automation, or replica reconciliation work for now. |
| Utility / messaging (23) | afk, ape, coin, help, crashcourse, info, lastonline, nicks, list, mail, msgchannel, note, notes, ping, users, say, sub, time, unsub, version, weather, ws, wsa | [Command-by-command audit](2026-09-11-utility-command-audit.md) |
| Runtime (11) | access, memory, mine, prefix, replica, replicaoff, replicastatus, restart, shutdown, sql, whiskey | [Command-by-command audit](2026-09-11-runtime-game-command-audit.md) |
| DBZ (6), excluded by user | dbzstr, dfight, dbzhelp, dbzregister, dspawn, dbzstats | Prior findings/commits retained as historical evidence; no remaining audit or repair obligation. |
| Agent entry (1) | l | Shared-path findings below; typed direct submitter, trusted invocation factory, process-owned runtime, native loop, final delivery and persistence traced. |
| Additional legacy identity | unlock / unlockroom | Task21 removed the unused handler/factory branch; Task22 fix1 removed the remaining ghost definition. Registered `lock off` is preserved and regression-tested. |

## Shared-path teardown

The traces describe the audited baseline. Current repair/verification status is
listed below; a historical trace is not a claim that the repaired code still
exhibits it.

### S1 — High: original content is damaged by command parsing

Trace: chat listener resolves the leading command using `Fields`; `directLCommand` then separately runs `Fields` and `Join` over the entire message. A multiline instruction, code block, SQL literal, or exact quoted payload loses whitespace before the LLM sees the prompt. SQL independently splits on every occurrence of literal `sql `, so a valid query containing that text is truncated; uppercase or anagram aliases resolve dispatch but fail SQL parsing. Mail, notes, and say also flatten message bodies.

Rebuild: one structural token/tail boundary, preserving body bytes after the command/required positional tokens; no keyword intent reasoning. Canonical dispatch identity is separate from payload text. Add literal forwarding tests with repeated spaces, tabs, newlines, embedded `sql `, Unicode, and aliases.

### S2 — High: successful delivery is confused with successful execution

Trace: shared `reply` drops send errors, while SQL explicitly renders a database error and returns SUCCESSFUL. Conversely, the Saturn tool adapter permits silent success only for kick; color, overflow, nuke, and successful moves can perform their effects without a chat response and are then rejected as unverified delivery. Repository mutations are not all captured at their commit boundary, so a failed acknowledgment can erase evidence of an earlier write.

Rebuild: propagate actual errors, distinguish optional delivery from execution, and record effects at the owning operation boundary. Preserve committed/partial/unknown states across later errors. Do not infer business success from message text or raw legacy status alone. Fix every producer before loosening receipt validation.

### S3 — Critical: public invocation captures private tool bodies

Trace: a public notes request can be changed to whisper privately, but capture still appends the whispered body to model-visible messages. The next public model response or durable public evidence can disclose it. This is independent of direct delivery privacy.

Rebuild: project observations according to trusted invocation visibility and actual output channel. Public model contexts get a non-sensitive private-delivery notice and real counts, not the body or private structured data. Private contexts retain observations. Task 3 owns this boundary.

### S4 — Critical: autonomous review authority exceeds its policy

Trace: `CreateModeration` uses the bot principal with creator trip, ModerationCommands, and a reviewed author. The current prompt permits mute only, but catalog availability also exposes unrelated public mutations and room-wide moderation. A model can purge the creator's notes or invoke a room-wide nuke. Target checking only applies to catalog entries marked as targeted, and nuke actually permanently bans despite requiring only ordinary moderation authority.

Rebuild: enforce the current fixed scope in visibility and execution; autonomous reviews may select a mute of the reviewed author or no action. Read tools remain available. Direct user capabilities are unchanged. Nuke requires permanent-ban authority and an honest description. This is authorization, not a deterministic abuse classifier.

### S5 — High: temporary workflows publish completion before finishing

Trace: coordinator marks Completed and removes the workflow before Apply; timeout and Cancel cannot interrupt the effect loop. Nuke continues bans after the caller deadline. Success is published before Flush, whose error is ignored. `OutcomeAbsentTarget`, `OutcomeEmpty`, and `OutcomeSkipped` are not distinctly interpreted by gateway capture. Session factory creation and timeout can race with state assignment. A gateway cancellation can return while detached effects still run.

Rebuild: request-owned temporary context, one operation owner, explicit executing state, cooperative checks before every write/wait, flush before terminal success, exactly-once completion, and truthful partial/unknown receipts. Synchronous agent actions await operation shutdown. Permanent replicas have a different, controller-owned lifetime.

### S6 — High: shared maps escape their owning locks

Trace: writers mutate active users under usersMu, but getters return the live map and user pointers. Contains-mode moderation and listeners iterate concurrently and can race/panic. AFK state has no mutex. Trip subscription writes use exact keys while lookup/removal case-fold credentials.

Rebuild: detached synchronized snapshots, copied input/output user records, AFK locking without network I/O under locks, and exact trip identity. Task 2 owns this boundary.

### S7 — Medium: parallel legacy implementations drift from registered handlers

Trace: concrete NewInstance reconstructs from the first alias instead of canonical; DBZ cloning omits canonical. Several legacy wrappers duplicate parsing and discard errors; legacy List creates its own zombie connection and closes a callback channel that a late event may still write to. These wrappers are not the current production typed route, but remain callable.

Rebuild: preserve canonical identity in cloning and route retained compatibility wrappers through the same context-aware handlers. Remove unreachable alternative implementations where no supported consumer exists. Do not count dead fallback behavior as a live production incident.

### S8 — Medium: dispatch routes have different lifecycle/error behavior

Trace: public command dispatch enters the lifecycle gate; whisper dispatch does not. Both accept a context-aware adapter with no returned error, and handler failures remain logs only. Restart/shutdown source commands return success for request admission while callback errors are discarded; the agent tool's zero-receipt check rejects this as unverified rather than establishing runtime completion. Optional capability decorators can make unavailable handlers look executable. Catalog placeholders are not registered production tools.

Rebuild: a shared contextual dispatch outcome path, explicit admitted-versus-completed lifecycle results, observable callback failures, and consistent capability validation. Audit logs must not turn a completed action into a repeatable failure, and logging failure must not erase receipts.

## Repair status

| Unit | Status |
|---|---|
| Mail identity, public history, private notes | Task 1 committed `955b736`, independently approved. Original privacy reproductions pass; later delivery/context integration is covered by Tasks3/11/15/25. |
| Room/AFK state and exact subscriptions | Task 2 committed `5a343b0` plus nested-JSON repair `cbab13c`; independent re-review approved. All six ownership boundaries and affected race tests covered. |
| Private model observations | Task 3 committed `bfecf24`, independently approved. Snapshot receipt timing is covered by Task8. |
| Autonomous action scope and nuke capability | Task 4 committed `86f02e5`, independently approved. `nuke` requires `AgentPermanentBan` and explicitly describes permanent roster bans plus locking; autonomous review exposes only matching mute and native reads. |
| Narrow transactional identity deletion | Task 5 committed `71524ef`, independently approved; real H2 rollback/sharing tests pass. |
| Utility service correctness | Task 6 committed `4934f28`, `d71d5fb`, `8d1de3f`; independent re-review approved clock/solar coherence, skipped-midnight offset handling and bounded JSON reads. |
| Replica lifecycle | Task 7 committed `63190b7`, `c4a6c51`; independent re-review approved owned lifetimes, configured capability checks and independent-cancellation cleanup. Host lifecycle is covered by Tasks18/24. |
| Moderation syntax/status | Task 17 committed `5e4e063` + `076ebab`, independently approved: complete selector validation, canonical deduplication, truthful no-match status and source/raw identity separation. Downstream wire identity is covered by approved Task26. |
| Snapshot lifecycle/receipts | Task 8 committed `4d84054` plus `2f27ad5`; independent review approved cleanup-first terminal ownership, cancellation through delivery and explicit counts. |
| DBZ | Excluded from further work by user. Prior `d7cf005`/`e444a18` source changes remain; scoped review approved. No additional DBZ receipt integration required. |
| Text parsing and canonical clones | Task 10 committed `90caf6f`, independently approved. Significant text/SQL bytes preserved, clone identity fixed, unused Say/AFK duplicates removed. |
| Per-recipient mail delivery | Task 11 committed `af16754`, independently approved: durable exact-recipient attempts, ambiguous-send no replay, context-aware source operations. |
| Exact credentials and atomic grants | Task 12 committed `452531f`, independently approved; root registration cancellation control passes. |
| Audit-row identity | Task 13 committed `c126ab3`, independently approved: actual insert-owned keys, concurrent real-H2 attribution tests. |
| Moderator read failures/history | Task 14 committed `5b69c7d`, independently approved: operational errors, positive counts, bounded UTF-8, cancellation and public-only history. |
| Contextual dispatch/audit privacy | Task 15 committed `770b49e`, independently approved. Shared leases, typed outcomes, caller context and private audit boundary. |
| Committed source mutations/reply errors | Task 16 committed `8f04981` + `24a246c`, independently approved after one fix round; committed counts survive errors, cancellation and panic without invented zero-row effects. |
| Host lifecycle admission/ownership | Task 18 committed `fb7929c` plus `7e6537d`; independent re-review approved owned admission/publication and stable failed-start cleanup errors. Gateway/request integration is covered by Tasks19/24. |
| Source data after failed delivery | Task 20 committed `73ee6e4`, independently approved: validated snapshot observations survive error without changing unknown/nonretryable status. Non-snapshot text producers are covered by Task25. |
| Configured availability and gateway admission | Task 19 committed `a087397` + `a216903`, independently approved after source-selection repair: configured visibility, one pinned owner, shared admission and typed-nil fallback parity. |
| Remaining legacy wrappers and runtime labels | Task 21 committed `4931494`: seven unused wrappers and duplicate preliminary registrations removed, supported aliases retained, Go runtime values labeled in MiB. Task22 fix1 removed the ghost definition fallback that this unit's initial report incorrectly claimed was already gone. |
| Registry ownership and ambiguity | Task 22 committed `3d9aefe` + `0f4881`, independently approved: atomic alias ownership, detached snapshots, validated catalog-only resolution and propagated composition errors. No current static catalog collision was found. |
| Shadow-ban matching | Task 23 committed `7849d35`, independently approved. Exact repository existence lookup isolates valid identities from unrelated corrupt hashes without hiding source errors. |
| Lifecycle request semantics | Task 24 committed `6049c96`, independently approved: typed admission/coalescing, actual receipts and synchronous acknowledgment; completed retirement is not promised. Explicit source rejection and uncertain cancellation remain distinct. |
| Text observations after failed delivery | Task 25 committed `34612c1`, independently approved: explicit successful source/runtime text survives failed delivery, with closed schema validation, trusted private filtering, bounded projection and full retrieval. Source errors and mutation acknowledgments are not relabeled as read data. |
| Canonical target through wire serialization | Task 26 committed `6841395` + `beb54c7`, independently approved. Direct serving-room moves resolve the nickname on the selected host/replica before the canonical wire handoff; absent users on an owned source cannot fall back remotely. |
| Transport request receipt semantics | Task27 committed `0922a85`, independently approved: source acknowledgments, catalog and shared model guidance distinguish request submission from remote application without changing counts or execution. |
| Live help truthfulness and formatting | Task28 committed `e337220` + `5eaaf19`, independently approved: unsupported routes, stale memory/lifecycle/authority descriptions and real-newline alignment corrected. Both nuke stages are requests; excluded entries retain their meaning. |
| Distinct room occupant counts | Task 29 committed `f4007e1`, independently approved: count exact observed nicknames instead of merging shared trip/hash identities. Do not infer unique humans or guess bot exclusions. |
| Remaining utility input boundaries | Task30 committed `3fee96b`, independently approved: reject trailing notes-purge operands, require the actual active AFK caller and preserve literal room destinations after one leading link marker. Real H2/source regressions and affected race/vet gates pass. |
| Historical nickname/last-online semantics | Task31 committed `8806152`, independently approved: exact public nickname history and independent message/presence facts replace credential merging, private-only aliases and inferred session duration. Safe source-owned ambiguity feedback preserves cancellation and receipt boundaries. Affected race/vet passed; no session tracker or schema expansion. |
| Final composition repairs | Committed `6f830bc`: canonical reviewed/snapshot targets, concrete roster/AFK owners, shadowban batch acknowledgment and six smaller contract/test fixes. Full suite/vet/affected race passed; independent scoped re-review approved all eleven findings with no new breakage. |

No live bot actions, deployment, process restart, production database mutation, or push has been performed. All numbered in-scope repair units are independently approved. The final whole-branch review's five Important composition defects and six Minor issues are repaired in `6f830bc`; the single scoped re-review approved all eleven with no new breakage. See [verification](2026-09-11-command-audit-verification.md) and the [complete chronological rulings](2026-09-11-command-audit-rulings.md).

Final-review composition findings repaired by that wave: canonical reviewed-author authorization
strips a source marker; snapshot nuke/remote move still normalize source names;
current roster joins still merge credentials; concrete AFK storage still changes
same-trip aliases; successful contains shadowban has no required delivery.
Smaller fixes cover stop-callback test observation, final mail ACCEPTED commit
response-loss coverage, websocket event/cleanup synchronization, the compatibility
prefix lock, native list's leading-marker current-room bypass, and shadowban help.

The earlier private-mail replay/cancellation and info/whisper context REDs are
now fixed by independently reviewed Tasks11/15. They must not be counted as
remaining defects.

## Context and correction evidence

Focused uncached tests passed for three-round original-request/receipt retention
under forced pruning, terminal count/error facts, a native three-call compound
flow, bounded correction feedback, duplicate call-ID repair, typed argument
correction and finite failure budgets. Model requests retain the original text
and execution facts; no semantic planner or obligation reducer is introduced.
Scripted model fixtures verify harness behavior, not arbitrary model reasoning
or proof that every final response fulfills the user request.

The production inventory excludes `run_command` when concrete command tools are
registered. Its presence in compatibility tests does not establish live routing
overlap. Task19 now verifies configured-capability visibility and admission.

### Routing and schema findings: what is and is not established

The production-composition test enforces one declared primary owner for each of
these potentially overlapping operations:

| Requested evidence | Exposed owner | Boundary |
|---|---|---|
| Current-room presence | `room_users` | Empty arguments; trusted caller room, not an arbitrary room. |
| Other-room presence | `saturn_list` | One required `room` token; live remote snapshot, not database history. |
| Named user's public history | `user_message_history` | Public historical evidence, not proof of current presence. |
| Nicknames associated with a trip | `saturn_nicks` | Trip-based history, not a live-room roster. |

`saturn_ping` also takes empty arguments because its target is fixed to
hack.chat:80. Weather/time use a required `location` string; neither requires
an invented room/host field or a reducer-derived subject. The current model
policy states these distinctions explicitly. The model still selects the tool:
declared intent ownership is inventory validation, not keyword routing or a
proof that an arbitrary model will never select the wrong valid tool.

The inspected common argument contracts are shallow objects: `{}`, one-string
operations, or a few named positional fields. Schema generation and command-tail
encoding share the catalog field definitions. Encoding rejects unknown/missing
fields, invalid types/tokens and multiple JSON values, and caps encoded command
tails at 4,000 bytes. No evidence supports blaming deep recursive schemas for
the reproduced room-count or canonical-target defects; those were source-data
and serialization bugs. Model-generated invalid arguments remain possible and
receive bounded correction feedback before execution.

Evidence: `cmd/zenbot/live_agent_test.go` checks unique primary owners and absence
of the generic wrapper; `internal/command/catalog/agent_contract.go` owns argument
schemas and encoders; the typed-argument, reused-call-ID and failure-budget tests
in `internal/agent/live/registry_loop_test.go` exercise correction without a
semantic planner. These are harness invariants, not a live-model success-rate
measurement.

### Error feedback, replay and state: established boundaries

The generic executor replaces invocation errors and recovered panics with static
error messages; it does not forward the Go error or panic value as a stack trace.
Known producer errors still need safe adapter-owned content: this is not a blanket
sanitizer for arbitrary tool-supplied strings. History ambiguity now has a typed
source sentinel and static `AMBIGUOUS_TARGET` guidance, without selector/SQL leakage.
See `internal/agent/tool/execution/execution.go` and
`internal/command/public_history_gateway_test.go`.

Correction is bounded, not an apology-and-repeat loop: duplicate call IDs are
rejected before execution, per-tool call/failure budgets limit repeated attempts,
and already-started action keys retain their earlier receipts. UNKNOWN blocks
further actions for that turn while read-only inspection remains available.
That conservative barrier can prevent a remaining unrelated action after an
uncertain write; it is intentional, not evidence that the task completed. Exact
tool name plus canonical JSON arguments is replay protection, not universal
semantic idempotence across aliases or turns. See
`internal/agent/tool/execution/ledger.go` and the failure-budget, reused-call-ID
and typed-argument correction regressions in `registry_loop_test.go`.

Context projection keeps the original request plus a compact, call-ID-linked
result index when older call/result pairs are pruned. Full results remain in the
request-local observation store for retrieval. Three-round tests verify retained
request identity, earlier receipts, mixed successful/failed observations and
valid bounded JSON. They do not prove a future LLM will interpret those facts
correctly, fulfill every requested dependency, or write a complete final answer.
No semantic completion authority has moved into a reducer.

Empty results are interpreted by their source contract, not by HTTP status or
text matching: an observed empty roster or an empty nickname list can be valid
data; an absent required history identity, failed SQL operation, malformed
upstream weather response or missing moderation target cannot establish the
requested action. The repairs below address these concrete producers instead
of classifying phrases such as "no records found" in model prose.

### S9 — High: a failed acknowledgment erases an already observed result

Execution trace:

1. The list operation parses a room snapshot containing two users and builds
   structured room/count data.
2. Final reply delivery fails; the coordinator correctly reports an unknown
   outward outcome after closing the session.
3. Capture/gateway success-only data checks discard the valid observed data;
   the tool error envelope contains `data:null`.
4. Final synthesis cannot use the count even though the source observation
   succeeded, and repeating the action is correctly forbidden.

Rebuild: producer-marked observation validity, optional structured data on error
results, declared-schema validation, private filtering and existing bounded
projection. Keep ACTION_OUTCOME_UNKNOWN and receipt counts unchanged. Reproduced
offline in `TestSourceObservationAuditRetainsObservedRoomCountAfterFailedDelivery`;
Task20 repaired this snapshot path in `73ee6e4`; independent review approved the
producer trust, privacy, validation, bounded projection and no-retry controls.
Other source producers are not implicitly covered by that repair.

### S10 — High: visible tools bypass configured source and host admission

Execution trace:

1. Inventory sees a command type or decorator interface, although the actual
   owner has no configured source or is already retiring.
2. The model selects the advertised tool; the gateway bypasses the manual
   dispatch lease and executes source work against that owner.
3. A known pre-work configuration failure becomes an unknown action outcome;
   in an owner-denied `say` reproduction, an actual send occurs.
4. A typed-nil preferred repository can also mask a working fallback: admission
   accepts the fallback, but source code's plain interface comparison selects
   the unusable preferred repository.

Rebuild: one source-owned structural availability predicate, advisory visibility,
one pinned execution owner, last-moment authorization/configuration validation,
and the existing host lease through source work, audit and receipt finalization.
Source selection must use the same typed-nil rules, without retrying a fallback
after a genuine query failure. Task19 and its source-selection fix implement
these boundaries; independent re-review approved both.
Evidence: `TestGatewayAdmissionAudit` and `TestDependencySelectionAudit`.

### S11 — Medium: registry ownership is mutable and ambiguous

Execution trace:

1. Register one command under `abc`.
2. Register another command with aliases `new, abc`: the live engine overwrites
   the existing owner and publishes the other alias without an atomic rejection.
3. Alternatively register `bca`; a later `cab` lookup can choose an owner by
   map iteration rather than a unique contract.
4. Independently, an inventory consumer can delete or inject live commands
   through `GetEnabledCommands`, or mutate retained definition alias slices.

Rebuild (Task22, independently approved): reject cross-owner exact/anagram collisions before
publication; retain unambiguous compatibility lookup; use a validated immutable
catalog and detached runtime snapshots; propagate registration errors to
composition. Constructors execute outside registry locks. The actual core and
common `TestRegistryOwnershipAudit` regressions fail on all four boundaries.
There is no demonstrated collision in today's static command catalog.

### S12 — High: a canonical moderation target changes at serialization

Execution trace:

1. A command receives `@@alice` and correctly removes one raw mention marker.
2. Active-user selection resolves the literal canonical nickname `@alice`.
3. The real core serializer normalizes again, sending a request for `alice`.
4. Ban/overflow additionally skip canonical active-user selection, accepting
   absent users or extra operands and transmitting caller spelling directly.

Rebuild (Task26, independently approved): normalize raw operands once, resolve the active
identity, and make typed `NickTarget` carry that identity literally through
JSON serialization. Validate exact operand count and absence before effects.
Evidence: `TestModerationTargetBoundaryAudit` forwards registered commands into
the actual core serializer rather than stopping at a command fake.

Final composition review additionally found that the snapshot nuke source still
normalizes each canonical nickname and that remote move normalizes its selector
twice, then normalizes source names during comparison. With `[alice,@alice]`, a
request for `@@alice` can act on `alice`. The combined final fix in `6f830bc` extends the
literal-source boundary through registered remote command→snapshot→decoded wire
payload, preserving cancellation and receipts. The direct-path Task26 approval
alone did not establish that integration.

### S13 — High: lifecycle admission is lost or confused with completion

Execution trace:

1. Restart/shutdown submits work to the real lifecycle owner while a dispatch
   lease prevents retirement from starting.
2. Source code returns success for request admission but sends no acknowledgment
   and provides no request mutation receipt or admission data.
3. The tool returns `UNVERIFIED_ACTION_OUTCOME` with `data:null`, hiding what
   actually happened. Waiting for retirement inside that same dispatch would
   instead deadlock its own lease.

Rebuild (Task24, independently approved): expose source admission/coalescing through a read-only
operation handle; count queue insertion once; snapshot admission facts before
acknowledgment; return a synchronous request acknowledgment without waiting.
Completion is `not_observed` unless an already-finished source handle proves
otherwise. Post-admission cancellation remains unknown/nonretryable with facts
retained. Shutdown describes the host, not the whole process or its replicas.
Evidence: `TestLifecycleRequestAuditAcceptedRequestRetainsAdmissionEvidence`.

### S14 — High: an outbound write is described as remote application

Execution trace:

1. A moderation operation serializes and writes a request to the transport.
2. The transport returns nil; no correlated server application acknowledgment
   is awaited by this source path.
3. Command acknowledgments use past-tense applied-action language, which the
   final model can repeat as proof that the server completed the action.

Rebuild (Task27, independently approved): state request-submission semantics in command replies,
catalog descriptions and shared result interpretation. Keep actual source and
delivery counts, but explain their boundary. Local database commits and completed
snapshot collection retain their stronger evidence; neither a local reply nor a
roster guess proves that the server applied a particular moderation request.
No automatic replay or additional semantic completion judge is warranted.
Evidence: `TestModerationRequestReceiptAuditWriteOnlyBanClaimsSubmissionNotApplication`
records one outbound ban frame, zero inbound reads, and the unchanged actual
request/delivery counts; only the applied-action claim fails.

### S15 — High: credential deduplication undercounts a room roster

Execution trace:

1. A parsed room snapshot contains two different nicknames sharing a tripcode,
   or sharing a network hash without a tripcode.
2. The list producer deduplicates using a global identity helper that prefers
   tripcode, then hash, before nickname. One observed occupant disappears.
3. Both the rendered list and typed count report the smaller set. A later model
   multiplication faithfully computes from incorrect source counts; reasoning
   cannot recover the missing occupant from the supplied observation.
4. Conversely, repeated records for one exact nickname can survive if their
   trip/hash metadata differ.

Rebuild (Task29, independently approved): deduplicate the list producer by exact source
nickname only, preserve the first record and stable ordering, and derive both
count fields from the emitted roster. Keep the global credential identity helper
unchanged for authorization and other consumers. This counts observed nicknames,
including bots present in the roster, not unique humans. Parse-to-operation and
concrete gateway/tool regressions cover successful and failed-delivery results.

### S16 — Medium: static help misrepresents the live command surface

Execution trace:

1. A caller requests help and receives entries for unimplemented routes such as
   mine/whiskey, obsolete JVM memory labels and overstated lifecycle scope.
2. The caller follows a documented route or no-argument move shortcut that the
   registered implementation does not support, or expects whole-process shutdown
   from a host-only request.
3. Help alignment additionally splits escaped newline text while its constants
   use real newlines, so multiple rows are formatted as a single line.

Rebuild (Task28, independently approved): correct the existing static reference against registered
in-scope commands, retain the valid move/recover/heal/resurrect aliases with their
actual required arguments, describe Go memory in MiB and lifecycle admission
accurately, and align real Unicode rows. Preserve private delivery, source
observations and excluded entries' meaning. This is a content/formatting repair,
not a new help framework or dynamic capability filter.

### S17 — High: malformed utility requests still mutate state or claim success

Execution trace:

1. A manual caller sends `notes purge keep-this` or `notes clear unexpected`.
2. The handler inspects only the first operand, deletes the caller's stored notes,
   and reports success. Extra input never reaches validation.
3. Separately, AFK matches all active users by trip alone: an absent caller can
   mark another same-trip nickname away, or change nothing and still receive a
   success acknowledgment.

Rebuild (Task30, independently approved): validate the whole notes grammar before mutation and
resolve one active caller by trusted nickname plus exact trip before AFK changes.
Recheck cancellation before the source mutation; retain actual receipts on later
acknowledgment failure. Explanatory delivery is not proof of a state change.
`TestUtilityBoundaryAudit` reproduces real H2 deletion and the invalid AFK states.

### S18 — High: room normalization silently redirects a message

Execution trace:

1. A caller chooses `other?room` as the msgchannel destination.
2. The handler removes every question mark, submitting `otherroom` instead.
3. The snapshot infrastructure faithfully executes against that different target.
   Meanwhile `list ?lounge` retains the leading marker and selects `?lounge`.

Rebuild (Task30, independently approved): normalize exactly one optional leading raw room-link
marker for list/msgchannel, preserve the remaining destination bytes and validate
the complete list operand count before effects. Never normalize source-owned
channel metadata or infer the intended room from punctuation. The regression
asserts the actual submitted target; it makes no assumption that the remote
server accepts every tested name. Unsupported destinations must fail as selected,
not be silently rewritten to a different destination.

### S19 — High: history queries merge credentials and fabricate an active session

Execution trace:

1. `nicks` lowercases the requested trip and the stored trip column, merging
   distinct credentials. Its unfiltered message source can expose aliases learned
   only from whispered messages; its descriptor incorrectly calls them registered.
2. `lastonline` requires a public non-presence message before it acknowledges any
   observation. A join-only or leave-only identity disappears.
3. It then selects the latest join independently, ignores later leave events and
   renders now-minus-join as session duration, increasing even after departure.
4. The fallback source has still different semantics and can return a successful
   empty-looking report for an absent identity. Retaining this text faithfully in
   model context cannot repair the incorrect source claims.

Rebuild (Task31, independently approved): exact-trip public nickname history, including recognized
presence observations, with stable ordering. Return independent timestamped public
message and JOINED/LEFT facts through one record/renderer for both source paths.
Report last observed time, not current membership or continuous duration. Preserve
public-only filtering and reject cross-kind nickname/trip collisions that would
combine different public matching row sets. No new tracking state, database schema
or semantic planner is needed; existing persistence already contains the facts.

### S20 — High: reviewed-author authorization rewrites its trusted subject

Execution trace:

1. Autonomous review carries the source-owned canonical author `@alice`.
2. TargetAllowed strips a leading `@` from both that trusted subject and the raw
   requested operand. An operand `alice` passes the comparison.
3. Source target resolution finds the distinct active nickname `alice` and sends
   its mute request. Conversely, the correct raw operand `@@alice` is denied.

Rebuild (`6f830bc`, independently approved): normalize the raw operand once, keep
the trusted canonical author literal, and retain established nickname comparison
semantics. Test competing names through real gateway/native target selection;
wrong subjects produce no moderation request. This is an authorization identity
boundary, not a model-driven judgment about whether a message deserves moderation.

### S21 — High: concrete room/AFK owners still merge different nicknames

Execution trace:

1. An initial roster contains Alice. A fresh join arrives for Bob, sharing Alice's
   trip or anonymous network hash.
2. EngineImpl.AddActiveUser replaces by global credential identity, evicting Alice.
   Current-room tool counts now disagree with the corrected remote snapshot count.
3. Independently, Alice is AFK. Bob setting AFK replaces Alice's reason, or Bob
   chatting clears her AFK state because RemoveIfAfk uses nickname OR trip.
4. Correct command-level caller selection cannot prevent these owner-level writes.

Rebuild (`6f830bc`, independently approved): exact observed nickname ownership for
the current roster and AFK replacement; chat removal additionally requires exact
trip equality. Preserve distinct names, literal source bytes, detached copies,
locks and the global credential helper. Use actual engine-backed commands,
message state updates and room directory/tool counts, with preexisting aliases.

### S22 — High: a successful batch shadow ban is reported as unverified delivery

Execution trace:

1. A valid `shadowban -c <fragment>` selects several active nicknames.
2. Every local persistence write and kick submission succeeds; their receipts exist.
3. The contains branch returns success without any acknowledgment, although its
   native contract requires room delivery. The adapter emits
   `UNVERIFIED_ROOM_DELIVERY` after the effects.

Rebuild (`6f830bc`, independently approved): one truthful summary acknowledgment
after the batch, distinguishing local persistence from submitted kick requests.
Do not loosen the whole command's delivery requirement or invent server receipts.
Cover success, zero matches, partial failure, cancellation and failed final
acknowledgment through the source/gateway/native path; prior receipts must survive.

### S23 — Medium: equivalent room operands bypass the declared tool owner

Execution trace:

1. The native remote-only list guard compares `?current` directly with `current`.
2. It admits the call, then the source parser removes the single room-link marker.
3. The command executes the local-list path, bypassing the remote-only owner and
   its declared structured observation contract.

Rebuild (`6f830bc`, independently approved): use the same single-leading-marker
operand interpretation before native admission. Compare against literal trusted
room metadata; retain `room_users` as the current-room tool and `saturn_list` as
the other-room tool. Do not add keyword routing or normalize source metadata.
