# AutoMove strict-TDD recovery

## Scope and authority

Restored only the manually compilable checkpoint specified by `.hermes/handoffs/automove-tdd-forensic.md` §§57–96. This is a targeted source-level recovery in the existing dirty worktree; no reset, clean, checkout, staging, or commit was used. Unrelated dirty hunks in shared files were preserved.

## Retained evidence-backed checkpoint

- `internal/command/automove_test.go` unchanged: `TestAutoMoveDefaultUsageIsConcreteModeratorCommand`.
- `internal/core/auto_move_test.go` unchanged: `TestAutoMoveDefaultsAreFalsePurgatoryAndLounge`.
- `internal/command/handlers.go`: the concrete `case "automove": return &automoveCommand{b}` remains; unrelated handler migration hunks remain untouched.
- `internal/command/automove.go`: `automoveCommand.Execute` now has only the evidenced default behavior. It returns `model.FAILED` and emits exactly:
  1. `<prefix>automove [on|off]`
  2. `Current status: false , Source rooms: [purgatory] , Destination room: lounge`
  3. `To set source: hell, destination: heaven - use: <prefix>automove hell heaven`
- `internal/common/automove.go`: `AutoMoveSnapshot` only.
- `internal/core/auto_move.go`: `AutoMoveState`, `NewAutoMoveState`, `Snapshot`, and case-sensitive `EligibleReplica` only.

## Removed unproven automove work

- Removed controller/join-policy contracts from `internal/common/automove.go`.
- Removed state mutation and lifecycle/reconciliation methods from `internal/core/auto_move.go`: `Configure`, `SetEnabled`, `EngineImpl.AutoMoveSnapshot`, `ConfigureAutoMove`, `EnableAutoMove`, `DisableAutoMove`, and `reconcileAutoMove`.
- Removed `EngineImpl` automove fields/accessors from `internal/core/engine_impl.go`.
- Removed capability-gated `automove` registration from `internal/command/dispatch_adapter.go`.
- Kept the broad catalog guard but excluded its automove generic-fallback assertion; the independently evidenced tracer is the sole automove behavior assertion at this checkpoint.
- Deleted untracked, unproven `internal/repository/automove.go` and `internal/listener/automove_join.go`.
- Removed H2 `selectUserTrips` and `(*Database).UserTrips` from `internal/repository/h2/user_queries.go`.
- Removed automove listener composition/field/call/constructor from `internal/listener/user_joined_listener.go` (the file now has no automove diff).
- Removed `EngineOptions.AutoMoveState`, factory injection, and `composeAutoMoveJoin` from `internal/factory/engine_factory.go`.
- Removed host/replica state construction and wiring from `cmd/zenbot/main.go`.

## Scoped-diff and preservation evidence

Before editing, the worktree had numerous unrelated tracked and untracked changes. Shared files were inspected hunk-by-hunk before patching. Final automove symbol search finds only the two tests, the minimal command/handler, snapshot/state, and pre-existing catalog/help references; it finds no `AutoMoveController`, `AutoMoveJoinPolicy`, `AutoMoveTripRepository`, `AutoMoveJoinAutomation`, lifecycle methods, `UserTrips`, listener composition, factory injection, or main wiring.

The shared files still dirty after recovery contain their unrelated existing changes plus only the targeted removal of automove hunks: `cmd/zenbot/main.go`, `internal/command/dispatch_adapter.go`, `internal/command/handlers.go`, `internal/core/engine_impl.go`, `internal/factory/engine_factory.go`, and `internal/repository/h2/user_queries.go`. `internal/listener/user_joined_listener.go` returned to its non-automove content. No unrelated files were reset, cleaned, staged, or removed.

## Verification record

All commands were run from `/Users/ab/workspace/go-projects/zenbot` after recovery.

```text
$ go test ./internal/command -run '^TestAutoMoveDefaultUsageIsConcreteModeratorCommand$' -count=1
ok  zenbot/internal/command  0.570s

$ go test ./internal/core -run '^TestAutoMoveDefaultsAreFalsePurgatoryAndLounge$' -count=1
ok  zenbot/internal/core  0.258s

$ go test ./internal/command ./internal/core -count=1
ok  zenbot/internal/command  9.088s
ok  zenbot/internal/core  0.542s

$ git diff --check
exit 0; no output

$ go test ./... -count=1
PASS: all packages; H2 package passed in 37.763s

$ go vet ./...
exit 0; no output

$ go build ./...
exit 0; no output
```

## Remaining recovery sequence

Do not implement more automove production code until a new focused test has been written and observed RED. Continue exactly as the forensic report §98–110 orders it: state mutation; core lifecycle; command parsing/registration; H2 eligibility repository; join policy; listener ordering; factory/main composition; then acceptance/race verification. Each is a separate RED → minimal GREEN cycle.
