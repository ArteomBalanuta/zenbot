# Rapid live-agent integration — implementation architecture

## Decision and delivery boundary

**Deliver two independent implementation slices in order:**

1. **Live mention route:** migrate the missing configured runner, response finalizer, trusted context adapter, listener composition, and shutdown ownership. This makes a public non-command `@<bot-nick> <prompt>` asynchronous end-to-end route live.
2. **Relay topology:** establish an explicit agent-child/host transport topology, then replace `RelayAgentMessage`'s no-op. This is not part of mention delivery and must not be smuggled into it.

[OBSERVED] Zenbot already has the essential seams but no production composition: `participation.Pipeline`, `runtime.APIBridge`, and `runtime.Runtime` are unused outside their packages; `message.AgentParticipation` and `message.RelayAgentMessage` are no-ops (`internal/agent/participation/invocation.go`, `internal/agent/runtime/*.go`, `internal/listener/message/handlers.go`). `live.DirectInvoker` is only a synchronous `l`-command adapter and cannot satisfy the listener path (`internal/agent/live/direct.go`).

[OBSERVED] Saturn puts relay after bot-ignore and participation immediately before command dispatch. A mention is always claimed after submission is attempted (`AgentRoomMessagePipeline.handleMention`), while `AgentParticipationHandler` translates only `PASS` into listener continuation. Saturn's service owns async admission, final delivery, reply-required failure text, and shutdown (`src/main/java/org/saturn/app/service/impl/AgentServiceImpl.java`).

[RECOMMENDED] Keep the first slice intentionally small: public, eligible, valid mentions only. Do not enable ambient, quiet, semantic moderation, tools, durable memory/history, or relay in that first merge. Migrate configuration needed to define the runtime and finalization now; defer enabling policy not required by a mention.

## Existing contracts to preserve

| Area | Grounded target behavior | Required live-route use |
|---|---|---|
| Listener chain | `Chain.Process` stops on `err != nil` or `next == false` (`internal/listener/message/chain.go`). | `CLAIMED` mention becomes `next=false`; submission failures return an error and never fall through to command dispatch. |
| Participation | `Pipeline.Handle` filters blank/whisper/self/conventional bot/prefix messages; a parseable mention invokes `submit(... MENTION, commandOriginated=true, Claimed)` and returns `Claimed` even when `Submit` fails (`internal/agent/participation/invocation.go`). | Use unchanged pipeline semantics, not custom mention parsing in the listener. |
| Runtime | `Runtime.Submit` is non-blocking and returns `ErrBusy`/`ErrClosed`; workers serialize by `Context.MemoryKey`; `Close` cancels runner context (`internal/agent/runtime/runtime.go`). | Use `runtime.APIBridge` as the sole participation submitter. Do not call the provider in `Handle`. |
| Delivery | `Runtime.execute` invokes `Sink.Deliver` only when runner returns nil error and `Result.ShouldReply()` (`internal/agent/runtime/runtime.go`). | Sink handles addressing/whisper mode. Runner owns result/finalization; listener does not send a reply. |
| Transport | `EngineImpl.SendChatMessage(author, text, whisper)` addresses public output and uses the source message's whisper choice; `SendAddressedMessage` has different formatting (`internal/core/engine_impl.go`). | Sink must call `SendChatMessage(inv.Context().Nick(), "\n"+result.Text(), inv.Context().Whisper())` for Saturn-compatible service formatting. |
| Capability construction | `participation.InvocationFactory.Create` assigns elevated capabilities only from `TrustedSnapshot`, never direct message claims (`internal/agent/participation/invocation.go`). | Snapshot must be made by a trusted composition-owned adapter from engine/config/security data. |

## Target shape after the live mention slice

```text
chat JSON
  → listener.UserChatListener.Notify (normalizes IsWhisper)
  → message.Chain.Process
      → ResolveUserMetadata → Audit → IgnoreBot → Relay(pass-through)
      → … existing handlers …
      → AgentParticipation{Participation: live.RoomParticipation}.Handle
          → participation.Pipeline.Handle(Event)
              → InvocationFactory.Create(TrustedSnapshot, message, prompt, MENTION, true)
              → runtime.APIBridge.Submit(api.Invocation)  [non-blocking]
          → PASS: continue to DispatchUserCommand
          → CLAIMED + nil: stop chain
          → CLAIMED + ErrBusy/ErrClosed/validation: stop chain; listener logs error
  → runtime worker (serialized by memory key)
      → live.Runner.Run: assemble → provider Complete → finalizer
      → Result{ShouldReply:false}: silence
      → Sink.Deliver: Engine.SendChatMessage(nick, "\n"+content, whisper)
```

