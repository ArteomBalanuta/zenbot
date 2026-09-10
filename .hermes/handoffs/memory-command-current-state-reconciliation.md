# ADMIN `memory` command: current-state reconciliation

## Verdict — **ACCEPTED: no implementation work remains**

The ADMIN process-memory vertical is already accepted. A new `memory` / `mem` / `memstats` implementation would duplicate the reachable, unchanged `f9079ca` implementation and risk drifting from its accepted contract. The selection in `next-core-command-after-msgchannel-architecture.md` is **stale as a next-work recommendation**; its source/runtime description is substantially correct, but it did not reconcile the already-recorded strict-TDD acceptance.

The only follow-up justified by this audit is documentation/planning reconciliation. Do not modify application or test code for this command unless a new, demonstrated contract defect is found.

## Scope and evidence inventory

### Command-specific handoffs

| Handoff | State | Reconciliation finding |
|---|---|---|
| `.hermes/handoffs/next-rapid-command-slice-architecture.md` | Historical plan | Specified the concrete ADMIN memory slice and its first tracer before the implementation. |
| `.hermes/handoffs/next-rapid-command-slice-implementation.md` | Historical implementation record | Records the named tracer's expected RED (`alias "mem" was not registered`) followed by GREEN. |
| `.hermes/handoffs/next-rapid-command-slice-qa.md` | Historical QA | **ACCEPT**; confirms aliases, concrete construction/registration, cancellation, runtime report, and routing. |
| `.hermes/handoffs/memory-command-tdd-forensic.md` | Current forensic reconciliation | **ACCEPT — no recovery/rebuild**; establishes that the files are tracked, committed by `f9079ca`, and unchanged. |
| `.hermes/handoffs/memory-command-current-architecture.md` | Current architecture | Factually stale in its claim that the implementation is dirty/untracked and lacks antecedent RED proof. Its ownership/runtime observations otherwise describe the accepted implementation. |
| `.hermes/handoffs/next-core-command-after-msgchannel-architecture.md` | Latest conflicting selection | **Stale as a next vertical:** it selects an already accepted command, despite describing the existing concrete path. |

### Related agent-memory handoffs (not this command)

- `.hermes/handoffs/rapid-agent-durable-memory-implementation.md` and `rapid-agent-durable-memory-qa.md` accept durable conversation persistence; they explicitly exclude tool memory and do not provide/administer the `memory` command.
- `.hermes/handoffs/rapid-agent-after-memory-architecture.md` proposes a future durable tool-evidence vertical; it is unrelated to the ADMIN process-memory report.

These agent-memory documents must not be used to broaden `memory` into database counts, queue/scheduler statistics, or user/agent-memory disclosure.

## Historical acceptance and clean ownership

[OBSERVED] Reachable commit `f9079caf4890c67f9e298369a23c42409151def8` (`feat: advance Saturn parity migration`, 2026-08-31) introduced together:

- `internal/command/memory.go`
- `internal/command/memory_test.go`
- the `case "memory"` concrete-construction hunk in `internal/command/handlers.go`
- the unconditional `"memory"` registration entry in `internal/command/dispatch_adapter.go`

[OBSERVED] Its parent has neither memory source/test file, no `case "memory"`, and no `"memory"` registration literal. Both local `migration/saturn-zenbot-parity` and `origin/migration/saturn-zenbot-parity` contain `f9079ca`.

[TEST-BACKED historical record] `next-rapid-command-slice-implementation.md` records the tracer first failing because `mem` was not registered, then passing after the four owned hunks. `next-rapid-command-slice-qa.md` independently accepts the completed slice. The parent-tree absence exactly matches that RED cause.

[OBSERVED current tree] `internal/command/memory.go` and `internal/command/memory_test.go` are tracked and have no working-tree diff. `git diff --quiet f9079ca --` those two paths exits zero. The broad dirty worktree affects shared integration files, but not either owned memory file. This audit makes no application/test edits.

## Present executable contract

### Registration and aliases

[OBSERVED] `internal/command/registry.go:RegisterAll` declares canonical `memory`, aliases in source order `mem`, `memory`, `memstats`, and `model.ADMIN`.

[OBSERVED] `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` includes `memory` in its unconditional concrete canonical list. Production calls this registration from `cmd/zenbot/main.go`; it does not depend on agent, repository, H2, snapshot, proxy, or replica capability.

[OBSERVED] `internal/command/handlers.go:newCommand` maps canonical `memory` to `*memoryCommand`. `internal/command/admin_moderator_catalog_guard_test.go` captures the source row and permits generic fallback only for `mine`, `restart`, `shutdown`, `sql`, and `whiskey`; `memory` is excluded.

### Role gate and runtime route

[OBSERVED] `internal/listener/message/handlers.go:DispatchUserCommand.Handle` parses the first prefix-delimited token, builds the registered command, and checks `Engine.IsUserAuthorized` before calling `ExecuteContext`. A denied resolved caller receives the shared denial and does not execute the command. There is no command-local duplicate authorization policy.

[OBSERVED] `memoryCommand.Execute` in `internal/command/memory.go` checks cancellation before work, calls `runtime.GC()`, reads `runtime.MemStats`, sends exactly one `reply`, and returns `model.SUCCESSFUL`. It ignores arguments. Pre-cancellation returns `model.FAILED` with the context error and no reply.

[OBSERVED] The formatter retains Saturn-shaped labels and literal `\\n` separators, with Go adaptation `Alloc`, `HeapIdle`, `HeapSys`, and `Sys` for Used, Free, Total, and Max MiB respectively. These are process-runtime measurements, not JVM-equivalent heap semantics or a Go hard memory limit.

[OBSERVED] `reply` preserves inbound whisper from `IsWhisper`, `Whisper`, or `Type == "whisper"`; target `EngineImpl.SendChatMessage` applies the target's addressed public/whisper wire format. The shared transport's whisper prefix differs from Saturn's, so the accepted command follows target transport conventions rather than performing a command-local rewrite.

### Current proof

[TEST-BACKED] `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport` verifies all three aliases register; `memstats` accepts surplus arguments; a whisper invocation sends one source-shaped report; and a pre-cancelled call sends no reply and returns `FAILED`/`context.Canceled`.

[TEST-BACKED] Current focused baseline passed:

```text
go test ./internal/command -run '^(TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport|TestAdminModeratorCatalogMatchesSaturnSource|TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded)$' -count=1
ok  zenbot/internal/command

go test ./internal/listener/message -run 'Test.*(Dispatch|Authorization)' -count=1
ok  zenbot/internal/listener/message

git diff --check
git diff --cached --check
# both clean
```

## Decision boundary

- **Accepted/no work:** aliases, ADMIN metadata, concrete construction, unconditional runtime registration, pre-execution authorization, GC/stat/report behavior, ignored arguments, addressed public/whisper routing, and cancellation behavior.
- **Not missing:** persistent agent memory, agent-tool evidence, database row counts, queue capacity/depth, scheduler status, OS memory/cap telemetry, or a new admin disclosure policy.
- **Planning correction:** remove `memory` / `mem` / `memstats` from the unimplemented-next-vertical candidate set. Do not recreate historical RED by deleting accepted code, and do not add duplicate handlers/tests merely to manufacture a new vertical.
