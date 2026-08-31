# Rapid agent parity after final-output normalization: verified quote-only delivery

## Decision

**Select the public live quote-only delivery guard: classify an uncommanded, no-tool public TALK/UNCLASSIFIED turn, then deliver only an exact entry from the existing verified-quote catalog.**

This is the next bounded Saturn behavior after accepted deterministic final-output normalization. It closes an observable delivery gap at the same finalization boundary: Zenbot now sanitizes, handles the no-reply marker, and bounds output, but it sends ordinary prose and fabricated quotation/attribution unchanged. Saturn classifies the turn, uses actual tool-attempt evidence to exempt tool-backed answers, and requires quote-only verification for qualifying non-command live turns before delivery.

The source implementation can ask the provider to repair an invalid quote, including an optional structured-output attempt and textual fallback. **That generic provider-correction/retry machinery is intentionally not selected.** The target's accepted live loop is deliberately bounded to one tool call and at most two provider completions; a third completion or provider-specific unsupported-format fallback is not justified by this vertical. The bounded adaptation deterministically chooses the first catalog entry for an invalid eligible response instead of asking the provider again. Thus the invariant is enforced without widening call, timeout, cancellation, or protocol surface.

### Why this is the highest-value remaining bounded vertical

| Candidate | Decision | Evidence-based reason |
|---|---|---|
| **Verified quote-only delivery (selected)** | Small final-delivery and request-metadata vertical | Target already contains the exact resource files, provider-neutral `LlmRequest.ResponseFormat` seam, request classifier foundation, finalizer, direct/live post-delivery hooks, and a frozen loop. Missing behavior is limited to carrying trusted kind/evidence to finalization plus locally validating/selecting a catalog line. |
| Saturn generic response corrector (stale cache, placeholder, evidence leak, multi-format correction) | Reject | `AgentResponseCorrector` owns several unrelated provider corrections and cache bypasses (`src/main/java/org/saturn/app/agent/routing/AgentResponseCorrector.java`). It would change the accepted completion/cost semantics and violate the rapid no-generic-retries boundary. |
| Additional database/schema/SQL tools | Reject | Requires persistence/SQL policy/query scope, safety policy, dynamic result bounds, and tool inventory growth. Existing `internal/agent/sql/policy.go` is not an executable database-tool vertical. |
| General reflective command catalog or privileged moderation | Reject | Explicitly excluded; command side effects and protected-principal/authorization semantics are a separate security vertical. |
| Transport/listener/runtime rewrite | Reject | No source prerequisite exists for this behavior. Existing `Runner`/`DirectInvoker` delivery and post-delivery persistence are sufficient. |

## Evidence map: observed facts vs recommendations

### [OBSERVED] Saturn contract

1. `AgentRequestClassifier.classifyCandidate` returns `TALK` for social text or text containing a terminal sentence marker, but returns `UNCLASSIFIED` for blank/non-letter/control/protocol-looking/actionable text. `finalizeKind` changes any actual tool-attempt turn to `TOOL_CALL` (`src/main/java/org/saturn/app/agent/routing/AgentRequestClassifier.java`; `src/test/java/org/saturn/app/agent/routing/AgentRequestClassifierTest.java`).
2. In `AgentResponseFinalizer.prepare`, quote-only is required only when all conditions are true: `!invocation.commandOriginated()`, mode is not `MODERATION`, final kind is `TALK` or `UNCLASSIFIED`, and `toolEvidence.attempted()` is false. The finalizer runs response sanitation/empty/no-reply checks before quote handling, then sanitizes again and applies marker removal and output truncation (`src/main/java/org/saturn/app/agent/routing/AgentResponseFinalizer.java`).
3. `VerifiedQuoteCatalog` loads `/agent/verified-quotes.json`, rejects absent/empty/incomplete/duplicate/structurally invalid entries, finds an entry by exact Java-stripped line, and returns its first entry as a deterministic fallback (`src/main/java/org/saturn/app/agent/routing/VerifiedQuoteCatalog.java`). The source catalog has exactly three entries (`src/main/resources/agent/verified-quotes.json`).
4. The source quote syntax is one line only: `"<non-empty quote>" — <book>, <author>`; leading-dash legacy syntax and newline/CR content are invalid (`AgentResponseCorrector.isQuoteOnly`; `src/test/java/org/saturn/app/agent/routing/AgentResponseCorrectorTest.java:21-29`).
5. Source behavior never delivers an unverified quotation. A verified exact line is canonicalized to its catalog line; a fabricated quote or wrong attribution receives corrective handling and ultimately the deterministic fallback if structurally quote-like; non-quote prose after correction fails (`AgentResponseCorrector.correctQuoteOnly`; `AgentResponseFinalizerTest.java:50-67`; `AgentResponseCorrectorTest.java:32-75`).
6. Source route order is material: assembly creates candidate kind, the router records tool evidence from actual execution, derives final kind, then finalization uses those trusted values before appending memory (`src/main/java/org/saturn/app/agent/routing/DefaultAgentRouter.java:163-318`).

