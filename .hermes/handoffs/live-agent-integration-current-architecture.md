# Smallest safe live-agent integration foundation: shared asynchronous `l`

## Decision

**[RECOMMENDED] Implement one capability-gated `l` vertical that submits a DIRECT invocation to the already-composed room `runtime.Runtime`; remove the synchronous direct-invoker execution path from public command dispatch.**

This is the smallest safe live integration still missing in the current target. It activates exactly one Saturn-visible capability (`l <prompt/question>`) through the existing agent runtime, while retaining the current public room pipeline, listener order, fixed public tool loop, H2-backed memory/evidence behavior, and semantic moderation's fail-closed state.

It excludes Whiskey, proxy configuration/dialers, replicas/remote-room work, semantic ingress activation, all public moderation/tool-command reachability, Dynamic SQL wiring, raw/admin SQL, and any LLM secret/config literal.

**Why this slice instead of a new provider/tool/relay feature:** the target already creates a live `runtime.Runtime` for room participation in `cmd/zenbot/main.go:newLiveAgent`, but creates a *second*, synchronous `live.DirectInvoker` in `directAgentInvoker`. `command.directLCommand.Execute` waits for that provider work inside inbound command dispatch. Saturn's `LUserCommandImpl.execute` constructs a `DIRECT` invocation and calls `AgentService.submit`, returning success without waiting for routing. The current target therefore has the needed runtime and configuration but not the source-shaped `l` admission/lifecycle boundary.

There is **no configuration/LLM-policy blocker** for this narrow vertical:

- **[OBSERVED]** `config.AgentConfig.Resolve` resolves enabled state, endpoint, model, API-key *environment-variable name*, bounded concurrency/queue values, timeout, memory settings, and output policy; it does not require a literal secret in TOML (`internal/config/agent_config.go:10-51,74-130,132-240`).
- **[OBSERVED]** `openai.New` accepts an absolute configured endpoint/model and only sets the Authorization header when the environment-resolved token is non-empty (`internal/agent/llm/openai/client.go:45-61,139-147`). No literal token is needed in code or tests.
- **[OBSERVED]** `runtime.NewWithFailureSink` already has bounded admission, cancellation, room serialization, delivery, and shutdown behavior (`internal/agent/runtime/runtime.go:46-88,109-145,220-236`).
- **[RECOMMENDED]** Enabled startup must still fail before listener startup if configuration resolution, repository composition, prompt/catalog loading, or provider construction fails; disabled configuration remains a pass-through and registers no `l` alias.

## Evidence ledger

### Saturn source behavior

