# Rapid vertical: semantic MODERATION gateway and conditional activation

## Decision

**Route to `@senior-developer`; keep live semantic ingress deferred.** The next bounded vertical is a separate, MODERATION-only action gateway and one-action tool loop that consumes a listener-captured `core.ModerationTarget`, maps the fixed six-alias Saturn inventory to the six reviewed target operations, and is never reachable from public `run_command`, `DispatchUserCommand`, or a human authorization path.

The implementation may compose `SemanticCandidate` and change `SemanticModerationIngressReady()` only after independent QA proves the gateway, tool-loop, silent lifecycle, and target identity invariants **and** a senior/source owner records the intended treatment of Saturn's nick/hash contradiction. Current source proves the contradiction exists; it does not prove that silently correcting it is source parity. Therefore activation is **not approved by this architecture artifact**.

This is not a broad command migration and does not claim literal full `RunCommandTool` command parity. It is a bounded autonomous moderation adaptation: fixed aliases, no model target field, no arbitrary arguments, exactly one reviewed action or no action.

## Evidence ledger

### [OBSERVED] Saturn `develop` source

| Source | Observed contract |
|---|---|
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `handlers`, `onMessage`, `handleSemanticModeration` | Order is monitor → eligibility → invocation preparation → quiet → mention → semantic → ambient (95–102). A semantic match constructs a bot context from current room/nick/users, carries `turn.message.getNick()` as `moderationTarget`, submits `MODERATION`, then returns `CONTINUE` (205–224); the event is not claimed. |
| Same file, `SEMANTIC_MODERATION_SIGNAL` | Fixed Java `(?iu)\b(?:kys|kill\s+(?:yourself|urself|u|you)|hang\s+(?:yourself|urself)|doxx?\b|swat(?:ting)?\b|rape\b|shoot\s+you|stab\s+you|bomb\s+(?:you|them|the room))` (29–33). |
| `src/main/java/org/saturn/app/agent/room/AgentRoomAutomationFactory.java`, `create` | The semantic policy is `moderationConfig.enabled() && !protectedPrincipals.isProtected(message.getTrip(), message.getNick())`; bot context has configured creator trip, null hash, public visibility, current users, and only `MODERATION_COMMANDS` (38–67). |
| `src/main/java/org/saturn/app/agent/api/AgentInvocationMode.java` | `MODERATION(false)` does not require a reply (5–29). |
| `src/main/java/org/saturn/app/agent/routing/AgentRequestAssembler.java`, `assemble` | MODERATION skips fresh-history requirements (89–98) and retains only the `run_command` definition (125–135). |
| `src/main/resources/agent/system/participation-moderation.txt` | Silent review says clear severe targeted abuse may mute the author, ambiguous/lesser content does nothing, permanent bans are forbidden, and final content must be the no-reply marker. |
| `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java`, `MODERATION_COMMANDS`, `execute`, `isTargetedModerationCommand` | Fixed inventory is exactly `captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban` (45–47). The source accepts a text `arguments` value and requires its first token to equal `context.moderationTarget()` for targeted aliases (159–181, 207–220). |
| `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java` | `enableCaptcha()` queues `enablecaptcha` (109–112); `mute`/`kick` normalize a nick (124–132, 267–270); `unmute(String hash)` queues `{"cmd":"unmute","hash":hash}` (130–133); `unshadowBan(String target)` deletes local records using target as name/trip and base64 UTF-8 target as hash (155–164). |
| `src/test/java/org/saturn/app/agent/room/DefaultAgentRoomAutomationTest.java` | Ordinary text does not create semantic work; an addressed message produces only `MENTION` (242–276). `I will doxx you` produces one MODERATION invocation (280–311). |

### [OBSERVED] Zenbot after reversal QA PASS

