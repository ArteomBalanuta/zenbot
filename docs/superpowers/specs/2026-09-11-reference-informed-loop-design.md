# Reference-informed LLM loop refinements

## Scope and authority

Baseline: Zenbot `20bf0dd1c6fb27f85423f980fc1b3740ed741793`, clean `master`.
The user authorizes autonomous design and implementation. Keep the existing checkout;
do not deploy, restart the bot, push, or change the reference repositories.
The LLM owns tool selection, task planning, conditions, recovery and completion.
No streaming, deterministic intent routing, semantic completion judge, fixed workflows,
keyword-based decisions, or unrelated security work.

All requested references were examined before selecting these changes:

| Source | Requested revision | Evidence caveat |
| --- | --- | --- |
| ace-kernel main | bf25f426ddf85bc161fd4f5295a418b4fca08716 | Working checkout is a newer feature branch; read main through Git. |
| ace-first-pilot-dsl main | bfeb44d714f89848986f7b4ff48fb96b72ff18ca | App DSL delegates its runtime to Kernel; unmerged history is not main. |
| saturn develop | 10a1ea3c2daecba168deac36f0e9a52a74cb19e5 | Dirty weather implementation excluded. |
| pastor Markdown | Current five files | Descriptive guides and a comparison, not runtime authority; Kernel guides describe 1d2cab89, not main. |

The five recursively discovered Markdown sources are AGENT_IMPLEMENTATION_COMPARISON.md,
KERNEL_AGENT.md, KERNEL_AGENT_CLEAN.md, SATURN_AGENT.md and SATURN_AGENT_CLEAN.md.
Relevant contracts, loop, context, state, failure, history and finalization sections were
read; streaming-specific mechanisms and unrelated room automation are excluded.

## Baseline strengths

One native model/tool feedback loop preserves call/result correlation and the original
request, executes actions synchronously, allows compatible reads to overlap, records
partial/unknown effects, blocks uncertain action replay, and provides full-result paging.
The model may answer directly, revise its approach, or finish after any observation.
Existing flat concrete command schemas and the shared production command gateway stay.

## Options considered

1. Import a reference loop or a second execution language. Rejected: Kernel main can
   discard prose-plus-call continuations; TCL reparses/replays entire programs; Saturn
   injects keyword-selected history calls and correction policies. Complexity and loss
   of model agency exceed the demonstrated benefit.
2. Leave the loop unchanged and lengthen instructions. Rejected: prompts cannot recover
   typed data discarded by adapters or feedback evicted by context projection.
3. Improve factual contracts and projection, retaining the loop. Selected: source-owned
   structured data, schema-derived return hints, protected current correction, bounded
   result previews, and an explicit failure receipt report.

## Changes

### 1. Preserve typed remote-list data at its source

ListRoomOperation already owns the snapshot and formatting. Return optional structured
data alongside the existing room Reply, containing room, users, count, returnedCount and
truncated. Users are deterministic, deduplicated identities using the same semantics as
the displayed roster; count is computed before any display limit. Empty rooms are valid
count-zero results. Never parse the chat string to reconstruct facts.

Carry optional JSON data through OperationResult, agentCaptureEngine, commandgateway.Execution,
and the Saturn command success payload. The existing command remains the execution path,
including remote-session completion, delivery and action receipts. Other commands remain
compatible without domain data. Result schemas declare the optional field and the list
shape for list only; other concrete commands must not advertise unrelated roster fields.
The legacy generic command adapter accepts optional generic data. Do not claim a
failed/unknown snapshot is successful because it produced data.

### 2. Generate compact return-shape guidance from the SDK schema

ProviderDefinition should expose a bounded, deterministic summary of ResultSchema.
Use existing schema fields/types, optionality and shallow nesting; no duplicate authored
schema or guessed domain semantics. Keep strings valid UTF-8 and summaries explicitly
partial when bounded. Do not advertise result fields as callable arguments. This borrows
the useful native Kernel pattern without its fallback schemas or executor heuristics.
The measured creator manifest grows from 32,513 to 39,793 bytes (+22.39%); retain a
40 KiB regression ceiling and count the complete invocation manifest in context budgets.
This is an intentional information/context tradeoff, not a token optimization.

### 3. Preserve useful feedback and facts under context pressure

Current TOOL_LOOP_FEEDBACK must be passed explicitly as required runtime context, just
like TOOL_LOOP_STATUS. A rejected batch is not execution evidence. Its diagnostic must
remain bounded, report whether previews are truncated, and not drown the original request.
Do not identify trusted runtime messages merely by a text prefix.

The required factual result index must retain bounded structured result previews and
bounded error messages as well as identities/receipts. Currently the optional transcript
can lose all result values while retaining only success metadata; in terminal synthesis
the paging tool is unavailable. This is a factual-information gap, not a need for a judge.
Use the existing observation projector to bound data, preserving explicit truncation.
Reduce previews before dropping any receipt. Full results remain in the request-local
store; unavailable details must never be represented as complete. Avoid turning this
index into task obligations, inferred progress, or a model-independent plan.

### 4. Report evidence when final answer generation fails

IncompleteTurnError already retains observations, but production failure delivery always
says only that the agent could not answer. Provide a bounded factual failure report from
that error's receipts, clearly labelled as an incomplete answer. Distinguish successful
tool execution, known failure/not-started, partial effects and unknown effects. Never
claim the user task is complete; never suggest replaying a whole action sequence; never
show raw stack traces, argument bodies or result payloads. Normal completion remains LLM
prose. This is an operational failure notice, not a deterministic finalizer or answer.
Keep interruption persistence and cancellation behavior intact.

## Validation

Regression tests must fail on baseline behavior before production edits. Cover typed
roster success/empty/dedup/source-room identity and end-to-end gateway retention; optional
data compatibility; schema hint bounds/order/optionality; current correction survival
under competing context; 3+ calls with pruned result values; terminal data availability;
unknown/partial/failure receipt reporting and bounded no-evidence fallback. Existing
tests for action replay, source ordering, malformed calls and original intent must pass.

Run full Go tests, race tests for affected agent/command/snapshot boundaries, and go vet.
Use the existing opt-in configured-provider evaluation with synthetic tools only; record
full traces, unchanged settings and failures. Such trials measure behavior, not proof of
semantic accuracy for arbitrary requests. Do not contact real rooms or execute moderation.

## Limits and deliberately deferred work

Model reasoning remains fallible. Preserving the exact request after several calls proves
context retention, not semantic fulfillment. No unconditional reliability claim is valid.
Paging/context estimation remain bounded/approximate. Unknown action outcomes remain
non-retryable during the request. Durable resumable workflows, cross-turn idempotency,
an execution language, dynamic registry caches, a generic calculation tool and a second
provider are not justified by the compared sources. Revisit only with a measured need.
