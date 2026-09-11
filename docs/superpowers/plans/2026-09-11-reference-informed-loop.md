# Reference-informed Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Improve factual tool contracts, context preservation and failure feedback without moving reasoning out of the model.

**Architecture:** Keep the existing native tool loop. Preserve typed command data at its producer, project SDK return schemas compactly, protect current feedback and bounded result previews, and report incomplete execution receipts without fabricating an answer.

**Tech Stack:** Go, existing JSON-schema contracts, existing command/snapshot gateway, existing provider evaluation.

**Spec:** docs/superpowers/specs/2026-09-11-reference-informed-loop-design.md

## Global Constraints

- The LLM owns tool selection, task planning, conditions, recovery and completion.
- No streaming, deterministic intent routing, semantic completion judge, fixed workflows, keyword-based decisions, or unrelated security work.
- Reference repositories are read-only. Work in the existing Zenbot master checkout; no push, deployment or bot restart.
- Preserve flat command arguments, existing room delivery, synchronous actions and non-retryable ACTION_OUTCOME_UNKNOWN.
- Tests exercise actual boundaries with external I/O replaced only at the network/session/provider seam.

### Task 1: Typed remote-list observations

**Files:** internal/listener/snapshot/{coordinator,list_operation}.go and tests;
internal/command/{agent_capture_engine,agent_gateway}.go and tests;
internal/agent/commandgateway/gateway.go; internal/agent/tool/{saturn_command,run_command}.go and tests.

**Interfaces:** Add optional `Data json.RawMessage` to `snapshot.OperationResult` and
`commandgateway.Execution`. A success command payload includes `data` only when present.
List data is `{room:string,users:string[],count:integer,returnedCount:integer,truncated:boolean}`.
Preserve all existing receipt fields and room messages. Use target room from operation context.

- [ ] Add failing tests: list snapshot with alice/bob/duplicate identity/nil entry yields exact full count two and users; empty snapshot yields count zero and empty users; gateway end-to-end result contains identical typed data and existing delivery receipts.

```go
var data struct { Room string; Users []string; Count, ReturnedCount int; Truncated bool }
if err := json.Unmarshal(result.Data, &data); err != nil { t.Fatal(err) }
if data.Room != "lounge" || data.Count != 2 || data.ReturnedCount != 2 || data.Truncated { t.Fatalf("data=%+v", data) }
```

- [ ] Run targeted tests and confirm expected missing-data failure.
- [ ] Preserve the source snapshot's typed values in `ListRoomOperation.Apply`; share roster dedup/order helper with formatting, not a text parser. Carry JSON through the command bridge and declare its schema. Clone mutable byte slices at boundaries.
- [ ] Run `go test ./internal/listener/snapshot ./internal/command ./internal/agent/tool` and inspect compatibility cases.
- [ ] Record tests and changed files for independent review. Do not commit other workers' files.

### Task 2: Model-facing result-shape hints

**Files:** internal/agent/tool/contract/manifest.go, new result_shape.go and tests as useful.

**Interfaces:** `ProviderDefinition()` derives a compact hint from existing `ManifestEntry.ResultSchema`; no new required public metadata. Optional fields clearly distinguished; schema stays authoritative.

- [ ] Add failing provider projection tests using a closed object schema with room/count/users, an optional field, nested array, and a large schema. Assert the actual emitted definition gives result shape while parameters remain unchanged.

```go
definition := entry.ProviderDefinition()
if !strings.Contains(definition.Description, "count:integer") { t.Fatal("result count shape missing") }
if !bytes.Equal(definition.Parameters, entry.Parameters) { t.Fatal("result fields leaked into arguments") }
```

- [ ] Run `go test ./internal/agent/tool/contract` and confirm absent return guidance fails.
- [ ] Implement deterministic sorted shallow schema projection, bounded to 512 bytes, explicit omission marker. Preserve valid UTF-8; do not serialize verbose schema descriptions or all validation keywords.
- [ ] Run contract and live tests; measure production manifest byte delta. Record exact tests for review.

### Task 3: Protected feedback and bounded factual result index

**Files:** internal/agent/live/turn_engine.go and registry_loop_test.go;
internal/agent/assemble/{observations,context}.go and tests;
resources/agent/system/system-policy.txt only if its existing index wording needs adjustment.

