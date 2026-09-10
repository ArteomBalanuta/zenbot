# Next bounded Saturn agent parity vertical: authoritative bot-author ingress exclusion

## Decision

**Select one narrow behavior: prevent live agent participation from responding to a public message whose resolved active-room author is flagged `isBot`.**

This closes the remaining source-tested reply-loop guard at ingress. The accepted target already rejects blank, whisper, self, conventional-bot-name, and command-prefixed public events; it does **not** carry the trusted active-user `IsBot` bit into `participation.Pipeline`. A non-conventional bot name can therefore still create a live mention or ambient invocation.

This is deliberately smaller than a moderation, provider, transport, tool, SQL, or generic routing change. It requires one immutable, trusted event bit and one eligibility branch before invocation construction; it has no provider completion, delivery, persistence, configuration, timeout, cancellation, or authorization side effect.

## Evidence map

### [OBSERVED] Saturn contract

| Evidence | Observed behavior |
|---|---|
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `filterIneligible` | After moderation monitoring, the room pipeline trims text and returns `PASS` without making an invocation when text is blank, whisper, self-authored, command-prefixed, conventionally bot-named, **or the active room's matching `User.isBot()` is true**. |
| Same source, `prepareInvocation`, `handleMention`, `handleAmbientParticipation` | Only an eligible event reaches trusted invocation creation, mention submission, or ambient cadence. A filtered bot message cannot advance ambient routing or be claimed as a mention. |
| `src/test/java/org/saturn/app/agent/room/DefaultAgentRoomAutomationTest.java`, `ignoresMessagesAuthoredByBotsToPreventReplyLoops` | A sender named `otherBot` is explicitly marked `isBot=true` in `engine.currentChannelUsers`; `@saturn answer this` returns `PASS` and creates zero submissions. This proves the source behavior is not limited to nick spelling. |
| Same test, `ignoresConventionalBotNicksWhenTheServerDoesNotFlagThem` | The independent conventional-name fallback remains required when the server does not set the bot flag. |
| `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, handler order | This ingress policy is before quiet, mention, semantic moderation, and ambient participation; source moderation observation still occurs before eligibility filtering. |

### [OBSERVED] accepted Zenbot state and gap

| Evidence | Observed behavior / precise gap |
|---|---|
| `internal/listener/message/chain.go`, `Context`; `internal/listener/message/handlers.go`, `ResolveUserMetadata` and `DefaultChainWithParticipation` | The live message context already has `Author *model.User`. `ResolveUserMetadata`, ordered before `AgentParticipation`, resolves it from the engine's active-user map by case-insensitive nick and copies the trusted hash. Therefore agent participation can consume server-maintained author metadata without parsing it from chat text. |
| `internal/model/user.go`, `User.IsBot` | The target model decodes the server-supplied `isBot` attribute as `bool`; it is available on the resolved `Context.Author`. |
| `internal/agent/live/participation.go`, `RoomParticipation.Handle` | The listener-to-pipeline adapter currently builds an event from message/snapshot/name/prefix/configuration but supplies no authoritative bot status. |
| `internal/agent/participation/invocation.go`, `Event` and `(*Pipeline).Handle` | The accepted pipeline calls `Monitor` before eligibility, then excludes blank/whisper/self/conventional-name/prefix inputs. It can submit `MENTION` or `AMBIENT` after that branch; it has no `AuthorIsBot` condition. |
| `internal/agent/participation/policies.go`, `isConventionalBot`; `policies_test.go`, `TestPipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors` | Current fallback nick filtering is already source-shaped but intentionally cannot identify a non-conventional bot name. |
| `cmd/zenbot/main.go`, `newLiveAgent` construction; `internal/agent/runtime/runtime.go`, `Submit`, `SubmitAmbient`, `execute` | Direct/mention/ambient/relay runtime topology, bounded public tools, delivery, persistence-after-delivery, latest-wins ambient behavior, cancellation, and failure routing are accepted. They are not needed to make the ingress exclusion correct and must remain unchanged. |
| `.hermes/handoffs/rapid-agent-internal-evidence-sentinel-qa.md` | The accepted finalizer now blocks source-shaped internal evidence before visible delivery/persistence. This selected ingress guard is complementary: it prevents bot-authored work from reaching provider/tool/finalizer paths in the first place. |

### [LIMITATION]

`Context.Author` is nil when the sender is absent from the active-user map. The source has the analogous practical dependency on its current-user collection. This slice must not infer bot status from an untrusted chat payload, trip, hash, or a new nickname heuristic. With no resolved author, retain the existing conventional-name fallback and normal eligibility semantics.

## Alternatives and rationale

| Candidate | Decision | Reason |
|---|---|---|
| **Resolved `isBot` ingress exclusion** | **Selected** | Exact, source-tested, live safety guard with one data-flow addition and no new capability. It blocks both mention and ambient loops before provider work. |
| Broaden `isConventionalBot` regex | Reject | Cannot reproduce Saturn's explicit `User.isBot()` behavior and risks false positives for ordinary user names. |
| Scan `Engine.GetActiveUsers()` inside `participation.Pipeline` | Reject | Would couple the pure participation package to listener/engine mutable state and create an unsynchronized second lookup seam. `ResolveUserMetadata` already established the boundary. |
| Drop all unresolved authors | Reject | Not source-grounded and could suppress normal users during online-set timing gaps. |
| Provider correction/retry, extra completion, output sanitizer/finalizer work | Exclude | Accepted finalization behavior already exists; these do not prevent the ingress/provider call. |
| Tools, SQL/dynamic DB, reflection catalogs, command gateway, moderation action, relay/transport rewrite | Exclude | Each expands authority/topology beyond this one read-only routing fact. |

## Exact Saturn contract and bounded target adaptation

### Contract to preserve

For each public room message, preserve current ordering:

```text
monitor event (existing)
-> normalize/eligibility filter
   -> if resolved active author has isBot=true: PASS; no invocation
