# Agent Harness Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single production-grade Zenbot turn engine with strict tool protocols, safe ordering, bounded recovery, long-context memory compaction, and configurable HITL interruption.

**Architecture:** Replace divergent generalized loop behavior with request-local state transitions coordinated by `live.TurnEngine`. Keep authorization, execution, context assembly, recovery, and persistence behind narrow interfaces so each boundary can be validated and tested independently.

**Tech Stack:** Go, OpenAI-compatible chat completions, H2 over PostgreSQL wire protocol, `context`, JSON Schema subset, table-driven tests, fuzz tests.

**Spec:** `docs/superpowers/specs/2026-09-10-agent-harness-hardening-design.md`

**Status:** Implemented and verified. The procedural checkboxes below are retained as the original TDD execution recipe; completion evidence is recorded in `HARDENING_CHECKLIST.md`.

## Global Constraints

- Preserve existing `*l`, exact-mention, shared public-room memory, whisper isolation, command catalog, and tool behavior.
- Default HITL policy must allow currently authorized calls; enabling interruption is an explicit configuration choice.
- Never place room messages, historical evidence, tool output, trip, hash, creator trip, API keys, or SQL text in a system-role message or logs.
- Tool action ordering is sequential; only pairwise-compatible read-only idempotent calls may share a parallel stage.
- Context configuration supports ceilings up to 1,000,000 estimated tokens and never truncates serialized JSON.
- Every production behavior change follows a witnessed red-green-refactor cycle.

---

### Task 1: Recursive Tool Contracts And Protocol Identity

**Files:**
- Modify: `internal/agent/tool/contract/schema.go`
- Modify: `internal/agent/tool/contract/definition.go`
- Modify: `internal/agent/tool/execution/execution.go`
- Modify: `internal/agent/live/registry_loop.go`
- Test: `internal/agent/tool/contract/contract_test.go`
- Test: `internal/agent/tool/execution/execution_test.go`
- Test: `internal/agent/live/registry_loop_test.go`

**Interfaces:**
- Produces: `contract.ValidateSchema`, `contract.ValidateArguments`, and `contract.ValidateResult` with recursive semantics.
- Produces: `execution.ValidateBatchIdentity(calls []Call) error`.
- Produces: executor results whose `CallID` and `ToolName` always equal the validated invocation.

- [ ] Add failing table tests where nested object fields, array items, nested required fields, and nested additional properties violate schemas.
- [ ] Run `go test ./internal/agent/tool/contract -run 'TestRecursive' -count=1` and confirm failures are validation misses.
- [ ] Implement recursive schema compilation and value validation, rejecting unsupported or internally inconsistent schemas.
- [ ] Run the focused contract tests and confirm they pass.
- [ ] Add failing tests for duplicate/blank call IDs and successful tool results returning the wrong or empty identity.
- [ ] Run focused execution/live tests and confirm protocol identity failures.
- [ ] Implement `ValidateBatchIdentity`; rebind every executor result to the originating call before envelope creation; enforce identity before transcript append.
- [ ] Run `go test ./internal/agent/tool/contract ./internal/agent/tool/execution ./internal/agent/live -count=1`.

### Task 2: Dependency-Aware Batch Planner

**Files:**
- Create: `internal/agent/tool/execution/planner.go`
- Create: `internal/agent/tool/execution/planner_test.go`
- Modify: `internal/agent/tool/execution/execution.go`
- Modify: `internal/agent/live/registry_loop.go`

**Interfaces:**
- Consumes: validated `execution.Call` and contextual `contract.Descriptor` values.
- Produces: `type Stage struct { Calls []Call; Parallel bool }`.
- Produces: `BatchPlanner.Plan(api.Context, []Call) ([]Stage, error)`.

