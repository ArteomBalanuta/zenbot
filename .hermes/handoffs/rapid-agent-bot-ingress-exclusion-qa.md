# Resolved bot-author ingress exclusion — independent QA

## Verdict

**PASS.** The active implementation carries exactly one trusted bit from the listener-resolved active user into the participation event and rejects `AuthorIsBot` immediately after monitoring. No production defect was found in the approved ingress branch.

## Evidence and ordering

- `internal/listener/message/handlers.go`, `ResolveUserMetadata`, resolves `Context.Author` from `Engine.GetActiveUsers()` by case-insensitive nick before `AgentParticipation` in `DefaultChainWithParticipation`.
- `internal/model/user.go`, `User.IsBot`, is server-decoded `json:"isBot"` metadata.
- `internal/agent/live/participation.go`, `RoomParticipation.Handle`, derives the event value exactly as `c.Author != nil && c.Author.IsBot`; it does not rescan the engine or inspect a provider/prompt/raw-message bot field.
- `internal/agent/participation/invocation.go`, `Pipeline.Handle`, invokes `Monitor(e)` first and then returns `PASS` on `e.AuthorIsBot` before `api.NewContext`, quiet handling, mention parsing/submission, moderation submission, or ambient counter mutation.
- The existing conventional bot-name check remains an independent exclusion.
- `internal/listener/message/handlers.go`, `AgentParticipation.Handle`, maps an unclaimed `PASS` to `next=true`, preserving downstream listener continuation.
- Saturn source is aligned: `AgentRoomMessagePipeline` orders moderation monitor, `filterIneligible`, invocation preparation, quiet, mention, moderation, and ambient; `isBotAuthor` checks matching active users' `isBot` state. Its test marks non-conventional `otherBot` as bot and expects `PASS` with no submissions.

## QA coverage added

No production change was needed. Two focused assertions were added to the shared active test changes:

- `internal/agent/live/participation_test.go`: a resolved non-bot `Author: &model.User{IsBot:false}` retains valid mention claiming, alongside the existing nil-author and resolved non-conventional bot cases.
- `internal/listener/message/chain_test.go`: an agent `PASS` executes the subsequent listener handler.

The existing bot pipeline test already verifies a non-conventional resolved bot is monitored, produces no submission, and cannot advance ambient cadence before the first ordinary human ambient event.

## Gates

Passed:

```text
gofmt -w internal/agent/live/participation_test.go internal/listener/message/chain_test.go
go test ./internal/agent/participation -run 'Test(Pipeline.*Bot|PipelinePrecedence|PipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors)' -count=1
go test ./internal/agent/live -run 'TestRoomParticipation.*(Bot|Mention|Ambient|Quiet)' -count=1
go test ./internal/listener/message -run 'Test(DefaultChain|AgentParticipation)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test -race ./internal/agent/participation ./internal/agent/live ./internal/listener/message -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` still reports the known unrelated copylocks warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions preserved

No production changes were made by this QA pass. No listener ordering, direct/relay/runtime/tool/provider/finalizer/delivery/persistence/configuration/SQL/resource/Saturn source changes, commits, or pushes were made. The repository remains extensively dirty/staged from concurrent and prior work; only the two focused test additions above and this QA handoff are attributable to this pass.
