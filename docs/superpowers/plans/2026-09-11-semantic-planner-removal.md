# Semantic Planner Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove semantic planning and obligation matching so one execution LLM owns intent interpretation while the controller retains mechanical safety and a grounded LLM completion gate.

**Architecture:** Surgically reverse the planner portions introduced by `7fbb842`; do not revert the whole commit. Preserve `Ledger.Available`, per-cycle projection, bounded observations, schemas, capabilities, synchronous actions, receipts, and non-retryable unknown action outcomes.

**Tech Stack:** Go, standard library tests, Zenbot's tool registry, turn state machine, and OpenAI-compatible function calling.

**Spec:** `docs/superpowers/specs/2026-09-11-semantic-planner-removal-design.md`

## Global Constraints

- Preserve the exact newest request in every projected model call.
- Retain `SemanticCompletionGate` as the bounded LLM finalizer.
- Keep deterministic enforcement for registry membership, capabilities, schemas, call identity, budgets, duplicate detection, action review, batching, receipts, cancellation, recovery, and context projection.
- Keep `ACTION_OUTCOME_UNKNOWN` non-retryable.
- Keep `room_users` for the current room, `saturn_list` for another room, and zero-argument `saturn_ping` for the bot's fixed hack.chat connection.
- Preserve dirty worktree changes unless they implement the planner or obligation behavior being removed.
- Do not run `git revert 7fbb842`; later safety commits depend on non-planner plumbing from it.

## File Map

- Delete `internal/agent/live/task_planner.go` and `internal/agent/live/task_planner_test.go`.
- Delete `internal/agent/turn/task_contract.go` and `internal/agent/turn/task_contract_test.go`.
- Remove planner composition from `cmd/zenbot/main.go`, `cmd/zenbot/live_agent_test.go`, and `internal/agent/live/tool_loop.go`.
- Remove task-state enforcement from `internal/agent/live/registry_loop.go`, `internal/agent/live/turn_engine.go`, and their tests.
- Simplify projection in `internal/agent/assemble/context.go`, `internal/agent/assemble/assemble.go`, and their tests.
- Remove the obsolete deterministic-obligation premise from `resources/agent/system/completion-gate.txt`.
- Preserve current routing guidance in `resources/agent/system/system-policy.txt` and `internal/command/catalog/catalog.go`.

---

### Task 1: Remove The Planner From Production Composition

**Files:**
- Modify: `cmd/zenbot/main.go:108-154`
- Modify: `cmd/zenbot/live_agent_test.go:268-305`
- Modify: `internal/agent/live/tool_loop.go:27-40,134-166,388-429`
- Delete: `internal/agent/live/task_planner.go`
- Delete: `internal/agent/live/task_planner_test.go`

**Interfaces:**
- Consumes: `NewRegistryToolLoop(*assemble.Assembler, llm.LlmClient, turn.CompletionGate, []tool.Tool, []string, turn.ExecutionLimits) (*ToolLoop, error)`.
- Produces: a `ToolLoop` with no planner field and no planning provider request.

- [ ] **Step 1: Write the failing production-composition test**

Add `reflect` to `cmd/zenbot/live_agent_test.go`. Replace the concrete planner assertion with:

```go
loopType := reflect.TypeOf(*loop)
if _, found := loopType.FieldByName("Planner"); found {
	t.Fatal("production tool loop still exposes semantic planner authority")
}
```

- [ ] **Step 2: Run the test and verify RED**

Run `go test ./cmd/zenbot -run TestNewAgentToolLoopRegistersEveryAgentCommandWithContextualVisibility -count=1 -v`.

Expected: FAIL with `production tool loop still exposes semantic planner authority`.

- [ ] **Step 3: Remove planner construction and APIs**

In `newAgentToolLoop`, keep `NewSemanticCompletionGate` but delete `NewSemanticTaskPlanner`. Compose directly:

```go
return live.NewRegistryToolLoop(assembler, client, completionGate, tools, allowed, turn.ExecutionLimits{
	MaxSteps:        resolved.MaxSteps,
	MaxToolCalls:    resolved.MaxTools,
	MaxCallsPerTool: resolved.MaxCallsPerTool,
	MaxToolFailures: resolved.MaxToolFailures,
	ToolTimeout:     time.Duration(resolved.ToolTimeoutMillis) * time.Millisecond,
})
```

