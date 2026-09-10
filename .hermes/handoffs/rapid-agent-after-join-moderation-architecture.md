# Next bounded Saturn agent parity vertical: deterministic public-message spam moderation

## Decision

**Select Saturn’s deterministic `RoomModerationMonitor.onMessage` path: observe each parsed public room message before participation filtering, issue only its bounded anti-spam decisions, and execute those decisions through a reviewed typed autonomous-moderation boundary.**

This is the highest-value remaining adjacent agent behavior after accepted join moderation. It completes the other half of Saturn’s one shared deterministic moderation monitor without starting a provider request: per-identity message bursts and repeated-message spam escalate `WARN → MUTE → KICK → SHADOWBAN`, with protected/whisper/disabled inputs excluded before monitor state changes. The existing target has only the accepted join half (`JoinMonitor`) and an inert participation `Monitor func(Event)` callback; `live.RoomParticipation` always sets `ModerationCandidate:false`, and no composition installs a message monitor.

This vertical is **not** source semantic/LLM moderation. The separate source regex-triggered `MODERATION` invocation is deferred: it exposes a model-visible severe-abuse prompt and tools, whereas this selected path is deterministic local state plus explicitly reviewed one-operation enforcement.

## Evidence map

### [OBSERVED] Saturn source contract

| Evidence | Observed behavior |
|---|---|
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `onMessage`, `monitorModeration`, `filterIneligible` | The first handler calls `moderationMonitor.onMessage(message)` and executes all decisions. Only after that does the pipeline strip/filter blank, whisper, self, bot, and prefix input. Thus commands and otherwise-ineligible public text remain observable to deterministic moderation, but whispers do not. |
| Same file, `handleSemanticModeration` | Semantic moderation is a separately gated later handler which submits `AgentInvocationMode.MODERATION` only for a fixed severe-abuse pattern and candidate predicate. It is not part of `RoomModerationMonitor.onMessage`. |
| `src/main/java/org/saturn/app/agent/moderation/RoomModerationMonitor.java`, `onMessage`, `escalate`, `allow` | An enabled, non-whisper, non-protected message uses trusted message identity and normalized body. Per-identity queues retain only the maximum of burst/repetition windows. At a threshold, the queue clears, action cooldown is checked, and offence state transitions from warning to mute to kick to shadow-ban within source windows. No `BAN` action exists. |
| Same file, `normalizeMessage`, `within` | Normalization changes literal `\\n` and LF to spaces, strips Java whitespace, lower-cases with `Locale.ROOT`, and collapses whitespace. Window endpoints are inclusive (`elapsed <= window`). |
| `src/main/java/org/saturn/app/agent/moderation/AgentModerationConfig.java` | Source owns explicit positive message burst count/window, repeated-message count/window, second-breach window, post-kick window, and action cooldown alongside accepted join settings. |
| `src/main/java/org/saturn/app/agent/moderation/EngineModerationActionExecutor.java`, `execute`, `warn` | `WARN` emits exactly `Please stop flooding.` to the target; `MUTE`, `KICK`, and `SHADOWBAN` use autonomous bot-owned moderation operations. Invalid/null/runtime failures are contained and return failure rather than affecting message routing. |
| `src/main/java/org/saturn/app/agent/room/AgentRoomAutomationFactory.java`, `create` | One monitor is composed with a system clock and protected-message predicate. The predicate derives protection from trusted local role/identity state, not model text. |
| `src/test/java/org/saturn/app/agent/moderation/RoomModerationMonitorTest.java`, `escalatesBurstRepeatedSpamAndPostKickReoffenceWithoutPermanentBan`, `excludesProtectedUsersBeforeRecordingDetectionState`, `ignoresWhispersAndPrunesExpiredMessageHistory` | Tests establish escalation ordering, no permanent ban, protection-before-state, whisper exclusion, expiry, cooldown, and exact window behavior. |
| `src/test/java/org/saturn/app/agent/room/DefaultAgentRoomAutomationTest.java`, `monitorsCommandsAndJoinEventsBeforeParticipationFiltering`, `toleratesModerationActionRejectionWithoutChangingMessageRouting` | A prefix command still generates `WARN` before participation returns `PASS`; a rejected action neither claims nor submits the message. |

