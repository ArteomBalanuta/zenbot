# Bounded routing/participation/invocation assembly implementation

Status: implemented and verified. This handoff covers only the bounded architecture slice; no listener/command autonomous-agent wiring was activated.

## Files created or modified

Created:
- `internal/agent/participation/policies.go` — mention parsing, quiet registry, deterministic request classifier and finalization.
- `internal/agent/participation/invocation.go` — trusted snapshot/factory, PASS/CLAIMED participation pipeline, explicit submitter seam, fail-closed unsupported submission.
- `internal/agent/participation/policies_test.go` — focused parser, expiry/identity, classifier, trusted capability, and pipeline regression tests.
- `internal/agent/routing/routing.go` — stable routing-policy package aliases plus deterministic command-intent definition filtering.
- `internal/agent/runtime/api_bridge.go` — explicit API invocation to bounded runtime admission bridge.

Modified:
- `internal/agent/runtime/contracts.go` — added runtime `Context.MemoryKey()` matching API UTF-16/public-vs-whisper session boundaries.
- `internal/agent/runtime/runtime.go` — same-memory-key invocations serialize; public and whisper sessions do not collide.
- `internal/agent/runtime/runtime_test.go` — regression test for public/whisper session-key separation.

## Implemented behaviors

- Mention matching is case-insensitive, exact/boundary-aware, requires `@`, removes address punctuation, normalizes whitespace before punctuation, and returns no prompt for a bare mention.
- Quiet requests require polite language plus a quiet intent. Entries are keyed by normalized room and `api.UserIdentity`, are concurrency-safe, and expire at the clock boundary.
- Classification is deterministic: blank/no-letter/control/protocol/action-leading input is `UNCLASSIFIED`; social/prose-ending input is `TALK`; attempted tool evidence always finalizes as `TOOL_CALL`.
- Command-intent filtering preserves ordinary tools, preserves all definitions for moderation, and hides `saturn_` definitions unless the newest prompt explicitly starts with the alias or `run`/`execute` plus alias.
- Factory copies trusted room/user/message state, preserves inbound current message text, propagates mode and command origin, and grants capabilities from trusted creator/admin/moderator metadata only. Prompt prose cannot grant authority.
- Participation precedence is explicit: eligibility filter, quiet consumption, immediate mention/CLAIMED, semantic moderation/PASS, ambient cadence/PASS. Unsupported or absent submission returns an explicit error rather than pretending success.
- APIBridge maps through the existing explicit runtime adapter and therefore retains validation, admission (`ErrBusy`/`ErrClosed`), cancellation, no-reply suppression, delivery-only-on-reply, and shutdown behavior.
- Runtime session locking now uses the stable memory key rather than room-only locking.
- Existing pure assembler remains the assembly boundary; API↔runtime adapters are used rather than JSON or guessed fields. No provider/tool/persistence/router loop was fabricated.

## Unsupported/excluded by design

Full Saturn `DefaultAgentRouter` parity remains unsupported because accepted target contracts for tool execution, turns/freshness, persistence/memory, moderation actions, and live listener lifecycle integration are absent. No tool schemas/execution, command implementation, SQL/database policy, memory persistence, freshness/turn stages, moderation actions, provider/config changes, transport/lifecycle changes, `IdentityKey` changes, or broad listener/command wiring were added. No Saturn wire/JSON parity is claimed for routing DTOs.

## Verification matrix (actual outputs)

- `gofmt -w internal/agent/participation/*.go internal/agent/routing/routing.go internal/agent/runtime/*.go` — passed.
- `go test -count=1 ./internal/agent/...` — passed (`api`, `assemble`, `llm`, `llm/openai`, `participation`, `prompt`, `runtime`; remaining agent packages compile/no tests).
- `go test -race ./internal/agent/participation ./internal/agent/runtime ./internal/agent/assemble` — passed.
- `go test -count=1 ./...` — passed all packages, including command/config/core/factory/listener/repository/service/transport.
- `go test -race ./...` — passed all packages.
- `go vet ./...` — passed with no output.
- `go build ./...` — passed with no output.
- `git diff --check` — passed with no output.

## Preserved worktree

The checkout was already substantially dirty/untracked before this task (Dockerfile, command/listener/core/config/repository/service changes, `.hermes`, `MIGRATION_PLAN.md`, `resources`, and the pre-existing broad `internal/agent` tree among others). Those changes were not reverted or cleaned. Changes from this implementation are limited to the files listed above, with no Saturn modifications and no broad listener/command activation.
