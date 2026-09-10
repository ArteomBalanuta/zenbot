# Join moderation implementation handoff

## Outcome

Implemented the bounded deterministic join-moderation vertical. It is enabled only by explicit complete `agent.moderation*` configuration; incomplete configuration or a repository without the authoritative shadow-ban persistence boundary composes no join automation / returns an execution error, rather than falling back to a ban or user-command dispatch.

## Source and protocol proof

- Saturn `UserJoinedListenerImpl.notify` orders `addActiveUser(user) -> getAgentRoomAutomation().onJoin(user) -> shareUserInfo(user)`.
- Saturn `RoomModerationMonitor.onJoin` supplies room bursts, same-hash nick variants, normalized-name clusters, cooldowns, and repeated same-hash escalation.
- Saturn `ModServiceImpl.enableCaptcha` emits `enablecaptcha`; Zenbot `EngineImpl.EnableCaptcha` now owns that exact authoritative protocol operation.
- Saturn `ModServiceImpl.shadowBan` persists trusted trip/name/base64-hash/reason rather than emitting `ban`; Zenbot `EngineImpl.ShadowBan` resolves the active trusted user and uses the new typed `repository.ShadowBanRepository` H2 persistence boundary. It never calls `Ban`.

## TDD evidence

Observed RED before each implementation slice:

- `go test ./internal/agent/moderation -run 'Test(Decision|JoinMonitor)' -count=1` failed with missing `JoinConfig`, `Decision.Validate`, and `NewJoinMonitor`.
- `go test ./internal/agent/moderation -run TestEngineActionExecutor -count=1` failed with missing `NewEngineActionExecutor`.
- `go test ./internal/core -run TestAuthoritativeCaptchaAndShadowBan -count=1` failed with missing `EngineImpl.EnableCaptcha` and `EngineImpl.ShadowBan`.

GREEN / final gates:

- focused moderation/listener/core tests: pass
- `go test -race ./internal/agent/moderation ./internal/listener ./internal/core -count=1`: pass
- `go test ./... -count=1`: pass (including H2, ~33s)
- `go build ./...`: pass
- `git diff --check`: pass
- `go vet ./...`: retains the pre-existing informational failure only: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value`.

## Security, order, and failure semantics

- Monitor inputs are only parsed `model.User`, composition-owned config, injected clock, and protected-principal predicate; it retains normalized primitives only under its mutex.
- Factory protection is source-shaped: creator/admin trips, local bot nick, `isme`, and trusted `IsBot` joins. Protected/disabled joins do not mutate monitor state.
- Listener invokes automation exactly once after `AddActiveUser` and before subscriber sharing/presence. Malformed JSON invokes neither.
- The automation executes decisions outside the monitor lock through a bounded 2-second context. Individual executor errors are logged and do not suppress sharing/presence.
- No join path reaches participation/runtime/provider/tools/delivery/memory/evidence. No retries, queue, `Ban` fallback, raw strings in agent code, or command dispatcher is used.

## Touched paths

- `internal/agent/moderation/{action.go,join_monitor.go,join_monitor_test.go,engine_executor.go,engine_executor_test.go,automation.go}`
- `internal/core/{engine_impl.go,moderation_test.go}`
- `internal/repository/{repository.go,h2/database.go,h2/shadow_ban.go}`
- `internal/listener/{user_joined_listener.go,user_joined_listener_test.go}`
- `internal/factory/engine_factory.go`
- `internal/config/agent_config.go`
- `config.example.toml`

## Adaptations/exclusions

The source persistence operation was migrated narrowly because Zenbot had no shadow-ban protocol/persistence boundary. This added only the `banned_users` H2 table and typed persistence method; no agent memory/evidence schema, message moderation, user command dispatch, SQL tool, mute/kick/warn executor, semantic moderation, or unrelated transport changes were introduced.