### [OBSERVED] accepted Zenbot state and precise gap

| Evidence | Existing behavior / gap |
|---|---|
| `.hermes/handoffs/rapid-agent-join-moderation-qa.md` | The accepted join vertical is deterministic, protected, typed, timeout-bounded, and isolated from conversational runtime/provider/tool/memory paths. It implements only room captcha and persisted shadow-ban actions used by joins. |
| `internal/agent/moderation/join_monitor.go`, `OnJoin`; `internal/agent/moderation/automation.go`, `Automation.OnJoin` | Target owns bounded join-only state and per-decision executor timeout. There is no public-message monitor or bridge. Reusing join buckets for message spam would violate their identity/data contract. |
| `internal/agent/participation/invocation.go`, `Pipeline.Handle` | The pipeline invokes its optional `Monitor(e)` before its eligibility filter, then can route mention, moderation candidate, or ambient work. The callback is void/inert and has no action executor, configuration, timeout, or factory wiring. This is the exact ordering seam for the selected bridge. |
| `internal/agent/live/participation.go`, `RoomParticipation.Handle` | The live adapter passes parsed message, resolved author metadata, snapshot, nick, and prefix, but hard-codes `ModerationCandidate:false`; it does not install `Pipeline.Monitor`. |
| `internal/listener/message/handlers.go`, `ResolveUserMetadata`, `DefaultChainWithParticipation` | Parsed server chat is resolved against active users before agent participation. The selected deterministic monitor must consume the already parsed `model.ChatMessage` and resolved author/protection predicate; it must not reparse JSON or scan untrusted text for privilege. |
| `internal/config/agent_config.go`, `AgentConfig.Validate` | Join moderation config is explicit and validated only when `moderationEnabled`; the source-required message thresholds/windows are absent. |
| `internal/agent/moderation/action.go`, `Decision.Validate`; `engine_executor.go`, `EngineActionExecutor.Execute` | The action vocabulary already contains warning/mute/kick, but the executor intentionally enables only captcha and shadow-ban and reports the other actions unavailable. |
| `internal/core/engine_impl.go`, `EnableCaptcha`, `ShadowBan`, `Kick` | Captcha/shadow-ban are accepted typed joins seams. Existing `Kick` is a legacy void public engine method that raw-formats JSON and takes a caller-supplied channel; it is not an adequate autonomous boundary. No typed mute or fixed-warning operation exists. |

### [LIMITATION]

Zenbot cannot claim parity merely by enabling the existing `Pipeline.Monitor` callback: it has no message-state monitor, no message moderation config, and no authoritative `WARN`/`MUTE`/`KICK` executor operations. Do **not** route decisions through `DispatchUserCommand`, `run_command`, the model, generic raw payload strings, `Ban`, or a best-effort background worker. Those paths change authority, authorization, delivery, cancellation, and protocol meaning.

## Alternatives considered

| Candidate | Decision | Reason |
|---|---|---|
| **Deterministic public-message spam moderation** | **Selected** | Exact sibling of accepted join moderation, source-tested, user-observable, and closes the source monitor’s existing ordered public-message behavior without provider work. |
| Semantic severe-abuse `MODERATION` LLM submission | Deferred | Requires fixed candidate/signal policy, trusted bot moderation context, provider admission/cancellation/delivery semantics, and model-visible authority. It must follow—not merge with—deterministic local enforcement. |
| Extend join detector thresholds/actions | Reject | Accepted join scope already covers its source contract; mixing event types would regress isolated bounded state. |
| Generic user-command dispatch for mute/kick/shadowban | Reject | Source executor has system-owned authority; user dispatcher has caller authorization, reply, parsing, and command catalog semantics. |
| Provider retry/correction/third completion | Excluded | The selected monitor produces no completion. |
| Dynamic DB/SQL, reflection catalog, transport rewrite | Excluded | No source behavior in this vertical needs them beyond the already accepted typed shadow-ban persistence. |

## Exact contract and adaptation

### Source behavior retained

For every successfully parsed inbound message in the normal listener chain:

