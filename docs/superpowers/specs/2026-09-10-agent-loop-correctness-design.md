# Agent Loop Correctness Design

## Objective

Correct six production defects in Zenbot's LLM tool harness, in this exact implementation order:

1. Prevent timed-out actions from continuing as detached zombie work.
2. Stop promoting transport or legacy-command success into unverified business success.
3. Preserve the user's semantic objective through multi-tool turns.
4. Remove competing tools that encode the same intent.
5. Make advertised argument schemas express every enforced conditional requirement.
6. Apply context budgeting to tool observations on every model cycle.

The order is a dependency order. Later work may consume contracts introduced by earlier work, but it must not be implemented ahead of the earlier item.

## Scope And Compatibility

This change is limited to the generalized registry-based agent loop and the command/service defects required to make its outcomes truthful. The public Saturn command syntax, caller capability model, OpenAI-compatible transport, shared-room memory behavior, and private whisper isolation remain compatible.

The legacy `command.Command.Execute(context.Context) (model.Status, error)` interface remains in place for non-agent callers. The agent-specific gateway becomes stricter and returns a typed outcome. Legacy commands that currently swallow a known business failure are corrected at their source so the gateway does not need command-name-specific guesses.

## Cross-Cutting Invariants

- An action never continues in an executor-owned detached goroutine after `Executor.Execute` returns.
- Cancellation of an action without a verified terminal result produces `ACTION_OUTCOME_UNKNOWN`.
- `ACTION_OUTCOME_UNKNOWN` is never automatically retried.
- A successful action result requires a committed-effect signal; `ROOM_DELIVERY` additionally requires a positive delivery receipt.
- The harness never invents a success message or delivery receipt.
- Every turn owns an immutable task contract derived from the trusted newest request.
- Completion requires all mandatory task obligations to be satisfied by typed current-turn evidence.
- Each primary intent has one model-facing authoritative tool.
- Every provider-advertised argument object is accepted or rejected by the same validation contract used at execution.
- Every provider request, including follow-ups and correction calls, is projected through one context budget.

## 1. Synchronous Action Execution And Unknown Outcomes

### Execution Model

`execution.Executor` resolves and validates the descriptor before invocation, then chooses an execution strategy from the descriptor effect:

- `READ_ONLY`: retain bounded preemptive waiting. A late read result is ignored and cannot mutate application state.
- `ACTION`: invoke the tool synchronously on the executor goroutine. The executor does not spawn a goroutine that it can abandon.

All shipped action tools already accept `context.Context`; action adapters must pass it through every repository, controller, snapshot, and delivery operation. Tests use deliberately blocking tools to prove that an action cannot outlive the `Execute` call as executor-owned background work.

If an action returns a verified success, that result wins even when the deadline became ready concurrently. If it returns without a verified terminal result after cancellation or deadline expiry, the executor emits:

```json
{
  "ok": false,
  "error": {
    "code": "ACTION_OUTCOME_UNKNOWN",
    "message": "action outcome is unknown after cancellation",
    "retryable": false
  }
}
```

The result means the controller cannot prove whether the mutation committed. It is not equivalent to `TOOL_TIMEOUT` or definite failure.

### Recovery

`turn.RecoveryPolicy` receives tool effect, idempotency, and retryability. It must never return `RETRY_MODEL` for `ACTION_OUTCOME_UNKNOWN`. The turn proceeds to terminal synthesis with a concise uncertainty statement and no action tools available. Exact or modified retries are both prohibited for that turn.

### Accepted Trade-Off

As approved, a non-cooperative action can delay a cancelled request because the harness waits for it instead of detaching it. This is preferable to returning while an untracked mutation continues. Runtime request deadlines remain signals that action implementations must honor; they are not claims that Go can forcibly terminate arbitrary in-process code.

## 2. Verified Business Outcomes

### Agent Gateway Contract

Replace the agent-facing `Execution{Executed bool, Messages []string}` contract with:

```go
type OutcomeStatus string

const (
    OutcomeSucceeded OutcomeStatus = "SUCCEEDED"
    OutcomeRejected  OutcomeStatus = "REJECTED"
    OutcomeNotFound  OutcomeStatus = "NOT_FOUND"
    OutcomeUnknown   OutcomeStatus = "UNKNOWN"
)

type DeliveryReceipt struct {
    Count int
}

type Execution struct {
    Status           OutcomeStatus
    EffectsCommitted bool
    Messages         []string
    Delivery         *DeliveryReceipt
}
```

`EffectsCommitted` represents a verified terminal command outcome, not merely that dispatch began. Successful calls to `SendChatMessage`, `SendWhisperMessage`, and `SendAddressedMessage` produce the delivery receipt. The gateway returns an error or a non-success status when the legacy command reports failure, context cancellation prevents a terminal result, or an expected room delivery has count zero.

### Source Corrections

The following known false-success producers are corrected directly:

- Restart and shutdown propagate lifecycle-controller errors instead of logging and returning `SUCCESSFUL`.
- Mail persistence propagates `INSERT` failures and does not acknowledge scheduling without a committed row.
- Multi-trip access grants the parsed requested role to every trip, stops on the first persistence failure, and never announces a role different from the stored role.
- Last-online lookup distinguishes a missing record from a populated record; missing data becomes `NOT_FOUND` with an explicit user-facing result instead of placeholder success.

### Tool Result And Suppression

`contract.Result` gains explicit outcome metadata used by the loop and completion reducer. Successful `SaturnCommand` results are constructed only from `OutcomeSucceeded`, `EffectsCommitted == true`, and—because its result mode is `ROOM_DELIVERY`—a non-nil receipt with `Count > 0`.

`suppressRegistryReply` requires the same verified delivery condition. `Runner` removes the fallback text `Completed the requested Saturn action.` If a supposedly delivery-owned result lacks a receipt, the turn is treated as an internal failure and is not persisted as completed.

## 3. Semantic Task Contract

### Immutable Objective

At turn intake, construct a request-local task contract from trusted request metadata and the exact newest prompt:

```go
type TaskContract struct {
    RequestID   string
    RequestHash string
    Objective   string
    Constraints []Constraint
    Obligations []Obligation
}

type Obligation struct {
    ID       string
    Kind     ObligationKind
    Subject  string
    Required bool
}
```

Before ordinary tool selection, a constrained planning pass proposes constraints and atomic obligations using only intent classes present in the caller-filtered manifest. The controller supplies `Objective` from the exact newest prompt, assigns stable obligation IDs, rejects unknown intent classes and dependency cycles, and freezes the result. A direct-answer request has one answer obligation; a compound executable request has one obligation per required intent. A malformed plan receives one bounded correction and then fails closed without executing an action.

Later tool calls may bind evidence requirements to an existing obligation, but they cannot create a substitute objective, remove required obligations, or mark obligations complete. The model may propose calls and prose; only the controller owns task-state transitions.

### Deterministic Reducer

`turn.TaskState` consumes call-bound typed outcomes. It records evidence by obligation and marks an obligation satisfied only when its declared intent, subject, dependency, and evidence rule match a successful result. Errors and unknown action outcomes never satisfy obligations. Ordered dependencies prevent a later successful call from erasing or bypassing an earlier required step.

Each follow-up receives a compact controller message containing the immutable objective, remaining obligations, satisfied evidence IDs, and prohibited retries. The message is generated from state, not from model prose.

### Completion Gate

Deterministic task-state validation runs before the semantic model gate. A candidate cannot be final while a required obligation is pending, an action receipt is absent, or an action outcome is unknown. The existing semantic gate remains a prose-quality and entailment check; it is not authoritative for execution completion.

The gate receives currently executable tools after ledger/budget restrictions, not the original stale manifest. Tests exercise at least three heterogeneous tool rounds with misleading observation text and prove that the original objective and outstanding obligations remain present.

## 4. One Authoritative Tool Per Intent

### Presence

Expose one model-facing `room_users` operation for current room membership. It first uses the managed-room directory; if no snapshot exists, it invokes the existing command gateway's temporary-room snapshot path internally. `saturn_list` remains a human command but is removed from the provider manifest.

### History And Identity

