# Agent room participation: next foundation decision

> Zenbot citations are relative to this repository root. Saturn citations are relative to `../../projects/saturn` from this repository root (the inspected read-only Saturn checkout).

## Decision

**[RECOMMENDED] No additional live-agent room-participation vertical should be opened after the accepted shared-runtime `l` and lifecycle-admission cancellation work.** The distinct bounded room-participation foundation is already live-composed: public mention admission/claiming, ambient cadence, identity-scoped quieting, listener ordering, shared runtime submission, delivery-gated persistence, and shutdown cancellation are all present.

**[RECOMMENDED] Do not start an H2 agent-persistence, agent-config, or agent-routing foundation either.** Those seams are already implemented and composed in the production root. The next foundation should be selected as a **non-agent command vertical** from the separately prioritized command-parity backlog. This handoff intentionally names no command because the supplied source has no ordering evidence that would make one non-agent command higher priority than another.

**Exact smallest next slice: no code change in the agent room-participation, agent persistence/config/routing, semantic-moderation, proxy, or raw/admin-SQL surfaces.**

## Source-grounded status

### Production composition

**[OBSERVED]** `cmd/zenbot/main.go:newLiveAgent` resolves configuration, creates one `runtime.Runtime`, wraps it in `live.RuntimeService`, then injects that same service into both `live.RoomParticipation` and `live.DirectSubmissionAdapter` (lines 108–197). `main` installs the participation-aware message chain and registers `l` only with that direct submitter (lines 247–257); shutdown closes the same service before stopping the engine (lines 274–280).

**[TEST-BACKED]** `cmd/zenbot/live_agent_test.go:TestNewLiveAgentSharesRuntimeWithDirectSubmitter` asserts that enabled composition gives participation and `l` one `RuntimeService`, while disabled composition yields `message.PassParticipation` and does not register `l`. `TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig` also asserts semantic ingress stays uncomposed.

**[OBSERVED]** This mirrors Saturn's composition boundary in `saturn/src/main/java/org/saturn/app/agent/routing/AgentRuntimeFactory.java:create`: it builds service/router infrastructure and installs `AgentRoomAutomation` on the engine only when enabled (lines 35–72). Zenbot has not copied Saturn's raw construction wholesale; it uses its own bounded runtime and frozen tool inventory.

### Ordered public message path, claims, and pass-through

**[OBSERVED]** Zenbot's message order is exactly:

```text
[OBSERVED]
transport -> EngineImpl.DispatchMessageContext
          -> UserChatListener.NotifyContext
          -> message.Chain.Process
             ResolveUserMetadata
             AuditChatMessage
             IgnoreBotMessage
             RelayAgentMessage
             LogChatMessage
             DeliverPendingMail
             UpdateAfkState
             YoutubePreview
             CernEasterEgg
             AgentParticipation
             DispatchUserCommand
```

The chain is declared in `internal/listener/message/handlers.go:DefaultChainWithParticipation` (lines 199–207), executed in `internal/listener/message/chain.go:Process` (lines 26–38), and receives the managed engine context from `internal/core/engine_impl.go:StartContext` (lines 171–222). `UserChatListener.NotifyContext` normalizes whisper forms and derives a child context for each inbound message (`internal/listener/user_chat_listener.go`, lines 26–45).

**[OBSERVED]** `message.AgentParticipation` converts a claimed agent outcome into `next=false`; a pass outcome proceeds to `DispatchUserCommand` (`internal/listener/message/handlers.go`, lines 153–165). Commands are thus not consumed by participation: `participation.Pipeline.Handle` rejects prefix-prefixed text before mention or ambient handling (`internal/agent/participation/invocation.go`, lines 133–163).

**[TEST-BACKED]** `internal/agent/live/participation_test.go` covers trusted mention invocation/claiming, listener-resolved bot metadata, pass-through for ordinary/prefixed/empty text, mention claim even when submission errors, ambient cadence, and quiet behavior (tests beginning at lines 31, 54, 91, 108, and 132).

**[OBSERVED]** Saturn has materially the same listener ordering in `saturn/src/main/java/org/saturn/app/listener/impl/UserMessageListenerImpl.java` (lines 30–58), and maps room automation `PASS` to continuation in `AgentParticipationHandler.java` (lines 7–13). Its pipeline ordering is monitor -> eligibility filter -> invocation preparation -> quiet -> mention -> semantic moderation -> ambient (`AgentRoomMessagePipeline.java`, lines 94–120).

### Mention, quiet, ambient, and trusted identity

**[OBSERVED]** `participation.Pipeline` excludes blank/whisper/self/bot/prefixed messages; records a polite quiet request without claiming it; claims and submits public mentions; and passes ambient outcomes (`internal/agent/participation/invocation.go`, lines 133–183). `MentionParser` is source-oriented and case-insensitive (`policies.go`, lines 18–67). `QuietRegistry` keys by normalized room plus authenticated API identity and only suppresses ambient turns (`policies.go`, lines 73–120).