- [ ] Add a failing test with three reads where call one is independent but calls two and three write/read the same resource; assert calls two and three never overlap.
- [ ] Add tests proving independent reads fan out and all actions create provider-ordered sequential stages.
- [ ] Run `go test ./internal/agent/tool/execution -run 'TestBatchPlanner' -count=1` and confirm the pairwise case fails.
- [ ] Implement stage construction using all-pairs conflict checks and descriptor safety metadata.
- [ ] Replace `ExecuteAll` grouping with planner stages while preserving result order.
- [ ] Run execution and registry-loop tests.

### Task 3: Unified Turn State, Recovery, Reflection, And HITL

**Files:**
- Create: `internal/agent/turn/phase.go`
- Create: `internal/agent/turn/recovery.go`
- Create: `internal/agent/turn/interrupt.go`
- Create: `internal/agent/turn/final_response.go`
- Create: `internal/agent/live/turn_engine.go`
- Modify: `internal/agent/live/tool_loop.go`
- Modify: `internal/agent/live/registry_loop.go`
- Modify: `internal/agent/turn/state.go`
- Test: `internal/agent/turn/phase_test.go`
- Test: `internal/agent/turn/recovery_test.go`
- Test: `internal/agent/turn/interrupt_test.go`
- Test: `internal/agent/live/turn_engine_test.go`

**Interfaces:**
- Produces: closed `turn.Phase` transitions and request-local `turn.State` checkpoints.
- Produces: `InterruptHook.Review(context.Context, ActionRequest) (InterruptDecision, error)` with allow, deny, and pause outcomes.
- Produces: `RecoveryPolicy.Decide(RecoveryInput) RecoveryDecision`.
- Produces: `FinalResponseValidator.Validate(FinalResponseInput) error`.

- [ ] Add failing phase-transition tests that reject skips, regressions, and execution after terminal state.
- [ ] Implement the closed transition table and expose read-only state snapshots.
- [ ] Add failing tests for default-allow, denial-as-observation, paused checkpoints, and authorization revalidation on resume.
- [ ] Implement interrupt contracts and a default allow hook without changing current behavior.
- [ ] Add failing loop tests for invalid arguments corrected in a later call, failed tools degraded into prose, last-round observations receiving terminal synthesis, and one invalid final answer receiving one reflection correction.
- [ ] Implement `TurnEngine` and recovery policy; reserve terminal synthesis independently from tool rounds.
- [ ] Make existing `ToolLoop` constructors delegate to the engine and remove duplicate fresh-history/registry budget logic after parity tests pass.
- [ ] Run `go test ./internal/agent/turn ./internal/agent/live -count=1`.

### Task 4: Structured Long-Context Budgeting And Compaction

**Files:**
- Create: `internal/agent/assemble/context.go`
- Create: `internal/agent/assemble/context_test.go`
- Create: `internal/agent/turn/compaction.go`
- Create: `internal/agent/turn/compaction_test.go`
- Modify: `internal/agent/assemble/assemble.go`
- Modify: `internal/agent/live/conversation_context.go`
- Modify: `internal/agent/live/memory.go`
- Modify: `internal/config/agent_config.go`
- Modify: `cmd/zenbot/main.go`

**Interfaces:**
- Produces: `assemble.ContextUnit` with source, timestamp, fingerprint, priority, and complete payload.
- Produces: `assemble.ContextBudgeter.Project(ContextInput) (ContextProjection, error)`.
- Produces: `turn.MemoryCompactor.Compact(context.Context, CompactionInput) (CompactionResult, error)`.

- [ ] Add failing tests proving untrusted context is outside the system role and recent-room JSON remains valid under tiny budgets.
- [ ] Add failing tests for source-aware deduplication, newest-first retention, deterministic projection, and a configured 1,000,000-token ceiling.
- [ ] Implement semantic context units and token-estimate budgeting with reserved policy/request/manifest/output capacity.
- [ ] Move historical evidence and room rows into separate untrusted context messages; remove trip, hash, and creator identity from provider metadata.
- [ ] Add failing compaction tests for contiguous oldest-turn summaries, fingerprints, covered ranges, and summary replacement.
- [ ] Implement the compactor seam and persisted summary model; summaries remain untrusted and raw storage remains authoritative.
- [ ] Update configuration and composition for context tokens, reserve tokens, raw-turn threshold, and summary limits.
- [ ] Run `go test ./internal/agent/assemble ./internal/agent/turn ./internal/agent/live ./internal/config ./cmd/zenbot -count=1`.

