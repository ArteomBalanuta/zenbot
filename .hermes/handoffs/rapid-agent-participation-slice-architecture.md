# Rapid agent participation / relay slice — architecture handoff

## Verdict

**BLOCKED: no implementation-ready combined participation/relay slice can be specified without inventing target topology or response policy.**

The target has reusable, intentionally unwired participation and runtime seams, but it has neither (a) Saturn's `AGENT` engine / host-reference relay topology nor (b) a composed asynchronous runner that can preserve Saturn mention/ambient response semantics. The accepted direct-`l` slice is deliberately synchronous and does not supply either missing contract.

This handoff therefore identifies the smallest evidence-backed prerequisite slice: compose a **mention-only** participation path after an explicit asynchronous runner/result-finalizer contract exists. It does **not** recommend ambient, moderation, or relay implementation in that slice.

## Scope and non-goals

- **In scope for the next executable slice once the blockers are resolved:** public, non-command `@<bot-nick>` messages submit one `MENTION` invocation; a successful reply-marked result is delivered to the originating user/whisper mode; the listener chain stops before `DispatchUserCommand`.
- **Not in scope:** ambient turns, quiet requests, moderation monitoring/actions, tools, memory/persistence, routing classification, SQL/H2, remote-room/session behavior, replica changes, or broad test matrices.
- **Relay is explicitly deferred/blockered**, not implemented as a no-op disguised as parity.
- Do not modify `MIGRATION_PLAN.md` or `.hermes/migration-audit.md`. Preserve all existing dirty files.

## Observed target evidence

### Listener dispatch and ordering

- `internal/core/engine_impl.go` (`(*EngineImpl).DispatchMessage`) routes `cmd == "chat"` to `UserChatListener.Notify`.
- `internal/listener/user_chat_listener.go` (`NewUserChatListener`, `(*UserChatListener).Notify`) unmarshals `model.ChatMessage`, normalizes whisper status, and calls a `message.Chain`.
- `internal/listener/message/chain.go` (`(*Chain).Process`) stops immediately on a handler error or on `next == false`.
- `internal/listener/message/handlers.go` (`DefaultChain`) preserves Saturn's relevant order:

  ```text
  ResolveUserMetadata → AuditChatMessage → IgnoreBotMessage → RelayAgentMessage
  → LogChatMessage → DeliverPendingMail → UpdateAfkState → YoutubePreview
  → CernEasterEgg → AgentParticipation → DispatchUserCommand
  ```

- `RelayAgentMessage.Handle` and `AgentParticipation.Handle` in that file currently return `(true, nil)` without side effects. `internal/listener/message/chain_test.go` only asserts a handwritten name list, not the returned handler sequence or agent behavior.

### Existing participation/runtime seams

- `internal/agent/participation/invocation.go` provides `Pipeline.Handle(Event)`, `InvocationFactory.Create`, `Submitter`, and `Pass`/`Claimed`. Its documented intended behavior includes eligibility filtering, mention parsing, quiet state, optional moderation/ambient submission, and a `Submitter` boundary.
- In particular, `Pipeline.Handle` returns `Claimed` for a parsed mention even if submission fails; `Pipeline.submit` returns the failure in `Outcome.Err`. This is a target implementation fact, not an unverified proposal.
- `internal/agent/participation/policies.go` supplies the accepted `MentionParser` and `QuietRegistry`; its package comment says it remains unwired until agent dependencies are available.
- `internal/agent/runtime/runtime.go` supplies bounded asynchronous admission, per-memory-key serialization, cancellation on `Close`, and `Sink` delivery only for `Result.ShouldReply()`. `internal/agent/runtime/api_bridge.go` converts the public `api.Invocation` submission boundary to this runtime.
- No production use of `participation.Pipeline`, `runtime.New`, or `runtime.APIBridge` was found outside their own packages/tests. The target currently has no production `runtime.Runner` implementation.
- `internal/agent/live/direct.go` (`DirectInvoker.Invoke`) is a synchronous command adapter: it builds a private `runtime.DIRECT` invocation, calls `assemble.Assembler`, then `llm.LlmClient.Complete`, and rejects empty provider content. It does not implement `runtime.Runner`, does not return `shouldReply`, and has no no-reply-marker processing.

### Composition/configuration limits

- `cmd/zenbot/main.go` resolves `config.AgentConfig`, constructs only the OpenAI-compatible client, prompt catalog, assembler, and `live.DirectInvoker`, then registers `l` via `command.RegisterUserUtilitiesWithDirectAgent`.
- `internal/config/agent_config.go` currently has `Enabled`, endpoint/model/key, timeout, token/step/tool limits, and `Ambient`; it does **not** contain Saturn participation fields `creatorTrip`, `ambientEveryMessages`, `quietMinutes`, `contextMessageLimit`, or `noReplyMarker`.
- `internal/factory/engine_factory.go` creates the permanent engine and assigns `listener.NewUserChatListener(e)` before `main` registers commands. It does not accept listener/agent composition options.
- `internal/common/engine.go` exposes channel, bot name, active users, and outbound message methods. Its active-user map can provide the room-user names needed by `participation.TrustedSnapshot`, but it exposes no trusted creator/admin/role participation snapshot.

