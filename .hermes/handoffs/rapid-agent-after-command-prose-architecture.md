# Rapid agent parity after command-prose: source-shaped final response normalization and bounding

## Decision

Select **Saturn's deterministic final-output contract only**: legacy-persona cleanup, marker-aware visibility, list normalization, and Unicode-safe `maxOutputChars` bounding immediately before Zenbot sends or persists an agent answer.

This is the next bounded high-value parity capability after the accepted command-prose channel. The current target `live.MarkerFinalizer` only trims to recognize an exact marker, rejects blank text, and otherwise returns the provider's raw content unchanged (`internal/agent/live/runner.go`, `MarkerFinalizer.Finalize`). Thus raw legacy persona boilerplate, unbounded output, and an embedded no-reply marker can reach the chat sink and durable memory. Saturn makes final visible content a separate deterministic boundary before reply/delivery (`src/main/java/org/saturn/app/agent/routing/AgentResponseFinalizer.java`, `prepare`; `AgentResponseSanitizer.java`, `sanitize`; `AgentTextBounds.java`, `truncate`).

This is **not** a generic response corrector, provider fallback, retry, tool, transport, SQL, or routing rewrite. No provider completion is added or replaced. The command-prose loop and its typed `SuppressReply` result remain authoritative: a command-delivered turn bypasses this finalizer just as it does today.

## Evidence map: observed versus recommended

| Classification | Repository-relative evidence | Verified observation or recommendation |
|---|---|---|
| [OBSERVED] | `src/main/java/org/saturn/app/agent/routing/AgentResponseFinalizer.java`, `prepare` | Saturn first obtains corrected/freshness-validated content in its broader router, then sanitizes, applies mode-aware marker behavior, removes embedded marker text, bounds output by code point, and returns a visible/silent result. This slice takes only the deterministic final-output operations after correction/freshness, not the prior correction calls. |
| [TEST-BACKED] | `src/test/java/org/saturn/app/agent/routing/AgentResponseSanitizerTest.java` | Saturn removes the legacy `[sips tea]`/opening/boilerplate patterns, normalizes Markdown unordered/numbered list items to `\u2009-\u2009`, preserves ordinary evidence, and maps null/blank content to empty. |
| [TEST-BACKED] | `src/test/java/org/saturn/app/agent/routing/AgentResponseFinalizerTest.java`, `rejectsNoReplyMarkerForARequiredDirectResponse`, `rejectsAnActuallyEmptyResponse` | A required direct response cannot silently use the exact marker; empty sanitized output is an error. |
| [OBSERVED] | `src/main/java/org/saturn/app/agent/routing/AgentResponseFinalizer.java`, `removeNoReplyMarker`; `AgentTextBounds.java`, `codePointCount`/`truncate` | An ambient/non-required exact marker is silent; an embedded marker is removed before delivery; truncation counts Unicode code points rather than UTF-16 units or bytes. |
| [OBSERVED] | `internal/agent/live/runner.go`, `MarkerFinalizer.Finalize`; `internal/agent/live/direct.go`, `InvokeCompletion` | Zenbot currently leaves embedded markers and all legacy formatting untouched, has no output maximum, and uses the same finalizer for runtime and direct paths. Both paths persist only a returned visible final text after their existing successful delivery seam. |
| [OBSERVED] | `internal/agent/runtime/contracts.go`, `Mode.RequiresReply`; `internal/agent/runtime/runtime.go`, runtime delivery path | Runtime mode semantics distinguish reply-required work from ambient silence/failure behavior. The existing runtime owns send-before-`Runner.AfterDelivery` sequencing. |
| [OBSERVED] | `internal/agent/live/tool_loop.go`, `Completion.SuppressReply`; `internal/agent/live/runner.go`, `Runner.Run`; `internal/agent/live/direct.go`, `InvokeCompletion` | The accepted command-prose path produces typed suppression before finalization. Final output normalization must not turn a successful command's deliberately empty completion into a new delivery/persistence path. |
| [LIMITATION] | `src/main/java/org/saturn/app/agent/routing/AgentResponseFinalizer.java`, `responseCorrector` calls | Saturn also invokes failure-placeholder, internal-evidence, and quote-only correction. Those can issue further provider work and are explicitly excluded by the fixed Zenbot one-call/two-completion contract and this task's no-generic-correction constraint. |
| [RECOMMENDED] | Target `internal/config/agent_config.go`, `AgentConfig.Resolve`/`Validate` | Add one resolved `maxOutputChars` setting with Saturn's observed default of `8000`, bounded through the existing target config reader. The setting bounds only final user-visible model prose; it neither alters `maxTokens` nor changes tool/result budgets. |

