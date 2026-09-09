# ADMIN `prefix` parity QA

## Acceptance verdict

**PASS — accepted as the bounded Saturn ADMIN `prefix` parity vertical.** No prefix-scope defect was reproduced, so this QA made no production or test changes. Existing unrelated dirty work was preserved.

## Source trace

Read-only Saturn comparison:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/admin/PrefixCommandImpl.java:15-45`
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/command/impl/admin/PrefixCommandImplTest.java:12-37`

The Zenbot implementation traces that contract through:

- `internal/command/prefix.go`: concrete command; cancellation precedes parsing/mutation/reply; uses only `args(...)[0]`, trims it, returns exact usage/success wording via `reply`.
- `internal/common/prefix.go`: minimal capability boundary.
- `internal/core/prefix_state.go`: host update plus a `ManagedEngines()` point-in-time snapshot, mutating only concrete `*EngineImpl` replicas and ignoring opaque entries.
- `internal/core/engine_impl.go:47-50,667-671`: private `prefixMu` guards `GetPrefix`; state mutations lock the same field.
- `internal/command/handlers.go:396-397`: `prefix` selects `*prefixCommand`, not `*saturnCommand`.
- `internal/command/dispatch_adapter.go:56-58`: registration is capability-gated on `common.PrefixController`.
- `internal/command/admin_moderator_catalog_guard_test.go:117-121`: the generic fallback allowlist excludes `prefix` while retaining the other transitional canonicals.

Static review confirmed no prefix command persistence, raw protocol emission, agent/listener/lifecycle call, or future-replica propagation. `reply` routes with `IsWhisper || Whisper || Type == "whisper"`; the command test proves the exact whisper route.

## Behavior and test evidence

Reviewed and executed existing focused tests:

- `TestPrefixCommandIsAdminConcreteAndPropagatesTrimmedFirstArgument`: sole alias and ADMIN role, concrete handler, first argument trimmed to `$`, trailing arguments ignored, exact one whisper reply `prefix changed from * to $`.
- `TestPrefixCommandMissingOrBlankArgumentUsesCurrentPrefixExampleWithoutMutation`: exact `Example: *prefix $` usage reply and no mutation for absent/blank input.
- `TestPrefixCommandCancelledHasNoMutationOrReply`: cancelled context returns failure/error without controller call or reply.
- `TestPrefixCommandWithoutCapabilityFailsWithoutSuccessReply` and `TestPrefixIsNotRegisteredWithoutPrefixController`: direct unavailable-capability rejection and registration gate.
- `TestUpdatePrefixChangesHostAndCurrentManagedReplicas`, `TestUpdatePrefixWithoutControllerChangesOnlyHost`, and `TestUpdatePrefixUsesCurrentManagedSnapshot`: host/current concrete fan-out, host-only fallback, opaque managed-engine tolerance, and no propagation to a later-added replica.
- `TestGetPrefixAndUpdatePrefixAreRaceFree`: concurrent reads/writes; race detector passed.
- `TestAdminModeratorCatalogMatchesSaturnSource` and `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded`: source-shaped catalog and concrete/generic closure.

## Real verification results

All commands ran in `/Users/ab/workspace/go-projects/zenbot` and passed:

```text
go test -race ./internal/core ./internal/command -run 'Test(UpdatePrefixChangesHostAndCurrentManagedReplicas|UpdatePrefixWithoutControllerChangesOnlyHost|GetPrefixAndUpdatePrefixAreRaceFree|PrefixCommand.*|PrefixIsNotRegisteredWithoutPrefixController|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))$' -count=1 -v
# core: PASS (1.358s); command: PASS (3.895s)

go test -race ./internal/core ./internal/command -count=1
# core: PASS (1.416s); command: PASS (31.096s)

go test ./internal/core ./internal/command -count=1
# PASS: core 0.382s; command 8.322s

go test ./internal/core -run 'TestUpdatePrefix' -count=1 -v
# PASS: propagation, host-only, snapshot tests

go test ./internal/command -run 'TestPrefix(Command|IsNotRegisteredWithoutPrefixController)' -count=1 -v
# PASS: all listed prefix command tests

go test ./... -count=1
# PASS: all packages (repository/h2 37.265s)

go vet ./internal/command
# PASS (no output)

git diff --check
# PASS (no output)
```

Coverage measurement (`go test ./internal/core ./internal/command -count=1 -cover`): `internal/core` 50.9% statements; `internal/command` 71.3% statements. This is package-wide coverage, not a prefix-only percentage.

## Race/static review

The full core/command race gate and the focused concurrency test passed. Prefix field accesses in the target runtime path are synchronized through `GetPrefix`, `UpdatePrefix`, and `setPrefix`; the remaining `.Prefix` search result is an unrelated agent event field, not `core.EngineImpl` state.

The known out-of-scope core vet diagnostic remains and was not modified:

```text
internal/core/engine_impl.go:96:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Fixes and limitations

- **Fixes:** none; no verified prefix defect warranted a scoped TDD change.
- Live-only behavior is intentional: no persistence and no newly created replica inheritance.
- Fan-out uses the manager's safe point-in-time snapshot; concurrent add/remove may change which replicas are in that snapshot, without a claim of transactional cluster-wide propagation.
- Existing repository dirtiness includes other verticals. QA added only this handoff; no commit, push, reset, clean, or checkout was performed.
