# Next bounded Saturn agent parity vertical: deterministic join-event moderation dispatch

## Decision

**Select Saturn’s join-event moderation bridge: after Zenbot has parsed and registered one join, forward the trusted `model.User` to a bounded agent moderation monitor and execute only its validated decisions.**

This is the remaining source-observable agent ingress not covered by accepted direct/mention/ambient/relay, bounded tools, durable evidence, fresh history, command prose, output guards, or resolved-bot message exclusion. Saturn’s ordinary-message monitor is deliberately ordered before participation filtering, while a separate `onJoin(User)` ingress runs after active-user registration and before the listener’s ancillary join behavior. Zenbot has the corresponding trusted join event and an intentionally unimplemented agent moderation decision/action seam, but no monitor composition or join forwarding.

This vertical necessarily touches autonomous moderation because that is the complete source behavior of `AgentRoomAutomation.onJoin`; it must **not** be used to smuggle in semantic LLM moderation, reflective command exposure, generic command dispatch, broad transport work, or message-event moderation. The first implementation remains deterministic and join-only.

## Evidence map

### [OBSERVED] Saturn source contract

| Source evidence | Contract |
|---|---|
| `src/main/java/org/saturn/app/listener/impl/UserJoinedListenerImpl.java`, `notify` | Saturn deserializes a `User`, calls `engine.addActiveUser(user)`, then calls `engine.getAgentRoomAutomation().onJoin(user)` before `shareUserInfo`, shadow-ban handling, recently-seen notification, and auto-move logic. |
| `src/main/java/org/saturn/app/agent/api/AgentRoomAutomation.java`, `onJoin`; `src/main/java/org/saturn/app/agent/room/DefaultAgentRoomAutomation.java`, `onJoin` | The public automation boundary exposes a void join hook; the facade delegates it unchanged to its room pipeline. A join is not a conversational invocation and has no `PASS`/`CLAIMED` outcome. |
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `onJoin` and `execute` | The pipeline calls `moderationMonitor.onJoin(user)` and attempts each returned decision through the moderator executor. It neither constructs an `AgentInvocation` nor calls the LLM/provider/tool router. Executor failure is contained per decision. |
| `src/main/java/org/saturn/app/agent/moderation/RoomModerationMonitor.java`, `onJoin` | With moderation enabled and a non-protected joiner, the synchronized monitor records a timestamped nick and detects bounded room join bursts, same-hash nick variants, and suspicious normalized-name clusters; it can emit `CAPTCHA_ON` and, after repeated same-hash signals inside the post-kick window, targeted `SHADOWBAN`. It clears a triggered bucket to avoid repeated immediate signalling. |
| `src/main/java/org/saturn/app/agent/room/AgentRoomAutomationFactory.java`, `create` | Source composition supplies a system UTC clock and `ProtectedPrincipalPolicy` to the monitor, creates a bot context containing only `MODERATION_COMMANDS`, and uses `EngineModerationActionExecutor` for real enforcement. |
| `src/main/java/org/saturn/app/agent/moderation/ModerationDecision.java`; `EngineModerationActionExecutor.java`, `execute` | A decision is structurally valid only when a target is present for target actions and absent for `CAPTCHA_ON`. The executor maps room captcha to `captcha on`, targeted shadowban/mute/kick to gateway commands, and warning to a fixed delivery; runtime exceptions are logged and return failure rather than escaping the listener. |
| `src/test/java/org/saturn/app/agent/room/DefaultAgentRoomAutomationTest.java`, `monitorsCommandsAndJoinEventsBeforeParticipationFiltering` | With join threshold one, a command message produces `WARN`, then a join produces `CAPTCHA_ON`; there are no conversational submissions. This proves join monitoring is independent of, and does not claim, normal participation. |

### [OBSERVED] target state and precise gap