```text
parsed ChatMessage + resolved local protection state
  -> deterministic public-message monitor
  -> zero or one structurally valid decision
  -> bounded attempt through authoritative executor
  -> existing participation eligibility / command routing
```

1. The monitor is called **before** agent participation’s blank/whisper/self/bot/prefix eligibility return, preserving Saturn’s command-observation order.
2. The monitor itself returns no decision and mutates no monitor state when disabled, whisper, protected, empty-after-normalization, or malformed/unusable identity input.
3. A threshold clears that identity’s retained message queue before the next signal. It emits at most one escalation decision for that input.
4. Escalation is source-shaped: first threshold `WARN`; a new offence within `secondBreachWindow` `MUTE`; another within that window `KICK`; another within `postKickWindow` `SHADOWBAN`; an already shadow-banned identity produces no action. A decision is action-cooldown suppressed by action plus identity.
5. A decision attempts one authoritative effect, outside the monitor lock. Failure is logged/contained and does not claim, reply to, suppress, or otherwise alter the inbound message’s current participation/command outcome.
6. No permanent ban fallback exists.

### [RECOMMENDED] bounded target adaptation

Keep message monitoring as a separate `MessageMonitor`; do not stretch `JoinMonitor` into a multi-event state bag. Both may share only small unexported normalization/window helpers after behavior-first tests prove parity.

Use existing trusted parsed data only. The identity key is stable priority `hash`, then `trip`, then a case-folded normalized nick only if both trusted identity fields are absent; document this explicit target adaptation because Saturn’s `AgentUserIdentity.from(ChatMessage)` must be inspected at implementation time before exact fallback text is frozen. Never use message body as identity. A malformed blank sender is a no-op.

Complete the prerequisite privileged operation boundary in the same implementation scope:

- `WARN`: one fixed non-whisper target delivery, exactly `Please stop flooding.`; decision reason never becomes delivered content.
- `MUTE`: one typed authoritative `mute` operation for the normalized active principal.
- `KICK`: one typed authoritative kick-to-current-room operation for the normalized active principal.
- `SHADOWBAN`: retain the accepted typed active-user persistence operation and no `Ban` fallback.

The concrete core owner must produce JSON through a single safe serializer / established outbound helper, not legacy `fmt.Sprintf` raw payload construction. Payload contract must be established in focused protocol tests from Saturn `ModServiceImpl.mute` and `kick` (`{"cmd":"mute","nick":...}` and `{"cmd":"kick","nick":...}` after source nick normalization). The target’s current legacy `Kick` must remain untouched unless a narrowly reviewed refactor proves it can delegate safely without changing user-command behavior.

## Target files, interfaces, and data flow

### Stage A — pure message monitor and configuration

**Create**

- `internal/agent/moderation/message_monitor.go`
- `internal/agent/moderation/message_monitor_test.go`

**Modify**

- `internal/agent/moderation/action.go` (only shared decision validation if necessary)
- `internal/config/agent_config.go`
- `internal/config/agent_config_participation_test.go` or focused new config test
- `config.example.toml`

Define a narrow local contract:

```go
type MessageMonitor interface {
    OnMessage(model.ChatMessage) []Decision
}

type MessageConfig struct {
    Enabled                bool
    BurstCount             int
    BurstWindow            time.Duration
    RepeatedCount          int
    RepeatedWindow         time.Duration
    SecondBreachWindow     time.Duration
    PostKickWindow         time.Duration
    ActionCooldown         time.Duration
}
```

Requirements:

- Inject `now func() time.Time` and `protected func(model.ChatMessage) bool`; no engine, listener, repository, provider, runtime, tool, or prompt dependency.
- Own a mutex, identity-keyed bounded timestamp/message queues, offence state, and action cooldown map. Prune all expired identity state on every eligible evaluation so distinct historical identities cannot accumulate indefinitely.
- Return an allocated/copy-safe empty slice for no decision and a fresh decision copy; do not retain `*model.User`, `*ChatMessage`, raw JSON, or caller-owned maps/slices.
- Add explicit `agent.moderationMessageBurstCount`, `moderationMessageBurstWindowSeconds`, `moderationRepeatedMessageCount`, `moderationRepeatedMessageWindowSeconds`, and `moderationSecondBreachWindowSeconds`. When `moderationEnabled=true`, all existing join and new message values must be positive. Keep moderation off by default; enabling conversational agent must not imply enforcement.

