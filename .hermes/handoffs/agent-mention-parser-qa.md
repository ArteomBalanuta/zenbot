# Agent Mention Parser Reconciliation QA

## Verdict

**PASS for the bounded AgentMentionParser contract slice.** The final Go implementation matches the inspected Saturn parser semantics for the covered contract, the focused Saturn test passes, all requested Go gates pass, and the task-owned scope is limited to the authorized parser files plus this QA handoff.

**Overall migration remains incomplete.** This QA result does not claim full Saturn migration, routing/engine/listener wiring, or any excluded migration rows.

## Sources inspected

- Zenbot: `internal/agent/participation/policies.go`
- Zenbot: `internal/agent/participation/policies_test.go`
- Saturn: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/agent/room/AgentMentionParser.java`
- Saturn: `/Users/ab/workspace/projects/saturn/src/test/java/org/saturn/app/agent/room/AgentMentionParserTest.java`
- Implementation handoff: `.hermes/handoffs/agent-mention-parser-implementation.md`
- Architecture handoff: `.hermes/handoffs/agent-mention-parser-architecture.md`
- Unblock analysis: `.hermes/handoffs/saturn-migration-unblock-analysis.md`

The implementation handoff listed an incorrect Saturn path prefix in its prose (`com/maistra/...`); the actual authoritative files were located under `org/saturn/app/agent/room/...` and were inspected directly.

## Contract verification

| Contract point | Evidence and result |
|---|---|
| Blank text / blank nick | Go retains the early `TrimSpace` rejection; Saturn returns `Optional.empty()` for null/blank inputs. **PASS.** |
| Mandatory literal `@` | Go scans only candidates beginning with literal `@`; the regression `Bot, explain this?` is rejected. **PASS.** |
| Preceding boundary | Go rejects a preceding Unicode letter, Unicode number, or `_`, matching Saturn `(?<![\\p{L}\\p{N}_])`. **PASS.** |
| Following boundary | Go rejects a following Unicode letter, Unicode number, `_`, or `-`, matching Saturn `(?![\\p{L}\\p{N}_-])`. **PASS.** |
| Unicode number coverage | An initial regression exposed `unicode.IsDigit` as too narrow for Saturn `\\p{N}`: `²@korin` and `@korin²` incorrectly matched. The helper now uses `unicode.IsNumber`; the regressions pass. **PASS after fix.** |
| Case-insensitive literal nick | Go uses `strings.EqualFold`; Saturn uses `(?iu)` plus `Pattern.quote`. Tested mixed-case matching passes. Nick regex metacharacters are treated literally. **PASS for covered contract.** |
| All-match removal | Go collects and removes every recognized mention, replacing each span with one space; multiple-mention regression passes. **PASS.** |
| Cleanup order | Go performs trim, leading `[\\s,;:.-]+` removal, whitespace-before-punctuation cleanup, terminal `,?`/`,!` normalization, then final trim/blank check, matching Saturn source order. **PASS.** |
| Punctuation quirks | Existing and added regressions cover address punctuation, prose comma cleanup, terminal `,?`, and terminal `,!`; all pass. **PASS.** |
| Empty body | `@korin` returns `("", false)` after cleanup, matching Saturn’s blank result. **PASS.** |
| Multiple mentions | `@korin hello @KORIN` yields `("hello", true)`. **PASS.** |
| API compatibility | Existing `MentionParser.Parse(string, string) (string, bool)` and `MentionParser` owner remain unchanged; no new package or wiring was introduced. Existing package/full tests compile and pass. **PASS.** |

## Regression and gate evidence

A RED run was obtained before the production fix:

- `go test ./internal/agent/participation -run '^TestMentionParserSaturnParity$' -count=1` — **FAIL as expected** for Unicode number preceding/following boundaries while `unicode.IsDigit` was still present.

After changing only the boundary helper from `unicode.IsDigit` to `unicode.IsNumber`, the focused and full gates returned:

- `go test ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1` — **PASS**
- `go test -race ./internal/agent/participation -run '^TestMentionParser(SaturnParity|BoundariesAndCleanup)$' -count=1` — **PASS**
- `go test ./...` — **PASS**
- `go test -race ./...` — **PASS**; emitted an existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`), but exit status was 0
- `go vet ./...` — **PASS**
- `go build ./...` — **PASS**
- `gofmt -l .` — **PASS**, no files listed
- `git diff --check` — **PASS**
- Saturn `./mvnw -q -Dtest=org.saturn.app.agent.room.AgentMentionParserTest test` — **PASS** (exit 0, no output)

## Changed-file attribution and preservation

Task-owned parser delta relative to the pre-existing staged baseline:

- `internal/agent/participation/policies.go` — minimal Unicode-number boundary correction (`unicode.IsNumber`) in addition to the reconciliation implementation already present.
- `internal/agent/participation/policies_test.go` — two regression cases for Unicode number boundaries, retained as requested.
- `.hermes/handoffs/agent-mention-parser-qa.md` — this QA artifact.

The initial target status already contained broad unrelated staged/unstaged/untracked migration work. That work was preserved. The final unstaged parser delta names only the two authorized parser files; no parser change was made in `invocation.go`, and no new parser package was added. Existing unrelated unstaged paths, including `internal/agent/sql`, `internal/agent/turn`, and `internal/repository/h2`, were not modified by this QA task.

Forbidden wiring/excluded areas were not changed by this task: no listener, transport, provider, remote-room, command, core, factory, SQL, row #324 B/C, or row #325 changes.

Saturn preservation checks for the correct parser paths all returned exit 0:

- Saturn worktree diff for `src/main/java/org/saturn/app/agent/room/AgentMentionParser.java` and its test — unchanged
- Saturn index diff for those paths — unchanged
- Saturn `HEAD` comparison for those paths — unchanged

Saturn’s unrelated dirty files remain unrelated service/test/docs and diagnostic artifacts; neither authoritative parser path is listed as modified.

## Residual limitations

- The parity claim is bounded to the inspected Saturn source/test contract and the explicit cases listed above; unspecified regex/case-fold corner cases are not expanded.
- Go `strings.EqualFold` is the chosen equivalent for Saturn’s Unicode/case-insensitive matching; Java regex Unicode case-fold corner cases beyond the tested contract were not independently characterized.
- The target repository is intentionally dirty from other migration work, so this QA does not claim a clean repository or isolate a historical commit baseline beyond the staged-vs-unstaged attribution checks.
- The overall Saturn migration remains **incomplete**, and this slice remains intentionally unwired.
