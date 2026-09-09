# AutoMove TDD forensic diagnostic

**Scope:** forensic-only audit of the dirty Zenbot checkout. No application or test source was changed during this audit. This report is the sole new artifact.

## Verdict

**Do not accept or repair-in-place as TDD-complete.** The live implementation transcript records only two RED→GREEN observations:

1. `TestAutoMoveDefaultUsageIsConcreteModeratorCommand` — expected assertion failure against generic `saturnCommand`, then pass.
2. `TestAutoMoveDefaultsAreFalsePurgatoryAndLounge` — expected build failure (`undefined: NewAutoMoveState`), then pass.

The second GREEN occurred at transcript lines 75–86 (14:26:59–14:27:37). After that point, the implementation added the command parser/lifecycle behavior, registration gate, H2 eligibility lookup, join action, listener composition, factory injection, and main shared-state wiring without any dedicated preceding RED. The transcript has no later test-file write or focused RED command. Its final package/full-suite passes are regression evidence only, not antecedent-TDD evidence.

The architecture handoff is authoritative for the missing tracers: `.hermes/handoffs/automove-current-architecture.md` §§4–9, especially its ordered map at §9.

## Evidence timeline

| Time / transcript lines | Event | TDD finding |
|---|---|---|
| 14:26:18–14:26:52 / 63–74 | Command test written; expected assertion RED; concrete command/handler support written; same test GREEN | Independently valid first tracer for concrete default usage only. |
| 14:26:59–14:27:37 / 75–86 | State-default test written; expected undefined-symbol RED; `internal/core/auto_move.go` and engine fields/methods written; same test GREEN | Independently valid default-state tracer only. The implementation patch also contained untested lifecycle code. |
| 14:27:54 onward / 87–122 | Full command parser; registration/catalog; repository/H2; join automation/listener; factory/main composition written | No antecedent RED evidence for any listed behavior. |
| 14:29:54 onward / 123–132 | `gofmt`, broad package tests, race core test, full suite, vet/build/diff check | All-green verification; it cannot establish test-first order for later production behavior. |

Current test sources confirm the scope: `internal/command/automove_test.go` has one default-usage test; `internal/core/auto_move_test.go` has one defaults/case-membership test. There is no `internal/listener/automove_join_test.go`; focused listener `^TestAutoMoveJoin` and H2 `^TestUserTrips` both return “no tests to run.” `internal/listener/user_joined_listener_test.go` has no automove composition/order test, and `internal/repository/h2/user_queries_test.go` has no `UserTrips` test.

## Exact unproven production introduced after the last valid GREEN

The last observed GREEN is the state-default test at 14:27:37. The following task-owned production additions/revisions occurred later without a prior behavior-specific RED:

| File | Functions / behavior introduced after last GREEN | Missing antecedent tracer |
|---|---|---|
| `internal/command/automove.go` | Expanded `(*automoveCommand).Execute`: context rejection, capability failure, `on`/`off` (including trailing tokens), configure, invalid arity, lifecycle error/no-success-reply behavior; `usage` dynamic snapshot branch | Command parsing/state/reply/cancellation/capability tracers beyond default usage. |
| `internal/command/dispatch_adapter.go` | `RegisterUserUtilitiesWithDirectAgent` AutoMoveController capability-gated `automove` registration | Complete/incomplete-engine registration and MODERATOR adapter tracer. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Removal of generic fallback allowance for `automove` | This test change has no dedicated RED and is dependent on concrete registration behavior. |
| `internal/repository/automove.go` | `AutoMoveTripRepository.UserTrips` contract | Repository boundary behavior tracer. |
| `internal/repository/h2/user_queries.go` | `selectUserTrips`; `(*Database).UserTrips` | USER-only rows, original-case preservation, query/scan/rows/context error tracers. |
| `internal/listener/automove_join.go` | `AutoMoveJoinAutomation`; `(*AutoMoveJoinAutomation).OnJoin` | Gates, exact membership, cancellation, query failure, notice-error-still-kicks, literal notice, dynamic kick destination tracers. |
| `internal/listener/user_joined_listener.go` | `autoMove` field; last-position `OnJoin` invocation; `NewUserJoinedListenerWithAutomations` | Registered-user visibility, semantic→share→log→automove order, malformed inertness tracer. |
| `internal/factory/engine_factory.go` | `EngineOptions.AutoMoveState`; injection; `composeAutoMoveJoin`; permanent listener composition | Permanent/temporary composition and pointer-identity tracer. |
| `cmd/zenbot/main.go` | `autoMoveState := core.NewAutoMoveState`, host installation, replica `EngineOptions` shared-state wiring | End-to-end host/replica shared-state composition tracer. |

