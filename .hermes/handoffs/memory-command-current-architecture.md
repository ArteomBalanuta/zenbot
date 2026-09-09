# Admin `memory` command parity: current architecture

## Decision

[RECOMMENDED] **Yes — `memory` is a safe concrete, non-agent command vertical**, provided parity means Saturn's four process-runtime numbers and preserves its forced collection. It has no arguments, no database/service dependency, no agent-memory disclosure, and no command-specific policy decision.

[OBSERVED] This checkout already has a dirty/untracked concrete implementation at `internal/command/memory.go` and its focused test at `internal/command/memory_test.go`; `go test ./internal/command` passed on the current worktree. Treat those as uncommitted work requiring review, not as a clean-baseline claim.

[RECOMMENDED] Do **not** redefine the command as agent memory, durable-memory row counts, scheduler state, queue depth, or LLM capacity. Those values are neither Saturn parity nor uniformly observable through the target's public interfaces; choosing their labels, scope, and disclosure would introduce a new operational policy.

## Source evidence: Saturn behavior

### Contract

[OBSERVED] `MemoryCommandImpl` is annotated with exactly `mem`, `memory`, and `memstats` in that order (`../projects/saturn/src/main/java/org/saturn/app/command/impl/admin/MemoryCommandImpl.java`, `@CommandAliases`, lines 17–18).

[OBSERVED] Its constructor delegates to `UserCommandBaseImpl`, then replaces the parsed alias list with the catalog aliases. `execute()` does not inspect `getArguments()`; extra tokens are accepted and ignored (same file, lines 21–25 and 28–56). There is no usage/error reply for absent, malformed, or surplus arguments.

[OBSERVED] On every authorized invocation, Saturn calls `Runtime.getRuntime().gc()` before sampling `totalMemory`, `freeMemory`, `maxMemory`, and deriving `usedMemory = totalMemory - freeMemory` (same file, lines 29–35). It renders exactly four integer-truncated MiB lines, each with a literal `\\n` terminator:

```text
JVM Used Memory: <used> MB \\n
JVM Free Memory: <free> MB \\n
JVM Total Memory: <total> MB \\n
JVM Max Memory: <max> MB \\n
```

[OBSERVED] Saturn then applies `Util.alignWithWhiteSpace(..., ":", "\u2009", false)` before routing the payload to `outService.enqueueMessageForSending(author, payload, isWhisper())`, logs the execution, and returns `SUCCESSFUL` (`MemoryCommandImpl.java`, lines 37–56). The alignment helper is a presentation concern; it does not alter the metric choice.

[OBSERVED] The command inherits `ADMIN` as the base default required role (`UserCommandBaseImpl.java`, `getAuthorizedRole`, lines 132–135). Its configured allowed-trip list is `adminTrips` plus `userTrips` (`Util.java`, `getAdminAndUserTrips`, lines 43–48). The shared dispatcher authorizes before command execution: app trip allow-list/wildcard or persisted role at/above the required role; denial queues `msg mercury for access.` to the caller preserving incoming whisper state (`AuthorizationServiceImpl.java`, lines 31–39 and 63–109; `UserCommandBaseImpl.java`, lines 83–102).

[OBSERVED] Reply transport preserves the inbound whisper flag. Saturn output formats a whisper as `/whisper @<author> <payload>` and a public answer as `@<author> <payload>` (`OutService.java`, `formatAddressedMessage`, lines 93–107). Therefore `memory` sends one addressed public reply for public input and one addressed whisper reply for whisper input.

[LIMITATION] No Saturn test mentioning `MemoryCommandImpl`, `memstats`, or the JVM report was found under `../projects/saturn/src/test`; the source implementation is the primary evidence for payload and forced-GC semantics.

## Target architecture and current state

### Registration, lookup, authorization, and dispatch

[OBSERVED] The target catalog declares canonical `memory`, aliases `mem`, `memory`, `memstats`, and role `model.ADMIN` (`internal/command/registry.go`, `RegisterAll`, line 214). `internal/command/admin_moderator_catalog_guard_test.go` source-shape guards that exact Saturn row and alias order (lines 21–32, 59–105).

[OBSERVED] `commandDefinitionFor` resolves case-insensitively across canonical names and aliases (`internal/command/handlers.go`, lines 379–390). `newCommand` maps canonical `memory` to `*memoryCommand`, not `*saturnCommand` generic fallback (lines 276–377). The explicit generic fallback allowlist excludes `memory` (`admin_moderator_catalog_guard_test.go`, lines 113–137), so a regression to placeholder behavior fails the guard.

