# Live `l` / legacy-command cancellation: smallest safe foundation

## Decision

**[RECOMMENDED] Build only an admission-phase inbound lifecycle-context bridge; do not cancel an accepted asynchronous `l` invocation from the inbound message context.**

The narrow, behavior-preserving foundation is feasible:

```text
Engine runtime context
  -> chat listener's one synchronous message-chain call
  -> DispatchUserCommand
  -> optional context-aware legacy adapter
  -> SaturnCommand.Execute(ctx)
  -> directLCommand -> DirectAgentSubmitter.Submit(ctx, ...)
  -> runtime admission
```

It lets `l` observe cancellation **before/during admission** when its engine is stopping or its transport has terminally failed. It leaves an admitted `l` detached and owned by the shared live runtime exactly as it is now. No per-user disconnect signal exists in the current hack.chat inbound protocol or listener contract, so this must not be described as client-disconnect cancellation.

A request to cancel an already accepted `l` on a message-context cancellation is **not safe to implement in this slice**. `runtime.Runtime.Submit` accepts an `Invocation` without a caller context and runs it under runtime-owned `rt.ctx`; changing that would decide whether a reply survives a transport reconnect, alter visible reply/persistence behavior, and diverge from Saturn's accepted-work contract. See the decision gate in [Accepted-work decision gate](#accepted-work-decision-gate).

Whiskey and proxy work are explicitly excluded. Semantic moderation remains private, uncomposed, and fail-closed. This proposal adds no tool, SQL, admin, or moderation reachability.

## Observed architecture

### Zenbot inbound and legacy dispatch

| Evidence | [OBSERVED] behavior |
|---|---|
| `internal/core/engine_impl.go:StartContext` | The engine derives a runtime context from its lifecycle parent. Its receive goroutine exits when that context is canceled and cancels it after a transport error (171-221). |
| `internal/core/engine_impl.go:DispatchMessage` | The receive path dispatches `chat` with only `UserChatListener.Notify(jsonMessage)`; no context reaches a listener (358-390). |
| `internal/listener/user_chat_listener.go:UserChatListener.Notify` | Decodes a message, normalizes whisper state, then invokes `chain.Process(context.Background(), ...)`; every inbound chat therefore loses the engine lifecycle context (26-36). |
| `internal/listener/message/chain.go:Chain.Process` | The chain already accepts a context and passes the exact same context serially to each handler; it has no goroutine, timeout, or cancellation creation of its own (26-38). |
| `internal/listener/message/handlers.go:DispatchUserCommand.Handle` | Authorization occurs before `cmd.Execute()`. The command interface is contextless, so the handler currently cannot pass its context beyond dispatch (167-189). |
| `internal/command/dispatch_adapter.go:legacyAdapter.Execute` | Migrated Saturn commands are invoked with `context.Background()` and errors only reach the existing log behavior (12-26). This is the direct cancellation gap. |
| `internal/common/command.go:Command` | The legacy public-internal registry contract is `Execute()`, `GetRole`, aliases, and `NewInstance`; changing it would require every command implementation and risks broad behavior changes (9-14). |
| `internal/common/command_registry.go:SaturnCommand` | The migrated command contract is already `Execute(context.Context) (Status, error)` (11-16). |
| `internal/listener/message/handlers.go:DefaultChainWithParticipation` and `internal/listener/message/chain_test.go:TestDefaultChainPlacesRelayBeforeAllDownstreamHandlers` | The ordered chain is metadata → audit → bot ignore → relay → log → mail → AFK → preview → Cern → participation → dispatch, and a test freezes that sequence (191-199; test 14-40). |
| `internal/listener/message/dispatch_authorization_test.go:TestDispatchUserCommandRejectsUnauthorizedPrincipal` | Unauthorized commands are rejected before `Execute`; the authorization reply behavior is already regression-covered (35-47). |

### Current `l` handoff and runtime ownership

