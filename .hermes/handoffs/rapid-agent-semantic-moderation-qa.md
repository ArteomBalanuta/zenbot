# Semantic severe-abuse MODERATION ingress QA

## Verdict: PASS — fail-closed scaffolding only

The implemented Stage-A scaffolding is safe to retain. **Live semantic MODERATION ingress is not activated.** `SemanticModerationIngressReady()` returns `false`, `newLiveAgent` leaves `RoomParticipation.SemanticCandidate` nil, and no candidate can reach the ordinary public tool loop.

## Source/target behavior evidence

### Saturn read-only source (`/Users/ab/workspace/projects/saturn`, `develop`)

- `AgentRoomMessagePipeline.handlers` orders `monitorModeration` → eligibility → invocation preparation → quiet → mention → semantic → ambient (`src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java:95-102`). `handleMention` returns `CLAIMED` (`193-197`); semantic returns `CONTINUE` (`205-225`).
- The fixed source signal is Java `(?iu)` and therefore uses Unicode character-class word boundaries (`29-33`). The candidate gate, bot context, and message author target are source-owned local state (`205-223`).
- Saturn's moderation command inventory is exactly `captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban` (`RunCommandTool.java:45-46`).

### Zenbot audited behavior

- `Pipeline.Handle` calls `Monitor` before all filters, then filters ineligible input, processes quiet and mention before semantic, and only then permits ambient (`internal/agent/participation/invocation.go:133-163`). A mention cannot double-submit moderation; a successful semantic submission remains `PASS` and can continue to ambient.
- The MODERATION factory uses bot nick, creator trip, empty hash, public visibility, copied room users, exactly `api.ModerationCommands`, and the separate typed target. It rejects blank bot nick/target; it never grants the alleged author moderation identity (`invocation.go:57-74`).
- The live adapter uses only resolved `message.Context.Author.Name` for the target and rejects absent, bot, and self author metadata before setting the candidate (`internal/agent/live/participation.go:25-30`). It does not use chat asserted identity or model output.
- Main composes only `SemanticModerationReady: participation.SemanticModerationIngressReady()` and does not set `SemanticCandidate` (`cmd/zenbot/main.go:165`). The readiness function is hard-coded false because reviewed typed operations for `unmute` and `unshadowban` do not exist (`semantic_moderation.go:66-70`; confirmed by source search).
- No scoped implementation path uses `DispatchUserCommand`, public informational `run_command`, user authorization, `Ban`, raw agent protocol construction, retry, or a new memory/evidence path. Existing MODERATION runtime semantics remain silent: no result reply bypasses sink/failure delivery and `AfterDelivery` appends only replies (`internal/agent/runtime/runtime.go:122-145`, `internal/agent/live/runner.go:182-195`).

## Concrete defect fixed

**Unicode boundary mismatch (fixed).** The initial Go port used Go `\b`, which is ASCII-only. Saturn's Java `(?iu)\b` treats a Unicode letter adjacent to `doxx` as a word character; the initial Zenbot candidate incorrectly matched `猫doxx`.

- RED evidence:
  ```text
  go test ./internal/agent/participation -run '^TestSemanticModerationSignalMatchesOnlySourceSevereAbuseCandidates$' -count=1
  --- FAIL: .../unicode_word_boundary
  SemanticModerationCandidate("猫doxx") = true, want false
  ```
- Fix: `internal/agent/participation/semantic_moderation.go` now finds the source alternatives without Go `\b` and applies source-compatible Unicode word-boundary checks, including letters, numbers, marks, connector punctuation, and join controls. It preserves source terminal-boundary behavior only for `doxx?`, `swat(?:ting)?`, and `rape`.
- Regression: added the Unicode-boundary test case; the focused test passed after the fix.

## Tests and command results

Focused:

```text
gofmt -w cmd/zenbot/live_agent_test.go internal/agent/participation/semantic_moderation.go internal/agent/participation/semantic_moderation_test.go

go test ./internal/agent/participation -run 'Test.*(Semantic|Moderation|Pipeline)' -count=1
ok   zenbot/internal/agent/participation  0.315s

go test ./internal/agent/live -run 'Test.*(Moderation|Participation|Runner)' -count=1
ok   zenbot/internal/agent/live  0.349s

go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
ok   zenbot/cmd/zenbot  0.370s

go test ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -count=1
ok   zenbot/internal/agent/participation  0.195s
ok   zenbot/internal/agent/live  0.234s
ok   zenbot/cmd/zenbot  0.426s
```

Required regression/build checks:

```text
go test -race ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -count=1
ok   zenbot/internal/agent/participation  1.333s
ok   zenbot/internal/agent/live  1.532s
ok   zenbot/cmd/zenbot  1.701s

go test ./... -count=1
PASS: all packages; slowest observed package zenbot/internal/repository/h2 32.402s

go build ./...
exit 0

git diff --check
exit 0
```

Informational vet result, unchanged and permitted by the architecture handoff:

```text
go vet ./...
# zenbot/internal/core
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

No other vet finding was emitted.

## Files fixed by this QA

- `internal/agent/participation/semantic_moderation.go` — source-compatible Unicode boundary behavior.
- `internal/agent/participation/semantic_moderation_test.go` — regression for Unicode boundary false positive.
- `cmd/zenbot/live_agent_test.go` — asserts main leaves semantic ingress uncomposed and fail-closed.
- `.hermes/handoffs/rapid-agent-semantic-moderation-qa.md` — this report.

## Activation status and remaining gate

**Disabled/fail-closed. Do not activate.** Before candidate composition may be enabled, a senior-reviewed MODERATION-only gateway/tool loop must map every source alias to a typed operation, including currently absent `unmute` and `unshadowban`. It must remain separate from public `run_command`, `DispatchUserCommand`, user authorization, and `Ban`; then its action isolation, cancellation, silent lifecycle, and no-persistence behavior require focused QA.