-> quiet request / mention / ambient handling (existing)
```

A flagged bot message must:

1. create no `api.Invocation`;
2. call neither `Submit` nor `SubmitAmbient`;
3. never claim a mention, so the listener chain continues exactly as Saturn's `PASS` outcome would;
4. not increment the pipeline's ambient eligible-event counter;
5. make no provider completion, tool attempt, command execution, sink/failure-sink call, finalizer call, memory append, durable-evidence append, or queue/admission reservation.

The conventional-name check remains an OR condition. The source accepts either trustworthy active-user `isBot` metadata or a conventional bot nick as enough to exclude participation.

### [RECOMMENDED] target adaptation

Saturn reads `message.getNick()` against `engine.currentChannelUsers` in the same Java pipeline. Zenbot's preceding `ResolveUserMetadata` already performs the active-user lookup and records the result in `message.Context.Author`. Do not repeat that lookup or alter `common.Engine`.

Add a private-to-agent-routing event datum:

```go
// internal/agent/participation/invocation.go
type Event struct {
    // existing fields...
    AuthorIsBot bool // set only by the listener adapter from resolved server user metadata
}
```

In `live.RoomParticipation.Handle`, populate it only from `c.Author`:

```go
AuthorIsBot: c.Author != nil && c.Author.IsBot,
```

Then extend the existing `Pipeline.Handle` eligibility condition, after `Monitor(e)` and alongside self/conventional-name/prefix filtering:

```go
if text == "" || e.Message.Whisper || e.Message.IsWhisper ||
    strings.EqualFold(e.Message.Name, e.BotNick) || e.AuthorIsBot ||
    isConventionalBot(e.Message.Name) ||
    (e.Prefix != "" && strings.HasPrefix(text, e.Prefix)) {
    return Outcome{Decision: Pass}
}
```

`AuthorIsBot` is a one-way trusted fact, not a model argument, capability, prompt field, tool argument, persistence value, or API/configuration setting. Keep its Go zero value (`false`) for existing unit-created events; only live listener composition may set it from resolved author metadata.

## File, interface, and data-flow plan

### Stage A — carry one immutable trusted fact

**Modify only:**

- `internal/agent/participation/invocation.go`
- `internal/agent/live/participation.go`
- `internal/agent/participation/policies_test.go`
- `internal/agent/live/participation_test.go`

No new production package, interface, configuration field, constructor, goroutine, resource, database access, or engine method is justified.

1. Add `AuthorIsBot bool` to `participation.Event`, documented as listener-resolved active-user metadata rather than message data.
2. In `Pipeline.Handle`, keep `Monitor(e)` as the first operation. Add `e.AuthorIsBot` to the already atomic eligibility predicate before `api.NewContext`, quiet evaluation, mention parsing, moderation candidate dispatch, and ambient counter mutation.
3. In `RoomParticipation.Handle`, derive the bool from `c.Author != nil && c.Author.IsBot` while it is constructing the event. Keep its existing nil-wiring error behavior and call `Pipeline.Handle` exactly once.
4. Do not add the flag to `TrustedSnapshot`, `api.Context`, `runtime.Invocation`, `assemble` metadata, prompts, tool definitions, or persistence records. The bit is relevant only until eligibility has rejected/accepted the inbound event.

### Proposed end-to-end sequence

```text
[OBSERVED / proposed narrow branch]
chat payload
  -> ResolveUserMetadata
       active-user nick match -> Context.Author (*model.User, includes IsBot)
  -> ... existing audit / relay / downstream ordering ...
  -> live.RoomParticipation.Handle
       Event{AuthorIsBot: Context.Author != nil && Context.Author.IsBot}
  -> participation.Pipeline.Handle
       Monitor(e)                         // unchanged ordering
       AuthorIsBot? yes -> PASS           // NEW: stop agent work only
                         no -> existing quiet / mention / ambient paths
  -> AgentParticipation maps PASS to chain continuation
