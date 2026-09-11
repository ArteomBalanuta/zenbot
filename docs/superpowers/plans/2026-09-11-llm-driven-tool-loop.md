# LLM-driven tool loop implementation plan

**Goal:** Implement the full attached SDK/tool-loop objective with model-owned orchestration and useful bounded execution feedback.

**Architecture:** One native model/tool loop, one authoritative command catalog, structured observations with retrievable full results, and mechanical execution limits. No semantic workflow planner or mandatory completion judge.

**Tech stack:** Existing Go packages, standard OpenAI-compatible chat protocol, existing command gateway and H2 persistence.

**Spec:** `docs/superpowers/specs/2026-09-11-llm-driven-tool-loop-design.md`

## Global constraints

- LLM owns tool selection, iterative planning, recovery and completion; no deterministic intent workflows.
- Keep actions synchronous and unknown outcomes non-retryable.
- Preserve current user request, partial successes and exact call/result identity.
- No unrelated security expansion; no user approval pauses.

## Tasks

- [x] Contracts: simplify provider descriptions and schemas, preserve prerequisites and delivery semantics, test actual catalog/provider definitions and command outputs.
- [x] Execution: distinguish attempted, in-flight, failed and completed calls; allow bounded legitimate retries, stop uncertain effects, expose actionable errors, verify independent reads and action barriers.
- [x] Results/context: emit structured bounded data with receipt/error metadata; add request-local full-result paging and retained result index; fix durable history schema; test pruning after multiple calls.
- [x] Loop: replace phase/recovery/judge choreography with one iterative model loop; unify/remove obsolete bounded command-prose execution; support structural correction and explicit terminal budgets.
- [x] Prompts/provider: concise integrated model instructions, raw-argument preservation, strict-compatible declarations and tool-free request isolation.
- [x] Integration: regression scenarios for live failure reproductions, dependent/conditional workflows, partial success, recovery, context retrieval and conversational completion.
- [x] Validation/report: full tests, focused race checks, vet/format, side-effect-free provider evaluation, updated architecture and requirement-by-requirement evidence report.

Each implementation task first adds a behavior regression where useful, reproduces the failure, implements the change, and runs the relevant package tests. The final review checks interactions and the full user objective, not only green unit tests.

## Integration review

The independent review found three additional issues, each reproduced and fixed:

1. Raw argument replay differed from the representation used for context sizing.
2. Filtering unexposed actions before execution bypassed the ordered failed-action barrier.
3. An early observation projection error could prevent retention of later executed effects.

The reviewer rechecked all three fixes and the new regressions. The final
requirement-to-evidence map is in `docs/2026-09-11-tool-loop-teardown-and-rebuild.md`.

## Empirical model check

The real configured endpoint was evaluated through the production loop with
synthetic tools only. Thinking-disabled baseline: 6/8 strict full-suite checks.
Thinking-enabled: 8/8, plus 6/6 repeated conditional cases. The baseline's
false-condition action failures were not context loss and were not hidden by
runtime heuristics. Local/example settings now enable thinking; details and
limits are recorded in `docs/evals/2026-09-11-tool-loop-provider.md`.

## Final verification

- `go test ./... -count=1` — passed, including H2 integration tests.
- `go test -race ./internal/agent/assemble ./internal/agent/live ./internal/agent/turn ./internal/agent/tool/... ./internal/command -count=1` — passed. macOS linker emitted LC_DYSYMTAB warnings; no race reports.
- `go vet ./...` — passed.
- `git diff --check` and `gofmt -l` over changed Go packages — clean.
- A fresh full provider run on the revised local reasoning-enabled setting
  returned 7/8 strict passes; the compound counts/kick case failed one check.
  Its exact assertion trace was not retained by the filtered command output.
  Three subsequent focused runs passed; these do not identify the failed
  assertion or erase that result. The provider evaluation report records these
  outcomes, fixed-size repeat blocks, and the missing-trace limitation.

No semantic runtime guard was added to conceal provider mistakes. Updated
local configuration is ignored by Git; no service was restarted or deployed.
