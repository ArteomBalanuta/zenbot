# Deterministic public-message spam moderation QA

## Verdict

**PASS with one scoped hardening fix.** The deterministic message monitor, typed executor, pre-filter observation bridge, and H2 shadow-ban path satisfy the requested bounded enforcement behavior. No provider, tool, runtime, memory, evidence, user-command dispatch, `Ban`, retry, or background-worker path was introduced by this vertical.

## Source and implementation trace

- Saturn `RoomModerationMonitor.onMessage` confirms protected/whisper exclusion, literal `\\n`/LF normalization, inclusive windows, bounded per-identity queues, and escalation `WARN → MUTE → KICK → SHADOWBAN → none`.
- Zenbot `MessageMonitor.OnMessage` is mutex-owned and returns decisions only; `live.MessageAutomation.Observe` executes decisions after monitor return, outside the monitor lock, with one two-second child context per decision and contained errors.
- `participation.Pipeline.Handle` invokes `Monitor` before blank/whisper/self/bot/prefix filtering. The live composition remains a side observer and does not alter `PASS`, `CLAIMED`, submit, ambient counter, or command dispatch behavior.
- `ComposeMessageAutomation` remains separate from join composition and fails closed for disabled/incomplete detector config or absent typed shadow-ban persistence.
- H2 shadow-ban verification passed through `go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1`.

## Finding fixed

The typed warning/mute/kick methods accepted arbitrary inactive target text and used caller casing. That allowed a decision-to-action race after an active user departed and made resolved case variants unsafe.

Regression tests were added first and failed as expected:

```text
TestAuthoritativeMessageModerationUsesSafeFixedProtocol
  warning="{\"cmd\":\"chat\",\"text\":\"@raider Please stop flooding.\"}"
TestAuthoritativeMessageModerationRejectsInactivePrincipal
  inactive principal was accepted
```

`EngineImpl` now resolves every warning/mute/kick target through the active-user set under its user mutex, case-insensitively, emits the canonical current active nick, and returns an error with zero outbound operation when the principal is absent. A composition regression also proves a case-variant resolved admin stays protected.

## Verification

Passed:

```text
go test ./internal/agent/moderation -run 'Test(MessageMonitor|MessageActionExecutor|EngineActionExecutor)' -count=1
go test ./internal/agent/participation -run 'TestPipeline.*(Monitor|Moderation|Prefix|Whisper)' -count=1
go test ./internal/agent/live -run 'Test(RoomParticipation|MessageAutomation)' -count=1
go test ./internal/core -run 'Test.*(Moderation|Mute|Kick|Captcha|ShadowBan)' -count=1
go test ./internal/factory -run 'TestCompose.*Moderation' -count=1
go test ./internal/config -run 'TestAgentModerationRequiresMessageDetectionSettings' -count=1
go test -race ./internal/agent/moderation ./internal/agent/participation ./internal/agent/live -count=1
go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` reports only the known unrelated warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files changed by this QA pass

- `internal/core/engine_impl.go`
- `internal/core/moderation_test.go`
- `internal/factory/engine_factory_test.go`
- `.hermes/handoffs/rapid-agent-message-spam-moderation-qa.md`

The repository remains massively dirty/staged from other work; no unrelated files were reverted, no commit or push was made.
