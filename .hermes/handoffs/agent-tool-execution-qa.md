# Stage A/B Agent Tool Execution QA

## Verdict

**PASS for the bounded Stage A/B pure-contract/execution scope after the fixes below.** The implementation is not full Saturn tool/turn migration and does not claim provider, turn, persistence, command, SQL, moderation, or live-router parity.

## Source and test basis

Compared target files under `internal/agent/tool` and `internal/agent/tool/contract` / `execution` with the read-only Saturn sources:

- `src/main/java/org/saturn/app/agent/api/AgentTool.java`
- `AgentToolDescriptor.java`, `AgentToolResult.java`, `ToolResponseEnvelope.java`
- `tool/contract/AgentToolSchemaValidator.java`, `AgentToolSchemas.java`
- `tool/execution/AgentToolCallValidator.java`, `AgentToolExecutionPolicy.java`, `AgentToolExecutionLedger.java`, `AgentToolCallScheduler.java`, `AgentToolExecutor.java`, `AgentToolInvoker.java`
- Focused Saturn tests including `AgentToolSchemaValidatorTest`, `AgentToolDescriptorTest`, `AgentToolResultTest`, `ToolResponseEnvelopeTest`, `AgentToolArgumentReaderTest`, `AgentToolCallValidatorTest`, `AgentToolExecutionPolicyTest`, `AgentToolExecutionLedgerTest`, `AgentToolCallSchedulerTest`, and `AgentToolExecutorTest`.

Target tests inspected: `internal/agent/tool/contract/contract_test.go`, `internal/agent/tool/execution/execution_test.go`, and `execution_extra_test.go`.

## Findings and fixes

1. **Result object required fields were not enforced (fixed).** `ValidateResult` checked the result type but omitted Saturn's required-field check for object results. Added the required-field check and regression test.
2. **Descriptor metadata was less strict than Saturn (fixed).** Blank/whitespace label, description, category, and negative-use guidance were accepted. Added trimmed nonblank validation and regression coverage.
3. **Null and blank argument semantics differed (fixed).** JSON `null` unmarshaled into a nil Go map and was accepted; blank argument text was rejected rather than treated as `{}`. `ValidateArguments` now rejects nil/non-object values and treats blank input as an empty object, matching the Saturn call validator boundary.
4. **Prerequisites were ignored (fixed).** Executor contained a no-op loop over required successful tools. Added synchronized ledger success tracking, prerequisite admission, and regression coverage proving failed prerequisites do not unlock dependents.
5. **Tool-returned failures were not counted (fixed).** Error results did not increment the failure ledger, so repeated expected failures could not disable a tool. Error and invalid-result paths now record failures; successful execution records successful tool names.
6. **Already-cancelled calls could start (fixed).** Executor now checks the batch context before descriptor/execution and returns `TOOL_BATCH_CANCELLED` or `TOOL_BATCH_DEADLINE` without invoking the tool.
7. **Per-tool timeout was only cooperative (fixed within Go limits).** Invocation now runs behind a result channel and returns on context cancellation/deadline, so tools that ignore context cannot block the caller past the configured timeout. The worker goroutine cannot be forcibly killed in Go and may continue if the implementation ignores cancellation; this is documented as a limitation.
8. **Descriptor policy and ledger input boundaries were loosened (fixed).** Access/effect/result-mode values are now closed-set checked, policy slices are copied and sorted deterministically, and ledger limits are copied at construction.

RED/GREEN evidence: the newly added required-result, whitespace-guidance, prerequisite/failure-ledger, and pre-cancel tests were run before fixes and failed for the expected missing behavior. After the production fixes, focused normal and race suites passed.

## Verified behavior

- Lowercase identifier shape, nonblank descriptor metadata/guidance, timeout validation, schema validation, and defensive copies of JSON/slices.
- Parameter object-root validation; supported primitive/structured/null/any types; required and closed-object checks; enum, Unicode rune length, integer-ness, and numeric bounds without coercion.
- Result type and object-required-field validation.
- Recursive canonical JSON object-key sorting with array-order preservation.
- Fixed success/error envelope shape and safe infrastructure error messages.
- Nonblank JSON string argument reader behavior.
- Allowlist versus unknown distinction (`TOOL_NOT_ALLOWED` before registry lookup, `UNKNOWN_TOOL` for an allowed-but-missing name).
- Capability checks, descriptor contract failures, canonical duplicate detection, per-tool limits, repeated-failure disabling, and request-local mutex protection.
- Only read-only/idempotent/prerequisite-free tools with known read resources are parallel candidates. Resource conflicts, writes, actions, prerequisites, and unknown resource metadata are barriers. Contiguous batches continue after a sibling failure and results retain provider order.
- Cancellation, deadline, per-tool timeout, interruption/error/panic/nil-result normalization, result-schema failures, and no raw exception text in infrastructure failures.

## Commands actually run

All commands ran in `/Users/ab/workspace/go-projects/zenbot`:

- `gofmt -w internal/agent/tool/contract/schema.go internal/agent/tool/contract/definition.go internal/agent/tool/execution/execution.go internal/agent/tool/contract/contract_test.go internal/agent/tool/execution/execution_extra_test.go` — PASS.
- `go test ./internal/agent/tool/... -count=1` — PASS.
- `go test -race ./internal/agent/tool/... -count=1` — PASS.
- `go test -count=1 ./...` — PASS.
- `go test -race ./...` — PASS.
- `go vet ./...` — PASS.
- `go build ./...` — PASS.
- `git diff --check` — PASS.

## Changed files

Production:
- `internal/agent/tool/contract/schema.go`
- `internal/agent/tool/contract/definition.go`
- `internal/agent/tool/execution/execution.go`

Tests:
- `internal/agent/tool/contract/contract_test.go`
- `internal/agent/tool/execution/execution_extra_test.go`

No Saturn files were modified.

## Limitations and explicit exclusions

- No full provider-specific definition/result wire-golden parity claim; Go uses provider-neutral JSON.
- Registry construction is a frozen-by-convention slice/map boundary, not Saturn's full contextual registry/catalog implementation; duplicate registration rejection and rich contextual availability are not claimed.
- Descriptor does not yet model Saturn's `whenToUse` or typed examples; Stage A implementation intentionally retained only the fields needed by the accepted provider-neutral boundary.
- Go cannot forcibly terminate a tool goroutine that ignores context after timeout/cancellation.
- This is contiguous batching, not arbitrary DAG scheduling.
- No concrete database/history/SQL, command/moderation/action, persistence/memory, listener, provider, turn/freshness, router, or live application integration was implemented or validated here.
- No claim of full tool/turn migration, full Saturn migration, or production activation.

## Worktree preservation

The target worktree was already broadly dirty/untracked before this QA. Only the five listed tool files were intentionally changed by this task, plus this QA handoff. The read-only Saturn worktree was inspected and not modified; its pre-existing dirty/untracked files remained unchanged.
