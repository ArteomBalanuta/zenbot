# Durable tool-evidence QA — accepted with hardening

## Verdict

Accepted for the stated durable public tool-evidence vertical after one boundary repair.

## Independent QA evidence

- Inspected the architecture and implementation handoffs, live H2 repository/store, turn memory, assembler, bounded tool loop, runner/runtime, direct command path, command registration, and main composition.
- Verified the H2 implementation uses the exact immutable memory key, `expires_on > now`, newest-window selection with chronological presentation, bound SQL values, and cleanup-plus-insert transaction semantics in its separate `agent_tool_memory` repository.
- Verified public-only historical load/append and projection: whispers load no evidence, receive no historical prompt section, and append no evidence. The section is tagged `HISTORICAL_TOOL_EVIDENCE_UNTRUSTED_DATA`, carries tool/timestamp/data only, precedes current-room context, and does not use the blocked legacy prefix.
- Verified the loop remains one executed call / at most two completions. Candidate evidence is created only after the second successful tools-disabled completion and travels in request-local `runtime.Result` / `runtime.DirectCompletion`; no mutable invoker pending state exists.
- Verified normal runtime persistence happens only after a successful sink delivery, and direct `l` persistence only after `SendChatMessage` succeeds. Silent/no-reply, blank/error, cancellation, sink/send failure, and shutdown paths do not reach evidence append; post-delivery failures are logged without retry or redelivery.

## Repair made

The initial implementation exposed public `PersistableEvidence` fields but only validated candidates at the tool-loop construction point. A caller holding `runtime.Result` or `runtime.DirectCompletion` could therefore supply an unknown tool or schema-invalid JSON to the append seam; loaded valid-JSON malformed records could also be projected.

Hardened `internal/agent/turn/memory.go` with one frozen-schema validation boundary shared by candidate creation, append, and historical load:

- admits only `user_message_history` or `room_users`;
- requires the matching result schema, nonblank valid JSON, and a <=32 KiB row cap;
- preserves descriptor checks for `MODEL_DATA`, read-only, idempotent, no writes, reads present, exact executed tool name, success, and descriptor result schema;
- skips malformed/unknown/schema-invalid historical rows before any prompt projection.

Added RED→GREEN regression coverage in `internal/agent/turn/tool_evidence_qa_test.go`; the first run failed because `unknown_tool` was accepted, then passed after the repair. Updated the direct persistence fixture to use an actual valid `room_users` result shape.

## Final gates

Passed:

```text
go test ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/assemble ./internal/agent/runtime ./internal/command ./cmd/zenbot -count=1
go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` remains independently blocked by the known unrelated copylocks warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions retained

No schema migration, dynamic SQL, moderation/router changes, provider-visible tools, loop expansion, retries/queues/shared evidence state, protected-document edits, commits, or pushes.