### Stage B — privileged message executor prerequisite

**Create**

- `internal/agent/moderation/message_executor.go`
- `internal/agent/moderation/message_executor_test.go`

**Modify only as proven by focused protocol tests**

- `internal/core/engine_impl.go`
- focused `internal/core/*moderation*_test.go`
- `internal/agent/moderation/engine_executor.go` and test, or replace it with a single reviewed executor that preserves accepted join behavior

Do not enlarge `common.Engine` with agent internals. Compose against a narrow internal interface implemented by `*core.EngineImpl`:

```go
type AuthoritativeMessageModerator interface {
    WarnFlood(context.Context, string) error
    MutePrincipal(context.Context, string) error
    KickPrincipal(context.Context, string) error
    ShadowBan(context.Context, string) error
}

type MessageActionExecutor interface {
    Execute(context.Context, Decision) error
}
```

`Execute` must validate decision/action-target shape and context first. It accepts no raw command, `api.Context`, provider result, tool argument, or user authorizer. Each valid decision performs exactly one operation; cancelled/expired context performs zero operation; errors return once with no retry, `Ban`, or alternate action. The existing join executor remains callable for `Captcha`/`ShadowBan`, but it must not silently become a general dispatcher.

### Stage C — message bridge composition and ordering

**Modify**

- `internal/agent/participation/invocation.go`
- `internal/agent/live/participation.go`
- `internal/factory/engine_factory.go`
- `internal/agent/live/participation_test.go`
- `internal/agent/participation/policies_test.go`
- focused `internal/factory/engine_factory_test.go`

Adapt the existing pre-filter callback into an explicit observer bridge rather than a provider routing condition:

```go
type MessageAutomation struct {
    Monitor  moderation.MessageMonitor
    Executor moderation.MessageActionExecutor
    Timeout  time.Duration
}

func (a *MessageAutomation) Observe(parent context.Context, e participation.Event)
```

`RoomParticipation` installs `Pipeline.Monitor` only when the factory has complete enabled moderation config and all authoritative operations. The callback receives the immutable event value, calls `Monitor.OnMessage(e.Message)`, then derives a composition-owned `context.WithTimeout` **per decision** (two seconds, matching accepted join automation) and logs a stable action/target/reason-class failure. It does not return an error to `Pipeline.Handle`, mutate event fields, or touch `Submit`.

Factory composition must use a shared trusted protected-principal predicate compatible with accepted join moderation: self, trusted `IsBot`, configured creator trip, configured admin trips, and local bot nick are protected. Message `Trip`/resolved `Author` may be consulted only from listener-resolved data. If message-author resolution is missing, the predicate must never manufacture privileged identity from text; preserve the monitor’s stable parsed identity fallback and ordinary routing.

### Proposed end-to-end flow

```text
[OBSERVED ordering; proposed target bridge]
server chat payload
  -> UserChatListener.Notify parses model.ChatMessage
  -> message.ResolveUserMetadata (trusted active-user metadata)
  -> existing audit/relay/downstream chain
  -> live.RoomParticipation.Handle
       -> participation.Pipeline.Handle
            -> MessageAutomation.Observe(event)              // NEW, first
                 -> MessageMonitor.OnMessage(parsed message) // local state only
                 -> Decision.Validate
                 -> MessageActionExecutor.Execute(timeout)
            -> existing eligibility branch / quiet / mention / ambient
  -> existing command dispatcher when participation returns PASS
```

## Boundary contract