| Target | Existing seam / gap |
|---|---|
| `internal/agent/participation/semantic_moderation.go` | The fixed candidate and alias inventory exist. `SourceModerationAliases()` returns the complete six aliases, but `SemanticModerationIngressReady()` is hard-coded false (62–72). Its comment is stale: `UnmuteTarget` and `UnshadowBanTarget` now exist. |
| `internal/agent/participation/invocation.go`, `Pipeline.Handle`, `CreateModeration` | Pipeline preserves monitor-first, eligibility/quiet/mention precedence, semantic `PASS`, and ambient continuation (133–163). `CreateModeration` already makes a bot context with only `api.ModerationCommands`; however `Event.ModerationTarget` is only a string (57–74, 99–123). |
| `internal/agent/live/participation.go`, `RoomParticipation.Handle` | Candidate admission currently requires resolved non-self/non-bot `c.Author`, but carries only `c.Author.Name` (21–31). It must carry a copied `core.ModerationTarget`, not just a nickname. |
| `internal/listener/message/handlers.go`, `ResolveUserMetadata.Handle` | Listener resolves an active `model.User`, assigns `c.Author`, and replaces message hash with that user's hash before participation (15–25). `model.User` has `Name`, `Trip`, raw `Hash`, `IsBot`, and `Isme` in `internal/model/user.go`. |
| `internal/core/moderation_target.go`, `NewModerationTarget`; `internal/core/moderation_reversal.go` | `NewModerationTarget` validates/copies resolved identity exactly. `UnmuteTarget(ctx, target)` requires raw `target.Hash`; `UnshadowBanTarget(ctx, target)` requires `target.Name` and uses the typed reversal repository (10–26; 11–50). Reversal QA passed exact unmute wire, source-shaped H2 delete, cancellation, zero-row idempotence, and snapshot behavior. |
| `internal/core/engine_impl.go` | Reviewed existing operations are `EnableCaptcha(ctx)`, `MutePrincipal(ctx, name)`, `KickPrincipal(ctx, name)`, and `ShadowBan(ctx, name)` (523–605). The latter three require current active-user lookup; no agent package constructs raw payloads. |
| `internal/repository/h2/shadow_ban.go` | `RemoveShadowBanBySourceTarget` is the reviewed parameterized local delete; it sends no protocol message (25–36). |
| `internal/agent/tool/run_command.go`; `internal/command/agent_gateway.go`; `internal/listener/message/handlers.go`, `DispatchUserCommand` | These are prohibited: the public tool has informational aliases and an `arguments` string; the public gateway checks active user plus `IsUserAuthorized`; dispatcher builds human commands and sends human authorization replies (17–67; 43–86; 167–189). |
| `internal/agent/live/tool_loop.go`; `cmd/zenbot/main.go` | `NewBoundedToolLoop` freezes exactly history, room-users, and public `run_command` (280–315). `newLiveAgent` builds that loop and leaves `RoomParticipation.SemanticCandidate` nil while readiness is false (127, 150–171). |
| `internal/agent/runtime/runtime.go`; `internal/agent/live/runner.go` | Runtime delivers/failure-delivers only reply-required work; `AfterDelivery` appends memory/evidence only after a reply (109–145; `AfterDelivery` 182–195). This is the silent/no-persistence seam to retain. |

## Required authority, identity, and control flow

### Target capture and authority

1. `ResolveUserMetadata` is the sole identity source. Before semantic eligibility is marked, live participation must call `core.NewModerationTarget(*c.Author)` and copy the result into immutable event/invocation state. Missing author, blank name, self, bot, protected principal, candidate false, or construction error means no semantic submission.
2. The autonomous principal remains `CreateModeration`'s bot/system context: bot nick, creator trip, null hash, public room, copied room-user snapshot, and exactly `api.ModerationCommands`. The alleged author never receives a capability.
3. The model may choose one closed alias; it never supplies/chooses a target, nickname, hash, trip, channel, reason, or arbitrary argument. The gateway receives the captured typed target separately from tool JSON.
4. A targeted operation may additionally reject a target that has left the active roster where its reviewed core operation requires that check. It must not substitute a second lookup by model text.

```text
[PROPOSED — disabled until gates pass]
parsed message
  -> ResolveUserMetadata: c.Author = active model.User
  -> RoomParticipation captures core.ModerationTarget{Name, Trip, raw Hash}
  -> candidate + protected-principal gate
  -> Pipeline: monitor -> filters -> quiet -> mention -> MODERATION -> ambient
       MODERATION uses bot context + captured target; returns PASS
  -> runtime MODERATION selector
  -> moderation-only tool descriptor { action: <closed alias> }
  -> ModerationGateway.Execute(ctx, bot-context, captured-target, alias)
  -> reviewed typed core operation
  -> no reply, no failure reply, no AfterDelivery persistence
```

The pipeline must preserve source ordering: a mention is `CLAIMED` and never double-submits MODERATION; ineligible/prefix/whisper/self/bot input cannot reach semantic work; a successful semantic submission returns `PASS` and can still yield the existing ambient submission.

## Closed alias schema and operation map

