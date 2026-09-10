# Agent Harness Hardening Checklist

This checklist records the production behavior implemented by the September 2026 agent-harness refactor. The detailed design and implementation plan are under `docs/superpowers/`.

## Turn And Recovery Safety

- [x] Request-local phase machine with closed transitions in `internal/agent/turn/phase.go`.
- [x] Generalized `MODEL -> PLAN -> GATE -> EXECUTE -> OBSERVE` loop in `internal/agent/live/turn_engine.go`.
- [x] Aggregate step, tool-call, per-tool, and failure bounds.
- [x] Independent tool-free terminal synthesis after the last legal tool round.
- [x] One bounded final-response reflection; no recursive reflection loop.
- [x] Tool failures become structured observations for model correction.
- [x] Recovery policy removes tools for disabled/terminal failures instead of permitting churn.
- [x] Whole-request, provider, and per-tool deadlines propagate through `context.Context`.

## Tool Protocol And Planning

- [x] Nonblank unique provider call IDs are required before execution.
- [x] Every result is rebound to the originating call ID and tool name.
- [x] Argument and result schemas are validated recursively.
- [x] Unsupported schema keywords fail contract construction instead of drifting silently.
- [x] Provider definitions include label, category, access, effect, result mode, idempotency, capabilities, prerequisites, resource metadata, and negative-use guidance.
- [x] Provider-order sequential execution is the default.
- [x] Parallel stages contain only adjacent, pairwise-compatible, read-only, idempotent calls.
- [x] Actions, commands, prerequisites, unknown tools, and descriptor failures are ordering barriers.
- [x] Tool panics, validation failures, timeouts, and ordinary execution errors are isolated at the SDK boundary.

## Human Interruption

- [x] Injectable allow/deny/pause hook for state-changing calls.
- [x] Default policy preserves existing authorized behavior.
- [x] Denial becomes an `ACTION_DENIED` observation without executing the tool.
- [x] Pause occurs before side effects and returns a typed pending action.
- [x] Resume token, call identity, current registry, schema, and caller capabilities are revalidated before execution.

## Context And Memory

- [x] Context ceilings support up to 1,000,000 estimated tokens.
- [x] Tool-manifest size and output reserve count against the context budget.
- [x] Pruning removes complete semantic units and never slices JSON.
- [x] Source-aware fingerprints deduplicate repeated context.
- [x] System policy and newest request are retained; optional units are priority/freshness ranked.
- [x] Room rows, durable history, summaries, and historical evidence remain outside the system role.
- [x] Raw trip, hash, creator trip, and room-user snapshots are absent from provider system metadata.
- [x] Oldest complete turns are compacted after `memoryRawTurns`.
- [x] Summaries are tool-free, output-bounded, explicitly untrusted, and persisted with a fingerprint and covered-row cursor.
- [x] Raw conversation rows remain authoritative until normal TTL expiry.
- [x] Public room memory is shared; whisper memory remains identity-isolated.

## Persistence And Runtime

- [x] User message, assistant outcome, and reusable evidence commit in one H2 transaction.
- [x] Evidence failure rolls back the entire turn.
- [x] Successful tool-owned room delivery is remembered without sending duplicate prose.
- [x] Reference-counted memory-key locks are reclaimed after the last waiter.
- [x] Shutdown cancellation is separated from reply-required execution failures.
- [x] Real endpoint and credential values remain in ignored production configuration; tracked examples contain sanitized defaults.

## Verification Evidence

- [x] Recursive contract and provider decoder fuzz targets complete bounded fuzz runs without panics.
- [x] `go test -race ./internal/agent/... -count=1` passes.
- [x] `go vet ./...` passes.
- [x] `go test ./... -count=1` passes, including real H2 integration tests.
- [x] `make check` passes.
