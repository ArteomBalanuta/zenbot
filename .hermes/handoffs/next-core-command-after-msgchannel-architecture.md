# Next core command after local `msgchannel`: ADMIN `memory` / `mem` / `memstats`

## Decision

**Select the bounded ADMIN process-memory report: `memory` (aliases `mem`, `memory`, `memstats`).**

It is the smallest remaining *unblocked command vertical* after accepted local `msgchannel`/`msgroom`: no arguments are semantically consumed; normal ADMIN dispatch authorization already exists; it needs one forced runtime collection, one in-process metrics read, one addressed reply, and no repository, service, H2, session, proxy, credential, raw-SQL, lifecycle, identity, or agent surface.

**Accuracy boundary:** the command's aliases, authorization position, forced collection, four source labels, MiB truncation, addressed/whisper routing, and successful outcome are source-exact. Java heap values cannot be made byte-for-byte identical to Go `runtime.MemStats`; the existing target maps Go process counters to Saturn's JVM-labelled slots. Treat that as the explicit, already-present runtime adaptation—not evidence that Go has JVM `totalMemory`, `freeMemory`, or `maxMemory` equivalents. Do not invent a memory-cap, agent-memory, queue, scheduler, or persistence interpretation.

## Candidate inventory and ranking

`MIGRATION_PLAN.md:13-25,175-181` requires concrete command behavior and focused evidence rather than catalog/fallback presence. `local-msgchannel-parity-qa.md:3-13,37-39` accepts only the local `msgchannel` branch and retains remote delivery as incomplete. The current scoped generic-fallback guard is decisive: it permits only `mine`, `restart`, `shutdown`, `sql`, and `whiskey` (`internal/command/admin_moderator_catalog_guard_test.go:113-137`). Thus every other admin/moderator catalog row is already concrete or has an accepted bounded vertical; `memory` is the smallest concrete-but-not-yet-independently-QA-accepted command seam.

| Rank | Candidate | Decision | Source-grounded reason |
|---|---|---|---|
| 1 | **`memory` / `mem` / `memstats`** | **Select** | One source class, no semantic inputs or persistence, existing concrete handler/registration/test seam, no external capability. The only fidelity qualification is the unavoidable Java-runtime-to-Go-runtime metric adaptation described above. |
| 2 | remote `msgchannel` / `msgroom` | Defer | Saturn's different-room branch creates a generated-nick `LIST_CMD` temporary session carrying host password; local QA deliberately rejects it (`next-core-command-after-support-relay-architecture.md:62-66`; `local-msgchannel-parity-qa.md:5,12-13`). |
| 3 | `mine` | Blocked | Requires a process-owned scheduler, generated credential-bearing snapshot join, secret-file ownership, and unresolved proxy disposition (`mine-current-architecture.md:3-6,64-69,162-167`). |
| 4 | ADMIN `whiskey` | Blocked | Zenbot lacks proxy configuration and proxy-capable dialing; no direct fallback is parity (`whiskey-current-architecture.md:3-7,69-75,90-100`). |
| 5 | `restart` / `shutdown` | Blocked | Process/supervisor ownership, terminal semantics, teardown ordering, caller feedback, and operational authority remain undecided (`restart-shutdown-current-architecture.md:3-6,91-100`). |
| 6 | ADMIN `sql` | Blocked | Raw query exposure needs an approved language, disclosure, privilege, timeout/limit, audit, and error-delivery policy (`sql-current-architecture.md:3-7,51-63`). |

`l` is not a candidate: shared-runtime direct `l` integration and lifecycle-admission cancellation are already accepted (`live-agent-integration-qa.md:3-13,27-36`; `agent-command-cancellation-qa.md:3-7,20-27`). Do not reopen it or source's Dynamic SQL capability behavior while raw SQL remains excluded.

## Saturn authority and bounded behavior

### Catalog, role, and dispatch