The proposed target tool is **not** `tool.RunCommand`. Give it a distinct internal-only name such as `moderation_action` so the frozen public contract cannot be widened. Its descriptor is the whole model-visible contract:

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "action": {
      "type": "string",
      "enum": ["captcha", "mute", "unmute", "kick", "shadowban", "unshadowban"]
    }
  },
  "required": ["action"]
}
```

No `target`, `arguments`, `hash`, `nick`, reason, destination, `-all`, alias synonym, or provider-defined extension is valid. Case normalization is not necessary: descriptor values are canonical lowercase and anything else is rejected.

| Fixed source alias | Captured typed input | Reviewed Zenbot operation | Result / boundary |
|---|---|---|---|
| `captcha` | none | `EngineImpl.EnableCaptcha(ctx)` | Fixed `enablecaptcha` command; no textual/model argument. |
| `mute` | `target.Name` | `EngineImpl.MutePrincipal(ctx, target.Name)` | Existing active-principal checked fixed mute operation. |
| `unmute` | `target.Hash` | `EngineImpl.UnmuteTarget(ctx, target)` | Fixed unmute protocol using copied raw hash; see incompatibility gate below. |
| `kick` | `target.Name` | `EngineImpl.KickPrincipal(ctx, target.Name)` | Existing active-principal checked fixed kick operation. |
| `shadowban` | `target.Name` plus captured target retained | `EngineImpl.ShadowBan(ctx, target.Name)` | Existing active-user lookup then local source-shaped persistence. |
| `unshadowban` | `target.Name` | `EngineImpl.UnshadowBanTarget(ctx, target)` | Source-shaped local delete only; no remote unban. |

Implement a closed `ModerationAction` enum in the moderation gateway, not a stringly generic command dispatcher. The tool parses the enum once; the gateway switches exhaustively over it; each branch receives the same request-local `core.ModerationTarget`. `captcha` is the only no-target branch. The gateway must reject a non-MODERATION invocation, non-bot context, missing exact capability, nil/blank typed target for targeted actions, canceled context, unsupported enum, and any incomplete operator dependency before making an operation call.

## Saturn nick/hash inconsistency — containment decision

### [OBSERVED inconsistency]

Saturn semantic ingress stores **nick** as `moderationTarget` (`AgentRoomMessagePipeline.handleSemanticModeration`, 209–218). `RunCommandTool.execute` then demands the first textual argument for `unmute` equal that nick (171–175). But `UnMuteUserCommandImpl.execute` treats its first argument as a **hash** and passes it to `ModServiceImpl.unmute(hash)` (29–40). For a usual user where nick and raw hash differ, the source semantic path cannot simultaneously satisfy its target guard and unmute's required hash.

`unshadowban` is different: its command interprets its first argument as target text and `ModServiceImpl.unshadowBan` uses that text for name/trip/base64 matching. The source inconsistency is specifically material to semantic `unmute`, not evidence to rewrite unshadowban or add a remote unban.

### [RECOMMENDED containment]

The target's captured `ModerationTarget` can accurately represent both source primitives: `Name` for name-targeted actions and raw `Hash` for unmute. It must never manufacture a model text argument to make them appear consistent. That is a **safe typed adaptation candidate**, not proof of literal end-to-end source behavior.

Before enabling ingress, the senior owner must choose and record one of these source-grounded dispositions:

1. **Native-identity adaptation accepted:** permit `unmute -> UnmuteTarget(captured raw Hash)` while documenting that this fixes an internally contradictory source text guard. This preserves each downstream primitive but is not literal `RunCommandTool` parity.
2. **Strict semantic compatibility:** retain the source's nick guard for semantic unmute. Since it conflicts with the hash primitive for ordinary identities, reject `unmute` without action unless nick equals raw hash. This is behaviorally conservative but leaves an intentionally unusable inventory member.
3. **Source correction first:** change/clarify Saturn outside this task, then update evidence and approve a matching target policy.

Until one disposition is approved and tested, `NewModerationToolLoop` / gateway construction must fail closed and `SemanticModerationIngressReady()` remains false. Do not silently choose option 1 in implementation and do not call any option “full command parity.” The current moderation prompt asks the model to mute on clear abuse and otherwise no-op; it does not supply authority to invent reversal behavior.

## Proposed bounded file map

| Path | Change | Purpose |
|---|---|---|
| `internal/agent/live/participation.go` | modify | Capture `core.ModerationTarget` from `c.Author` only after local identity/protection checks; never derive it from text. |
| `internal/agent/participation/invocation.go` | modify | Replace string-only semantic target event transport with copied typed target (or a private immutable wrapper); `CreateModeration` receives only that type. Keep public mode construction unchanged. |
| `internal/agent/participation/semantic_moderation.go` | modify last | Readiness becomes a composition predicate only after all gateway gates are true; no new config. |
| `internal/agent/participation/*_test.go`, `internal/agent/live/*_test.go` | modify/add | Provenance, ordering, immutable capture, and readiness tests. |
| `internal/agent/tool/moderation_action.go` + test | create | Separate closed-schema tool; no public `run_command`, no `arguments`. |
| `internal/agent/commandgateway/moderation_gateway.go` + test | create | Private MODERATION-only enum-to-typed-operation boundary, no human dispatcher/gateway. |
| `internal/agent/live/moderation_tool_loop.go` + test | create | Separate one-action loop that accepts only `moderation_action`, disables command-prose correction, and retains two-completion/one-tool-call bound. |
| `internal/agent/live/runner.go` + test | modify | Select the moderation loop only for `runtime.MODERATION`; direct/mention/ambient keep the public loop unchanged. |
| `internal/agent/assemble/assemble.go` + test | modify narrowly | Permit only `moderation_action` for target MODERATION assembly rather than source-name/public `run_command`; retain no-freshness behavior and no other tools. |
| `cmd/zenbot/main.go` + test | modify last | Construct the separate moderation loop only for a qualifying `*core.EngineImpl`, wire typed candidate capture, and compose only after the complete gate. |
| `internal/core/*`, `internal/repository/*` | no new primitive expected | Six primitive operations are accepted. Any discovered signature gap is a stop condition, not authority to broaden core or protocol code. |

Do not modify config, prompts, `MIGRATION_PLAN.md`, frozen audit records, Saturn, public tool definitions, public command gateway, normal listener ordering, provider/model setup, or persistent schemas for this vertical.

## Failure semantics and invariants

- Candidate false, protected/self/bot/unresolved author, invalid typed target, missing bot context, readiness false, or incomplete construction: no semantic runtime submission; listener remains unclaimed.
- Runtime admission rejection: preserve `PASS`, return the existing error at the listener boundary, and never transform the message into a conversational reply.
- Tool calls with any unknown field, absent action, unsupported alias, wrong mode/principal, model-supplied target-like data, or attempted command prose: no operation; the MODERATION turn fails silently.
- Pre-canceled or canceled context before/after tool execution: no retry, no background recovery, no late second action. Existing typed core/repository context behavior remains authoritative.
- Core/repository/transport error: no fallback to `Ban`, `Unban`, `Kick(name, channel)`, raw payload construction, generic `DispatchUserCommand`, `common.BuildCommand`, public `run_command`, or `IsUserAuthorized`. No failure sink message because MODERATION does not require reply.
- Exactly one tool call and exactly one action maximum per invocation. Retain current `ToolLoopLimits()` bounds (`MaxSteps: 2`, `MaxToolCalls: 1`); moderation must not add correction, retry, a third completion, or a second model-selected action.
- Successful no-op or action must finalize to the exact no-reply marker and `ShouldReply=false`; runtime must not deliver to sink, call `AfterDelivery`, append conversation memory, or persist tool evidence.

## Red → green test plan

1. **RED identity capture:** mutate the resolved `model.User` after `NewModerationTarget`; prove the MODERATION invocation retains exact original name/trip/raw hash. Missing author, self, bot, blank name, and a chat-asserted name different from `c.Author.Name` submit no moderation target.
2. **RED source ordering:** recording submitter proves monitor runs first; whitespace/whisper/prefix/self/bot do not submit; a severe mention is only MENTION/CLAIMED; a qualifying public non-mention creates one MODERATION then may enter ambient/PASS.
3. **RED bot authority:** prove MODERATION has bot nick, creator trip, null hash, public room, copied users, exactly `ModerationCommands`, and a separate captured target. Prove DIRECT/MENTION/AMBIENT contexts are byte-for-byte behaviorally unchanged.
4. **RED descriptor isolation:** moderation loop exposes exactly one `moderation_action` tool and the six canonical aliases. Public loop remains exactly history, room-users, public `run_command`, and its informational enum. Assert no target/arguments property and reject extras, mixed case, command prose, a second call, or a second tool completion.
5. **RED gateway map:** table-driven test covers every fixed alias, captures the exact typed operation and field used, and asserts unknown action, wrong context/capability, blank target, cancelled context, and absent operator call zero operations. `captcha` invokes only captcha; each other branch gets the copied target—not model data.
6. **RED nick/hash gate:** test nick `alice`, hash `h-1` and prove the chosen senior disposition. Until a disposition is injected/approved, loop construction and readiness fail closed. If native adaptation is approved, test `unmute` receives `h-1`; also label it as adaptation, not source text-command reproduction.
7. **RED silent lifecycle:** scripted no-action and successful-action provider responses with exact marker produce no sink, no failure sink, no `AfterDelivery`, no memory append, and no tool-evidence append. Tool/gateway errors do the same.
8. **RED cancellation/race:** channel-blocked operator verifies cancel/deadline prevents a late action and no second alias runs. Race tests prove target copies are not shared/mutated across two room turns.
9. **GREEN sequencing:** implement capture and isolated loop/gateway tests first; compose last. Do not turn readiness true to make earlier tests pass.

## Rapid gates and verification

Implementation is allowed to move from construction to composition only when every gate is recorded by independent QA:

- [ ] All six aliases are represented by the closed enum and map only to the reviewed operations above.
- [ ] The nick/hash disposition is explicitly approved; otherwise constructor/readiness stays false.
- [ ] Captured `core.ModerationTarget`, not prompt/tool text, reaches every targeted action.
- [ ] Public `run_command`, `command.NewAgentCommandGateway`, `DispatchUserCommand`, generic user authorization, `Ban`, `Unban`, arbitrary arguments, and raw agent package protocol construction have zero new production call sites.
- [ ] One action/bounded cancellation and silent/no-reply/no-persistence pass focused tests.
- [ ] Semantic ordering, mention precedence, ambient continuation, fixed Unicode candidate behavior, and existing deterministic spam monitor remain unchanged.
- [ ] Focused tests, relevant race tests, `go test ./... -count=1`, `go build ./...`, and `git diff --check` pass. Treat the recorded existing `go vet ./...` copylocks warning in `internal/core/engine_impl.go:95` as informational only unless it blocks this slice.

Suggested commands after implementation, from `/Users/ab/workspace/go-projects/zenbot`:

```sh
gofmt -w internal/agent/live/participation.go internal/agent/participation/invocation.go \
  internal/agent/participation/semantic_moderation.go internal/agent/tool/moderation_action.go \
  internal/agent/commandgateway/moderation_gateway.go internal/agent/live/moderation_tool_loop.go \
  internal/agent/live/runner.go cmd/zenbot/main.go

go test ./internal/agent/participation ./internal/agent/live ./internal/agent/tool ./internal/agent/commandgateway -run 'Test.*(Semantic|Moderation|ToolLoop|Gateway|Runner)' -count=1
go test ./internal/core ./internal/repository/h2 -run 'Test.*(Moderation|Mute|Kick|ShadowBan|Unmute|Unshadow)' -count=1
go test -race ./internal/agent/participation ./internal/agent/live ./internal/agent/tool ./internal/agent/commandgateway ./internal/core -count=1
go test ./... -count=1
go build ./...
git diff --check
```

## Risk and routing

| Area | Risk | Owner |
|---|---|---|
| Captured identity / bot authority | High: an author-derived context or model target would authorize the wrong principal. | `@senior-developer` |
| Autonomous action gateway | High: privileged state changes must not inherit human authorization or public tool behavior. | `@senior-developer` |
| Saturn nick/hash contradiction | High parity risk: a silent target-side correction would be an undocumented behavior change. | `@senior-developer` plus source owner decision |
| Silent runtime lifecycle | Moderate: delivery/evidence persistence regressions are externally visible and durable. | Senior-owned implementation, independent QA |
| Candidate/order propagation | Moderate: false positives and mention/ambient precedence are user-visible. | `@developer` only after senior freezes interfaces |

`@developer` may implement pure typed event propagation, candidate/order tests, and the isolated descriptor after the senior owner freezes the disposition and gateway interfaces. The gateway, runner selection, activation composition, and acceptance decision remain senior-owned.

## Exclusions

- No public `run_command` modification; no `DispatchUserCommand`, generic user authorization, human command catalog, `Ban`, `Unban`, `-all`, synonyms, arbitrary arguments, raw protocol construction in agent packages, or permanent-ban capability.
- No new provider, model, endpoint, retry/correction policy, worker topology, queue, config flag, prompt rewrite, data migration, H2 schema change, or broad command migration.
- No claim that source's generic textual `RunCommandTool` behavior, human confirmations, aliases such as `undumb`/`shadowmercy`/`unblock`, or all Saturn command behavior is reproduced.
- No activation solely because the six primitive methods exist. The current activation state remains disabled until all rapid gates, including the explicit nick/hash disposition, pass.
