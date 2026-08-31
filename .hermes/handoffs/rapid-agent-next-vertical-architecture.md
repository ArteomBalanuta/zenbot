# Next rapid live-agent vertical: ambient participation with per-user quiet and latest-wins coalescing

## Decision

**Select ambient room participation, including polite per-user quiet suppression and the indispensable ambient latest-wins admission path.** This is the smallest next live behavior that turns the already wired mention runtime into an autonomous room participant without importing tools, H2 history, durable memory, moderation, or the full router.

[OBSERVED] Direct `l`, public mention submission/delivery, and AGENT-child relay are now live and accepted (`rapid-agent-live-slice-qa.md`, `rapid-agent-live-integration-qa.md`, `rapid-agent-relay-topology-qa.md`). `newLiveAgent` already owns a configured runtime, sink, failure sink, snapshot, listener injection, and shutdown; `live.RoomParticipation` currently deliberately supplies `AmbientEnabled:false`, `AmbientEvery:0`, and `EligibleCount:0` (`cmd/zenbot/main.go`, `internal/agent/live/participation.go`).

[OBSERVED] The target already owns the source-shaped policy primitives: `participation.Pipeline` filters ineligible events, records quiet state before mention parsing, submits `AMBIENT` on `EligibleCount % AmbientEvery == 0`, and keeps ambient outcomes as listener pass-through (`internal/agent/participation/invocation.go`); `QuietRegistry` uses a room + stable agent identity key and expires entries (`internal/agent/participation/policies.go`). Configuration already resolves and validates `Ambient`, `AmbientEveryMessages`, and `QuietMinutes` (`internal/config/agent_config.go`).

[OBSERVED] The missing lifecycle prerequisite is not optional: Saturn's `AgentServiceImpl.submitAmbient` retains one pending ambient invocation and, while work is executing, replaces it with the newest; direct/mention work remains admitted normally (`src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java`). Zenbot's generic `runtime.Runtime.Submit` instead returns `ErrBusy` when its shared admission is full (`internal/agent/runtime/runtime.go`). Enabling cadence alone would silently lose a source-required latest ambient event under load.

[RECOMMENDED] Do **not** select tools, persistence/history, or broad routing/finalization next. Each depends on multiple currently unwired owners and expands into SQL/tool-loop/security work. Do not select only response sanitizer/finalizer either: it does not create a new live interaction. Ambient + quiet has a full configured ingress→runtime→outbound path today and one bounded runtime prerequisite that can be migrated in the same vertical slice.

## Saturn contract to migrate

The selected contract is limited to these exact behavior owners:

| Saturn owner | Contract carried into this slice |
|---|---|
| `agent/room/AgentRoomMessagePipeline.filterIneligible` | Trim input and pass blank, whisper, self, conventional-bot, and command-prefix messages without constructing/submitting an ambient invocation. |
| `AgentRoomMessagePipeline.prepareInvocation` and `handleQuietRequest` | For every eligible public event, make the normal trusted invocation context; a polite quiet request stores silence for that author identity and then passes. Quiet processing precedes mention handling, so an addressed quiet request is not submitted as a mention. |
| `AgentRoomMessagePipeline.handleMention` | Mention precedence remains unchanged: an ordinary valid mention is submitted/claimed before ambient cadence. Ambient must never delay, replace, or claim a mention. |
| `AgentRoomMessagePipeline.handleAmbientParticipation` | If ambient is disabled or that invocation context is quiet, pass. Otherwise increment one pipeline-wide eligible-event counter; submit only when `floorMod(incrementAndGet(), ambientEveryMessages) == 0`; always pass the listener chain. |
| `AgentQuietRegistry` | Key quiet state by lowercased room and `AgentUserIdentity`: trip first, then hash, then normalized nick; duration is `quietMinutes`; expiry at the exact deadline removes the entry. Polite lexical recognition is independent of bot nick. |
| `AgentServiceImpl.submitAmbient` / `executeNextAmbient` | Disabled/closed ambient submission is false/silent. There is one replaceable pending ambient slot. If a slot is already scheduled/running, subsequent ambient submissions overwrite the pending slot and return accepted. After one ambient completes, execute the latest pending ambient, then repeat if a newer one arrived. Ambient does not consume the reply-required admission semaphore. |
| `AgentServiceImpl.execute` | A successful ambient result sends only when `shouldReply`; ambient failure sends no fixed failure text; silent result sends nothing. |