## Slice 1 — migrate the live mention prerequisites

### Stage 1: resolved configuration and validation

**Ownership:** agent/runtime implementation owner. No listener wiring in this stage.

**Modify** `internal/config/agent_config.go`.

Add these TOML fields to `AgentConfig`, resolve them through the existing `ValueReader`, and retain their resolved values in `ResolvedAgentConfig` through embedding:

```go
CreatorTrip          string `toml:"creatorTrip"`
AmbientEveryMessages int    `toml:"ambientEveryMessages"`
QuietMinutes         int    `toml:"quietMinutes"`
ContextMessageLimit  int    `toml:"contextMessageLimit"`
NoReplyMarker        string `toml:"noReplyMarker"`
MaxConcurrentRequests int   `toml:"maxConcurrentRequests"`
QueueCapacity        int    `toml:"queueCapacity"`
```

Use Saturn-derived defaults, with names preserved exactly:

```text
creatorTrip = "595754"
ambientEveryMessages = 8
quietMinutes = 15
contextMessageLimit = 60
noReplyMarker = "[[SATURN_NO_REPLY]]"
maxConcurrentRequests = 1
queueCapacity = 0
```

`Ambient bool` remains backward-compatible configuration but is **not enabled by this slice**. It is insufficient by itself to reproduce ambient policy; do not reinterpret it as a cadence.

**Validation rules:**

- `creatorTrip` and `noReplyMarker`: `strings.TrimSpace` must be non-empty.
- `ambientEveryMessages`, `quietMinutes`, `contextMessageLimit`, `maxConcurrentRequests`: must be positive.
- `queueCapacity`: must be non-negative.
- Retain current endpoint/model/timeout/limits validation.
- Validate participation fields when `agent.enabled=true`; disabled configuration can still resolve defaults without requiring provider credentials.
- Pass `CreatorTrip` and `NoReplyMarker` to `assemble.New(assemble.Config{...}, catalog)`. This replaces the target assembly fallback `"NO_REPLY"` (`internal/agent/assemble/assemble.go`) with Saturn's configured marker.

**Focused TDD:** create `internal/config/agent_config_participation_test.go`.

1. RED: table test default `Resolve` values match all seven listed defaults and include a non-empty `NoReplyMarker`.
2. RED: table test rejects blank `creatorTrip`/`noReplyMarker`, zero/negative positive-only values, and negative `queueCapacity`; assert field-specific errors.
3. RED: explicit `ValueReader.Runtime` values override TOML struct values for each new field.
4. GREEN: add the fields, resolver calls, defaults, and validation.

### Stage 2: minimal runner and response finalizer

**Ownership:** agent/runtime implementation owner.

**Create** `internal/agent/live/runner.go` and `internal/agent/live/finalizer.go`.

Do not make `DirectInvoker` implement `runtime.Runner`: its API returns `(string,error)`, it requires a command message, and it treats an empty provider result as an error. Keep it unchanged for the `l` command.

Define the live runner explicitly:

```go
type Runner struct {
    Assembler *assemble.Assembler
    Client    llm.LlmClient
    Finalizer Finalizer
}

func (r Runner) Run(ctx context.Context, inv runtime.Invocation) (runtime.Result, error)
```

Minimal first-slice `Run` behavior:

1. Reject missing assembler/client and propagate `ctx.Err()` before doing work.
2. Call `Assembler.Assemble(ctx, inv, nil, "", nil, assemble.Talk)`. `nil` history/recent/tools are an explicit first-slice constraint, not an accidental omission.
3. Call `Client.Complete(ctx, prepared.LlmRequest())`.
4. Pass `response.Content()` plus `inv.RequestID()` to the finalizer.
5. Return `runtime.NewResult(inv.RequestID(), content, shouldReply)`, or return the provider/assembly/finalizer error unchanged/wrapped with operation context.