**[OBSERVED]** `src/main/java/org/saturn/app/command/impl/admin/MemoryCommandImpl.java:17-25` declares aliases exactly `mem`, `memory`, `memstats`; it extends `UserCommandBaseImpl` without overriding the default ADMIN role. The base parses post-alias whitespace arguments, performs shared authorization before the concrete `execute`, and exposes the inbound whisper bit (`src/main/java/org/saturn/app/command/UserCommandBaseImpl.java:51-71,83-103,132-140`).

**[OBSERVED]** `MemoryCommandImpl.execute` does not inspect arguments: absent or extra arguments are accepted and ignored. It invokes `Runtime.getRuntime().gc()`, samples total/free/max bytes, derives used as `total - free`, integer-divides each by 1024², aligns the four labelled lines with thin spaces, sends once to the invoking author using `isWhisper()`, and returns `SUCCESSFUL` (`MemoryCommandImpl.java:28-56`).

Source payload before alignment is exactly:

```text
JVM Used Memory: <used> MB \\n
JVM Free Memory: <free> MB \\n
JVM Total Memory: <total> MB \\n
JVM Max Memory: <max> MB \\n
```

**[LIMITATION]** No focused Saturn test for `MemoryCommandImpl` was found; this contract is source-derived. The source's base-level authorization is not permission to add a target command-local trip list, H2 role lookup, ACL, confirmation, or audit policy.

## Existing target seams and exact target behavior

| Concern | Verified Zenbot seam | Required ownership / boundary |
|---|---|---|
| Source-shaped catalog | `internal/command/registry.go:RegisterAll` | Keep canonical `memory`, ordered aliases `mem,memory,memstats`, and `model.ADMIN`; do not add a definition. |
| Catalog source guard | `internal/command/admin_moderator_catalog_guard_test.go:saturnAdminModeratorCatalog,TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded` | The test preserves this row and proves `memory` is no longer generic. Do not modify the fallback allowlist. |
| Concrete construction | `internal/command/handlers.go:newCommand` | Already selects `*memoryCommand` for canonical `memory`; do not route via `*saturnCommand`. |
| Live registration | `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` | Already unconditionally includes `memory`, so all aliases arrive through the definition. It must not depend on agent, H2, snapshots, proxies, or a new capability. |
| Authorization | `internal/listener/message/handlers.go:DispatchUserCommand.Handle` | Existing resolved-user ADMIN gate remains before execution. No command-local auth work. |
| Collection/report | `internal/command/memory.go:memoryCommand.Execute,formatMemoryReport` | Keep `runtime.GC()` before `runtime.ReadMemStats`; emit one addressed reply and preserve inbound whisper. The current source-shaped adaptation maps `Alloc`, `HeapIdle`, `HeapSys`, `Sys` into Used/Free/Total/Max slots. |
| Current focused proof | `internal/command/memory_test.go:TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport` | Extend only if necessary to distinguish registration/role/concrete handler, cancellation, and deterministic formatter shape. Do not assert nondeterministic live values. |

**[OBSERVED] target adaptation.** `memoryCommand.Execute` returns `FAILED, ctx.Err()` before collection on a pre-cancelled direct context (`internal/command/memory.go:15-24`). Saturn has no context. This is retained target cancellation safety, not a source claim. On non-cancelled execution, `reply` follows existing addressed transport behavior, including its target whisper encoding (`internal/command/handlers.go:reply`; `internal/core/engine_impl.go:SendChatMessage`). Do not attempt a command-local transport rewrite to force Saturn's different whisper wire shape.

## Chosen behavior and data flow

```text
active authorized ADMIN: !memstats ignored arguments (whisper/public)
  -> ResolveUserMetadata
  -> DispatchUserCommand.Handle: ADMIN authorization
  -> registered legacyAdapter / canonical memory definition
  -> memoryCommand.Execute(ctx)
  -> runtime.GC()
  -> runtime.ReadMemStats(&stats)
  -> formatMemoryReport(stats)
  -> reply(author, report, inbound-whisper)
  -> SUCCESSFUL
```