Delete `ToolLoop.Planner`, its planning block, `NewRegistryToolLoopWithPlanner`, and `planningTools`. Retain direct setup of `initialRequest`, `providerTools`, `newestRequest`, and `observations`. Delete both planner files.

- [ ] **Step 4: Adapt planner-only tests**

Delete tests for `submit_task_plan`, its schema/parser, correction, subject canonicalization, and planned ready-frontier visibility. Replace any remaining `NewRegistryToolLoopWithPlanner` calls with `NewRegistryToolLoop` and remove the scripted planner response.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run `go test ./cmd/zenbot ./internal/agent/live -run 'NewAgentToolLoop|RegistryToolLoop' -count=1 -v`.

Expected: PASS, and no provider request contains `submit_task_plan`.

- [ ] **Step 6: Commit**

```bash
git add cmd/zenbot/main.go cmd/zenbot/live_agent_test.go internal/agent/live/tool_loop.go internal/agent/live/registry_loop_test.go internal/agent/live/task_planner.go internal/agent/live/task_planner_test.go
git commit -m "refactor(agent): remove semantic planning pass"
```

---

### Task 2: Remove Semantic Obligations From The Turn Engine

**Files:**
- Modify: `internal/agent/live/registry_loop.go:61-66`
- Modify: `internal/agent/live/turn_engine.go:22-475`
- Modify: `internal/agent/live/turn_engine_test.go`
- Delete: `internal/agent/turn/task_contract.go`
- Delete: `internal/agent/turn/task_contract_test.go`

**Interfaces:**
- Consumes: `turn.State`, `execution.Ledger`, `turn.RecoveryPolicy`, `turn.CompletionGate`, and `assemble.ObservationStore`.
- Produces: a `TurnEngine` with no semantic task field; mechanically valid calls reach review/execution without subject guessing.

- [ ] **Step 1: Write the failing structural regression**

Add `reflect` to `internal/agent/live/turn_engine_test.go` and add:

```go
func TestTurnEngineHasNoSemanticTaskAuthority(t *testing.T) {
	engineType := reflect.TypeOf(TurnEngine{})
	if _, found := engineType.FieldByName("Task"); found {
		t.Fatal("turn engine still owns planner-derived semantic task state")
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run `go test ./internal/agent/live -run TestTurnEngineHasNoSemanticTaskAuthority -count=1 -v`.

Expected: FAIL with `turn engine still owns planner-derived semantic task state`.

- [ ] **Step 3: Remove obligation state and preflight logic**

Delete `TurnEngine.Task`, fallback task construction, `TaskState.Observe`, `TASK_OBLIGATION_FEEDBACK`, `ValidatePendingCall`, `toolsForPending`, and `primaryIntent`. Keep:

```go
providerTools = availableProviderTools(providerTools, executor.Ledger)
```

Keep completion evaluation grounded in `Prompt`, candidate answer, bounded conversation, tool capabilities, and typed results. Remove the task argument from `completeRegistryLoop` and delete both task-contract files after all references are gone.

- [ ] **Step 4: Strengthen correction-history assertions**

In `TestTurnEngineSemanticGateContinuesPromiseUntilToolResultSatisfiesRequest`, assert the correction request contains both `engine.Prompt` and `Execute the available action`. Assert the following request contains `engine.Prompt`, call ID `action-1`, and the bounded `executed` observation.

```go
if !messagesContain(client.requests[0].Messages(), engine.Prompt) ||
	!messagesContain(client.requests[0].Messages(), "Execute the available action") {
	t.Fatalf("semantic correction lost request or feedback: %#v", client.requests[0].Messages())
}
```

- [ ] **Step 5: Run focused tests and verify GREEN**

Run `go test ./internal/agent/live ./internal/agent/turn ./internal/agent/tool/execution -count=1`.

Expected: PASS, including existing unknown-action non-retry tests.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/live/registry_loop.go internal/agent/live/turn_engine.go internal/agent/live/turn_engine_test.go internal/agent/turn/task_contract.go internal/agent/turn/task_contract_test.go
git commit -m "refactor(agent): remove heuristic task reducer"
```

