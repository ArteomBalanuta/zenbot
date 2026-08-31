# Routing / Participation / Invocation Assembly QA

**Date:** 2026-08-30
**Repository:** `/Users/ab/workspace/go-projects/zenbot`
**Verdict:** **PASS for the bounded supported slice, after one scoped defect fix.** Full Saturn router parity remains explicitly unsupported/excluded.

## Scope and evidence reviewed

Compared the implementation handoff and architecture handoff with the read-only Saturn sources/tests:

- Saturn `AgentMentionParser.java` / `AgentMentionParserTest.java`
- `AgentQuietRegistry.java`
- `AgentRequestClassifier.java`
- `AgentInvocationFactory.java`
- `AgentRoomMessagePipeline.java` / `DefaultAgentRoomAutomationTest.java`
- `AgentRequestAssembler.java` / `AgentRequestAssemblerTest.java`
- Saturn `AgentSessionLockManager.java`

Reviewed all target Zenbot files under `internal/agent/participation`, `internal/agent/routing`, `internal/agent/runtime`, and `internal/agent/assemble`, plus the accepted API/adapters and the current model DTO.

## Findings and correction

One real parity defect was found in `internal/agent/participation/invocation.go`:

- The eligibility filter compared the author to the bot nick case-sensitively and did not reject Saturn’s conventional bot nick forms. It also incorrectly compared author nick to the room name rather than the bot identity.
- Corrected the filter to use `strings.EqualFold` for self-authored messages and to reject Saturn-equivalent conventional bot names (`bot`, `bot_2`, `helper-bot`, etc.) with `isConventionalBot`.
- Added `TestPipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors` to `internal/agent/participation/policies_test.go`.
- Verified RED before the production correction (the new regression test failed because `BoT` was claimed/unsupported), then GREEN after the correction.

No other production defect was found in the bounded slice.

## Behavior verification

- **Mention parser:** Case-insensitive exact `@` mention, boundary rejection for partial names/email-like text, punctuation/whitespace cleanup, and bare mention returning no prompt are implemented and covered by `TestMentionParserBoundariesAndCleanup`; behavior matches Saturn regex/cleanup vectors.
- **Quiet registry:** Polite-language AND quiet-intent predicate, normalized room plus API `UserIdentity` key, clock-based expiry at the exact boundary, and mutex-protected map access are implemented. `TestQuietRegistryUsesIdentityAndExpiresAtBoundary` covers expiry and identity behavior; race focused/full suites passed.
- **Classifier/finalization:** Deterministic blank/no-letter/control/protocol/action rejection, TALK social/prose-ending classification, and attempted-tool evidence overriding candidate classification are implemented in `participation/policies.go`; `TestClassifierDeterministicPrecedence` passes. No model/history consultation exists.
- **Command intent:** `routing.CommandIntentPolicy` and pure assembler filtering preserve ordinary tools, expose `saturn_` tools only for explicit alias / `run` / `execute` intent, and preserve the existing moderation policy. `TestAssembleFiltersCommandsModerationAndInternalEvidence` proves ordinary, moderation, and explicit-command paths. The routing package is a policy alias with no independent mutable state.
- **Invocation factory:** Trusted snapshot fields, defensive user snapshot, current inbound message text, mode, command-origin flag, and capability grants from trusted creator/admin/moderator metadata are preserved. Prompt prose is not consulted for authority. `TestFactoryUsesTrustedCapabilitiesAndPreservesMessage` passes. The API adapter remains explicit and does not JSON round-trip fields.
- **Participation precedence:** Monitor runs before eligibility; quiet consumption precedes mention; mentions are immediate and `CLAIMED`; moderation and ambient paths are `PASS`; absent submitter/factory returns an explicit `agent submission is unsupported` error rather than fabricated success. `TestPipelinePrecedenceAndUnsupportedFailure` and the new bot/self regression pass.
- **Runtime/API bridge:** `APIBridge` uses `FromAPIInvocation`; validation, `ErrBusy`, `ErrClosed`, cancellation, no-reply suppression, reply-only delivery, and close behavior remain in the runtime seam. Runtime `Context.MemoryKey()` uses UTF-16 room length and separates public sessions from whisper sessions by stable trip/hash/nick identity. `TestRuntimeMemoryKeySeparatesPublicAndWhisperSessions` and runtime lifecycle tests pass.
- **Assembly:** Existing `internal/agent/assemble` remains pure: no dispatch, repository, network, provider call, or orchestration was added. System/history/current-user ordering, internal evidence exclusion, tool filtering, paired tool projection, defensive copies, fingerprinting, truncation, freshness metadata, and cancellation tests pass.

## Exact files modified by this QA pass

- `internal/agent/participation/invocation.go` — corrected case-insensitive self/conventional-bot eligibility.
- `internal/agent/participation/policies_test.go` — added regression test for self/conventional-bot filtering and unsupported submission behavior.

The implementation handoff’s target files were otherwise inspected and left unchanged by this QA pass:

- `internal/agent/participation/policies.go`
- `internal/agent/routing/routing.go`
- `internal/agent/runtime/api_bridge.go`
- `internal/agent/runtime/adapters.go`
- `internal/agent/runtime/contracts.go`
- `internal/agent/runtime/runtime.go`
- `internal/agent/runtime/runtime_test.go`
- `internal/agent/assemble/assemble.go` and `assemble_test.go`
- accepted `internal/agent/api/*`

`gofmt` was run on all intentional target files listed by the implementation handoff and the QA-ed files.

## Actual verification results

All commands were run from `/Users/ab/workspace/go-projects/zenbot` after the correction:

- `go test -count=1 ./internal/agent/participation ./internal/agent/routing ./internal/agent/runtime ./internal/agent/assemble` — **PASS** (`routing` has no test files; other packages OK).
- `go test -race -count=1 ./internal/agent/participation ./internal/agent/routing ./internal/agent/runtime ./internal/agent/assemble` — **PASS**, no race reports.
- `go test -count=1 ./...` — **PASS**, all packages.
- `go test -race -count=1 ./...` — **PASS**, all packages, no race reports.
- `go vet ./...` — **PASS**, no output.
- `go build ./...` — **PASS**, no output.
- `git diff --check` — **PASS**, no output.
- `gofmt -w` on intentional routing/participation/runtime files — **PASS**.

## Worktree and Saturn preservation

The Zenbot worktree was already substantially dirty before this QA pass, including broad command/listener/core/config/repository/service changes, accepted agent foundation files, handoffs, resources, `.idea`, and other untracked artifacts. Those unrelated changes were preserved; no cleanup, reset, or broad wiring was performed.

Saturn was inspected read-only. Its pre-existing dirty state (including weather-service changes and prior QA/spec artifacts) was unchanged; no Saturn files were modified.

## Unsupported / excluded scope

This QA does **not** claim:

- full Saturn `DefaultAgentRouter` parity or a complete provider/tool/turn loop;
- tool schema/execution, command implementation, SQL/database policy, memory persistence, freshness/turn stages, moderation actions, or provider/config changes;
- live listener/engine autonomous-agent wiring, lifecycle activation, ambient production activation, or transport integration;
- Saturn JSON/wire parity for routing DTOs;
- changes to `internal/model.IdentityKey` or broad listener/command cleanup.

These are dependencies or exclusions documented by the architecture/implementation handoffs, not failed requirements for this bounded QA slice.