**[OBSERVED]** `live.RoomParticipation` obtains the bot flag from listener-resolved `message.Context.Author`, while `main.go:newLiveAgent` builds the invocation snapshot from the active-user state, channel, configured creator trip, and admin trips (`internal/agent/live/participation.go`, lines 21–35; `cmd/zenbot/main.go`, lines 175–187). This is not payload-authority inference.

**[TEST-BACKED]** The aforementioned `participation_test.go` confirms the quiet scope remains user-identity-and-room scoped and mentions still work while that identity is quiet. `internal/agent/participation/policies_test.go` is the package-level parser/quiet policy coverage.

**[OBSERVED]** Saturn's `AgentQuietRegistry` uses the same room-plus-agent-identity key and expiry removal (`AgentQuietRegistry.java`, lines 47–98). Its ambient counter increments only after preceding eligibility, quiet, and mention stages (`AgentRoomMessagePipeline.java`, lines 181–240). Zenbot's ordering agrees on externally visible claim/pass behavior.

### Join and active-user ordering

**[OBSERVED]** `UserJoinedListener.Notify` parses the user, adds it to active users, invokes existing join automation, shares subscribed user information, logs presence, then runs AutoMove last (`internal/listener/user_joined_listener.go`, lines 24–39). Factory composition installs join automation only for permanent engines (`internal/factory/engine_factory.go`, lines 92–105).

**[TEST-BACKED]** `internal/listener/user_joined_listener_test.go:TestUserJoinedListenerRunsAutoMoveLastAfterExistingJoinEffects` verifies registration precedes both automations and the final order is semantic automation, subscription share, presence log, AutoMove.

**[OBSERVED]** Saturn's `UserJoinedListenerImpl` adds the active user, invokes `getAgentRoomAutomation().onJoin`, shares user info, applies shadow-ban behavior, then performs its legacy AutoMove branch (`saturn/src/main/java/org/saturn/app/listener/impl/UserJoinedListenerImpl.java`, lines 48–79). Zenbot's existing join-monitor automation is separately composed in `factory.composeJoinAutomation` and is not a missing room-agent participation ingress (`internal/factory/engine_factory.go`, lines 124–150).

### Runtime admission, cancellation, delivery, and persistence

**[OBSERVED]** `live.DirectSubmissionAdapter.Submit` checks cancellation before constructing a direct invocation; `l` routes through it via `directLCommand.Execute` and `legacyAdapter.ExecuteContext` (`internal/agent/live/direct_submission.go`, lines 32–53; `internal/command/handlers.go`, lines 25–55; `internal/command/dispatch_adapter.go`, lines 22–38). `DispatchUserCommand` preserves the inbound listener context for contextual commands (`internal/listener/message/handlers.go`, lines 167–197).

**[OBSERVED]** `runtime.Runtime.Submit` has one bounded admission channel for running and queued non-ambient work; `SubmitAmbient` retains only the newest ambient invocation; per-memory-key locks serialize execution; and `Close` closes admission, cancels runtime context, and waits (`internal/agent/runtime/runtime.go`, lines 66–88, 109–146, 148–236).

**[OBSERVED]** `live.Runner.AfterDelivery` writes conversational memory only after successful visible delivery, skips non-replies, and persists durable tool evidence only for public turns (`internal/agent/live/runner.go`, lines 182–195). `PersistentMemoryStore` propagates context cancellation, uses the API memory key, and rejects whisper tool-evidence persistence (`internal/agent/live/memory.go`, lines 27–93).

**[TEST-BACKED]** `internal/agent/live/direct_submission_test.go`, `runner_test.go`, `memory_test.go`, and `internal/agent/runtime/runtime_test.go` exercise direct submission, delivery/persistence ordering, cancellation, bounded admission, ambient coalescing, and close behavior. The focused baseline listed below passed these packages.

**[OBSERVED]** Saturn's `AgentServiceImpl` uses non-blocking semaphore admission for non-ambient work, latest-wins ambient scheduling, reply-required failure handling, and close-time executor shutdown (`AgentServiceImpl.java`, lines 40–182). Its `AgentSessionLockManager` is 64 fair striped locks (`AgentSessionLockManager.java`, lines 7–41). Zenbot already has a working per-memory-key serialization mechanism; replacing it with Saturn's striped implementation would be churn, not a bounded missing vertical.

### Config, routing, room context, and H2 persistence

**[OBSERVED]** Zenbot `AgentConfig.Resolve` validates enabled configuration, provider endpoint/model, memory bounds, output bound, admission capacity, and resolves environment-backed credentials (`internal/config/agent_config.go`, lines 10–240). `newLiveAgent` consumes the resolved values to compose client, prompt assembler, frozen tool loop, memory, room context provider, runtime, and service (`cmd/zenbot/main.go`, lines 108–197).