## Ranked alternatives

| Rank | Candidate | Decision |
|---:|---|---|
| 1 | **Final response normalization and Unicode output bound** | **Selected.** It closes an exposed target gap on every already-live direct, mention, relay, and ambient final reply; it needs no new provider call, persistence schema, command authority, or tool registration. |
| 2 | `database_schema` / `database_query` / `database_sql` | Deferred. Saturn requires capability-gated database metadata/query execution and H2 visibility/policy composition (`DatabaseSchemaTool.java`, `DatabaseQueryTool.java`, `DatabaseSqlTool.java`); this violates the bounded no-broad-SQL scope. |
| 3 | `SaturnCommandToolCatalog` / per-command reflected tools | Deferred. The source scans command handlers and widens the provider surface (`SaturnCommandToolCatalog.java`); accepted Zenbot intentionally exposes only fixed `run_command`. |
| 4 | Saturn `AgentResponseCorrector` or quote-only correction | Rejected for this vertical. It changes provider-call state, may require further completions, and is precisely the generic correction/fallback excluded by the task. |
| 5 | Moderation monitor/action execution | Deferred. It requires protected-principal, authorization, audit, join/message telemetry, and real public enforcement behavior. |
| 6 | Room automation/quiet/ambient/session scheduling | Already delivered in the accepted live path: target has quiet registry, cadence, and latest-wins ambient admission (`internal/agent/participation/*`, `internal/agent/runtime/runtime.go`). Do not reselect it. |

## Exact selected contract

### Deterministic transform order

For every non-suppressed provider response that reaches `Finalizer.Finalize(invocation, raw)`:

1. **Sanitize deterministic legacy formatting only.** Treat nil/blank input as empty. Remove Saturn's evidenced legacy persona markers; strip an initial Markdown decoration sequence and legacy opening; remove complete legacy boilerplate lines; convert list items matching `*`, `•`, `N.`, or `N)` into `\u2009-\u2009<content>`; preserve ordinary content and all nonmatching lines. This is a source-port, not a broad content policy.
2. **Reject empty sanitized content** with the stable existing target error text `agent returned an empty response`.
3. **Exact marker mode handling.** Compare trimmed sanitized content with the resolved `NoReplyMarker`:
   - when `inv.Mode().RequiresReply()` is false (ambient), return `("", false, nil)`;
   - when `inv.Mode().RequiresReply()` is true (direct/mention reply-required turn), return `agent declined a required response` and do not deliver/persist.
4. **Remove embedded marker occurrences**, then trim only ASCII control whitespace (`' '`, `\t`, `\n`, `\r`) from both ends, matching Saturn's `removeNoReplyMarker`/`trimControlWhitespace` boundary. If that produces empty content, return the existing empty-response error.
5. **Unicode-safe bound.** Truncate the visible content to at most resolved `MaxOutputChars` Unicode code points (Go runes). Do not append an ellipsis, split a rune, or use byte length. Return the bounded content with `shouldReply=true`.

The marker comparison is exact only after the first sanitation/trim. Embedded ordinary marker text is not silence; it is removed and remaining prose is delivered. This is a deliberate correction to current target tests that expect embedded marker preservation (`internal/agent/live/runner_test.go`, `TestMarkerFinalizer`), because Saturn's source finalizer removes it.

### Source-shaped sanitizer boundary

Create `internal/agent/live/response_sanitizer.go` as an unexported pure helper, for example:

```go
type responseSanitizer struct{}
func (responseSanitizer) sanitize(raw string) string
func (responseSanitizer) containsLegacyPersona(raw string) bool
```

Keep it `live`-local because its sole consumer is user-visible finalization. Do **not** modify `turn.MemoryStore` filtering in this slice: that existing load-time legacy filtering has a different responsibility and must remain a regression boundary, not become a second final-output pipeline.