**Interfaces:** Retain `ObservationStore.Index(maxArgumentBytes)` receipt-only compatibility.
Introduce `IndexWithData(maxArgumentBytes, maxDataBytes int)` or equivalently scoped factual
index method. Previews contain structured data, error message and explicit truncation, never
derived task status. ProjectTurn uses bounded previews starting at 1024 bytes per result,
shrinking data and arguments to fit required policy/request/runtime/receipts.

- [ ] Add failing forced-budget tests where three tool outputs disappear from optional transcript but their small count facts remain in the required index, including zero and errors; full store is unchanged.
- [ ] Add a live-loop test with malformed response plus competing observations: next provider request contains current correction, unchanged original prompt and loop status. Current diagnostic is bounded even for huge IDs/names/arguments; rejected calls never execute.

```go
if !messagesContain(next.Messages(), "TOOL_LOOP_FEEDBACK=") { t.Fatal("repair feedback evicted") }
if !messagesContain(next.Messages(), original) { t.Fatal("original request lost") }
if !messagesContain(next.Messages(), `"count":3`) { t.Fatal("count unavailable for terminal synthesis") }
```

- [ ] Run targeted tests; verify baseline loses values/feedback as expected.
- [ ] Pass current feedback explicitly via required runtime messages, not prefix inference. Bound rejected-call diagnostic. Reuse observation projection for preview data; shrink previews before receipts; mark omissions and retain retrieval IDs.
- [ ] Run `go test ./internal/agent/assemble ./internal/agent/live ./internal/agent/tool/execution`. Preserve atomic native call/result transcript and terminal no-tools behavior.
- [ ] Record tests and files for independent review.

### Task 4: Evidence-based failed-turn notices

**Files:** internal/agent/live/runner.go or focused failure_reply.go and tests;
cmd/zenbot/main.go and relevant composition tests.

**Interfaces:** Add `live.FailureReply(cause error) string` as a pure bounded renderer of
IncompleteTurnError observations; generic message when no evidence. Production failure
sink uses it. It renders only receipt identity/status/effect counts, never raw cause,
arguments or full result content. Remains explicitly a failure, not successful completion.

- [ ] Add failing tests for committed, partial, unknown, rejected, skipped and mixed results through a wrapped IncompleteTurnError, and for plain error fallback. Include many receipts and hostile/huge call names to test bounded readable output without payload leakage.

```go
reply := FailureReply(fmt.Errorf("wrapped: %w", incomplete))
if !strings.Contains(reply, "incomplete") || !strings.Contains(reply, "unknown") { t.Fatalf("reply=%q", reply) }
if strings.Contains(reply, secretPayload) { t.Fatal("failure report leaked result body") }
```

- [ ] Run targeted tests and verify missing renderer/old generic behavior is the failure.
- [ ] Implement receipt-only notice bounded to 2000 runes with explicit omitted-receipt count. Preserve no-evidence generic fallback and context cancellation behavior. Never recommend full replay or claim user-goal success.
- [ ] Run `go test ./internal/agent/live ./cmd/zenbot` and independently review production integration.

### Task 5: Comparative report, integration verification and provider trials

**Files:** docs/2026-09-11-reference-loop-comparison.md;
docs/evals/2026-09-11-reference-loop-provider.md;
cmd/zenbot/provider_eval_test.go fixtures only as necessary to match new real list data;
AGENTIC_ARCHITECTURE.md and COMMAND_TOOL_INVENTORY.md relevant current behavior.

- [ ] Document all sources/revisions, pattern problem/mechanism/agency/existing solution/gain/cost/decision and documentation discrepancies. Distinguish observed harness correctness from model inference quality.
- [ ] Adjust synthetic provider list fixture to return actual production typed data; do not alter expected tool ordering/conditions or loosen assertions to pass.
- [ ] Run `go test ./...`, `go vet ./...`, and race tests for affected agent, command and snapshot packages. Inspect and fix regressions.
- [ ] Run opt-in configured-provider trials without real command side effects; retain full JSON traces and settings. Report failures honestly and repeat focused cases to characterize variance, not to erase a failed run.
- [ ] Independent final review for spec compliance, regression risk and unjustified complexity. Resolve load-bearing findings and rerun covering tests.
- [ ] Complete final before/after report and hand off the verified local changes. No push/deploy/restart.