| Source | [OBSERVED] contract |
|---|---|
| `src/main/java/org/saturn/app/command/impl/user/LUserCommandImpl.java:LUserCommandImpl.execute` | Alias is exactly `l`; authorized role is `REGULAR`; missing arguments fail with `l <prompt/question>` usage; null agent service replies `The agent is unavailable.`; otherwise it creates a `DIRECT`, command-originated invocation using `AgentInvocationFactory`, calls `AgentService.submit`, logs request ID, and returns success without waiting for provider routing (18-48). |
| `src/main/java/org/saturn/app/service/AgentService.java` | The public service boundary is only `boolean submit(AgentInvocation)` plus `close()` (5-10). |
| `src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java:submit` | Non-ambient work rejects disabled, closed, and full admission states synchronously; accepted work is scheduled asynchronously. The reply-required modes receive one stable busy/disabled/closed/admission failure message; execution failures get one stable failure message (41-68,105-174). |
| `src/test/java/org/saturn/app/command/impl/user/LUserCommandImplTest.java` | Tests require trimming prompt to `how many users?`, `DIRECT`, command-originated, trusted room/user snapshot, whisper preservation, and factory-derived capabilities; normal success is submission success, not provider completion (36-56,59-102). |
| `src/test/java/org/saturn/app/service/impl/AgentServiceImplTest.java` | Tests establish nonblocking acceptance, admission bound, one final response, no raw update, same-session ordering, silent ambient behavior, and rejection after close (27-80,176-229,304-390). |
| `src/main/java/org/saturn/app/listener/impl/UserMessageListenerImpl.java` | Listener order is Resolve metadata → Audit → Ignore bot → Relay AGENT → Log → Mail → AFK → Preview → Cern → Agent participation → Command dispatch (33-45). |
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java` | Room automation order is monitor → eligibility → prepare → quiet → mention → semantic → ambient; mentions submit then claim; semantic returns continue/pass; ambient remains non-claiming (94-120,138-240). |
| `src/main/java/org/saturn/app/listener/impl/UserJoinedListenerImpl.java` | Active user is added before agent join automation and subsequent joined-user work (48-79). |

### Zenbot current architecture

| Target | [OBSERVED] behavior / gap |
|---|---|
| `cmd/zenbot/main.go:newLiveAgent` | Resolves enabled configuration, builds one persistent memory/context/provider/catalog/assembler/public tool loop, creates a `runtime.Runtime`, and returns `RoomParticipation`; disabled config returns `message.PassParticipation` (107-191). |
| `cmd/zenbot/main.go:directAgentInvoker` | Duplicates the resolution/provider/catalog/assembler/tool-loop/memory construction and returns `live.DirectInvoker`; this is separate from the room runtime (198-250). |
| `cmd/zenbot/main.go:main` | Builds both objects independently; installs participation before command registration; closes only `roomAgent.Runtime` at shutdown (296-310,327-336). This makes `l` lifecycle ownership distinct from room turns. |
| `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` | Registers `l` only when a direct invoker is supplied (44-104). |
| `internal/command/handlers.go:directLCommand.Execute` | Synchronously invokes provider work and then sends/persists through the inbound command execution path (32-76). The legacy adapter invokes it with `context.Background()` (`internal/command/dispatch_adapter.go:22-26`), so inbound cancellation does not reach provider work. |
| `internal/agent/runtime/APIBridge.Submit` | Correctly maps API invocations to the existing runtime and sends AMBIENT through coalescing, all other modes through ordinary bounded admission (5-21). It is the direct submission seam to reuse. |
| `internal/agent/runtime/runtime.go` | For DIRECT/MENTION, failure sink delivery is only while open; sink delivery precedes `AfterDelivery`; memory/evidence write occurs only after successful visible delivery. Close rejects future work, cancels in-flight work, and waits (66-88,109-145,220-236). |
| `internal/agent/live/runner.go:Runner` | Executes assembled provider/tool work using invocation context, finalizes output, and persists only via `AfterDelivery` for visible replies (114-195). |
| `internal/listener/message/handlers.go:DefaultChainWithParticipation` | Matches Saturn's message ordering exactly, including participation immediately before command dispatch (191-199). It must remain unchanged. |
| `internal/listener/user_joined_listener.go:Notify` | Adds active user before automation and shares/audits after it; preserve this order (24-40). |
| `internal/agent/live/tool_loop.go:NewBoundedToolLoop` | Frozen public inventory is exactly `user_message_history`, `room_users`, and public-information-only `run_command`; it rejects replacement/inventory expansion (280-315). |
| `internal/agent/tool/run_command.go` / `internal/command/agent_gateway.go` | Public command tool/gateway is deliberately closed to informational aliases. It must not gain moderation/admin/SQL aliases (17-67; 24-96). |
| `internal/agent/participation/semantic_moderation.go:SemanticModerationIngressReady` | Is literally false pending the separately required source-owner nick/hash decision and private activation gate (62-72). |
| `cmd/zenbot/live_agent_test.go:TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig` | Explicitly asserts `SemanticCandidate == nil` and `SemanticModerationReady == false` (42-61). |
| `internal/agent/sql/policy.go:JSqlParserAgentSqlPolicy.Validate` | Only permits validated single SELECTs. This vertical must neither compose it nor expose dynamic/admin SQL to `l` (74-127,130-212). |

## Required target contract

### Proposed private boundary

Create an application-owned submission interface in `internal/agent/live` (or `internal/agent/runtime`; choose one owner and do not duplicate it):

```go
// AgentService is application-private and source-shaped.
type AgentService interface {
    Submit(api.Invocation) error
    Close()
}

type RuntimeService struct {
    Runtime *runtime.Runtime
}

