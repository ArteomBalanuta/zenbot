# AgentMentionParser Bounded Acceptance

## Decision

**ACCEPTED for row #56 `AgentMentionParser`, limited to the bounded contract-only reconciliation.** This acceptance records the independently QA-approved parser contract result; it is not migration closure and does not change the frozen closure interpretation of the governing documents.

**Full Saturn → Zenbot migration remains incomplete (`NOT COMPLETE`).** No other audit row, SQL occurrence, repository/service method, subsystem, or migration gate is accepted by this handoff.

## Evidence

### Authoritative Saturn source and tests inspected

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/agent/room/AgentMentionParserTest.java`
- `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/agent/routing/AgentGapContractRedTest.java` (inspected for the bounded task; it supplied no additional parser behavior)

### Zenbot target files

- `internal/agent/participation/policies.go` — existing `MentionParser.Parse(string, string) (string, bool)` implementation and minimal boundary logic.
- `internal/agent/participation/policies_test.go` — source-derived parser parity and boundary regressions.

### Handoffs reviewed

- `.hermes/handoffs/agent-mention-parser-architecture.md`
- `.hermes/handoffs/agent-mention-parser-implementation.md`
- `.hermes/handoffs/agent-mention-parser-qa.md`

### QA artifact and executed evidence

The acceptance is based on `.hermes/handoffs/agent-mention-parser-qa.md`, whose verdict is **PASS for the bounded AgentMentionParser contract slice**. Its recorded evidence includes:

- `go test ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1` — PASS
- `go test -race ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1` — PASS
- `./mvnw -q -Dtest=org.saturn.app.agent.room.AgentMentionParserTest test` from Saturn — PASS
- `go test ./...` — PASS
- `go test -race ./...` — PASS; the QA artifact records the existing macOS `internal/agent/sql.test` malformed `LC_DYSYMTAB` linker warning with exit status 0
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `gofmt -l .` — PASS, no files listed
- `git diff --check` — PASS

The accepted behavior is limited to the source-backed literal `@` mention recognition, Unicode/case-insensitive matching, evidenced preceding/following boundaries, all-match removal, and the documented cleanup ordering and punctuation behavior.

## Changed-file scope

The implementation/QA evidence attributes the parser slice to:

- `internal/agent/participation/policies.go`
- `internal/agent/participation/policies_test.go`
- `.hermes/handoffs/agent-mention-parser-architecture.md`
- `.hermes/handoffs/agent-mention-parser-implementation.md`
- `.hermes/handoffs/agent-mention-parser-qa.md`

This acceptance handoff adds:

- `.hermes/handoffs/agent-mention-parser-acceptance.md`

The governing documents `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` were inspected but intentionally left unchanged because the user-directed frozen closure audit/history must not be broadly or unsafely rewritten. No application source was changed by this acceptance task.

## Explicit exclusions

This acceptance does **not** include full agent integration or runtime wiring, routing/engine/listener changes, providers, transport, remote-room or Whiskey work, SQL policy or persistence work, row #324 B/C, row #325, unrelated migration rows, SQLite elimination, or overall migration closure. Saturn remains read-only. Existing unrelated dirty, staged, and untracked target files were preserved.
