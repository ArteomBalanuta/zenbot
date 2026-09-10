# Agent Loop Correctness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct the six audited LLM tool-loop defects in their required priority order, ending with a bounded, outcome-aware, intent-stable execution loop.

**Architecture:** Keep the legacy human command interface compatible while hardening the agent boundary. First make action execution synchronous and cancellation non-retryable, then introduce verified business outcomes, freeze a semantic task contract, remove model-facing routing collisions, enforce truthful conditional schemas, and finally reproject the complete transcript through one context budget before every provider request.

**Tech Stack:** Go, `context`, OpenAI-compatible chat completions, recursive JSON Schema validation, H2 integration tests, table-driven tests, race detector.

**Spec:** `docs/superpowers/specs/2026-09-10-agent-loop-correctness-design.md`

## Global Constraints

- Implement Tasks 1 through 6 strictly in numeric order.
- Preserve public Saturn command syntax, caller capabilities, OpenAI-compatible transport, public-room memory, and whisper isolation.
- Keep `command.Command.Execute(context.Context) (model.Status, error)` compatible for non-agent callers.
- Never return from `Executor.Execute` while an executor-owned action goroutine remains active.
- Never automatically retry `ACTION_OUTCOME_UNKNOWN`.
- Never treat command dispatch, `model.SUCCESSFUL`, or HTTP success alone as proof of a committed business outcome.
- Never let the model mutate the frozen objective or deterministically completed obligations.
- Never expose two provider tools with the same primary intent.
- Advertised schemas and execution-time encoders must accept and reject the same argument shapes.
- Every provider request must fit the configured context budget.
- Every production behavior change follows a witnessed red-green-refactor cycle.

---

### Task 1: Synchronous Actions And Non-Retryable Unknown Outcomes

**Files:**

- Modify: `internal/agent/tool/execution/execution.go`
- Modify: `internal/agent/tool/execution/execution_test.go`
- Modify: `internal/agent/turn/recovery.go`
- Modify: `internal/agent/turn/recovery_test.go`
- Modify: `internal/agent/live/turn_engine.go`
- Modify: `internal/agent/live/turn_engine_test.go`

**Interfaces:**

- Produces: `invokeRead` for preemptible read-only work.
- Produces: `invokeAction` for synchronous action work.
- Extends: `turn.RecoveryInput` with `Effect contract.Effect` and `Idempotent bool`.
- Produces: stable error code `ACTION_OUTCOME_UNKNOWN` for cancelled actions without a verified terminal result.

- [ ] **Step 1: Add the failing executor regression test**

  Add `TestExecutorCancelledActionDoesNotReturnBeforeActionStops` to `execution_test.go`. Use a real test tool whose `Execute` closes `started`, deliberately ignores its cancelled context until `release` closes, then returns `ctx.Err()`. Run `Executor.Execute` in a test goroutine, cancel after `started`, and assert that the result channel remains blocked for 20ms. Close `release`, receive the result, and assert `ErrorCode == "ACTION_OUTCOME_UNKNOWN"` and that the tool closed `finished` before the result arrived.

  The production mutation caught by this test is restoring the current action path through the detached `invoke` goroutine.

- [ ] **Step 2: Run the test and witness the current early return**

  Run:

  ```bash
  go test ./internal/agent/tool/execution -run TestExecutorCancelledActionDoesNotReturnBeforeActionStops -count=1 -v
  ```

  Expected red result: the executor returns `TOOL_TIMEOUT` or cancellation before `release` is closed.

- [ ] **Step 3: Split read and action invocation**

  Refactor `execution.go` so panic normalization lives in a direct helper:

  ```go
  func invokeDirect(t tool.Tool, ctx context.Context, agent api.Context, args json.RawMessage) (result contract.Result, err error) {
      defer func() {
          if recover() != nil {
              result = contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed")
              err = nil
          }
      }()
      result, err = t.Execute(ctx, agent, args)
      if err == nil && result.ToolName == "" {
          err = errNilResult
      }
      return result, err
  }
  ```

  Keep the existing result-channel/select behavior only in `invokeRead`. For `descriptor.Effect() == contract.Action`, call `invokeDirect` synchronously. When that call returns an error and `ctx.Err() != nil`, return `contract.ErrorResult(callID, toolName, "ACTION_OUTCOME_UNKNOWN", "action outcome is unknown after cancellation")`. A typed tool result, including a definite typed error, remains authoritative.