| Target evidence | Existing behavior / gap |
|---|---|
| `internal/listener/user_joined_listener.go`, `(*UserJoinedListener).Notify` | Zenbot parses `model.GetUser`, calls `e.AddActiveUser(u)`, then share-user-info and presence audit/logging. It has no agent join hook, so no agent-derived join decision can occur. |
| `internal/model/user.go`, `User` | The parsed join object already holds the trusted server fields needed by the source monitor: name, trip, hash, and `IsBot`. The monitor must receive this parsed value, never reparse raw JSON or derive identity from chat text. |
| `internal/agent/moderation/action.go` | Zenbot already reserves the narrow vocabulary `Warn`, `Captcha`, `Mute`, `Kick`, `ShadowBan`, a `Decision`, and `ActionExecutor`, but has no monitor, validation, composition root, or listener use. This is a suitable target seam, not evidence that enforcement exists. |
| `internal/common/engine.go`, `Engine` | The engine supports targeted `Kick`, `Ban`, and addressable chat delivery, but has no declared captcha-on or shadow-ban operation. A source-compatible executor therefore cannot be invented safely from the current public engine contract. |
| `internal/agent/participation/invocation.go`, `Pipeline.Handle`; `.hermes/handoffs/rapid-agent-bot-ingress-exclusion-qa.md` | Accepted bot ingress exclusion is a message-only eligibility decision after `Monitor(e)` and before invocation creation. It does not observe joins and must remain unchanged. |
| `internal/agent/runtime/runtime.go`, `Runtime`; `internal/agent/live/runner.go`, `Runner` | Accepted runtime/provider/delivery/persistence behavior serves conversational invocations. Join monitoring must not submit there, reserve admission, issue a completion, deliver an agent answer, or append memory/evidence. |

### [LIMITATION]

The target does not presently expose enough authoritative engine operations to implement Saturn’s full action executor honestly. In particular, no target definition/usage establishes a wire payload or command boundary for `captcha on` or a source-equivalent shadow-ban. Do **not** synthesize a raw protocol string, quietly map shadow-ban to `Ban`, or route autonomous decisions through the user-command dispatcher. The bounded prerequisite below explicitly adds a reviewed privileged engine boundary before any decision is live.

## Ranked alternatives

| Rank | Candidate | Decision | Reason |
|---:|---|---|---|
| 1 | **Trusted join-event deterministic moderation bridge** | **Selected** | Exact source ingress with a target placeholder contract and no provider/model/tool loop. It closes an unported autonomous safety path and keeps its decision data outside prompts and persistence. |
| 2 | Full message-event monitor plus semantic moderation | Deferred | Would merge source deterministic message monitoring, participation ordering, protected principals, and LLM moderation submission; this is materially wider than the selected join vertical. |
| 3 | `database_query` / schema / dynamic SQL | Deferred | Requires capability/visibility design and database contracts; explicitly outside rapid bounded scope. |
| 4 | Reflected `saturn_*` command catalog | Deferred | Broadens provider-visible authority and command catalog policy. Zenbot intentionally retains the fixed accepted `run_command` tool. |
| 5 | Provider correction/retry/third completion | Excluded | The accepted loop is bounded to one tool call and two completions; this ingress should make no completion at all. |
| 6 | Session locking, tool scheduling, command-intent filtering | Not selected | Already represented in target: `runtime.Runtime` serializes by `MemoryKey`, and `assemble.filterTools` mirrors source `AgentCommandIntentPolicy` for `saturn_*` definitions. |

## Exact contract and bounded adaptation

### Source contract retained

For each successfully parsed join event:

```text
trusted join JSON -> model.User
  -> AddActiveUser(user)
  -> deterministic agent join monitor
  -> zero or more validated autonomous moderation decisions
  -> attempt each decision through authoritative executor
  -> existing share/presence/log join behavior
```

Required outcomes:

1. A join hook is asynchronous only insofar as the existing listener is invoked; it does not create an agent `Invocation`, provider request, tool call, queue/admission reservation, model-visible result, chat reply, conversation record, or durable evidence.
2. The monitor owns bounded, time-windowed join state. It must reject disabled moderation and protected principals before state mutation.
3. Every decision is structurally validated before executor dispatch. Targeted actions require a normalized nonblank target; room `Captcha` requires an empty target.
4. Executor errors are contained and logged without blocking share-user-info/presence behavior, stopping the connection, or producing a user-facing agent failure response.
5. Bot-authored **messages** remain excluded by the accepted `AuthorIsBot` branch. Bot **join** policy must be explicit and source-grounded through the protected-principal predicate; it must not infer that every `IsBot` join is harmless unless the chosen policy says so.

### [RECOMMENDED] staged target adaptation

