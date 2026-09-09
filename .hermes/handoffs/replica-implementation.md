# Replica command parity implementation

## Scope and result

Implemented only the concrete ADMIN `replica`, `replicaoff`, and `replicastatus` command slice over the existing `command.ReplicaController` lifecycle boundary. The capability gate in `RegisterUserUtilitiesWithDirectAgent` is retained unchanged. No core manager/controller/factory/main lifecycle code changed.

## Source authority

* Saturn `ReplicaCommandImpl.java:18-73`: aliases `replica`, `bot`, `agent`; first argument; exact add success and validation replies; author-directed whisper behavior.
* Saturn `ReplicaOffCommandImpl.java:15-49`: aliases `replicaoff`, `offline`, `botoff`, `agentoff`; exact validation/success replies; success only after stop.
* Saturn `ReplicaStatusCommandImpl.java:16-32`: aliases `replicastatus`, `status`; exact `Host room:... \\nServing channels: ...` rendering.
* Zenbot architecture handoff `.hermes/handoffs/replica-current-architecture.md:59-71,73-91`: retain `ReplicaManager` ownership/locks, managed controller lifecycle ordering, authorization-before-execution, and the controller capability gate.

## TDD record

Commands ran in `/Users/ab/workspace/go-projects/zenbot`.

1. **Registration tracer**
   * Initial gate-only form of `TestReplicaRegistersOnlyWithReplicaController` passed because the existing capability gate already exposed aliases only for engines implementing `ReplicaController`.
   * The same required test was tightened to also assert registered replica routes were concrete rather than `*saturnCommand`.
   * RED: `go test ./internal/command -run '^TestReplicaRegistersOnlyWithReplicaController$' -count=1` failed for every replica alias with `remains a generic fallback`.
   * GREEN: added concrete route selection in `newCommand`; the same command passed.

2. **Add aliases / first argument / exact success**
   * RED: `go test ./internal/command -run '^TestReplicaAliasesAndFirstArgumentUseSaturnContract$' -count=1` failed: reply was ` replicas: 1`, not Saturn's exact success reply.
   * GREEN: concrete add command parses only the first post-command token, retains whisper delivery, calls `AddReplica`, snapshots the count, and emits the exact Saturn reply. Same command passed.

3. **Add validation replies**
   * RED: `go test ./internal/command -run '^TestReplicaSaturnFailuresReplyAndFail$' -count=1` failed for host and duplicate because both were accepted.
   * GREEN: host and duplicate preflight replies were added; missing usage reply was already introduced by the prior concrete add tracer. Same command passed.

4. **Off aliases / failures / reply-after-remove**
   * RED: `go test ./internal/command -run '^TestReplicaOffAliasesRepliesAndFailures$' -count=1` failed: no success reply, missing returned a bare error, and host/absent were accepted.
   * GREEN: concrete off command now validates usage/host/absence from a snapshot, calls `RemoveReplica`, and sends its exact success reply only after no removal error. Same command passed.

5. **Status rendering**
   * RED: `go test ./internal/command -run '^TestReplicaStatusExactReplyForNoneAndSortedChannels$' -count=1` failed with legacy ` channel: ...` replies.
   * GREEN: concrete status snapshots once, sorts a copied channel list, renders `none` for empty, and emits the exact target string. Same command passed.

6. **Concrete and lifecycle-error guards**
   * `TestConcreteReplicaCommandsAreNotGenericFallbacks` and `TestReplicaLifecycleFailureDoesNotClaimSuccess` pass. The latter verifies add/remove lifecycle errors return `FAILED`, propagate the error for adapter logging, and send no success reply.
   * The bounded generic fallback allowlist removes exactly `replica`, `replicaoff`, and `replicastatus`; generic switch cases for exactly those canonicals were removed.

## Files changed

* `internal/command/handlers.go` — concrete route selection for three canonicals.
* `internal/command/registry.go` — concrete command types/execution; removed only the three generic switch cases.
* `internal/command/replica_test.go` — capability, aliases, exact replies, failures, sorting, concrete-type, and lifecycle-error tests.
* `internal/command/admin_moderator_catalog_guard_test.go` — removes exactly three migrated canonicals from the generic fallback allowlist.
* `.hermes/handoffs/replica-implementation.md` — this record.

## Limitations

* `model.ChatMessage.GetArguments()` uses `strings.Fields`, so an inbound text consisting of the command plus only whitespace is indistinguishable from a missing argument. It therefore produces the Saturn usage reply, while Saturn's direct list API can distinguish an empty first argument and use its host/blank reply. Normal whitespace-delimited channels, host channels, and duplicates are covered.
* Manager/controller ordering and contextual lifecycle errors remain deliberately untouched. Preflight duplicate/absence checks are advisory snapshots; managed controller operations remain authoritative under races.
