# Next rapid Saturn → Zenbot parity slice: semantic severe-abuse moderation ingress

## Decision

**Select exactly one adjacent behavior: Saturn’s silent semantic severe-abuse path in `AgentRoomMessagePipeline.handleSemanticModeration`.** After deterministic message-spam observation has completed and normal message eligibility/mention handling has run, a non-protected public message matching Saturn’s fixed severe-abuse signal is submitted as one `MODERATION` invocation for the bot/system principal. It is not a conversational reply and does not claim the listener event.

The accepted deterministic public-message spam QA **permits moving to this slice**. Its verdict is PASS, its bridge preserves the source’s pre-filter deterministic ordering, and it explicitly excludes semantic/provider moderation rather than identifying a defect that blocks it. This is the next adjacent source handler in the same ordered Saturn pipeline; it should not be merged with more spam heuristics, provider correction/retry, broad command migration, or agent redesign.

This slice unavoidably uses the **already-composed** live provider/runtime path because Saturn’s selected behavior is an LLM `MODERATION` invocation. It must not add a provider, model, completion retry, prompt catalog, memory system, or unrelated command path. The only new authority work is the narrow prerequisite described below: a source-shaped, mode-scoped moderator action boundary for the existing `run_command` concept.

## Source evidence

### [OBSERVED] Saturn behavior

| Source path and symbol | Evidence / retained contract |
|---|---|
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `handlers`, `onMessage` | Handler order is deterministic: `monitorModeration` → `filterIneligible` → `prepareInvocation` → `handleQuietRequest` → `handleMention` → `handleSemanticModeration` → `handleAmbientParticipation`. `PASS` stops the pipeline and `CLAIMED` stops it as a claimed event. |
| Same file, `filterIneligible` | Blank, whisper, self, bot, and prefix-command input returns `PASS` before semantic moderation. Deterministic spam observation remains earlier. |
| Same file, `handleMention` | A parsed mention is submitted as `MENTION` and returns `CLAIMED`; it therefore does **not** reach semantic moderation. |
| Same file, `handleSemanticModeration` | Semantic processing requires a non-null bot moderation context, `semanticModerationCandidate.test(message)`, and a match of `SEMANTIC_MODERATION_SIGNAL`; it builds a public bot context, uses the current message nick as target, submits `AgentInvocationMode.MODERATION`, and returns `CONTINUE` rather than claiming the event. |
| Same file, `SEMANTIC_MODERATION_SIGNAL` | The fixed Java pattern is `(?iu)\\b(?:kys|kill\\s+(?:yourself|urself|u|you)|hang\\s+(?:yourself|urself)|doxx?\\b|swat(?:ting)?\\b|rape\\b|shoot\\s+you|stab\\s+you|bomb\\s+(?:you|them|the room))`. It is a candidate gate, not a model-selected policy. |
| `src/main/java/org/saturn/app/agent/room/AgentRoomAutomationFactory.java`, `create` | The bot context is public, uses engine room/nick, configured creator trip, no hash, current room users, and exactly `MODERATION_COMMANDS`. The candidate predicate is gated by `AgentModerationConfig.enabled()` and protected-principal policy. |
| `src/main/java/org/saturn/app/agent/api/AgentInvocationMode.java`, `MODERATION`, `requiresReply` | `MODERATION(false)` is a silent mode. |
| `src/main/java/org/saturn/app/agent/routing/AgentRequestAssembler.java`, `assemble`, `definitions` | Moderation skips required-fresh history lookup and retains only `run_command` definitions before command-intent filtering. |
| `src/main/resources/agent/system/participation-moderation.txt` | The system instruction is a silent review; it says severe targeted abuse may mute the author, lesser/ambiguous cases do nothing, permanent bans are forbidden, and final content is the no-reply marker because command delivery owns any effect. |
| `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java`, `MODERATION_COMMANDS`, `allowedCommands` | In a moderator-capable context, source `run_command` exposes `captcha`, `mute`, `unmute`, `kick`, `shadowban`, and `unshadowban`; it is not the normal public-information-only tool contract. |
| `src/test/java/org/saturn/app/agent/room/DefaultAgentRoomAutomationTest.java`, `doesNotSpendSemanticModerationOnOrdinaryMessagesOrMentions` | Ordinary text produces no semantic invocation; an otherwise candidate-like mention remains only `MENTION`. |
| Same test, `dispatchesSevereSemanticModerationSignalsToTheModerationMode` | `I will doxx you` produces exactly one `MODERATION` invocation whose prompt contains `doxx`. |