### [OBSERVED] Zenbot state

1. The accepted normalization finalizer only takes `(runtime.Invocation, raw string)`. It sanitizes, applies marker semantics, ASCII control trimming, and rune bounding, but receives neither request kind nor actual tool-attempt evidence (`internal/agent/live/runner.go:14-60`). Therefore it cannot decide quote-only eligibility and currently delivers ordinary no-tool public prose.
2. `participation.Classifier` already mirrors the candidate and final kind primitives. It recognizes `TALK`/`UNCLASSIFIED`; `Finalize` changes actual attempted evidence to `TOOL_CALL`. `internal/agent/routing/routing.go` merely aliases that foundation (`internal/agent/participation/policies.go:122-177`; `internal/agent/routing/routing.go:10-18`).
3. `assemble.PreparedRequest` retains a candidate `RequestKind`, but live callers currently always assemble with `assemble.Talk`; the regular bounded loop does not expose trusted attempted-tool metadata in `Completion` (`internal/agent/assemble/assemble.go:239-257,279-318`; `internal/agent/live/tool_loop.go:35-53,79-189`).
4. Public `MENTION`, `AMBIENT`, and relay-backed invocations construct a runtime invocation with `commandOriginated=false`; direct `l` explicitly constructs `DIRECT` with `commandOriginated=true` (`internal/agent/live/direct.go:41`; runtime invocation construction and listener callers under `internal/agent/live/` and `internal/agent/participation/`). The direct path is therefore source-exempt from quote-only enforcement.
5. Zenbot already has exact copies of the source catalog and quote-only correction prompt under `resources/agent/verified-quotes.json` and `resources/agent/correction/router-quote-only-correction.txt`; no resource migration is a prerequisite. Their three source entries match the Saturn resource on inspection.
6. `llm.LlmRequest` can carry a provider-neutral response format, and the OpenAI-compatible adapter serializes it as `response_format`; this proves a future correction vertical has a seam, but this slice does not use it (`internal/agent/llm/client.go:94-110`; `internal/agent/llm/openai/client.go:188-217`).
7. `Runner.AfterDelivery` and `DirectInvoker.PersistDelivery` append conversation/evidence only after visible delivery. A silent/error result reaches neither callback (`internal/agent/live/runner.go:136-149`; `internal/agent/live/direct.go:102-125`).

### [RECOMMENDED] bounded target contract

Implement a local `QuoteOnlyFinalizer`/policy that consumes **only trusted final-turn metadata** and the already normalized final text:

```go
type FinalizationContext struct {
    CandidateKind participation.RequestKind
    ToolAttempted bool
}

func (f OutputFinalizer) FinalizeWithContext(
    inv runtime.Invocation,
    raw string,
    meta FinalizationContext,
) (text string, shouldReply bool, err error)
```

`Finalize` remains a compatibility wrapper with a safe source-conservative default for existing unit callers; production `Runner` and `DirectInvoker` must use `FinalizeWithContext`.

Eligibility is exact:

```text
quoteOnly := !inv.CommandOriginated() &&
             inv.Mode() != runtime.MODERATION &&
             (meta.CandidateKind == participation.Talk ||
              meta.CandidateKind == participation.Unclassified) &&
             !meta.ToolAttempted
```

