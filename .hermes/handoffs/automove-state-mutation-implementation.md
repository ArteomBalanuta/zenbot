# AutoMove state-mutation strict-TDD implementation

## Scope

Completed recovery step 2 only: synchronized volatile `AutoMoveState.Configure`. No lifecycle, command parsing/registration, repository, listener, factory, controller, or main composition behavior was added.

## Strict TDD record

### RED

A single focused test, `TestAutoMoveStateConfigure`, was added to `internal/core/auto_move_test.go` before production changes. It specifies trimming, blank rejection, additive source state, destination replacement, sorted immutable snapshots, and preservation of snapshots returned before configuration.

Executed from `/Users/ab/workspace/go-projects/zenbot`:

```text
$ go test ./internal/core -run '^TestAutoMoveStateConfigure' -count=1
# zenbot/internal/core [zenbot/internal/core.test]
internal/core/auto_move_test.go:23:16: s.Configure undefined (type *AutoMoveState has no field or method Configure)
internal/core/auto_move_test.go:39:17: s.Configure undefined (type *AutoMoveState has no field or method Configure)
internal/core/auto_move_test.go:42:17: s.Configure undefined (type *AutoMoveState has no field or method Configure)
FAIL	zenbot/internal/core [build failed]
FAIL
```

This is the expected RED: `Configure` was absent.

### GREEN

Added only `(*AutoMoveState).Configure` and the private locked snapshot helper it needs. `Configure` trims both arguments, rejects either blank input before state mutation, adds the source to the existing set, replaces destination, and returns a copied, sorted snapshot while holding the state lock. `Snapshot` now uses that helper under its read lock. No enabled-state or lifecycle mutation was introduced.

```text
$ go test ./internal/core -run '^TestAutoMoveStateConfigure' -count=1
ok  	zenbot/internal/core	0.465s
```

## Verification

```text
$ go test -race ./internal/core -run '^TestAutoMoveState' -count=1
ok  	zenbot/internal/core	1.378s

$ go test ./internal/core -count=1
ok  	zenbot/internal/core	0.322s

$ git diff --check
exit 0; no output
```

## Files touched

- `internal/core/auto_move_test.go` — one focused state-mutation tracer.
- `internal/core/auto_move.go` — minimal `Configure` implementation and locked snapshot helper.
- `.hermes/handoffs/automove-state-mutation-implementation.md` — this evidence record.

## Source/spec justification

Saturn `AutoMoveUserCommandImpl` configuration adds a source and replaces destination without launching replicas. The architecture handoff §3.3 and §3.65 requires the bounded Go safety adaptation: trim source/destination and reject blank values while retaining exact case-sensitive membership. The recovery forensic §102 identifies this exact state-mutation tracer as the next permitted slice; snapshot sorting/copy isolation make state reporting deterministic and prevent callers from mutating shared state.

## Explicit exclusions

Not added: `SetEnabled`; any `EngineImpl` fields/accessors/controller/reconciliation; command behavior or registration; repository/H2 queries; join policy/listener composition; factory/replica/main wiring; persistence/config/schema; source removal; or lifecycle/integration behavior.