## Additional strict-TDD defect predating that GREEN

`internal/core/auto_move.go` was written wholesale between the state RED and its GREEN. The test exercises only `NewAutoMoveState`, `Snapshot`, and case-sensitive `EligibleReplica`. The same write also introduced unproven production methods:

- `(*AutoMoveState).Configure`
- `(*AutoMoveState).SetEnabled`
- `(*EngineImpl).AutoMoveSnapshot`
- `(*EngineImpl).ConfigureAutoMove`
- `(*EngineImpl).EnableAutoMove`
- `(*EngineImpl).DisableAutoMove`
- `(*EngineImpl).reconcileAutoMove`

Related unproven wiring placed in `internal/core/engine_impl.go` is `autoMove`, `autoMoveMu`, `SetAutoMoveState`, and `AutoMoveState`. No test in the transcript failed first for configure, sorted/immutable snapshots, enable/disable, absent-only add, present-only remove, lifecycle error state semantics, cancellation, or concurrent reconciliation. Therefore this code cannot be retained merely because the default-state test passed.

## What may be retained without contradicting the recorded evidence

Only these narrow behavior slices have independently observed expected REDs and later GREENs:

1. **Concrete default command usage:** `automove` resolves to a non-generic command and, with no arguments, returns `FAILED` and emits the three exact default messages. This supports retaining `TestAutoMoveDefaultUsageIsConcreteModeratorCommand`, the `handlers.go` `case "automove"`, and a minimal default-only command implementation.
2. **Default state:** a fresh state is disabled with source `purgatory`, destination `lounge`, and exact/case-sensitive source lookup. This supports retaining `TestAutoMoveDefaultsAreFalsePurgatoryAndLounge`, `AutoMoveSnapshot`, and only the minimal `AutoMoveState` constructor/snapshot/eligibility code needed by that test.

These are **not acceptance evidence for the automove feature vertical**. Neither proves runtime registration, mutable state, lifecycle reconciliation, persistence boundary behavior, join actions, or host/replica composition.

## Safe restoration boundary: compilable two-tracer checkpoint

Do **not** use `reset`, `clean`, `checkout`, staging, or broad reversion in this dirty worktree. The safe boundary is a targeted, task-owned restoration to the state immediately after the two independently evidenced tracer behaviors, expressed as a manual source-level boundary for a later repair agent:

### Retain only

- `internal/command/automove_test.go` unchanged.
- `internal/core/auto_move_test.go` unchanged.
- `internal/command/handlers.go`: only the task-owned `case "automove": return &automoveCommand{b}` line, preserving unrelated dirty work.
- `internal/command/automove.go`: a default-only concrete command that emits the exact three expected default replies and returns `FAILED`; no controller assertion, parser, configuration, or lifecycle calls.
- `internal/common/automove.go`: `AutoMoveSnapshot` only.
- `internal/core/auto_move.go`: `AutoMoveState`, `NewAutoMoveState`, `Snapshot`, and `EligibleReplica` only.

### Remove from the automove slice before the next RED

- All controller/join-policy interfaces and mutable/lifecycle methods in `internal/common/automove.go` and `internal/core/auto_move.go`.
- AutoMove fields/accessors in `internal/core/engine_impl.go`.
- The AutoMove registration condition in `internal/command/dispatch_adapter.go` and the catalog-guard edit.
- New repository interface/H2 `UserTrips` query/method.
- New join automation file and `UserJoinedListener` automove field/call/constructor.
- `EngineOptions.AutoMoveState`, `composeAutoMoveJoin`, and related factory injection.
- Main host/replica AutoMove state construction/wiring.