Define a narrow finalizer contract:

```go
type Finalizer interface {
    Finalize(inv runtime.Invocation, raw string) (content string, shouldReply bool, err error)
}

type MarkerFinalizer struct { NoReplyMarker string }
```

**Finalizer semantics for this slice:**

- Normalize only for decision-making with `strings.TrimSpace(raw)`; do not mutate reply content beyond the explicitly stated marker handling.
- If normalized content equals `NoReplyMarker`, return `("", false, nil)` for every mode. This matches Saturn's response-finalizer silence decision; it is essential even though the initial route is `MENTION`.
- If normalized content is empty, return an error for `DIRECT` or `MENTION` because they require a reply; this must cause the runtime to suppress sink delivery. Do not fabricate a provider answer.
- Otherwise return the provider content with `shouldReply=true`.
- Marker embedded in prose is content, not silence. Only exact trimmed equality is silent.
- This runner intentionally does not claim Saturn tool-loop, memory, context repository, truncation, or command-prose parity. Those must be added behind this `Runner` later, not inside listener code.

**Failure delivery policy:** `runtime.Runtime` currently discards runner errors. To preserve Saturn's reply-required failure behavior, add a **runner-result finalization boundary before runtime usage**, rather than asking the listener to reply. The smallest explicit change is to extend the runtime contract:

```go
type FailureSink interface {
    DeliverFailure(ctx context.Context, inv Invocation, err error)
}
```

and add an optional `failureSink FailureSink` argument/field to `runtime.New` (or a new `NewWithFailureSink` constructor that leaves current `New` source-compatible). When `Runner.Run` returns an error, invoke it only if `inv.Mode().RequiresReply()` and runtime is not canceled. Its concrete live implementation must send exactly:

```text
failed: the agent could not answer that request.
```

to `inv.Context().Nick()` using `inv.Context().Whisper()`. It must log but not retry a transport send failure. Do not send a failure for `AMBIENT` or `MODERATION` when those later modes are added.

For admission rejection, `runtime.APIBridge.Submit` returns an error synchronously before a worker exists. The participation adapter must preserve target `Claimed` behavior and return this error to the listener; the bridge cannot safely produce an asynchronous failure. In this first slice, the listener log is the observable rejection, matching existing target chain error handling. A later UX-parity slice may add an explicit admission-failure delivery policy after confirming it cannot block or race the listener.

**Focused TDD:** create `internal/agent/live/runner_test.go` and extend `internal/agent/runtime/runtime_test.go`.

- `Runner.Run` sends an assembled `MENTION` request to a scripted client and returns correlation ID equal to request ID.
- Exact marker with surrounding whitespace returns `ShouldReply()==false`; `"answer [[SATURN_NO_REPLY]]"` remains a reply.
- Empty mention output fails and does not produce a reply result.
- Provider error and canceled context are returned; no sink delivery occurs.
- Runtime invokes `FailureSink` once for a failed `MENTION`, never for `AMBIENT`, and never after `Close` cancellation.
- Existing `Runtime` admission, serialization, reply-only sink, and close tests remain green.

### Stage 3: trusted room-context and participation adapter

**Ownership:** agent/listener integration owner.

**Create** `internal/agent/live/participation.go`.

Define the listener-facing interface in `internal/listener/message` first so message handlers do not import agent internals:

```go
type Participation interface {
    Handle(context.Context, *Context) (claimed bool, err error)
}

type PassParticipation struct{}
func (PassParticipation) Handle(context.Context, *Context) (bool, error) { return false, nil }
```

Define `live.RoomParticipation` to implement it:

```go
type RoomParticipation struct {
    Pipeline *participation.Pipeline
    Snapshot func(*message.Context) participation.TrustedSnapshot
}

func (p RoomParticipation) Handle(ctx context.Context, c *message.Context) (bool, error)
```

`Handle` must:

1. Reject programmer configuration defects (`nil` context/message/pipeline/snapshot) with an error; it must not panic in a listener goroutine.
2. Build `participation.Event` from the already normalized `c.Message`, `Snapshot(c)`, `c.Engine.GetName()`, and `c.Engine.GetPrefix()`.
3. Set `AmbientEnabled=false`, `AmbientEvery=0`, `EligibleCount=0`, and `ModerationCandidate=false` in this first slice.
4. Call `Pipeline.Handle(event)` exactly once.
5. Return `out.Decision == participation.Claimed, out.Err`. Never convert an error to pass.