### Relay topology is absent in the target

- `internal/model/engine_type.go` has only `MASTER`, `REPLICA`, and `ZOMBIE`; there is no `AGENT` engine type.
- `internal/core/engine_impl.go` has no host-reference field or host-to-agent relay route. Searches found no target use of `model.AGENT`, `HostRef`, or `hostRef`.

## Observed Saturn evidence

### Exact listener behavior

- `src/main/java/org/saturn/app/listener/impl/UserMessageListenerImpl.java` constructs the same ordered chain, putting `RelayAgentMessageHandler` after bot-ignore and `AgentParticipationHandler` immediately before command dispatch.
- `src/main/java/org/saturn/app/listener/message/handler/AgentParticipationHandler.java` calls `engine.getAgentRoomAutomation().onMessage(message)` and continues only when its `Outcome` is `PASS`.
- `src/test/java/org/saturn/app/listener/message/handler/AgentParticipationHandlerTest.java` proves `PASS → true` and `CLAIMED → false` handler continuation.
- `src/main/java/org/saturn/app/listener/message/handler/RelayAgentMessageHandler.java` applies **only** to `EngineType.AGENT`; it requires `hostRef`, enqueues `nick + ": " + escapeJava(text)` on the host, calls `hostRef.shareMessages()`, then returns `false`. If the AGENT engine lacks a host reference, it logs a warning and returns `false`.

### Saturn room automation and service semantics

- `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java` filters blank/whisper/self/bot/command messages; parses mentions; submits `MENTION` and returns `CLAIMED`; submits ambient only on configured cadence and returns `PASS`.
- That pipeline also owns quiet requests, moderation monitoring, semantic moderation, and bot detection. Those behaviors cannot be folded into a minimal mention slice without expanding scope.
- `src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java` runs requests asynchronously, delivers only `result.shouldReply()`, suppresses ambient failure replies, emits fixed failure replies for reply-required modes, bounds admission, coalesces ambient work, and closes its executor on shutdown.
- `src/main/java/org/saturn/app/agent/api/AgentParticipationConfig.java` defines the missing participation configuration and defaults: creator trip `595754`, ambient disabled, cadence `8`, quiet duration `15m`, context limit `60`, and no-reply marker `[[SATURN_NO_REPLY]]`.
- `src/main/java/org/saturn/app/agent/routing/AgentRuntimeFactory.java` composes config, participation config, infrastructure/tools/router, `AgentServiceImpl`, and `AgentRoomAutomation`; when disabled it installs `AgentRoomAutomation.none()`.

## Concrete blockers

1. **Relay cannot map directly.** Saturn relay is an AGENT-engine-to-host transport bridge. Zenbot has no AGENT engine, host reference, or comparable outbound ownership model. Adding a relay handler now would require inventing a topology and message escaping/delivery semantics. Keep `RelayAgentMessage` unchanged until a separate AGENT-engine/host lifecycle slice establishes that topology.
2. **No behaviorally compatible asynchronous runner exists.** Reusing `live.DirectInvoker` for participation would turn a listener callback into synchronous provider I/O and force every non-empty provider response to reply. That conflicts with Saturn's queued execution and `shouldReply`/no-reply semantics.
3. **The config prerequisite is incomplete.** The target can reuse existing `agent.enabled`, endpoint/model/key, timeout, and limits, but it cannot source Saturn's participation policy or no-reply marker. `Ambient bool` is not a grounded substitute for Saturn cadence and quiet/context settings.
4. **Trusted capability snapshot is incomplete.** The target can derive room and active-user names, but target composition does not expose creator/admin/role data required by its own `participation.InvocationFactory.Create` capability calculation. A mention-only slice must either provide that snapshot through an existing trusted authorization owner or explicitly submit with no elevated capabilities; source evidence does not justify manufacturing role data from chat payloads.

## Recommended gated prerequisite (not implementation instructions until blockers close)

Only after the four blockers are closed by a separately grounded architecture/implementation slice, use this minimal interface shape:

```go
// internal/listener/message: injected, not global engine state.
type Participation interface {
    Handle(context.Context, *Context) (claimed bool, err error)
}
```

- `AgentParticipation` owns a non-nil `Participation`; it maps `claimed` to chain continuation as `!claimed`.
- A disabled/unconfigured agent must install an explicit pass-through implementation, not a nil pointer and not a command-only `DirectInvoker` reuse.
- The adapter builds a `participation.Event` from the normalized `model.ChatMessage`, engine name/prefix/channel, and a trusted active-user/capability snapshot; it submits through `runtime.APIBridge` only after the runner/finalizer prerequisite exists.
- Construct the handler dependency at the composition root and inject it when creating the chat listener/chain. Do not put agent service state on `common.Engine` merely to reach one listener handler.

### Conditional execution sequence

```text
[observed dispatch]
chat JSON → UserChatListener.Notify → Chain.Process
  → ... Audit / IgnoreBot / Relay (still pass-through while relay blocked) ...
  → AgentParticipation.Handle
      → injected mention adapter
      → participation.Pipeline.Handle(Event)
      → runtime.APIBridge.Submit(api.Invocation)
      → Runtime worker → compatible runner/finalizer
      → Sink.Deliver only when result.ShouldReply
  → claimed mention: stop; otherwise DispatchUserCommand
```

