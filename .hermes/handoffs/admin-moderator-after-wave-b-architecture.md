# Post–Wave B rapid parity slice: live `prefix` command

## Decision and readiness

**Selected exactly one slice:** port Saturn's ADMIN `prefix` command, including its observed host-plus-*currently managed replica* prefix fan-out. Route to **@senior-developer** because the target must close a shared mutable-state race and traverse the managed-replica boundary. It is small and independent of persistence, agent moderation, snapshot operations, automove, lifecycle, and admin SQL.

**Wave B is sufficient to advance.** The accepted QA handoffs report focused PASS for S2, S3, and S4. S4 additionally reports `go test ./... -count=1` PASS. In this worktree, the available S5 activity artifacts were validated rather than assumed: their focused activity/factory gate and a current `go test ./... -count=1` both passed. That makes `prefix` the next bounded rapid command-parity vertical; it does not make unimplemented catalog placeholders acceptable.

**Known outside-scope diagnostic:** `go vet ./internal/core` currently reports `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`. Record it in the implementation result but do not repair it in this slice.

## Evidence map

### [OBSERVED] Saturn contract

- `src/main/java/org/saturn/app/command/impl/admin/PrefixCommandImpl.java:15-45` declares the sole alias `prefix`. The full approved inventory identifies this class as ADMIN; target's source-shaped catalog guard independently records `PrefixCommandImpl`, canonical `prefix`, ADMIN, and `[]string{"prefix"}` in `internal/command/admin_moderator_catalog_guard_test.go:22-26`.
- `PrefixCommandImpl.execute` obtains the first parsed argument, rejects absent/blank input through `failWithUsage("prefix $")`, snapshots `engine.prefix`, trims the new prefix, updates the invoking engine, iterates `engine.replicasMappedByChannel.values()` to update each current replica, then replies `prefix changed from <old> to <new>` and returns `SUCCESSFUL` (`PrefixCommandImpl.java:23-40`). No raw protocol payload, database access, listener change, or retry appears in that command.
- `src/test/java/org/saturn/app/command/impl/admin/PrefixCommandImplTest.java:12-37` proves the two precise source cases: missing argument returns FAILED, queues `@admin Example: *prefix $`, and leaves `*`; valid `*prefix $` returns SUCCESSFUL, changes both host and registered `lounge` replica to `$`, and queues `@admin prefix changed from * to $`.
- Saturn's fan-out is a point-in-time traversal of the current concurrent replica map (`src/main/java/org/saturn/app/facade/impl/EngineImpl.java:61,315-317`); the command does **not** persist the setting or specify behavior for replicas created after invocation. Do not invent durable configuration semantics.

### [OBSERVED] current Zenbot seams and gap

- `internal/command/registry.go:92,128-133` has a deterministic `prefix` ADMIN definition, but `internal/command/handlers.go:311-398` has no `case "prefix"`; it returns generic `saturnCommand`. The catalog guard explicitly allows this transitional generic route at `admin_moderator_catalog_guard_test.go:113-121`. `RegisterUserUtilities` likewise does not register `prefix` (`internal/command/dispatch_adapter.go:47-90`). Thus catalog presence is not live command parity.
- `internal/common/engine.go:5-57` exposes only `GetPrefix() string`; it has no safe prefix mutation capability. `internal/core/engine_impl.go:47-50,666-668` stores exported `Prefix string` and returns it without synchronization. Prefix is read by inbound dispatch (`internal/listener/message/handlers.go:171-174`), the information listener (`internal/listener/info/handlers.go:83-86`), command replies, and agent-participation suppression. A command write to the current field would race those reads.
- `internal/core/EngineImpl` owns only a private `replicaController` (`internal/core/engine_impl.go:85-88,246-265`). `ManagedReplicaController` privately owns its `ReplicaManager` (`internal/core/replica_controller.go:11-23`); `ReplicaManager.ManagedEngines()` provides a lock-protected snapshot of managed engines (`internal/core/replica_manager.go:83-95`). This is the existing safe seam for current-replica fan-out, without exposing the manager to command code.
- Main installs the managed controller after master construction (`cmd/zenbot/main.go:254-275`). Target replica construction clones the config prefix (`internal/factory/replica_factory.go:17-22`). This slice must not change future-replica initialization because the Saturn command only proves propagation to replicas present at execution time.
- The listener chain currently calls `GetPrefix` during dispatch and must not be reordered. The join listener/agent automation (`internal/listener/user_joined_listener.go`, `internal/factory/engine_factory.go:105-130`) is unrelated.