- [ ] **Step 4: Run the executor package tests**

  Run:

  ```bash
  go test ./internal/agent/tool/execution -count=1
  ```

  Expected green result: read-only timeout tests still return `TOOL_TIMEOUT`; the new action test proves no early return.

- [ ] **Step 5: Add the failing recovery test**

  Add a table row to `TestRecoveryPolicyChoosesBoundedCorrectionOrDegradation`:

  ```go
  {
      name: "unknown action outcome never retries",
      in: RecoveryInput{
          ErrorCode: "ACTION_OUTCOME_UNKNOWN",
          Effect: contract.Action,
          Idempotent: false,
          ToolRoundsRemaining: 3,
          HasObservations: true,
      },
      want: RecoveryFinalize,
  }
  ```

  Add a `TurnEngine` test where an unknown action outcome is followed by available rounds; assert the next provider request contains no action tools and no second execution occurs.

- [ ] **Step 6: Run the recovery tests and witness retry behavior**

  Run:

  ```bash
  go test ./internal/agent/turn ./internal/agent/live -run 'UnknownAction|RecoveryPolicy' -count=1 -v
  ```

  Expected red result: the current policy chooses `RETRY_MODEL` and the engine retains tools.

- [ ] **Step 7: Make recovery effect-aware**

  Extend `RecoveryInput`, look up the call descriptor in `TurnEngine.recoveryDecision`, and return `RecoveryFinalize` for `ACTION_OUTCOME_UNKNOWN` regardless of remaining rounds. Disable tools before terminal synthesis whenever this code appears so modified arguments cannot bypass exact-call deduplication.

- [ ] **Step 8: Verify Task 1 and commit**

  Run:

  ```bash
  go test -race ./internal/agent/tool/execution ./internal/agent/turn ./internal/agent/live -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/agent/tool/execution internal/agent/turn internal/agent/live
  git commit -m "fix(agent): prevent detached timed-out actions"
  ```

---

### Task 2: Verified Business Outcomes And Delivery Receipts

**Files:**

- Modify: `internal/agent/commandgateway/gateway.go`
- Modify: `internal/command/agent_gateway.go`
- Modify: `internal/command/agent_gateway_test.go`
- Modify: `internal/command/restart_shutdown.go`
- Modify: `internal/command/restart_shutdown_test.go`
- Modify: `internal/service/services.go`
- Modify: `internal/service/mail_group_c_parity_test.go`
- Modify: `internal/command/identity_commands.go`
- Modify: `internal/command/identity_commands_test.go`
- Modify: `internal/repository/user_queries.go`
- Modify: `internal/repository/h2/user_queries.go`
- Modify: `internal/repository/h2/last_online_test.go`
- Modify: `internal/service/last_online_test.go`
- Modify: `internal/command/last_online.go`
- Modify: `internal/agent/tool/contract/definition.go`
- Modify: `internal/agent/tool/saturn_command.go`
- Modify: `internal/agent/tool/saturn_command_test.go`
- Modify: `internal/agent/tool/run_command.go`
- Modify: `internal/agent/tool/run_command_test.go`
- Modify: `internal/agent/live/tool_loop.go`
- Modify: `internal/agent/live/tool_loop_test.go`
- Modify: `internal/agent/live/registry_loop_test.go`
- Modify: `internal/agent/live/runner.go`
- Modify: `internal/agent/live/runner_test.go`
- Modify: `cmd/zenbot/live_agent_test.go`

**Interfaces:**

- Produces: `commandgateway.OutcomeStatus`, `DeliveryReceipt`, and typed `Execution` from the approved spec.
- Adds: `repository.LastOnlineRecord.Found bool`.
- Adds: `contract.Result.EffectsCommitted`, `DeliveryCount`, and `VerifiedRoomDelivery() bool`.
- Produces: `contract.ActionSuccessResult(call, tool string, value any, deliveryCount int)`.

- [ ] **Step 1: Write failing source-level business tests**

  Change the lifecycle failure cases to require `model.FAILED` and the original controller error. Change `TestMailGroupCQueueIgnoresFailedWriteLikeSaturn` into `TestMailGroupCQueueReturnsFailedWrite` and require a non-nil error after dropping `mail`. Change comma-target access expectations from `first:User`/`second:User` to `first:Admin`/`second:Admin`, and add a two-target test where the second `GrantTrip` fails and the command returns `FAILED` without sending a success reply. Change missing last-online tests to assert `Found == false` at the repository boundary and `errors.Is(err, repository.ErrNotFound)` at the service boundary.