Port the source patterns behaviorally, with Go `regexp`/Unicode equivalents where Java syntax differs. Do not copy the Java expressions blindly. The implementation must be linear over the input where practical and must never make a network, repository, provider, tool, command, runtime, or clock call.

### Configuration contract

Extend the target's existing `[agent]` config only with:

```toml
# Maximum Unicode code points sent as one visible agent response (1..1000000).
maxOutputChars = 8000
```

Add `MaxOutputChars int `toml:"maxOutputChars"`` to `config.AgentConfig` and resolve it through the existing `ValueReader.Int("maxOutputChars", ...)` path. Default absent/zero file configuration to `8000` before runtime value resolution, so an explicit runtime zero remains invalid under existing precedence rules. Require `1..maxConfigLimit` when the agent configuration is resolved; retain disabled-agent parsing compatibility but do not construct a finalizer/client/runtime merely because this field exists.

This is a bounded target adaptation of Saturn's `AgentConfigLoader` default of `8000` (`src/main/java/org/saturn/app/agent/config/AgentConfigLoader.java`, `load`). Zenbot uses rune count, which is the correct Go representation of Saturn code points for valid strings. No timeout, cancellation budget, max token, tool timeout, max steps, or provider option changes.

### Finalizer shape and composition

Replace the marker-only value with a configuration-owned deterministic finalizer while preserving the narrow existing interface:

```go
type OutputFinalizer struct {
    NoReplyMarker string
    MaxOutputChars int
}
func (f OutputFinalizer) Finalize(inv runtime.Invocation, raw string) (string, bool, error)
```

`MarkerFinalizer` may remain as a compatibility alias/wrapper only if current tests or external construction require it, but production `cmd/zenbot/main.go` must instantiate the one `OutputFinalizer` with both resolved values for `newLiveAgent` and `directAgentInvoker`. Avoid an interface expansion: `Finalizer` stays `Finalize(runtime.Invocation, string) (string, bool, error)`.

## End-to-end effects

```text
public DIRECT l | MENTION | AGENT relay | admitted AMBIENT
  -> existing bounded ToolLoop or provider-only path
  -> command prose action success?
       yes: Completion.SuppressReply -> existing no delivery/no persistence (unchanged)
       no: OutputFinalizer.Finalize(invocation, provider content)
            -> deterministic sanitize
            -> blank: error
            -> exact marker + non-required: silent
            -> exact marker + required: error
            -> remove embedded marker -> ASCII-control trim -> empty: error
            -> rune cap -> visible final text
  -> existing direct send / runtime sink
  -> only on successful visible delivery: existing conversation + eligible read-evidence persistence
```

The finalizer is synchronous and per invocation. It does not retain state and therefore cannot reorder tool protocol messages, submit work, or alter `ToolLoop`'s frozen one tool call/two completion maximum. It receives completion-two synthesis or completion-one no-tool content exactly where the current finalizer does.

## Visibility, authorization, timeout, cancellation, delivery, and persistence

| Concern | Required semantics |
|---|---|
| Visibility | Transform only final assistant content. It must not read prompts, current-room context, private memory, tool envelopes, command output capture, room users, nick/trip/hash, capabilities, or H2 data. No new cross-room/private disclosure path exists. |
| Authorization | No model-controlled or caller-controlled authorization is introduced. The transform cannot execute `run_command`, create a tool call, grant capability, or alter the accepted fixed command aliases. |
| Timeout | No independent timeout, goroutine, queue, retry, provider request, or transport call. Existing provider/tool timeouts occur before this pure function. |
| Cancellation | Runner/direct retain their existing context checks before provider/tool work. Once called, finalization is synchronous; if cancellation is observed before finalization, current code returns before delivery. Do not add a late cancellation retry or post-finalizer goroutine. |
| Required reply | Exact sanitized marker is an error for `RequiresReply()==true`; runtime/direct use their existing error paths. It is not converted to an ordinary text reply, a command, a second completion, or durable state. |
| Ambient | Exact marker remains silent. Sanitized blank/error remains a runtime error that the existing ambient runtime suppresses/logs without a fixed failure chat; no newline-only send and no memory/evidence append. |
| Delivery | A visible bounded string passes unchanged to the current sink/direct sender exactly once. Sanitizer output is not independently sent, and marker removal never causes a second message. `SuppressReply` still returns before finalization. |
| Persistence | Existing `Runner.AfterDelivery` and `DirectInvoker.PersistDelivery` run only after successful visible delivery. They persist the finalized bounded text, never raw provider content. Marker silence, empty/declined/error, cancellation, failed sink/send, and command suppression append neither conversation nor durable evidence. |
| Tool protocol | No change to advertised definitions, call IDs, assistant/tool pairing, registry, ledger, schema validation, one-call limit, two-completion limit, mandatory fresh-history precedence, or command-prose correction. |
| Concurrency/order | `OutputFinalizer` has no mutable state. Existing runtime same-memory-key serialization and ambient latest-wins behavior are untouched. Within each invocation, finalization remains after the last accepted provider completion and before the one existing delivery/persistence sequence. |

