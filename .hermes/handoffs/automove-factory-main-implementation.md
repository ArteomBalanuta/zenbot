# AutoMove factory/main composition — implementation record

## Scope

Completed strict recovery step 8 only: production shared-state composition for host/permanent replicas, permanent listener composition, and main construction options. Temporary snapshot sessions remain outside automove/replica composition.

## Tracer 1 — factory composition

**RED**

```text
$ go test ./internal/factory -run '^TestNewEngineWithOptionsSharesAutoMoveStateAndComposesPermanentReplicaJoinPolicy$' -count=1
internal/factory/automove_composition_test.go:18:24: unknown field AutoMoveState in struct literal of type EngineOptions
... master.AutoMoveState undefined
FAIL zenbot/internal/factory [build failed]
```

**GREEN**

```text
$ go test ./internal/factory -run '^TestNewEngineWithOptionsSharesAutoMoveStateAndComposesPermanentReplicaJoinPolicy$' -count=1
ok zenbot/internal/factory 0.471s
```

The tracer constructs a master and a future permanent replica through `ReplicaFactory`, proves their `AutoMoveState` pointers are identical, and drives a permanent replica `onlineAdd` through the listener: the literal notice precedes the dynamic-destination kick. A `TemporaryOnlineSet` engine given the same option retains its dummy listener and exposes no installed automove state.

## Tracer 2 — main composition

**RED**

```text
$ go test ./cmd/zenbot -run '^TestNewAutoMoveProductionOptionsSharesOneStateAndRegistersController$' -count=1
cmd/zenbot/automove_composition_test.go:15:42: undefined: newAutoMoveProductionOptions
FAIL zenbot/cmd/zenbot [build failed]
```

**GREEN**

```text
$ go test ./cmd/zenbot -run '^TestNewAutoMoveProductionOptionsSharesOneStateAndRegistersController$' -count=1
ok zenbot/cmd/zenbot 0.667s
```

`newAutoMoveProductionOptions` creates exactly one state and returns master/replica options holding the same pointer. The tracer builds the master, runs normal utility registration, and verifies the complete engine registers `automove`.

## Task-owned changes

- `internal/factory/engine_factory.go`: adds `EngineOptions.AutoMoveState`; installs it only on permanent engines; composes `AutoMoveJoinAutomation` only when both state and trip repository capability exist; retains temporary dummy listeners.
- `internal/core/engine_impl.go`: exposes construction-only state accessor/setter and aligns the pre-existing lifecycle methods with `common.AutoMoveController` snapshot-returning contract, enabling capability-gated registration.
- `cmd/zenbot/main.go`: creates one state via the composition helper and passes it into both master snapshot options and the future permanent replica factory options.
- `internal/factory/automove_composition_test.go`, `cmd/zenbot/automove_composition_test.go`: new tracer tests.
- `internal/core/auto_move_test.go`: mechanical call-site updates for the controller's existing snapshot-returning API contract.

## Verification

```text
$ go test ./internal/factory ./cmd/zenbot ./internal/core ./internal/listener ./internal/command -count=1
PASS
$ go test -race ./internal/core ./internal/factory ./internal/listener -count=1
PASS
$ go test ./... -count=1
PASS
$ go vet ./...
PASS
$ go build ./...
PASS
$ git diff --check
PASS
```

## Retained limitation

Chat-dispatched commands still run through `legacyAdapter` with `context.Background()`; cancellation propagation is deliberately unchanged. No runtime/websocket integration, command parsing, repository/policy behavior, temporary-session behavior, schema, or remote snapshot behavior was altered.
