# Next rapid command slice — admin memory (`mem` / `memory` / `memstats`)

## Decision

**Select the admin memory command as the next minimal executable non-SQL slice.** It is a complete local process-observation command: it needs no database/service method, no H2/schema change, no remote-room/session/replica topology, and no agent runtime. Zenbot already has catalog aliases, role-gated listener dispatch, outbound replies, and a deliberately narrow concrete-command registration seam. The required work is a single concrete command plus registration and one focused TDD test.

This is preferable to the nearby candidates:

- `prefix` requires a mutable prefix capability and replica propagation. Zenbot's `common.Engine` exposes only `GetPrefix`, and `core.EngineImpl` only implements that getter; there is no setter or replica enumeration/propagation seam.
- `mine` is explicitly dependent on Saturn snapshot/session/proxy lifecycle (`MineTripCommandImpl` builds `EngineSnapshotSession` and a scheduled miner), which is not a rapid standalone command.
- `lastonline` requires a target `UserService.LastOnline`/repository behavior that has no target owner; current Zenbot only catalogues/help-documents it and its generic catalog branch returns a placeholder.
- The direct `l` command is already accepted separately; agent participation/relay remains blocked by the reviewed topology/runner/finalizer handoff and is not reopened here.

## Observed source behavior

### Saturn

- `src/main/java/org/saturn/app/command/impl/admin/MemoryCommandImpl.java` (`MemoryCommandImpl`, `execute`) declares exactly `mem`, `memory`, and `memstats` (`@CommandAliases`, line 17).
- It inherits the default `ADMIN` command role from `src/main/java/org/saturn/app/command/UserCommandBaseImpl.java` (`getAuthorizedRole`, lines 132–135); its constructor supplies the configured admin-and-user trip list.
- `MemoryCommandImpl.execute` calls `Runtime.getRuntime().gc()`, reads total/free/max JVM memory, computes used as `total - free`, converts each to integer MiB by division with `1024 * 1024`, formats four labels, aligns them with a thin space (`U+2009`), and replies to the invoking nick in the incoming whisper mode. It returns `SUCCESSFUL` and does not inspect command arguments.
- The four output labels and their source order are `JVM Used Memory`, `JVM Free Memory`, `JVM Total Memory`, and `JVM Max Memory` (`MemoryCommandImpl.execute`, lines 37–53).
- `src/main/java/org/saturn/app/util/Util.java` (`alignWithWhiteSpace`, lines 111–143) splits the literal `\\n` separators, pads keys to the longest key with the provided thin space, writes `key:value\\n` for every line, and preserves the trailing literal `\\n`. In this payload, the longest key is `JVM Total Memory`; therefore used/free receive one thin space and max receives two before `:`.
- No dedicated Saturn `MemoryCommandImplTest` exists in the source test tree. The Go focused test must therefore assert the source-observed contract, not invent fixed process-memory values.

### Zenbot

- `internal/command/registry.go` (`RegisterAll`, line 131) already has the exact alias set `[]string{"mem", "memory", "memstats"}` and assigns `model.ADMIN`; the same file's generic `saturnCommand.Execute` includes `memory` in an empty/no-op branch (line 92). Thus catalog presence is **not** runtime parity.
- `internal/command/handlers.go` (`newCommand`, lines 278–339) dispatches known concrete command types and otherwise returns `saturnCommand`; it has no memory case.
- `internal/command/dispatch_adapter.go` (`RegisterUserUtilitiesWithDirectAgent`, lines 37–80) intentionally registers only verified concrete commands. Its `canonicals` slice does not contain `memory`, so no memory alias reaches the listener today.
- `internal/listener/message/handlers.go` (`DispatchUserCommand.Handle`, lines 132–155) builds the command, checks `cmd.GetRole()` through `Engine.IsUserAuthorized`, and invokes it only when authorized. This supplies the needed admin gate without new authorization code.
- `internal/common/engine.go` (`Engine`, lines 5–57) already supplies `SendChatMessage`; `internal/command/handlers.go` (`reply`, lines 72–74) routes replies to the source nick and selects whisper when `IsWhisper`, `Whisper`, or `Type == "whisper"` is set.
- `internal/command/handlers_test.go` (`commandEngineStub`, lines 25–126) is a reusable recording engine with an authorization-positive stub and reply capture. `internal/command/dispatch_integration_test.go` proves the existing listener-to-registered-command route.
- Existing command-package baseline was executed before this architecture handoff: `go test ./internal/command -count=1` passed.

## Selected behavior contract

### Command and aliases

- Canonical command: `memory`.
- Registered aliases: `mem`, `memory`, `memstats` exactly, retaining the existing catalog definition and `ADMIN` role.
- Do not add aliases or change catalog ordering.

### Successful invocation

1. Authorization remains entirely in `message.DispatchUserCommand`: an authorized admin invocation reaches the command; an unauthorized invocation produces the existing listener denial behavior and does not execute it.
2. The command ignores all arguments, requests a Go runtime GC (`runtime.GC()`), reads `runtime.MemStats`, and emits one reply to `ChatMessage.Name` with the normal command `reply` whisper selection.
3. Preserve Saturn's observable label/order/alignment and literal escaped-newline shape:

   ```text
   JVM Used Memory : <usedMiB> MB \nJVM Free Memory : <freeMiB> MB \nJVM Total Memory: <totalMiB> MB \nJVM Max Memory  : <maxMiB> MB \n
   ```

   Here ` ` is U+2009 and `\n` denotes two literal characters (`\\` then `n`), not an actual line break. A terminal `\n` is required.