## Required implementation contract [RECOMMENDED]

### Exact file map

| Path | Change ownership |
|---|---|
| `internal/common/prefix.go` **new** | Define the minimal command-facing mutation capability only: `type PrefixController interface { UpdatePrefix(string) (previous string, err error) }`. The method applies a previously validated, trimmed value to the host and current managed replicas. It has no context because it performs no blocking I/O. |
| `internal/core/engine_impl.go` | Make prefix access race-safe: add a private `prefixMu sync.RWMutex`; make `GetPrefix` take `RLock`; implement `UpdatePrefix`. Preserve construction compatibility for current `Prefix:` literals, but do not allow command code to assign the field. If changing the field's visibility is necessary, update only its direct construction/test users shown by source search. |
| `internal/core/prefix_state.go` **new** | Keep propagation logic separate from the broad engine file. `UpdatePrefix` must atomically serialize competing updates at the host boundary, snapshot the managed engines through the controller/manager, apply the exact supplied value to host and each `*EngineImpl` current replica, and return the old host prefix. Ignore opaque non-`*EngineImpl` managed engines rather than type-asserting a foreign implementation into a failure or mutating it unsafely. No persistence and no replica creation/stopping. |
| `internal/core/prefix_state_test.go` **new** | TDD coverage for host mutation, managed replica fan-out, no-controller host-only behavior, immutable manager-snapshot behavior, and concurrent `GetPrefix`/`UpdatePrefix` race coverage. |
| `internal/command/prefix.go` **new** | Concrete context-aware `prefixCommand`. Check `ctx.Err()` before usage, mutation, or reply; parse only first command argument using the established `commandBase` parser; trim it; require nonblank; require `common.PrefixController`; invoke `UpdatePrefix`; reply with Saturn's exact success/usage strings through `reply` so whisper routing is preserved. |
| `internal/command/prefix_test.go` **new** | Command-level TDD tests listed below using a recording prefix-controller stub and target command definitions. |
| `internal/command/handlers.go` | Add only `case "prefix": return &prefixCommand{b}`. |
| `internal/command/dispatch_adapter.go` | Conditionally append canonical `prefix` only when `e` implements `common.PrefixController`; never expose the alias with a generic handler. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Remove only `"prefix"` from `allowedScopedGenericFallbacks`; retain every other transition row unchanged. |

No factory, config, repository, service, listener, agent, or Saturn source edit is required by the observed contract.

### Interfaces and data flow

```text
Inbound chat "<old>prefix <value>"
  -> message dispatch reads host.GetPrefix() under RLock
  -> authorization resolves ADMIN before command execution (existing dispatch)
  -> concrete prefixCommand.Execute(ctx)
      -> first parsed argument; strings.TrimSpace
      -> common.PrefixController.UpdatePrefix(newValue)
          -> host prefix serialized + old value captured
          -> ManagedReplicaController's current manager snapshot
          -> host + each current *core.EngineImpl prefix setter under lock
      -> reply issuer (same whisper/public mode):
         "prefix changed from <old> to <new>"
```

Failure semantics:

1. Cancelled context: return `FAILED`/context error; no mutation and no reply.
2. Missing or whitespace-only first argument: reply `Example: <current-prefix>prefix $`, return `FAILED`, and do not mutate. Additional arguments are ignored, matching Saturn's first-argument behavior.
3. Engine without `PrefixController`: do not register `prefix`. A direct handler invocation must return `FAILED` with an explicit unavailable error and no success reply.
4. Successful mutation: exactly one reply, `SUCCESSFUL`; no raw socket message, SQL, audit-format rewrite, config write, replica add/remove, or agent action.
5. Current managed-replica snapshot may race a concurrent lifecycle removal/addition. It must never panic or data-race; apply to engines present in the snapshot. This matches Saturn's point-in-time map traversal rather than claiming a transactional cluster-wide state machine.

## TDD execution plan

Follow one RED → GREEN → REFACTOR tracer at a time. Do not write the production command/implementation before observing its corresponding focused failure.