Zenbot should first port the full **deterministic decision production** contract, but gate executor composition until senior review confirms authoritative captcha and shadow-ban operations. This is not a fake implementation: a monitor can be tested with a recording executor, while production construction must remain disabled/pass-through if no complete executor is available.

Once operations are confirmed, preserve the source action meanings exactly:

| Source action | Target adaptation |
|---|---|
| `CAPTCHA_ON` | One authoritative room-level captcha enable operation. No target nick and no generic raw JSON construction. |
| `SHADOWBAN` | One authoritative targeted shadow-ban operation; do not substitute `Ban`. |
| `WARN` | Fixed source-shaped warning delivery only if the target’s reviewed executor confirms a non-duplicating delivery contract. |
| `MUTE`, `KICK` | Keep out of the first join implementation unless the source monitor can emit them for the selected thresholds and a reviewed executor maps them without using user authorization. |

The smallest safe deliverable therefore includes the prerequisite API/executor review and supports only the decisions that the join monitor can produce under the explicitly enabled configuration. If the authoritative captcha/shadow-ban operations cannot be established from target protocol code/fixtures, stop before production wiring and record the vertical as blocked rather than delivering a monitor that silently drops security decisions.

## File and interface plan

### Stage A — pure, deterministic join monitor and decision validation

**Create:**

- `internal/agent/moderation/join_monitor.go`
- `internal/agent/moderation/join_monitor_test.go`

**Modify:**

- `internal/agent/moderation/action.go`
- `internal/config/agent_config.go`
- `internal/config/agent_config_participation_test.go`
- `config.example.toml`

1. Make `Decision` construction private or add `Validate()` and require it at dispatch. Enforce the action/target matrix; preserve the reason as log/audit-only data, not delivery text or model input.
2. Add a typed `JoinMonitor` with injected `Clock`, typed configuration, and an injected `ProtectedJoin func(*model.User) bool`. Its public operation is:

   ```go
   type JoinMonitor interface {
       OnJoin(*model.User) []moderation.Decision
   }
   ```

   Return an allocated empty slice for disabled/protected/no-decision cases. Never return internal buckets/maps.
3. Port source-bounded join mechanics into this monitor only: room-burst queue/window, same normalized hash variant queue/window, normalized-name-cluster queue/window, signal cooldown, and clearing a bucket after its signal. Use a mutex owned by the monitor; inject a clock and never sleep in tests.
4. Extend resolved agent configuration with explicit positive join thresholds/windows and an off-by-default `moderation` switch. Configuration must not enable enforcement merely because the conversational agent is enabled. Document all defaults in `config.example.toml`.
5. Do not add SQL, agent memory, request history, provider, prompt, resource, or tool dependencies to the monitor.

### Stage B — authoritative executor prerequisite and composition gate

**Modify only after discovering the real target protocol operation and focused engine tests:**

- `internal/common/engine.go`
- concrete engine implementation and its focused tests under `internal/core/`
- `internal/agent/moderation/action.go`
- create `internal/agent/moderation/engine_executor.go` and tests

1. Locate the existing target protocol implementation for room captcha and shadow-ban before adding methods. Add typed `Engine` methods only if they delegate to that established implementation; the method names and wire shape must come from target source/tests, not this handoff.
2. `EngineActionExecutor.Execute` validates first, then performs exactly one operation. It receives a trusted bot/system context fixed at composition; it must not accept caller/model text, `api.Context`, tool calls, or command strings.
3. Return errors to the bridge after logging stable operation/reason metadata; never panic. Do not retry, fall back to `Ban`, run user commands, or fabricate a success result.
4. Keep executor construction unavailable when moderation is disabled or any required privileged engine operation is absent. This is the production fail-closed gate.

### Stage C — join listener bridge, only after A/B are green

**Modify:**

- `internal/listener/user_joined_listener.go`
- `internal/listener/user_joined_listener_test.go`
- composition owner(s) that build `UserJoinedListener` in `internal/factory/engine_factory.go` and focused tests, only if dependency injection requires it.

Introduce a minimal listener-owned collaborator rather than expanding `common.Engine` with agent internals:

```go
type JoinAutomation interface {
    OnJoin(context.Context, *model.User)
}
```

`Notify` must retain this order:

```go
u := parsed trusted user
l.e.AddActiveUser(u)
l.joinAutomation.OnJoin(context.Background(), u) // only if non-nil and enabled
l.shareUserInfo(u)
l.e.LogPresence(...)
```

The bridge obtains decisions from the monitor and dispatches them one at a time to the executor. It recovers no panic and does not return an error to the transport listener. It must not change the existing malformed-JSON return, active-user registration, subscriber payload, presence audit, or logging semantics.

## Trusted data flow and boundaries

```text
[server join payload]
  -> model.GetUser                         // parsing boundary
  -> UserJoinedListener.Notify
       -> Engine.AddActiveUser(user)        // source ordering retained
       -> JoinAutomation.OnJoin(ctx, user)
            -> JoinMonitor.OnJoin(user)     // trusted name/trip/hash; clock + private state
            -> validate Decision
            -> EngineActionExecutor.Execute // privileged, reviewed engine operations only
       -> existing shareUserInfo / LogPresence
```

| Boundary | Required behavior |
|---|---|
| Trust and visibility | Only parsed `*model.User` fields and composition-owned policy/config enter the monitor. Raw JSON, room chat text, LLM content, tool arguments, provider responses, and historical memory cannot influence an action. Reasons remain internal logs/audit metadata. |
| Authorization | Autonomous actions are system-owned, not caller-authorized. The monitor must receive a protected-principal predicate based on trusted local role/identity state. Do not reuse normal user-command authorization or derive authority from nick/trip text. |
| Cancellation and timeout | The listener has no request context from the server. The bridge uses a bounded composition-owned context/timeout solely for executor operations; monitor evaluation is synchronous, local, and cancellation-free. Cancellation/timeout means no late retry or fallback action. |
| Delivery | There is no agent conversational delivery. An executor action may have its own authoritative moderation/delivery effect; warning output must be fixed and at most once. No runtime sink, failure sink, relay, direct `l` reply, or ambient reply is used. |
| Persistence/audit | This vertical writes no agent conversation/tool evidence/memory. Existing presence logging remains in its original listener position. Add a moderation audit store only in a later dedicated audited-enforcement vertical; do not overload agent memory or message audit. |
| Protocol | No LLM protocol and no command-tool assistant/tool pair exists. Privileged wire/protocol calls must be supplied by proven engine methods, never string-built by the agent package. |
| Concurrency | `JoinMonitor` owns lock-protected bounded deques/maps. It copies only primitive normalized values into state and never retains `*model.User`. Executor calls occur outside the monitor lock so blocked I/O cannot serialize joins or mutate monitor state twice. |
| Failure isolation | A malformed join remains current behavior. Monitor validation/config failure or executor failure is logged and isolated; sharing/presence still proceeds. In disabled mode, the hook is a true no-op with no state allocation or privileged engine use. |

## RED → GREEN TDD plan

1. **RED — decision matrix:** in `internal/agent/moderation/join_monitor_test.go`, assert invalid targeted-empty and captcha-with-target decisions reject before executor access. Assert decisions are immutable/copy-safe from caller mutation.
2. **RED — threshold/window behavior:** inject a fixed clock. Prove disabled and protected joins return no decisions/no state effect; Nth join at `JoinBurstCount` emits exactly one room `Captcha`, clears the burst bucket, and joins outside the window are pruned. Confirm parsed `Name`, `Hash`, and `Trip` values survive only as required identity inputs.
3. **RED — source escalation:** table-drive same-hash distinct-nick threshold, suspicious-name-cluster threshold, and repeated same-hash signal inside/outside cooldown/post-kick windows. Assert exact action, target cardinality, reason class, and bucket reset. No executor/provider/engine test substitute is used here.
4. **GREEN A:** implement the monitor/config with only enough code to satisfy each test; run the focused package after every increment.
5. **RED — executor authority:** after real target operation discovery, fake an engine with counters. Assert captcha sends one room operation and no target; shadow-ban sends one target operation and never `Ban`; invalid decision/cancelled context performs zero calls; executor failure is returned once with no retry.
6. **GREEN B:** implement only typed engine delegation. Add focused core protocol tests that prove the emitted payload through the existing target transport seam. If those tests cannot be written from existing target protocol behavior, do not proceed to Stage C.
7. **RED — listener order/isolation:** update `user_joined_listener_test.go` with recording engine/automation. Assert `AddActiveUser` occurs before `OnJoin`, then subscriber/presence behavior remains; malformed JSON invokes neither. Make executor failure observable only in logs/recording collaborator and prove it does not suppress share/presence.
8. **GREEN C:** inject the optional bridge and call it exactly once after registration. Prove no `participation.Pipeline`, `runtime.Runtime`, `Runner`, LLM client, tool registry, memory repository, or sink interaction for a join.
9. **Race RED/GREEN:** concurrent joins under `go test -race` must not race monitor state and must never call the executor while holding the monitor lock. Use channels in test fakes; no time-based sleeps.

