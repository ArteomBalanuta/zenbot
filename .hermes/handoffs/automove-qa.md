# AutoMove independent QA

## Verdict: FAIL — do not accept into the migration roadmap as strict-TDD-complete

Runtime/behavioral validation is green after two QA fixes, but the required historical TDD-evidence gate is not met. The recovery records do not show an antecedent RED → GREEN for every current join-policy branch. Passing tests written/run after a production branch cannot repair that missing historical proof.

## Scope and preservation

- Audited against `automove-current-architecture.md`, `automove-tdd-forensic.md`, `automove-tdd-recovery.md`, all seven recovery-step handoffs, and Saturn `AutoMoveUserCommandImpl.java`, `UserJoinedListenerImpl.java`, and `SqlUtil.java`.
- No reset, clean, checkout, staging, or commit was performed.
- No automove persistence, config, schema, migration, retry loop, source removal, remote/snapshot composition, or temporary-session composition was added.
- `git diff --check` passed. The working tree remains dirty from concurrent/unrelated migration work.

## TDD-evidence audit

| Current slice | Recorded preceding RED → GREEN | Result |
|---|---|---|
| default state; configure mutation | state-mutation handoff | PASS |
| lifecycle state ordering, missing/present replicas, cancellation, stop-first-error, on/off serialization | core-lifecycle handoff | PASS |
| moderator command parsing, capability registration, aliases/role, replies, cancellation | command handoff | PASS |
| H2 USER-trip lookup and context propagation | H2 handoff | PASS; QA additionally recorded source-literal query RED → GREEN |
| join happy path, host/gates, cancellation, action-error logging | join-policy handoff | PASS |
| exact-case mismatch, trip-query failure, notice-failure-still-kicks | join-policy handoff lists only later focused passing checks, not their own preceding RED observations | **FAIL** |
| active-user visibility and semantic → share → log → automove final order | listener-order handoff | PASS |
| shared master/permanent replica state, permanent composition, temporary inertness, main registration | factory/main handoff | PASS |

The failed row is acceptance-blocking under the requested rule: each current production slice must have recorded preceding RED → GREEN evidence. Do not retroactively describe its later green checks as test-first evidence. A future recovery must either restore the documented prior checkpoint and reimplement these branches one tracer at a time, or obtain an explicit decision to relax the strict historical-TDD gate.

## Contract audit

- Volatile defaults, trim/blank rejection, additive sources, destination replacement, sorted copied snapshots: verified in `internal/core/auto_move.go` and focused core tests.
- Lifecycle order/race handling: enabled/disabled mutation precedes reconciliation; missing-only add, present-only remove, first stop error after continued removal, and serialized enable/disable are covered and race-tested.
- Command: `automove` is a concrete `MODERATOR` command with sole alias, toggle-before-arity/case-insensitive parsing, capability-gated registration, failure without success reply, and dispatch remains the authorization boundary.
- H2: corrected to Saturn literal `SELECT trip FROM trips WHERE type = 'USER';`; USER-only and case-preserving behavior covered.
- Join: exact trip membership, host/non-source/disabled/temporary inertness, literal lounge text then dynamic typed kick, and listener-final ordering are covered.
- Factory/main: master and permanent replicas receive the same state pointer; temporary online-set sessions retain dummy listeners and no state.

## QA production/test fixes

1. **Public notice defect fixed**
   - Root cause: `AutoMoveJoinAutomation.OnJoin` passed `true` to `EngineImpl.SendChatMessage`; that parameter is `whisper`, so the source-required public address notice became a whisper.
   - RED: `go test ./internal/listener -run '^TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica$' -count=1` failed with `whisper true`.
   - GREEN: changed only the action call to pass `false`; focused test and listener race tests pass.
   - Files: `internal/listener/automove_join.go`, `internal/listener/automove_join_test.go`.

2. **Source-exact H2 query hardened**
   - Root cause: target query was semantically equivalent but not literal Saturn SQL (`type='USER'`, no semicolon), while Saturn `SqlUtil.SELECT_LOUNGE_TRIPS` is `SELECT trip FROM trips WHERE type = 'USER';`.
   - RED: `go test ./internal/repository/h2 -run '^TestUserTripsUsesSaturnLoungeTripQuery$' -count=1` failed with the differing literal.
   - GREEN: changed only `selectUserTrips` and added its literal contract test.
   - Files: `internal/repository/h2/user_queries.go`, `internal/repository/h2/user_queries_test.go`.

## Commands and verified results

```text
go test ./internal/listener -run '^TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica$' -count=1  PASS (after QA fix)
go test ./internal/repository/h2 -run '^TestUserTrips' -count=1                                PASS
go test ./internal/listener -run '^(TestAutoMoveJoin|TestUserJoinedListener)' -count=1        PASS
go test ./internal/core -run '^Test(Engine(AutoMove|EnableAutoMove|DisableAutoMove)|AutoMove)' -count=1 PASS
go test ./internal/command -run '^TestAutoMove' -count=1                                      PASS
go test ./internal/factory -run 'AutoMove' -count=1                                           PASS
go test ./cmd/zenbot -run 'AutoMove' -count=1                                                 PASS
go test -race ./internal/core ./internal/listener ./internal/factory ./internal/command ./internal/repository/h2 ./cmd/zenbot -count=1 PASS
go test ./... -count=1                                                                         PASS
go vet ./...                                                                                   PASS
go build ./...                                                                                 PASS
git diff --check                                                                               PASS
```

## Limitations

- Strict historical proof remains incomplete for the three join-policy branches identified above; this is the sole acceptance blocker.
- Chat dispatch intentionally uses `context.Background()` through the legacy adapter; direct command/policy cancellation is tested, but inbound cancellation propagation remains outside this slice.
- The existing dirty worktree prevents attributing all modified tracked files to automove; this QA altered only the four paths listed in the QA-fixes section plus this handoff.