Build the `TrustedSnapshot` only from trusted owners at composition time:

```text
Room        = engine.GetChannel()
Users       = copied current active-user names
CreatorTrip = resolved.CreatorTrip
AdminTrips  = copied config.AdminTrips
Roles       = nil in the mention slice
```

Do **not** derive `AdminTrips`, creator identity, or roles from the `model.ChatMessage`. [OBSERVED] `InvocationFactory.Create` already gates elevated capabilities from this snapshot. [LIMITATION] the current common-engine interface exposes active users and `IsUserAuthorized`, but not a trusted role map. Therefore roles remain nil rather than guessing. Admin triples from `config.Config.AdminTrips` and creator trip from resolved agent config are sufficient for the existing capability factory's grounded branches.

Active users are mutable. The snapshot function must copy names before returning; it must not retain/return the `map[*model.User]struct{}` owned by `EngineImpl`. If concurrent map access proves unsafe under the race test, add a read-safe `ActiveUserNames() []string` snapshot method on the concrete trusted engine/service boundary rather than iterating a map in the listener package.

Instantiate dependencies as follows:

```go
bridge := runtime.APIBridge{Runtime: rt}
pipeline := &participation.Pipeline{
    Factory: participation.NewInvocationFactory(nil),
    Parser:  participation.MentionParser{},
    Submit:  bridge,
    // Quiet nil; Monitor nil; no ambient/moderation in this slice.
}
participation := live.RoomParticipation{Pipeline: pipeline, Snapshot: snapshot}
```

`InvocationFactory.Snapshot` is currently unused by `Create`; pass `nil` only if it remains unused. Prefer deleting that unused field in a separately scoped cleanup rather than making the live path depend on it.

**Focused TDD:** create `internal/agent/live/participation_test.go`.

- Public `"@bot: help"` produces exactly one `api.MENTION` invocation with prompt `"help"`, `commandOriginated=true`, normalized room/nick/trip/hash, copied active users, and expected creator/admin capabilities.
- Whisper, self, conventional bot, prefixed command, blank text, and non-mention return `(false,nil)` and do not submit.
- A mention with a submitter `ErrBusy` returns `(true, ErrBusy)`, proving it remains claimed.
- Snapshot copied values cannot be mutated by caller after submission.

### Stage 4: listener injection and exact chain behavior

**Ownership:** listener integration owner.

**Modify** `internal/listener/message/handlers.go`:

```go
type AgentParticipation struct { Participation Participation }

func (h AgentParticipation) Handle(ctx context.Context, c *Context) (bool, error) {
    p := h.Participation
    if p == nil { p = PassParticipation{} }
    claimed, err := p.Handle(ctx, c)
    if err != nil { return false, err }
    return !claimed, nil
}
```

This makes a claimed mention stop the chain; an adapter error also stops the chain because `Chain.Process` returns it immediately. A disabled agent installs `PassParticipation{}` deliberately; never use a nil pointer as configuration.

Replace or augment `DefaultChain` without breaking existing tests and non-agent callers:

```go
func DefaultChain() *Chain { return DefaultChainWithParticipation(PassParticipation{}) }
func DefaultChainWithParticipation(p Participation) *Chain
```

The constructed sequence must remain exactly:

```text
ResolveUserMetadata → AuditChatMessage → IgnoreBotMessage → RelayAgentMessage
→ LogChatMessage → DeliverPendingMail → UpdateAfkState → YoutubePreview
→ CernEasterEgg → AgentParticipation → DispatchUserCommand
```

**Modify** `internal/listener/user_chat_listener.go`:

```go
func NewUserChatListener(e common.Engine) *UserChatListener
func NewUserChatListenerWithChain(e common.Engine, chain *message.Chain) *UserChatListener
```

The existing constructor delegates to `DefaultChain`; the injected constructor rejects/replaces nil with `DefaultChain()` so production and tests cannot panic. `Notify` keeps current JSON normalization and logging; it must not use `context.Background()` as a cancellation substitute for runtime work because `Runtime` owns process shutdown cancellation.