- [ ] **Step 2: Run the source-level tests and witness all four false successes**

  Run:

  ```bash
  go test ./internal/command ./internal/service ./internal/repository/h2 -run 'RestartShutdown|MailGroupCQueueReturns|AccessCommandComma|AccessCommandStops|LastOnline' -count=1 -v
  ```

  Expected red result: lifecycle and mail errors are swallowed, the wrong role is granted, and missing last-online data renders placeholders.

- [ ] **Step 3: Correct the four producers**

  In `restart_shutdown.go`, return `model.FAILED, err` from failed controller requests. In `MailService.QueueResolved`, return the `Exec` error. In the comma-target access branch, call `GrantTrip(ctx, trip, role)` and check each error before replying. Add `Found` to `LastOnlineRecord`, set it only after the first query scan succeeds, define `repository.ErrNotFound`, and have `UserService.LastOnline` return that sentinel when `Found` is false.

- [ ] **Step 4: Re-run the source-level tests**

  Run the command from Step 2 and confirm all targeted tests pass before changing the agent gateway.

- [ ] **Step 5: Add failing gateway and tool-result tests**

  Add tests asserting:

  - A successful command with one successfully delivered message returns `Status: OutcomeSucceeded`, `EffectsCommitted: true`, and `Delivery.Count == 1`.
  - A legacy `SUCCESSFUL` command with no captured delivery does not produce a verified room delivery.
  - `SaturnCommand` rejects `OutcomeRejected`, `OutcomeUnknown`, `EffectsCommitted == false`, nil delivery, and zero delivery count.
  - A verified result is the only result for which `suppressRegistryReply` returns true.
  - `Runner` never synthesizes `Completed the requested Saturn action.` and never persists an unverified result as tool-owned completion.

- [ ] **Step 6: Run the gateway/live tests and witness boolean promotion**

  Run:

  ```bash
  go test ./internal/command ./internal/agent/tool ./internal/agent/live -run 'AgentCommandGateway|SaturnCommand|VerifiedRoomDelivery|Suppress|ToolOwned' -count=1 -v
  ```

  Expected red result: `Executed:true` and empty messages are currently accepted and suppression does not inspect receipts.

- [ ] **Step 7: Implement typed agent outcomes**

  Replace `Executed` with the approved gateway types. Populate delivery receipts only inside the successful send methods already captured by `agentCaptureEngine`. After a legacy command returns, map non-success status to `OutcomeRejected`, cancellation without a terminal result to `OutcomeUnknown`, and a successful command to `OutcomeSucceeded` with `EffectsCommitted:true`. A zero-message result has no delivery receipt.

  Extend `contract.Result` with outcome metadata while leaving its JSON `data` payload backward compatible. Implement:

  ```go
  func (r Result) VerifiedRoomDelivery() bool {
      return !r.IsError && r.EffectsCommitted && r.DeliveryCount > 0
  }
  ```

  Require this predicate in `SaturnCommand`, `RunCommand`, reply suppression, and tool-owned persistence. Delete the runner fallback completion sentence.

- [ ] **Step 8: Update gateway stubs mechanically and verify Task 2**

  Replace successful test fixtures with explicit typed successful outcomes and positive receipts. Do not give rejection fixtures success metadata merely to preserve old assertions.

  Run:

  ```bash
  go test -race ./internal/command ./internal/service ./internal/repository/h2 ./internal/agent/tool ./internal/agent/live ./internal/agent/runtime -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/command internal/service internal/repository internal/agent
  git commit -m "fix(agent): require verified command outcomes"
  ```

---

### Task 3: Immutable Semantic Task Contract

**Files:**

- Create: `internal/agent/turn/task_contract.go`
- Create: `internal/agent/turn/task_contract_test.go`
- Create: `internal/agent/live/task_planner.go`
- Create: `internal/agent/live/task_planner_test.go`
- Modify: `internal/agent/turn/completion.go`
- Modify: `internal/agent/turn/state.go`
- Modify: `internal/agent/live/turn_engine.go`
- Modify: `internal/agent/live/turn_engine_test.go`
- Modify: `internal/agent/live/tool_loop.go`
- Modify: `internal/agent/live/semantic_completion_gate.go`
- Modify: `cmd/zenbot/main.go`
- Modify: `resources/agent/system/completion-gate.txt`