## Developer and senior routing

- **Developer:** Stage A after reviewing the source monitor tests and exact target `model.User` normalization behavior. It is deterministic, no-I/O policy work with strict injected-clock tests.
- **Senior engine/security owner (mandatory):** Stage B. Approve the real captcha/shadow-ban protocol owner, protected-principal predicate, executor authorization boundary, bounded context/timeout, and action/audit semantics before enabling production composition.
- **Senior listener/runtime reviewer:** Stage C. Verify source ordering after `AddActiveUser`, no listener-blocking behavior beyond the approved executor timeout, failure isolation, and no accidental admission into live agent runtime/provider/durable memory.
- **Independent QA:** replay focused monitor/executor/listener/race gates and inspect the diff for prohibited provider/tool/SQL/command-catalog changes.

## Rapid gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only files owned by this vertical after each stage:

```sh
gofmt -w \
  internal/agent/moderation/action.go \
  internal/agent/moderation/join_monitor.go \
  internal/agent/moderation/join_monitor_test.go \
  internal/agent/moderation/engine_executor.go \
  internal/agent/moderation/engine_executor_test.go \
  internal/listener/user_joined_listener.go \
  internal/listener/user_joined_listener_test.go

go test ./internal/agent/moderation -run 'Test(JoinMonitor|Decision|EngineActionExecutor)' -count=1
go test ./internal/listener -run 'TestUserJoinedListener' -count=1
go test ./internal/core -run 'Test.*(Captcha|ShadowBan|Moderation)' -count=1
go test -race ./internal/agent/moderation ./internal/listener -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go vet ./...` as an informational gate. If it continues to report only the known unrelated copylocks warning at `internal/core/engine_impl.go:95:22`, record it separately and do not expand this vertical to repair it.

## Explicit adaptations and exclusions

### Explicit adaptations

- Zenbot’s source-visible `common.Engine` currently lacks authoritative captcha/shadow-ban methods. A fail-closed executor prerequisite is therefore required; do not claim source parity before it exists and is tested.
- The source invokes the automation synchronously. Zenbot may apply an executor-only composition timeout to protect its listener, but must not add a queue, retry worker, or background best-effort action that reorders joins.
- The decision monitor is deterministic. Source semantic LLM moderation remains excluded even if the source room pipeline can create `MODERATION` invocations from messages.

### Excluded

- No changes to `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, Saturn source/resources, existing handoffs, commits, or pushes.
- No conversational direct/mention/ambient/relay behavior, bot-message ingress policy, participation order, runtime admission, provider client, completion count, output finalizer, command-prose channel, prompt, tool inventory, tool executor, memory/evidence, or persistence changes.
- No database/schema/dynamic SQL/query work, reflection command catalog, broad command gateway/dispatcher reuse, generic raw transport rewrite, or new user-facing configuration beyond the bounded join moderation settings.
- No `Ban` substitution, model/caller-provided targets, silent decision dropping, retries, unrestricted timers, or privileged moderation action without senior-approved engine protocol evidence.

## Completion checklist

- [ ] Every citation above resolves in the Saturn or Zenbot checkout; source facts and target recommendations are kept distinct.
- [ ] Monitor handles only trusted parsed joins, is bounded/thread-safe, and tests all threshold/window/protection transitions with an injected clock.
- [ ] Every action has an explicit target matrix and a real reviewed engine operation; unavailable operations keep production enforcement disabled.
- [ ] Join bridge runs after active-user registration, isolates failures, and preserves existing share/presence behavior.
- [ ] A join creates no LLM request, tool call, runtime admission, agent delivery, conversation append, or durable evidence.
- [ ] Focused, race, full test, build, and `git diff --check` gates are recorded by the implementing/QA handoffs.