---

### Task 3: Simplify Projection Without Losing Intent Or Evidence

**Files:**
- Modify: `internal/agent/assemble/context.go:118-250`
- Modify: `internal/agent/assemble/assemble.go:326-360`
- Modify: `internal/agent/assemble/context_test.go`
- Modify: `internal/agent/assemble/assemble_test.go`
- Modify: `internal/agent/live/turn_engine.go:426-431`
- Modify: `resources/agent/system/completion-gate.txt`

**Interfaces:**
- Produces: `ProjectTurn(messages []Message, tools []any, observations *ObservationStore, input ContextInput) (Projection, error)`.
- Produces: `(*Assembler).ProjectTurn(messages []Message, tools []any, observations *ObservationStore, newestRequest Message) (Projection, error)`.

- [ ] **Step 1: Rewrite the projection regression first**

Rename `TestProjectTurnPreservesTaskAndAtomicBoundedObservation` to `TestProjectTurnPreservesNewestRequestAndAtomicBoundedObservation`. Remove task construction and call the desired task-free signature:

```go
projection, err := ProjectTurn(messages, tools, store, ContextInput{
	RequiredPrefix: []Message{policy},
	RequiredSuffix: []Message{newest},
	MaxTokens:      350,
	ReserveTokens:  20,
})
```

Assert the exact objective remains, no message starts with `TASK_STATE_JSON=`, and the call/result pair remains atomic, JSON-valid, bounded to 300 bytes, and different from the full result envelope.

- [ ] **Step 2: Run the test and verify RED**

Run `go test ./internal/agent/assemble -run TestProjectTurnPreservesNewestRequestAndAtomicBoundedObservation -count=1 -v`.

Expected: build failure because the desired task-free signature does not exist.

- [ ] **Step 3: Implement the task-free projection API**

Use:

```go
func ProjectTurn(messages []Message, tools []any, observations *ObservationStore, input ContextInput) (Projection, error) {
	input.ManifestTokens = manifestTokenCost(tools)
	sanitized := projectedObservationMessages(messages, observations)
	before, after := splitTurnContext(sanitized, input.RequiredPrefix, input.RequiredSuffix)
	input.Optional = append(input.Optional, before...)
	input.OptionalTail = append(input.OptionalTail, after...)
	return (ContextBudgeter{}).Project(input)
}
```

Delete `taskStateMessage` and the `turn` import. Update the assembler and engine call sites. Select the first system policy directly instead of special-casing `TASK_STATE_JSON`.

- [ ] **Step 4: Correct finalizer instructions and tests**

Remove the claim from `completion-gate.txt` that the controller already enforced semantic obligations. In `TestEveryProviderRequestFitsMultiRoundBudgetAndKeepsProtocolAtomic`, require the exact objective on every request but remove the `TASK_STATE_JSON=` assertion.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run `go test ./internal/agent/assemble ./internal/agent/live -run 'ProjectTurn|EveryProviderRequestFitsMultiRoundBudget|SemanticGate' -count=1 -v`.