The candidate kind must be computed once from `inv.Prompt()` at assembly/loop entry. The final kind must be derived from the actual executed turn, never model text: an attempted tool call (including a failed read) makes `ToolAttempted=true`, so the no-tool quote policy cannot erase an error explanation or tool-backed answer.

For a quote-only eligible response, sanitize first using the accepted `responseSanitizer`. Preserve existing empty/no-reply semantics. Then:

1. Java-strip the result using the accepted `stripJavaWhitespace` behavior; do not use Go `strings.TrimSpace` because the accepted normalizer intentionally preserves NBSP parity.
2. If it exactly matches a validated catalog line, canonicalize to that line before the existing marker-removal/rune-cap path.
3. If it does not exactly match, return the first validated catalog line. The original ordinary prose, fabricated quote, wrong attribution, malformed quote, raw JSON, and provider diagnostic text are never delivered.
4. The fallback is a normal visible response for qualifying `MENTION`/`AMBIENT` public work. It is not a no-reply marker, error, retry, or tool result.

This is intentionally stricter/deterministic than Saturn's provider correction path while preserving the user-visible safety invariant. It does **not** call the provider, send response-format JSON, bypass a prompt cache, construct correction messages, or add a correction budget.

## Exact source behavior and target adaptation

### Source behavior retained

| Source behavior | Target implementation rule |
|---|---|
| `commandOriginated` suppresses quote-only enforcement | Preserve: `DirectInvoker` creates `commandOriginated=true`; no direct `l` change. Do not infer command origin from prompt text. |
| `MODERATION` is excluded/silent | Preserve existing moderation finalization/suppression behavior; do not select moderation tooling. |
| Candidate `TALK`/`UNCLASSIFIED` is eligible; actual attempted tool evidence wins | Carry `CandidateKind` and `ToolAttempted` from trusted loop state to finalizer. No model-provided metadata is accepted. |
| Exact catalog line is canonical | Return the exact catalog line, including source punctuation and `—`, not a whitespace-variant provider string. |
| Fabricated/incorrect quote is never delivered | Return catalog fallback locally rather than provider-correcting. |
| Quote grammar disallows CR/LF/leading-dash legacy form | Validation rejects those catalog entries; runtime matching remains exact catalog membership, not regex-only acceptance. |
| Sanitize then validate/correct then sanitize/output-bound | Reuse `OutputFinalizer` sanitation/marker/bounding sequence; quote selection occurs after the initial sanitation/empty/no-reply decision and before final marker removal/bounding. |

### Intentional adaptations and exclusions

- **No provider correction:** Saturn makes a structured correction request and may perform a textual fallback if structured output is unsupported. Zenbot does neither. It uses the same source catalog's first line to produce deterministic safe output. This avoids a third provider completion on a tool-assisted turn and avoids provider-specific unsupported-response-format detection.
- **No generic stale-response, placeholder, internal-evidence, or unverified-action correction:** these remain deferred as distinct `AgentResponseCorrector` behavior. Do not add retries, cache bypass, `response_format`, or correction templates to production use.
- **No catalog edit:** `resources/agent/verified-quotes.json` already matches source and is read-only for this slice. Do not add a quote, alter attribution, fetch its URLs, or make catalog configuration dynamic.
- **No broad routing migration:** no general Saturn router, multi-tool/multi-step execution, session-lock change, provider rewrite, listener ordering change, gateway change, moderation behavior, database/schema/H2 work, memory schema work, or transport change.
- **No policy application to direct command `l`:** source command-origin distinction is retained exactly. Do not classify direct `l` requests just to change their output contract.

## Target file and interface plan

### Stage A — pure catalog and quote-only policy

**Files**

- Add `internal/agent/live/verified_quote_catalog.go`
- Add `internal/agent/live/verified_quote_catalog_test.go`
- Modify `internal/agent/live/runner.go`
- Extend `internal/agent/live/runner_test.go`

**Owner and interfaces**

Keep the catalog private to `live`; it is a response-delivery policy, not an agent tool or command registry.

```go
type verifiedQuoteCatalog struct { entries []verifiedQuote }
type verifiedQuote struct { ID, Quote, Book, Author, Reference string }

func loadVerifiedQuoteCatalog(fsys fs.FS) (verifiedQuoteCatalog, error)
func (c verifiedQuoteCatalog) find(line string) (string, bool)
func (c verifiedQuoteCatalog) fallback() string
```