**Focused TDD:** create `internal/listener/message/agent_participation_test.go` and `internal/listener/user_chat_listener_participation_test.go`.

- Handler maps pass to `next=true`, claimed to `next=false`, and claimed-plus-error to returned error (no subsequent sentinel handler invocation).
- `DefaultChainWithParticipation` has the observed handler order and contains the injected `AgentParticipation` instance before `DispatchUserCommand`.
- An integration chain with a recording participation adapter receives normalized public message data; `@bot hello` stops before sentinel command dispatch.
- A non-mention reaches the sentinel command dispatch; the adapter makes no submit.
- Malformed JSON retains existing log-and-return behavior; no participation invocation.

### Stage 5: composition root, delivery, and lifecycle

**Ownership:** application/composition owner; this is the only production wiring stage.

**Modify** `cmd/zenbot/main.go`. Factor a tested helper rather than placing construction in `main`:

```go
type liveAgent struct {
    Runtime       *runtime.Runtime
    Participation message.Participation
}
func newLiveAgent(c *config.Config, engine common.Engine) (*liveAgent, error)
func (a *liveAgent) Close()
```

Construction order inside the helper:

1. Resolve `c.Agent` once from environment values. Reuse this resolved object for direct-command and room-agent construction; avoid resolving one config twice with potentially divergent values.
2. If disabled, return a `liveAgent` with `message.PassParticipation{}` and nil runtime. This lets process startup remain possible when the agent is intentionally disabled. Preserve the existing deliberate `l`-command policy separately: either do not register it when disabled, or keep its current startup failure only if product ownership explicitly retains that behavior. Do not make disabled room participation fatal.
3. Create OpenAI client, prompt catalog, and assembler with `assemble.Config{CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker}`.
4. Create `live.Runner{Assembler: assembler, Client: client, Finalizer: live.MarkerFinalizer{NoReplyMarker: resolved.NoReplyMarker}}`.
5. Create a sink bound to the engine only through `common.Engine`:

```go
runtime.SinkFunc(func(ctx context.Context, inv runtime.Invocation, result runtime.Result) error {
    if err := ctx.Err(); err != nil { return err }
    _, err := engine.SendChatMessage(inv.Context().Nick(), "\n"+result.Text(), inv.Context().Whisper())
    return err
})
```

6. Create the optional reply-required `FailureSink` described in Stage 2.
7. Call `runtime.NewWithFailureSink(runtime.Config{MaxConcurrent: resolved.MaxConcurrentRequests, QueueCapacity: resolved.QueueCapacity}, runner, sink, failureSink)`.
8. Construct `runtime.APIBridge`, `participation.Pipeline`, `live.RoomParticipation`, and its trusted snapshot closure.
9. After `factory.NewEngineWithOptions` returns the permanent master, replace its installed `UserChatListener` with `listener.NewUserChatListenerWithChain(e, message.DefaultChainWithParticipation(agent.Participation))`. This avoids broadening `factory.EngineOptions` merely to pass an application-only dependency. Do not alter temporary/replica factory behavior in this slice.
10. Register direct utilities independently from mention-route availability. Reuse the same provider/assembler only if ownership/lifetime makes it safe; otherwise leave `DirectInvoker` construction separate but use the same resolved config.

**Shutdown ordering:** on signal, stop accepting/processing agent work before tearing down the engine transport and database:

```text
signal context canceled
→ liveAgent.Close()  // Runtime.Close: reject new work, cancel runners, wait
→ lifecycle.Stop(stopCtx)
→ manager.StopAll(stopCtx)
→ deferred DB/server close
```

Do not `defer Runtime.Close()` after the database teardown defer if that reverses the required ordering. Sink delivery after cancellation must not race a stopped transport.

**Focused TDD:** create `cmd/zenbot/live_agent_test.go` if package-level construction can be isolated; otherwise create `internal/agent/live/composition_test.go` and keep `main` a thin call.

- Enabled valid config produces non-nil runtime and an injected participation path.
- Disabled config produces pass-through participation without provider/client construction.
- Sink delivers to requester with a newline-prefixed body and invocation whisper flag.
- `Close` causes later `Submit` to return `runtime.ErrClosed` and a blocked runner to observe cancellation before engine teardown is invoked (test through interfaces/fakes, not real signals).