| Evidence | [OBSERVED] behavior |
|---|---|
| `internal/command/handlers.go:directLCommand.Execute` | `l` checks `ctx.Err()`, trims the prompt, and calls `DirectAgentSubmitter.Submit(ctx, message, prompt)`; it performs no output or provider work itself (20-55). |
| `internal/agent/live/direct_submission.go:DirectSubmissionAdapter.Submit` | The adapter rejects a canceled context before building an invocation, then creates one `DIRECT`, command-originated invocation and calls `AgentService.Submit`; it does not retain the context after admission (25-53). |
| `internal/agent/live/service.go:RuntimeService` | The service delegates to `runtime.APIBridge` and owns runtime close, without a caller context parameter (8-23). |
| `internal/agent/runtime/api_bridge.go:APIBridge.Submit` | API bridge converts the invocation and admits it to the runtime; only invocation data crosses the boundary (5-21). |
| `internal/agent/runtime/runtime.go:Runtime.Submit`, `Runtime.execute`, `Runtime.Close` | Ordinary work is nonblocking bounded admission. Workers run with runtime-owned `rt.ctx`, not caller context. `Close` prevents later admission, cancels `rt.ctx`, and waits for workers/executions (66-88, 109-145, 220-236). |
| `cmd/zenbot/main.go:main` | Signal context drives lifecycle start. On shutdown main closes the shared room agent runtime before stopping host/replicas and deferred database teardown (210-284). |
| `internal/transport/connection.go:Connection` | Connection close stops reader/ping; it does not expose a per-message sender/session cancellation handle. Messages are raw byte payloads (53-57, 108-125, 173-187). |

**[LIMITATION]** The direct adapter's context check is useful only until `AgentService.Submit` returns. An accepted job is intentionally detached, so a later cancellation of the listener context cannot cancel its provider/tool/delivery work under current runtime ownership.

### Saturn comparison: source fact versus Go adaptation

| Saturn source | [OBSERVED] source behavior | Go consequence |
|---|---|---|
| `src/main/java/org/saturn/app/listener/impl/UserMessageListenerImpl.java:30-58` | A synchronous listener builds the exact ordered chain and processes a `ChatMessageContext`; there is no cancellation token. | Do not claim a Saturn source cancellation contract. The Go bridge is a lifecycle adaptation. |
| `src/main/java/org/saturn/app/listener/message/handler/DispatchUserCommandHandler.java:9-22` | Prefix detection creates `UserCommandBaseImpl` and calls `execute()` synchronously. | Go may pass context only through an optional internal adapter; preserve authorization/output/order rather than redesigning command dispatch. |
| `src/main/java/org/saturn/app/command/UserCommand.java:8-20`, `UserCommandBaseImpl.java:83-103` | Commands return an optional status; base dispatch performs factory lookup/authorization, executes, then audits status. No cancellation parameter exists. | Context propagation is not source parity. It must remain internal and preserve current Go’s preexisting authorization/error logging behavior. |
| `src/main/java/org/saturn/app/command/impl/user/LUserCommandImpl.java:31-49` | `l` validates prompt/service, constructs a `DIRECT` command-originated invocation, invokes `AgentService.submit`, logs request ID, and returns success without waiting for route completion. | Accepted `l` work must remain detached from its synchronous command call. |
| `src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java:40-68,105-146,176-181` | Service accepts/rejects synchronously, schedules accepted work on an executor, and `close()` marks closed then closes the executor. The task body has no request cancellation token. | Do not introduce per-message cancellation for accepted Go jobs as though Saturn supplied it. Go runtime shutdown cancellation is an existing stronger lifecycle policy, not a reason to bind jobs to input scope. |
| `src/main/java/org/saturn/app/facade/impl/EngineImpl.java:247-265` | Engine stop closes transport/replicas then closes agent service and database. | Zenbot’s owner is main/shared `roomAgent`; retain its existing shutdown ordering. |

## Recommended minimal design

### Ownership and exact interfaces

Do **not** change `common.Command`, `common.Listener`, command aliases, authorization, audit behavior, listener order, output formatting, snapshot/automove APIs, or any agent capability/tool composition.

1. **Engine owns the parent cancellation source.** Add a context-aware internal dispatch entry point on `*core.EngineImpl`:

   ```go
   // Compatibility entry point for existing callers/tests.
   func (e *EngineImpl) DispatchMessage(jsonMessage string) {
       e.DispatchMessageContext(context.Background(), jsonMessage)
   }

   // Called only by StartContext's receive loop with its lifecycle ctx.
   func (e *EngineImpl) DispatchMessageContext(ctx context.Context, jsonMessage string)
   ```

   `StartContext` calls `DispatchMessageContext(ctx, string(msg))`. The new method preserves parsing/switch behavior. For `chat`, it performs a local optional assertion:

   ```go
   if listener, ok := e.UserChatListener.(interface {
       NotifyContext(context.Context, string)
   }); ok {
       listener.NotifyContext(ctx, jsonMessage)
   } else {
       e.UserChatListener.Notify(jsonMessage)
   }
   ```

   Keep this optional assertion local to `core`; do not add a broad context method to `common.Listener` or alter non-chat listener semantics.