### [OBSERVED] Zenbot seams and current gap

| Target path and symbol | Existing seam / exact gap |
|---|---|
| `internal/agent/participation/invocation.go`, `Event.ModerationCandidate`, `(*Pipeline).Handle` | The target already has a dormant `ModerationCandidate` branch after mention handling: it calls `p.submit(..., api.MODERATION, false, Pass)`. Thus its placement preserves source mention precedence and non-claiming `PASS` semantics **if** the candidate is trustworthy. No live producer sets it. |
| `internal/agent/live/participation.go`, `RoomParticipation.Handle` | The live adapter currently hard-codes `ModerationCandidate: false`; this is the immediate ingress gap. |
| `internal/agent/participation/invocation.go`, `InvocationFactory.Create` | Current factory always creates context from the message author and grants no moderation capability when `mode == api.MODERATION`. That is intentionally wrong for Saturn semantic moderation, which requires a bot/system context and an explicit moderation target. |
| `internal/agent/api/api.go`, `NewContextWithModerationTarget`, `Context.ModerationTarget`; `internal/agent/runtime/contracts.go`, `Context.ModerationTarget` | Both exact API and runtime compatibility context types already carry a moderation-target field, but current live construction never populates it. This is the existing typed target seam. |
| `internal/agent/runtime/api_bridge.go`, `APIBridge.Submit` | All non-ambient modes, including `MODERATION`, use existing bounded runtime admission and cancellation. No new worker topology is required. |
| `internal/agent/assemble/assemble.go`, `SystemPrompt.Render`, `AssembleWithHistoricalEvidence`, `filterTools` | Zenbot already renders `resources/agent/system/participation-moderation.txt`, skips freshness for `MODERATION`, excludes non-`run_command` tools for that mode, and excludes `MODERATION` from quote-only delivery. The prompt resource equals the observed Saturn moderation instruction. |
| `internal/agent/live/runner.go`, `OutputFinalizer.FinalizeWithContext`, `quoteOnlyRequired`; `internal/agent/runtime/runtime.go`, `execute` | Existing `MODERATION` has no required reply. A successful no-reply finalization produces `ShouldReply=false`; runtime neither sends a chat answer nor calls `AfterDelivery`. A failure does not use `FailureSink` because the mode does not require reply. |
| `cmd/zenbot/main.go`, `newLiveAgent` | Existing live composition constructs one fixed public tool loop and one `RoomParticipation`, but wires only deterministic `MessageAutomation` into `Pipeline.Monitor`; it does not compose semantic candidate policy. |
| `internal/agent/live/tool_loop.go`, `NewBoundedToolLoop`; `internal/agent/tool/run_command.go`, `RunCommand.Descriptor`, `RunCommand.Execute`; `internal/command/agent_gateway.go`, `agentCommandGateway.Execute` | Target `run_command` is frozen to three public tools and allows only informational aliases. It cannot execute source moderation aliases, and its gateway performs caller active-user authorization. It must **not** be reused as autonomous bot authority. |
| `internal/core/engine_impl.go`, `MutePrincipal`, `KickPrincipal`, `ShadowBan` | The accepted deterministic-spam vertical has typed, active-principal-checked mute/kick/shadow-ban operations. They are useful authoritative operation seams, but are not currently exposed through a semantic-model action boundary. |
| `internal/factory/engine_factory.go`, `ComposeMessageAutomation` | Current composition already constructs trusted protected-principal logic using engine self/bot state, configured creator/admin trips, and active users. Semantic candidate composition needs the same source policy, not a new text-derived role check. |

## Exact scope and narrow prerequisite

### Selected behavior

The delivered behavior is the source semantic handler, including these observable outcomes:

```text
parsed public chat
  -> accepted deterministic message-spam observer (already present)
  -> existing eligibility / quiet / mention logic
  -> fixed severe-abuse candidate + protected-principal gate       // NEW
  -> one bot-context MODERATION invocation, or no invocation       // NEW
  -> existing bounded MODERATION execution / silent completion
  -> existing ambient branch continues; listener event remains PASS
```