[OBSERVED] `RegisterUserUtilitiesWithDirectAgent` includes `memory` in its unconditional concrete list, independently of direct-agent, repository, snapshot, moderation, or replica capabilities (`internal/command/dispatch_adapter.go`, lines 48–115). It wraps the definition in `legacyAdapter`, which creates the command and calls `Execute(ctx)`; command errors are logged by the adapter (`dispatch_adapter.go`, lines 12–46).

[OBSERVED] Production composition calls that registration routine at `cmd/zenbot/main.go:256`. `EngineImpl.RegisterCommand` indexes aliases lowercased and trimmed (`internal/core/engine_impl.go`, lines 730–748); `common.BuildCommand` performs the matching lookup (`internal/common/command.go`, lines 21–31).

[OBSERVED] Incoming chat dispatch requires the current prefix, uses the first whitespace-delimited token as the alias, builds the command, and authorizes it before `ExecuteContext` (`internal/listener/message/handlers.go`, `DispatchUserCommand.Handle`, lines 173–197). A denied caller receives ` you are not authorized to run: <entered-alias> command.` only when an author was resolved; no command handler executes.

[OBSERVED] Target authorization is owned by `core.EngineImpl.IsUserAuthorized`, which fails closed with nil `SecurityService` and delegates otherwise (`internal/core/engine_impl.go`, lines 776–781). Thus the target applies role authorization at dispatch, rather than relying on a command-local check.

### Concrete report and output ownership

[OBSERVED] `internal/command/memory.go` defines `memoryCommand.Execute`: context cancellation returns `(FAILED, context error)` before work; otherwise it calls `runtime.GC()`, reads `runtime.MemStats`, sends one `reply`, and returns `SUCCESSFUL` (lines 13–25).

[OBSERVED] The current metric mapping is intentionally Go-adapted rather than a claim of JVM equivalence: `Alloc` -> Used, `HeapIdle` -> Free, `HeapSys` -> Total, `Sys` -> Max (`formatMemoryReport`, lines 27–35). All are runtime process capacity/allocation counters and are divided by `1024 * 1024` for truncated MiB values.

[OBSERVED] `reply` routes through `Engine.SendChatMessage` with whisper true when any of `IsWhisper`, `Whisper`, or `Type == "whisper"` is set (`internal/command/handlers.go`, lines 70–72). `EngineImpl.SendChatMessage` addresses public replies as `@<author> <message>` and whispers as `/whisper @<author> .\n<message>` (`internal/core/engine_impl.go`, lines 430–439). The target's whisper prefix includes ` .\n`, so byte-for-byte Saturn transport parity is not available from the shared target reply helper; the command correctly follows target transport conventions.

[TEST-BACKED] `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport` (`internal/command/memory_test.go`, lines 13–61) proves all three aliases register, `memstats` accepts ignored arguments, a whisper invocation emits exactly one addressed whisper reply, the four labels/newline shape match the target formatter regex, and a pre-cancelled context emits no reply and returns `FAILED` with `context.Canceled`.

[TEST-BACKED] `go test ./internal/command` passed on this worktree.

### Agent-memory and runtime capacity boundary

[OBSERVED] Agent conversation persistence is separate: `live.PersistentMemoryStore` loads/appends exchanges through `repository.AgentMemoryRepository`, scoped by `api.Context.MemoryKey()`, bounded by configured turns and TTL (`internal/agent/live/memory.go`, lines 15–99). It is not read by the command.

[OBSERVED] Agent configuration owns memory turns/TTL and request concurrency/queue capacity (`internal/config/agent_config.go`, fields at lines 10–45; validation/defaults at lines 63–72 and 98–130). `runtime.Runtime` owns unexported `jobs`, `slots`, `admission`, and an internal `closed` flag; it exposes submission/close, not a stats snapshot (`internal/agent/runtime/runtime.go`, lines 17–40 and 42–88).

[OBSERVED] `runtime.GC`/`runtime.ReadMemStats` is the only discovered target source of process memory metrics (`internal/command/memory.go`). No application-wide memory-stat service, runtime scheduler-stat interface, or queue-stat interface was found in the inspected target paths.

[RECOMMENDED] Keep this command process-metric-only. It reveals aggregate local-process allocation rather than user/agent content. Do not add durable memory count, per-room keys, queue occupancy, request IDs, configured limits, or scheduler lifecycle status without a separately approved observability contract and authorization/data-disclosure review.

