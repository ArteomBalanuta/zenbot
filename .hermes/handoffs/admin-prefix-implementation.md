# ADMIN `prefix` implementation handoff

## Scope and source trace

- Target repository: `/Users/ab/workspace/go-projects/zenbot`, branch `migration/saturn-zenbot-parity`.
- Read-only Saturn source re-read:
  - `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/admin/PrefixCommandImpl.java:15-45`
  - `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/command/impl/admin/PrefixCommandImplTest.java:12-37`
- The implemented behavior follows Saturn exactly: sole `prefix` alias, first argument only, blank/missing usage `Example: <current>prefix $`, trim before application, host plus current replica fan-out, and `prefix changed from <old> to <new>` reply.
- Zenbot seams re-read: `internal/common/engine.go`, `internal/core/engine_impl.go`, `internal/core/replica_controller.go`, `internal/core/replica_manager.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/command/admin_moderator_catalog_guard_test.go`, command base/parser and related command tests.

## Implemented paths

- Created `internal/common/prefix.go`: minimal `common.PrefixController` with `UpdatePrefix(string) (previous string, err error)`.
- Created `internal/core/prefix_state.go`: serialized host mutation; manager `ManagedEngines()` point-in-time snapshot; applies only to `*EngineImpl` replicas; ignores opaque managed engines; no persistence/lifecycle work.
- Modified `internal/core/engine_impl.go`: added private `prefixMu sync.RWMutex`; `GetPrefix` now uses `RLock`; retained existing construction-compatible exported `Prefix` field.
- Created `internal/core/prefix_state_test.go`: host/current concrete replica propagation, no-controller host-only behavior, future-replica non-propagation, opaque managed-engine tolerance, concurrent `GetPrefix`/`UpdatePrefix` coverage.
- Created `internal/command/prefix.go`: concrete cancellation-aware handler with first-argument trimming, exact usage/success text, capability failure without reply, and normal `reply` whisper/public routing.
- Created `internal/command/prefix_test.go`: concrete ADMIN/alias path, trimmed first arg with additional args ignored, missing/blank usage, cancellation, absent-capability direct invocation, and conditional registration.
- Modified `internal/command/handlers.go`: `case "prefix"` now returns `*prefixCommand`.
- Modified `internal/command/dispatch_adapter.go`: registers canonical `prefix` only for `common.PrefixController` engines.
- Modified `internal/command/admin_moderator_catalog_guard_test.go`: removed only `prefix` from the generic fallback allowlist.

## Strict TDD evidence

1. **Core propagation RED:** `go test ./internal/core -run '^TestUpdatePrefixChangesHostAndCurrentManagedReplicas$' -count=1 -v` failed to compile with `host.UpdatePrefix undefined`.
   **GREEN:** the same command passed.
2. **Command success RED:** `go test ./internal/command -run '^TestPrefixCommandIsAdminConcreteAndPropagatesTrimmedFirstArgument$' -count=1 -v` failed: `prefix must be a concrete command`.
   **GREEN:** the same command passed after the concrete handler/switch case.
3. **Capability-registration RED:** focused rejection/registration command tests failed only with `prefix not registered with PrefixController`.
   **GREEN:** the same focused command test set passed after capability-gated registration.
4. Edge/snapshot/race core tests were added after the propagation tracer was green and passed against that minimal synchronized implementation; command cancellation/usage/direct-capability tests passed against the concrete handler.

## Final verification

Passed:

```text
go test -race ./internal/core -run 'Test(UpdatePrefixChangesHostAndCurrentManagedReplicas|UpdatePrefixWithoutControllerChangesOnlyHost|GetPrefixAndUpdatePrefixAreRaceFree)$' -count=1
go test -race ./internal/command -run 'Test(PrefixCommand.*|PrefixIsNotRegisteredWithoutPrefixController|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))$' -count=1
go test -race ./internal/core ./internal/command -run 'Test(UpdatePrefixChangesHostAndCurrentManagedReplicas|UpdatePrefixWithoutControllerChangesOnlyHost|GetPrefixAndUpdatePrefixAreRaceFree|PrefixCommand.*|PrefixIsNotRegisteredWithoutPrefixController|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))$' -count=1
go test ./internal/core ./internal/command -count=1
go test ./... -count=1
go vet ./internal/command
git diff --check
```

All passed. `go vet ./internal/core` still reports the pre-existing out-of-scope warning exactly as anticipated:

```text
internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Limitations/exclusions preserved

- No config/repository/service/factory/listener/agent/lifecycle/S2/S3/S4 changes were made for this slice.
- Prefix updates are live-only: no persistence and no automatic propagation to replicas created after the command.
- Fan-out is a safe point-in-time manager snapshot; concurrent replica add/remove can legitimately include only engines present in that snapshot.
- No raw protocol, listener reorder, or agent operation is performed.
- Existing unrelated dirty work was preserved; no commit, push, reset, clean, or checkout was used.
