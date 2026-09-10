# Next rapid live-agent vertical: live managed-room `room_users` tool

## Decision

**Select `room_users`: one public, read-only, live managed-room directory tool, integrated into the already accepted two-completion bounded tool loop.** It is the smallest next source-grounded live agent capability after `user_message_history` because it supplies fresh connection-state evidence without adding persistence, H2/schema work, memory, command side effects, moderation actions, routing policy, or a general multi-tool router.

This is not another conversation-history/context slice. The accepted context is a request-start snapshot embedded in the system prompt (`internal/agent/assemble/assemble.go`, `SystemPrompt.Render` includes `roomUsersSnapshot`). The selected tool performs a second, live lookup after the model asks for it. That makes a user-visible question such as “who is in lounge now?” executable through the existing model-call → tool-result → synthesis route.

[OBSERVED] Saturn exposes this exact capability as `RoomUsersTool`: optional model-selected `room`, defaulting to the invocation room; it returns `{room, users, count}` from a live managed-room directory (`src/main/java/org/saturn/app/agent/tool/RoomUsersTool.java`, `execute`). Its `EngineAgentRoomDirectory` resolves case-insensitively across the host, its replicas, and the current engine (`EngineAgentRoomDirectory.find`/`managedEngines`); its focused test proves host/replica lookup and unknown/blank failure (`src/test/java/org/saturn/app/agent/tool/EngineAgentRoomDirectoryTest.java`). Saturn’s registry installs `room_users` before the history/database/command tools (`AgentToolRegistryFactory.create`).

[OBSERVED] Zenbot already has every agent-loop transport primitive but freezes the live loop to history alone: `live.ToolLoop.Complete` accepts exactly one named `user_message_history` call, creates a one-tool `execution.Ledger`, and makes a no-tools follow-up; `NewHistoryToolLoop` registers only that tool (`internal/agent/live/tool_loop.go`). The generic registry, descriptor/schema validation, deadline executor, result envelope, and assistant/tool pairing are already real target code (`internal/agent/tool/tool.go`, `internal/agent/tool/execution/execution.go`, `internal/agent/tool/contract/definition.go`).

[OBSERVED] Zenbot’s source of fresh room state is its engine, not H2: `core.EngineImpl.ActiveUserNames` returns a copied list under `usersMu`; the host owns a `core.ReplicaManager`, whose managed replicas are created by `ManagedReplicaController.AddReplica` and dynamically stored in `ReplicaManager` (`internal/core/engine_impl.go`, `ActiveUserNames`; `internal/core/replica_controller.go`; `internal/core/replica_manager.go`). Today `ReplicaManager.Replicas()` intentionally exposes only the `Replica` stop contract, so the live directory prerequisite is missing. This is vertical scope, not a blocker.

[RECOMMENDED] Preserve Saturn’s cross-managed-room selection and current-room default, but make Zenbot’s result deterministic and bounded for an LLM. This is an intentional target adaptation necessitated by Zenbot’s map-backed active-user storage, which has no source-order guarantee. It does not alter authorization or create a new command.

### Rejected next candidates

- **`run_command`/per-command tools:** high-value eventually, but command dispatch/output capture, per-command authorization, room delivery, and action semantics turn it into a larger security vertical. Saturn labels it a room-message/moderation-capable action tool (`RunCommandTool`).
- **`database_query`, `database_schema`, or `database_sql`:** each introduces H2 metadata/query policy and/or dynamic-SQL capability and prerequisite sequencing (`DatabaseQueryTool`, `DatabaseSchemaTool`, `DatabaseSqlTool`). They are not a smaller live route.
- **Durable agent memory:** requires real-H2 persistence lifecycle and prompt/turn ownership, not merely a tool registration.
- **Moderation monitor/action executor:** has public safety and authorization consequences; defer until a dedicated moderation vertical.
- **Reopening history/context:** already accepted. No changes to its room visibility query, prompt context, fresh-history policy, or result envelope are in this slice.

## Exact bounded contract

### Model definition and input

The frozen public tool inventory becomes exactly:

1. `user_message_history` (accepted behavior unchanged), and
2. `room_users`.

`room_users` has this OpenAI function schema:

```json
{
  "type": "function",
  "function": {
    "name": "room_users",
    "description": "List the current users in one managed public room.",
    "parameters": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "room": {"type": "string", "minLength": 1, "maxLength": 100}
      }
    }
  }
}
```

`room` is optional. Omitted means the trusted `api.Context.Room()`. If present, it is only a selector for an already managed public room; it is not an authorization grant and cannot create, join, query, or infer an unmanaged room. Trim before lookup, reject blank/over-100-rune values, and make the directory’s room match case-insensitive. Do not add limit, cursor, user identity, trip, hash, whisper, replica lifecycle, or command arguments.

### Descriptor, visibility, and output

The tool descriptor is:

- `AccessUser`, `ReadOnly`, `ModelData`, idempotent, no capability, no successful-tool prerequisite;
- `ResourceReads: ["managed_room_users"]`, no writes;
- a positive **2-second** executor timeout;
- negative guidance: do not use for whisper/private rooms, unmanaged rooms, or historical messages.

Only public tool-eligible invocations advertise or execute it. The current loop already sends `nil` definitions for whispers and rejects a hallucinated whisper tool call before repository/tool execution (`ToolLoop.Complete`); retain that policy for both tools. `DIRECT`, `MENTION`, `AGENT` relay-backed public invocation, and `AMBIENT` use the same frozen definitions. A model tool result is never sent to chat; only the final synthesized answer reaches the existing direct/runtime finalizer and sink.

The successful `contract.Result.Content` is JSON generated by `encoding/json`:

```json
{"room":"lounge","users":["alice","bob"],"count":2,"returnedCount":2,"truncated":false}
```

`count` is the full live number before output bounding; `users` is a copied list of nonblank nicks, sorted by Unicode case-folded comparison with raw-string tie break; `returnedCount` is the number exposed; `truncated` is `count > returnedCount`. Set a composition-private maximum of **200** users. It is not model input or a new configuration field. This hard bound is the target adaptation that prevents unbounded tool/prompt payloads while preserving truthful count. It must not emit trip, hash, moderation state, IP/connection data, database identifiers, transport state, or an unmanaged channel inventory.

For bad arguments, unmanaged room, unavailable directory, a stopped/removed replica race, or serialization failure, return the normal stable tool error envelope (`TOOL_EXECUTION_FAILED`, generic `tool execution failed`) and do not leak channel topology, driver-like details, or source error strings. The second completion may transparently say the lookup is unavailable, but it may not assert users were read. No retry, fallback to the invocation snapshot, or synthetic empty success is allowed.

## Proposed end-to-end route

```text
public direct l | public mention | AGENT relay | ambient
  -> existing trusted runtime.Invocation
  -> live.ToolLoop.Complete(ctx, inv, existingRecentContext)
     -> Assemble with frozen [user_message_history, room_users]
     -> provider completion #1
        -> no tool calls: existing finalizer/output
        -> exactly one allowed call:
             room_users({}) -> trusted invocation room
             room_users({room}) -> core managed-room directory
                  -> host ActiveUserNames or managed replica ActiveUserNames
                  -> restricted JSON result envelope
             append assistant tool-call + tool result with same call ID
             -> provider completion #2 with tools omitted
             -> finalizer -> existing direct return / runtime sink
        -> batch, unknown, malformed, duplicate, or second tool call: failure
```

The directory reads memory only. It neither owns a goroutine nor performs transport, H2, command, or listener calls. It sees a consistent copied snapshot per selected engine, not a cross-engine transaction; a join/leave after snapshot acquisition can legitimately be reflected on the next call. A replica removed before lookup is unavailable. A replica removed after it supplies a copy may return that copy; no handle is retained.

## Implementation stages

### Stage A — source-compatible managed-room directory prerequisite

**Owner:** `@senior` reviews lifecycle/concurrency API; `@developer` implements focused core tests.

Add a narrow core-owned read interface, preferably in new `internal/core/room_directory.go`:

