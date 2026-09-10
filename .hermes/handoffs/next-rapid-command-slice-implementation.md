# Next rapid command slice — admin memory implementation

## Outcome

Implemented the source-grounded admin `memory` command path. The existing catalog continues to provide the exact aliases `mem`, `memory`, and `memstats` with `ADMIN` role; concrete registration now exposes those aliases to listener dispatch.

## TDD record

### RED

Test added first: `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport`.

```text
--- FAIL: TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport (0.08s)
    memory_test.go:22: alias "mem" was not registered
FAIL
FAIL    zenbot/internal/command    0.609s
FAIL
```

This was the expected missing-concrete-registration failure.

### GREEN

```text
ok      zenbot/internal/command    0.579s
```

The focused test verifies all three aliases register, `memstats` ignores extra arguments, returns one whisper reply with Saturn-shaped labels/order/thin-space padding/literal escaped `\\n` trailing shape, and returns `FAILED` with `context.Canceled` without a reply for a canceled context.

## Implementation

- `memoryCommand.Execute` checks cancellation before observation, calls `runtime.GC()`, reads `runtime.MemStats`, replies via the existing `reply` behavior, and returns `SUCCESSFUL`.
- The Go-runtime metric adaptation preserves Saturn labels but maps them explicitly:
  - `JVM Used Memory` ← `Alloc / MiB`
  - `JVM Free Memory` ← `HeapIdle / MiB`
  - `JVM Total Memory` ← `HeapSys / MiB`
  - `JVM Max Memory` ← `Sys / MiB`
- `newCommand` now owns canonical `memory`; the dispatch adapter unconditionally includes it in its concrete canonical list.

## Exact touched files

- Created `internal/command/memory.go`
- Created `internal/command/memory_test.go`
- Modified `internal/command/handlers.go` (canonical `memory` concrete dispatch case)
- Modified `internal/command/dispatch_adapter.go` (unconditional concrete `memory` registration)
- Created `.hermes/handoffs/next-rapid-command-slice-implementation.md`

`handlers.go` and `dispatch_adapter.go` contained unrelated pre-existing dirty work; this slice changed only the memory case/list entry.

## Verification

```text
$ go test ./internal/command -count=1
ok      zenbot/internal/command    5.446s

$ go test ./...
PASS: all packages; no package reported a failure.

$ git diff --check
PASS: no output, exit 0.
```

`gofmt -w` ran on the four permitted Go files before the focused GREEN run.

## Exclusions retained

No changes to configuration, agent, H2/SQL, main, migration-plan/audit files, catalog/help cleanup, transport/lifecycle/replica code, or Saturn source. No commit or push was made.