4. Map metrics explicitly rather than pretending the Go and JVM heaps are identical: `usedMiB = Alloc / MiB`, `freeMiB = HeapIdle / MiB`, `totalMiB = HeapSys / MiB`, `maxMiB = Sys / MiB`. The labels remain Saturn-compatible, while the implementation/test documentation must identify this as a Go-runtime adaptation. Do not introduce OS memory probing, SQL persistence, or a new metrics service.

### Errors and edge behavior

- A canceled input `context.Context` returns `(model.FAILED, ctx.Err())` before GC/stat reading and sends no reply, consistent with the concrete command convention in `internal/command/handlers.go`.
- Empty, whitespace-only, or extra arguments remain successful and do not alter output format; Saturn does not validate arguments.
- Existing `reply` discards outbound-send errors and returns `SUCCESSFUL`; retain that local command convention rather than expanding error semantics in this slice.
- A caller not authorized at listener dispatch is denied by the established `DispatchUserCommand` message and the memory handler is not called. No command-local duplicate authorization check is needed.

## Implementation shape and exact target files

1. **Create `internal/command/memory.go`.**
   - Define `memoryCommand` embedding `commandBase`.
   - Implement `Execute(ctx context.Context) (model.Status, error)` with the cancellation check, `runtime.GC`, `runtime.ReadMemStats`, explicit MiB conversion, a small private formatter for the exact four-line literal-`\\n` payload, `reply`, and successful status.
   - Keep the formatter private to this file. It is command-specific output parity, not a general metrics abstraction.

2. **Modify `internal/command/handlers.go`.**
   - Add `case "memory": return &memoryCommand{b}` in `newCommand`.
   - Do not modify generic `saturnCommand.Execute` other than removing `memory` from its empty placeholder group if needed to make the concrete ownership unambiguous. Do not alter unrelated placeholders.

3. **Modify `internal/command/dispatch_adapter.go`.**
   - Append canonical `memory` to the concrete `canonicals` registration list so the existing catalog aliases are actually exposed.
   - Registration is unconditional: no service bundle, database, transport, agent, or replica capability is required.

4. **Create `internal/command/memory_test.go`.**
   - Own the focused baseline described below. Use the existing `commandEngineStub`; do not broaden cross-package stubs or alter unrelated dispatch tests.

No changes are authorized for `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, SQL/H2 files, `cmd/zenbot/main.go`, configuration, factory wiring, `common.Engine`, `core.EngineImpl`, agent code, transport, lifecycle, or replica code.

## Focused TDD baseline

Write this test **before** `memory.go`, run it red, then implement only enough to pass:

`TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport`

- Construct `commandEngineStub` with an admin author, call `RegisterUserUtilities`, and verify `mem`, `memory`, and `memstats` are all registered.
- For one alias (use `memstats`) create the command through the registered definition or listener path, invoke it with extra arguments and a whisper message, and assert `SUCCESSFUL`, no error, and exactly one whisper reply.
- Assert the payload matches four ordered labels, U+2009 padding counts `1/1/0/2`, decimal MiB fields, a space before each literal `\\n`, and a final literal `\\n`. Do **not** assert the dynamic numbers.
- In the same focused test, invoke with an already-canceled context and assert `FAILED`, `context.Canceled`, and no reply. This proves the existing concrete-command error convention without a matrix of runtime statistic cases.

The implementation author must record the expected initial RED failure (missing concrete handler/registration) before writing production code, then run the same test green.

## Required validation

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation:

```sh
gofmt -w internal/command/memory.go internal/command/memory_test.go internal/command/handlers.go internal/command/dispatch_adapter.go
go test ./internal/command -run '^TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport$' -count=1
go test ./internal/command -count=1
go test ./...
git diff --check
```

The first is the focused TDD test; the second is the relevant package gate; the third is the rapid-policy full gate. Do not require new SQL/H2, race, vet, build, or stress gates unless one of these commands exposes an active-slice blocker.

## Explicit exclusions and deferred behavior

- JVM heap semantics beyond the documented Go-runtime metric adaptation; no OS/process/host memory abstraction and no fixed metric-value promise.
- `prefix` mutation/replica propagation, `mine` snapshot/proxy scheduling, remote-room/Whiskey, replica/session lifecycle, DBZ, persistence-backed `lastonline`, moderation/admin placeholders, and lifecycle commands.
- All SQL/H2 schema/repository/service work.
- Agent participation, relay, topology, asynchronous runner/finalizer, tools, memory persistence, and `l` work; the accepted direct-`l` and blocked participation architecture remain unchanged.
- Help-text or catalog-wide cleanup. `internal/command/help.go` already documents `mem,memory` as JVM memory usage, but its text is not proof that the command is executable and does not need a change for this slice.

## Complexity, risk, and routing

- **Complexity:** low; four production-file changes/additions and one focused test, all within `internal/command`.
- **Primary risk:** treating catalog presence or a generic acknowledgement as completion. The implementation must register all aliases and produce the source-shaped report through real listener/command seams.
- **Secondary risk:** claiming a Go heap is a JVM heap. Preserve labels/output, document the metric mapping, and keep it bounded as a runtime adaptation.
- **@developer routing:** standard @developer implementation task. The command has complete local execution/authorization/reply seams and no unresolved topology, lifecycle, SQL, or agent prerequisite.