Load `resources/agent/verified-quotes.json` through the existing embedded/resource loading convention used by `internal/agent/prompt`; do not hard-code three strings in Go. Validate at construction/startup: nonempty entries; nonblank `id`, `quote`, `book`, `author`, and `reference`; unique IDs and rendered lines; and source quote grammar. Fail configuration/composition closed if the catalog is invalid. The parser must reject unknown JSON fields only if existing resource-loader conventions already do so; do not introduce a new global JSON strictness rule.

Add a private policy helper:

```go
func quoteOnlyRequired(inv runtime.Invocation, meta FinalizationContext) bool
func (c verifiedQuoteCatalog) selectVerifiedOrFallback(content string) string
```

Exact lookup is by `stripJavaWhitespace(content)` and rendered source line. Do not recognize a merely regex-shaped fabricated quote as valid.

### Stage B — carry trusted kind/evidence through the frozen completion path

**Files**

- Modify `internal/agent/live/tool_loop.go`
- Extend `internal/agent/live/tool_loop_test.go`
- Modify `internal/agent/live/runner.go`
- Modify `internal/agent/live/direct.go`
- Extend `internal/agent/live/direct_test.go`

Add immutable metadata to `Completion`, for example:

```go
type Completion struct {
    Response        llm.LlmResponse
    DurableEvidence []turn.PersistableEvidence
    SuppressReply   bool
    CandidateKind   participation.RequestKind
    ToolAttempted   bool
}
```

At `CompleteWithEvidenceAndHistorical` entry, calculate `CandidateKind` once with the existing `participation.Classifier{}.Classify(inv.Prompt())`. Use `state.AttemptedToolCount() > 0` (or one small accessor on `turn.State`) after the loop's outcome is known. It must represent execution attempt, not success, a provider tool-call array, selected definition count, or persisted evidence count. On all successful normal responses, expose it in `Completion`; suppress-command replies return before finalization and may leave metadata unused.

For the no-`ToolLoop` compatibility path, compute only candidate kind and set `ToolAttempted=false`. Its existing fresh-history fail-closed guard remains unchanged. This keeps test-only/minimal composition deterministic without claiming a tool was attempted.

Do not change the closed public tool inventory, `ToolLoopLimits`, one-call ledger, exact synthetic fresh-history protocol, `run_command` suppression behavior, direct/runtime construction, or `Completion` evidence lifecycle.

### Stage C — finalizer integration and lifecycle proof

**Files**

- Modify `internal/agent/live/runner.go`
- Modify `internal/agent/live/direct.go`
- Extend `internal/agent/live/runner_test.go`
- Extend `internal/agent/live/direct_test.go`
- Extend `cmd/zenbot/live_agent_test.go` only if composition cannot be established in package tests

Production finalization receives the `Completion` metadata on both live and direct paths. For the direct path, the meta remains source-exempt because the invocation has `CommandOriginated()==true`; add the test explicitly so a future refactor cannot unintentionally enforce quote-only output for `l`.

The finalizer must not create a new result or delivery path. A selected fallback continues through the existing `runtime.Result`/`runtime.DirectCompletion`, sink/send, then `AfterDelivery`/`PersistDelivery` lifecycle. It does not create durable tool evidence and does not change conversation-memory key/visibility rules.

## Authorization, visibility, cancellation, timeout, delivery, persistence, concurrency, and protocol effects

