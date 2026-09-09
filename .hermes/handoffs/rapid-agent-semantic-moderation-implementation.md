# Semantic severe-abuse MODERATION ingress implementation handoff

## Outcome

Implemented the safe Stage-A/fail-closed prerequisite scaffolding from `rapid-agent-after-message-spam-architecture.md`.

**Live semantic ingress remains disabled.** Zenbot does not have reviewed typed operations for every Saturn moderation `run_command` alias, so this is intentionally **not full semantic moderation parity** and no candidate is composed into the live agent. No moderation gateway, tool loop, provider/model, retry, config flag, user-command dispatch path, raw protocol construction, Ban path, persistence path, or Saturn source was changed.

## Source evidence inspected

Read-only Saturn `develop` evidence:

- `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`
  - `SEMANTIC_MODERATION_SIGNAL` (lines 29–33) is the exact fixed severe-abuse pattern ported to Zenbot.
  - `handlers` (lines 95–102) orders `monitorModeration`, eligibility, preparation, quiet, mention, `handleSemanticModeration`, then ambient.
  - Its event loop makes only `PASS` and `CLAIMED` terminal (lines 113–120); semantic handler is a `CONTINUE` stage.
- `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java`
  - `MODERATION_COMMANDS` (lines 45–46): `captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban`.

Zenbot evidence:

- `internal/agent/participation/invocation.go`: monitor remains before eligibility; quiet and mention remain before semantic handling; mention remains `CLAIMED`; ambient follows a successful semantic submission.
- `internal/agent/api/api.go`: `NewContextWithModerationTarget` and `Context.ModerationTarget` are the existing typed target seam.
- `internal/core/engine_impl.go`: typed owner operations exist for `EnableCaptcha`, `MutePrincipal`, `KickPrincipal`, and `ShadowBan`; search found no typed `Unmute` or `UnshadowBan` operation.
- `internal/agent/tool/run_command.go`: remains the unchanged public informational alias contract.
- `cmd/zenbot/main.go`: composes `SemanticModerationReady` from the prerequisite function, which currently returns false; it does not set a semantic candidate.

## Target contracts implemented

- `participation.SemanticModerationCandidate(model.ChatMessage) bool`
  - Exact fixed Saturn candidate signal, package-private compiled regexp.
  - Candidate is only a gate; it never performs an action.
- `participation.SourceModerationAliases() []string`
  - Full source inventory, explicitly distinct from the public informational aliases.
- `participation.SemanticModerationIngressReady() bool`
  - Hard fail-closed boundary returning false until all six aliases have reviewed typed Zenbot operations. Current missing prerequisites are `unmute` and `unshadowban`; they were not invented.
- `participation.Event.ModerationTarget string`
  - Typed canonical target field.
- `(*participation.InvocationFactory).CreateModeration(...)`
  - Builds a public bot/system context: room, bot nick, creator trip, empty hash, copied room users, exactly `api.ModerationCommands`, and typed target. It preserves alleged-author text as current message text but never uses the alleged author as caller.
  - Rejects blank bot nick or target.
- `participation.Pipeline.SemanticModerationReady`
  - Composition-owned readiness, not a config flag. If false, candidate data cannot submit.
  - When test-enabled, semantic submission follows mention handling and returns `PASS`; the existing ambient branch may submit afterward.
- `live.RoomParticipation.SemanticCandidate`
  - Future composition seam only. When supplied, the adapter requires resolved non-bot/non-self `Context.Author` and takes target only from `Author.Name`, never chat payload/model output. Main does not compose it while readiness is false.

## Files touched

Modified:

- `internal/agent/participation/invocation.go`
- `internal/agent/live/participation.go`
- `cmd/zenbot/main.go`

Created:

- `internal/agent/participation/semantic_moderation.go`
- `internal/agent/participation/semantic_moderation_test.go`
- `internal/agent/live/semantic_moderation_test.go`
- `.hermes/handoffs/rapid-agent-semantic-moderation-implementation.md`

Preserved unchanged:

- `.hermes/handoffs/rapid-agent-after-message-spam-architecture.md` remains untracked as supplied.
- Saturn worktree, `MIGRATION_PLAN.md`, configs, frozen audit records, public `run_command`, and existing handoffs.

## RED → GREEN record

1. Initial participation test compile failed as expected because `SemanticModerationCandidate`, `SemanticModerationReady`, and `ModerationTarget` did not exist.
2. Implemented fixed candidate/pipeline/context construction; focused participation and live tests passed.
3. Added prerequisite readiness test; it failed as expected because `SemanticModerationIngressReady` and `SourceModerationAliases` did not exist.
4. Implemented the explicit false readiness boundary and source alias list; focused tests passed.

## Test commands and real results

Focused final run:

```sh
gofmt -w internal/agent/participation/semantic_moderation_test.go
go test ./internal/agent/participation -run 'Test.*(Semantic|Moderation|Pipeline)' -count=1
# ok   zenbot/internal/agent/participation  0.435s

go test ./internal/agent/live -run 'Test.*(Moderation|Participation)' -count=1
# ok   zenbot/internal/agent/live  0.266s

go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
# ok   zenbot/cmd/zenbot  0.279s

git diff --check
# exit 0
```

Rapid regression run:

```sh
go test -race ./internal/agent/participation ./internal/agent/live -count=1
# ok   zenbot/internal/agent/participation  1.323s
# ok   zenbot/internal/agent/live  1.533s

go test ./... -count=1
# all packages passed; H2 repository package passed in 32.859s

go build ./...
# exit 0

git diff --check
# exit 0
```

Informational vet result:

```text
# zenbot/internal/core
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

This is the architecture-recorded unrelated pre-existing `copylocks` warning. It was not changed.

## Unresolved prerequisite before activation

Do not flip `SemanticModerationIngressReady` or compose `RoomParticipation.SemanticCandidate` until a senior-reviewed, moderation-only action loop/gateway maps **all** source aliases (`captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban`) to typed Zenbot operations. Specifically, do not fabricate unmute/unshadowban behavior, fall back to `Ban`, use `DispatchUserCommand`, use the public informational `run_command` loop, or invoke caller authorization. After that vertical is complete, add its tool/gateway cancellation, action-isolation, silent lifecycle/no-persistence, and live-composition tests before enabling ingress.