```

## Boundaries

| Boundary | Required behavior |
|---|---|
| Visibility | Inspect only the already resolved server active-user bit. Do not surface `IsBot`, user metadata, prompt, room snapshot, tool envelope, or provider content. |
| Authentication / authorization | This is not identity authorization. It grants no capability and changes no creator/admin/moderator/protected-principal evaluation. It only excludes an already flagged automated sender from agent participation. |
| Delivery | Filtered input creates no result, ordinary/failure response, command output, sink call, relay message, or duplicate acknowledgment. `PASS` means agent participation does not claim the listener event. |
| Persistence | No invocation means no conversation/evidence loading or append, tool result, audit schema change, or durable state mutation from the agent. Existing audit/log handler ordering is unchanged. |
| Provider / protocol | No LLM request, tool definition, tool call, synthesis, correction/retry, response finalizer, no-reply processing, quote selection, or internal-evidence guard invocation occurs for the filtered event. Accepted one-call/two-completion limits remain untouched. |
| Cancellation / timeout | No context, deadline, queue entry, ambient drain, semaphore reservation, or goroutine is created by this branch. Existing runtime cancellation/timeout behavior is unchanged for eligible work. |
| Concurrency | `AuthorIsBot` is copied as an immutable bool in a per-event value. No map read, shared counter mutation, lock, clock, or new synchronization is introduced. The existing ambient counter therefore does not advance for flagged bots. |
| Listener ordering | Preserve `ResolveUserMetadata` before agent participation and preserve `Pipeline.Monitor` before the eligibility branch. Do not move bot exclusion to `IgnoreBotMessage`, which currently intentionally ignores only self messages and would alter relay/mail/AFK/downstream behavior. |
| Missing metadata | Nil/unresolved `Context.Author` yields `false`; preserve existing conventional-name fallback and normal processing. Never manufacture a bot flag from raw message fields. |

## RED → GREEN test plan

Perform strict incremental TDD; establish each failing behavior before production code.

1. **RED — pure pipeline bot flag:** extend `internal/agent/participation/policies_test.go` with a table where `Event.AuthorIsBot=true`, sender `otherBot` (not a conventional bot nick), public `@bot help`, `AmbientEnabled=true`, cadence `1`, and a recording submitter. Assert `Pass`, `Submitted=false`, `Err=nil`, zero invocations, and no ambient counter advancement (a following ordinary human event is the first cadence hit). This must fail before the new condition exists.
2. **RED — source ordering:** use a `Monitor` recorder with `AuthorIsBot=true`. Assert monitor runs once but `Factory.Create`, quiet mutation, mention parser submission, and ambient submission do not run. This protects Saturn's monitor-before-filter ordering without importing moderation behavior.
3. **RED — fallback/non-regression:** table-drive (a) `AuthorIsBot=false` non-conventional author with valid mention still submits/claims, (b) `AuthorIsBot=false` conventional name remains filtered, and (c) nil/unresolved live `Context.Author` does not turn an ordinary non-conventional user into a bot. Existing quiet/mention/ambient behavior must remain green.
4. **RED — live adapter provenance:** extend `internal/agent/live/participation_test.go` with a `message.Context` containing `Author: &model.User{Name: "otherBot", IsBot: true}` and a non-conventional matching chat sender. Assert zero submitter invocations and `claimed=false`. Repeat with `Author:nil` and assert the valid mention reaches the recording submitter. This proves the bit comes from resolved metadata rather than sender spelling.
5. **GREEN:** add the field, derive it only in `RoomParticipation.Handle`, and add the single pipeline condition. Do not modify test fixtures to fabricate alternate engine state, and do not add production tests for provider/runtime/persistence because the rejection must occur before those seams.
6. **Regression:** retain existing `TestPipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors`, live mention trusted-snapshot coverage, ambient cadence/quiet tests, relay topology tests, internal-evidence sentinel tests, and normal direct tests unchanged.

## Risk routing and rapid baseline gates

### Risk and review routing

- **Implementation complexity:** low. This is two small production-file changes plus focused tests.
- **Primary correctness risk:** treating untrusted `ChatMessage` fields as authoritative or moving the exclusion above `Monitor`/outside agent participation. The implementation owner must use only `Context.Author.IsBot` and retain pipeline ordering.
- **Primary regression risk:** accidentally treating unresolved authors as bots, which would block legitimate mentions/ambient participation during active-user timing gaps. Test the nil-author fallback explicitly.
- **Route:** suitable for `@developer` with review by the live-agent/listener owner. Escalate to architecture only if the proposed change requires a `common.Engine` API, global active-user scan, listener-chain reorder, provider/runtime change, or a new authority source; those are out of scope.

### Required gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only slice-owned Go files:

```sh
gofmt -w \
  internal/agent/participation/invocation.go \
  internal/agent/participation/policies_test.go \
  internal/agent/live/participation.go \
  internal/agent/live/participation_test.go