### Error and silence semantics for that later slice

- Parsed mention: preserve the target `Pipeline` contract: claim before command dispatch even if `Submit` rejects; return the rejection as the listener error so `UserChatListener.Notify` logs it and no command executes.
- Not a valid mention / ineligible message: return pass with no provider call, no reply, and normal command dispatch behavior.
- Agent `shouldReply == false`: no outbound message; the runtime already enforces this.
- Provider/runner failure, full admission, or shutdown: exact Saturn user-facing failure text is not currently reproducible until the response-finalizer/agent-service prerequisite is migrated. Do not invent it in the listener.
- Relay: remain pass-through in this slice; Saturn's AGENT relay would stop the chain in all AGENT cases, including absent host reference, but there is no target condition to evaluate.

## Exact target files for the first unblocked mention-only implementation

These are conditional ownership paths, not changes authorized by this blocked handoff:

- Modify `internal/listener/message/handlers.go` — inject the participation dependency and map claim to chain stop; leave `RelayAgentMessage` untouched.
- Modify `internal/listener/message/chain.go` — add a dependency-aware default-chain constructor while preserving `DefaultChain()` compatibility for existing callers/tests.
- Modify `internal/listener/user_chat_listener.go` — accept the constructed chain (or add a constructor overload) so composition supplies it.
- Create `internal/agent/live/participation.go` — adapter from listener context to `participation.Event` and `runtime.APIBridge`; only after a compatible runner/finalizer exists.
- Modify `cmd/zenbot/main.go` — construct the resolved configured runtime, sink, participation adapter, and injected chat listener after engine creation; close the runtime during shutdown before transport/database teardown.
- Modify `internal/factory/engine_factory.go` **only if** composition cannot replace `e.UserChatListener` cleanly after factory creation; prefer avoiding a factory API expansion.
- Modify `internal/config/agent_config.go` only in the prerequisite configuration slice that maps the observed Saturn participation fields and validates them.
- Test additions: `internal/listener/message/handlers_test.go` (new or existing focused handler test) and `internal/listener/user_chat_listener_test.go` (new focused listener integration test).

Do **not** list `internal/core/engine_impl.go` or `internal/model/engine_type.go` as mention-slice changes. They become candidate files only for the separately blocked relay-topology slice.

## Focused TDD baseline concept

After a compatible runtime runner/finalizer is available, add one focused listener-level test:

1. Build a chain with an injected participation adapter backed by a recording `Submitter`/runtime bridge and a fake engine with bot name, prefix, channel, and active users.
2. Process public `"@bot hello"`.
3. Assert exactly one `MENTION` invocation with the parser-cleaned prompt `"hello"`, normalized room/user context, and no command execution; assert the chain stops before a sentinel command handler.
4. In the same table, process a non-mention and assert no submission and continuation to the sentinel.

This is intentionally one baseline behavior test. It does not attempt an ambient/memory/tools/moderation matrix.

## Validation commands

Run from `/Users/ab/workspace/go-projects/zenbot` after the prerequisite and mention-only changes:

```sh
gofmt -w internal/listener/message/handlers.go internal/listener/message/chain.go internal/listener/user_chat_listener.go internal/agent/live/participation.go cmd/zenbot/main.go

go test ./internal/listener/message -run 'TestAgentParticipation.*Mention' -count=1
go test ./internal/listener -run 'TestUserChatListener.*Participation' -count=1
go test ./internal/agent/participation ./internal/agent/runtime -count=1
go test ./...
git diff --check
```

Do not add race/vet/build as a blocker for this rapid slice unless the active capability or its focused/full test gate fails; this follows `MIGRATION_PLAN.md` rapid-parity policy.

## Deferred Saturn behaviors

- AGENT engine relay, host reference, Java escaping, host queue flush, and relay-chain stop.
- Ambient scheduling/coalescing/cadence, quiet registry integration, and `AgentParticipationConfig` defaults.
- Moderation monitor/actions, semantic moderation, protected-principal policy, and join automation.
- Router, response finalizer/no-reply marker, memory/context repository reads, tools, SQL policy execution, cancellation parity beyond the existing generic runtime contract, and exact failure replies.
- Replica/remote-room/Whiskey interaction.

## Complexity, risk, and developer routing

- **Complexity:** low for the eventual listener injection and mention claim mapping; high if combined with missing runner/finalizer/config/relay topology. Keep those as separate slices.
- **Primary risk:** a superficially working bridge can block commands while synchronously calling the provider, reply when Saturn would be silent, or grant capabilities from untrusted payload data.
- **@developer:** **Not yet standard enough for @developer implementation.** The exact listener claim mapping is standard, but the required asynchronous execution, response-finalization, trusted authorization snapshot, and relay topology are unresolved source-grounded prerequisites. Hand this document to architecture/migration ownership first; route only the narrow mention wiring to @developer after a reviewed prerequisite handoff supplies those contracts.