**Interfaces:**

- Produces: immutable `turn.TaskContract`, `Constraint`, `Obligation`, and `TaskState` from the approved spec.
- Produces: `turn.TaskState.Observe(toolName, call, result) error` and `Pending() []Obligation`.
- Produces: `live.TaskPlanner` and constrained `SemanticTaskPlanner`.
- Adds: `TurnEngine.Task *turn.TaskState` and current-tool capability projection.

- [ ] **Step 1: Add failing contract/reducer unit tests**

  Test that construction copies all slices, computes `RequestHash` from the exact objective, rejects blank/duplicate obligation IDs and dependency cycles, and prevents returned slices from mutating internal state. Test that a result satisfies an obligation only when its frozen provider tool name, normalized subject, dependencies, verified outcome, and required receipt all match. Assert errors and `ACTION_OUTCOME_UNKNOWN` never satisfy an obligation. Task 4 will replace tool-name identity with stable primary-intent identity after duplicate routes have been removed.

- [ ] **Step 2: Run the new turn tests and witness missing types**

  Run:

  ```bash
  go test ./internal/agent/turn -run 'TaskContract|TaskState' -count=1 -v
  ```

  Expected red result: the task-contract API does not exist.

- [ ] **Step 3: Implement immutable contract and deterministic reducer**

  Implement constructors that own their data, stable SHA-256 request hashing, cycle validation, normalized subjects, and append-only evidence records. `TaskState` is the only component allowed to change obligation status.

- [ ] **Step 4: Add failing structured-planner tests**

  Model the planner after `SemanticCompletionGate`: require exactly one `submit_task_plan` call with arrays of constraints and obligations. Tests must reject provider tool names absent from the caller-filtered manifest, unknown dependencies, duplicate IDs, an objective supplied by the model that differs from the controller's exact prompt, and a second malformed response. The controller must overwrite—not trust—the model's objective and IDs.

- [ ] **Step 5: Run and implement the planner**

  Run:

  ```bash
  go test ./internal/agent/live -run SemanticTaskPlanner -count=1 -v
  ```

  Implement one constrained planning request plus one bounded correction. Add `NewRegistryToolLoopWithPlanner`; keep the existing constructor using an objective-only deterministic planner for compatibility tests, while production composition injects `SemanticTaskPlanner`.

- [ ] **Step 6: Add the failing three-tool drift integration test**

  Script a plan containing three dependent obligations, then return three heterogeneous tool calls over separate cycles. Put instruction-like text in observation two asking the model to ignore the original objective. Assert every post-observation provider request contains the exact objective hash and pending obligations, the third obligation cannot complete before the first two, and the completion gate cannot finalize until all three typed outcomes are observed.

- [ ] **Step 7: Run the drift test and witness missing semantic state**

  Run:

  ```bash
  go test ./internal/agent/live -run 'ThreeTool.*Objective|TaskObligation' -count=1 -v
  ```

  Expected red result: current state exposes only counters and the gate can accept without deterministic obligations.

- [ ] **Step 8: Integrate task state into every cycle**

  Build the task before the first ordinary completion. Inject a bounded controller-generated task-state message before each model call. Feed each call-bound result through `TaskState.Observe`. Before consulting the semantic gate, reject finalization when `Pending()` is nonempty or an unknown action outcome exists. Pass only tools still executable under the ledger and call budgets.

- [ ] **Step 9: Verify Task 3 and commit**

  Run:

  ```bash
  go test -race ./internal/agent/turn ./internal/agent/live ./cmd/zenbot -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/agent/turn internal/agent/live cmd/zenbot resources/agent/system/completion-gate.txt
  git commit -m "fix(agent): preserve semantic task obligations"
  ```

---

### Task 4: Authoritative Intent Routes

**Files:**