The candidate must not submit for an ordinary message, whisper, self/bot/protected author, prefix command, quiet request, or addressed mention. A qualifying public non-mention must create one silent moderation invocation **and may still advance through the existing ambient branch**, matching Saturn’s `CONTINUE` result after `agentService.submit`.

### Required narrow prerequisite vertical — not a permanent blocker

Before a production semantic invocation may be enabled, implement and accept one **mode-scoped autonomous moderation command boundary**:

```text
MODERATION invocation + trusted bot context
  -> source-shaped run_command moderation allowlist
  -> typed authoritative operation for selected command/target
  -> no user authorization, no caller impersonation, no textual protocol construction
```

This is narrower than broad moderator-command parity: it owns only `run_command` when `Invocation.Mode()==MODERATION`, is unavailable for `DIRECT`/`MENTION`/`AMBIENT`, and cannot alter user command dispatch. It must revalidate exact source aliases (`captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban`) against current Zenbot command/protocol seams before activation. Existing target evidence supports typed `mute`, `kick`, and `shadow-ban`; target definitions/usages for `unmute` and `unshadowban` are not present in the inspected source, so those cannot be fabricated or silently accepted.

**Recommended rapid sequencing:** first implement a fail-closed `MODERATION` tool loop that exposes no action until every source command in the selected source allowlist has a reviewed typed operation. Then enable candidate ingress. If the senior owner chooses an explicitly documented compatibility adaptation to a mute-only semantic action based on the source moderation prompt, that must be separately approved and must not be called full `RunCommandTool.MODERATION_COMMANDS` parity. This is a narrow prerequisite vertical, not a reason to defer semantic moderation indefinitely.

## Proposed file and module map

### Stage A — pure candidate and bot-context invocation construction

**Create**

- `internal/agent/participation/semantic_moderation.go`
- `internal/agent/participation/semantic_moderation_test.go`

**Modify**

- `internal/agent/participation/invocation.go`
- `internal/agent/participation/policies_test.go`
- `internal/agent/live/participation.go`
- `internal/agent/live/participation_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/engine_factory_test.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

Do not add a config switch in this stage: the source candidate is gated by the already-existing `agent.moderationEnabled`, and Zenbot validates that setting and its deterministic moderation values in `internal/config/agent_config.go`. A new independent semantic flag would diverge from source behavior without evidence.

Recommended private interfaces/types:

```go
// participation: pure candidate policy; exact signal is owned here.
type SemanticModerationCandidate func(model.ChatMessage) bool

type SemanticModerationPolicy interface {
    Candidate(model.ChatMessage) bool
}

// live: only listener-resolved metadata crosses this adapter boundary.
type RoomParticipation struct {
    // existing fields...
    SemanticCandidate func(*message.Context) bool
}