### Task 5: Atomic Turn Persistence And Tool-Owned Delivery Memory

**Files:**
- Create: `internal/repository/agent_turn.go`
- Modify: `internal/repository/h2/agent_memory.go`
- Modify: `internal/agent/turn/memory.go`
- Modify: `internal/agent/live/runner.go`
- Modify: `internal/agent/runtime/contracts.go`
- Test: `internal/repository/h2/agent_memory_test.go`
- Test: `internal/agent/live/runner_test.go`

**Interfaces:**
- Produces: `repository.AgentTurnRecord` and `AgentTurnRepository.AppendAgentTurn(context.Context, AgentTurnRecord) error`.
- Produces: runtime results carrying delivery ownership and memory outcome without exposing raw internal evidence.

- [ ] Add a failing H2 integration test that forces evidence insertion failure and asserts the conversation pair is rolled back.
- [ ] Implement one transaction for conversation rows, action receipts, and reusable evidence.
- [ ] Add a failing live test proving successful `ROOM_DELIVERY` requests are persisted once even when runtime sends no second reply.
- [ ] Propagate tool-owned delivery receipts through completion/result and persist them after confirmed tool execution.
- [ ] Run repository, live, and runtime tests.

### Task 6: Runtime Deadlines And Keyed-Lock Reclamation

**Files:**
- Create: `internal/agent/runtime/keyed_lock.go`
- Create: `internal/agent/runtime/keyed_lock_test.go`
- Modify: `internal/agent/runtime/runtime.go`
- Modify: `internal/agent/runtime/contracts.go`
- Modify: `internal/config/agent_config.go`
- Modify: `cmd/zenbot/main.go`
- Test: `internal/agent/runtime/runtime_test.go`
- Test: `internal/agent/live/runner_test.go`

**Interfaces:**
- Produces: reference-counted `keyedLocker.Acquire(string) func()`.
- Adds: `runtime.Config.RequestTimeout time.Duration`.

- [ ] Add failing tests proving lock entries return to zero after unique room/whisper invocations.
- [ ] Implement reference-counted keyed locks without breaking per-memory-key serialization.
- [ ] Add failing tests where context loading and provider work exceed the request deadline and are cancelled.
- [ ] Apply the request deadline before context loading and propagate it through delivery/persistence.
- [ ] Replace the race-sensitive sentinel test synchronization with request-completion synchronization and rerun it under `-race -count=20`.
- [ ] Run `go test -race ./internal/agent/runtime ./internal/agent/live -count=1`.

### Task 7: Fuzzing, Documentation, And Completion Audit

**Files:**
- Create: `internal/agent/tool/contract/schema_fuzz_test.go`
- Modify: `internal/agent/llm/openai/client_test.go`
- Modify: `AGENTIC_ARCHITECTURE.md`
- Modify: `README.md`
- Modify: `config.example.toml`
- Modify: `.env.example`

**Interfaces:**
- Consumes all hardened public contracts; produces no new runtime API.

- [ ] Add fuzz targets asserting arbitrary schemas/arguments and arbitrary provider payloads never panic.
- [ ] Run each seed corpus and a bounded fuzz pass.
- [ ] Update architecture/configuration documentation to match the actual state machine, context roles, limits, HITL defaults, and persistence semantics.
- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test -race ./internal/agent/...`.
- [ ] Run `go vet ./...`, `go test ./...`, and `make check`.
- [ ] Re-read the specification and map every requirement to code and verification evidence before declaring completion.