- Modify: `internal/agent/tool/contract/definition.go`
- Modify: `internal/agent/tool/contract/manifest.go`
- Modify: `internal/agent/tool/contract/manifest_test.go`
- Modify: `internal/agent/tool/tool.go`
- Modify: `internal/agent/tool/manifest_test.go`
- Modify: `internal/agent/tool/room_users.go`
- Modify: `internal/agent/tool/room_users_test.go`
- Modify: `internal/agent/tool/database_query.go`
- Create: `internal/agent/tool/database_query_test.go`
- Modify: `internal/agent/tool/saturn_command.go`
- Modify: `internal/command/catalog/agent_contract.go`
- Modify: `internal/command/catalog/catalog.go`
- Modify: `internal/command/catalog/catalog_test.go`
- Modify: `internal/agent/live/task_planner.go`
- Modify: `internal/agent/live/task_planner_test.go`
- Modify: `cmd/zenbot/main.go`
- Modify: `cmd/zenbot/live_agent_test.go`
- Modify: `resources/agent/system/system-policy.txt`

**Interfaces:**

- Adds: `contract.Descriptor.PrimaryIntent() string` and `contract.WithPrimaryIntent(string)`.
- Adds: `contract.WithInternalFallback()` and `Descriptor.InternalFallback() bool`.
- Adds: `RoomUserResolver.ResolveRoomUsers(context.Context, api.Context, string) (RoomUserSnapshot, error)` for managed-first, temporary-snapshot fallback.
- Produces: registry manifest validation that rejects duplicate exposed primary intents.

- [ ] **Step 1: Add failing intent-collision tests**

  Construct two allowed descriptors with `PrimaryIntent == "room_presence"` and assert `Registry.Manifest` fails. Mark one internal fallback and assert it is omitted from provider definitions. Add production-manifest assertions that exactly one exposed tool owns each of `room_presence`, `named_user_public_history`, and `trip_nicknames`.

- [ ] **Step 2: Run and witness current collisions**

  Run:

  ```bash
  go test ./internal/agent/tool ./internal/agent/live ./cmd/zenbot -run 'PrimaryIntent|IntentCollision|AuthoritativeIntent' -count=1 -v
  ```

  Expected red result: descriptors have no intent identity and the provider manifest includes both presence/history/nickname routes.

- [ ] **Step 3: Implement descriptor and manifest invariants**

  Add normalized primary-intent metadata. Require every model-facing descriptor to declare it. During manifest construction, reject repeated nonblank primary intents; omit internal fallbacks. Include primary intent in compact provider descriptions and task-planner capabilities. Migrate frozen Task 3 obligations from provider tool names to primary intents without changing their dependency or evidence semantics.

- [ ] **Step 4: Make `room_users` authoritative**

  Replace the error instructing the model to call `saturn_list` with a `RoomUserResolver`. Its implementation checks the existing directory first and otherwise awaits the existing temporary snapshot workflow, returning structured users without requiring another model-selected tool. Keep the human `list` command registered, but mark its agent descriptor internal fallback so `saturn_list` is absent from provider definitions.

- [ ] **Step 5: Remove database intent duplicates**

  Remove `recent_messages_for_user` and `known_nicks_for_trip` from the model-facing `database_query` enum and dispatch. Keep repository methods used by `user_message_history` and `saturn_nicks`. Assign distinct primary intents to moderator trip-message lookup and nickname-based public history.

- [ ] **Step 6: Verify manifest routing and commit**

  Run:

  ```bash
  go test ./internal/agent/tool/... ./internal/agent/live ./internal/command/catalog ./cmd/zenbot -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/agent/tool internal/agent/live internal/command/catalog cmd/zenbot resources/agent/system/system-policy.txt
  git commit -m "fix(agent): enforce authoritative intent routes"
  ```

---

### Task 5: Truthful Conditional Schemas

**Files:**

- Modify: `internal/agent/tool/contract/schema.go`
- Modify: `internal/agent/tool/contract/contract_test.go`
- Create: `internal/agent/tool/contract/schema_fuzz_test.go`
- Modify: `internal/command/catalog/agent_contract.go`
- Modify: `internal/command/catalog/agent_contract_test.go`
- Modify: `internal/agent/live/tool_loop.go`
- Modify: `internal/agent/live/registry_loop_test.go`

**Interfaces:**

- Extends: supported schema keywords with `oneOf` and `const`.
- Produces: recursive exactly-one-branch validation in both `ValidateSchema` and `ValidateArguments`/`ValidateResult`.
- Adds: provider function definition field `strict: true` for supported contracts.
- Produces: catalog-wide `AgentArgumentContract` schema/encoder parity coverage.