// participation.Event additions, retaining Event as an immutable value:
type Event struct {
    // existing fields...
    ModerationCandidate bool
    ModerationTarget    string // canonical active principal, empty means no candidate
}
```

Do not expose the regex through configuration, prompts, tools, or model input. Use a package-private compiled Go regexp and table-driven source-string tests. The live adapter derives `ModerationTarget` only from resolved active-user metadata when present; if resolution is absent, retain parsed nick only if the authoritative moderator operation will resolve it at execution. It must never derive a target from model output.

Refactor `InvocationFactory.Create` only enough to make `api.MODERATION` a distinct source-compatible construction:

```go
// For MODERATION only:
api.NewContextWithModerationTarget(
    snapshot.Room,
    event.BotNick,              // engine/bot principal, not alleged author
    snapshot.CreatorTrip,
    "",                         // source bot context has no hash
    false,
    snapshot.Users,
    []api.Capability{api.ModerationCommands},
    event.ModerationTarget,
)
```

For `DIRECT`, `MENTION`, and `AMBIENT`, preserve the existing caller-derived context/capability behavior exactly. The factory must reject blank semantic target/bot nick or an impossible mode/context combination before runtime submission.

### Stage B — prerequisite `MODERATION` action loop

**Create** (names are proposed; keep actual names narrow and mode-specific)

- `internal/agent/tool/moderation_run_command.go`
- `internal/agent/tool/moderation_run_command_test.go`
- `internal/agent/commandgateway/moderation_gateway.go`
- `internal/agent/commandgateway/moderation_gateway_test.go`

**Modify only when source operation evidence is confirmed**

- `internal/agent/live/tool_loop.go`
- `internal/agent/live/tool_loop_test.go`
- `internal/core/engine_impl.go` and focused `internal/core/*moderation*_test.go`, only for a missing source-command operation
- `cmd/zenbot/main.go` and `cmd/zenbot/live_agent_test.go`

Module boundaries:

```go
// The model may select an allowed source command, never the principal.
type ModerationGateway interface {
    ExecuteModeration(context.Context, api.Context, string) (commandgateway.Execution, error)
}

// The descriptor has an enum limited to source-approved aliases and no
// arbitrary arguments that can replace Context.ModerationTarget.
type ModerationRunCommand struct { Gateway ModerationGateway }
```

`ModerationRunCommand` must require all of:

1. `caller.HasCapability(api.ModerationCommands)`;
2. non-nil/nonblank `caller.ModerationTarget()` for targeted actions;
3. public, bot-originated moderation context—not a human caller context;
4. one action only, current request context only, and zero raw JSON built in agent/tool code;
5. zero fallback to `Ban`, generic `DispatchUserCommand`, normal `command.NewAgentCommandGateway`, or user authorization.

Build a separate `NewModerationToolLoop` rather than weakening `NewBoundedToolLoop`. It may preserve the current one-call/two-completion execution limit, but it must use the moderation-only tool inventory and source-compatible silent completion contract. Do not make public `run_command` aliases broader.

### Stage C — composition and lifecycle enablement

`cmd/zenbot/main.go:newLiveAgent` composes semantic policy only when all are true:

1. live agent is enabled;
2. `agent.moderationEnabled` is true and currently valid;
3. the target is a real `*core.EngineImpl` with the same trusted protected-principal source as deterministic message moderation;
4. the prerequisite moderation tool loop/gateway is complete; and
5. the bot principal, creator trip, room snapshot, and selected typed action operations are available.

Otherwise leave `SemanticCandidate` nil/false and preserve every existing direct/mention/ambient and deterministic-spam path. This is fail closed: a candidate cannot be admitted into the ordinary public tool loop.

## Lifecycle, control flow, and data flow

```text
[OBSERVED source order / proposed target adaptation]
server chat payload
  -> UserChatListener.Notify parses model.ChatMessage
  -> message.ResolveUserMetadata resolves active user / canonical identity
  -> existing audit, relay, mail, AFK, and listener handlers
  -> live.RoomParticipation.Handle
       Event{Message, Snapshot, BotNick, AuthorIsBot,
             ModerationCandidate, ModerationTarget}
  -> participation.Pipeline.Handle
       Pipeline.Monitor(event)                         // existing spam observer first
       eligibility filter                              // existing PASS behavior
       mention parser                                  // existing CLAIMED behavior
       if candidate:
          InvocationFactory.Create(... MODERATION ...)
          runtime.APIBridge.Submit(invocation)         // existing admission path
          return PASS                                  // source CONTINUE then ambient
       existing ambient handling
  -> existing DispatchUserCommand only when not claimed

MODERATION runtime path (only after Stage B)
  -> per-public-room runtime lock / bounded admission
  -> existing assembler: moderation prompt, no freshness, moderation tool inventory
  -> existing provider client, at most existing bounded loop limits
  -> one reviewed moderation action or no action
  -> exact no-reply marker -> ShouldReply=false
  -> no sink, no failure sink, no conversation/evidence append
```

### Failure semantics

- Candidate false, protected, unresolved/invalid source identity, disabled moderation, incomplete composition, or a nonmatching signal: no semantic submission; current participation behavior remains.
- Submission admission failure: preserve pipeline `PASS`, return the existing error to the listener boundary, and do not convert the input into a conversational reply or claim.
- Context cancellation before tool execution: zero action; no retry or background recovery.
- Invalid tool call, target, action, or gateway state: reject the moderation turn; no user-visible failure delivery and no fallback to a command/user authorization route.
- Provider/finalization failure: mode is non-reply; existing runtime must not emit a failure-sink chat message or append memory/evidence.
- A qualifying semantic submission does not suppress subsequent ambient handling merely because it was submitted; source returns `CONTINUE`.

## Compatibility invariants and exclusions

### Invariants

1. Preserve source ordering: deterministic message spam observes before filtering; semantic moderation is after eligibility, quiet, and mention handling, before ambient.
2. A mention is `CLAIMED` and cannot additionally create a semantic invocation. A prefix command and whisper are `PASS` before semantic candidate action.
3. The fixed signal only gates work. It does not itself mute/kick/shadow-ban anyone.
4. The MODERATION identity is the bot/system principal. The alleged author is only the typed moderation target; do not grant the alleged author `MODERATION_COMMANDS`.
5. The target and protection decision originate from parsed/resolved local state, not model response, tool arguments, prompt prose, durable memory, room history, or raw transport JSON.
6. `MODERATION` remains silent: no normal agent reply, no direct/mention failure response, no quote selection, no public agent sink, and no post-delivery conversation or tool-evidence persistence.
7. Keep the public three-tool loop and public `run_command` informational allowlist unchanged. The new moderation-only loop is distinct.
8. Retain existing bounded runtime cancellation, request admission, per-memory-key serialization, and no provider correction/retry/third-completion guarantees unless a later separately accepted source slice changes them.

### Exclusions

- No changes to `MIGRATION_PLAN.md`, frozen audit records, Saturn, config files, existing handoffs, application behavior unrelated to this slice, commits, or pushes in this architecture task.
- No broad moderator command family implementation, generic reflected command catalog, user-command dispatcher reuse, raw agent protocol payload construction, `Ban`, permanent-ban capability, or source-unverified aliases.
- No new provider/model, prompt/resource rewrite, model retry/correction, completion-format change, tool-memory/evidence schema, SQL/H2 change, transport rewrite, listener reorder, queue, background worker, lifecycle redesign, or agent-config switch.
- No deterministic spam threshold/action changes, join moderation change, quote/finalizer change, bot ingress change, direct `l` change, relay change, or ambient cadence redesign.

## Complexity and risk classification

| Area | Classification | Why |
|---|---|---|
| Candidate regex plus pipeline ordering | Moderate | Pure code, but false positives, ordering, and mention/ambient interaction are user-observable. |
| Bot context / target construction | High | A caller-derived context would turn model-selected moderation into user impersonation. |
| Moderation-only `run_command` prerequisite | High / security-sensitive | It introduces model-triggered privileged effects and must not reuse normal user authorization or broaden public tools. |
| Existing provider/runtime use | Moderate | No new provider work, but silent no-reply and no-persistence behavior must be verified across the existing loop. |
| Overall route | **@senior-developer** | The selected slice crosses privileged autonomous action, trusted identity, tool capability, and runtime delivery boundaries. A developer may implement Stage A only under senior-owned interface/test approval. |

## RED → GREEN focused tests

1. **RED — exact pure signal:** add `semantic_moderation_test.go` table cases for every positive alternation, case/Unicode behavior, word-boundary nonmatches, and ordinary/quoted/near-miss text. Assert source-shaped positive `I will doxx you` and negatives such as ordinary text. Do not make the pattern configurable.
2. **RED — candidate trust gate:** factory/policy tests prove `moderationEnabled=false`, self, `IsBot`, configured creator/admin trip, and absent required bot context produce false before runtime submission. A public unprotected non-bot source message can be considered.
3. **RED — pipeline precedence:** in `policies_test.go`, a recording submitter proves a source-pattern mention produces exactly `MENTION`, is claimed, and never produces `MODERATION`; prefix/whisper/self/resolved-bot events do not submit semantic work; a qualifying ordinary public message produces one `MODERATION`, `Submitted=true`, `Decision=PASS`.
4. **RED — source bot context:** test `InvocationFactory`/pipeline construction proves a moderation invocation has bot nick, creator trip, empty hash, public visibility, copied room users, exactly `ModerationCommands`, `ModerationTarget` equal to the canonical resolved alleged author, `CommandOriginated=false`, and source message text preserved. Prove direct/mention/ambient construction remains unchanged.
5. **RED — live provenance:** `participation_test.go` must demonstrate the live adapter derives the candidate/target from parsed message plus resolved `Context.Author`; it must never promote an unresolved/chat-text asserted privilege. The accepted spam monitor still runs first.
6. **RED — moderation tool isolation prerequisite:** tool/gateway tests prove MODERATION request definitions contain only the reviewed source moderation `run_command` contract; normal public requests retain exactly the existing three tools. Assert a malformed/unauthorized/non-bot context, blank target, cancelled context, unsupported alias, and `Ban` execute zero operations. Assert one valid mute uses the typed core owner exactly once and no normal user authorization is consulted.
7. **RED — silent lifecycle:** scripted client tests prove a MODERATION no-action response with exact no-reply marker has no sink delivery, no `FailureSink`, no `AfterDelivery`, no memory append, and no durable evidence append. A moderation tool action remains within existing bounded completion limits and has no visible response.
8. **GREEN:** implement Stage A first, then the prerequisite in the smallest source-complete increment, then activate composition. Do not enable a partial semantic producer that sends candidate work to the ordinary public loop.
9. **Race/regression:** run the relevant race packages. Use channel-controlled gateway fakes to prove cancellation prevents late action and does not leave a mutable target/context shared across turns.

## Minimal rapid verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation. Format only files owned by this future slice.

```sh
gofmt -w \
  internal/agent/participation/semantic_moderation.go \
  internal/agent/participation/semantic_moderation_test.go \
  internal/agent/participation/invocation.go \
  internal/agent/participation/policies_test.go \
  internal/agent/live/participation.go \
  internal/agent/live/participation_test.go \
  internal/agent/tool/moderation_run_command.go \
  internal/agent/tool/moderation_run_command_test.go \
  internal/agent/commandgateway/moderation_gateway.go \
  internal/agent/commandgateway/moderation_gateway_test.go \
  internal/agent/live/tool_loop.go \
  internal/agent/live/tool_loop_test.go \
  internal/factory/engine_factory.go \
  internal/factory/engine_factory_test.go \
  cmd/zenbot/main.go \
  cmd/zenbot/live_agent_test.go

go test ./internal/agent/participation -run 'Test.*(Semantic|Moderation|Pipeline)' -count=1
go test ./internal/agent/live -run 'Test.*(Moderation|ToolLoop|Runner)' -count=1
go test ./internal/agent/tool ./internal/agent/commandgateway -run 'Test.*Moderation' -count=1
go test ./internal/core -run 'Test.*(Mute|Kick|ShadowBan|Moderation)' -count=1
go test ./internal/factory -run 'Test.*(Semantic|Moderation)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test -race ./internal/agent/participation ./internal/agent/live ./internal/agent/tool ./internal/agent/commandgateway -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go vet ./...` as informational only. The accepted QA records the unrelated pre-existing copylocks warning at `internal/core/engine_impl.go:95:22`; do not repair it as part of this slice unless it demonstrably blocks the selected behavior.

## Routing decision

**Route to `@senior-developer`.** The implementation owner must approve the bot-context/target invariant and the mode-scoped autonomous moderation gateway before a live candidate is enabled. `@developer` may take the pure signal, candidate policy, event propagation, and Stage-A tests only after that interface is frozen. Independent QA must verify source handler ordering, silent no-delivery/no-persistence behavior, fixed tool isolation, no user authorization path, no `Ban`, and that the accepted deterministic spam vertical remains unchanged.

## Completion checklist

- [ ] Accepted message-spam QA is cited as permission to proceed, not as semantic moderation acceptance.
- [ ] Every cited Saturn/Zenbot path and symbol resolves in the current read-only inspected trees.
- [ ] One public severe-abuse non-mention creates exactly one bot-context `MODERATION` invocation and remains listener `PASS`.
- [ ] Mentions, commands, whispers, protected principals, self, and bots cannot enter semantic moderation; deterministic monitoring still occurs first.
- [ ] A live `MODERATION` invocation cannot use the public informational loop or user authorization; target/action authority is typed and bounded.
- [ ] Silent moderation produces no normal delivery, failure reply, memory append, or durable evidence append.
- [ ] The source moderator command-inventory prerequisite is independently accepted before candidate composition is enabled; no partial target command mapping is misrepresented as full parity.
- [ ] Focused, relevant, race, full, build, vet-record, and diff checks are recorded by future implementation/QA handoffs.