## Bounded file and interface plan

### Stage A — pure source-shaped normalization

**Modify/create:**

- create `internal/agent/live/response_sanitizer.go`
- create `internal/agent/live/response_sanitizer_test.go`
- modify `internal/agent/live/runner.go`
- extend `internal/agent/live/runner_test.go`

1. Add the pure sanitizer and tests porting the source behavioral examples.
2. Add `OutputFinalizer` and make it run the exact ordered contract above.
3. Keep `Finalizer` unchanged. If retaining `MarkerFinalizer`, implement it as a compatibility wrapper that delegates to `OutputFinalizer` with a documented safe test default; do not let production retain an unbounded marker-only implementation.
4. Do not touch `ToolLoop`, command channel, runner delivery logic, runtime, direct invocation logic, persistence, or prompt resources in this stage.

### Stage B — config and one composition source

**Modify:**

- `internal/config/agent_config.go`
- `internal/config/agent_config_participation_test.go` or a focused new config test
- `config.example.toml`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`

1. Resolve/validate/default `maxOutputChars` using target config conventions and source default `8000`.
2. Build exactly one configured `live.OutputFinalizer` value in `main`, or a tiny constructor called by both `newLiveAgent` and `directAgentInvoker`; both public runtime and direct `l` must receive identical marker/cap behavior.
3. Retain disabled-agent ordering: no provider, tool loop, runtime, direct invoker, or new persistence work is constructed when the agent is disabled.
4. No resource file is required: the source contract is code/config finalization, not a prompt or correction resource.

## RED → GREEN test sequence

1. **RED — sanitizer behavior:** create `internal/agent/live/response_sanitizer_test.go`. Port the source cases: `[sips tea]`, legacy opening and `Carpe diem` boilerplate removal; ordinary evidence preservation; `*`, bullet, numeric `.`/`)` list conversion; null-equivalent empty string/whitespace; and Unicode ordinary content preservation. Run:
   ```sh
   go test ./internal/agent/live -run 'TestResponseSanitizer' -count=1
   ```
2. **GREEN — pure sanitizer:** add only `response_sanitizer.go`; rerun the focused test.
3. **RED — finalizer contract:** extend `TestMarkerFinalizer` or add `TestOutputFinalizer` to prove: source-style embedded marker removal; required direct/mention marker error; ambient marker silence; sanitation-to-empty error; 8000/short test cap truncation by rune with no split emoji; raw long bytes do not bypass rune cap; and list-normalized output is what finalization returns. Prove normal ordinary prose remains visible after the deterministic transform.
4. **GREEN — finalizer:** implement `OutputFinalizer`; retain a narrow compatibility wrapper only if needed. Run:
   ```sh
   go test ./internal/agent/live -run 'Test(OutputFinalizer|MarkerFinalizer)' -count=1
   ```
5. **RED — lifecycle/no-persistence regression:** extend `internal/agent/live/runner_test.go` and `internal/agent/live/direct_test.go`. Script required-marker, embedded-marker remainder, sanitized blank, and over-cap content through both paths. Assert required-marker/blank yields no `runtime.Result`/`DirectCompletion`; ambient marker yields `ShouldReply=false`; a delivered result contains only bounded final text; existing post-delivery append receives that same bounded text only after successful sink/send. Assert typed command `SuppressReply` still bypasses finalization and persistence.
6. **GREEN — integration:** change only wiring necessary for the shared finalizer; rerun focused live/direct tests.
7. **RED — config/composition:** add default, runtime override, zero/negative/over-limit rejection, and a `cmd/zenbot` enabled direct/live shared-finalizer assertion. Verify disabled composition does not construct live/direct agent work.
8. **GREEN — config/composition:** finish config/example/main wiring. Re-run all focused tests.

## Rapid gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only files intentionally changed by this vertical.

```sh
gofmt -w \
  internal/agent/live/response_sanitizer.go internal/agent/live/response_sanitizer_test.go \
  internal/agent/live/runner.go internal/agent/live/runner_test.go internal/agent/live/direct_test.go \
  internal/config/agent_config.go internal/config/agent_config_participation_test.go \
  cmd/zenbot/main.go cmd/zenbot/live_agent_test.go