| Boundary | Required behavior |
|---|---|
| Visibility/trust | The monitor receives parsed `model.ChatMessage` plus composition-owned protected predicate only. Raw payload JSON, model output, tool arguments, memory, historical evidence, and command text as authority inputs are excluded. Message content is detector input only; it cannot choose target/action. |
| Authentication/authorization | Autonomous action authority is fixed at factory composition. Do not call `IsUserAuthorized`, user command dispatch, or model capability calculation. Protection uses trusted local identity/role state and must be checked before any state mutation. |
| Cancellation/timeout | Monitor evaluation is local and synchronous. Each executor action gets a two-second composition-owned child context. Cancellation/deadline yields no late retry, no queue worker, and no fallback. |
| Delivery | `WARN` only uses fixed source text once. `MUTE`, `KICK`, and `SHADOWBAN` use their authoritative effects. There is no conversational agent reply, failure-sink response, direct `l` output, relay, or duplicate command acknowledgment. |
| Persistence | No agent conversation, memory, durable evidence, tool evidence, or message schema write is introduced. Shadow-ban continues only through accepted typed repository persistence. Offence state is process-local and intentionally resets at lifecycle replacement, as Saturn monitor state does. |
| Protocol | No LLM/tool protocol occurs. The core moderation owner serializes mute/kick payloads safely and proves their wire shape; agent packages never construct protocol strings. |
| Concurrency | Monitor state is mutex-owned and bounded/pruned. Executor I/O happens after the lock is released. The existing atomic ambient counter runs later and is unaffected. No new goroutine, timer, queue, or background retry is introduced. |
| Failure isolation | Malformed JSON retains existing listener behavior. Disabled/protected/whisper/empty monitor input is a no-op. Invalid decision/config/composition/executor failure is logged and contained; agent participation and command chain behavior continues unchanged. |

## RED → GREEN plan

1. **RED — normalization and no-state inputs:** in `message_monitor_test.go`, injected clock tests prove disabled, protected, whisper, blank-after-source normalization, and blank identity return no decisions and no retained state. Cover literal `\\n`, LF, Unicode/whitespace, case folding, and inclusive cutoff edges.
2. **RED — exact detector thresholds:** table-drive burst versus repeated body behavior. Prove threshold input returns exactly one decision and clears that identity queue; a message outside each window is pruned and does not count. Assert distinct identity queues never cross-trigger.
3. **RED — escalation/cooldown:** prove source action sequence `WARN`, `MUTE`, `KICK`, `SHADOWBAN`, then none; prove second-breach/post-kick expiry restarts at warning; prove action cooldown suppresses action without producing `BAN`; prove concurrent messages are race-safe and no executor is invoked from monitor tests.
4. **GREEN A:** implement only local monitor/config needed by these tests. Run `go test ./internal/agent/moderation -run TestMessageMonitor -count=1` after each increment.
5. **RED — authoritative executor:** core tests prove a validated warning makes one fixed non-whisper delivery, mute emits exactly one safely serialized `mute` payload, kick emits exactly one safely serialized current-room kick payload, and shadow-ban reuses the accepted typed store. Invalid/cancelled/timeout decisions make zero calls; each engine failure returns once with no retry/Ban/command dispatch.
6. **GREEN B:** add only reviewed typed engine methods and executor routing. Do not reuse the existing raw `Kick` method as evidence without protocol tests.
7. **RED — ordering and isolation:** pipeline/live tests install a recording `MessageAutomation`. For a public prefix command that reaches `Pipeline.Handle`, assert observation happens once before participation filter, decisions execute before `PASS`, and `Submit`/ambient counter/provider-facing fake remain untouched. For whisper/protected input, assert observation calls but executor stays zero. For executor error, assert the same `PASS`/`CLAIMED` outcome as without enforcement and the listener chain still reaches command dispatch as applicable.
8. **GREEN C:** factory wires automation only with enabled complete config plus all required core/repository capabilities; otherwise it installs no monitor (not a partially dropping one). Confirm joins retain accepted behavior and live agent construction has no provider/runtime changes.
9. **Race regression:** `go test -race ./internal/agent/moderation ./internal/agent/participation ./internal/agent/live -count=1`; channel-controlled executor fakes prove I/O cannot occur while the monitor lock is held.

## Risk routing and rapid gates

### Routing