func (s RuntimeService) Submit(inv api.Invocation) error
func (s RuntimeService) Close()
```

`RuntimeService.Submit` delegates exactly once to `runtime.APIBridge{Runtime: s.Runtime}.Submit(inv)`. It does no provider work, no retry, no direct chat send, no persistence, no command construction, and no policy conversion. `Close` is idempotent by delegation to the runtime.

The command-side contract becomes submission-only:

```go
// internal/command/handlers.go
// DirectAgentSubmitter is a narrow command-to-live-runtime boundary.
type DirectAgentSubmitter interface {
    Submit(context.Context, *model.ChatMessage, string) error
}
```

Its production implementation is an adapter owned by `internal/agent/live`, not `internal/command`. It must:

1. reject a canceled context, nil message, blank trimmed prompt, blank trusted channel, and nil service before creating an invocation;
2. receive the original listener/command `model.ChatMessage`, preserving channel, nick, trip, hash, and whisper bit as payload already resolved by `ResolveUserMetadata` for message dispatch;
3. take the active-room user snapshot from a supplied trusted snapshot function, copying it before invocation creation;
4. use `participation.InvocationFactory.Create(snapshot, *message, prompt, api.DIRECT, true)` so capability derivation stays centralized and no nick/hash inference is introduced;
5. call the runtime service exactly once and return its admission result without doing provider work or direct delivery.

The adapter must not infer a principal from nick/hash. It consumes the canonical event identity and trusted active-user snapshot supplied by the engine/listener path. No model output, tool result, command argument other than the prompt, or second active-user lookup may select identity/capabilities.

### Exact vertical flow

```text
[PROPOSED: `l` only; semantic ingress stays disabled]

chat "-l question"
  -> existing message chain, unchanged:
     ResolveUserMetadata -> Audit -> IgnoreBot -> Relay -> ... -> AgentParticipation -> DispatchUserCommand
  -> DispatchUserCommand authorization for REGULAR
  -> command legacy adapter (existing background context limitation; see deviation)
  -> directLCommand submits, does not call provider
  -> live.DirectSubmissionAdapter
       -> InvocationFactory.Create(trusted snapshot, DIRECT, commandOriginated=true)
       -> RuntimeService -> runtime.APIBridge -> runtime.Runtime.Submit
  -> admission result immediately returns to command dispatcher
  -> one runtime worker / per-room lock
       -> live.Runner -> fixed public tool loop -> finalizer
       -> sink: SendChatMessage(nick, "\n"+text, whisper)
       -> only after a successful visible send: Runner.AfterDelivery memory/evidence
