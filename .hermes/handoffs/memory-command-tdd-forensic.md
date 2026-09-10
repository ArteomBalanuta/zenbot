# `memory` command strict-TDD forensic

## Verdict: ACCEPT — no recovery/rebuild is required

Contrary to the newer `memory-command-current-architecture.md` limitation, durable historical tracer evidence exists. The command is **not currently dirty/untracked**: `internal/command/memory.go` and `internal/command/memory_test.go` are tracked, unchanged from `HEAD`, and were introduced together by reachable commit `f9079caf4890c67f9e298369a23c42409151def8` (`feat: advance Saturn parity migration`, 2026-08-31), on both local and `origin/migration/saturn-zenbot-parity`.

The evidence supports acceptance under strict tracer TDD:

1. `.hermes/handoffs/next-rapid-command-slice-architecture.md:83-94` specified the one end-to-end tracer before implementation: register aliases, execute `memstats` with ignored arguments as a whisper, verify the report shape, then cancellation/no-reply behavior.
2. `.hermes/handoffs/next-rapid-command-slice-implementation.md:9-21` records that exact named test was added first and observed RED:
   ```text
   --- FAIL: TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport (0.08s)
       memory_test.go:22: alias "mem" was not registered
   FAIL
   FAIL    zenbot/internal/command    0.609s
   FAIL
   ```
   This is the expected pre-feature failure: the parent of `f9079ca` has neither `memory.go`/`memory_test.go`, no `case "memory"`, nor `"memory"` in the adapter concrete canonicals.
3. The same implementation record:23-29 records GREEN for the same tracer; :51-61 records package and full-suite validation plus `git diff --check`. Independent contemporaneous QA is in `.hermes/handoffs/next-rapid-command-slice-qa.md:17-32`.
4. Current rerun is green but is corroboration only, not antecedent evidence:
   ```text
   go test ./internal/command -run '^TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport$' -count=1
   ok      zenbot/internal/command    0.561s
   ```

No source-search hit supplied contrary RED evidence. The newer architecture file's statement that no antecedent RED evidence was present is factually stale/incomplete; it searched/considered the current dirty-worktree premise but omitted the tracked `next-rapid-command-slice-{architecture,implementation,qa}.md` record and commit history.

## Exact ownership boundary

### Memory vertical owned by commit `f9079ca`

| File | Exact owned hunk |
|---|---|
| `internal/command/memory.go` | Entire new 36-line file: `memoryMiB`; `memoryCommand.Execute` cancellation gate, `runtime.GC`, `runtime.ReadMemStats`, `reply`; private four-field formatter. |
| `internal/command/memory_test.go` | Entire new 61-line tracer `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport`. It verifies all three registrations, ignored arguments, whisper reply/report shape, and pre-cancelled no-reply failure. |
| `internal/command/handlers.go` | Only `case "memory": return &memoryCommand{b}` in `newCommand`. |
| `internal/command/dispatch_adapter.go` | Only `"memory"` appended to the unconditional `canonicals` literal. |

### Pre-existing or unrelated work: do not attribute/remove

| Location | Forensic finding |
|---|---|
| `internal/command/registry.go` | The exact `memory` catalog definition/aliases/`ADMIN` role existed in `f9079ca^`; it is a prerequisite, not an implementation hunk owned by this vertical. |
| `internal/command/admin_moderator_catalog_guard_test.go` | Currently untracked and not part of `f9079ca`; its memory catalog/fallback assertions are later/shared migration guard work, not required for the tracer's original GREEN. Retain it. |
| `internal/command/handlers.go` in `f9079ca` | The direct-`l` invoker interface/command implementation is unrelated. Current additional dirty handler changes (moderation/activity/replica/etc.) are also unrelated. |
| `internal/command/dispatch_adapter.go` in `f9079ca` | The direct-`l` registration helper is unrelated. Current dirty capability-gated registration changes are unrelated; the pre-existing `memory` list entry remains valid. |
| Current worktree | `git diff --exit-code -- internal/command/memory.go internal/command/memory_test.go` exits 0. The broad dirty worktree must not be reset, cleaned, staged, or rewritten for this vertical. |

## Dirty-worktree-safe recovery boundary (only if the accepted provenance is rejected later)

No recovery action is warranted now. If a fresh strict-TDD reimplementation is mandated despite the evidence, use manual surgical edits only—**no Git rollback/reset/checkout/clean/stage/commit**:

1. Preserve every file outside the four owned hunks above, including `registry.go` and `admin_moderator_catalog_guard_test.go`.
2. Manually delete `internal/command/memory.go` and only the `memory` test file; delete only the one `newCommand` case and only the `"memory"` literal in the unconditional registration list. Do not touch neighboring direct-`l` or current dirty changes.
3. On that manually produced pre-feature boundary, write the single tracer below and run it RED. Do not use the deleted implementation as a design source; use the approved architecture/source contract.
4. GREEN only the four owned application/test hunks. Then rerun the focused package gate and `git diff --check`; inspect the resulting path-level diff to confirm no unrelated hunk moved.

## First RED tracer and expected failure

First test: `TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport` in `internal/command/memory_test.go`.

It must create `commandEngineStub`, call `RegisterUserUtilities`, assert `mem`, `memory`, `memstats` registrations, execute `!memstats ignored arguments` as a whisper, expect one whisper reply matching the four thin-space-padded literal-`\\n` MiB lines, then verify a pre-cancelled context returns `(model.FAILED, context.Canceled)` with no reply.

On the stated manual pre-feature boundary the first expected failure is:

```text
--- FAIL: TestMemoryAliasesRegisterAndRenderSaturnShapedRuntimeReport
    memory_test.go:22: alias "mem" was not registered
```

This matches the historical RED record and proves the missing concrete registration rather than a typo or environmental error.

## Verification performed now

- Searched all handoffs for the command/test/RED record and searched current command sources.
- Inspected commit `f9079ca`, its parent, reachable refs, and current diffs. The commit introduces the two files and exact integration hunks; its parent lacks them.
- Focused tracer passed as shown above.
- `git diff --check` completed silently with exit 0.
- Current package-level working diff remains broad and unrelated; this assessment made no application or test modification.

[LIMITATION] A handoff's captured terminal output is historical process evidence, not an independently reproducible timestamped CI artifact. It is nevertheless a specific test-first RED record, matches the parent-tree absence and the committed GREEN implementation exactly, and is sufficient under the stated strict tracer-TDD standard; passing tests alone were not treated as proof.
