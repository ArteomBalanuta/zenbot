# Stage A/B tool contract and execution implementation handoff

## Outcome
Implemented the bounded, provider-neutral Stage A/B slice described by `agent-tool-execution-architecture.md`. The implementation is invocation-level only and does not claim full tool/turn/router migration.

## Files created
- `internal/agent/tool/contract/schema.go` — schema constructors, strict schema/argument/result validation, primitive/structured types, required and closed-object checks, enum, Unicode length, integer and numeric bounds, recursive canonical JSON, success/error envelopes.
- `internal/agent/tool/contract/definition.go` — immutable-by-convention descriptors, definitions, results, stable error fallback, defensive JSON/slice copies.
- `internal/agent/tool/contract/contract_test.go` — schema, canonical ordering, and envelope tests.
- `internal/agent/tool/tool.go` — explicit tool interface, nonblank string argument reader, contextual allowlist registry, deterministic definitions.
- `internal/agent/tool/execution/execution.go` — call conversion/keying, synchronized request-local ledger, cancellation wrapper, executor admission/authorization/schema validation/result normalization, resource policy, contiguous parallel-read scheduler preserving provider order.
- `internal/agent/tool/execution/execution_test.go` and `execution_extra_test.go` — fake-tool tests for parallel ordering/barriers, ledger duplicate/limit/failure state, authorization, invalid arguments, nil/exception normalization, timeout/cancellation, and resource policy.

## Behavior implemented
- Descriptor construction rejects invalid lowercase names, missing metadata/negative-use guidance, invalid timeout, malformed parameter/result schemas, and retains copied JSON/slices.
- Parameter schemas require an object root; supported values are `any`, string, boolean, number, integer, object, array, and null. Required fields, closed objects, type checks, enum, Unicode length, integer-ness, minimum, and maximum are enforced without coercion.
- Result values are checked against declared result schemas.
- Canonical invocation keys recursively sort object keys while preserving array order.
- Definitions are filtered by contextual allowlist and sorted by tool name. Registry and descriptor access are separate so `TOOL_NOT_ALLOWED` and `UNKNOWN_TOOL` remain distinguishable.
- Only read-only, idempotent, prerequisite-free tools with known read resources are parallel candidates. Writes/actions/dependencies and resource conflicts are barriers; parallel result slices retain provider order and sibling failures do not suppress other calls.
- Ledger state is request-local and mutex-protected; canonical duplicates, per-tool limits, and repeated failures are represented by stable codes.
- Executor checks allowlist, registry, descriptor contract, capabilities, arguments, deadlines, cancellation, result validity, nil results, and panics. Model-visible infrastructure failures use safe stable messages and do not expose exception text.

## Verification actually run
- Focused: `go test ./internal/agent/tool/...` — PASS.
- Focused race: `go test -race ./internal/agent/tool/...` — PASS.
- Execution-only after priority test additions: `go test ./internal/agent/tool/execution -count=1` — PASS.
- Execution-only race: `go test -race ./internal/agent/tool/execution -count=1` — PASS.
- Earlier broad verification before the focused test additions: `go test ./...` — PASS; `go test -race ./...` — PASS; `go vet ./...` — PASS; `go build ./...` — PASS; `git diff --check` — PASS.

## Explicit exclusions and limitations
- No concrete database/history/SQL, command/moderation/action, listener, provider, persistence/memory, freshness, turn-state, or router integration was added.
- No Saturn source was modified.
- Turn state remains an explicit future seam; workers do not mutate turn state because no turn package exists in this slice.
- The registry currently takes a frozen-by-convention tool slice and allowlist; a richer contextual availability policy can be added at the future turn/router boundary.
- Result validation operates on provider-neutral JSON content. Provider-specific wire parity and golden serialization were not asserted.
- This is contiguous batching, not arbitrary DAG scheduling.
- Existing unrelated dirty and untracked worktree changes were preserved; only the new `internal/agent/tool/` tree was task-owned.
