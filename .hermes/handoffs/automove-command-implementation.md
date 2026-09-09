# AutoMove command and capability-gated registration — strict TDD

## Scope

Recovered recovery step 4 only: `automove` command parsing and registration in the already-dirty worktree. No repository, H2, listener, factory, main, configuration, or controller-core implementation was changed. No reset, clean, checkout, staging, or commit was used.

## Tracer evidence

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

### 1. Toggle before arity, trailing arguments

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveToggleUsesControllerBeforeArityAndIgnoresTrailingArguments$' -count=1
--- FAIL: TestAutoMoveToggleUsesControllerBeforeArityAndIgnoresTrailingArguments
    automove_test.go:75: status=FAILED err=<nil>
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveToggleUsesControllerBeforeArityAndIgnoresTrailingArguments$' -count=1
ok   zenbot/internal/command  0.443s
```

Added `common.AutoMoveController` and minimal case-insensitive `on` parsing. `on` is resolved before arity, ignores trailing input, calls enable once, and returns the exact success reply only after a successful controller call.

### 2. Off toggle

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveOffUsesControllerAndRepliesDisabled$' -count=1
--- FAIL: TestAutoMoveOffUsesControllerAndRepliesDisabled
    automove_test.go:93: status=FAILED err=<nil>
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveOffUsesControllerAndRepliesDisabled$' -count=1
ok   zenbot/internal/command  0.440s
```

Added case-insensitive `off` parsing with trailing-input tolerance, one disable call, and exact disabled reply.

### 3. Two non-toggle arguments configure

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveTwoArgumentsConfiguresAdditivelyAndRepliesSnapshot$' -count=1
--- FAIL: TestAutoMoveTwoArgumentsConfiguresAdditivelyAndRepliesSnapshot
    automove_test.go:113: status=FAILED err=<nil>
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveTwoArgumentsConfiguresAdditivelyAndRepliesSnapshot$' -count=1
ok   zenbot/internal/command  0.431s
```

Added exact-two non-toggle configuration; reply uses the returned snapshot’s deterministic source/destination formatting.

### 4. Invalid arity usage/status/configuration

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveBadArityUsesSnapshotShapedUsageReplies$' -count=1
--- FAIL: TestAutoMoveBadArityUsesSnapshotShapedUsageReplies
    automove_test.go:143: chat[1]="moderator|Current status: false , Source rooms: [purgatory] , Destination room: lounge|false" want "moderator|Current status: true , Source rooms: [hell purgatory] , Destination room: heaven|false"
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveBadArityUsesSnapshotShapedUsageReplies$' -count=1
ok   zenbot/internal/command  0.440s
```

Invalid arity now emits the three exact source-shaped replies from a controller snapshot. The pre-existing default-only behavior remains compatible when there is no controller.

### 5. Pre-cancelled context

RED:
```text
$ go test ./internal/command -run '^TestAutoMovePreCancelledContextHasNoControllerCallOrReply$' -count=1
--- FAIL: TestAutoMovePreCancelledContextHasNoControllerCallOrReply
    automove_test.go:158: status=SUCCESSFUL err=<nil>
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMovePreCancelledContextHasNoControllerCallOrReply$' -count=1
ok   zenbot/internal/command  0.438s
```

A pre-cancelled context returns `FAILED, context.Canceled` before any controller call or reply.

### 6. Missing controller fails closed

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveWithoutControllerFailsClosedWithoutReply$' -count=1
--- FAIL: TestAutoMoveWithoutControllerFailsClosedWithoutReply
    automove_test.go:170: status=FAILED err=<nil> chats=[moderator|!automove [on|off]|false]
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveWithoutControllerFailsClosedWithoutReply$' -count=1
ok   zenbot/internal/command  0.417s
```

Controller-dependent `on`, `off`, and configuration paths return an error and no success reply when the engine lacks `AutoMoveController`.

### 7. Capability-gated registration and catalog guard

RED:
```text
$ go test ./internal/command -run '^TestAutoMoveRegistersOnlyWithControllerAsSoleModeratorAlias$' -count=1
--- FAIL: TestAutoMoveRegistersOnlyWithControllerAsSoleModeratorAlias
    automove_test.go:188: automove not registered with controller
FAIL
```

GREEN:
```text
$ go test ./internal/command -run '^TestAutoMoveRegistersOnlyWithControllerAsSoleModeratorAlias$' -count=1
ok   zenbot/internal/command  0.710s
```

`RegisterUserUtilitiesWithDirectAgent` now registers `automove` only for `common.AutoMoveController`. The tracer verifies no incomplete-engine registration and that the registered command exposes only alias `automove` with `MODERATOR` role, preserving dispatch’s existing authorization gate. The generic-fallback catalog guard now includes automove in this registration tracer’s scope.

## Verification

```text
$ go test ./internal/command -run '^TestAutoMove' -count=1
ok   zenbot/internal/command  0.760s

$ go test ./internal/command -count=1
ok   zenbot/internal/command  9.620s

$ git diff --check
exit 0; no output
```

## Touched paths

- `internal/common/automove.go`
- `internal/command/automove.go`
- `internal/command/automove_test.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `.hermes/handoffs/automove-command-implementation.md`

## Explicit exclusions

No changes were made to repository/H2, listeners, factory/main composition, configuration, lifecycle/controller core implementation, or unrelated command behavior.