This boundary is compilable because the retained command does not require a controller and the retained state does not require `EngineImpl`; it preserves both observed tests while removing all unproven dependencies. Required checkpoint verification after a future manual restoration:

```text
go test ./internal/command -run '^TestAutoMoveDefaultUsageIsConcreteModeratorCommand$' -count=1
go test ./internal/core -run '^TestAutoMoveDefaultsAreFalsePurgatoryAndLounge$' -count=1
go test ./internal/command ./internal/core -count=1
git diff --check
```

## Strict recovery: one tracer RED→GREEN at a time

Each numbered item is an exclusive cycle: add exactly one focused test, execute it and record its expected failure, write only the code necessary for that test, rerun it to GREEN, then run the stated package check. Do not pre-create later production seams.

1. **Retained checkpoint:** rerun and preserve recorded command-default and state-default GREENs.
2. **State mutation:** configure trim/blank rejection, additive source, replacement destination, sorted immutable snapshots, and configure-does-not-start. Then implement only state mutation.
3. **Core lifecycle:** direct `EngineImpl` tests for pre-cancelled context, enabled/disabled ordering, absent-only add, present-only remove, continue-on-stop-error/first-error, and concurrent reconciliation. Then add controller contract/engine lifecycle code and its fields.
4. **Command behavior:** capability absence, role/alias, `on|off` toggle-before-arity/trailing args, configure, invalid arity, cancellation, replies, and no success reply on lifecycle failure. Then expand command and capability-gated registration; remove generic fallback allowance only when its dedicated guard RED proves it.
5. **H2 eligibility repository:** USER-only exact query, case preservation, and query/scan/rows/context errors. Then introduce `AutoMoveTripRepository` and `UserTrips`.
6. **Join policy:** one behavior per test: disabled/non-replica/non-source/nil/blank gates; exact USER-trip match; case mismatch; query error; pre-cancel; literal notice; notice error still kicks; dynamic destination. Then introduce `AutoMoveJoinAutomation`.
7. **Listener order:** test add-active-user visibility and precise existing semantic → share → log → automove order, plus malformed payload inertness. Then compose it last in `UserJoinedListener`.
8. **Factory/composition:** prove incomplete engines do not register automove; fully composed host does; permanent replica (not temporary session) receives the hook; host/future replica share the identical state pointer; end-to-end eligible replica join moves user. Then wire factory/main.
9. **Acceptance only after every tracer has evidence:** focused command/core/H2/listener/factory tests, `go test -race ./internal/core -run '^TestAutoMove' -count=1`, `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check`.

## Current non-mutating baseline

Executed during this audit from `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./... -count=1                                  PASS
go test ./internal/command -run '^TestAutoMove' -count=1 PASS
go test ./internal/core -run '^TestAutoMove' -count=1    PASS
go test -race ./internal/core -run '^TestAutoMove' -count=1 PASS
go test ./internal/listener -run '^TestAutoMoveJoin' -count=1
  PASS, but “no tests to run”
go test ./internal/repository/h2 -run '^TestUserTrips' -count=1
  PASS, but “no tests to run”
git diff --check                                        PASS (no output)
```

A combined focused package command (`go test ./internal/command ./internal/core ./internal/listener ./internal/factory ./internal/repository/h2 -count=1`) passed command/core/listener/factory but exceeded the audit’s 180-second tool timeout before reporting H2; the separately executed full suite subsequently passed H2 and all packages. This is a timeout limitation, not a test failure.

## Source/spec impact

The current source compiles and the full suite is green, but that does not cure the missing test-first evidence. The implementation otherwise targets the architecture’s volatile shared state, source-shaped defaults/parsing, H2 USER-trip lookup, literal lounge notice, typed kick, last listener ordering, and shared host/replica state. The recovery boundary intentionally removes those unproven integrations until each architecture-required tracer has a recorded RED.