**Intentional bounded adaptation:** Zenbot's existing runtime is a worker/channel design rather than Saturn's virtual-thread executor. Preserve observable latest-wins and silence behavior, not Java executor internals. The counter must be shared by the one master `live.RoomParticipation` instance, not reset per event or per user.

## Target shape

```text
public chat event
  -> existing chain through relay/mail/AFK/etc.
  -> live.RoomParticipation.Handle
     -> participation.Pipeline.Handle(Event{ambient enabled, cadence, Quiet})
     -> Pipeline increments its private atomic counter only after eligibility/quiet/mention precedence
         quiet request: Quiet.Silence(stable room/user key), PASS
         mention: submit MENTION, CLAIMED (existing behavior)
         cadence hit: submit AMBIENT, PASS
  -> runtime.AmbientBridge.Submit
     -> replace one pending ambient request; schedule at most one ambient drain
  -> same configured live.Runner -> assemble AMBIENT -> provider -> finalizer
     -> exact no-reply marker / blank / error: no outbound message
     -> visible result: existing sink sends `"\n" + content` addressed to invocation nick
  -> chain continues to command dispatch for every ambient/quiet path
```

## Implementation stages and ownership

### Stage A — make the existing participation adapter own cadence and quiet state

**Owner:** `@developer` (agent/listener integration); no factory, relay, or command changes.

Modify `internal/agent/live/participation.go` so `RoomParticipation` carries only composition configuration:

```go
type RoomParticipation struct {
    Pipeline *participation.Pipeline
    Snapshot func(*message.Context) participation.TrustedSnapshot
    AmbientEnabled bool
    AmbientEvery uint64
}
```

`Handle` continues to reject nil wiring and calls `Pipeline.Handle` exactly once. It supplies `AmbientEnabled` and `AmbientEvery` in the event, but **does not** calculate `EligibleCount`.

Move `eligibleAmbientMessages atomic.Uint64` into `participation.Pipeline` and remove `Event.EligibleCount`. `Pipeline.Handle` increments it directly immediately before its ambient modulo check: after its existing eligibility filter, quiet handling, and mention handling. This is the sole cadence state, maps one-to-one to Saturn, and makes callers unable to forge cadence state. The counter therefore does not advance for blank/whisper/self/conventional-bot/prefix events, quiet requests, or parseable mentions.

Add `Quiet: participation.NewQuietRegistry(time.Duration(resolved.QuietMinutes)*time.Minute)` to the production pipeline in `cmd/zenbot/main.go`. `AmbientEnabled` maps from `resolved.Ambient`; cadence maps from `resolved.AmbientEveryMessages`. Disabled agent configuration remains pass-through and creates neither provider nor runtime.

No new config field is needed. Preserve existing validation: enabled config already requires positive cadence/quiet duration. Convert only after validation; never cast a non-positive integer to `uint64`.

### Stage B — add a narrow ambient coalescer at the runtime boundary

**Owner:** senior agent/runtime reviewer + `@developer`.

Do not overload `Runtime.Submit`, alter `ErrBusy`, or make mentions share a replaceable queue: those are accepted direct/mention contracts. Add `SubmitAmbient` to the same runtime owner:

```go
func (rt *Runtime) SubmitAmbient(inv Invocation) error
```

Keep its pending/scheduled state private on `Runtime` so it shares `Close`, cancellation context, workers, room serialization, sink, and runner lifecycle without a second goroutine owner.

Required algorithm:

1. Convert and validate the API invocation exactly as `APIBridge` does; reject non-`AMBIENT` at this boundary as a programmer error.
2. Under the runtime mutex, return `ErrClosed` when closed. For accepted ambient, set `pendingAmbient = invocation` (overwrite any earlier pending value).
3. If an ambient drain is already scheduled/running, return nil. Otherwise set `ambientScheduled=true` and enqueue exactly one internal drain job. This scheduling must not reserve normal `admission` capacity and must not make an ambient return `ErrBusy` merely because direct/mention capacity is full.
4. The drain snapshots-and-clears the pending value, executes it through the existing `execute` path with no reply-required failure behavior, then checks the newest pending value. Continue until no pending value remains. Clear `ambientScheduled` only while holding the same mutex; if a new ambient arrived during the clear, reschedule/drain it.
5. `Close` sets closed, clears `pendingAmbient`, cancels runtime context, and waits for both ordinary and ambient drain execution before returning. No pending ambient may start after close. A running ambient receives cancellation through the same context.

A simple separate `ambientJobs` goroutine is acceptable only if `Close` owns/waits for it and it cannot race a post-close enqueue. Do not call the provider from listener code, spin unbounded goroutines, retry provider/transport failures, or send busy/unavailable text for ambient.

Wire a mode-dispatching submitter into the existing `participation.Pipeline`: it sends `MENTION` through the current `runtime.APIBridge` and converts/validates `AMBIENT` once before calling `rt.SubmitAmbient`. It must reject unsupported modes. This is the required in-slice prerequisite, not a blocker.

### Stage C — ambient output correctness at the existing finalization seam

**Owner:** senior agent/runtime reviewer.

Fix the one ambient-specific defect before enabling the behavior: `live.MarkerFinalizer.Finalize` currently makes a blank `AMBIENT` provider content a reply (`raw==""`, `shouldReply=true`), so the sink would emit a newline-only chat message (`internal/agent/live/runner.go`). Saturn's response finalizer rejects blank content; `AgentServiceImpl` suppresses ambient failures.

Change the narrow marker finalizer contract:

- Exact trimmed `NoReplyMarker`: `("", false, nil)` for `AMBIENT` and existing modes.
- Blank trimmed content: return `agent returned an empty response` for **every** mode. Runtime failure handling already suppresses it for ambient and retains the accepted fixed failure path for direct/mention.
- Embedded marker remains visible content.
- Do not fold in Saturn's full `AgentResponseCorrector`, `AgentResponseSanitizer`, quote-only policy, truncation, freshness validation, tools, or memory in this slice. Those are separate router work; this stage only prevents a visible blank ambient output and preserves current accepted live finalizer scope.

## Exact input, silence, failure, and output handling

| Event/result | Ambient counter | Submission | Listener result | External output |
|---|---:|---|---|---|
| Agent disabled | none | none | pass | none |
| Blank, whisper, self, conventional bot, prefixed command | unchanged | none | pass | none |
| Polite quiet request, including `@bot please be silent` | unchanged | none; store silence for author identity | pass | none |
| Valid mention, even while caller is quiet | unchanged | existing MENTION path | claimed | existing mention behavior/failure policy |
| Eligible unaddressed event, cadence not hit | +1 | none | pass | none |
| Eligible unaddressed event, cadence hit, runtime open | +1 | one AMBIENT accepted/coalesced | pass | later one addressed public reply only if visible final result |
| Multiple cadence hits while an ambient is running/pending | each qualifying event +1 | newest replaces pending ambient | pass | first running and latest pending may execute; stale pending does not |
| Ambient provider/assembly/finalizer error, including blank content | n/a | already accepted | pass | none; log operational error only, no fixed failure reply/retry |
| Exact trimmed marker | n/a | already accepted | pass | none |
| Marker embedded in prose | n/a | already accepted | pass | one normal sink delivery with original visible prose |
| Ambient sink/transport error | n/a | already accepted | pass | log/return internally; no retry or secondary failure reply |
| Runtime shutdown/close | no new accepted ambient | pending cleared; running canceled | no new listener work | no late delivery after cancellation |

The output address remains Saturn-compatible existing live behavior: `engine.SendChatMessage(inv.Context().Nick(), "\n"+result.Text(), inv.Context().Whisper())`. Ambient ingress rejects whispers, so accepted ambient replies are public; retain `Whisper()` rather than hard-coding false at the sink because the sink is shared with the accepted mention route.

## Focused TDD baseline

Perform strict RED→GREEN one behavior at a time; do not first implement runtime coalescing and write tests afterwards.