2. **User chat listener owns a bounded synchronous message child.** Add:

   ```go
   func (u *UserChatListener) Notify(text string) {
       u.NotifyContext(context.Background(), text)
   }

   func (u *UserChatListener) NotifyContext(parent context.Context, text string)
   ```

   Normalize nil parent to `context.Background()`. After JSON/whisper normalization, use `messageCtx, cancel := context.WithCancel(parent)` and `defer cancel()` around the existing single `chain.Process(messageCtx, &m, u.engine)` call. The child scope ends when synchronous chain processing ends; no handler may retain it for background work.

   This is a *message execution scope*, not a request deadline. Add no timeout: there is no current source or product timeout contract, and a timeout could change command output/automove/snapshot behavior.

3. **Dispatch owns the optional bridge, without changing legacy commands.** In `DispatchUserCommand.Handle(ctx, c)`, retain parsing and authorization byte-for-byte in behavior. Replace the final unconditional call with:

   ```go
   if contextual, ok := cmd.(interface {
       ExecuteContext(context.Context)
   }); ok {
       contextual.ExecuteContext(ctx)
   } else {
       cmd.Execute()
   }
   ```

   The interface remains unexported at the message package call site or can be a narrow exported-internal interface only if command tests require it. It is deliberately optional: all historical `common.Command` implementations keep `Execute()` untouched.

4. **`legacyAdapter` is the only production contextual command in this slice.** Refactor its existing body once:

   ```go
   func (a *legacyAdapter) Execute() { a.execute(context.Background()) }
   func (a *legacyAdapter) ExecuteContext(ctx context.Context) { a.execute(ctx) }
   func (a *legacyAdapter) execute(ctx context.Context) {
       if ctx == nil { ctx = context.Background() }
       status, err := a.def.New(a.engine, a.msg).Execute(ctx)
       if err != nil { log.Printf(/* existing format */, ...) }
   }
   ```

   It must still log errors exactly where it does today and return no status to the legacy handler. It neither sends new replies nor changes authorization/audit policy. Every registered migrated Saturn command receives the same lifecycle context, but only `l` has an adapter-level caller-context check before asynchronous admission.

5. **The live runtime remains sole owner after acceptance.** Make **no** change to `DirectAgentSubmitter`, `DirectSubmissionAdapter`, `AgentService`, `RuntimeService`, `runtime.APIBridge`, `runtime.Runtime`, `cmd/zenbot/main.go`, or `internal/agent/participation/*` for this foundation. `DirectSubmissionAdapter.Submit` already observes a canceled context before construction/admission. After a successful `Service.Submit`, it intentionally has nothing to cancel.

### Proposed sequence

```text
[PROPOSED — admission cancellation only]
engine StartContext(ctx)
  -> inbound chat bytes
  -> EngineImpl.DispatchMessageContext(ctx, raw)
  -> UserChatListener.NotifyContext(ctx, raw)
       -> child messageCtx; normal chain order unchanged
  -> DispatchUserCommand.Handle(messageCtx, state)
       -> existing prefix/build/authorization
       -> legacyAdapter.ExecuteContext(messageCtx)
       -> directLCommand.Execute(messageCtx)
       -> DirectSubmissionAdapter.Submit(messageCtx, ...)
            if messageCtx canceled before admission: FAILED/log path; no invocation
            otherwise: shared runtime admits DIRECT work
  -> message handler returns; child is canceled
  -> accepted runtime job continues under rt.ctx only
       -> normal final reply and post-delivery persistence rules
```

## Cancellation semantics and races