- **Developer:** Stage A and narrow live callback plumbing after reviewing source tests. No provider or persistence work.
- **Senior engine/security owner (mandatory):** Stage B must approve typed mute/kick/warn operation ownership, current-room target semantics, safe payload construction, protected-principal policy, timeout, and explicit no-`Ban` guarantee.
- **Senior listener/agent reviewer:** Stage C verifies `Monitor` remains first in `Pipeline.Handle`, action failure does not alter chain continuation, and no semantic moderation/provider admission is introduced.
- **Independent QA:** replay exact escalation/window/order tests; inspect diff for command-dispatch, raw agent protocol, dynamic SQL, model/tool/runtime, retry-worker, or protected-document scope creep.

### Gates

Run from `/Users/ab/workspace/go-projects/zenbot`:

```sh
gofmt -w \
  internal/agent/moderation/message_monitor.go \
  internal/agent/moderation/message_monitor_test.go \
  internal/agent/moderation/message_executor.go \
  internal/agent/moderation/message_executor_test.go \
  internal/agent/moderation/engine_executor.go \
  internal/agent/live/participation.go \
  internal/agent/live/participation_test.go \
  internal/agent/participation/invocation.go \
  internal/agent/participation/policies_test.go \
  internal/factory/engine_factory.go

go test ./internal/agent/moderation -run 'Test(MessageMonitor|MessageActionExecutor|EngineActionExecutor)' -count=1
go test ./internal/agent/participation -run 'TestPipeline.*(Monitor|Moderation|Prefix|Whisper)' -count=1
go test ./internal/agent/live -run 'TestRoomParticipation.*Moderation' -count=1
go test ./internal/core -run 'Test.*(Moderation|Mute|Kick|Captcha|ShadowBan)' -count=1
go test ./internal/factory -run 'TestCompose.*Moderation' -count=1
go test -race ./internal/agent/moderation ./internal/agent/participation ./internal/agent/live -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go vet ./...` informationally. If the only result remains the known unrelated copylocks warning at `internal/core/engine_impl.go:95:22`, record it without repairing that unrelated code.

## Explicit adaptations and exclusions

### Adaptations

- Zenbot’s accepted join monitor is split from source’s shared monitor to retain a bounded event-specific state ownership model; parity is behavioral for each source event contract, not identical class shape.
- The existing participation callback is used only to preserve Saturn ordering. It becomes a factory-owned autonomous bridge, never a model moderation candidate or submission path.
- A two-second executor-only deadline is retained from accepted join moderation to prevent listener I/O from blocking indefinitely. Source synchronous ordering remains; no asynchronous reordering is added.

### Exclusions

- No semantic severe-abuse pattern, `MODERATION` invocation, provider completion, prompt, model-visible moderation target, tool definition/execution, correction/retry, third completion, output finalizer, or quote/evidence behavior.
- No user command dispatcher/gateway, generic `run_command`, user authorization reuse, reflected catalog, or raw agent-created transport payload.
- No broad transport rewrite, SQL/schema/dynamic database work, audit schema, new agent memory/evidence persistence, background worker, retry queue, timer, or lifecycle redesign.
- No change to direct `l`, mention, ambient, relay, bot ingress, listener handler order outside the required existing pipeline callback, command output contracts, accepted join detector behavior, Saturn source/resources, `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, existing handoffs, commits, or pushes.
- No `Ban` fallback, model/caller-selected target/action/reason, partial factory composition, silent decision dropping, or unbounded retained identity state.

## Completion checklist

- [ ] Source citations resolve and source observations are distinct from target recommendations.
- [ ] Every source message threshold/window/normalization/escalation/protection/cooldown behavior has a focused deterministic test.
- [ ] Public message observation precedes participation filtering; whisper/protected detection is a no-op; an action error changes neither claim nor downstream command behavior.
- [ ] Warning, mute, kick, and shadow-ban each have one proven typed effect, zero raw agent protocol construction, zero `Ban` fallback, and bounded cancellation behavior.
- [ ] No message reaches provider/tools/runtime/memory/delivery through this deterministic branch; no new conversation/evidence persistence occurs.
- [ ] Factory fails closed for disabled/incomplete config or unavailable mandatory privileged operations.
- [ ] Focused, race, full test, build, vet-record, and `git diff --check` results are recorded by later implementation/QA handoffs.
- [ ] This architecture task created only `.hermes/handoffs/rapid-agent-after-join-moderation-architecture.md`.