Expected: PASS with bounded valid observation JSON and the newest request in every model request.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/assemble/context.go internal/agent/assemble/assemble.go internal/agent/assemble/context_test.go internal/agent/assemble/assemble_test.go internal/agent/live/turn_engine.go internal/agent/live/turn_engine_test.go internal/agent/live/registry_loop_test.go resources/agent/system/completion-gate.txt
git commit -m "refactor(agent): ground completion in request evidence"
```

---

### Task 4: Prove The Live Compound Request Works Directly

**Files:**
- Modify: `internal/agent/live/registry_loop_test.go`
- Verify: `resources/agent/system/system-policy.txt`
- Verify: `internal/command/catalog/catalog.go`

**Interfaces:**
- Consumes: production `NewRegistryToolLoop`, `RoomUsers`, concrete `SaturnCommand` tools, typed outcomes, and action review.
- Produces: an end-to-end regression with two counts, one ordered kick, and one synthesized response.

- [ ] **Step 1: Write the live-shaped regression**

Replace the planner ready-frontier test with `TestRegistryToolLoopExecutesCompoundCountsThenKickWithoutSemanticPlanner`. Script exactly three execution responses: a batch containing `saturn_list(room=lounge)` and zero-argument `room_users`; a `saturn_kick(mode=exact, targets=[@tajweed29])`; then one combined final answer.

Construct only those three tools with `NewRegistryToolLoop`. Keep the real moderation capability on the invocation.

- [ ] **Step 2: Assert direct flow and results**

Assert:

```go
if len(client.requests) != 3 {
	t.Fatalf("provider requests=%d, want no planning request", len(client.requests))
}
for _, name := range []string{"saturn_list", "room_users", "saturn_kick"} {
	if !providerRequestHasTool(client.requests[0], name) {
		t.Fatalf("initial execution request omitted %q", name)
	}
}
```

Also assert the directory is called once, `gateway.commands` equals `[]string{"list lounge", "kick @tajweed29"}`, no request contains `TASK_OBLIGATION_MISMATCH` or `TASK_DEPENDENCY_PENDING`, and the final content is the one combined summary.

- [ ] **Step 3: Run the regression and verify GREEN**

Run `go test ./internal/agent/live -run TestRegistryToolLoopExecutesCompoundCountsThenKickWithoutSemanticPlanner -count=1 -v`.

Expected: PASS.

- [ ] **Step 4: Verify routing guidance**

Run `rg -n 'room_users|saturn_list|saturn_ping|current room|other room|hack.chat' resources/agent/system/system-policy.txt internal/command/catalog/catalog.go internal/agent/tool/room_users.go`.

Expected: explicit current-room, other-room, and fixed hack.chat distinctions. Add only a missing sentence and cover it with the existing policy/catalog test if the check exposes a gap.

- [ ] **Step 5: Run routing suites and commit**

Run `go test ./internal/agent/live ./internal/agent/tool ./internal/command/catalog -count=1`.

Then:

```bash
git add internal/agent/live/registry_loop_test.go resources/agent/system/system-policy.txt internal/command/catalog/catalog.go
git commit -m "test(agent): cover direct compound tool execution"
```

---

### Task 5: Verify Safety And Remove Residual References

**Files:**
- Modify if stale: `AGENTIC_ARCHITECTURE.md`
- Modify if stale: `README.md`
- Verify: `cmd/zenbot`, `internal/agent`, `internal/command/catalog`, and `resources/agent/system`

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: a planner-free production build with preserved mechanical safety.

- [ ] **Step 1: Scan runtime code for removed concepts**

Run:

```bash
rg -n 'SemanticTaskPlanner|ObjectiveOnlyTaskPlanner|TaskPlanner|TaskContract|TaskState|Obligation|submit_task_plan|TASK_STATE_JSON|TASK_OBLIGATION|TASK_DEPENDENCY_PENDING|subjectFromArguments|toolsForPending' cmd internal resources AGENTIC_ARCHITECTURE.md README.md
```

Expected: no runtime/current-architecture matches. Historical superseded documents may retain references.

- [ ] **Step 2: Run focused safety verification**

```bash
go test ./internal/agent/tool/execution -run 'Action|OutcomeUnknown|LedgerAvailability' -count=1 -v
go test ./internal/agent/live -run 'UnknownAction|Delivery|SemanticGate|CompoundCountsThenKick|EveryProviderRequestFitsMultiRoundBudget' -count=1 -v
go test ./internal/agent/tool ./internal/command/catalog -count=1
```

Expected: PASS with non-retryable unknown actions, receipt-gated delivery, unique intent ownership, and schema parity intact.

- [ ] **Step 3: Run race and full verification**

```bash
go test -race ./internal/agent/...
go vet ./...
go test ./...
make check
git diff --check
```

Expected: every command exits 0 with zero test failures or races.

- [ ] **Step 4: Inspect and commit remaining in-scope changes**

Run `git status --short --branch` and `git diff --stat`. Confirm the diff still contains ledger availability, context budgeting, authoritative routing, schemas, synchronous actions, receipts, and `SemanticCompletionGate`.

Then commit only remaining in-scope files:

```bash
git add AGENTIC_ARCHITECTURE.md README.md cmd/zenbot internal/agent internal/command/catalog resources/agent/system
git commit -m "refactor(agent): finish planner-free execution loop"
```