| Concern | Required behavior |
|---|---|
| Authorization | No authorization/capability checks, command execution, catalog mutation, or privileged operation is added. Catalog selection is local immutable data. |
| Visibility | Apply only to public non-command live `MENTION`/`AMBIENT` results eligible by the exact predicate. A whisper may be candidate-classified but must not acquire a public catalog/data read; quote finalization itself does not inspect room data. Preserve existing whisper context/evidence suppression. |
| Cancellation | Check `ctx.Err()` at existing loop/client boundaries. Catalog parse/select is synchronous, CPU-local, and must not spawn a goroutine. If context is canceled before finalization, existing runner/direct cancellation behavior wins; do not convert cancellation into fallback output. |
| Timeout | Add no timer, deadline, provider call, HTTP request, retry, or backoff. Existing provider/tool deadlines are unchanged. |
| Delivery | Valid exact catalog line or deterministic fallback is one normal final response. Raw provider invalid prose/quote is never sent. Existing marker silence remains silent; required-response marker error remains an error. |
| Persistence | Existing post-visible-delivery sequencing remains authoritative. A visible fallback may be appended as ordinary conversation memory only after sink/send success, exactly like any visible no-tool answer. It never creates tool evidence. Error, silence, command suppression, send failure, or cancellation appends neither exchange nor evidence. |
| Concurrency | Catalog is loaded once at composition and retained as immutable value/slice; `find`/`fallback` do not mutate state. No locks, queues, workers, session-order changes, or cross-invocation state are introduced. |
| Provider protocol | No extra LLM request; no tools/response-format/cache-bypass fields; no assistant/tool message is appended. Existing one-call/two-completion ceiling and exact assistant/tool ID pairing are unchanged. |
| Output normalization | Reuse accepted Java-whitespace sanitation and rune cap. The catalog line is selected before the existing final marker-removal/rune-cap phase, so a future configuration with a very small bound remains bounded rather than silently bypassing output policy. |

## End-to-end sequence

```text
public mention or ambient invocation (commandOriginated=false)
  -> ToolLoop derives CandidateKind from trusted inv.Prompt()
  -> completion #1
     -> no tool: ToolAttempted=false
     -> one permitted read/action attempt: ToolAttempted=true
     -> existing bounded optional completion #2 as applicable
  -> Runner.FinalizeWithContext(inv, response.Content(), meta)
     -> existing sanitizer / empty / exact no-reply handling
     -> quote-only eligible?
          no: existing normalized output
          yes + exact catalog line: canonical catalog line
          yes + any other text: deterministic catalog fallback
     -> existing marker removal and rune bound
  -> existing runtime sink or direct command send
  -> only after successful visible send: existing memory persistence
```

For `DIRECT l`, `commandOriginated=true`, so the same code path reaches `quoteOnlyRequired == false`; accepted direct behavior remains unchanged. For a normal tool attempt, `ToolAttempted=true` even when the tool returns an error and the current bounded loop allows a synthesis; its explanatory prose remains eligible for ordinary output rather than being replaced by a literary quote.

## RED → GREEN test plan

Implement in this order; every RED test must fail before the smallest implementation makes it green.

1. **Catalog RED — `internal/agent/live/verified_quote_catalog_test.go`:** load the embedded target resource and assert its three expected rendered lines; exact Java-stripped lookup canonicalizes surrounding Java whitespace; fallback is the first entry. Table-test absent/empty list, blank required fields, duplicate IDs, duplicate rendered lines, malformed JSON, malformed quote syntax, CR/LF, and leading-dash legacy syntax. Do not use the network references.
2. **Eligibility RED — `internal/agent/live/runner_test.go`:** table-test all predicate axes: non-command `MENTION` TALK and UNCLASSIFIED with no attempt are eligible; `DIRECT` command-originated, `MODERATION`, tool-attempted TALK, and non-eligible kind are not. Include NBSP boundary preservation to retain the accepted source-shaped whitespace behavior.
3. **Finalization RED — same package:** exact catalog content returns its canonical line; whitespace variant returns exact line; fabricated syntactically quote-shaped content, wrong attribution, ordinary prose, multiline content, JSON, and marker-like noncatalog prose each return deterministic fallback for eligible invocations. Assert no provider client is touched. Preserve empty/marker behavior before quote selection: ambient exact marker remains silent; required marker errors; sanitized empty errors.
4. **Metadata RED — `internal/agent/live/tool_loop_test.go`:** a no-tool public response carries `CandidateKind` and `ToolAttempted=false`; `room_users`, `user_message_history`, and `run_command` successful calls carry `ToolAttempted=true`; a read-tool error that still reaches existing synthesis carries true. Assert the frozen three definitions, one total tool call, no extra completion, and unchanged assistant/tool pairing.
5. **Fresh/command regression RED — same package:** required trusted history and rendered-command correction retain their current protocol and completion bounds; `SuppressReply` returns before finalizer policy. Do not retrofit quote logic into the private command-correction protocol.
6. **Runner/direct lifecycle RED — `runner_test.go`, `direct_test.go`:** non-command mention invalid prose yields fallback and normal visible result; direct `l` ordinary prose is unchanged; attempted-tool final prose is unchanged; marker/error/suppression/cancellation produce no evidence. Prove fallback conversation append occurs only through existing post-delivery callbacks, and a simulated sink/send failure persists nothing.
7. **Composition RED — `cmd/zenbot/live_agent_test.go`:** enabled production composition constructs a catalog-aware finalizer for both live/direct consumers; direct remains command-exempt; disabled setup creates no provider/catalog-dependent agent work. Keep construction local with no endpoint call.