```go
type RoomUserSnapshot struct { Room string; Users []string }
type RoomUserDirectory interface {
    FindRoomUsers(room string) (RoomUserSnapshot, bool)
}

type EngineRoomUserDirectory struct {
    Host     *EngineImpl
    Replicas *ReplicaManager
}
func (d EngineRoomUserDirectory) FindRoomUsers(room string) (RoomUserSnapshot, bool)
```

It treats `Host` as mandatory construction-owned state and reads the host plus the current registered managed replicas. It normalizes only the lookup key (`strings.TrimSpace`, then `EqualFold`), returns the engine’s stored channel spelling and an immutable `ActiveUserNames` copy, and returns `(RoomUserSnapshot{}, false)` for blank/unmanaged input. It must never start/stop a replica, call the network, or mutate the manager.

Add a narrow safe observation seam to `ReplicaManager`, rather than widening `common.Engine` or casting opaque `Replica` values outside `core`. For example, let the private `managedReplica` wrapper expose its `ManagedEngine`, and add:

```go
func (m *ReplicaManager) ManagedEngines() map[string]ManagedEngine
```

which returns a newly allocated snapshot of only wrappers known to be managed engines. Ordinary `Replica` test doubles remain valid and are simply absent. `EngineRoomUserDirectory` consumes only this snapshot. `ManagedReplicaController.AddReplica` remains the sole path that installs managed replicas; `Remove`/`StopAll` make subsequent lookups absent. Do not change existing replica command, relay, factory, or transport behavior.

This deliberately solves the existing target prerequisite rather than degrading the source vertical to the host-only invocation room. It is also useful to the existing unresolved remote `list` command, but that command is explicitly not changed in this slice.

### Stage B — `room_users` tool and two-tool frozen loop

**Owner:** `@developer`; `@senior` reviews loop bounds/result handling.

1. Add `internal/agent/tool/room_users.go`:

   ```go
   type RoomUserDirectory interface {
       FindRoomUsers(room string) (RoomUserSnapshot, bool)
   }
   type RoomUsers struct {
       Directory RoomUserDirectory
       MaxUsers int
   }
   ```

   Keep this agent-facing interface structurally small so `tool` does not import `core`; `core.EngineRoomUserDirectory` satisfies it. `Descriptor` has the closed optional-room schema and restricted result schema. `Execute` derives the default from `api.Context`, performs one lookup, filters/sorts/bounds copied nick strings, and produces only the specified JSON. A nil directory, invalid trusted default room, bad model argument, missing room, or marshal failure produces the stable tool error described above. Invalid JSON argument parsing may return an error to `execution.Executor`, which must become its existing `INVALID_ARGUMENTS` envelope; it must not call the directory.

2. Generalize only the **frozen one-call loop**, not into a Saturn general router. Rename `NewHistoryToolLoop` to a compatibility wrapper over a new constructor such as:

   ```go
   func NewBoundedToolLoop(
       assembler *assemble.Assembler,
       client llm.LlmClient,
       tools []tool.Tool,
       allowed []string,
   ) (*ToolLoop, error)
   ```

   The constructor clones/validates a nonempty unique tool set, derives sorted provider definitions once from the frozen registry, and records the allowed names privately. `ToolLoop.Complete` continues to allow **exactly one** call total, one assistant/tool pair, and one no-tools follow-up. Replace the hard-coded `userMessageHistoryTool` name check with `Registry.Allowed`; retain rejection of blank IDs, batch calls, first/second `length`, whisper calls, malformed arguments, and all tool calls on completion #2. Build one request-local ledger with a limit of one for every frozen name and preserve `turn.ExecutionLimits{MaxSteps:2, MaxToolCalls:1}`.

   This is a constrained prerequisite: adding a second known read-only tool must not allow tool batches, parallel calls, a third completion, caller-selected registration, mutable registry contents, tool result persistence, turn-memory writes, or prerequisite chains.

3. Preserve `NewHistoryToolLoop` as a thin test/backward-compatible one-tool wrapper only if current package tests use it. Production composition must call `NewBoundedToolLoop` exactly once with `UserMessageHistory` and `RoomUsers`.