**[OBSERVED]** Public room context is bounded, room-scoped, public-only, chronological, and parameterized in `internal/repository/h2/agent_context.go` (lines 13–104); the live context provider excludes whispers and removes one newest duplicate of the current message (`internal/agent/live/conversation_context.go`, lines 56–102).

**[OBSERVED]** H2 agent memory and durable tool evidence are already exact-keyed, expiry-filtered, bounded, chronological, and cancellation-aware (`internal/repository/h2/agent_memory.go`, lines 13–170). The adapter and runner already load those stores and append only after delivery (`internal/agent/live/memory.go`; `internal/agent/live/runner.go`).

**[TEST-BACKED]** Real-H2 repository tests exist for public context, user history, agent memory, and tool evidence: `internal/repository/h2/agent_context_test.go`, `agent_user_history_test.go`, `agent_memory_test.go`, and `agent_tool_memory_test.go`.

**[OBSERVED]** Saturn's `AgentRouterFactory` composes provider, registry, H2 memory store, and repository room context (`saturn/src/main/java/org/saturn/app/agent/routing/AgentRouterFactory.java`, lines 11–38). Zenbot has equivalent bounded local composition. Therefore “H2 agent persistence/config/routing” is not an unstarted foundation.

## Semantic moderation: explicitly private and fail-closed

**[OBSERVED]** Semantic candidate detection is private policy code only. `SemanticModerationIngressReady()` returns `false` because Saturn aliases `unmute` and `unshadowban` lack reviewed complete typed operations (`internal/agent/participation/semantic_moderation.go`, lines 62–72). `newLiveAgent` sets readiness from that false function and does not set `RoomParticipation.SemanticCandidate` (`cmd/zenbot/main.go`, line 187). The composition test asserts both facts.

**[REQUIRED CONSTRAINT]** Keep semantic moderation private, uncomposed, and fail-closed. Do not add identity inference from payload/model output, a public moderation/action/tool route, proxy activation, or any raw/admin SQL route as part of any follow-on. Saturn's richer semantic path (`AgentRoomMessagePipeline.java`, lines 205–224; `AgentRoomAutomationFactory.java`, lines 38–68) is comparison evidence only, not authority to activate it here.

## First RED/TDD

**[RECOMMENDED] No RED test applies to the explicit no-agent-change result.** Do not add a test merely to justify unchanged composition.

For whichever non-agent command vertical is later selected, first add one narrow failing external-behavior test in that command's owning package, then run only that test to observe the intended failure. The test must cover its command registration gate, authorization/whisper routing, and its existing typed service/repository boundary; it must not reach agent packages or introduce a generic SQL/proxy escape hatch.

## File map for the no-change boundary

| Area | Evidence files |
|---|---|
| Production composition and shutdown | `cmd/zenbot/main.go`, `cmd/zenbot/live_agent_test.go` |
| Listener order and claims | `internal/listener/user_chat_listener.go`, `internal/listener/message/chain.go`, `internal/listener/message/handlers.go`, `internal/agent/live/participation.go` |
| Participation policies | `internal/agent/participation/invocation.go`, `internal/agent/participation/policies.go`, `internal/agent/live/participation_test.go` |
| Direct `l` and cancellation | `internal/agent/live/direct_submission.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/listener/message/handlers.go` |
| Runtime, delivery, persistence | `internal/agent/runtime/runtime.go`, `internal/agent/live/runner.go`, `internal/agent/live/memory.go`, `internal/repository/h2/agent_context.go`, `internal/repository/h2/agent_memory.go` |
| Join state and production factory | `internal/listener/user_joined_listener.go`, `internal/factory/engine_factory.go`, `internal/core/engine_impl.go` |
| Saturn comparison | `../../projects/saturn/src/main/java/org/saturn/app/agent/{room,routing}/...`, `../../projects/saturn/src/main/java/org/saturn/app/{listener,service}/...` |

## Risk QA and no-scope

**[OBSERVED] Focused baseline (documentation-only assessment):**

```text
go test ./cmd/zenbot ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./internal/listener ./internal/listener/message
PASS
```

**[REQUIRED QA for any later non-agent vertical]** rerun the focused agent/listener baseline above unchanged, plus the owning command/core/service/repository test package. Verify a prefixed message still passes participation to command dispatch; a public mention still claims; whispers remain ineligible to room participation; ambient remains non-claiming/latest-wins; `l` and mentions share one runtime; and cancellation prevents admission/persistence.

**No scope:** no modification to `cmd/zenbot/main.go` agent composition, `internal/agent/**`, `internal/listener/message/**` agent ordering, semantic moderation readiness, proxy transport, or raw/admin SQL. No changes to existing dirty application files are authorized by this handoff.
