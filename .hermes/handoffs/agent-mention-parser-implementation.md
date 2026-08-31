# Agent Mention Parser Contract Implementation

## Scope and status

This is a contract-only reconciliation of the existing `MentionParser.Parse(string, string) (string, bool)` in `internal/agent/participation/policies.go`. It does **not** claim overall Saturn migration completion.

## Exact Saturn semantics

Authoritative sources inspected:

- `src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`
- `src/test/java/org/saturn/app/agent/room/AgentMentionParserTest.java`
- `src/test/java/org/saturn/app/agent/routing/AgentGapContractRedTest.java`

Saturn's parser:

1. Returns empty for blank input or blank bot nick.
2. Builds a Unicode/case-insensitive pattern with a literal `@`, `Pattern.quote(botNick.strip())`, a preceding boundary excluding Unicode letters, Unicode numbers, and `_`, and a following boundary excluding Unicode letters, Unicode numbers, `_`, and `-`.
3. Finds mentions case-insensitively and removes **all** matches, replacing each with one space.
4. Applies cleanup in this order: `strip`; remove leading `[\\s,;:.-]+`; remove whitespace before `[?!.,]`; remove terminal `,?` to `?` or terminal `,!` to `!`; then returns empty when the body is blank.
5. Therefore exact mentions require literal `@`, partial mentions are rejected by the evidenced boundaries, nick regex metacharacters are literal, and multiple mentions are removed.
6. `AgentGapContractRedTest` concerns unrelated routing projector/router hooks; it supplied no additional parser behavior and was not changed.

## RED/GREEN evidence

After adding `TestMentionParserSaturnParity` to `internal/agent/participation/policies_test.go`, before production edits:

```text
gofmt -w internal/agent/participation/policies_test.go
go test ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1
```

RED: failed as expected on Unicode preceding-letter boundary, prose comma cleanup, all-mention removal, and trailing `,?` / `,!` cleanup. The failures demonstrated the existing implementation did not satisfy the source-backed contract.

After the minimal implementation change:

```text
gofmt -w internal/agent/participation/policies.go internal/agent/participation/policies_test.go
go test ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1
go test -race ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1
```

GREEN: both commands passed.

## Implementation

The existing API and owner were retained. The parser now uses rune scanning to model Saturn's Unicode boundaries (Go's RE2 does not support the Java lookarounds used by the source), `strings.EqualFold` for case-insensitive literal nick matching, and span replacement with one space for every recognized mention. Cleanup mirrors the Saturn ordering, including terminal `,?` and `,!` normalization.

## Changed files

- `internal/agent/participation/policies.go` — `MentionParser.Parse` and one small unexported boundary helper only.
- `internal/agent/participation/policies_test.go` — source-derived parity regression test only.
- `.hermes/handoffs/agent-mention-parser-implementation.md` — this handoff.

## Exclusions and preservation

No new parser package was created. No Pipeline, caller, routing, engine, listener, wiring, transport, provider, remote-room, SQL, row #324 B/C, or row #325 work was performed. Saturn source was read-only and was not modified. Existing unrelated dirty/staged/untracked target files were not intentionally touched.

## Verification commands and actual results

All requested gates passed:

- `go test ./...` — PASS
- `go test -race ./...` — PASS (the run emitted an existing macOS linker warning for `internal/agent/sql.test`: malformed `LC_DYSYMTAB`; exit status was 0)
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `gofmt -l .` — PASS (no files listed)
- `git diff --check` — PASS

Scope attribution checks:

- Zenbot target status shows pre-existing/unrelated `internal/agent/participation/invocation.go` staged addition; only `policies.go` and `policies_test.go` are the parser files touched by this slice, plus this untracked handoff.
- Saturn `git status --short` shows only its pre-existing dirty files (`.saturn_*`, registry/weather/service docs/source/tests, and other untracked diagnostics); the authoritative parser source/test paths are not listed as modified.

## Unresolved limitations

- The implementation is bounded to the exact parser semantics evidenced by the Saturn source/tests and the existing focused Go test; unspecified behavior is not expanded.
- `strings.EqualFold` is the Go equivalent used for case-insensitive literal matching; exact Java regex Unicode case-fold corner cases beyond the tested contract are not independently characterized.
- This document does not assert overall migration completion.