go test ./internal/agent/live -run 'Test(ResponseSanitizer|OutputFinalizer|MarkerFinalizer|Runner.*(Final|CommandProse)|Direct.*(Final|CommandProse))' -count=1
go test ./internal/config -run 'Test.*Agent.*(Output|Config)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/live ./internal/agent/runtime ./internal/config ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` is informational under the rapid policy. If it remains nonzero only for the recorded unrelated copylocks warning at `internal/core/engine_impl.go:95:22`, record it rather than changing that file. A race sweep is not newly required because this slice adds no shared mutable state; run a focused race test only if implementation accidentally introduces shared state (which is out of design).

## Explicit adaptations and exclusions

### Intentional adaptations

- Preserve Saturn's **deterministic post-correction** sanitizer/marker/bound behavior while deliberately omitting the source's corrective provider calls. Zenbot retains its accepted one-call/two-completion ceiling.
- Use Go rune count for Unicode code points and the target's `maxOutputChars` TOML naming; source default remains `8000`.
- Use target stable errors `agent returned an empty response` and `agent declined a required response`; do not expose source exception classes/stack traces.
- Apply the finalizer to target-supported reply-required direct/mention modes and ambient based on `Mode.RequiresReply`, rather than recreating Saturn's full request-kind/freshness/quote-only decision tree.

### Excluded

- `AgentResponseCorrector`, quote-only enforcement, failure-placeholder correction, internal-evidence correction, stale-response correction, provider fallback, retries, or any third completion.
- New or generalized tools; database/schema/query/SQL work; H2 migrations; tool evidence schema/storage changes; command catalog/reflection; command aliases/authorization/gateway changes; moderation or protected-principal behavior.
- Prompt resource/catalog changes, request classification changes, fresh-history changes, room context/history changes, durable-memory schema/TTL changes, listener ordering, relay topology, ambient scheduling, runtime admission, transport changes, or output capture rewrites.
- Changes to accepted command-prose recognition/correction/suppression other than regression tests proving it remains before finalization.
- Edits to `MIGRATION_PLAN.md`, `.hermes/migration-audit.md`, existing handoffs, Saturn source, or application Go code during this architecture task.

## Complexity and routing

**Complexity: low-to-moderate.** The sanitizer and output cap are pure/local; the material risk is user-visible exactly-once delivery and persistence sequencing if composition accidentally bypasses or doubles the finalizer.

| Work | Route |
|---|---|
| Pure sanitizer, rune truncation, finalizer unit tests | Standard agent/live developer with source-case review. |
| Config/default/validation and single direct/live composition | Standard developer; verify disabled branch remains early. |
| Required-vs-ambient marker behavior, command suppression regression, delivery-before-persistence proof | Senior live/runtime reviewer. |
| Independent QA | Replay focused direct/runtime/ambient/command-suppression tests; inspect no provider/tool/SQL/transport work was added; verify rune cap and persisted post-delivery text. |

## Artifact verification

- Material cited Saturn paths resolve under the read-only `saturn` checkout; target citations resolve under the Zenbot checkout. Proposed `internal/agent/live/response_sanitizer.go` and its test are explicitly new target paths, not claimed existing evidence.
- The only file created by this task is `.hermes/handoffs/rapid-agent-after-command-prose-architecture.md`.
- No Go source, protected migration documents, existing handoffs, or Saturn files were edited by this task.