1. **RED — `internal/agent/live/participation_test.go`:** With ambient enabled/cadence 2, an eligible unaddressed first event passes/no submit and the second creates exactly one `AMBIENT` invocation; an intervening mention and polite quiet request do not advance cadence. This proves source ordering rather than merely modulo arithmetic.
2. **RED — same package:** A polite request silences only the stable user+room through expiry, suppresses that user's later ambient event, does not suppress another user's cadence hit, and does not suppress that user's direct mention. Use an injected clock through `NewQuietRegistryAt`, not sleeps.
3. **RED — `internal/agent/runtime/ambient_test.go`:** Block the first ambient runner; submit stale ambient, direct/mention, then latest ambient. Release the first and prove observed execution order is `first ambient`, normal admitted request(s) per the existing runtime contract, then `latest ambient`, never `stale ambient`. Assert one scheduled drain and no `ErrBusy` because normal admission is occupied.
4. **RED — same runtime test:** `Close` clears pending ambient, cancels a running ambient, waits for it, and later ambient submission returns `ErrClosed`.
5. **RED — `internal/agent/live/runner_test.go`:** blank ambient content returns an error and causes neither sink delivery nor failure sink delivery; exact marker remains silent and embedded marker remains visible.
6. **GREEN:** implement only the minimum Stage A/B/C code. Then add a composition test in `cmd/zenbot/live_agent_test.go` proving enabled resolved config places the non-nil quiet registry and ambient-mode submitter in the master listener path; disabled configuration stays pass-through.

## Verification commands

Run from `/Users/ab/workspace/go-projects/zenbot`; format only intentionally changed slice files.

```sh
gofmt -w \
  internal/agent/participation/invocation.go \
  internal/agent/live/participation.go internal/agent/live/runner.go \
  internal/agent/runtime/runtime.go internal/agent/runtime/api_bridge.go internal/agent/runtime/ambient.go \
  cmd/zenbot/main.go

go test ./internal/agent/participation -run 'Test(Pipeline|Quiet|Mention)' -count=1
go test ./internal/agent/live -run 'Test(.*Ambient|MarkerFinalizer)' -count=1
go test ./internal/agent/runtime -run 'Test(.*Ambient|Runtime)' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test -race ./internal/agent/participation ./internal/agent/live ./internal/agent/runtime ./internal/listener/message -count=1
go test ./...
go vet ./...
go build ./...
git diff --check
```

If Stage B adds no file named `ambient.go`, replace that path in `gofmt` with the actual slice-owned runtime file; do not format unrelated dirty work. Full race/vet/build remain relevant final-batch gates but are not required to block this rapid vertical absent a concrete failure, per `MIGRATION_PLAN.md` rapid policy.

## Exclusions

- No H2 schema/query work, durable memory, conversation history/context, or persistence changes.
- No tool registry bridge, tool loop, SQL policy, fresh-data loop, command-prose correction, or capability expansion.
- No moderation monitoring, semantic moderation, join automation, or protected-principal execution.
- No full Saturn response corrector/sanitizer/quote-only/truncation migration beyond the blank-ambient prerequisite stated above.
- No changes to direct `l` semantics, AGENT-child relay topology, remote/replica behavior, command ordering, or admission-failure UX for mention/direct.
- No protected-document edits, commits, resets, or cleanup of unrelated dirty changes.

## Complexity, risk, and developer routing

**Complexity:** moderate. The room-policy portion is small and source-aligned; the risk is concurrency lifecycle, not file volume.

**Primary risks:** counting messages at the wrong point in the pipeline, accidentally turning addressed quiet requests into mentions, replacing normal queued/direct work with ambient work, leaking a blank newline message, data racing the cadence counter, and allowing ambient drain after `Close`.

**Routing:**

- Stage A: `@developer`, reviewed by the existing live-agent integration owner.
- Stages B/C: senior agent/runtime reviewer plus `@developer`; these alter the shared runtime/finalizer behavior and require the focused race gate.
- Do not combine with a tool, memory, or broad router PR. Once this slice is accepted, the next candidate should be a concrete read-only `user_message_history` tool bridge plus minimal H2 conversation context, not speculative full tool infrastructure.
