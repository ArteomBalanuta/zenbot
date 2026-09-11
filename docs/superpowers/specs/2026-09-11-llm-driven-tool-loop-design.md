# LLM-driven tool loop redesign

## Objective and authority

Implement the user's full SDK/tool-loop redesign request from the attached objective. The user authorizes architectural decisions and implementation without approval pauses. The LLM owns intent, iterative planning, tool selection, ordering, recovery and completion. Application code supplies contracts, execution, receipts, bounded state, and feedback. Unrelated security work is excluded.

## Findings in the starting implementation

- A separate completion judge adds a provider round trip to every answer, sees less evidence than the worker, and can discard useful partial answers. Phase transitions and recovery rules duplicate the natural model/tool loop.
- RecoveryPolicy disables every tool after ordinary missing-prerequisite, duplicate, not-found and command failures. The ledger permanently records failed arguments, preventing legitimate retries after state changes.
- Provider descriptions discard command descriptions and prerequisites in favor of an undocumented abbreviated metadata language. Root union schemas and strict-mode declarations do not consistently match provider requirements.
- Observation projection double-encodes JSON, loses effect receipts, and advertises an inaccessible continuation. Context pruning can erase early results while the model is instructed to analyze complete history.
- Historical message evidence uses an obsolete durable schema. Compatibility command-prose parsing and a second bounded loop preserve a different architecture from the production loop.

## Selected architecture

Use one standard chat-completion loop. The model emits ordinary final text or native tool calls. A tool-call response denotes a batch selected by the model; independent compatible reads may run concurrently, while effects run synchronously in source order. Data-dependent or conditional calls belong in a later response after observations arrive. No semantic planner, completion reducer, mandatory judge, command-prose parser, or deterministic fallback routing remains in the production path.

Every attempted call produces an observation, including validation, budget and unavailable-tool failures. Correctable failures keep other tools available. Known failed calls can be retried within explicit bounds; completed effects cannot be accidentally repeated. Unknown action outcomes remain non-retryable. A failed ordered action yields observations for skipped later actions so the model can revise the sequence. Whole-request cancellation still terminates execution.

The loop retains the exact request and canonical transcript. It supplies remaining budget and terminal-synthesis instructions explicitly, and reserves one tool-free answer after resource exhaustion. Empty, truncated or malformed protocol responses receive bounded structural feedback without pretending they succeeded. Ordinary final text completes the turn; the LLM decides whether work is done. Intentional silence uses the existing no-reply protocol with receipts where a direct request has already been delivered by a command.

## Contracts and information

Provider definitions use normal concise prose: purpose, discriminating constraints, delivery behavior, prerequisite names and important examples. Parameters stay flat where possible. Runtime validation enforces full application semantics; provider strict mode is used only for supported shapes. The existing command catalog remains the authoritative source for command names and argument encoders.

Observations expose structured JSON data, concise error code/message/recovery information, call identity, actual effect/delivery metadata and explicit truncation information. Full results remain request-local and become retrievable through a read-only paging tool. A compact current-turn result index survives transcript pruning and lets the model retrieve earlier evidence without rerunning effects. It summarizes execution facts only, never invents obligations or chooses work.

## Validation and completion criteria

- Single-argument kick, remote-room listing, weather and fixed-target ping retain accurate contracts.
- Tests exercise actual argument repair, prerequisite repair, not-found alternatives, transient retry, repeated effects, mixed batch outcomes, serial/parallel ordering and uncertain cancellation.
- Multi-round tests prove the original request and earlier results remain reachable after at least three dependent calls and forced context pruning.
- Provider serialization preserves malformed raw arguments for model correction and does not leak stale configured tools into tool-free synthesis.
- Ordinary conversation requires no extra judge call; compound requests can finish with one coherent final answer based on tool observations.
- Full Go tests, vet, formatting and relevant race checks pass. A provider-backed, side-effect-free evaluation checks representative tool selection and multi-step recovery when the configured endpoint is available.
- Architecture/inventory documentation and a requirement-to-evidence report match the implemented behavior and identify remaining empirical limitations honestly.

## Tradeoffs

Removing the mandatory judge saves latency and avoids competing model decisions, but completion quality must be evaluated empirically rather than claimed as deterministic proof. Execution limits bound loops without deciding intent. Stateful chat commands remain ordered because their shared transport and room delivery are not safe to parallelize. Tool result retrieval costs an additional call only when the model needs omitted evidence.