go test ./internal/agent/participation -run 'Test(Pipeline.*Bot|PipelinePrecedence|PipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors)' -count=1
go test ./internal/agent/live -run 'TestRoomParticipation.*(Bot|Mention|Ambient|Quiet)' -count=1
go test ./internal/listener/message -run 'Test(DefaultChain|AgentParticipation)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Run `go test -race ./internal/agent/participation ./internal/agent/live ./internal/listener/message -count=1` as the relevant concurrency confidence gate; it should remain low-cost because this slice adds no synchronization. If `go vet ./...` still reports the known unrelated `internal/core/engine_impl.go` copylocks warning recorded by the accepted QA, report it separately and do not modify that unrelated core behavior.

## Explicit exclusions

- No changes to `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, Saturn source, resources, existing handoffs, commits, or pushes.
- No direct `l`, mention, ambient, relay, queue, runtime, listener-chain, transport, shutdown, failure-sink, or command-dispatch semantic rewrite.
- No provider correction/retry, third completion, output-normalization change, quote catalog/resource change, or internal-evidence-sentinel change.
- No tool registry/definition/executor change, `run_command`, command gateway, SQL/dynamic database, H2 schema/query, reflection catalog, moderation action, protected-principal, capability, or authorization expansion.
- No new persistence, conversation/evidence read/write, audit schema, configuration, global state, background worker, timer, lock, or clock.

## Completion checklist

- [ ] One non-conventional resolved `IsBot=true` author is `PASS`, unsubmitted, and cannot advance ambient cadence.
- [ ] Monitor remains invoked before the new rejection.
- [ ] A `false`/nil resolved flag preserves valid normal mention and ambient behavior; conventional-name filtering remains intact.
- [ ] The adapter derives the bit only from `message.Context.Author`, not inbound payload fields or a second engine scan.
- [ ] No provider, tool, delivery, persistence, config, runtime, or protected migration document changes are introduced.
- [ ] Focused, full, build, race, and `git diff --check` gates have recorded results.
