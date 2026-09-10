# Semantic Planner Removal Design

## Decision

Remove the semantic task planner, planned obligations, heuristic subject matching, and planner-driven tool filtering from Zenbot's production LLM tool loop.

One execution model will interpret the newest user request, choose tools, order calls, consume observations, and write the answer. A separate semantic completion gate remains as a bounded finalizer: it judges the proposed answer against the exact newest request and the actual tool-result ledger, then either accepts it or returns one actionable continuation instruction.

The deterministic controller remains authoritative only for facts it can prove without interpreting natural language.

This design supersedes the semantic task contract and deterministic reducer described in section 3 of `2026-09-10-agent-loop-correctness-design.md`. The safety, verified-outcome, routing-uniqueness, schema, and context-budgeting sections of that design remain in force.

## Why The Planner Must Be Removed

The current flow performs two independent semantic interpretations:

1. A planning model translates the user request into tool obligations, dependencies, and free-form subjects.
2. The execution model translates the same request into concrete tool calls and arguments.
3. A deterministic reducer guesses a subject from selected JSON keys and requires the two model interpretations to match exactly.

That reducer does not verify user intent. It verifies agreement between a planner's lossy representation and an argument-extraction heuristic. Valid calls can therefore be rejected even when the execution model selected the correct tool and arguments. The live `saturn_list` failure is one instance: the correct tool was repeatedly rejected as `TASK_OBLIGATION_MISMATCH` before execution.

The planner also adds a provider round trip, a second correction loop, extra context, and an additional failure point without providing deterministic semantic truth.

## Authority Boundary

### Model-owned semantics

The execution model owns:

- decomposing a compound request;
- selecting among caller-visible tools;
- choosing concrete arguments allowed by each tool schema;
- deciding the semantic order implied by words such as "after" and "then";
- calculating or combining tool outputs;
- deciding whether more tool calls are needed after receiving results;
- synthesizing one final response.

The completion model owns only the final semantic audit. It cannot execute tools, mutate state, rewrite tool results, or declare an action successful without evidence.

### Controller-owned mechanics

The deterministic controller continues to own:

- caller capability and contextual tool visibility;
- one authoritative model-facing tool per primary intent;
- provider schema validation and execution-time argument validation;
- call identity, tool identity, and registry membership;
- global, per-tool, step, failure, and observation budgets;
- duplicate-call detection;
- action review and authorization;
- synchronous action execution;
- read/action batch barriers and provider-emitted call order;
- verified business outcomes, committed-effect metadata, and delivery receipts;
- cancellation handling, including non-retryable `ACTION_OUTCOME_UNKNOWN`;
- bounded observation projection and context pruning;
- preservation of the exact newest request on every model call;
- syntactic validation of the final response.

The controller must not infer requested subjects, invent semantic dependencies, or reject a valid schema-conforming call because it disagrees with an LLM-authored plan.

## Runtime Data Flow

The production turn becomes:

1. Assemble the trusted system policy, optional bounded history, exact contextualized newest request, and caller-visible authoritative tool definitions.
2. Call the execution model directly. There is no planning request and no `submit_task_plan` pseudo-tool.
3. Validate every proposed call mechanically against the registry, capability policy, schema, budgets, duplicate ledger, and action-review policy.
4. Execute calls using the existing batch barriers. Actions execute synchronously. Record each typed result in the request-local ledger and bounded observation store.
5. Reproject the transcript for the next model call. Always retain the original newest request and actual observations. Expose only tools still available under deterministic policy and ledger budgets.
6. When the model proposes a final answer, send the completion gate the exact newest request, candidate answer, bounded conversation, currently available tool capabilities, and all current-turn typed results.
7. If the gate returns `FINAL`, run the existing output validator and complete the turn.
8. If the gate returns `CONTINUE`, append one structured, actionable semantic-feedback message and allow another ordinary execution-model cycle within the existing limits.
9. If no continuation budget remains, terminate with an explicit completion error. Never fabricate success.

## Immutable Intent And Evidence

The exact newest request remains the immutable semantic contract. `Assembler.ProjectTurn` already verifies that the newest request is present and places it in the required suffix; that invariant replaces the planner-authored objective hash and `TASK_STATE_JSON` message.

Actual tool calls and typed tool results are the evidence contract. The existing observation store remains request-local and is the source used for bounded model-visible observations. The completion gate receives the typed results directly, so it does not need planner-authored obligation IDs or inferred subjects.

No model-produced planning artifact is persisted as truth. Durable evidence rules remain unchanged: only approved reusable read-only evidence and verified final outcomes may be persisted.

## Tool Routing

The execution model receives the complete caller-visible authoritative inventory on the initial request. Subsequent requests receive the subset still mechanically available under the ledger and call budgets.

Tool routing remains constrained by registry design rather than a semantic planner:

- `room_users` is for the invocation's current room and takes no room argument.
- `saturn_list` is for another named room and receives that room in its command arguments.
- `saturn_ping` targets this bot runtime's fixed hack.chat connection and therefore needs no host argument.
- primary-intent uniqueness remains a catalog invariant, preventing two visible tools from claiming the same operation.

