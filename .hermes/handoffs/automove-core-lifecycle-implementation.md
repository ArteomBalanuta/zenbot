# AutoMove core lifecycle strict-TDD implementation

## Tracer 1 — enable reconciliation

### RED

```text
$ go test ./internal/core -run '^TestEngineEnableAutoMoveMarksStateBeforeAddingOnlyMissingSources$' -count=1
internal/core/auto_move_test.go:61:41: unknown field autoMove in struct literal of type EngineImpl
internal/core/auto_move_test.go:76:29: engine.EnableAutoMove undefined (type *EngineImpl has no field or method EnableAutoMove)
FAIL    zenbot/internal/core [build failed]
```

### GREEN

Added only the state enable mutation plus `EngineImpl.EnableAutoMove` and its private reconciliation mutex. It checks cancellation before mutation, serializes reconciliation, enables before taking a state snapshot, adds only missing exact source rooms, and takes no state or manager lock across `AddReplica`.

```text
$ go test ./internal/core -run '^TestEngineEnableAutoMoveMarksStateBeforeAddingOnlyMissingSources$' -count=1
ok      zenbot/internal/core  0.435s
```

## Tracer 2 — pre-cancelled lifecycle

### RED

```text
$ go test ./internal/core -run '^TestEngineAutoMovePreCancelledContextDoesNotMutateOrAct$' -count=1
internal/core/auto_move_test.go:116:19: engine.DisableAutoMove undefined
FAIL    zenbot/internal/core [build failed]
```

### GREEN

`DisableAutoMove` now accepts and rejects a cancelled context before it can mutate state or call the replica controller.

```text
ok      zenbot/internal/core  0.469s
```

## Tracer 3 — disable reconciliation

### RED

```text
$ go test ./internal/core -run '^TestEngineDisableAutoMoveMarksDisabledRemovesPresentSourcesAndReturnsFirstStopError$' -count=1
DisableAutoMove() error = automove disable is not implemented, want first stop error
FAIL
```

### GREEN

`DisableAutoMove` now serializes with enable, marks state disabled first, snapshots managed channels, removes only currently present configured sources, continues after stop failures, and returns the first error.

```text
ok      zenbot/internal/core  0.355s
```

## Tracer 4 — concurrent on/off reconciliation

### RED

```text
$ go test ./internal/core -run '^TestEngineAutoMoveConcurrentEnableDisableReconcilesToDisabledWithoutReplicas$' -count=1
disable completed before blocked enable reconciliation: <nil>
FAIL
```

### GREEN

Added the private reconciliation mutex to `EngineImpl`. It serializes enable and disable while state and replica-manager locks remain confined to their own snapshot/mutation calls; `AddReplica` and `RemoveReplica` run without either of those locks held.

```text
$ go test ./internal/core -run '^TestEngineAutoMoveConcurrentEnableDisableReconcilesToDisabledWithoutReplicas$' -count=1
ok      zenbot/internal/core  0.530s
```

## Final scoped verification

```text
$ go test -race ./internal/core -run '^TestEngine(AutoMove|EnableAutoMove|DisableAutoMove)' -count=1
ok      zenbot/internal/core  1.496s

$ go test ./internal/core -count=1
ok      zenbot/internal/core  0.435s

$ git diff --check
exit 0; no output
```