## Input, output, silence, and failure contract for the live mention route

| Condition | Submit? | Chain continuation | Outbound result |
|---|---:|---:|---|
| blank, whisper, self/bot author, command-prefix, invalid/no-content mention | no | continue | none |
| public valid `@bot` mention, accepted | once, `MENTION` | stop | async addressed public reply if final result says reply |
| valid mention, exact no-reply marker | once | stop | none |
| valid mention, runner/provider/finalizer error | once | stop | one fixed reply-required failure via failure sink, unless shutdown cancellation |
| valid mention, admission busy/closed/invalid invocation | attempted, rejected | stop; listener logs error | none in this slice |
| disabled agent | no | continue | none |
| sink/transport error | already submitted | already stopped | log/return from sink; no retry and no secondary error reply |
| process shutdown | admission closes/cancels | no new work | no late delivery after cancellation |

## Baseline implementation commands

Run from `/Users/ab/workspace/go-projects/zenbot` after each stage; preserve unrelated dirty changes and stage only slice-owned files.

```sh
gofmt -w internal/config/agent_config.go \
  internal/agent/live/runner.go internal/agent/live/finalizer.go internal/agent/live/participation.go \
  internal/agent/runtime/runtime.go internal/agent/runtime/contracts.go \
  internal/listener/message/handlers.go internal/listener/user_chat_listener.go cmd/zenbot/main.go

go test ./internal/config -run 'TestAgentConfig.*Participation|TestAgent.*Resolve' -count=1
go test ./internal/agent/live ./internal/agent/runtime ./internal/agent/participation -count=1
go test ./internal/listener/message ./internal/listener -run 'Test.*Participation|TestDefaultChain' -count=1
go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
go test ./...
git diff --check
```

Run `go test -race ./internal/agent/runtime ./internal/agent/live ./internal/listener/...` before merging because the route snapshots active users and has async runtime/sink ownership. The baseline must be green before enabling ambient or relay.

## Slice 2 — relay topology (separate design and merge)

### Why it is separate

[OBSERVED] Saturn relay only applies to `EngineType.AGENT`: it takes a child engine's inbound text, enqueues `nick + ": " + escapeJava(text)` on its `hostRef`, calls `hostRef.shareMessages()`, and stops the child chain even when `hostRef` is absent (`RelayAgentMessageHandler.java`). Zenbot has only `MASTER`, `REPLICA`, and `ZOMBIE`, no `AGENT` type, no host reference, and no host queue-flush API (`internal/model/engine_type.go`, `internal/core/engine_impl.go`, `internal/common/engine.go`). Therefore reusing a replica or the mention sink would invent source behavior.

### Required topology contract

**Create** `internal/relay/topology.go` (or `internal/agent/relay/topology.go`; choose one package and keep listener dependencies one-directional) with a narrow host abstraction:

```go
type HostRelay interface {
    RelayAgentMessage(ctx context.Context, author, text string) error
}

type AgentHostRef interface {
    HostRelay() HostRelay
}
```

**Modify** `internal/model/engine_type.go` to add `AGENT` only with a documented ownership meaning: an agent-child connection whose inbound chat is relayed to its host and never independently participates in commands/room automation.

**Modify** `internal/core/engine_impl.go` and `internal/factory/engine_factory.go` to carry a private/set-at-creation host relay dependency for `AGENT`; it must not be mutable global state. Add an explicit factory entry point/options field, e.g.:

```go
type EngineOptions struct { /* existing */ HostRelay relay.HostRelay }
```

Reject `model.AGENT` construction without `HostRelay` at the factory boundary. If compatibility forces construction, its relay handler must safely log and claim the message; it must never pass it to command/participation handlers.

Implement the permanent host's `RelayAgentMessage` with its existing outbound API, not a new queue protocol:

```go
func (h *hostRelay) RelayAgentMessage(ctx context.Context, author, text string) error {
    if err := ctx.Err(); err != nil { return err }
    _, err := h.engine.SendChatMessage("", author+": "+escapeRelayText(text), false)
    return err
}
```