```

This activates no room behavior that is not already composed. Mention/ambient/relay continue using their existing runtime submission. The only intended change is that `l` joins that bounded, cancellable runtime instead of blocking the listener through a second direct provider execution path.

## Lifecycle, delivery, cancellation, and evidence rules

| Situation | Required result |
|---|---|
| Agent disabled | No live runtime/service/direct submitter is constructed and no `l` alias is registered. Existing listener remains pass-through. Do not register an alias that replies “not configured.” |
| Enabled but invalid config, unavailable required repository, malformed catalog, or invalid provider setup | `newLiveAgent`/shared composition fails before listener/lifecycle start. Do not create a partial command handler, second DB, fallback client, default secret, or literal configuration. |
| Valid `l` prompt admitted | Command returns `SUCCESSFUL` promptly after submission; it emits no immediate acknowledgement or request ID. Provider completion is asynchronous. |
| Empty prompt | Preserve existing command parsing/usage behavior. No invocation/submission/send/persistence. |
| `ErrBusy` / `ErrClosed` | Map to the existing stable reply-required failure behavior **only through the shared runtime service policy**; do not claim command success or block/retry. Exact human-facing wording must be frozen by the first tracer against Saturn’s busy/closed behavior before implementation. |
| Runner/provider/tool/finalizer failure | For DIRECT, runtime's existing failure sink produces only `failed: the agent could not answer that request.` while runtime is open. No provider detail, SQL detail, secret, tool envelope, or partial answer is delivered. |
| No-reply marker | DIRECT finalizer treats an exact marker as an error (current `OutputFinalizer`), therefore failure sink semantics apply. Do not change finalizer policy in this vertical. |
| Visible reply | Runtime sink sends exactly once using captured invocation nick/whisper. `Runner.AfterDelivery` appends memory and public durable tool evidence only after the send succeeds. |
| Send failure/no reply/cancellation | No `AfterDelivery`, no memory append, no evidence append, no compensating resend/retry. |
| Shutdown | Main owns exactly one shared live service/runtime. Signal cancellation then `service.Close()` must occur before engine/replica/DB teardown. Close rejects subsequent submissions, cancels in-flight LLM/tool/repository calls, and waits. Remove the split `roomAgent.Close()`/unowned direct invoker lifecycle. |
| Parent command context | Current legacy dispatch uses `context.Background()` by design (`legacyAdapter.Execute`). This slice must not falsely claim client-disconnect cancellation. It must preserve runtime shutdown cancellation. A later listener/command-contract migration may pass inbound context end-to-end, but is out of scope. |

## Implementation-ready file map

| Path | Change | Required responsibility |
|---|---|---|
| `internal/agent/live/service.go` **new** | Add | Private `AgentService`/`RuntimeService` wrapper around one runtime/API bridge, compile-time interface assertions, no provider or engine logic. |
| `internal/agent/live/direct_submission.go` **new** | Add | `DirectSubmissionAdapter` constructs trusted `api.DIRECT` invocation via `participation.InvocationFactory` and submits it; no output/persistence. |
| `internal/agent/live/service_test.go` **new** | Add | Runtime forwarding, busy/closed behavior, close/cancellation ownership tracer. |
| `internal/agent/live/direct_submission_test.go` **new** | Add | Prompt/context/snapshot/capability preservation and no identity inference; canceled/blank inputs make zero submit calls. |
| `internal/command/handlers.go` | Modify | Replace synchronous `DirectAgentInvoker` use in `directLCommand` with submission-only boundary. Delete direct command-side delivery/persistence handling only after runtime path proves equivalence. |
| `internal/command/handlers_test.go` | Modify | Replace synchronous output assertions with prompt/submission/admission/error semantics; retain missing-prompt coverage. |
| `internal/command/dispatch_adapter.go` | Modify minimally | Rename/refit registration argument to submitter. Keep registration conditional and do not append any other alias. |
| `internal/command/agent_gateway.go`, `internal/agent/tool/run_command.go`, `internal/agent/live/tool_loop.go` | **No change** | Keep public tool command inventory frozen. |
| `cmd/zenbot/main.go` | Modify last | Replace `directAgentInvoker` + `newLiveAgent` duplication with one `newLiveAgentService` composition that returns `{Service, Participation, DirectSubmitter}` from a single runtime/runner/tool-loop/memory/context/client setup. Install participation and register `l` only when enabled. Main closes this one service before engine lifecycle stop. |
| `cmd/zenbot/live_agent_test.go` | Modify | Assert enabled composition returns a shared runtime service/direct submitter and disabled composition returns pass participation/no `l` submitter; retain semantic fail-closed assertions. |
| `internal/listener/message/handlers.go`, `internal/listener/message/chain.go`, `internal/listener/user_joined_listener.go` | **No change** | Preserve ordering and join semantics. Add regression assertions only if needed. |
| `internal/agent/participation/*`, `internal/agent/tool/moderation_action.go`, `internal/agent/commandgateway/moderation_gateway.go`, `internal/core/*`, `internal/repository/*`, `internal/config/*` | **No change** | Do not activate semantic ingress, add tools/operations, touch H2, add config, or broaden capabilities. |

A local composition struct is acceptable:

```go
type liveAgent struct {
    Service       live.AgentService
    Participation message.Participation
    DirectSubmitter command.DirectAgentSubmitter
}
```

It must not export an engine-wide mutable agent field or add agent state to `common.Engine`. Main is the composition/lifecycle owner.

## Strict one-tracer TDD order

Do not begin with a broad refactor or a pile of tests. A numbered tracer is one behavior only: write its focused test, run the stated command and observe the expected feature-missing RED, write only the production code required by that test, then rerun the **same** command to GREEN before writing the next tracer. A test that passes on its first run is existing regression coverage, not a RED tracer; retain it but do not count it as one.

1. **RED → GREEN command admission tracer — first and mandatory.** Add `TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects` in `internal/command/handlers_test.go`. Give the desired command a recording submitter and assert `!l hello world` calls `Submit(context, originalMessage, "hello world")` exactly once, returns `SUCCESSFUL` as soon as that admission call returns, and makes no `SendChatMessage`, persistence, or provider assertion in command code. Its initial RED is expected to be a compile/type failure because `directLCommand` and `directLDefinition` currently require `DirectAgentInvoker.Invoke`, not a submitter. **Immediately GREEN only this tracer:** introduce the minimal command-owned, testable seam `DirectAgentSubmitter interface { Submit(context.Context, *model.ChatMessage, string) error }`; refit `directLCommand`, `directLDefinition`, and the conditional registration parameter to use it; make `Execute` trim the prompt and delegate exactly once. Remove the synchronous output/delivery/persistence branches. Do not add a compatibility adapter that calls `DirectAgentInvoker.Invoke`: that would retain provider execution on inbound dispatch. Run:
   ```sh
   go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1
   ```
   Existing missing-prompt/zero-submission coverage remains a regression assertion; if absent, add it as its own later RED → GREEN tracer, never bundled with this admission behavior.
2. **RED → GREEN trusted invocation-construction tracer.** Only after tracer 1 is green, add `TestDirectSubmissionAdapterCreatesTrustedDirectInvocation` in `internal/agent/live/direct_submission_test.go`. With a captured trusted snapshot and a message containing room/nick/trip/hash/whisper state, assert one `api.DIRECT` invocation, `commandOriginated=true`, factory-derived capabilities, exact payload identity, and a defensively copied users slice after the source snapshot is mutated. **Immediately GREEN only this tracer:** add the private `live.AgentService` submission contract and `DirectSubmissionAdapter` sufficient to construct through `participation.InvocationFactory.Create` and call its service once. The adapter performs no provider work, send, persistence, identity inference, or second snapshot lookup. Run:
   ```sh
   go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1
   ```
3. **RED → GREEN adapter rejection tracer.** After tracer 2 is green, add a separate `TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit` covering canceled context, nil message, blank trimmed prompt, blank trusted channel, and nil service. Assert zero factory/service submissions for each invalid input. **Immediately GREEN only this tracer:** add precisely those validation guards; do not change capabilities, command parsing, or runtime policy. Run:
   ```sh
   go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
   ```
4. **RED → GREEN runtime-service forwarding/lifecycle tracer.** After tracer 3 is green, add `TestRuntimeServiceForwardsToOneRuntimeAndClosesIt` in `internal/agent/live/service_test.go`, using a blocking cancellable runner. Prove one accepted submission reaches the supplied runtime through `runtime.APIBridge`, a full runtime rejects promptly, `Close` cancels accepted work and late submit is rejected. **Immediately GREEN only this tracer:** add `RuntimeService` as the one-runtime `AgentService` implementation, delegating `Submit` exactly once to `runtime.APIBridge` and delegating idempotent close to that runtime. It must not construct a runner, client, memory owner, or second runtime. Run:
   ```sh
   go test ./internal/agent/live -run '^TestRuntimeServiceForwardsToOneRuntimeAndClosesIt$' -count=1
   ```
5. **RED → GREEN composition/registration tracer — last production tracer.** After tracer 4 is green, add `TestNewLiveAgentSharesRuntimeWithDirectSubmitter` in `cmd/zenbot/live_agent_test.go`. Assert enabled composition returns participation and a direct submitter backed by the same runtime/service; disabled configuration constructs neither provider/runtime nor submitter and registers no `l`. Retain the separate existing assertions that `SemanticCandidate == nil` and semantic readiness is false. **Immediately GREEN only this tracer:** replace `directAgentInvoker` plus duplicate composition with one `newLiveAgentService` composition result, pass its `DirectSubmitter` to conditional `l` registration, and make main close its one service before engine/replica/DB teardown. Do not change listener/chain order. Run:
   ```sh
   go test ./cmd/zenbot -run '^TestNewLiveAgentSharesRuntimeWithDirectSubmitter$' -count=1
   ```
6. **Post-GREEN regression verification, not a new tracer.** Once the preceding focused tracers are green, run the existing `runtime.TestRuntimeCallsPostDeliveryOnlyAfterSuccessfulVisibleSink` plus a direct-path integration assertion that a visible DIRECT reply calls `AfterDelivery` once only after sink success and sink error/no reply/close cause zero persistence. If that direct-path assertion exposes a missing behavior, stop: make it the next single RED tracer and immediately implement only its minimal GREEN. Do not label a first-run pass as RED. Run:
   ```sh
   go test ./internal/agent/runtime ./internal/agent/live -run 'Test.*(Direct|Runtime|Service|Delivery)' -count=1
   ```

## Source parity deviations and explicit limits

| Item | Disposition |
|---|---|
| Saturn `AgentService.submit` returns boolean and can immediately send busy/disabled/closed output; Go runtime returns errors. | **[RECOMMENDED adaptation]** Keep error values private at the submission adapter. The command must map accepted vs rejected synchronously to the frozen user-visible policy; do not expose Go error strings. Capture exact behavior with tracer tests. |
| Saturn uses one single-thread executor; Go runtime permits configured concurrent work but serializes each room with a mutex. | **[OBSERVED deviation]** Existing target architecture already differs. This vertical must reuse it, not alter worker/queue topology. Same-room `l`/mention ordering is preserved by `runtime.roomLock`; cross-room behavior is outside this slice. |
| Saturn's disabled service is present and can reply disabled; target disabled config currently produces no direct invoker/`l` registration. | **[RECOMMENDED safe target policy]** Retain no public reachability while disabled rather than adding an unconfigured command. This agrees with the plan's prohibition on “not configured” aliases (`MIGRATION_PLAN.md:17-18`). |
| Legacy Zenbot dispatcher passes `context.Background()` to old commands. | **[LIMITATION]** Direct client cancellation cannot be added honestly here. Runtime close cancellation remains guaranteed. Do not broaden listener/command framework solely to repair it. |
| Current `directLCommand` invokes and delivers synchronously. | **[REQUIRED correction]** This is not source-shaped acceptance and bypasses shared runtime lifecycle; remove it in this vertical. |

## Risk and QA matrix

| Risk | Control | Focused proof |
|---|---|---|
| Listener blocked by LLM | Submission-only command adapter; no call to `Complete` in command package | First RED tracer uses a blocked submit/runtime and proves command returns after admission. |
| Two clients/runtimes/memory owners | Single composition factory/service; direct adapter reuses runtime | Composition tracer asserts shared runtime/service identity/lifecycle. |
| Reply before/without durable persistence correctness | Keep runtime sink → `AfterDelivery` ordering | Runtime integration tracer: success once; sink error/no reply/cancel zero writes. |
| Inferred or model-selected identity | InvocationFactory + listener-derived message/snapshot only | Construction tracer checks exact room/nick/trip/hash/whisper and defensive copies. |
| Public moderator/admin/tool reachability | No changes to public `run_command`, gateway, tool loop, command catalog beyond existing conditional `l` | Search/diff review has zero added call sites to forbidden public/moderation interfaces. |
| Semantic moderation activation | No semantic file/main candidate composition changes; existing disabled assertion remains | `TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig` remains green. |
| Visibility leak | No repository/query/tool changes; existing public/whisper memory rules remain | Direct/runner tests preserve whisper no-public-context/evidence behavior. |
| Secret/config literal | Use only config resolver/environment name; tests use fake clients/values | Diff review rejects token/password/endpoint literals outside existing fixture-safe localhost tests. |
| Raw SQL/admin SQL | No SQL/tool/policy changes | Production diff has no changes under `internal/agent/sql`, persistence, or H2. |
| Whiskey/proxy scope creep | No transport/config/replica/command changes | File-map enforcement and diff review. |

## Verification gates

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation; scope formatting only to intentional implementation files:

```sh
go test ./internal/command -run 'TestDirectLCommand' -count=1
go test ./internal/agent/live ./internal/agent/runtime -run 'Test.*(Direct|Runtime|Service|Delivery)' -count=1
go test ./internal/listener/message ./internal/listener -run 'Test.*(Chain|Participation|Joined)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test -race ./internal/command ./internal/agent/live ./internal/agent/runtime ./internal/listener/message ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Independent QA must read back the final `DefaultChainWithParticipation` type sequence and verify it is unchanged, verify `SemanticModerationIngressReady() == false`, and inspect every new production call site for: `run_command`, `NewAgentCommandGateway`, `DispatchUserCommand`, `Moderation`, `DynamicSQL`, `SendRawMessage`, SQL policy/repository, proxy/Whiskey, and configuration secrets. Any new reachability or semantic composition is a fail.

## No-scope / hard stops

- Do not enable `SemanticCandidate`, `SemanticModerationIngressReady`, `moderation_action`, or a moderation tool loop. The Saturn unmute nick/hash contradiction remains a separate source-owner decision.
- Do not modify `messages.visibility`, history queries, H2 schemas, persistence, tool evidence schema, or dynamic SQL policy.
- Do not add public moderator/admin/SQL commands or extend `run_command` beyond its current informational enum.
- Do not implement/activate Whiskey, proxy configuration, proxy dialers, remote rooms, replicas, or relay topology changes.
- Do not add agent credentials, API keys, passwords, or endpoint secrets to config/tests/logs. `APIKeyEnv` names an environment variable; it is not a secret value.
- Do not use raw SQL/admin SQL, raw agent protocol payload construction, `common.BuildCommand`, or human command dispatch as an agent action mechanism.
- Do not reset/clean/checkout/stage/commit unrelated dirty work.

## Completion criteria

This foundation slice is complete only when a regular user’s configured `l` command is registered, derives its invocation from trusted message/snapshot state, is admitted asynchronously by the **same** live runtime as room participation, gives at most one final runtime-managed reply/failure, records memory/evidence only after visible delivery, cancels on runtime shutdown, and leaves listener order, visibility, public tool reachability, SQL policy, semantic moderation fail-closed state, and Whiskey/proxy scope unchanged.