- [ ] **Step 1: Add failing schema tests**

  Add literal schemas proving: valid `const`; invalid const type; `oneOf` with fewer than two branches rejected; zero matching branches rejected; more than one matching branch rejected; nested error paths retained. Fuzz seeds must include ambiguous branches and malformed conditional schemas and assert no panic.

- [ ] **Step 2: Run and witness unsupported keywords**

  Run:

  ```bash
  go test ./internal/agent/tool/contract -run 'OneOf|Const' -count=1 -v
  ```

  Expected red result: `oneOf` and `const` are rejected as unsupported.

- [ ] **Step 3: Implement recursive conditional validation**

  Allow schema nodes either to declare a normal `type` or a top-level `oneOf`. Validate every branch at construction. At value validation, count successful branches and require exactly one. Validate `const` by canonical JSON equality and require its value to match the node type.

- [ ] **Step 4: Add failing command contract parity tests**

  For `automove`, assert configure requires source and destination while enable/disable reject them. For `kick`, assert exact/contains reject two targets and multiple accepts two. For each remaining database query branch, assert its required selector and reject irrelevant selectors. Each accepted schema fixture must successfully encode; each rejected fixture must fail before `Encode` is called.

- [ ] **Step 5: Replace flat schemas with discriminated unions**

  Build `oneOf` branches using literal operation/mode `const` values and `additionalProperties:false`. Preserve the encoded Saturn command tails exactly. Add `strict:true` to provider function declarations in `providerToolDefinition` and update the live provider-manifest test to assert its presence without changing tool-choice behavior.

- [ ] **Step 6: Verify Task 5 and commit**

  Run:

  ```bash
  go test ./internal/agent/tool/contract ./internal/command/catalog ./internal/agent/live ./internal/agent/llm/openai -count=1
  go test ./internal/agent/tool/contract -run Fuzz -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/agent/tool/contract internal/command/catalog internal/agent/live
  git commit -m "fix(agent): advertise truthful conditional schemas"
  ```

---

### Task 6: Per-Cycle Observation Projection And Context Budgeting

**Files:**

- Create: `internal/agent/assemble/observations.go`
- Create: `internal/agent/assemble/observations_test.go`
- Modify: `internal/agent/assemble/context.go`
- Modify: `internal/agent/assemble/context_test.go`
- Modify: `internal/agent/assemble/assemble.go`
- Modify: `internal/agent/assemble/assemble_test.go`
- Modify: `internal/agent/tool/contract/definition.go`
- Modify: `internal/agent/tool/user_message_history.go`
- Modify: `internal/agent/tool/user_message_history_test.go`
- Modify: `internal/agent/tool/database_query.go`
- Modify: `internal/agent/live/registry_loop.go`
- Modify: `internal/agent/live/turn_engine.go`
- Modify: `internal/agent/live/registry_loop_test.go`
- Modify: `internal/agent/live/semantic_completion_gate.go`
- Modify: `internal/agent/live/semantic_completion_gate_test.go`

**Interfaces:**

- Produces: request-local `assemble.ObservationStore` and bounded `ObservationView` from the approved spec.
- Adds: `contract.Descriptor.MaxModelResultBytes() int` with a validated positive default.
- Produces: `assemble.ProjectTurn(messages, tools, taskState, observations, ContextInput) (Projection, error)` used before every provider call.
- Enforces: `assemble.Config.MaxPromptChars` against the exact newest prompt.

- [ ] **Step 1: Add failing observation projection tests**

  Store a large object containing an array of rows and assert the full JSON remains retrievable by call ID while the model view is valid JSON, below its byte limit, reports the original count, sets `truncated:true`, and preserves bounded head/tail samples. Add Unicode string and malformed-result fixtures; malformed success data must become a typed invalid-result observation rather than sliced text.

- [ ] **Step 2: Run and witness missing observation store**

  Run:

  ```bash
  go test ./internal/agent/assemble -run Observation -count=1 -v
  ```

  Expected red result: the observation storage/projection API does not exist.

- [ ] **Step 3: Implement deterministic projections**

  Own full results in a call-ID map. For arrays, retain total count plus bounded head/tail elements; for objects, retain fields in lexical order until the byte ceiling; for strings, truncate by rune. Marshal after selection and reduce samples until the complete envelope fits. Never truncate serialized JSON bytes.