This uses the host transport's direct send path. It is an intentional Zenbot adaptation: Saturn's `enqueue + shareMessages` is not directly representable because Zenbot has no public host flush method and `SendChatMessage` sends immediately through transport. Preserve the required externally visible content and one delivery, not Java's internal queue mechanism.

**Escaping decision:** do not port `StringEscapeUtils.escapeJava` blindly. Zenbot's `SendChatMessage` already applies JSON escaping through `escapeJSON` (`internal/core/engine_impl.go`); double-escaping would visibly alter content. Define `escapeRelayText` only if direct tests prove a chat-protocol-level transformation is needed before JSON serialization. Its initial implementation should be identity over valid Go strings, with JSON serialization left to `SendChatMessage`.

### Relay handler and semantics

**Modify** `internal/listener/message/handlers.go`:

```go
type RelayAgentMessage struct { /* resolver or narrow agent-engine capability */ }

func (h RelayAgentMessage) Handle(ctx context.Context, c *Context) (bool, error) {
    if agent, ok := c.Engine.(interface { EngineType() model.EngineType; HostRelay() relay.HostRelay }); ok && agent.EngineType() == model.AGENT {
        host := agent.HostRelay()
        if host == nil {
            log.Printf("agent relay host missing")
            return false, nil
        }
        if err := host.RelayAgentMessage(ctx, c.Message.Name, c.Message.Text); err != nil {
            return false, err
        }
        return false, nil
    }
    return true, nil
}
```

- Non-AGENT engines: `(true,nil)`; their normal listener chain, including the new mention path, remains unchanged.
- AGENT with host: relay exactly one `"<nick>: <text>"` host public message, then `(false,nil)`.
- AGENT without host: log warning, `(false,nil)`, zero outbound messages. Never let child text run its command/agent participation handlers.
- Host relay transport error: `(false,error)` so the listener logs it; do not retry or fall through.
- Relay must precede log/mail/AFK/participation/dispatch, as it already does in the chain.

**Focused TDD:** create `internal/listener/message/relay_agent_message_test.go`; extend `internal/factory/engine_factory_test.go`; add narrow host-relay tests in the chosen relay package.

1. MASTER and REPLICA return continuation and never call host relay.
2. AGENT with host relays one exact author/text pair and stops the chain before a sentinel handler.
3. AGENT without host stops and makes no outbound delivery.
4. Host error is returned and the sentinel is not called.
5. Factory rejects AGENT without host relay and installs the host reference exactly once.
6. Newline, quotes, backslashes, and non-ASCII text are delivered once with no double escaping; assert the generated chat payload as well as the logical message.

**Relay validation commands:**

```sh
gofmt -w internal/model/engine_type.go internal/core/engine_impl.go internal/factory/engine_factory.go \
  internal/listener/message/handlers.go internal/relay/topology.go

go test ./internal/relay ./internal/listener/message -run 'TestRelayAgentMessage' -count=1
go test ./internal/factory -run 'Test.*Agent.*Host' -count=1
go test ./...
git diff --check
```

## Explicit deferrals after these two slices

- Ambient cadence/coalescing, quiet requests, and the existing `Ambient bool` policy activation.
- Moderation monitoring/actions, semantic moderation, bot-presence policy, and join automation.
- Full Saturn router migration: durable turn memory, conversation-context repository query bounded by `contextMessageLimit`, tools, tool loop/budgets, fresh-data policy, command-prose policy, and tool evidence persistence.
- Replica/remote-room behavioral parity beyond the distinct AGENT-child relay topology.
- User-facing admission-rejection reply policy, pending a separate decision about safe nonblocking delivery before listener error logging.

## Senior routing

- **Stage 1–2:** `@developer` only with a senior agent/runtime reviewer. The finalizer and failure-sink addition are contract changes that affect every future runner.
- **Stage 3–5:** `@developer` after Stage 2 is merged and its focused/full tests pass. This is standard dependency injection plus listener composition, but must be reviewed for trusted snapshot construction and shutdown ordering.
- **Relay slice:** senior architecture owner plus `@developer`; it introduces a new engine ownership/topology contract and must not be delegated as a handler-only change.
- **Do not combine slices in one PR.** The mention route must be independently runnable and testable while relay remains pass-through.