These distinctions belong in tool descriptions and the trusted system policy. They must not be duplicated as planner-only rules or enforced through free-form subject equality.

## Completion And Correction

`SemanticCompletionGate` remains the LLM finalizer. Its input is grounded in:

- the exact newest request;
- the candidate answer;
- actual current-turn typed results;
- bounded conversation context;
- available tool capability descriptions;
- whether another cycle is mechanically possible.

The gate may return:

- `FINAL`: the candidate satisfies the request given the evidence; or
- `CONTINUE`: the response is incomplete, with concise instructions describing the missing semantic work.

The controller validates this enum and bounds the correction by the existing step and call budgets. A continuation does not create an obligation record, alter history, or pre-authorize a tool. It is simply feedback to the execution model.

Mechanical failure recovery remains separate from semantic completion. Structured tool errors are passed through the bounded observation view. Raw stack traces are never added to model context. `ACTION_OUTCOME_UNKNOWN` remains terminal and non-retryable; neither the execution model nor completion gate can authorize a second action attempt in that turn.

## Code Removal And Interface Changes

Use commit `7fbb842` as the provenance map for the semantic planner, but do not apply a raw whole-commit revert. That commit also introduced `Ledger.Available` and projection plumbing later consumed by authoritative routing and per-cycle observation budgeting. Remove only planner/obligation behavior, then adapt the retained projection APIs to operate without `TaskState`.

Remove:

- `internal/agent/live/task_planner.go` and its tests;
- `internal/agent/turn/task_contract.go` and its tests;
- `TaskPlanner`, `SemanticTaskPlanner`, and `ObjectiveOnlyTaskPlanner`;
- `ToolLoop.Planner` and `NewRegistryToolLoopWithPlanner`;
- `TurnEngine.Task` and every obligation preflight, observation, ready-frontier, and completion branch;
- planner-derived `PlanningTool` construction;
- `TASK_STATE_JSON`, `TASK_OBLIGATION_FEEDBACK`, `TASK_OBLIGATION_MISMATCH`, and `TASK_DEPENDENCY_PENDING` handling;
- `toolsForPending` and heuristic `subjectFromArguments` logic.

Simplify:

- production composition to construct `NewRegistryToolLoop` directly with the existing semantic completion gate;
- turn projection APIs so they accept the observation store and immutable newest request, but no task state;
- tool-loop startup so the first provider call is the execution call, not a planning call;
- tests that currently assert planner construction or obligation state.

Retain:

- `SemanticCompletionGate` and its constrained `FINAL`/`CONTINUE` contract;
- `turn.State`, `PhaseMachine`, `RecoveryPolicy`, `FinalValidator`, `BatchPlanner`, execution ledger, and observation store;
- authoritative tool registry and primary-intent validation;
- synchronous action execution and receipt verification;
- current-room versus remote-room routing guidance in system/tool descriptions.

## Failure Handling

- Invalid or unauthorized calls become structured tool observations and consume the configured failure/call budgets.
- Definite read failures may be retried only through the existing bounded recovery policy.
- Definite action failures follow descriptor idempotency and recovery policy.
- Unknown action outcomes are never retried and force terminal uncertainty synthesis.
- A completion-gate `CONTINUE` response cannot expand permissions or budgets.
- A malformed completion-gate response fails closed.
- Context projection failure stops before contacting the provider.
- Hitting the final step limit returns a clear error instead of claiming the request was completed.

## Verification Strategy

Implementation follows red-green-refactor.

Required regression coverage:

1. Production composition contains no planner and makes no `submit_task_plan` provider call.
2. The exact live-shaped request—count lounge, count programming, multiply, kick a named user, then summarize once—can execute both presence calls and the ordered action without a task-obligation rejection.
3. `room_users` and `saturn_list` remain distinct and correctly described for current-room versus remote-room use.
4. A three-or-more-cycle turn retains the exact newest request on every projection and exposes bounded, valid observation JSON.
5. Completion cannot be accepted merely because a tool returned HTTP success; typed business failure, missing commit metadata, or missing delivery receipt remains failure evidence.
6. Completion feedback produces a bounded continuation without overwriting prior user, assistant, call, or result history.
7. Action cancellation still produces non-retryable `ACTION_OUTCOME_UNKNOWN`, with no duplicate action execution.
8. Catalog tests continue to prove one visible owner per primary intent and schema/encoder parity.

Final verification requires focused affected-package tests, `go test -race ./internal/agent/...`, `go vet ./...`, `go test ./...`, `make check`, and `git diff --check`.

## Non-Goals

- Replacing the semantic completion gate with deterministic keyword matching.
- Adding another planner, DAG generator, subject extractor, or prompt classifier under a different name.
- Letting the execution model bypass schemas, capability checks, action review, receipts, or budgets.
- Guaranteeing that an LLM always interprets natural language correctly. The architecture instead removes false deterministic rejection and relies on grounded final semantic review.
- Changing public Saturn command syntax, persistence schema, or unrelated command behavior.