- `user_message_history` is the sole model-facing named-user public-message history operation. Remove overlapping `database_query.recent_messages_for_user` from the model-facing enum.
- `saturn_messages` remains the moderator-only trip-based command because its subject and authorization differ from nickname history.
- `saturn_nicks` is the sole model-facing trip-to-nickname operation. Remove `database_query.known_nicks_for_trip` from the model-facing enum.

Repository methods may remain shared internally; the requirement concerns the model-facing routing surface.

### Catalog Validation

Descriptors gain a stable `PrimaryIntent` value. Catalog construction rejects two exposed tools with the same primary intent unless one is explicitly marked as an internal fallback and is omitted from provider definitions. This makes future overlap a startup/test failure instead of a prompt-quality problem.

## 5. Truthful Conditional Schemas

The supported schema dialect is extended with `oneOf` and `const`, including recursive validation and exactly-one-branch enforcement. Provider definitions enable strict mode when supported by the OpenAI-compatible function format.

Conditional contracts become discriminated unions:

- `automove`: `enable`, `disable`, and `configure` branches; configure requires source and destination, while the other branches forbid them through `additionalProperties:false`.
- `kick`: exact and contains require exactly one target; multiple requires at least one target.
- Remaining named database queries are separate branches requiring only the fields valid for that query and rejecting irrelevant fields.

Descriptor construction validates every example against the advertised schema. `Arguments.Encode` remains defense in depth and must agree with schema acceptance. A catalog-wide parity test enumerates valid and invalid cases and fails when schema and encoder disagree.

## 6. Per-Cycle Observation Budgeting

### Observation Projection

Full tool results remain available request-locally in an `ObservationStore` keyed by call ID. The model transcript receives a bounded projection:

```go
type ObservationView struct {
    CallID         string
    Tool           string
    Status         string
    Summary        string
    ReturnedCount  int
    Truncated      bool
    ContinuationID string
}
```

Each descriptor declares a maximum model-visible result size. Results exceeding it are summarized deterministically by type: arrays retain counts and a bounded head/tail sample, objects retain allowlisted fields, and strings are Unicode-safe truncated. No raw JSON is sliced into invalid JSON.

### Reprojection On Every Request

Before initial, tool-follow-up, semantic-correction, terminal-synthesis, and final-reflection provider calls, one projector budgets:

1. Trusted system policy.
2. Immutable task contract and unresolved obligations.
3. Newest request.
4. Tool manifest still available.
5. Recent typed observations.
6. Conversation and older context.
7. Output reserve.

Assistant tool calls and matching tool observations are atomic protocol units during pruning. Aggregate observation tokens have a hard ceiling independent of the number of calls. If required policy, task contract, newest request, manifest, and minimum output reserve cannot fit, the harness fails before contacting the provider.

`MaxPromptChars` is enforced at intake. `user_message_history` and database result schemas gain concrete item definitions and maximum sizes.

## Persistence And Migration

The new task contract, full observation store, and unknown outcomes are request-local. Durable memory stores only the final user-visible outcome and already-approved reusable read-only evidence. Unknown actions and unverified deliveries are never persisted as completed actions.

No database migration is required except for any repository signature needed to distinguish last-online `NOT_FOUND`; that distinction should use the existing query result's validity fields where possible.

## Verification

Implementation follows red-green-refactor for each numbered section in order. Completion requires all of the following:

- An action-blocking regression test proves `Executor.Execute` does not return while the action remains active, and cancellation yields non-retryable `ACTION_OUTCOME_UNKNOWN` when no verified result exists.
- Lifecycle, mail, multi-access, last-online, command gateway, delivery suppression, and persistence tests prove false success cannot escape.
- A three-plus-tool integration test proves immutable objective and obligation retention under adversarial observation content.
- Manifest tests prove each primary intent is unique and the removed overlaps are unavailable to the provider.
- Schema/encoder parity tests cover every conditional branch and malformed combination.
- Tiny-budget multi-round tests prove every provider request remains within budget and every projected observation is valid JSON.
- Focused tests, `go test -race ./internal/agent/...`, `go vet ./...`, `go test ./...`, and `make check` pass from a clean checkout.