## Focused and full rapid gates

Run from `/Users/ab/workspace/go-projects/zenbot`; format only slice-owned Go files.

```sh
gofmt -w \
  internal/agent/live/verified_quote_catalog.go \
  internal/agent/live/verified_quote_catalog_test.go \
  internal/agent/live/tool_loop.go \
  internal/agent/live/tool_loop_test.go \
  internal/agent/live/runner.go \
  internal/agent/live/runner_test.go \
  internal/agent/live/direct.go \
  internal/agent/live/direct_test.go \
  cmd/zenbot/live_agent_test.go

go test ./internal/agent/live -run 'Test(VerifiedQuote|QuoteOnly|OutputFinalizer|ToolLoop.*(Quote|Command|History)|Runner.*Quote|Direct.*Quote)' -count=1
go test ./internal/agent/participation -run 'TestClassifier' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent|Quote)' -count=1
go test ./internal/agent/live ./internal/agent/participation ./internal/agent/runtime ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

Per the rapid migration policy in `MIGRATION_PLAN.md`, focused capability tests, relevant package tests, full `go test ./...`, build, and diff hygiene are acceptance gates. A targeted `go test -race ./internal/agent/live -count=1` is useful confirmation of immutable catalog composition but is informational unless the slice introduces mutable state (it must not). Observe `go vet ./...` separately; do not misattribute the known unrelated copylocks warning in `internal/core/engine_impl.go` to this slice.

## Complexity routing and review

**Complexity: low-to-moderate.** The catalog/finalizer code is small and deterministic. The review-sensitive portion is preserving trusted metadata through the shared bounded loop without weakening the one-call/two-completion contract or changing direct `l` semantics.

- **Developer:** Stage A catalog/validator and Stage C finalizer integration/tests.
- **Senior agent/runtime reviewer:** Stage B metadata provenance, required-history path, command suppression ordering, cancellation, and no-extra-provider-call proof.
- **Independent QA:** verify source/target catalog byte-equivalent logical entries, quote eligibility matrix, tool-attempt exemption on successful and failed attempted calls, direct command exemption, post-delivery persistence, focused/full gates, and that no provider/transport/SQL/moderation files changed.

## Explicit exclusions

- No writes to Saturn source, protected migration plan/audit, existing handoffs, application code, commit, or push in this architecture task.
- No generic `AgentResponseCorrector` port: stale cache retry, failure-placeholder correction, internal-evidence correction, quote structured-output correction, textual correction fallback, response-format use, cache bypass, or third provider completion.
- No tool inventory/capability/authorization expansion; no reflective catalog; no moderation/command/gateway changes; no database query/schema/SQL work; no new persistence tables/queries; no tool-memory semantics change.
- No listener/relay/transport/runtime admission rewrite, retry queue, background work, session locks, config property, resource edits, or broad normalization change.

## Architecture completion checklist

- [ ] All cited source/target paths resolve in the current checkouts.
- [ ] Exactly one vertical is implemented: locally verified quote-only delivery for eligible public live no-tool turns.
- [ ] Source behavior and intentional deterministic no-provider-correction adaptation are separately labeled.
- [ ] Candidate kind and actual attempt evidence have one trusted owner and cannot be model-controlled.
- [ ] Direct command, moderation, tool-attempt, marker, cancellation, delivery, persistence, and protocol boundaries have focused tests.
- [ ] The frozen three-tool/one-call/two-completion contract is unchanged.
- [ ] Only slice-owned files are edited; `git diff --check` passes.
