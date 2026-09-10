# Nuke snapshot composition — corrective architecture

## Decision

Implement the missing composition prerequisite before the `nuke` command. It is a high-risk, senior-owned boundary because it gives a moderator command access to a temporary room session. It must reuse `snapshot.RoomSnapshotCoordinator` and `snapshot.CoordinatedSessionFactory`; no command may construct sessions, transports, or sockets.

This is a prerequisite specification only. It does not activate `nuke`, `resurrect`, `mine`, automove, agent moderation, public agent tools, or any generic catalog fallback.

## Observed seam and gap

- `internal/listener/snapshot/coordinator.go` owns `RoomSnapshotRequest` validation, workflow state, payload parsing, terminal cleanup, failure/cancel/timeout behavior, and operation execution.
- `internal/listener/snapshot/session_factory.go` owns temporary context registration and cleanup through `TemporarySessionRegistry`, and forwards transport failure/close to the coordinator.
- `factory.EngineOptions` already declares `SnapshotCoordinator` and `SessionRegistry`, but `factory.NewEngineWithOptions` does not retain them on the command-serving engine.
- `cmd/zenbot/main.go` creates the master engine and replica controller but currently creates no command-facing snapshot composition.
- `internal/command/handlers.go` constructs catalog commands solely from `common.Engine` and message data. `internal/command/dispatch_adapter.go` deliberately registers only concrete capability-gated commands.

Therefore the missing boundary is composition, not a new temporary-session system.

## Exact boundary

Add a narrow capability in `internal/common`, named for room-snapshot submission rather than the nuke behavior:

```text
type RoomSnapshotSubmitter interface {
    SubmitRoomSnapshot(snapshot.RoomSnapshotRequest) error
}
```

`*core.EngineImpl` is the only production implementation. It holds a private `snapshotCoordinator` set once at construction/composition. Its method:

- returns an explicit configuration error if no coordinator was installed;
- delegates unchanged to `RoomSnapshotCoordinator.Submit`;
- neither creates a session nor parses a snapshot nor sends raw protocol payloads;
- takes no command context because the existing coordinator API has no context; command code must reject a pre-cancelled context before submission. This is a documented legacy-coordinator limit, not end-to-end cancellation parity.

The coordinator is injected through `factory.EngineOptions.SnapshotCoordinator` and installed by `NewEngineWithOptions` on the engine. Keep the field private to core; callers use only the capability type assertion.

## Composition owner

`cmd/zenbot/main.go` owns production composition:

1. Create `snapshot.TemporarySessionRegistry`.
2. Create a `snapshot.RoomSnapshotCoordinator` with a `factory.NewCoordinatedSessionFactory` configured from the master transport config and registry.
3. Bind the factory to the coordinator through the existing constructor path.
4. Provide a reply sink that routes coordinator operation replies to the originating command author with its inbound whisper mode. The command request must carry enough trusted delivery information for that sink; no model/agent path is involved.
5. Pass the coordinator and registry in `factory.EngineOptions` when constructing the master engine.

The coordinator/session factory retains ownership of inbound temporary payload routing, transport error/close routing, session close, and registry removal. The master replica manager must never contain these temporary sessions.

## Nuke ownership after the prerequisite

A later `nuke` command creates a fully populated `snapshot.RoomSnapshotRequest` and calls only `RoomSnapshotSubmitter.SubmitRoomSnapshot`.

The later `snapshot.NukeRoomOperation` owns parsed snapshot users and calls `RoomSnapshotContext.SendRaw`. It emits source-shaped normalized-ban actions serially, waits 200ms between them, then emits the target-established room-lock action only if all prior sends succeeded. The command owns parsing/usage/status; the coordinator owns session lifecycle; the operation owns raw actions.

## Registration gate

`dispatch_adapter.go` may register canonical `nuke` only when the command-serving engine implements `common.RoomSnapshotSubmitter` and the concrete nuke command/operation is present. No coordinator capability means no nuke aliases are registered. `handlers.go` must not route nuke to a generic fallback once it is registered.

## Exact task-owned file map

Prerequisite implementation:

- `internal/common/room_snapshot.go` — capability interface.
- `internal/core/engine_impl.go` or a focused `internal/core/room_snapshot.go` — private coordinator field, install method/constructor assignment, forwarding method.
- `internal/factory/engine_factory.go` — install `EngineOptions.SnapshotCoordinator` on the constructed engine.
- `cmd/zenbot/main.go` — construct/bind registry, coordinated factory, and coordinator; pass options to master construction.
- focused core/factory/main composition tests as needed.

Subsequent nuke command tracer:

- `internal/listener/snapshot/nuke_operation.go` and test.
- `internal/command/nuke.go` and test.
- `internal/command/handlers.go` and `internal/command/dispatch_adapter.go` plus registration/catalog tests.

## First TDD tracer

RED first, before any production edit:

```sh
go test ./internal/core -run '^TestEngineSubmitRoomSnapshotRequiresInstalledCoordinator$' -count=1
```

The test must construct an `EngineImpl` with no installed coordinator and assert the desired capability/method is absent at compile time. Add the minimum interface/method and verify it then returns the explicit configuration error. Next, use a recording/fake coordinator seam or a real coordinator factory fixture to prove a valid request is forwarded exactly once and that no session/transport is created by the engine itself.

Only after the prerequisite GREEN tests pass may the next RED tracer add `nuke` command/operation behavior.

## Verification

At prerequisite acceptance, run the focused core/factory/main composition tests, `go test ./internal/listener/snapshot ./internal/core ./internal/factory ./cmd/zenbot -count=1`, then `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check`.

## Exclusions

No generic command activation; no raw moderation payload construction in `command` or `core` forwarding code; no new session lifecycle; no agent/public-tool integration; no resurrect, mine, automove, or semantic activation; no changes to Saturn, plans, or frozen audit.
