# Complete Command Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Audit the complete command surface and repair verified defects with regression evidence.

**Architecture:** Keep the existing catalog and model-driven tool loop. Repair authorization, data ownership, state transitions, and service outcomes in their owning layers. Independent command families may execute concurrently with explicit file ownership; shared gateway changes are integrated after those contracts are understood.

**Tech Stack:** Go, H2 SQL fixtures, native tool contracts, context-aware transport and snapshot coordinator.

**Spec:** `docs/superpowers/specs/2026-09-11-command-audit-design.md`

## Global Constraints

- The LLM remains the primary reasoning and decision-making driver.
- Do not introduce a semantic planner, task-obligation reducer, keyword intent classifier, or prose completion judge.
- Do not deploy, restart the live bot, run live moderation, push, or rewrite Git history.
- Preserve the current documentation cleanup.
- Compatibility does not justify fabricated successes, disclosure of private data, or state corruption.
- Each fix starts with an offline failing regression.
- User scope update: exclude all DBZ-related commands and aliases from further audit, repair, receipt integration and completion requirements. Keep existing commits unchanged; no revert was requested.
- Further user scope update: also exclude automove and its aliases for now, including configuration, join automation, replica reconciliation and command-specific receipt integration. Preserve current production behavior and defer its new audit tests outside the active suite.

## Task 1: Protect mailbox identity and private command data

**Files:** `internal/service/services.go`, `internal/repository/h2/user_queries.go`, `internal/command/mail_notes.go`, and their focused regression tests.

**Interfaces:** Preserve public service method signatures. `MailService.Pending(name, trip)` must only select a resolved trip recipient using nonblank exact `trip`; registered nicknames are resolved at queue time, not treated as delivery credentials. Preserve legitimate public-mail semantics where the stored visibility explicitly permits it. Last-online results must exclude private message text. Notes listing always whispers.

- [x] Run existing red tests in `internal/service/utility_audit_regression_test.go` and `internal/command/utility_audit_regression_test.go` with focused `-run 'PrivateMail|NotesArePrivate|LastOnline'`. Extend tests to cover the legitimate exact-trip recipient, anonymous claimant, public history, and successful private delivery.
- [x] Change the query/formatting boundaries, not model prompts. The required mailbox assertion is:

```go
pending, err := mail.Pending("AbC123", "attacker")
if err != nil || len(pending) != 0 { t.Fatalf("private mail identity bypass: %v %v", pending, err) }
```

- [x] Run focused service, command, and H2 tests; document RED/GREEN evidence and inspect the diff. Do not commit unrelated audit tests or documentation.
- [x] Obtain independent spec and code review before marking complete.

## Task 2: Make engine user and AFK reads ownership-safe

**Files:** `internal/core/engine_impl.go`, focused new core state tests; other callers only if a concrete pointer-identity dependency is found.

**Interfaces:** Keep `GetActiveUsers`, `GetActiveUserByName`, `GetAfkUsers`, `AddAfkUser`, `RenameAfkUser`, `RemoveIfAfk`, and `NotifyAfkIfMentioned` signatures. Return detached maps/user records, copy incoming user records, guard AFK state with its own mutex. Release locks before chat output. Empty-trip users must not match every message through `strings.Contains(text, "")`.

Also make SubscribeTrip, IsSubscribedTrip, and UnsubscribeTrip consistently use exact trimmed trip identities; a case variant cannot read or remove another credential's subscription. Update the concrete snapshot pointer-identity assertion in `internal/factory/temporary_isolation_test.go` to verify isolated contents instead of requiring shared pointers.

- [x] Write and run failing ownership tests using `EngineImpl{}` and literal user records. Mutating returned records/maps must not alter engine state:

```go
e := &EngineImpl{}
e.ReplaceActiveUsers([]*model.User{{Name: "alice", Trip: "trip"}})
for u := range *e.GetActiveUsers() { u.Name = "changed" }
if e.GetActiveUserByName("alice") == nil { t.Fatal("caller mutated owned active user") }
```

- [x] Add concurrent readers/writers for active and AFK state. Implement synchronized snapshots, initialize maps on first add, and avoid locks around sends. Preserve command-visible state semantics.
- [x] Run `go test ./internal/core ./internal/listener/... ./internal/command -count=1` and `go test -race ./internal/core -run 'Active|Afk|State' -count=1`.
- [x] Record evidence, inspect the diff, and obtain independent review.

## Task 3: Prevent private command output entering public model context

**Files:** `internal/command/agent_capture_engine.go`, `internal/command/agent_gateway.go`, `internal/command/agent_gateway_test.go` (or focused new capture-privacy tests).

**Interfaces:** Keep gateway and engine method signatures. The capture engine must know the invocation visibility. It counts successfully delivered private messages as effects, but their body and structured data may only enter an invocation whose trusted context is private. Public invocations receive a fixed non-sensitive delivery notice. Direct sends, addressed sends, explicit whispers, and snapshot completion callbacks follow the same rule. Never infer privacy from body text.

- [x] Write a gateway test invoking notes publicly with a stored secret; the actual send must be private and the gateway messages must not contain the secret. The Task 1 private-listing change is a prerequisite for this integration assertion; independently test capture with explicit whisper sends before that change lands.
- [x] Assert private invocations retain the complete observation, ordinary public sends remain visible, delivery/action counts do not disappear, failed sends create no receipts, and private snapshot `Data` is withheld from public context.

```go
encoded, err := json.Marshal(result)
if err != nil { t.Fatal(err) }
if bytes.Contains(encoded, []byte("private secret")) { t.Fatal("private output entered public observation") }
```

- [x] Implement visibility-aware observation capture using the trusted caller flag and actual output channel. Do not redact the real private delivery; change only what the public model can see.
- [x] Run `go test ./internal/command -run 'AgentCommandGateway|Capture.*Privacy|Private.*Observation' -count=1`, inspect changes and record RED/GREEN evidence. Obtain independent spec/code review.

## Task 4: Enforce the actual autonomous moderation and permanent-ban authority

**Files:** `internal/command/catalog/catalog.go`, `internal/command/agent_catalog.go`, `internal/agent/tool/saturn_command.go`, `internal/agent/tool/run_command.go`, `internal/agent/tool/tool.go`, `internal/agent/tool/execution/execution.go`, and focused policy/gateway tests. No capture-private changes.

**Interfaces:** Catalog `nuke` requires `AgentPermanentBan` and describes permanently banning the room roster and locking the room. The existing `resources/agent/system/participation-moderation.txt` authorizes only `saturn_mute` for the reviewed author; enforce that fixed authority for contexts with `ModerationTarget`, without deciding whether abuse warrants action. Other native read tools remain available. Public/mutating utility commands must not run using the bot's creator trip in an autonomous review.

- [x] Run `TestAuditModerationNukeRequiresPermanentBanCapability` RED. Add tests for reviewed-author contexts attempting nuke, lock, notes purge, register, mail, shadowban, and kick: no side effects; only mute with matching target is admitted. Direct authorized users retain existing capabilities, including creator nuke.
- [x] Make manifest visibility, registry lookup, executor admission, tool direct execution, and command gateway agree on the scope restriction. A small optional tool authorization interface is acceptable; do not encode denied authorization as an invalid descriptor or fake capability.

```go
result, _ := gateway.Execute(ctx, reviewedCaller, "notes", "purge")
if result.Status != commandgateway.OutcomeRejected { t.Fatal("review mutated creator notes") }
```

- [x] Preserve target normalization and exact nickname enforcement for mute. Filter legacy run-command aliases consistently. Do not replace LLM judgment with a trigger or classifier.
- [x] Run focused catalog/tool/executor/gateway capability tests; record RED/GREEN and obtain independent review. Update the inventory's nuke capability/description after code is verified.

## Task 5: Limit identity deletion to the selected relationship

**Files:** `internal/repository/h2/sql_util_group_b.go`, `internal/repository/h2/audit_moderation_repro_test.go`, and deletion/transaction tests.

**Interfaces:** Preserve `DeleteIdentityAuthorized` and `DeleteResult`. Resolve the selector inside the same transaction as the delete, require an unambiguous matching relationship, match trips exactly, remove only that relationship, and delete parents only if no surviving relationships reference them. Do not broaden a nickname selection through a shared trip or vice versa. Preserve the internal authorization gate.

- [x] Run the existing shared-trip RED test; add symmetric shared-name, unambiguous pair, ambiguous selector, cancellation, and rollback tests. Assert unrelated links and parents survive, not merely an error status.

```go
if links != 1 { t.Fatalf("unrelated registration removed: %d", links) }
```

- [x] Use transaction-bound queries and parameterized deletes; guard orphan cleanup against surviving links. Retain affected-row reporting from the actual mutations.
- [x] Run `go test ./internal/repository/h2 -run 'Delete|Identity|AuditModeration' -count=1`, inspect the exact destructive SQL, record RED/GREEN and obtain independent review.

## Task 6: Return truthful weather, time, and ping observations

**Files:** Weather/Time sections and HTTP helper in `internal/service/services.go`, `internal/command/services.go`, `internal/command/user_utilities.go`, utility audit regressions, existing weather/time/ping tests. Keep unrelated mail/identity sections unchanged. A focused weather/time source file may replace the large service-file sections if it avoids further growth without API changes.

**Interfaces:** Preserve WeatherService.Get, TimeService.Get, PingService.Ping, custom endpoint/client test seams, and command identity. Use local defaults rather than mutating shared service configuration on reads. Explicit JSON geocoder requests, numeric forecast values including null, validated provider timestamps/timezones, signed hour-and-minute offsets, and errors for missing/failed providers. Missing optional metrics are unavailable, not zero measurements or year-one dates. Use provider time arrays to select hourly/daily observations; do not assume local process date or array index equals the requested local time.

