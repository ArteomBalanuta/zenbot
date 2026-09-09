# Nuke / resurrect: current architecture handoff

## Delivery shape
Implement **two sequential concrete slices**: (1) `nuke`, then (2) three-argument `resurrect`. This is safer because both consume the same temporary-snapshot substrate, while nuke has no destination-presence decision. Do not register either command until its full concrete path is wired and tested.

## Target file map and boundaries

| File | Responsibility / change |
|---|---|
| `internal/command/handlers.go` | Replace generic fallback behavior with concrete Saturn commands. Keep command parsing/replies here; add a narrow snapshot-submit dependency to the concrete command constructor/definition, not coordinator/session mechanics. |
| `internal/command/dispatch_adapter.go` | Add only completed concrete definitions to `canonicals`, behind explicit capability checks. Do not expose catalog placeholders. |
| `internal/listener/snapshot/coordinator.go` | Reuse unchanged public seam: `RoomSnapshotRequest`, `RoomSnapshotOperation.Apply(RoomSnapshotContext, Snapshot)`, `SessionFactory`, and coordinator lifecycle/outcomes. The composition root must construct and provide the coordinator/session factory. |
| `internal/listener/snapshot/snapshot.go` | Reuse its parsed online-set `Snapshot`; operations must use that parsed set rather than independently parsing raw payloads. |
| `internal/command/handlers_test.go` | RED/GREEN command behavior: parsing, usage, normalized names, status, snapshot submission, and raw payload assertions. |
| `internal/command/dispatch_adapter_test.go` | Registration is absent without required capabilities and present only for the completed concrete slice. |
| `internal/listener/snapshot/coordinator_test.go` | Keep/add lifecycle coverage for temporary sessions: success, operation failure, cancel, timeout, transport error, and transport close all remove active workflow and close/flush the session. |

The only command-to-snapshot contract should be submission of a fully-populated `snapshot.RoomSnapshotRequest`. The operation owns snapshot-derived decisions and raw actions through `RoomSnapshotContext.SendRaw`; the coordinator owns session creation, routing, terminal cleanup, reply/outcome delivery, and no further command semantics.

## Slice 1 — `nuke`

- Accept exactly one room argument after removing a leading `@`; empty/missing input replies exactly `\\n Example: <prefix>nuke hotlinks` and returns `FAILED`.
- Submit a temporary snapshot workflow for the target room. On successful submit, return `SUCCESSFUL`; do not report success before submission succeeds.
- The operation iterates every nick in the snapshot online set, normalizes each nick, and emits the existing ban raw-message shape once per normalized nick. Only after all ban sends succeed, emit the existing lock raw-message shape for the target room. Preserve Saturn's 200 ms source delay between bans; do not lock early.
- Raw payloads are protocol messages, marshalled once and sent via `RoomSnapshotContext.SendRaw`; tests must compare decoded JSON/object fields, not substring text. The expected action order is every `{cmd:"ban", nick:<normalized snapshot nick>}` followed by `{cmd:"lock", channel:<target room>}` (use the target protocol's established field names if its concrete raw structs differ; do not invent a new envelope).

## Slice 2 — three-argument `resurrect`

- Register aliases `move`, `recover`, `heal`, and `resurrect`; role is `MODERATOR`.
- Implement only `<nick> <from> <to>`. Normalize the target nick before all comparisons/actions.
- If the locally serving engine is present, issue the normal local kick/move to destination. Otherwise, submit a temporary snapshot to `<from>`; the operation verifies the normalized target is present in the parsed online set, then emits exactly one raw kick/move payload `{cmd:"kick", nick:<normalized nick>, channel:<to>}` through `SendRaw`. An absent target must not emit a kick.
- **Exclude no-argument “last moved” resurrect behavior.** It is a separate scope: no fallback, state store, history lookup, or registration implication in these slices.

## Temporary-session lifecycle

The composition-root session factory must register each temporary connection/session with the snapshot coordinator so its inbound payload, transport error, and close callbacks route by temporary session ID. Every terminal path—successful snapshot, operation failure, explicit cancellation, timeout, transport error, and transport close—must flush/close and unregister the temporary session. No temporary session may be reused as the local serving engine, and late payloads after terminal cleanup must be ignored.

## Registration gates

Keep the current `dispatch_adapter.go` rule: concrete paths only. `nuke` requires the snapshot workflow capability plus the moderation/raw-action capability it needs. Three-argument `resurrect` additionally requires the local-engine move capability or the temporary snapshot/raw-kick path; if the required concrete dependency is missing, do not register it. Do not widen registration merely because Saturn has a catalog definition.

## TDD execution

Run RED first for each slice:

```sh
go test ./internal/command ./internal/listener/snapshot -run 'Test.*(Nuke|Resurrect|RoomSnapshot|Register)'
```

Implement the smallest path to GREEN, then run:

```sh
go test ./internal/command ./internal/listener/snapshot
go test ./...
go vet ./...
```

## Scope exclusions and routing

Out of scope: no-arg resurrect/last-moved state, generic Saturn catalog activation, source refactors, new protocol envelopes, timing changes beyond nuke's 200 ms ban delay, application-wide command rewrites, Saturn changes, git cleanup, and plan changes.

Route implementation to the command/dispatch owner for `handlers.go` and `dispatch_adapter.go`; route temporary-session factory/composition wiring and lifecycle tests to the snapshot/listener owner. The nuke slice should land first; resurrect follows only after the reusable snapshot registration/cleanup path is proven.
