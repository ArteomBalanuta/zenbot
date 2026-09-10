# Join moderation independent QA

## Verdict: PASS with one informational pre-existing vet finding

The join moderation path is deterministic and source-grounded against Saturn `RoomModerationMonitor.onJoin`, `UserJoinedListenerImpl.notify`, and `ModServiceImpl`. It uses only the parsed `model.User`, injection-owned config/clock/protection predicate, and typed enforcement seams. No join reaches the conversational participation/runtime/provider/tool/delivery/memory/evidence path.

## Findings fixed

1. **Factory did not fail closed for missing persistence.** `composeJoinAutomation` previously composed a live executor for any enabled, complete config, even if its repository did not implement `repository.ShadowBanRepository`. A later shadow-ban could therefore fail only after a monitor decision. It now returns no automation unless the repository has the authoritative typed shadow-ban store. Regression: `TestComposeJoinAutomationFailsClosedWithoutShadowBanRepository`.
2. **Monitor maps retained expired one-off identity state.** Hash/name buckets (and signal/cooldown maps) could accumulate across distinct identities. `monitor.pruneExpired` now removes expired entries across every owned map under the monitor mutex before processing each join. Regression: `TestJoinMonitorPrunesExpiredStateAcrossDistinctPrincipals`.
3. **Target validation accepted whitespace-padded targeted principals.** `Decision.Validate` now requires a nonblank, already normalized principal, so exact active-user resolution cannot receive an ambiguous target. Regression added to `TestDecisionValidateActionTargetMatrix`.
4. **Captcha could outlive the listener’s bounded execution context.** `EnableCaptcha` checked the context but could block indefinitely writing to an unconsumed legacy outbound queue. It now sends through a context-aware private helper; ordinary calls retain the existing five-second outbound deadline. Regression: `TestEnableCaptchaHonorsExecutionDeadlineWhenOutboundQueueIsBlocked`.
5. **Name clustering was not source-equivalent for Unicode punctuation/symbols.** Go normalization had retained every non-ASCII rune, including emoji. It now retains only Unicode letters/numbers, matching Saturn `[^\p{L}\p{N}]` removal. Regression: `TestNormalizeClusterKeepsOnlyUnicodeLettersAndNumbers`.
6. **Real H2 persistence proof added.** `TestPersistShadowBanStoresTrustedIdentityInRealH2` verifies the bootstrapped `banned_users` table and parameterized typed write retain trip/name/reason and Base64-encode hash. The engine resolves the active trusted user and does not call `Ban`.

## Verified contracts

- Disabled/protected joins return before time/state mutation; monitor retains normalized primitive values only and executes no action under its lock.
- Room burst, same-hash distinct nick variants, name cluster detection, cooldown, post-kick escalation, and bucket clearing follow the observed Saturn monitor.
- Captcha emits only established `enablecaptcha`; shadow ban persists the active trusted user via `ShadowBanRepository`, without raw command dispatch or `Ban` fallback.
- Listener order is `AddActiveUser -> automation -> shareUserInfo -> LogPresence`; malformed input invokes neither registration nor automation. Automation contains per-decision executor errors and uses a two-second bounded context without retry.
- Factory is disabled for disabled/incomplete config or unavailable shadow-ban persistence.

## Gates

- `go test ./internal/agent/moderation -run 'Test(Decision|JoinMonitor|NormalizeCluster)' -count=1` — PASS
- `go test ./internal/listener -run TestUserJoinedListener -count=1` — PASS
- `go test ./internal/core -run 'Test.*(Captcha|ShadowBan|Moderation)' -count=1` — PASS
- `go test ./internal/repository/h2 -run TestPersistShadowBanStoresTrustedIdentityInRealH2 -count=1` — PASS
- `go test -race ./internal/agent/moderation ./internal/listener ./internal/core ./internal/repository/h2 -count=1` — PASS
- `go test ./... -count=1` — PASS (H2 included)
- `go build ./...` — PASS
- `git diff --check` — PASS
- `go vet ./...` — informational known failure only: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.

## Exclusions

No changes were made to agent live/runtime/provider/tool/command flows, message ingress policy, durable memory/evidence, user command dispatch, Saturn source, protected handoffs/resources, commits, or pushes. The repository addition is restricted to typed H2 shadow-ban persistence required by the authoritative operation.