1. **RED core state:** `TestUpdatePrefixChangesHostAndCurrentManagedReplicas` expects old value `*`, host `$`, and a registered managed replica `$`; first failure should be absent `UpdatePrefix`/unsafe behavior. **GREEN:** minimal synchronized core implementation.
2. **RED core edge:** `TestUpdatePrefixWithoutControllerChangesOnlyHost` and `TestGetPrefixAndUpdatePrefixAreRaceFree` establish no-controller behavior and concurrent safe reads/writes. **GREEN:** lock/snapshot only; no lifecycle or factory expansion.
3. **RED command success:** `TestPrefixCommandIsAdminConcreteAndPropagatesTrimmedFirstArgument` expects alias/role, controller call with `$` from `" $ ignored"`, and source-shaped issuer reply. **GREEN:** concrete handler plus one handler switch case.
4. **RED command rejection:** `TestPrefixCommandMissingOrBlankArgumentUsesCurrentPrefixExampleWithoutMutation`, `TestPrefixCommandCancelledHasNoMutationOrReply`, and `TestPrefixIsNotRegisteredWithoutPrefixController`. **GREEN:** validation/capability gate only.
5. **RED catalog closure:** extend the existing generic-fallback guard expectation so `prefix` must be concrete. **GREEN:** remove only its allowlist entry.

Use real `core.EngineImpl` plus `ReplicaManager`/`ManagedReplicaController` for core fan-out; mocks are acceptable only for the command capability boundary. Do not test a private field assignment as the behavioral assertion—assert `GetPrefix()` and recorded outbound chat instead.

## Required verification

Run in `/Users/ab/workspace/go-projects/zenbot`; preserve the dirty baseline and create no commit/push.

```bash
# Each named RED test first, then the same command after GREEN.
go test ./internal/core -run 'Test(UpdatePrefixChangesHostAndCurrentManagedReplicas|UpdatePrefixWithoutControllerChangesOnlyHost|GetPrefixAndUpdatePrefixAreRaceFree)$' -count=1 -v
go test -race ./internal/core -run 'Test(UpdatePrefixChangesHostAndCurrentManagedReplicas|UpdatePrefixWithoutControllerChangesOnlyHost|GetPrefixAndUpdatePrefixAreRaceFree)$' -count=1

go test ./internal/command -run 'Test(PrefixCommand|PrefixIsNotRegisteredWithoutPrefixController|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))$' -count=1 -v
go test -race ./internal/command -run 'Test(PrefixCommand|PrefixIsNotRegisteredWithoutPrefixController|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))$' -count=1

go test ./internal/command ./internal/core -count=1
go test ./... -count=1
go vet ./internal/command
git diff --check
git status --short
```

`go vet ./internal/core` may be recorded separately only as the already-known copylock warning above; it is **not** a completion blocker for this slice and must not be “fixed” incidentally. If any other vet error appears, stop and report it.

## Explicit exclusions

- `automove`, its static source/destination state, USER-trip whitelist query, replica creation/stopping, and join-listener movement are a separate moderator/lifecycle slice.
- Durable prefix config/schema persistence, process restart retention, and propagation to replicas created after the command are not proven by the Saturn command and are excluded.
- Replica command response parity, snapshot operations, remote room moves, lifecycle commands, `sql`, Whiskey, catalog-wide cleanup, full audit redesign, S5 activity changes, and every accepted Wave B manual moderation file are excluded.
- Do not alter agent moderation protections, command authorization ordering, listener-chain order, transport JSON handling, existing handoffs/plans/configuration, or Saturn.

## Risk and acceptance

| Risk | Control |
|---|---|
| New command write races dispatcher/agent reads of `Prefix`. | Private RW locking plus `-race` core and command gates. |
| Replica fan-out reaches opaque/temp sessions or changes lifecycle. | Use only current `ReplicaManager.ManagedEngines()`; mutate only concrete managed `*EngineImpl`; no Start/Stop/Add/Remove. |
| Conditional registration accidentally exposes a generic placeholder. | Capability-gate registration and remove only `prefix` from the source catalog guard allowlist. |
| Output/parse drift. | Direct Saturn unit-test fixtures for missing and valid arguments; preserve first-argument-only and `reply` whisper behavior. |
| Dirty concurrent work is overwritten. | Touch only the file map above; stage nothing, commit nothing, and compare final `git status --short` to baseline. |

**Acceptance:** all focused/race tests and `go test ./...` pass; `prefix` is concrete and capability-gated; host/current managed replica behavior matches the two Saturn tests; no unrelated paths changed; only the stated out-of-scope core copylock vet warning remains.
