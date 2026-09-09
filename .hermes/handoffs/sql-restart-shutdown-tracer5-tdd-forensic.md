# SQL / restart / shutdown tracer-5 strict-TDD forensic

**Verdict:** tracer 5 is **not** a legitimate regression-only test. It exposes a tracer-4 scope breach. The current tracer-4 GREEN contains the lifecycle command execution semantics that tracer 5 was supposed to drive, so the first inbound lifecycle test passing immediately is expected and is not strict-TDD evidence.

**Repository inspected:** `/Users/ab/workspace/go-projects/zenbot`, branch `migration/saturn-zenbot-parity`, `HEAD f9079ca`, dirty worktree. No source/test/staging/reset/restore/commit operation was performed by this review. This report is the only file created.

## Evidence

### What tracer 4 independently proves

`internal/command/restart_shutdown_test.go:25` exercises only capability-gated registration:

- an engine without `HostLifecycleController` has no lifecycle aliases;
- an engine with a recording controller has `restart`, `reload`, `re`, `exit`, `quit`, and `shutdown`;
- every alias retains `ADMIN` metadata and resolves to a concrete `*restartCommand` or `*shutdownCommand`.

The existing catalog guard in `internal/command/admin_moderator_catalog_guard_test.go:117-129` also confirms `restart` and `shutdown` are no longer allowed generic fallbacks; only `mine`, `sql`, and `whiskey` remain. `RegisterUserUtilitiesWithDirectAgent` capability-gates the two canonicals at `internal/command/dispatch_adapter.go:87-89`, and `newCommand` routes them at `internal/command/handlers.go:381-384`. Those are the independently evidenced tracer-4 registration/route facts.

### What tracer 4 improperly implemented

The untracked `internal/command/restart_shutdown.go` is not a registration-only shell:

- `restartCommand.Execute` checks `ctx.Err()` and returns `FAILED` without a controller call when cancelled (lines 26-29);
- otherwise it calls `controller.RequestRestart(ctx)` and returns `SUCCESSFUL, nil` (lines 30-33);
- `shutdownCommand.Execute` provides the corresponding cancellation and `RequestShutdown` behavior (lines 41-48);
- neither handler parses arguments or calls `reply`/`SendChatMessage`.

Those are exactly tracer 5's intended observable behaviors: controller invocation, no reply, ignored arguments, and cancellation with no side effect. They are not required to determine catalog availability, aliases, role metadata, or concrete routing. A concrete type must satisfy `common.SaturnCommand.Execute`, but that interface requirement does not require tracer 4 to implement the real lifecycle action.

The current listener path makes the relationship direct: `DispatchUserCommand.Handle` resolves/authorizes then invokes a context-capable registered command (`internal/listener/message/handlers.go:180-204`), and `legacyAdapter.ExecuteContext` forwards that context to the definition (`internal/command/dispatch_adapter.go:26-37`). Thus a listener tracer observing an authorized alias necessarily executes the already-written `RequestRestart`/`RequestShutdown` call. The implementation record's immediate-green result for `^TestDispatchUserCommandLifecycle` is therefore explained by pre-existing tracer-5 behavior, not by duplication of a behavior necessarily established by tracer 4.

The attempted listener test is currently absent: `go test ./internal/listener/message -run '^TestDispatchUserCommandLifecycle' -count=1` returned `ok ... [no tests to run]`. Its prior immediate green is recorded in the implementation handoff.

## Dirty-worktree-safe recovery boundary

Do not use `git reset`, `git restore`, `git clean`, staging, or broad file deletion. Preserve the independently evidenced capability registration work:

1. Retain `internal/common/host_lifecycle.go`, the corrected controller/supervisor composition artifacts, and their focused tests. They are prerequisite tracer work, not tracer-5 command behavior.
2. Retain `lifecycleControllerProvider` / `lifecycleController`, the capability check in `dispatch_adapter.go`, the `restart`/`shutdown` `newCommand` cases, `restart_shutdown_test.go`, and the catalog-guard removal of only `restart`/`shutdown`.
3. In **only** `internal/command/restart_shutdown.go`, manually remove the lifecycle behavior: the stored `common.HostLifecycleController`, both `ctx.Err()` branches, and both `RequestRestart` / `RequestShutdown` calls. Keep minimal concrete types solely so registration can resolve to non-generic types and satisfy `common.SaturnCommand`; their temporary `Execute` methods must not invoke a controller, send a reply, inspect arguments, or establish cancellation semantics. Use an explicit internal unimplemented error/status rather than source-like success/no-reply, so the retained tracer-4 shell cannot be mistaken for lifecycle behavior.
4. Manually adjust only the two `newCommand` constructors if the removed controller fields require it. Do not touch unrelated dirty hunks in the shared tracked files.
5. Run the retained tracer-4 command test and catalog guard. The capability test must remain green after this reduction.

The boundary deliberately preserves only registration/route evidence. It does **not** retain a no-op-success handler, because that would itself pre-establish part of tracer 5's source-visible no-reply/success behavior.

## Safe strict-TDD resume

1. First verify the retained prerequisite controller/supervisor tracers are green, including the post-dispatch barrier and race checks.
2. Apply the narrow manual reduction above and run:

```text
go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
```

3. Create a new tracer-5 listener/direct-execution test **before** reintroducing any lifecycle handler logic. It must dispatch every lifecycle alias through `listener.NewUserChatListener` with an active authorized caller and assert exactly one corresponding controller request, zero chats, and ignored extra arguments; it must also prove an unauthorized active caller gets existing denial with zero controller calls and a pre-cancelled direct execution makes no call/reply.
4. Observe a meaningful RED caused by the preserved non-invoking shell (the authorized controller-call assertion must fail). Then add only the parse-free controller-invoking/cancellation handlers to make it GREEN. Do not add tracer-6 catch-all failure conversion or no-self-deadlock scheduling changes in this cycle.
5. Only after that GREEN may tracer 6 begin. Tracer 6 remains the distinct behavior for swallowing/logging controller operational failures as successful/no-reply and proving controller work waits until dispatch release.

## Checks run in the inspected state

```text
go test ./internal/command -run 'Test(Restart|Shutdown|AdminModeratorCatalog)' -count=1
# ok zenbot/internal/command

go test ./internal/listener/message -run '^TestDispatchUserCommandLifecycle' -count=1
# ok zenbot/internal/listener/message [no tests to run]

go test -run '^$' ./...
# exit 0; all packages compile with tests skipped

go test ./internal/core -run TestHostLifecycle -count=1
# ok zenbot/internal/core

go test -race ./internal/core -run TestHostLifecycle -count=1
# ok zenbot/internal/core

go test ./cmd/zenbot -run 'TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
# ok zenbot/cmd/zenbot

go test -race ./cmd/zenbot -run 'TestHostSupervisor|TestMainProductionHostLifecycle' -count=1
# ok zenbot/cmd/zenbot

git diff --check
# exit 0
```

These checks establish the current dirty baseline only; they do not validate tracer 5 as a strict-TDD cycle.