| Event | Required semantics |
|---|---|
| Normal inbound `l` | Same command alias, authorization, no immediate reply, queue/admission result, listener order, and runtime-owned final reply as current source. |
| Engine stop before or during synchronous command admission | Engine context cancels; direct `l` sees `ctx.Err()` or adapter sees it before invocation creation. Existing legacy adapter logs the returned command error. No agent invocation/submission after the observed cancellation. |
| Transport terminal error | `StartContext` cancels its engine context after reporting the error. A command currently in synchronous dispatch can observe cancellation. The root application/lifecycle may later retry an engine; this is **not** proof that a user disconnected. |
| A `l` job already admitted | It continues across the inbound message scope ending and across transient engine reconnect handling. Only `roomAgent.Close()` / `runtime.Close()` cancels it. This is intentional accepted-work detachment. |
| Process shutdown | Existing main closes `roomAgent` before engine/replica/DB teardown; runtime cancels jobs and waits. This foundation must not change order or add a second close owner. |
| Queue full / closed runtime | Existing `runtime.ErrBusy`/`ErrClosed` route stays unchanged. Do not map errors to new output wording here. |
| Listener cancellation after a non-`l` command has started a side effect | Context-aware migrated commands may stop at their existing context checks. This is the only intentional broader effect; it occurs only at engine stop/error and preserves their existing error logging path. Legacy commands without `ExecuteContext` behave exactly as today. |
| Snapshot/automove workflow accepted by its own coordinator | No change. Their timers/workflow ownership remains independent; do not capture/retain the message context in coordinator callbacks. |
| Concurrent engine stop and listener execution | Context cancellation is safe/idempotent. The handler remains synchronous; no new mutable command registry state or goroutine is introduced. The runtime has its existing locks/wait groups. |

**[RISK]** A test-only or direct caller of `EngineImpl.DispatchMessage` still uses `context.Background()` through the compatibility wrapper. This is correct: only `StartContext` owns a real lifecycle cancellation source. Tests that require cancellation must call `DispatchMessageContext` or `UserChatListener.NotifyContext` directly.

## Accepted-work decision gate

Do not propagate `messageCtx` into `runtime.Runtime.execute` until product owners select one explicit contract and a source-deviation review approves it:

1. **Detached accepted work (recommended now):** once `l` admission returns success, it may finish and reply after transport reconnection or after the listener returns. Only application/runtime shutdown cancels it. This matches Saturn’s submit-and-return shape and existing Zenbot runtime ownership.
2. **Connection-bound work:** an accepted `l` is canceled when the bot’s current WebSocket engine context ends. Define whether queued work is removed, whether failure output is suppressed, whether memory/evidence is skipped, and whether a replacement/reconnected engine may resume/retry it.
3. **User-session-bound work:** an accepted `l` is canceled only when the originating user session disconnects. This cannot be built until transport/listener exposes a trustworthy session identity and disconnect event; raw inbound chat payloads currently do not supply an ownership handle.

The gate is passed only when the owner supplies: (a) chosen scope and reconnect semantics; (b) user-visible result for cancellation after admission (silent versus stable failure); (c) persistence rule for cancellation racing sink/`AfterDelivery`; and (d) a decision on whether it intentionally departs from Saturn’s executor behavior. Without all four, binding jobs to `messageCtx` is unsafe.

**Different next foundation if the product wants post-admission cancellation:** create a separately reviewed runtime job-cancellation ownership model first (job IDs plus a runtime-owned cancellation registry and explicit lifecycle/session owner), then expose a cancellation source from transport. Do not smuggle this policy through `DirectAgentSubmitter` or the legacy adapter.

## Strict TDD order: one tracer only

This is a documentation-only handoff. The next implementer must not modify production code before the first test fails.

1. **RED — the only initial tracer.** Add `TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission` in the existing `internal/command/handlers_test.go`. Compose a real `legacyAdapter` from `directLDefinition(recordingSubmitter)`, register it on a minimal authorized engine, create a `UserChatListener` with the normal chain, and call the new intended `NotifyContext(cancelledCtx, rawChatFor("!l question"))` after canceling the parent. Assert all of the following:
   - the recording submitter is never called;
   - no chat/raw output is emitted by the command;
   - command authorization is still evaluated before the cancellation reaches execution (record this separately in the test fixture, not by changing production authorization);
   - `Notify` is not used for this test, because it intentionally retains background compatibility.

   Run and observe a **feature-missing RED** (compile failure for `NotifyContext`/`ExecuteContext`, or a failing call-count assertion because the current path uses `context.Background()`):

   ```sh
   go test ./internal/command -run '^TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission$' -count=1
   ```

2. **GREEN — only the minimal vertical above.** Implement `DispatchMessageContext`/optional chat notifier, `UserChatListener.NotifyContext`, optional context execution in `DispatchUserCommand`, and contextual `legacyAdapter`. Re-run the same command until green. Do not add a runtime job context, timeout, output mapping, command-interface migration, agent config, or second test before this tracer passes.