### Stage C — one composition root for host, replicas, direct, and live paths

**Owner:** `@developer`; `@senior` review for construction order and disabled-agent behavior.

In `cmd/zenbot/main.go`:

- Replace `newAgentToolLoop` with a function accepting both the existing narrow history repository and an injected `tool.RoomUserDirectory`; it builds exactly the two tools and no future inventory.
- Construct `core.NewReplicaManager(c.Channel)` before `directAgentInvoker`, then construct one `core.EngineRoomUserDirectory{Host:e, Replicas:manager}` after the host engine exists. Pass that same immutable directory reference to `directAgentInvoker` and `newLiveAgent`.
- Thread the directory through both composition helpers so public direct `l` and room/runtime invocations share the same tool policy and see newly registered replicas. The command-facing `DirectAgentInvoker` interface remains unchanged.
- Keep the current disabled branch before provider, directory-dependent tool loop, and credentials construction. It remains `PassParticipation` and a nil direct invoker.
- Keep current conversation context/history repository wiring untouched. There is no H2 query, schema edit, new config property, or second database/server connection.

## Cancellation, failure, visibility, and output matrix

| Condition | Required result |
|---|---|
| Public completion #1 has no tool call | Existing one-completion finalization/output unchanged. |
| Valid `room_users` call and valid visible completion #2 | One final direct reply/runtime sink delivery; raw JSON is model-only. |
| Omitted room | Read only trusted invocation room; no model-controlled scope change. |
| Explicit managed replica room | Case-insensitive selection; source channel spelling and copied live nick snapshot returned. |
| Blank, >100-rune, malformed, unknown, or unmanaged room | No host/replica mutation and generic tool error envelope; one no-tools synthesis attempt remains allowed. |
| Whisper | Definitions are empty; any tool call fails before directory access and no private data is returned. |
| Completion #1 batch/unknown/blank-ID call | Fail before directory access; no follow-up. |
| Directory/tool failure then valid completion #2 | One final response only; it must not claim the failed lookup succeeded. |
| Completion #2 empty, `length`, or any call | Bounded-loop error; direct returns an error, reply-required runtime follows existing failure sink, ambient remains silent/log-only. |
| Parent cancellation/runtime close | Propagate the same context; stop before follow-up; no late reply. The directory itself must not spawn work. |
| Exact no-reply marker / blank final content | Existing `MarkerFinalizer` policy applies unchanged: exact marker silent; blank is an error, which ambient suppresses. |

The system prompt’s treatment of tool evidence as untrusted data remains in force. User nick strings can contain prompt-like text and must never be trusted as instructions.

## Focused TDD plan

Follow one RED → GREEN tracer bullet at a time; no broad router rewrite.

1. **RED — core directory (`internal/core/room_directory_test.go`):** create host and managed replica `EngineImpl` fixtures with distinct channels/users. Prove host/replica case-insensitive lookup, source room spelling, copied result (caller mutation cannot mutate engine), blank/unknown rejection, and ordinary opaque replicas not exposed. Remove/stop a managed replica and prove a subsequent lookup is absent. Run `go test ./internal/core -run 'TestEngineRoomUserDirectory|TestReplicaManager' -count=1`; observe expected failure before implementation.
2. **GREEN:** add the minimal manager observation seam and `EngineRoomUserDirectory`; rerun that focused command green.
3. **RED — tool (`internal/agent/tool/room_users_test.go`):** prove closed optional-room descriptor, omitted-room trusted default, explicit cross-managed room selection, case-insensitive lookup, deterministic sorting, full count versus 200-item output bound, JSON escaping, no identity metadata, invalid/malformed room does not call directory, and generic unavailable error. Run `go test ./internal/agent/tool -run 'TestRoomUsers' -count=1` red, then green.
4. **RED — loop (`internal/agent/live/tool_loop_test.go`):** using the real `RoomUsers` tool and a scripted client, prove completion #1 receives exactly two frozen definitions; a `room_users` call creates matching assistant/tool IDs and exactly one no-tools follow-up; it never calls history. Prove history still works unchanged. Prove batch/unknown/second call, unknown room, whisper call, cancellation during the directory call, and follow-up tool call never touch a directory or make a third provider call. Run the focused live test red, then green.
5. **RED — composition (`cmd/zenbot/live_agent_test.go`):** prove enabled composition gives both direct and live paths the same two-tool loop and directory; a registered replica becomes lookup-visible without recreating the loop; disabled setup constructs no invoker/runtime/provider/tool loop. Keep production test construction local/no network. Run `go test ./cmd/zenbot -run 'Test.*LiveAgent|Test.*DirectAgent' -count=1` red, then green.
6. **Regression and race:** run the relevant packages plus full suite. The new `ManagedEngines` snapshot and `ActiveUserNames` copy must be race-safe under concurrent lookup/add/remove tests.