Provider references checked by root: [GeoNames search](https://www.geonames.org/export/geonames-search.html) documents XML as default and JSON selection; [Open-Meteo](https://open-meteo.com/en/docs) documents numeric arrays, time axes and timezone behavior. Use official documentation only when further verification is necessary; no production service request is required.

- [x] Run remaining `TestUtilityAudit` RED tests for geocoder request, numeric arrays, invalid timezone, fractional offset and ping failure. Extend HTTP fixtures to realistic time-axis shapes, provider business-error/empty payloads, malformed URL/time, missing metrics, and cancellation.

```go
status, err := definition.New(engine, message).Execute(ctx)
if err == nil || status == model.SUCCESSFUL { t.Fatal("failed probe reported as a measurement") }
```

- [x] Propagate service errors and delivery errors from weather/time/ping handlers. An absent service returns unavailable, not a location echo or zero latency. Preserve successful escaped-newline presentation; change assertions requiring fabricated success.
- [x] Run `go test ./internal/service ./internal/command -run 'Weather|TimeService|Ping|UtilityAudit' -count=1` and affected race tests. Confirm no new service field writes race under concurrent calls. Record RED/GREEN evidence and obtain independent review.
- [x] Fix independent review's clock/solar coherence failure: fetch the clock first, pin the solar request to its local date and IANA `timezone` parameter, and validate response date/zone metadata. Derive the displayed current UTC offset from the clock. Cover midnight rollover, conflicting zones, fractional offsets, and a DST-transition date with differing valid daily/current offsets. Bound the shared JSON response reader and reject oversized bodies without unbounded trailing-value allocation. Re-review the fix independently.

## Task 7: Give permanent replicas a safe, functional lifecycle

**Files:** startup/stop regions of `internal/core/engine_impl.go`, `internal/core/replica_controller.go`, `internal/core/replica_manager.go`, production replica composition in `cmd/zenbot/main.go` or a focused helper, and core/composition tests. Preserve Task 2 state changes. Avoid a factory-to-command import cycle: legacy command/List already imports factory.

**Interfaces:** Preserve request cancellation during construction/start, but successful replica lifetime belongs to the manager/controller and survives request cancellation and master replacement. Explicit removal/process teardown stops it. Startup failure returns after bounded cleanup even if no dispatcher was launched. A dispatcher error cannot synchronously wait for itself. Failed registration cleans up the exact created instance; a late failure must not remove a replacement replica for the same room. Production replicas receive supported utility registration before startup, without host-only lifecycle/credentialed remote capabilities.

Registration must also exclude controller-backed commands when the actual controller/relay is nil, even if EngineImpl has forwarding methods. Add a small optional `common.CommandAvailability` owner contract with `CommandAvailable(canonical string) bool`, implement configured-controller checks on EngineImpl, and consult it in the existing registration loop. Keep local prefix support when usable. Do not copy a second inventory or configure host privileges just to make methods work. This adds `internal/common/command_availability.go` and a narrow `internal/command/dispatch_adapter.go` check to owned files; later gateway/capture availability work reuses the same owner contract.

- [x] Reproduce successful transport start followed by failed join send with a controlled transport; assert StartContext returns, Close runs, and no unstarted done channel is awaited.
- [x] Start a replica under a cancelable request, cancel after success, inject a normal inbound command and verify dispatch still works until explicit removal. Inject transport failure and require dispatcher exit, manager removal, and no leaked worker. Cover duplicate concurrent starts and late callback identity.
- [x] Through the production constructor, dispatch a local command in a replica and assert the response, not only its websocket join.
- [x] Add a join-frame regression with quotes and backslashes in room/nickname/password; marshal a typed join payload instead of interpolating raw values into JSON. Assert exact decoded field values and that no user-controlled extra protocol field is introduced.

```go
select {
case err := <-started: if err == nil { t.Fatal("join failure hidden") }
case <-time.After(time.Second): t.Fatal("startup cleanup deadlocked")
}
```

- [x] Implement owned lifetime and non-reentrant cleanup with deterministic completion signaling. Closed inbound/error channels must not spin. Run focused core/factory/cmd lifecycle tests and race tests; document exact evidence and obtain independent review.

## Task 8: Make snapshot completion follow execution, cleanup, and receipts

**Files:** `internal/listener/snapshot/coordinator.go`, `session_factory.go`, remote operation files and tests; `internal/core/room_snapshot.go`; snapshot submissions in `internal/command/{handlers,nuke,resurrect,msg_channel}.go`; snapshot capture in `agent_capture_engine.go`, the gateway and focused tests; snapshot reply composition in `cmd/zenbot/main.go` and its tests. Run after Tasks 4 and 7, which touch gateway policy and production composition. Keep unrelated command/service sections unchanged.

**Interfaces:** Preserve command identities and the shared remote command path. Add a request context to `RoomSnapshotRequest` and an execution context to `RoomSnapshotContext`; nil means a legacy background context, not a reason to ignore an explicit caller context. `ReplySink` must return an error so the coordinator can distinguish attempted output from successful delivery. Extend `OperationResult` with source-owned action/delivery counts and unknown-outcome metadata, then carry them through capture into existing gateway/tool receipts. Do not infer receipt counts by parsing message text or treat session join as the requested action.

Tool projection also needs a single catalog-owned `AgentToolSpec.AllowsSilentAction` field/option. Mark verified intentionally silent `kick`, `color`, `overflow`, `nuke`, and `resurrect`; derive SaturnCommand result mode and result-verification policy from the field, requiring positive action receipts. Preserve delivery-required verification for other commands. Extend owned files narrowly to catalog metadata/the five entries and `internal/agent/tool/{saturn_command,run_command}.go` with tests. Do not broaden RunCommand's allowed aliases. Coordinate catalog.go's separate register-description hunk with Task 12.

- [x] Add controlled-session RED tests: blocked creation canceled before return must never start the returned session; operation-running state remains active; callback and terminal state cannot precede a blocked flush or close; failed flush cannot publish success; `Failed()` with nil Go error is failed; concurrent duplicate snapshots execute once; late callbacks cannot terminate another workflow with a reused ID; retention does not grow without a bound.

```go
select {
case <-completed: t.Fatal("workflow completed before operation/flush cleanup")
default:
}
if coordinator.ActiveWorkflowCount() != 1 { t.Fatal("running operation lost lifecycle owner") }
```

- [x] Refactor the existing coordinator around one execution/cleanup owner. Session fields/state use consistent synchronization. Cancellation and transport callbacks request cancellation without closing or waiting on their own executing callback. Context cancellation remains observable while an operation runs. A terminal outcome is published once, only after operation return and cleanup. Close errors are retained, not ignored. Closed inbound/error channels terminate rather than spinning. Bounded network cleanup must also work when startup never launched a reader.
- [x] Add an operation-panic-after-one-successful-write regression. Recover at the execution-owner boundary, close/flush as appropriate, retain source-observed partial counts even if Apply never returns its result, and publish one non-retryable unknown terminal outcome. A synchronous startup callback must not deadlock its owner.
- [x] Use caller context during temporary factory construction/start and operation writes. Make nuke's inter-action wait context-aware and stop before subsequent writes after cancellation. Count each actual successful operation send, retain partial counts, and mark a failed/ambiguous write or flush unknown without claiming rollback. Do not silently turn remote operation errors into only a failure string. Validate nil users/senders and send the actual snapshot nickname after case-insensitive lookup. Hash the entire workflow ID for temporary nicknames; distinct resurrect IDs sharing their prefix must produce distinct valid nicknames.
- [x] Add gateway integration REDs for silent successful nuke/resurrect receipts, absent targets, empty unsent remote messages, partial nuke sends, failed reply delivery, and cancellation during a controlled send. The canceled agent call must not return before the action owner stops; the resulting tool failure must be non-retryable `ACTION_OUTCOME_UNKNOWN`, retaining known counts. A successful reply callback alone must not manufacture delivery evidence.
- [x] Preserve Task 3 private observation rules for both `Reply` and `Data`; real private delivery is unchanged. Keep list count-zero data truthful. Adapt gateway outcome mapping with explicit coordinator metadata, not a blanket "all errors unknown" or prose classifier; shared non-snapshot command errors remain the later outcome task.
- [x] Run focused snapshot/core/command/cmd tests, then affected race suites and vet. Record RED/GREEN evidence, scoped commits, and independent spec/code review. All transports are fakes or local fixtures; no real room operations.

## Task 9: Make DBZ mutations atomic and command outcomes truthful

**Historical task; excluded from further work by the user's updated scope.**
Existing commits d7cf005/e444a18 remain unchanged. The checklist below records
prior requirements, not remaining work; no additional DBZ receipt integration
or targeted investigation is required for this audit's completion.

**Files:** `internal/service/dbz.go`, `internal/repository/h2/dbz.go`, `internal/command/dbz.go`, focused tests in service/H2/command and message dispatch; repository DBZ errors/interface only if the new atomic operation requires it. No weather/time, engine lifecycle, catalog authority, or shared reply-helper edits.

**Interfaces:** Keep database repository signatures where possible. Replace separate command-side Fight/LevelUp calls with one context-aware service fight-and-reward operation returning whether an enemy was defeated plus an error. Update all actual callers and tests; do not retain a reward-bypassing compatibility command path. Stats are positive 32-bit allocations, cannot overdraw, and repository checks cover direct callers. Registration and both level/reward updates commit or roll back together. Existing help must use actual game identities and the engine prefix.

Reuse H2 `WithTx` instead of a DBZ-only copy. Add generic `repository.ErrCommitOutcomeUnknown` and wrap only failures returned by `tx.Commit`, preserving the original error chain; closure/write failures retain their existing definite-failure meaning. This adds the narrow transaction helper in `internal/repository/h2/audit.go` and focused transaction tests to owned files. An uncertain reward removes/quarantines the selected enemy from availability to prevent blind replay but returns false plus the sentinel, not a claimed verified defeat. The command gives no success acknowledgment; shared outcome mapping must retain the uncertainty.

- [ ] Write RED tests showing nonexistent/repeated enemy fights grant no level or points; two concurrent attempts on one enemy produce one reward; definite failed reward leaves the enemy; canceled calls do not write. Replace compatibility tests currently asserting success on failed Register/AddStrength/LevelUp or treating FreeStats errors as a zero balance.
- [ ] Add real H2 REDs for registration second-statement failure, level/reward rollback, absent character, negative/zero allocation, exact remaining balance, excessive amount, integer overflow, and simultaneous spending. Assert actual before/after rows, not just return errors.

```go
if err := db.AddStrength(ctx, "goku", 6); err == nil { t.Fatal("spent more than five free points") }
stats, found, err := db.Stats(ctx, "goku")
if err != nil || !found || stats.FreeStats != 5 || stats.Strength != 1 {
    t.Fatalf("rejected spend changed character: %+v, %v", stats, err)
}
```

- [ ] Implement transactional registration/reward and a guarded `UPDATE ... WHERE free_stats >= amount` with positive amount and affected-row checks. Apply the same spend contract to all exposed repository stat allocators; restrict dynamic column selection to code-owned constants. Distinguish missing character, insufficient points, invalid input, and actual storage error without interpreting error strings.
- [ ] Make fight consume/reward under one service-owned critical section, with no chat I/O under it. Definite failed persistence retains the enemy; preserve an explicit non-replayable uncertainty if commit success cannot be established. Do not add an LLM planner, game action retry, or unconditional reward shortcut.
- [ ] Commands propagate errors and only acknowledge successful mutations. No successful no-op on an unknown canonical handler. Fix DBZ clone canonical identity and replace fictional help with actual commands/mechanics and current prefix. Missing stats cannot be an action-success claim. Record successful mutation receipts before optional acknowledgment once the common receipt hook is integrated by the shared outcome task; do not fake receipts from success strings.
- [ ] Run focused service/H2/command/message DBZ tests, affected race tests and vet; report RED/GREEN and inspect scoped commits. Independent review must cover transaction rollback, concurrent spending/fighting, and command dispatch through the real adapter.

## Task 10: Preserve command content and canonical execution identity

**Files:** `internal/command/handlers.go`, `mail_notes.go`, `msg_channel.go`, `sql.go`, associated tests including `shared_audit_regression_test.go`; `internal/model/chat_message.go` only if a shared structural token/tail helper belongs there. Run after Tasks 8 and 11 to avoid edits to the same command submission/context regions. DBZ-specific clone repair remains Task 9; no engine lifecycle or tool authority changes.

Verified scope additions: `subscriptions.go` constructor must retain canonical
identity and sub/unsub must be constructible through the same canonical clone
path; cover actual subscription effects. Remove the unused duplicate legacy
`say.go` and `afk.go` classes: a whole-source caller search found only their own
definitions/NewInstance, while the registered canonical handlers retain these
commands and aliases. This removes obsolete lossy parsing implementations; do
not broaden removal to other legacy wrappers without their own caller audit.
DBZ-specific clone work is excluded by the latest scope, regardless of the
historical Task9 reference above.

**Interfaces:** Command lookup resolves identity separately from its body. Strip the leading command token and structural separator, then preserve interior text exactly; commands with required positional arguments consume those tokens without re-tokenizing the remaining content. Outer blank padding may be trimmed where the existing command contract does so, but do not collapse repeated spaces, tabs, real newlines, Unicode, or literal backslashes. This is structural parsing only, never semantic intent routing. Existing valid aliases and anagram resolution continue to reach the same canonical handler.

- [x] Run root's `go test ./internal/command -run '^TestSharedAudit' -count=1 -v` RED reproduction. It proves LLM prompt flattening, embedded SQL delimiter truncation, literal backslash rewriting, uppercase/anagram parser disagreement, SQL error-as-success/raw diagnostic output, failed delivery hiding, and a memory clone resolving to unsupported `mem`.
- [x] Add note/mail/msgchannel/say/AFK body-forwarding tests with literal multiline content through actual dispatch. Preserve service JSON storage escaping as a separate encoding boundary; do not double-escape or flatten stored values. `say` must honor its exact-message contract rather than silently dropping non-ASCII text and punctuation for ordinary authorized callers; still enforce the existing command authorization.

```go
want := "line one\n\tline  two: café \\n"
// Dispatch through the registered handler and assert the exact body reaching
// the actual output/store/submission boundary equals this independent literal.
```

- [x] Implement one structural token/tail helper and reuse it at text-bearing command boundaries. SQL uses that tail exactly, not `Split("sql ")` or global escape replacement. Do not mutate `ChatMessage.Text` to repair one handler.
- [x] Preserve canonical handler and direct-agent submitter when cloning. Every retained command constructor/clone must execute the same operation with its new engine/message; no generic fallback for a first alias. Cover memory/access/shutdown and direct `l` with concrete behavior, not only type identity. Unknown canonical commands fail explicitly.
- [x] SQL query errors return failed status/error without publishing raw driver diagnostics as successful content. Query cancellation emits no success; delivery failure is returned without rerunning SQL. Use the actual send error at this handler rather than broadening the shared reply-helper change before its own task.
- [x] Run focused shared parsing, SQL, mail/note/msgchannel/say/AFK/clone/direct-agent tests and relevant message-dispatch tests. Replace compatibility tests requiring source corruption. Record RED/GREEN, inspect scoped commit, and obtain independent review.

## Task 11: Preserve per-recipient mail attempts and caller cancellation

**Files:** mail/note sections of `internal/service/services.go` (extract focused `mail_notes.go` and `mail_delivery.go` where useful), `internal/listener/message/handlers.go` delivery region, `internal/command/mail_notes.go` service calls/error handling only, H2 schema/bootstrap and mail migration tests, mail/note service/command/listener tests. Run after Tasks 6 and 9; Task 10 consumes the context-aware command calls afterward. Keep engine/snapshot/moderation/game implementation unchanged.

**Interfaces:** Add a service-owned `DeliverPending(ctx context.Context, receiver, trip string, send func(context.Context, model.Mail) error) error`. It owns durable claim/finalization and awaits synchronous sends; the listener owns formatting and actual output visibility. Mail queue/read and note methods take `context.Context`; update every in-repository caller instead of leaving production paths on contextless wrappers. Remove the unqualified `MarkDelivered(id)` path, which cannot express recipient ownership or actual attempt identity. The new delivery table uses `(mail_id, recipient_trip)` as a unique key and a source-owned attempt identifier; states distinguish pending/attempting, accepted and unknown. Reuse generic commit-uncertainty classification from Task 9. If the transaction helper must become reusable by the existing SQL-backed service, extract its one implementation to `internal/repository/transaction.go` and keep H2's method delegating to it, with its existing rollback/error tests retained. Do not import the H2 adapter into the service or duplicate the helper.

- [x] Run root's `go test ./internal/listener/message -run '^TestMailDeliveryAudit' -count=1 -v` RED tests. They verify prior private delivery is not replayed after public failure; a canceled listener sends nothing; two exact recipients each receive once; and an ambiguous send is not replayed through a fresh service instance.

```go
for _, trip := range []string{"trip-a", "trip-b", "trip-a", "trip-b"} {
    _, err := (DeliverPendingMail{}).Handle(ctx, &Context{Engine: engine,
        Message: &model.ChatMessage{Name: "shared-name", Trip: trip}})
    if err != nil { t.Fatal(err) }
}
if engine.whispers != 2 { t.Fatalf("recipient deliveries=%d, want 2", engine.whispers) }
```

- [x] Add real H2 regressions for simultaneous delivery through two service instances, successful send followed by failed finalization, interrupted/persisted attempts observed by a new instance, and cancellation after one confirmed send preventing the next. Assert persisted recipient status and transport count together. Retain exact-trip/case/nickname privacy and explicit public-mail tests. Use controlled local send callbacks; do not call a real room.
- [x] Add the delivery table non-destructively through normal bootstrap. Existing pending CSV-recipient mail remains deliverable independently to each intended trip; existing delivered rows must not be resurrected. Claim under a transaction/unique constraint before sending, using an exact recipient membership check. A claim failure or uncertain claim commit sends nothing. Finalization must match the actual attempt identity; do not let a stale completion overwrite a newer state. Parent mail may be marked delivered only after all intended recipient attempts are confirmed accepted.
- [x] Send one bounded message/attempt at a time, finalize its known outcome, then advance. A send error or persistence error after sending retains an unknown attempt and returns a typed non-replayable uncertainty plus the underlying error chain. Do not reset such an attempt to pending, retry the batch, expire/reclaim the claim, or infer failed delivery from elapsed time. Keep safe identifiers/state queryable for investigation. Definite cancellation before any send must not falsely mark delivery accepted or prevent a genuinely unattempted mail from remaining pending.
- [x] Propagate context through queue resolution, insert, note reads/writes/purge, pending selection and claim. Final receipt persistence after a known send uses a bounded cleanup context if the caller was canceled, while no subsequent send begins. The current engine's synchronous send has its own bounded transport deadline; check caller context before entering it, and use an existing contextual send capability if available without inventing a detached goroutine.
- [x] Replace mail error-string comparisons with typed source errors for blank recipient/unregistered recipient. Preserve actual payload encoding as an output/storage boundary and Task 1/3 privacy. Capture confirmed queue/note mutations through the later common receipt boundary; this task must not claim that integration is done before it exists.
- [x] Run focused mail/note/privacy/listener/bootstrap tests, race tests for concurrent service instances, and vet. Re-run Task 5/9 transaction tests if the helper moved. Record exact RED/GREEN, inspect the scoped commit, and obtain independent spec/code review covering migration, claims, partial delivery, and no-replay behavior.

## Task 12: Make identity and authorization exact, contextual, and atomic

**Files:** `internal/repository/user_queries.go`, `repository.go`, H2 `identity.go` and `authorization.go`, UserService methods in `internal/service/services.go`, `internal/service/security_service.go`, `internal/command/identity_commands.go`, focused identity/security/access tests and actual-call-site adapters. Catalog registration description may be narrowed to its existing implemented semantics; no other catalog edits. Do not edit DBZ, the transaction helper, snapshot/coordinator, or mail/note service regions. Coordinate the register-only fixture in root's `audit_moderation_repro_test.go` with Task 8's separate nuke fixture; do not stage that shared root file wholesale.

**Interfaces:** Add context to all `IdentityRepository` methods and corresponding UserService calls; preserve existing registration operations, making H2 use Query/ExecContext and WithTx(ctx). Extend `AuthorizationRepository` with `GrantTrips(context.Context, []string, model.Role) error`; `GrantTrip` delegates a single-target request to the same transaction implementation. `access` sends the complete validated target set once, not sequential grants. The existing typed room `authorize` operation remains distinct from role grants. No heuristic intent routing or new agent planner.

- [x] Run root's RED tests: `go test ./internal/service ./internal/repository/h2 -run '^(TestIdentityAudit|TestSecurityIdentityAudit)' -count=1 -v` and `go test ./internal/command -run TestAuditModerationRegisterDoesNotMutateAfterLookupCancellation -count=1`. First group proves wrong-case credentials are authorized/linked and missing storage grants TRUSTED authority; second proves cancellation during lookup does not stop registration.

```go
allowed, err := database.IsTripAuthorized(ctx, "abc123", model.ADMIN, []string{"AbC123"})
if err != nil || allowed { t.Fatalf("wrong credential authorized: %t, %v", allowed, err) }
```

- [x] Make trip comparisons exact in H2 lookup/registration and configured admin/user/lifecycle checks. Preserve explicit wildcard and exact-match success controls. Missing-repository fallback uses REGULAR role hierarchy. Add a regression that granting then lowering a persisted role actually revokes its elevated authority; a prior runtime grant must not have inserted it into the configured AdminTrips override. If retained legacy in-memory grants are mutable, synchronize access and return/copy owned configuration rather than racing slice append/read.
- [x] Propagate context end-to-end through identity registration and history APIs, including the command's after-lookup mutation boundary. Add actual canceled-query/transaction tests, exact case-variant trips coexisting independently, ambiguous nickname lookup rejection with no orphan rows, and rollback after a second registration statement fails. Update known compatibility tests that currently expect case-folded credentials; keep case-folded nickname behavior where resolution is unambiguous.
- [x] Write real-H2 access regressions proving an invalid/empty target anywhere changes no role, failure on the second persisted grant rolls back the first, canceled grants do not write, and exact case-variant targets remain distinct. Reject embedded empty CSV elements before any mutation; harmless trailing separators may retain existing compatibility only when at least one valid target remains. Deduplicate exact repeated trips. Bind values and perform every grant in one WithTx transaction, preserving Task 9 commit-uncertainty errors.
- [x] Keep register/access help tied to the configured prefix and real semantics; do not claim two existing identities were linked if the handler rejects that operation. Propagate send errors after successful commands instead of false completion, and explicitly carry the successful mutation boundary into the later shared receipt integration. Do not downgrade a known committed mutation to an automatic replayable failure when only its acknowledgment failed.

The shared committed-mutation receipt hook does not exist yet. Do not invent an identity-only observer or use SUCCESSFUL-plus-error as a new signaling protocol. Keep current handler status/error conventions for acknowledgment failures, document each successful mutation boundary, and record the mandatory later shared-outcome integration as incomplete until the common hook is installed across families.
- [x] Run focused security/identity/access/registration/history/dispatch tests, concurrent authorization race tests, H2 transaction regressions and vet. Record RED/GREEN outputs and inspect scoped commits. Independent review must cover privilege boundaries, exact-trip semantics, canceled mutation paths, atomic batch roles and compatibility adapters.

## Task 13: Return the audit row identity belonging to the actual insert

**Files:** `internal/repository/h2/audit.go` insertReturning region only, `internal/repository/h2/audit_identity_regression_test.go`, existing audit tests as needed. Do not edit WithTx or DBZ identity insertion. This unit is independent of snapshot, identity-service and mail changes except the separate transaction-helper region in the same source file; coordinate that region if Task 11 extracts the helper.

**Interfaces:** Preserve MessageAudit, PresenceAudit and CommandAudit signatures and existing visibility validation. Obtain the generated key from the actual insert statement, not a later table-global MAX(id), pooled connection identity, or process-local mutex. Reuse the H2 inserted-row mechanism already proven in Task 9; keep table/column targets code-owned. Do not turn receipt recording into a retry of the underlying command.

- [x] Run root's `go test ./internal/repository/h2 -run '^TestAuditIdentityReturnedID' -count=1 -v` RED regression. It uses real H2 with a controlled second writer immediately after the first insertion; all three audit tables currently return the second writer's ID.

```go
if err := db.QueryRowContext(ctx, "SELECT name FROM messages WHERE id=$1", returnedID).Scan(&owner); err != nil { t.Fatal(err) }
if owner != "first-writer" { t.Fatalf("audit identity belongs to %q", owner) }
```

- [x] Change insertReturning to fetch the inserted row's key in the same SQL operation. Keep quoted-column inserts, optional/blank fields, validation errors, canceled calls and underlying storage errors correct. Add concurrent public API checks pairing every returned ID with its actual supplied row; do not rely solely on unique positive IDs.
- [x] Run focused H2 audit, visibility, command-record and recent-presence tests; run the affected concurrent tests under race and vet. Assert failed/canceled inserts create no success ID and do not retry a potentially committed insert. Record RED/GREEN and scoped commit; obtain independent review.

## Task 14: Keep moderator read failures and history output truthful

**Files:** `internal/service/activity.go`, `internal/command/activity.go`, the messagesCommand region in `internal/command/identity_commands.go`, relevant service/command tests including `moderator_read_audit_test.go`, and only active's catalog description/argument description. Do not edit registration/access, shared reply helpers, shadow-ban persistence, snapshot lifecycle or excluded commands. Run after approved Task 12; no overlap with Task 13 audit insertion or Task 8 coordinator fix.

**Interfaces:** Preserve Stats and history interfaces and the active tool's existing `identity` argument key, but describe its actual trip-only semantics. Operational read errors return errors, never successful data containing driver diagnostics. A valid empty query remains a legitimate no-records observation. Manual history counts must be positive; preserve the existing harmless clamp above 30 with truthful delivery error handling. Limit each history message to the existing 200-byte bound without splitting a UTF-8 encoding; encode output only after truncation.

- [x] Run root's `go test ./internal/command -run '^TestModeratorReadAudit' -count=1 -v` RED regressions: activity database errors become successful chat data, activity send errors are swallowed, nonpositive history counts reach queries, a byte-200 split corrupts a Unicode character, and cancellation during a successful history lookup still sends output.
- [x] Add direct ActivityService error/cancellation/missing-store cases before fixing it; reject source failures and preserve their error chain without formatting diagnostics as data. Preserve valid table and empty-result behavior. Update compatibility tests explicitly requiring leaked successful errors.
- [x] Return actual activity delivery errors and keep one query/one send per execution. Narrow active's descriptions to exact trip input without changing the existing tool key, inventing nickname lookup, or routing heuristics.
- [x] Validate positive history counts before either primary or fallback query. Recheck caller cancellation after lookup and before every output, including the over-limit warning; propagate each failed send without requerying. Keep both primary GroupB and fallback Identity paths context-aware and PUBLIC-only. Add nil/missing-store, legitimate no-records, supplementary Unicode and exact-limit controls. Empty history must say no records rather than send a blank message.
- [x] Use a byte-budget-aware UTF-8 truncation boundary. Do not change stored text, flatten whitespace, drop valid non-ASCII characters, or manufacture replacement characters. Preserve configured prefix and public/whisper output selection.
- [x] Run focused activity/history/count/Unicode/cancellation tests, affected package suites once, and vet. Record RED/GREEN, inspect a scoped commit and obtain independent review. General tool outcome categorization and shared observation receipts remain later integration work; do not claim them complete here.

## Task 15: Unify contextual public/whisper dispatch and audit privacy

**Integration amendment:** EngineImpl.LogMessageRecord also belongs to this unit: use the legacy repository only for explicit PUBLIC records when no typed repository exists, retaining cancellation checks and private/unknown fail-closed behavior. Never fallback after a typed store error. Shared test stubs may implement typed audit and precancel expectations may change to no admission. Existing DBZ dispatch fixtures may receive only mechanical shared-error-contract expectations; DBZ production and game behavior remain excluded.

**Files:** dispatch branch of `internal/core/engine_impl.go`; `internal/listener/info_chat_listener.go`, `info/handlers.go`, `info/chain.go`, `message/handlers.go` audit/dispatch regions, `message/chain.go`; one focused shared command-dispatch helper in `internal/common` and `internal/command/dispatch_adapter.go` execution adapter; relevant tests including root `shared_dispatch_audit_test.go` in core/info and message `audit_visibility_regression_test.go`. Do not edit mail delivery, host lifecycle's own state machine, snapshot coordinator, tool receipt mapping, Task10 parsing/clones, or excluded commands. Preserve Task10's changed handler behavior and Task14's errors.

**Interfaces:** Existing `Command.Execute()` and optional void `ExecuteContext(ctx)` remain compatibility entry points. Add one result-bearing optional method on the legacy adapter (`ExecuteResult(ctx) (model.Status,error)`) and one common invocation helper used by both public and whisper dispatch. Prefer actual returned status/error when available, otherwise call the contextual or legacy fallback synchronously after cancellation checks. Known FAILED-with-nil-error is a typed rejection, not a fabricated success. This helper is structural dispatch, not LLM reasoning. Both paths acquire/release the existing lifecycle dispatch lease around execution, including error/panic unwinding; do not build another lifecycle controller.

- [x] Reproduce root REDs: `go test ./internal/core -run '^TestSharedDispatchAudit' -count=1 -v`; `go test ./internal/listener/info -run '^TestSharedInfoDispatchAudit' -count=1 -v`; `go test ./internal/listener/message -run '^TestSharedMessageAudit' -count=1 -v`. They prove info dispatch loses caller context, whispers bypass lifecycle lease, a pre-canceled whisper invokes legacy execution, converted whispers lose trusted room, and chat auditing bypasses typed visibility/context.
- [x] Add result-propagation REDs using actual registered Saturn adapters and both public/whisper handlers. A command error reaches the chain without a second execution; a FAILED nil-error handler yields a known rejection. Keep actual authorization and profiling boundaries. Preserve compatibility for legacy void commands without claiming a verified action receipt from their return.
- [x] Give InfoChatListener a NotifyContext path with a synchronous child lifetime like UserChatListener; Notify delegates to background compatibility. The engine info branch chooses contextual notification when available. Normalize nil context and stop both chains before invoking a handler when cancellation is already observed. Add a canceled-between-handlers test proving later output/state changes do not occur.
- [x] Reuse one shared helper for lifecycle lease acquisition and invocation in both dispatchers. Keep the lease until execution and its existing audit path return; release exactly once on normal/error/panic returns. Do not hold locks over execution. Test with controlled callbacks, including a real HostLifecycle worker that cannot restart until a whispered command returns. Existing authorization policy must remain unchanged.
- [x] Converted whisper messages retain the engine's trusted Channel and explicit whisper markers, never an untrusted payload's room. Public/whisper auditing prefers MessageRecordAuditor with the actual caller context and PUBLIC/WHISPER visibility. A legacy logger that cannot represent whisper visibility must never receive the private body as a public record: return a safe audit-capability error and stop that chain. Public-only legacy audit fallback remains compatible. Add actual private-body non-disclosure tests and typed-store-error propagation.
- [x] Preserve original command execution status when a subsequent command-audit write fails; log that separate audit failure without rerunning the command or erasing its effect history. Common execution-receipt integration remains a later task. Return unauthorized-reply transport errors rather than swallowing them. Do not dump raw failures into user-facing chat.
- [x] Run focused context/visibility/dispatch/authorization/lifecycle/adapter tests and affected race checks, affected suites once and vet. Record exact behavioral RED/GREEN and shared fixture adaptations, inspect a scoped commit, and obtain independent spec/quality review. No targeted DBZ or automove work.

## Task 16: Preserve committed mutations across acknowledgment failures

**Counted-delete amendment:** Existing shadow-ban repository removal APIs may return affected counts, with H2 and mechanical caller/test adaptations (including the core reversal adapter). Zero changed rows produce no committed-mutation receipt; a positive known count records one logical operation. Preserve selector grammar for the later moderation unit. Keep a thin reply compatibility helper for excluded callers and use explicit context-aware error propagation for in-scope callers.

**Files:** a small shared context-scoped receipt facility under `internal/common` (or a focused dependency-neutral package if necessary), `internal/command/agent_gateway.go`, `agent_capture_engine.go`, service UserService mutation methods in `services.go`, `mail_notes.go` mutation paths, `shadow_ban.go` mutation paths, access's confirmed grant boundary in `identity_commands.go`, and in-scope command reply callers/tests. Start after Task10 commits to avoid its handlers/mail parsing overlap; coordinate separate common files with Task15. Do not edit Task15 dispatch adapter or host/snapshot lifecycle internals. DBZ and automove production paths and their command-specific receipt integration remain excluded.

**Interfaces:** One request-context recorder accumulates positive known committed-mutation counts; a missing recorder is a no-op. It carries no arguments, subjects, user text or private data, and is never attached to shared service instances. The gateway consumes a detached snapshot on every exit, merging existing action/delivery receipts without double counting. Existing service interfaces remain wherever possible. Access may emit through the same common hook immediately after its confirmed atomic GrantTrips, since that command currently calls the authoritative repository directly. Contextless subscription/AFK operations are recorded at the capture wrapper's synchronous source boundary using actual result semantics. This is not a planner or semantic completion reducer.

- [x] Run root `go test ./internal/command -run '^TestMutationReceiptAudit' -count=1 -v` RED. Real H2 proves register/access/note/mail/offline-shadowban each committed while the gateway reports zero effects after one failed acknowledgment; note/mail/shadowban and subscription also falsely return SUCCEEDED. Preserve those independently queried source assertions, not just mocked counters.
- [x] Add remove-identity, note purge, shadow-ban reversal, AFK and partial in-room shadow-ban cases. Prove a confirmed first mutation survives a later failed moderation send, cancellation, and audit/handler panic. Add rejected/rolled-back mutation controls proving zero source receipts; a context-canceled invocation must never manufacture a commit. Parallel invocations sharing service objects must not share or overwrite receipt state. Do not extend any of this to DBZ or automove.
- [x] Record confirmed atomic registration/alias writes, identity deletion, role grants, queued mail, note save/clear and shadow-ban mutations at the boundary owning their known result. No record before Commit. Reuse `repository.ErrCommitOutcomeUnknown`; uncertain outcomes get no invented successful count and remain non-retryable. Preserve transaction atomicity, exact credentials, private visibility and Task11's durable delivery table/at-most-one attempt behavior.
- [x] Make shared reply return its send error, then explicitly propagate errors at all in-scope callers, including usage/error/over-limit/multi-message paths. Prefer straightforward handler changes over an additional execution wrapper or hidden mutable command error slot. Preserve original source errors if an error acknowledgment also fails. Where cancellation can occur between a source result and a reply, check it before sending; first retain any confirmed mutation receipt. Leave excluded command callers' behavior unchanged even if Go permits them to ignore the shared helper's new return value.
- [x] Gateway error/cancellation/panic exits retain known source counts and existing private-filtered output. A failed acknowledgment cannot become SUCCEEDED. Cancellation after dispatched action execution and uncertain commit remain UNKNOWN with non-retryable ACTION_OUTCOME_UNKNOWN; do not pretend zero known writes means execution never started. Existing snapshot owner classifications and silent-action metadata remain authoritative.
- [x] Add real tool/executor integration assertions: partial/unknown results retain action counts, unknown actions block later actions, exact repeats cannot replay committed effects, and read-only observations remain available. No raw storage errors or private payloads enter failure prose. Do not loosen success verification merely to make a source success string pass.
- [x] Run focused source-mutation/gateway/tool/receipt/privacy/reply tests, affected race checks and vet, then affected suites once. Record exact RED/GREEN and scoped source boundaries, inspect a scoped commit and obtain independent review. Remaining moderation selector semantics, availability and broader source-result projection are separate required units, not claimed complete here.

## Task 17: Validate moderation selectors before executing effects

**Files:** kickCommand selection region in `internal/command/handlers.go`, `shadow_ban.go`, `unshadow_ban.go`, focused selection helper if useful, ShadowBanService Remove/RemoveAll result signatures, H2 shadow-ban name predicate, their mechanical callers/tests, root `moderation_selector_audit_test.go` and the unshadow case in `audit_moderation_repro_test.go`; shadow-ban exact-mode argument serialization in `catalog/agent_contract.go` only if its regression proves a collision. Start after Task16 is independently approved, preserving its receipts and explicit reply errors. Do not edit DBZ/automove, host lifecycle, other moderation authority, or unrelated catalog entries. Legacy duplicate wrappers remain a separate disposition unit.

**Interfaces:** Full syntax validation precedes effects; no guessed intent. Preserve legitimate single, multi and contains capabilities. Use existing normalized active-nickname resolution and source counts. Return affected counts through existing service removal APIs instead of creating another interface or inferring changes from successful SQL. Keep the recorder's one-operation count for positive rows. Preserve exact trip/hash credentials; only the name predicate may strip the optional mention marker. A no-match mutation returns a safe not-found/rejection, never SUCCESSFUL or a claimed unban.

- [x] Reproduce root `go test ./internal/command -run '^TestModerationSelectorAudit' -count=1 -v`: mixed `-all` deletes all records, incomplete contains mode persists `-c`, trailing operands are ignored, malformed late multi-target input acts before validation, duplicate aliases send three kicks, and empty matches return success. Also rerun `TestAuditModerationUnshadowAllReportsSuccessfulDeletion`.
- [x] Validate the exact supported grammars before lookup/mutation: kick `<nick>` / `-m <nick...>` / `-c <fragment>`; shadowban `<nick>` / `-c <fragment>`; unshadowban `<identity>` / standalone `-all`. Reject missing operands, mode mixing and trailing arguments. A small common structural selection helper is allowed; do not introduce a general workflow language or planner. Resolve and validate the full kick batch before sending anything, deduplicate resolved canonical names, and retain valid multi-target skip-absent behavior when at least one target is present. Require a nonempty match set for action success.
- [x] Add valid syntax controls, Unicode/mention/canonical-casing targets, pre- and mid-execution cancellation, and later send failure after a confirmed first effect. Ensure duplicate target resolution is stable and no model-prose evidence participates. Preserve source partial/unknown receipts and typed transport calls.
- [x] Add real H2 zero-row and positive unshadow tests, optional mention normalization for names with unchanged exact trip/hash predicates, and a controlled delete result differing from the prior listing. Eliminate pre-list/delete-all TOCTOU claims: render the actual changed count after successful deletion. Positive deletion plus successful acknowledgment returns SUCCESSFUL; zero rows produces an accurate no-records outcome; failed acknowledgment retains Task16's committed count.
- [x] Add a structured exact-mode shadow-ban target collision regression before changing serialization. The rendered command must preserve the declared exact mode or reject an unrepresentable target before dispatch; it must never broaden to contains mode. Preserve existing JSON keys and contains support. No changes to excluded contracts.
- [x] Run focused command/catalog/service/H2 selectors, receipts and authority controls, affected race tests and vet. Run affected packages once while naming other known root REDs, not claiming global GREEN. Record behavioral evidence and inspect a scoped commit; obtain independent spec/quality review. Include the already-green nuke/register controls if committing the root shared audit fixture, without changing their behavior.

## Task 18: Serialize host lifecycle ownership and reject unsafe admission

**Files:** `internal/core/host_lifecycle.go`, common host lifecycle/command-dispatch boundary, `cmd/zenbot/host_supervisor.go`, `master_binding.go`, lifecycle composition and shutdown/recovery regions of `main.go`/`startup.go`, host binding region of `internal/core/room_directory.go`, and related tests including root host lifecycle/supervisor audit regressions. Task15 is approved. Task16 owns the gateway until reviewed; do not edit it during this unit. Agent gateway admission plus lifecycle command/tool acceptance-versus-completion mapping remains a required later integration unit, not claimed complete here. Do not edit moderation selection, snapshot lifecycle, replica reconciliation, DBZ or automove.

**Interfaces:** Replace the unsafe unconditional dispatch lease with one error-capable context-aware admission boundary, reused by the common invocation helper. Prefer a coherent method-contract change with mechanical caller/test adaptations over parallel gates. Admission fails promptly while pending/active/terminal/closed, never waits inside the engine dispatcher. Preserve exactly-once release and existing command audit scope. Request methods remain nonblocking from command dispatch and distinguish rejected/unavailable/closed admission from accepted/coalesced work; never claim callback completion from nil admission error.

- [x] Reproduce root `go test ./internal/core ./cmd/zenbot -run '^(TestHostLifecycleAudit|TestHostSupervisorAudit)' -count=1 -v`: command runs during active retirement, nil/closed owners accept requests, second Close panics, and failed replacement retains both published old-host pointers.
- [x] Add controlled pending/active/terminal/closed admission tests through common.InvokeCommand, successful admission and exactly-once release controls, and canceled admission. No sleeps to sequence callbacks. Preserve the already-admitted command's ability to return and release its lease; do not await lifecycle completion from that command or dispatcher.
- [x] Give production lifecycle callbacks an owner/process context, not context.Background or the short request context. Preserve callback errors through a bounded source-owned completion/error result and production reporting; test actual callback failure and cancellation. Coalescing must not erase an active failure silently. Close is idempotent, rejects new requests, cancels active owned work, settles pending requests and wakes Wait; add a deterministic pending-close/Wait liveness regression and callback-panic cleanup/error regression. Do not invent detached retry workers or unbounded result history.
- [x] Serialize supervisor StartInitial/Restart/Shutdown, including signal teardown. Add a controlled restart-build versus process-cancellation/teardown race and prove no candidate remains published or running afterward. Process signal cancels/quiesces lifecycle before final retirement; no new restart can be admitted after closure. Preserve host-only shutdown semantics versus process-owned replica/runtime teardown.
- [x] Withdraw unusable/retired hosts from supervisor, master binding and room directory, permit explicit nil unbinding, and publish only after successful startup. Preserve cleanup ownership for a partially constructed/failed candidate; do not retry source work to erase an error. Add build-failure, start-failure, retire-failure and canceled-candidate controls. Missing host after failed replacement must not appear healthy to recovery, while intentional terminal shutdown must not trigger recovery loops. Healthy separate replicas remain discoverable during host unavailability.
- [x] Run focused lifecycle/supervisor/binding/directory/common dispatch tests, affected race tests, vet and affected suites with known unrelated root REDs identified. Test with local fake callbacks/transports only. Document exact behavioral RED/GREEN, API/caller adaptations and unresolved tool-completion integration; commit explicit scoped files and obtain independent review. This local gate does not solve lifecycle receipt semantics or gateway admission until their later explicit unit.

## Task 19: Align tool visibility and execution with configured command admission

**Files:** command availability/dependency selection in `internal/command/dispatch_adapter.go`, a focused command-owned availability helper if needed, `agent_catalog.go`, `agent_gateway.go`, `agent_capture_engine.go`, optional availability interface in `internal/agent/commandgateway`, SaturnCommand visibility/execute policy and focused gateway/tool/composition tests including root `gateway_admission_audit_test.go`. Start only after Task18 admission and Task20 gateway/result changes are independently approved. Registry alias/index construction and obsolete wrapper removal belong to later units, not this change. Do not edit lifecycle owner state or lifecycle request result semantics, moderation selectors, DBZ or automove-specific dependency logic.

**Interfaces:** One command-owned configured-availability predicate must be reused by manual registration, model-facing visibility and last-moment execution admission. It checks real source/controller dependencies and any owner's existing CommandAvailability, not capabilities advertised by a capture decorator alone. Add optional canonical availability to the gateway without breaking small Gateway fakes. Resolve the current host once for execution; visibility can resolve independently but is only advisory. Enforce authorization and availability again before invoking source work. Reuse `common.BeginCommandDispatch` for the gateway's lease through execution, snapshot completion and auditing; no second lifecycle gate or waiting for retirement. Centralize duplicated caller policy at a dependency-neutral command/tool boundary without changing reviewed-author or capability grants.

- [x] Reproduce root `go test ./internal/command -run '^TestGatewayAdmissionAudit' -count=1 -v`: an owner-denied command remains visible and sends, the gateway bypasses a rejecting host lease, and a resolving gateway with no current host leaves its tool visible. The initial fixture passed nil room users and failed context construction; correct it to an empty slice before recording behavioral RED.
- [x] Test actual manual registration and the concrete tool/gateway against configured and missing dependencies for representative ordinary service, moderation, replica, snapshot and lifecycle commands. Preserve valid minimal engines and return safe known-unavailable outcomes with no effects. Do not hide a working tool just because a compatibility fake lacks the optional availability seam. Validate real production composition remains capable; no production registration of generic run_command.
- [x] Test visibility followed by dependency withdrawal, nil host resolution and replacement between separate requests. Execution resolves one engine and does not dispatch against another engine after admission. Availability rejection is not ACTION_OUTCOME_UNKNOWN because no command has started. Keep safe tool-facing messages and existing capabilities.
- [x] Acquire the shared context-aware lifecycle lease before command work; reject pending/active/terminal/closed owners promptly. Retain the lease through command audit and synchronous snapshot completion; release once on success, source error, cancellation and panic. Add controlled channel-based tests, including source action remaining non-retryable after cancellation. No dispatcher waits for its own lifecycle operation.
- [x] Forward configured availability through relevant decorators where required, while checking the underlying source owner. Extract matching caller policy instead of copying more switches; prove direct execute, gateway and manifest agree for reviewed-author and normal capability controls. Do not change argument keys, routing intent, result data or Task20 observation semantics.
- [x] Run focused availability/admission/authorization/capture/result tests, affected suites once, focused race and vet. Name any pending audit exclusions explicitly. Record RED/GREEN and actual scoped commit parent; obtain independent review before later gateway lifecycle-contract work.

## Task 20: Retain validated source observations independently of delivery errors

**Files:** snapshot list result producer and OperationResult observation metadata/clone in `internal/listener/snapshot`; capture/gateway data propagation; `internal/agent/commandgateway`, tool contract Result/envelope, command tool result mapping, executor optional-data validation, `internal/agent/assemble/observations.go` and relevant tests including root `source_observation_audit_test.go`. Begin after approved Task16 and coordinate future gateway availability/lifecycle integration to avoid overlapping edits. Do not change snapshot owner execution/cancellation/cleanup, lifecycle, moderation behavior, DBZ or automove.

**Interfaces:** Add one optional typed observation-data path for error results, separate from safe error prose and effect receipts. Producer-owned validity metadata marks observations before delivery; do not trust arbitrary Data on failed operations. Avoid a second result framework or mutable service-side recorder. Reuse declared result schemas, existing bounded projections, full-result retrieval and source privacy filters. Preserve every existing success envelope and error code/effect classification unless a specific regression requires a compatible optional field.

- [x] Reproduce root `go test ./internal/command -run '^TestSourceObservationAudit' -count=1 -v`. Actual list/coordinator/gateway/tool observes two users and closes its session, but failed reply returns an error envelope with data:null. Keep the test inspecting the structured envelope, not concatenated JSON inside error prose.
- [x] Mark list data observed at the successful parse/result construction boundary and preserve it across later flush/reply/cancellation failure without altering UNKNOWN/nonretryable state or adding delivery/action counts. Preserve absent/malformed source outcomes as no observation. Never expose private snapshot data to a public invocation.
- [x] Carry validated optional observations through commandgateway and contract.Result as separate data from error text. The command failure mapping preserves source data and safe error codes without treating it as successful execution. Validate optional observed payload against the declared tool result schema; reject/drop malformed data conservatively while retaining prior unknown state, source counts and diagnostic safety. Do not serialize arbitrary error objects or driver text as data.
- [x] Extend ObservationStore projection/index/full retrieval for errors with data. Apply the same per-result and total-context bounds with valid JSON, explicit truncation and immutable receipts. Preserve error code/message, action/delivery counts and unknown barrier even when data is present. No new planner, prose evaluator or intent classifier.
- [x] Add actual-source positive/empty snapshot data after failed acknowledgment; private suppression; malformed/unmarked data rejection; cancellation/cleanup; bounded large data plus error message; full-evidence retrieval; and three-round final-context visibility controls. Tests must prove room/count facts remain available while action repeats stay blocked and successful delivery is never claimed.
- [x] Run focused snapshot/capture/tool/contract/executor/projection tests, affected race checks and vet, with unrelated pending root REDs named. Inspect scoped commit and obtain independent review. Non-snapshot commands whose successful source output is lost before failed delivery remain explicitly inventoried for separate disposition rather than solved by assuming all error strings are observations.

## Task 21: Retire unused compatibility paths and correct runtime memory reporting

**Files:** legacy wrapper files `internal/command/ban.go`, `unban.go`, `unbanAll.go`, `lock.go`, `kick.go`, `list.go`, `unlock.go`; their direct-only tests; only the preliminary legacy registration block in `dispatch_adapter.go`; unused unlock fallback/handler in `handlers.go`; `memory.go` and memory tests; unused MailService.RegisteredUsers in `internal/service/mail_notes.go`. Start after Task17 commits its overlapping handler/test changes. Do not edit agent gateway/capture, configured-availability code, active moderation handlers, DBZ or automove fallback branches. The seven legacy types and helper functions must be rechecked for actual callers before removal.

**Interfaces:** Registered canonical handlers and harmless aliases stay intact. `lock off` remains the supported unlock operation. Do not introduce a new unlock tool or manual route for the previously unregistered ghost definition. Remove dead duplicate implementations, not working capabilities. Label the measured Go runtime values truthfully: Go Alloc, Go HeapIdle, Go HeapSys, Go Sys, in MiB; Sys is not a maximum and HeapIdle is not generic free process memory. Preserve context/error propagation and the measured fields themselves.

- [x] Add a deterministic memory-format RED using runtime.MemStats values; the current JVM/free/max labels must fail truthful Go-field assertions. Preserve positive/zero values, exact MiB conversion and output delivery/cancellation controls. Do not change GC policy in this formatting unit.
- [x] Recheck all Go callers of seven legacy types and their helpers. Remove the four preliminary registrations that canonical adapters overwrite before startup. Remove only wrappers whose remaining uses are direct tests; migrate any meaningful behavior assertions to the actual registered canonical path, removing tests that exist only to preserve dead implementations. Keep existing production registry/alias/role tests and add a no-duplicate-registration control using a recording engine before removing the duplicate registrations.
- [x] Remove the unregistered unlock definition fallback and unused unlock handler after confirming `lock off` is the supported registered path; test that it still dispatches the typed unlock operation. Preserve every actual registered alias. Leave excluded generic command branches unchanged even if unreachable.
- [x] Remove the unused MailService.RegisteredUsers string-returning query after confirming zero callers. Preserve SaturnRegisteredUsers and UserService.RegisteredUsers, both legitimate separate APIs. No database behavior or schemas change.
- [x] Run focused memory, registration/alias/clone and affected command/service tests with pending root audit groups explicitly excluded; vet. Recheck symbol references and diff scope. Record deletion inventory/recoverability through Git, RED/GREEN evidence and exact scoped commit, then independent spec/quality review. Do not stage root documentation or pending tests owned by later tasks.

## Task 22: Fail closed on registry ambiguity and freeze command-definition ownership

**Files:** common command registry/lookup, command catalog materialization/definition resolution, core runtime command registration/map ownership, production registration callers and mechanical tests. Start after Task19 availability and Task21 preliminary-registration removal are reviewed. Do not change command grammar, permission grants, excluded command behavior or tool result semantics.

**Interfaces:** Validate command registration before exposing an inventory; no ignored RegisterAll error or partial catalog. Build one immutable validated catalog index instead of reconstructing all definitions per lookup. Preserve canonical/exact aliases and existing unambiguous anagram compatibility. Runtime registration rejects cross-command exact/anagram collisions atomically, never overwrites an owner or picks map iteration order. Prefer a coherent error-returning registration contract with mechanical adapters over hidden mutable error flags; propagate production composition errors. Returned definition/alias snapshots cannot mutate registry ownership.

- [x] Add behavioral REDs for actual runtime duplicate exact aliases, cross-owner anagram aliases and alias-slice/map mutation leaking into registry state. Do not assert a collision exists in today's static catalog: none was found. Include same-owner alias controls, unknown commands, and unchanged direct L/registered production behavior.
- [x] Propagate registry construction errors and preserve original valid entries atomically on failed registration. Resolve definitions from the validated immutable index, failing closed rather than skipping missing agent entries or building partial inventories. No second competing routing policy.
- [x] Make BuildCommand deterministic for every supported engine implementation. Handle legacy map-only implementations conservatively if a new ownership index is unavailable; do not guess an owner on ambiguous fallback. Avoid locks while executing constructors or command code. Keep registered engines construction-safe and protect exposed maps/alias slices as needed by the actual ownership boundary.
- [x] Inspect every production registration call site for errors; mechanical test interface adaptations only for excluded fixtures if necessary, no excluded behavior changes or targeted work. Run focused common/core/command/production registration tests, affected race/vet and independent review.

## Task 23: Isolate shadow-ban matching from unrelated corrupt records

**Files:** ShadowBanService.Matches, existing ShadowBanCommandRepository contract and H2 implementation, mechanical fakes/callers, focused service/repository tests including root `shadow_match_audit_test.go`. Start after Task17 counted removal is independently reviewed. Do not change removal grammar, join automation, automove, or DBZ.

**Interfaces:** Matching is a repository-owned exact existence lookup, not a full rich-row list/decode operation. Compare supplied nonblank trip, name and encoded hash against their own columns with parameterized SQL. Preserve the existing exact identity matching semantics, including case sensitivity; no nickname/trip interchange, wildcard matching, or case-folded credentials. Keep ListShadowBans honest: corrupt displayed records may still return an error rather than silently dropping rows.

- [x] Reproduce root `go test ./internal/service -run '^TestShadowMatchAudit' -count=1 -v`: a valid name/trip/hash record cannot match because one unrelated malformed stored hash makes ListShadowBans fail. Root real H2 RED exit1/1.060s, session83046.
- [x] Add a context-aware matching method to the existing repository boundary and have the service use it. Query only existence, encoding the supplied hash rather than decoding every stored hash. Add exact positive/negative identities, blank/nil users, corrupt unrelated rows, SQL-looking values, source error/cancellation, and no-record controls. Do not hide database failure as an unmatched success.
- [x] Keep list/delete/persist and positive mutation receipt behavior unchanged. Mechanical fake adapters must implement real matching semantics, not always return true. Run focused source/query/service tests, affected suites with other pending REDs named, race/vet and independent review.

## Task 24: Make lifecycle tools report request admission without inventing completion

**Files:** common lifecycle result/admission contract, core lifecycle source request insertion/aliases, `command/restart_shutdown.go`, a narrow command-produced observation helper and capture implementation, only restart/shutdown catalog descriptions, command result schema selection/builders and focused tests. Start after Task19 is independently reviewed. Preserve Task18 owner, cancellation and cleanup state machine; no detached worker, waiting dispatcher, new retry mechanism, whole-process shutdown implementation or model planner. DBZ and automove remain excluded.

**Interfaces:** The synchronous action is explicitly requesting host restart/shutdown and acknowledging its admission. A command cannot await host retirement while holding its own dispatcher/lease. Expose existing structured admission through a read-only common operation interface, retaining error-only compatibility methods; core aliases avoid parallel result types. Source insertion records one confirmed request-state mutation, coalescing records zero. Successful acknowledgment retains ordinary actual delivery receipts, including for coalescing. No new tool success category is required.

- [x] Add offline REDs for the actual lifecycle owner, gateway and Saturn tool: current accepted requests become UNVERIFIED_ACTION_OUTCOME with no admission facts. Test with controlled leases/channels, not sleeps, and do not require a callback to complete before the command releases its own lease.
- [x] Project typed admission data: operation restart/shutdown, scope host, requestStatus accepted/coalesced, completionStatus not_observed/succeeded/failed. Read a source operation result once without waiting; completed state is allowed only from a finished source handle. No raw error, stack, handle, credential or unbounded result history enters model data. A fixed safe failure code may accompany a genuinely observed failure.
- [x] Record request-state mutation exactly once at the owner queue insertion boundary, including accepted shutdown supersession; never count coalescing as a second mutation. Capture the producer-returned admission snapshot before acknowledgment/cancellation can obscure it. Prefer an explicit command-produced data observer reused by later source commands rather than wrapping the lifecycle controller and accidentally losing its admission capabilities. Preserve trusted public/private visibility filtering.
- [x] Send a synchronous acknowledgment explicitly stating request admission and unconfirmed completion, with distinct coalesced wording. Cancellation before admission is not-started; after admission/acknowledgment it remains non-retryable ACTION_OUTCOME_UNKNOWN with retained facts and actual counts. Failed acknowledgment cannot undo admission or claim successful delivery. Test coalescing using already-admitted concurrent requests or the source API: do not bypass the host gate merely to permit a repeat request.
- [x] Update only lifecycle tool names/descriptions/result schemas to the request contract and host-only scope, preserving names, aliases, empty arguments and administrator authority. Never claim process/replica/agent shutdown. Closed schemas validate admission payload; malformed optional data is withheld without destroying error/effect evidence. Existing ledger rules remain unchanged.
- [x] Run focused real-owner admission/acknowledgment/cancellation/coalescing/privacy/schema tests, affected suites with pending audit groups named, race/vet and independent review. State explicitly that request admission is verified but completed host retirement is not promised by this invocation.

## Task 25: Retain successful text observations before failed outward delivery

**Files:** the command-produced observation helper/capture path from Task24, ordinary command result schema selection, registered successful source-output branches in services/user_utilities, last_online, users_nicks, activity, mail_notes notes-list only, identity history, SQL, runtime informational reports and relevant tests. Start after Task19/21/24 are reviewed to avoid availability, dead-code and observation-helper overlaps. Do not modify source query semantics, mutation acknowledgments, SQL parsing, mail sending, snapshot lifecycle, DBZ or automove.

**Interfaces:** Reuse Task20's existing Data/DataObserved/ObservedData path with a closed text-observation payload. Producers explicitly mark successfully obtained, formatted source output before delivery; do not classify arbitrary reply or error strings as valid data. One optional capture observer suffices; no second service recorder or result framework. Preserve source text, cancellation/error status, receipts, private filtering, full retrieval and bounded model projections. Explicit usage/error/acknowledgment branches do not create successful read observations.

- [x] Reproduce root `go test ./internal/command -run '^TestTextObservationAudit' -count=1 -v`: actual PingService measures a local connection, failed room send returns UNKNOWN with data:null and loses the measurement. Root RED exit1/0.572s session17046; no production observation fix was made.
- [x] Add source-controlled weather/time/lastonline/registered-users/nicks/activity/history/SQL and private-notes examples. Record only after authoritative source success, before later context/delivery checks can discard known facts. A valid empty query can be an empty observation; absent/malformed/failed sources cannot. Observe actual output visibility, not just invocation visibility; public invocation must never expose private notes even after a failed whisper.
- [x] Select a closed result schema for ordinary text observations while keeping list's typed room/count schema and lifecycle's typed admission schema. Preserve existing success payload compatibility with optional data, reject invalid optional payloads conservatively and retain safe errors/counts. Do not expand the legacy generic tool into the production inventory.
- [x] Use one explicit observation-and-reply helper at valid source branches when appropriate, retaining different output channels. Do not automatically mark every reply: user-facing usage, rejection, mutation acknowledgment and raw diagnostics have different evidence semantics. Inventory every remaining in-scope informational producer, including memory/info/version/help/replica status, and either apply the helper to its valid source branch or document why no separate source observation exists.
- [x] Test actual source work occurs once, failed/canceled delivery never gains a receipt or causes requery/replay, large text remains bounded in model context and fully retrievable, and three-round final synthesis retains the observation plus error. Preserve UTF-8 and private-data guarantees. Run affected tests/race/vet with pending groups named, inspect scoped commit, obtain independent review.

## Task 26: Preserve canonical moderation targets through the real protocol boundary

**Files:** common NickTarget contract, core moderation serializer and exact-payload tests, ban/overflow target selection, necessary in-scope raw-operand callers, focused command/core tests including root `moderation_target_boundary_audit_test.go`. Begin after Task21 removes dead wrappers and Task17 is reviewed. Do not edit the excluded automove caller or its tests, DBZ, snapshot workflow ownership, gateway availability, shadow-ban matching or authority grants. Shared raw serialization is in scope; excluded command-specific work is not.

**Interfaces:** Raw user operands are normalized at the command input boundary exactly once; a typed already-resolved NickTarget carries the canonical source nickname literally. The transport boundary validates but must not strip a mention marker or change a selected identity. Prefer clarifying the existing type's ownership contract and migrating raw-input call sites over adding competing moderation APIs. Core tests that currently require normalization of raw input passed as an already-resolved type must be adapted to the coherent contract, while raw command-input normalization remains tested end to end.

- [x] Reproduce root `go test ./internal/command -run '^TestModerationTargetBoundaryAudit' -count=1 -v`, RED exit1/0.595s session80582. Actual registered kick correctly selects @alice but real EngineImpl serialization changes it to alice, in both exact and contains modes. Ban/overflow send lowercased caller spelling instead of active Merc and send requests for absent or extra-operand inputs.
- [x] Trace all in-scope NickTarget producers and pass the selected canonical identity through to JSON unchanged. Preserve exact escaping, cancellation and transport error behavior; reject malformed/blank raw input before any effect. Add literal-marker, Unicode, case, JSON-special-character and ordinary-name controls using the actual core serializer, not only command stubs. Do not change NormalizeNickTarget's one-marker raw-input rule.
- [x] Ban and overflow accept exactly one raw nickname, resolve the actual active canonical user, and reject absent users/trailing operands before sends. Preserve actual typed operations and silent overflow behavior. Add pre-/post-lookup cancellation and failed-send controls; no transport confirmation is fabricated. Other single-target handlers must be checked for the same raw-versus-canonical handoff, with only necessary argument-boundary changes included.
- [x] Verify source-owned names from kick/shadow contains selections, active profile/mute targets, and resurrect/in-scope join paths remain literal. Do not rewrite excluded source files merely to test a shared contract. Record any unavoidable mechanical compatibility question before changing those files.
- [x] Run focused command-to-core regressions, raw payload tests, affected suites with pending groups named, race/vet and independent review. This repairs target identity, not remote server application acknowledgment; success still only establishes the documented outward-request boundary until a separate acknowledgment exists.

## Task 27: Describe transport request receipts without claiming server application

**Files:** common/core moderation boundary documentation, the affected remote moderation command acknowledgments, their catalog descriptions, snapshot moderation completion wording, and the shared tool-result interpretation prompt/descriptor contract with focused tests. Start after Task26 and the relevant observation/lifecycle integration commits are reviewed. No server protocol redesign, response polling, automatic replay, new workflow engine, excluded command-specific edits or changes to who may moderate.

**Interfaces:** The inspected production path returns from a synchronous outbound transport write; it does not await a correlated server application acknowledgment. Keep source effects/receipt counts and cancellation behavior intact, but state exactly what they verify. Tool success for a documented request operation means request submission, not proven server application. Local database changes and independently verified source results keep their stronger meanings. Do not add a generic semantic success classifier or infer application from a roster snapshot.

- [x] Add a controlled write-only transport/command regression: a successful send alone currently produces past-tense applied moderation claims. Source tests should verify transmission once and no server acknowledgment consumed; command/model-facing wording must identify request submission without inventing a completed kick/ban/mute. This is a boundary-evidence test, not a test that supplies an LLM completion judgment.
- [x] Narrow affected moderation acknowledgments and catalog descriptions to request-sent semantics. Keep true local shadow-ban persistence distinct from its separate kick request. Snapshot nuke/move completion wording distinguishes completed local workflow/flush from the remote server applying every request. Preserve counts, target identity, privacy and UNKNOWN/non-retryable outcomes after uncertainty.
- [x] Clarify shared model-facing tool-result interpretation: count/source receipts prove only their documented boundary. The LLM still decides and summarizes, but must not treat a local chat acknowledgment as independent remote server evidence. Prefer existing descriptions/prompt fields; no new state machine or schema layer merely for wording.
- [x] Preserve all actual successful response data and exact source errors. Update compatibility assertions that require overstated acknowledgments while retaining their action/authority/privacy/error checks. Verify positive/partial/canceled/failure cases and multi-tool final-context guidance. Run focused/affected tests and independent review; document that remote application remains unconfirmed where the protocol provides no correlated acknowledgment.

## Task 28: Correct the live help surface without changing excluded commands

**Files:** `internal/command/help.go` and focused help tests. Begin after Task25 observation integration is reviewed. Task27's completed preflight assigns it no help-file edits, so the two wording units may run concurrently with these explicit separate owners; both use the same source-verified request-submission boundary. No command registration, permissions, service behavior, new routing metadata, DBZ or automove command-specific changes.

**Interfaces:** Preserve private help delivery, explicit source observation, live prefix substitution and send/cancellation errors. Correct the existing in-scope static help text against the actual registered catalog and documented source behavior. A new help framework is unnecessary for this bounded content/formatting repair. Keep excluded help entries' meaning intact; do not add an audit-exclusion mechanism to production code.

- [x] Add behavioral REDs through the actual registered help handler for unsupported mine/whiskey routes, JVM memory labels, incorrect lifecycle scope and duplicate/misclassified public msgchannel. Current help advertises mining and whiskey implementations that do not exist, describes host-only lifecycle as replica/application shutdown, and labels Go memory as JVM. Root verified these existing utility-report findings remain in help.go after Task21. Preserve move: it is a real alias of resurrect in the catalog.
- [x] Remove only unsupported in-scope route claims; retain move/recover/heal/resurrect aliases and describe the actual required nickname/source/destination grammar, not a no-argument last-kicked-user shortcut. Describe Go runtime values in MiB, lifecycle request admission and host-only scope, and public relay authority truthfully. Preserve Task27's distinctions between request submission and remote application, including permanent-ban semantics for nuke. Do not change excluded command rows or invent missing functionality.
- [x] Add a direct multiline alignment RED: alignHelp currently splits literal backslash-n while its constants contain actual newline characters, so multiple command rows are processed as one line. Format actual rows consistently with Unicode-aware padding and preserved content. Do not alter server protocol framing or silently strip descriptions, blank lines or examples.
- [x] Keep all successful source-observation/private-filtering and failed/canceled whisper behavior. Run focused help and affected command tests with other pending groups named, vet and independent review. This unit makes help truthful, not dynamically capability-filtered; clearly state that remaining static reference scope.

## Task 29: Count distinct room occupants instead of credential identities

**Files:** `internal/listener/snapshot/list_operation.go`, focused list-operation tests and an actual command/gateway list data control if needed. Start after Task20 observation handling; its producer schema stays unchanged. Do not change global IdentityKey semantics, authorization, repository identity matching, legacy snapshot Store, workflow lifecycle, DBZ or automove.

**Interfaces:** Room presence is the set of observed source nicknames, not the set of tripcodes or network hashes. Different active nicknames can share a trip or hash; both must remain in a roster and its count. Deduplicate only exact repeated source nicknames, preserving the first source record and existing stable output ordering. Never case-fold, strip mention markers or rewrite source nicknames at this boundary. Count and returnedCount must agree with the emitted names, on success and retained failed-delivery observations.

- [x] Add actual Parse→ListRoomOperation REDs for distinct names sharing a trip, sharing a hash without trips, and repeated exact nickname records. Existing stable-list test deliberately calls a distinct same-trip nickname "duplicate" and collapses it; that compatibility assertion encodes the defect and must change. Root verified the current producer uses model.IdentityKey, which prioritizes trip/hash over nick.
- [x] Replace credential-identity deduplication only in the list formatter/producer with exact source-nickname deduplication. Preserve stable hash/name/trip sorting, nil filtering for direct operation callers, empty schema shape, DataObserved, privacy and delivery/error semantics. Keep global identity matching and moderation action selection unchanged.
- [x] Prove successful list data and failed-delivery retained data both contain all distinct nicknames and the correct count through the concrete gateway/tool path. Keep unknown/nonretryable status and actual receipt counts on delivery failure. No re-query, remote application claim or planner.
- [x] Document the precise boundary: this counts the observed roster, including any bots present in that roster; it does not infer unique humans. Do not guess or remove a temporary observer without source-certified identity metadata. Run focused and affected snapshot/command tests with pending groups named, race/vet and independent review.

## Task 30: Validate utility syntax and preserve caller/room identities before effects

**Files:** `internal/command/mail_notes.go` notes command only, `internal/command/handlers.go` AFK/list boundaries, `internal/command/msg_channel.go` room selection only, focused tests including root `utility_boundary_audit_test.go`. Preserve approved source observation and receipt paths. No service/database API, registry, schemas, unrelated command parsing, DBZ or automove changes. This is independent of Task27 wording and Task28 help files.

**Interfaces:** Notes permits no operands for private listing, or exactly one `purge`/`clear` operand for deletion. Unexpected trailing operands must be rejected before any service mutation. AFK changes only the current active caller identified by trusted nickname and exact nonblank trip; a different nickname with the same trip is not the caller. Source names retain their literal values. Unknown/absent/mismatched callers cannot become a successful state change merely because an explanation was delivered.

Raw list/msgchannel room operands may remove one leading `?` room-link marker, never interior/trailing question marks. The normalized room name is passed literally to local comparison and remote submission; neither command may silently redirect `other?room` to `otherroom`. Do not normalize source-owned channel metadata or introduce server room-name guessing. This rule only repairs the two documented raw utility selectors, not excluded movement configuration.

- [x] Reproduce root `go test ./internal/command -run '^TestUtilityBoundaryAudit' -count=1 -v`, RED exit1/2.149s session86749. Real H2 notes are deleted by both malformed purge/clear calls; AFK returns success for three invalid caller states and mutates another same-trip nickname.
- [x] Validate the complete notes grammar before deletion/list source work. Keep valid purge/clear and private list behavior, exact-trip ownership, context-aware database operations, committed mutation receipts, and acknowledgment failure behavior. Use the actual configured prefix in explanatory usage; do not add an argument-repair heuristic or reinterpret extra words.
- [x] Resolve one active caller with exact trip and normal nickname lookup semantics, reject blank/whitespace trip and absent/mismatched source identity, and recheck cancellation after source lookup before mutation. Mutate only that resolved record, not every nickname sharing its trip. Keep existing AFK ownership/storage and source receipt semantics; do not invent a new state manager.
- [x] Reproduce root room-selector RED `go test ./internal/command -run '^TestUtilityBoundaryAuditRoomSelector' -count=1 -v`, exit1/0.536s session30136. Msgchannel drops interior `?` bytes; list retains the leading link marker. Share one small raw-selector helper if useful; reject bare marker/blank and extra list operands before remote/local effects. Preserve no-argument manual list's explanatory fallback and the remote-only Saturn schema. Adapt the old msgchannel fixture that incorrectly expects `?other?` to become `other`; trailing `?` is part of the literal destination. Cover local/remote routing, marker/case/Unicode/interior punctuation, failed submission and cancellation without changing workflow lifetimes or message bodies.
- [x] Extend real gateway/source tests for valid caller alongside another same-trip nickname, source canonical spelling, valid notes list/purge/clear, malformed-input state preservation, pre-/post-lookup cancellation, and failed acknowledgment after actual mutation. Failed usage delivery must propagate its error without creating mutation evidence. Run focused and affected tests with other pending audit groups named, race/vet and independent review.

## Task 31: Return exact public history instead of inferred sessions

**Files:** existing repository user-history record/interfaces, H2 nicks/last-online/LastSeen implementations, UserService history selection/renderer, only nicks/lastonline catalog descriptions, focused repository/service/command tests and necessary mechanical fake adaptations. Also allow the narrow existing gateway outcome/tool-error classification and its tests described below. Read the bounded `history-source-preflight.md` in this plan's ignored workspace for exact current source paths and regression inventory. Start after Task27 releases its catalog-file ownership; preserve Task25 observation integration and Task19 typed-nil source selection. Do not change schemas/tables/indexes, listeners, current-room tools, DBZ or automove.

**Interfaces:** Nicks means historical nicknames publicly observed for one exact trip, not registered ownership. Query recognized current presence events plus PUBLIC message history, exclude private-only aliases and blank names, deduplicate exact nickname values and return deterministic latest-first order with a stable name tie-break. Keep successful empty results and source errors distinct. Do not lowercase credentials.

Last-online reports independent persisted public-message and JOINED/LEFT observations across stored rooms. It cannot prove current membership, continuous activity or session duration. Keep one common LastOnlineRecord with Found, LastMessage, LastMessageMillis, LastPresenceEvent and LastPresenceMillis; nullable fields distinguish absent facts. Current and legacy presence sources must be explicitly recognized and public. Found is true when either fact exists. Keep LastSeen method compatibility by making it use the same fact record/query and one renderer; remove the duplicate weaker renderer/record rather than layering an adapter that fabricates missing facts.

- [x] Write actual H2 REDs for case-variant trip isolation, private-only nickname exclusion, presence-only nickname discovery, historical-vs-registered identity, deterministic exact-name ordering; existing tests preserving credential case-folding must change. Require exactly one nicks trip operand before source work rather than ignoring trailing words.
- [x] Write actual H2/service REDs for join-only, leave-only, leave-after-join, independent public-message/presence timestamps, current/legacy source parity, no-record errors in both configured paths and stable equal-timestamp selection. The latest public message excludes JOINED/LEFT markers and WHISPER rows; legacy presence excludes whispers too. Use message id for same-table timestamp ties and deterministic source/event ordering for cross-table ties without claiming causal sequence.
- [x] Replace now-minus-join session inference with absolute `Last observed`, `Last presence event` and `Last public message` facts. Last observed is the later available fact; no `Seen active`, `Session duration` or current-online claim remains. Preserve escaped public message text, valid empty message text, source error/cancellation propagation, no fallback after a real preferred-source error and explicit observation-before-delivery retention. Retain UserService.Now where unrelated SeenRecently still uses it.
- [x] Preserve exact nickname-or-trip lookup, but reject genuine cross-kind collisions before combining unrelated observations: public rows match both selector kinds and their matching row sets differ. A row whose nickname and trip both equal the selector is not by itself ambiguous. Use existing source-owned error conventions and public observation sources only; no private-row existence leak, selector guessing, new schema or model router. Cover collision rejection and the same-name-and-trip positive case. Explain this limitation in the descriptor: callers may resolve a trip's observed names with nicks; ambiguity is not permission to select an arbitrary identity.
- [x] Carry source-owned `repository.ErrAmbiguousHistory` through existing `commandgateway.OutcomeAmbiguous` classification to safe `AMBIGUOUS_TARGET` feedback in the native and compatibility command adapters. Use static actionable guidance, not the raw selector, SQL or error text. Cancellation precedence stays UNKNOWN/nonretryable; positive receipts/observations and partial state remain intact. A zero count alone is not source proof. This permits narrow edits to commandgateway/gateway.go, command/agent_gateway.go, tool/run_command.go and tool/saturn_command.go as needed, with real H2 gateway/native/compatibility, safe-message, cancellation and positive-receipt controls. No new result schema, workflow state machine or unrelated error-classification redesign.
- [x] Make H2 LastSeen delegate the same fact query, and adapt internal fakes/callers mechanically to the one record. Catalog retains tool names, aliases, argument shape and authority, but describes publicly observed history, exact trip semantics, cross-room scope and no current-membership inference. No source tracker or new workflow is justified.
- [x] Run focused RED/GREEN, affected H2/service/command/agent tests, race/vet and independent review. Keep all current privacy regressions and Task25 retained-error observations. Stage only owned hunks after exclusive index reservation; report exact parent/commit and any concurrent pending groups honestly.

## Final integration review fix wave

All in-scope numbered units cleared their scoped gates. The whole-branch review
at `8806152` found five remaining composition defects and six bounded smaller
issues. The combined fix `6f830bc` and its independent scoped re-review closed all eleven
findings; full uncached tests, affected race and full vet passed on the final code.

- [x] Preserve canonical reviewed-author identity and snapshot nuke/move source names; normalize raw selectors once and verify competing-name wire targets.
- [x] Use exact nickname ownership in the concrete current roster and AFK state; shared trips/hashes cannot evict a different nickname. Chat-driven AFK removal also requires exact trip equality.
- [x] Give contains-mode shadowban one truthful completed-batch acknowledgment and preserve receipts on later failures.
- [x] Repair bounded stop-callback observation, final ACCEPTED-response-loss coverage, websocket event waiting/owned cleanup and the retained prefix setter lock.
- [x] Enforce native list's current-room restriction after one raw leading-marker normalization, and correct shadowban help grammar.
- [x] Record real RED/GREEN and coverage-only evidence distinctly; run final full uncached suite, affected race and full vet; obtain one independent scoped re-review and finalize audit dispositions.

Keep global IdentityKey and excluded commands unchanged. Reuse existing source
owners, error/receipt shapes, setters and test hooks; no model planner or new
production lifecycle mechanism is needed.

## Remaining repair units

The audit reports own exact findings and reproduction traces for utility services, lifecycle, moderation/workflows, persistence, and shared dispatch/tool outcomes. Add a bounded, file-owned task for each verified unit before implementing it; do not turn an unverified observation into an edit. Completion requires every remaining in-scope command and repair unit to have a checked disposition in the coverage ledger. DBZ-related commands and automove are excluded by the user's updates.