A denied resolved caller must execute none of the command path and receives only the existing dispatch denial. A pre-cancelled direct command must run no collection/read/reply and return `FAILED, context.Canceled`. Extra arguments must not cause usage, error, filtering, or different metrics.

## Exact implementation/QA file map

This is primarily an **acceptance/reconciliation vertical**, because its narrowly scoped implementation already exists as dirty worktree content. Do not rewrite it merely to manufacture change.

| Path | Intended handling |
|---|---|
| `internal/command/memory.go` | Read-only unless a focused tracer identifies a real source-shape defect. Preserve forced GC, four labels/order/newlines/thin-space alignment, current explicit Go metric adaptation, one `reply`, and cancellation gate. |
| `internal/command/memory_test.go` | Add/adjust only focused deterministic evidence needed to lock the selected contract: all aliases/ADMIN/concrete construction, ignored arguments, formatter shape with fixed `runtime.MemStats`, public/whisper addressing through command fixture, and cancellation/no-reply. |
| `internal/command/handlers.go` | Regression-inspection surface only: retain `case "memory"`. |
| `internal/command/dispatch_adapter.go` | Regression-inspection surface only: retain unconditional concrete registration. |
| `internal/command/registry.go` | Regression-inspection surface only: retain source catalog row. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Regression-inspection surface only: retain source row and keep `memory` absent from generic fallback. |
| `internal/listener/message/handlers.go` and `internal/core/engine_impl.go` | Read-only shared authorization/transport constraints. |

No `internal/common/**`, `internal/core/**` production change, `internal/service/**`, `internal/repository/**`, H2/schema, config, listener-order, agent, snapshot/session, transport/proxy, or `cmd/zenbot/main.go` change belongs to this vertical.

## One-at-a-time RED → GREEN tracers

The existing dirty implementation and current focused test mean historical RED cannot honestly be recreated without deleting working code. Do **not** claim a new RED event for a test that is already green. On a clean pre-feature baseline, execute these one at a time; in this worktree, treat them as reconciliation tests unless one exposes a defect, then record its actual RED before the minimal Green change.

1. **RED — catalog/registration/construction.** Add a focused test asserting exact alias order and ADMIN metadata, all three registered aliases, and `definition.New(...)` producing `*memoryCommand`, never `*saturnCommand`.
   - Run: `go test ./internal/command -run '^TestMemoryAliasesRegisterAndConstructConcreteAdminCommand$' -count=1`.
   - Expected clean-baseline RED: absent registration or generic construction.
   - GREEN: only catalog-preserving concrete switch/registration work if genuinely absent; do not broaden registration gates.

2. **RED — deterministic source-shaped report.** Test `formatMemoryReport` with a fixed `runtime.MemStats` fixture. Assert four ordered JVM labels, exact thin-space alignment/newline literals, and integer MiB formatting according to the documented target mapping. Execute `!memstats ignored arguments` and prove arguments do not alter outcome; use shape assertions rather than live metric values.
   - Run: `go test ./internal/command -run '^TestMemoryReportUsesSourceLabelsAndIgnoresArguments$' -count=1`.
   - Expected clean-baseline RED: no concrete formatter/handler or non-source report shape.
   - GREEN: only `memory.go` formatting/collection/reply correction required by the failing expectation. Do not replace the adaptation with invented JVM semantics.

3. **RED — dispatch/authorization and routing.** Through the real registered command/listener seam, prove an authorized ADMIN public and whisper request emits exactly one addressed report with the corresponding target whisper flag; a resolved denied author performs no collection-visible command reply and receives only the dispatch denial.
   - Run: `go test ./internal/command ./internal/listener/message -run 'TestMemory(Dispatch|Authorization|Whisper)' -count=1`.
   - GREEN: only a demonstrated registration/dispatch integration defect; do not alter global listener order or authorization policy.