## Smallest strict-TDD vertical

[RECOMMENDED] The feature requires no new service, configuration, repository, engine interface, scheduler hook, or agent interface. Its smallest ownership path is:

```text
chat prefix + alias
  -> DispatchUserCommand.Handle (role gate)
  -> common.BuildCommand / EngineImpl enabled-command map
  -> legacyAdapter.ExecuteContext
  -> memoryCommand.Execute
  -> reply -> Engine.SendChatMessage
```

[RECOMMENDED] Implementation file map for a clean starting point:

| Responsibility | File | Change |
|---|---|---|
| Source-shaped catalog | `internal/command/registry.go` | Existing `memory` definition; do not alter aliases/role. |
| Concrete selection | `internal/command/handlers.go` | Add `case "memory": return &memoryCommand{b}` only if absent. |
| Report logic | `internal/command/memory.go` | Add `runtime.GC`, `runtime.ReadMemStats`, pure formatter, then `reply`. |
| Runtime registration | `internal/command/dispatch_adapter.go` | Add `memory` to unconditional concrete canonicals only if absent. |
| Tests | `internal/command/memory_test.go` | Add behavior/registration/whisper/cancellation tests. |
| Guard | `internal/command/admin_moderator_catalog_guard_test.go` | Existing exact catalog + non-generic guard should remain unchanged. |

[RECOMMENDED] No new interface is justified. `common.Engine` already provides the output seam; dispatch already owns `ADMIN` authorization. `runtime.MemStats` stays inside the command formatter as a standard-library value, not an application capability.

[RECOMMENDED] Exact **one-tracer RED -> GREEN order** (do not batch tests):

1. RED: write `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport` in `internal/command/memory_test.go`. It must register the normal utilities, assert all three aliases, execute `!memstats ignored arguments` as a whisper, assert one whisper-addressed reply matching all four labeled MiB lines, then assert pre-cancellation produces no reply and `(FAILED, context.Canceled)`.
2. Run `go test ./internal/command -run '^TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport$'`; on a clean pre-feature baseline it must fail because `memory` is not registered/concrete.
3. GREEN: add only the `memoryCommand`, `formatMemoryReport`, concrete switch, and unconditional registration needed by that test. Preserve `runtime.GC()` because omitting it is a source-parity change.
4. Re-run the same focused test, then `go test ./internal/command`.
5. Only after green, add a small listener-level authorization tracer if the existing generic authorization coverage cannot show `memory`'s `ADMIN` role through the real registration path; do not add agent/runtime capacity tests.

[LIMITATION] The current dirty implementation and test already pass, so this worktree cannot provide historical proof that its test was observed RED. The sequence above is the required provenance for a clean reimplementation or a changed behavior.

## Risks, QA, and explicit no-scope

### Risks

[OBSERVED] Forced GC is an intentional process-wide side effect inherited from Saturn, so concurrent allocations can make reported values non-deterministic and may briefly affect latency. Tests must validate labels/unit/shape, not exact live numbers.

[RECOMMENDED] Do not make the forced GC optional silently. Removing it may be an acceptable target performance decision, but it is not strict Saturn parity and needs an explicit product decision.

[OBSERVED] Go has no `Runtime.maxMemory()` analogue. Mapping `Sys` to the Saturn-labeled "JVM Max Memory" is a display compatibility approximation, not a memory limit. Do not describe it to operators as a hard process cap.

[OBSERVED] The report uses JVM labels despite Go metrics, and help advertises “JVM memory usage” (`internal/command/help.go`, admin command text at lines 67–78). This is source-shaped naming but semantically misleading.

[RECOMMENDED] Retain labels for parity in this vertical; changing labels/help is a separate compatibility decision.

### QA exit criteria

- `go test ./internal/command -run '^TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport$'`
- `go test ./internal/command`
- Verify `memory` is not in `allowedScopedGenericFallbacks` and `newCommand("memory", ...)` is concrete.
- Verify public and whisper routing through target `reply` semantics; do not assert Saturn's different whisper prefix byte-for-byte.
- `git diff --check`
- Review only the intended files plus this handoff in the already-dirty repository; do not reset, clean, stage, or overwrite unrelated work.

### No scope

- Whiskey/proxy.
- `mine`, `restart`, or `shutdown`.
- Raw admin SQL.
- Semantic moderation activation.
- Agent command execution, direct `l`, scheduler/queue telemetry, persistent agent-memory introspection, memory deletion/export, or any new admin disclosure policy.
