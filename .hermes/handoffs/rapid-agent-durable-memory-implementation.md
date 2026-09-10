# Durable bounded H2 memory implementation

## Delivered
- Added resolved `[agent]` memory bounds: `memoryTurns` default 6 (1..60) and `memoryTtlMinutes` default 1440 (1..525600), with `MemoryTTL` and `config.example.toml` documentation.
- Added narrow `repository.AgentMemoryRepository` and real-H2 `agent_memory` implementation. Loads bind the exact `MemoryKey`, use strict expiry, newest bounded pairs followed by chronological prompt ordering; appends clean expired rows and insert user/assistant records in one transaction. `agent_tool_memory` is untouched.
- Added `live.PersistentMemoryStore` with injected clock/repository and atomic `AppendExchange` dispatch from `turn.TurnMemory`; no pending shared mutable exchange exists.
- Both direct and runtime agent paths load durable history before context/request construction. Assembly keeps the existing `system -> history(memory) -> current prompt` ordering, and tool completion two extends the first prepared request without reloading memory.
- Runtime persistence occurs only after `Sink.Deliver` succeeds. Direct `l` persistence occurs only after `SendChatMessage` succeeds. No-reply/blank/provider/tool/cancellation/delivery failure do not persist; persistence errors are logged and never redeliver.
- Enabled composition uses the existing shared H2 database. Disabled branches return before memory construction.

## TDD evidence
- RED config: `go test ./internal/config -run 'TestAgentMemoryConfiguration' -count=1` initially failed because `MemoryTurns`/`MemoryTtlMinutes` were absent.
- GREEN config: passed.
- RED real-H2 repository: `go test ./internal/repository/h2 -run 'TestAgentMemoryRepository' -count=1` initially failed because `LoadAgentMemory`/`AppendAgentMemory` were absent.
- GREEN real-H2 repository: passed.

## Verification
- Focused/config/repository/live/runtime/command/main tests: passed.
- `go test -race ./internal/repository/h2 ./internal/agent/turn ./internal/agent/live ./internal/agent/runtime -count=1`: passed.
- `go test ./...`: passed.
- `go build ./...`: passed.
- `git diff --check`: passed.
- `go vet ./...`: blocked only by pre-existing unrelated `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.

## Scope/exclusions
No schema/index changes were needed; the existing H2 schema/table/indexes supported real-H2 ordering and expiry tests. No tools, SQL routing, tool evidence persistence, moderation, router changes, or protected-document edits were introduced.