- [ ] **Step 4: Add failing multi-round budget tests**

  Configure a tiny but viable context budget. Script three tool rounds returning large observations, a semantic correction, terminal synthesis, and final reflection. For every captured `LlmRequest`, independently serialize messages and tools and assert estimated tokens plus reserve are within the configured maximum. Assert assistant tool calls and matching tool messages are either both present or both absent, and the immutable objective/task state is always present.

  Add an assembler test where `len([]rune(inv.Prompt())) > MaxPromptChars`; assert failure before `client.Complete` is called.

- [ ] **Step 5: Run and witness follow-up overflow**

  Run:

  ```bash
  go test ./internal/agent/assemble ./internal/agent/live -run 'EveryProviderRequestFits|MaxPromptChars|MultiRoundBudget' -count=1 -v
  ```

  Expected red result: initial assembly is bounded, but raw follow-up envelopes bypass `ContextBudgeter`, and prompt characters are not independently enforced.

- [ ] **Step 6: Reproject before every provider call**

  Replace direct `llm.NewLlmRequest(messages, tools, ...)` construction in tool follow-up, semantic retry, terminal synthesis, and final reflection with one projector. Classify assistant-call/tool-result pairs as atomic high-priority context units. Preserve trusted policy, frozen task state, newest request, live manifest, and output reserve as required units; rank observations above ordinary history and prune older units deterministically.

- [ ] **Step 7: Tighten result contracts**

  Give `user_message_history.rows` a concrete item schema and `maxItems:500`. Replace `database_query`'s `type:any` result with a bounded object/row schema for its remaining operations. Enforce each descriptor's maximum model-result bytes before transcript insertion.

- [ ] **Step 8: Verify Task 6 and commit**

  Run:

  ```bash
  go test -race ./internal/agent/assemble ./internal/agent/live ./internal/agent/tool -count=1
  git diff --check
  ```

  Commit:

  ```bash
  git add internal/agent/assemble internal/agent/live internal/agent/tool
  git commit -m "fix(agent): budget every tool-loop observation"
  ```

---

### Task 7: Completion Audit And Documentation

**Files:**

- Modify: `AGENTIC_ARCHITECTURE.md`
- Modify: `README.md`
- Modify: `docs/superpowers/plans/2026-09-10-agent-loop-correctness.md`

**Interfaces:**

- Consumes: all six completed runtime contracts.
- Produces: no new runtime API.

- [ ] **Step 1: Update architecture documentation from current code**

  Document synchronous action execution, unknown outcomes, verified delivery receipts, semantic task contracts, unique primary intents, conditional schemas, and per-cycle context projection. Remove statements that action deadlines forcibly stop arbitrary in-process work or that legacy status alone proves success.

- [ ] **Step 2: Run focused mutation-oriented regressions**

  Run:

  ```bash
  go test ./internal/agent/tool/execution -run 'CancelledAction|Timeout' -count=20
  go test ./internal/command ./internal/service ./internal/repository/h2 -run 'RestartShutdown|MailGroupC|AccessCommand|LastOnline' -count=3
  go test ./internal/agent/live -run 'ThreeTool|Objective|AuthoritativeIntent|EveryProviderRequestFits' -count=3
  go test ./internal/agent/tool/contract ./internal/command/catalog -run 'OneOf|Const|Parity' -count=3
  ```

- [ ] **Step 3: Run repository-wide verification**

  Run in this order and inspect every exit code:

  ```bash
  git diff --name-only --diff-filter=ACM HEAD~6..HEAD | rg '\.go$' | xargs gofmt -w
  go test -race ./internal/agent/...
  go vet ./...
  go test ./...
  make check
  git diff --check
  git status --short
  ```

- [ ] **Step 4: Perform the six-requirement evidence audit**

  For each numbered requirement, record the exact regression test and production invariant that proves it. Treat a green broad suite without the named regression as insufficient. Confirm no unrelated behavior or files were added.

- [ ] **Step 5: Commit documentation and audit status**

  After all verification succeeds, mark the plan status implemented with the command evidence and commit:

  ```bash
  git add AGENTIC_ARCHITECTURE.md README.md docs/superpowers/plans/2026-09-10-agent-loop-correctness.md
  git commit -m "docs(agent): document loop correctness guarantees"
  ```