No real-H2 gate is relevant: the selected tool’s source is live engine state and the vertical intentionally does not read/write persistence. Preserve the existing real-H2 history tests as regression coverage; do not manufacture an H2 fixture for this route.

## Verification gates

Run from `/Users/ab/workspace/go-projects/zenbot`, formatting only slice-owned files:

```sh
gofmt -w \
  internal/core/replica_manager.go internal/core/replica_controller.go internal/core/room_directory.go \
  internal/agent/tool/room_users.go internal/agent/live/tool_loop.go \
  cmd/zenbot/main.go \
  internal/core/room_directory_test.go internal/agent/tool/room_users_test.go \
  internal/agent/live/tool_loop_test.go cmd/zenbot/live_agent_test.go

go test ./internal/core -run 'Test(EngineRoomUserDirectory|ReplicaManager)' -count=1
go test ./internal/agent/tool -run 'Test(RoomUsers|UserMessageHistory)' -count=1
go test ./internal/agent/live -run 'TestToolLoop' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test -race ./internal/core ./internal/agent/tool ./internal/agent/live -count=1
go test ./...
go build ./...
git diff --check
```

Per `MIGRATION_PLAN.md` rapid policy, focused package tests and `go test ./...` are acceptance gates; full vet/race/build expansion is not a blocker absent a concrete slice failure. The targeted race command above is required because the vertical reads dynamic replica/user state. Record any unrelated pre-existing broad gate failure instead of attributing it to this slice.

## Exclusions and risk

**Excluded:** general Saturn router; multi-tool/multi-step sessions; parallel execution; fresh-data forcing; durable memory and agent-tool-memory writes; database query/schema/SQL tools; command gateway/`run_command`; command catalogs; moderation monitor/actions; H2 schema/query work; listener reorder; replica behavior changes; host/agent relay changes; remote `list` command changes; user-history/context changes; tool result persistence; config expansion; retries; output correction/sanitization beyond the accepted finalizer.

**Risks and controls:**

- **Private/topology leakage:** public-only definitions, managed-directory lookup only, no raw engine/replica fields, generic unavailable errors.
- **Unbounded or nondeterministic prompt data:** copied names, deterministic sorting, private 200-result cap with truthful `count`/`truncated`.
- **Scope regression into an open router:** constructor freezes exactly two known tools; one total call, two maximum completions, no tools on synthesis.
- **Replica lifecycle race:** `ReplicaManager` returns a copy snapshot; directory retains no replica handle; user lists are copied under existing lock.
- **Different direct/live behavior:** one injected directory and one `newAgentToolLoop` composition path; focused composition assertions cover both.
- **Dirty tree damage:** edit only files listed above plus the requested new tests and this handoff; do not reformat or modify current unrelated work/protected documents.

## Routing

- **`@developer`:** Stage A core directory implementation/tests; Stage B tool and frozen-loop generalization; Stage C composition/test wiring.
- **`@senior`:** approve the managed-replica observation boundary, inspect the one-call/two-completion invariant, cancellation, output cap, whisper policy, and direct/live composition order before merge.
- **Independent QA reviewer:** replay focused tool-loop and race gates, inspect that `room_users` output contains only room/nicks/count metadata, prove replica removal is reflected, and confirm no H2/history/moderation/command files changed.