3. **Post-green regressions, not extra RED tracers.** Run existing focused checks for listener sequence, authorization, direct adapter cancellation, and runtime shutdown; if any exposes an unimplemented contract, stop and make that one behavior the next strict RED→GREEN tracer:

   ```sh
   go test ./internal/listener/message ./internal/listener -run 'Test(DefaultChainPlacesRelayBeforeAllDownstreamHandlers|DispatchUserCommandRejectsUnauthorizedPrincipal|.*UserChat.*)' -count=1
   go test ./internal/agent/live -run 'TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit' -count=1
   go test ./internal/agent/live ./internal/agent/runtime -run 'Test(RuntimeServiceForwardsToOneRuntimeAndClosesIt|.*Close.*|.*Cancellation.*)' -count=1
   ```

## File map and no-scope

| Path | Change / status |
|---|---|
| `internal/core/engine_impl.go` | **Minimal modification:** add context-aware dispatch entry point and use it from `StartContext`; retain contextless compatibility wrapper and all non-chat dispatch behavior. |
| `internal/listener/user_chat_listener.go` | **Minimal modification:** add `NotifyContext`; keep `Notify` background-compatible. |
| `internal/listener/message/handlers.go` | **Minimal modification:** optional `ExecuteContext` branch after unchanged authorization. |
| `internal/command/dispatch_adapter.go` | **Minimal modification:** context-aware legacy adapter with background-compatible `Execute`. |
| `internal/command/handlers_test.go` | **Add one strict tracer**, then follow-up regressions only after green. |
| `internal/listener/message/chain_test.go`, `dispatch_authorization_test.go` | **No production behavior change;** extend only if needed to lock existing order/authorization. |
| `internal/command/handlers.go`, `internal/agent/live/direct_submission.go`, `internal/agent/live/service.go`, `internal/agent/runtime/*`, `cmd/zenbot/main.go` | **No change for this foundation.** |
| `internal/agent/participation/*`, `internal/agent/tool/*`, `internal/agent/commandgateway/*`, `internal/repository/*`, `internal/transport/proxy.go`, Whiskey paths | **Hard no-scope.** |

No new capability must be registered; `l` remains conditionally registered exactly as it is in `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent`. No moderation/admin/SQL/tool/gateway call path may be added. Do not compose semantic moderation: `internal/agent/participation/semantic_moderation.go:SemanticModerationIngressReady` remains false and main must retain the existing fail-closed composition assertion.

## Verification gates

From `/Users/ab/workspace/go-projects/zenbot` after the one tracer is green:

```sh
gofmt -w internal/core/engine_impl.go internal/listener/user_chat_listener.go internal/listener/message/handlers.go internal/command/dispatch_adapter.go internal/command/handlers_test.go
go test ./internal/command -run 'Test(LiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission|DirectLCommand)' -count=1
go test ./internal/listener/message ./internal/listener -run 'Test(DefaultChainPlacesRelayBeforeAllDownstreamHandlers|DispatchUserCommandRejectsUnauthorizedPrincipal|.*UserChat.*)' -count=1
go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Cancellation)' -count=1
go test -race ./internal/command ./internal/listener/message ./internal/listener ./internal/agent/live ./internal/agent/runtime -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Manual/read-back QA must confirm:

- `DefaultChainWithParticipation` handler sequence is unchanged.
- The `DispatchUserCommand` authorization branch still precedes either execution method and emits the same unauthorized reply.
- `legacyAdapter.Execute()` still uses background compatibility, while only live engine chat receives `StartContext` cancellation.
- The direct submission adapter receives cancellation before invocation creation, but runtime execution still uses `rt.ctx` after accepted admission.
- No new imports/call sites touch `run_command`, `NewAgentCommandGateway`, moderation, DynamicSQL, SQL/H2, semantic composition, proxy, Whiskey, credentials, snapshot coordinator, or automove.
- `git diff --check` is clean and no unrelated dirty file is reset, cleaned, checked out, staged, or committed.

## Baseline recorded for this handoff

**[TEST-BACKED]** Before this handoff, the dirty checkout passed:

```text
go test ./internal/command ./internal/listener/message ./internal/listener ./internal/agent/live ./internal/agent/runtime ./cmd/zenbot -count=1
ok  zenbot/internal/command
ok  zenbot/internal/listener/message
ok  zenbot/internal/listener
ok  zenbot/internal/agent/live
ok  zenbot/internal/agent/runtime
ok  zenbot/cmd/zenbot
```

**[LIMITATION]** This baseline is focused only. The worktree already contains substantial unrelated tracked and untracked migration work; this handoff creates no application source/test change and makes no claim that the full dirty tree is release-clean.