4. **RED — cancellation/error boundary.** Directly execute with a pre-cancelled context; assert `(FAILED, context.Canceled)` and zero sends. Preserve no-retry behavior.
   - Run: `go test ./internal/command -run '^TestMemoryPreCancelledDoesNotCollectOrReply$' -count=1`.
   - GREEN: only the pre-work context check if missing. This is target safety, explicitly not Saturn behavior.

5. **Regression gates.** Re-run local `msgchannel` and support relay proofs to show `memory` acceptance did not re-open accepted relay scope.

## Blockers, exclusions, and invariants

- Do not implement remote `msgchannel`/`msgroom`; it remains deferred pending a separately designed credentialed temporary-session architecture.
- Do not activate proxy/Whiskey, `mine`, credentials, credentialed joins, raw SQL, restart/shutdown/process controls, agent tools/routes, semantic moderation, H2/schema/repository/service work, or an identity/auth policy.
- Do not report Go `Sys` as a hard memory limit, or present any Go slot as an actual JVM metric. Keep the source labels only as the documented compatibility presentation.
- Do not add agent-memory counts, durable-memory contents, queue depth, runtime capacity, per-user metrics, new observability endpoints, or data-disclosure policy.
- Preserve normal ADMIN authorization at dispatch; do not add configured-trip matching, H2 lookups, ACLs, confirmation, password, or special whisper-only policy.
- Preserve `runtime.GC()`; removing or making it optional is a behavior change requiring an explicit decision.
- No application/test code is authorized by this architecture task. No staging, reset, clean, restore, checkout, commit, or Saturn-source change.

## Verification after any implementation/QA change

Run from `/Users/ab/workspace/go-projects/zenbot`:

```sh
gofmt -w internal/command/memory.go internal/command/memory_test.go # only if either changed

go test ./internal/command -run 'TestMemory|TestMsgChannel|TestSupportRelay' -count=1
go test ./internal/listener/message -run 'Test.*(Dispatch|Authorization)' -count=1
go test ./internal/command ./internal/listener/message -count=1
go test ./...
go vet ./...
go build ./...
git diff --check
git diff --cached --check
git status --short
```

Acceptance requires: exact aliases/ADMIN metadata; concrete registration; normal dispatch authorization; forced collection before metric sampling; documented runtime-adaptation report shape; ignored arguments; one addressed public/whisper reply; cancellation no-side-effect proof; no scope expansion; and green focused/package/full/diff gates. It must not call the metric mapping JVM-equivalent or close the wider migration.

## Evidence and baseline record

**[OBSERVED]** Every cited Zenbot and Saturn source path above was opened from the current target checkout and read-only Saturn checkout. The accepted support relay and local-msgchannel QA handoffs were also read; neither was changed. The working tree was already extensively dirty before this architecture pass.

**[TEST-BACKED]** Read-only baseline after this document was written, from `/Users/ab/workspace/go-projects/zenbot`:

```text
$ go test ./internal/command -run 'TestMemory|TestMsgChannel|TestSupportRelay' -count=1
ok  zenbot/internal/command  2.119s

$ go test ./internal/listener/message -run 'Test.*(Dispatch|Authorization)' -count=1
ok  zenbot/internal/listener/message  16.104s

$ go test ./internal/command ./internal/listener/message -count=1
ok  zenbot/internal/command           12.710s
ok  zenbot/internal/listener/message  15.489s

$ go test ./... && go vet ./... && go build ./...
# all packages passed; vet/build exited 0

$ git diff --check && git diff --cached --check
# both exited 0 with no output
```

A citation/structure check resolved all 19 cited source/handoff paths, found every required heading, and confirmed the handoff is non-empty. `git status --short -- .hermes/handoffs/next-core-command-after-msgchannel-architecture.md` reports only this untracked task artifact. This handoff is the only task-owned write; no application or test path is changed by this architecture pass.
