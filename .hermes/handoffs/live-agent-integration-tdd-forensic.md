# Live-agent `l` strict-TDD forensic recovery

## Scope and evidence status

- **[OBSERVED]** Repository: `/Users/ab/workspace/go-projects/zenbot`; the working tree is already substantially dirty. `internal/agent/live/direct_submission.go` and `internal/agent/live/direct_submission_test.go` are untracked; the command files are modified alongside unrelated in-progress work.
- **[OBSERVED]** `.hermes/handoffs/live-agent-integration-implementation.md` records a valid RED then GREEN for tracer 1 and tracer 2, followed by an explicit hard stop before tracer 3.
- **[OBSERVED]** The current focused baseline passes: `go test ./internal/command ./internal/agent/live -count=1`.
- **[OBSERVED]** `git diff --check` exits successfully with no whitespace diagnostics.
- **[LIMITATION]** The report verifies recorded tracer evidence and the current tree. It cannot independently reconstruct a historical editor timeline beyond the recorded RED/GREEN commands.

## Deterministic evidence ledger

| Slice | Independently evidenced retained behavior | Evidence | Verdict |
|---|---|---|---|
| Tracer 1: command admission | `internal/command/handlers.go:DirectAgentSubmitter`, `directLCommand.Execute`, and `directLDefinition` trim the command prompt, make one submission, return its admission result, and do not deliver/persist/provider-execute in command code. | Recorded expected compile RED and focused GREEN in `live-agent-integration-implementation.md:5-20`; current `TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects` (`internal/command/handlers_test.go:226-245`); current focused package test passes. | **[TEST-BACKED / retain]** |
| Tracer 2: trusted construction | `internal/agent/live/direct_submission.go:AgentService`, `DirectSubmissionAdapter`, copied users snapshot, `InvocationFactory.Create(..., api.DIRECT, true)`, and one `AgentService.Submit` form the desired construction/submission seam. | Recorded undefined-symbol RED and focused GREEN in `live-agent-integration-implementation.md:22-37`; current `TestDirectSubmissionAdapterCreatesTrustedDirectInvocation` (`internal/agent/live/direct_submission_test.go:24-56`); current focused package test passes. | **[TEST-BACKED / retain only to the boundary below]** |
| Tracer 3: invalid-input rejection | Cancelled context, nil dependencies/message, blank trimmed prompt, and blank trusted room reject before factory/service submission. | No tracer-3 RED test exists. The architecture requires `TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit` before these guards (`live-agent-integration-current-architecture.md:179-182`); the implementation handoff explicitly identifies the early guards and stops at tracer 2 (`live-agent-integration-implementation.md:39-40`). | **[UNPROVEN / remove before resuming]** |

## Exact unproven production code

**[OBSERVED]** The following code in `internal/agent/live/direct_submission.go:27-40` is ahead of its antecedent test:

| Lines | Behavior | Why unproven |
|---|---|---|
| 27-29 | Returns `ctx.Err()` for an already-cancelled context. | No tracer-3 test has been written or observed RED. |
| 30-32 | Rejects nil `message`, `Service`, `Factory`, or `Snapshot` with `direct agent submission is unavailable`. | No tracer-3 test has been written or observed RED. The approved tracer explicitly requires nil message and nil service; factory/snapshot guards are also validation beyond tracer 2 and require their own RED coverage or inclusion in an explicitly revised next tracer. |
| 33-36 | Trims and rejects blank prompt with `direct agent prompt is required`. | No tracer-3 RED test exists. Prompt trimming/rejection here is adapter validation, distinct from tracer-1 command prompt trimming. |
| 38-40 | Rejects a blank trimmed snapshot room with `direct agent room is required`. | No tracer-3 RED test exists. |

**[OBSERVED]** `fmt` and `strings` imports at `direct_submission.go:5-6` support only the unproven guards. They must disappear with those guards. The snapshot copy at line 41 and factory/service path at lines 42-46 are tracer-2 behavior.

## Manual recovery boundary — no Git rollback

**[RECOMMENDED]** Manually edit *only* the untracked file `internal/agent/live/direct_submission.go`; do not run `git reset`, `git clean`, `git checkout`, staging, or any bulk formatter/diff reverter.

1. Retain the package declaration, `context` import, `api`, `participation`, and `model` imports; remove only `fmt` and `strings` imports.
2. Retain `AgentService` (`lines 13-17`) and `DirectSubmissionAdapter` (`lines 19-24`) exactly.
3. Replace only `DirectSubmissionAdapter.Submit` (`lines 26-47`) with this tracer-2-only body:

```go
func (a DirectSubmissionAdapter) Submit(_ context.Context, message *model.ChatMessage, prompt string) error {
    snapshot := a.Snapshot()
    snapshot.Users = append([]string(nil), snapshot.Users...)
    invocation, err := a.Factory.Create(snapshot, *message, prompt, api.DIRECT, true)
    if err != nil {
        return err
    }
    return a.Service.Submit(invocation)
}
```

4. Do **not** edit `internal/agent/live/direct_submission_test.go`, the command files, `main.go`, runtime/participation files, or any already-dirty unrelated source. In particular, retain the tracer-2 test unchanged; it is the evidence for the retained construction behavior.
5. Run the recovery checks below. This leaves nil/cancelled/blank inputs intentionally unsupported until the next test-first tracer supplies their contract.

**[RATIONALE]** The exact retained body is the smallest implementation exercised by tracer 2: trusted snapshot acquisition and copy, factory-derived DIRECT command-originated invocation, and one service submission. It deliberately does not claim validation behavior that has not had a RED test.

## Next strict tracer: tracer 3

**[RECOMMENDED — RED first]** Add `TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit` to `internal/agent/live/direct_submission_test.go`. Use table-driven cases for:

1. a cancelled context;
2. nil message;
3. blank/whitespace prompt;
4. blank/whitespace trusted snapshot room; and
5. nil service.

For every case, assert a non-nil error and **zero** factory and service submissions. The recording factory must distinguish `Factory.Create` calls, rather than relying only on the service call count. Run and record the expected feature-missing failure:

```sh
go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
```

**[RECOMMENDED — minimal GREEN only after that RED]** Reintroduce exactly the guards needed by those five cases: context cancellation; nil message/service; `strings.TrimSpace(prompt) == ""`; fetch trusted snapshot; `strings.TrimSpace(snapshot.Room) == ""`; then defensive-copy users, create once, and submit once. Do not add factory/snapshot nil validation unless a separately failing test is first added (or the tracer-3 test is explicitly expanded and observed RED). Do not alter capability derivation, command parsing, admission policy, runtime service/lifecycle, delivery/persistence, or composition.

## Other breach / scope review

- **[OBSERVED / no additional strict-TDD breach in this vertical]** Tracer-1 command test is narrow and asserts no direct chat/raw side effects. Tracer-2 asserts exact payload identity, DIRECT mode, command-originated flag, defensive snapshot copy, and factory-derived capabilities. `participation.InvocationFactory.Create` currently grants the asserted privileged capabilities when `CreatorTrip` equals the message trip for a DIRECT invocation (`internal/agent/participation/invocation.go:32-54`). Thus this test observes current centralized factory behavior; it does not add a new capability or public tool route.
- **[OBSERVED / no architecture no-scope change in the l-specific untracked files]** `direct_submission.go` imports no runtime provider, command gateway, tool loop, SQL, repository, transport/proxy, or moderation code. It creates an invocation solely through `participation.InvocationFactory` and calls the private service boundary.
- **[LIMITATION / not attributed to this breach]** The modified `internal/command/{handlers.go,handlers_test.go,dispatch_adapter.go}` also contain broad unrelated in-progress command/moderation/registration changes in their working-tree diff. They preclude treating the whole files as an `l`-only implementation diff, but this report neither reverts nor attributes them to the live-agent tracers. The l-specific hunks match tracer 1; recovery must not touch the other hunks.
- **[OBSERVED]** No runtime-service tracer, composition/registration tracer, shared-runtime lifecycle proof, or direct delivery/persistence regression proof has been started. These are incomplete future work, not evidence of code-ahead-of-test, because their planned production code is absent from the l-specific untracked implementation.

## Focused recovery verification

Run from `/Users/ab/workspace/go-projects/zenbot` after the manual removal, then after each later tracer:

```sh
# Current retained tracer evidence
go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1
go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1

# Package compile/regression baseline
go test ./internal/command ./internal/agent/live -count=1

# Dirty-tree-safe integrity inspection
git diff --check
git diff -- internal/command/handlers.go internal/command/handlers_test.go internal/command/dispatch_adapter.go
git diff --no-index /dev/null internal/agent/live/direct_submission.go
git diff --no-index /dev/null internal/agent/live/direct_submission_test.go
```

**[OBSERVED current results]** Before recovery, `go test ./internal/command ./internal/agent/live -count=1` passed (`internal/command` 10.065s; `internal/agent/live` 0.385s) and `git diff --check` passed. The last two `--no-index` commands are inspection-only and will return exit status 1 for untracked files when differences are present; assess their output, not that status, as the expected untracked-file diff.
