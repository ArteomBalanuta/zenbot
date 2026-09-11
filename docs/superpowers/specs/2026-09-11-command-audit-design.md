# Complete command audit and repair design

## Authority and scope

Audit every catalog command, implemented legacy command, and its production dispatch path. Implement verified repairs under the user's autonomous audit-and-fix request. Preserve the current documentation cleanup. Do not deploy, restart the live bot, run live moderation, push, or rewrite Git history.

User scope update: DBZ-related commands are excluded from all remaining audit,
repair and completion requirements. This includes dbzstr, dfight, dbzhelp,
dbzregister, dspawn and dbzstats and their aliases. Previously committed DBZ
changes remain unchanged; no reversal was requested. Shared infrastructure
continues to be repaired for the remaining commands, without DBZ-specific work.

Further user scope update: automove and its aliases are also excluded for now.
Do not repair its configuration, join automation, or replica reconciliation in
this audit. Existing behavior remains unchanged; new audit reproductions are
preserved outside the active test suite for possible later work. Remaining scope
is 57 catalog identities plus legacy unlock, including 52 catalog agent tools.

The LLM remains the primary reasoning and decision-making driver. Do not introduce a semantic planner, task-obligation reducer, keyword intent classifier, or prose completion judge. Deterministic checks enforce typed argument contracts, authorization, execution receipts, cancellation, and data ownership only.

## Architectural choice

Retain the existing command catalog, native tool loop, typed moderation boundary, service layer, and repositories. Repair defects at the boundary that owns the fact or state. A wholesale command rewrite would unnecessarily duplicate working behavior and increase migration risk. Prompt-only repairs cannot correct failed I/O, authorization bypasses, lost cancellation, or database races.

Compatibility does not justify fabricated successes, disclosure of private data, or state corruption. Replace tests that explicitly require such bugs with regression tests for the intended contract. Preserve harmless aliases, formatting, and manual-command capabilities unless a concrete conflict is demonstrated.

### Common committed-mutation receipts

Use one invocation-scoped receipt recorder for verified local mutations. The
gateway installs it in the caller context; service/command owners record a
confirmed commit immediately after its authoritative write returns successfully
and before any acknowledgment or cancellation check can obscure that fact.
The recorder carries counts only, not message bodies, identity credentials,
user-intent obligations or model prose. It is isolated per invocation and safe
for concurrent callers; recording a known fact is allowed after cancellation.
Source errors never generate a committed receipt. Unknown commit outcomes remain
unknown, never retryable merely because the known count is zero.

Do not store the recorder on shared service instances, add family-specific
observers, or infer a write from a success string. Keep existing typed transport
and snapshot receipts without double-counting. Contextless local state operations
may be counted by the capture wrapper at their synchronous ownership boundary.
Acknowledgment errors propagate through every in-scope handler; previously
recorded commits survive failures, cancellation and panic. This is execution
accounting, not deterministic task-completion authority.

## Repair units

1. Privacy and identity: private mailbox delivery must match an authenticated exact trip, never a claimant-controlled nickname or case-folded trip. Public history cannot reveal whisper contents. Private notes remain private independently of the command request channel.
2. State ownership: room and AFK maps returned to callers must be detached snapshots; writes and snapshots use the same mutex. Do not hold locks while sending network messages. Copy user records at ownership boundaries.
3. Utility services: preserve upstream numeric types and timezone offsets; request the expected format; reject missing, malformed, or failed service results rather than constructing plausible success text. Propagate delivery errors.
4. Lifecycle: a started replica belongs to its controller, not a short command context. Failed startup must finish cleanup without waiting on an unstarted worker. Runtime failure cleanup cannot wait on the reporting worker itself. Replicas receive their supported command registrations.
5. Moderation and workflows: authorization matches actual side effects; structured arguments cannot reinterpret operands as flags. Partial effects and cancelled/uncertain outcomes retain evidence and are not automatically replayed. A remote workflow is not complete before its operation and transport flush finish.
6. Persistence: failed writes never become success, concurrent mutations preserve their owning records, and commit uncertainty does not authorize blind replay. DBZ-specific persistence work is excluded by the updated scope.
7. Shared dispatch and tool outcomes: retain original argument text where content is meaningful; clone canonical handlers correctly; separate verified action execution from optional chat delivery. An informational fallback is not evidence. Preserve useful, bounded errors without raw stack traces or invented semantic classification.

### Snapshot execution ownership

One workflow owns session creation, operation execution, flush, closure, and terminal publication. A request context is propagated through session creation and operation sends; cancellation signals this owner, not a second competing finalizer. The workflow remains active while execution or cleanup is outstanding. Terminal callbacks run exactly once after cleanup, and stale callbacks cannot mutate a replacement workflow. A failed operation result is failed even when its Go error is nil.

Actual successful remote sends and actual successful reply delivery are counted independently. A reply string alone is not a delivery receipt. Transport write/flush ambiguity and cancellation after dispatch remain non-retryable unknown outcomes; already observed effects stay visible. The agent waits for the synchronous action to return before receiving this outcome. Empty roster data remains a valid list result, while absent move targets or unsent remote messages cannot become claimed actions. This is execution bookkeeping, not an LLM task planner.

The existing coordinator is the ownership boundary; do not add a second remote-workflow engine or detach timed-out action goroutines. Finished workflow-state retention is bounded. Temporary nicknames derive from the complete unique workflow identity, not a shared command prefix.

An operation panic cannot escape the owner goroutine and terminate the process or bypass cleanup. Capture successful operation writes at their source boundary so a panic before an operation returns still preserves partial receipts and an uncertain terminal outcome.

### Historical game mutation work — excluded from remaining scope

The following records earlier completed source changes only. It imposes no
further DBZ audit, repair, receipt-integration or completion obligation.

Character registration and level rewards are atomic database transactions. Stat allocation is one guarded positive, affordable mutation whose affected-row result is checked; a preceding balance read does not authorize spending. Fighting and rewarding are one service operation, serialized over the selected enemy: an absent/already-consumed enemy yields no reward, and a definite failed reward does not consume it. No command ignores persistence failure or announces success before the mutation succeeds. Unknown commit outcomes must not invite blind reward replay. Help describes the actual registered game commands and configurable command prefix.

### Mail delivery ownership

Mail recipient credentials remain exact trips resolved at queue time. A separate durable delivery record is keyed by mail identity and exact recipient, so one recipient cannot consume another recipient's delivery. Keep existing stored mail and explicitly public/private visibility; legacy comma-separated recipients are interpreted without destructively rewriting the source rows.

Claim a recipient's attempt atomically before invoking transport, and persist the confirmed transport acceptance afterward. Concurrent listeners and replacement service instances must observe the same claim. A failed or interrupted write, or failure to persist its accepted result, leaves an explicit uncertain attempt that cannot be replayed automatically. Unfinished attempts are not reclaimed on a timer: a crash does not prove the write failed. Unknown attempts retain identifiers and safe operational error evidence without exposing private message bodies. This provides at most one automatic dispatch attempt, not an exactly-once recipient-receipt guarantee. A claim definitely canceled before entering the send boundary may be returned to pending; uncertainty cannot.

The delivery owner completes each message's send and persistence before advancing, preserving prior accepted effects when a later message fails. All mail/note database operations consume caller context, and delivery checks cancellation before each synchronous send. The listener formats the existing content/visibility; the service owns claim transitions. No in-memory dedup cache, durable workflow language, new planner, automatic retry scheduler, or broad mailbox deletion is required.

### Identity and authorization consistency

Trip credentials are exact, trimmed identifiers at every configured and persisted authorization/registration boundary. Nickname case-insensitive lookup does not authorize case-insensitive trip comparison. Keep the explicit configured wildcard behavior, but an ordinary unknown user without a repository has REGULAR authority, not TRUSTED. Persisted role grants must not be copied into an irrevocable in-memory administrator allowlist; configured grants remain configuration-owned.

Registration queries and mutations use the caller's context throughout and reject ambiguous nickname resolution instead of taking an arbitrary matching row. Preserve the existing registration operation choices and accurately describe their boundaries. Multi-trip role assignment is one validated transaction: all selected targets change together, or definite failure leaves them unchanged. An uncertain commit is not a verified rollback. These rules enforce credential and mutation ownership, not semantic interpretation of an LLM request.

### Explicit moderation selection

Validate the complete command grammar before any database mutation or moderation
send. `kick` accepts exactly one nickname, `-m` followed by one or more nicknames,
or `-c` followed by exactly one nonblank fragment. `shadowban` accepts exactly one
nickname or `-c` followed by exactly one nonblank fragment. `unshadowban` accepts
exactly one identity or the standalone `-all` selector. Reject mixed modes,
missing operands and ignored trailing arguments; never find a destructive flag
anywhere inside an otherwise malformed argument list. This is command syntax,
not interpretation of the user's intent.

Resolve and validate the complete kick selection before sending, retain the
existing skip-absent behavior for valid multi-target requests, and send at most
once per resolved canonical nickname. A zero-match mutation request is not a
successful action. Preserve known partial receipts if execution later fails.
Unshadow deletion output must reflect actual affected rows, not an earlier list
that may change before deletion. No rows deleted must not be reported as an
unban. A nickname's optional mention marker may be normalized only for the name
predicate; trip and hash credential matching remain exact. Structured tool
serialization must not reinterpret a declared exact target as a mode flag.

### Host lifecycle ownership

Lifecycle work and command admission share one synchronized owner. Reject new
dispatch while lifecycle work is pending, active, terminal, or closed; do not
block the retiring engine dispatcher waiting for a restart. Already admitted
commands keep their leases through execution and auditing. A command requesting
restart must return before the callback can retire its dispatcher, so request
acceptance and completed lifecycle effects are distinct facts.

The owner has a process lifetime and cancellation-aware callbacks; a command's
short context controls admission only. Callback errors remain observable, never
discarded. Close is idempotent, rejects future work, settles queued state and
wakes waiters. Process teardown first prevents further recovery/admission and
quiesces lifecycle work before final host retirement, so no replacement can be
published after teardown. Never automatically retry an uncertain lifecycle
operation merely to manufacture completion evidence.

The supervisor serializes construction/retirement/publication, publishes only a
successfully started host, and withdraws retired or unavailable hosts from every
binding. Failed replacement preserves the error and cleanup ownership, not a
pointer advertised as a usable host. Clearing the host binding must not hide
healthy separately owned replicas. No model task planner or semantic finalizer
is involved in these structural ownership rules.

### Observations survive delivery failures

Verified source data and outward-effect status are independent facts. A room
snapshot successfully parsed before a failed reply remains usable evidence,
even though delivery is unknown and may not be retried. Producers explicitly
mark valid observation data; no adapter infers validity from arbitrary response
strings, a count, or a transport success. Preserve private-output filtering and
schema validation before exposing that data to the model.

Carry optional observed data separately from safe error prose through the
gateway, tool result envelope, execution validation and bounded observation
projection. Error results may contain validated observations without becoming
successes or gaining action/delivery counts. Full request-local evidence and
compact receipt indexes retain that distinction. Invalid/unobserved/private
data is withheld; raw diagnostic strings are never repurposed as observations.

### Configured command admission

Tool visibility describes commands the current source owner can actually run,
not just interfaces exposed by a decorator. The same configured-dependency
predicate serves manual registration, model visibility and execution. Visibility
is advisory: authorize and validate actual owner availability again before
dispatch. A resolving gateway pins one host for the entire execution. Missing
dependencies and rejected lifecycle admission are known not-started outcomes.

Agent and manual commands share the existing context-aware dispatch lease.
The lease covers source work, synchronous completion and command auditing,
including cancellation and panic unwinding. Never block a retiring dispatcher
waiting for lifecycle completion. Caller policy has one shared implementation;
the LLM continues selecting among the available tools without a semantic router.

### Lifecycle request evidence

Host lifecycle tools request an operation and acknowledge admission. They do
not synchronously promise host retirement, process termination, or replica
termination. Source queue insertion is one request-state mutation; a coalesced
request reuses it and creates no new mutation. Ordinary successful acknowledgment
delivery remains a separate counted effect. Commands never await an operation
that requires their own dispatcher or lease to finish first.

Typed source data distinguishes accepted/coalesced requests from observed
completion. A nonblocking finished source handle is the only basis for completed
state; acknowledgment prose cannot establish it. Admission facts survive a
failed acknowledgment and post-admission cancellation, while those outcomes
remain unknown and non-retryable. Do not expose callback errors or handles.

### Text source observations

Successful source reads may explicitly publish a closed text-observation payload
before their outward reply. This reuses the existing observed-data path and
visibility filtering. Never infer validity from arbitrary reply/error text or
attach mutation acknowledgments as read evidence. Validated source output may
survive failed delivery without gaining successful delivery or replay permission.
Full request-local evidence remains retrievable; model projections stay bounded.

### Canonical moderation identity at the wire boundary

The selected source nickname is immutable across moderation serialization. Raw
command input may remove one mention marker once; a typed resolved nickname
must not be normalized again by the protocol layer. Validate raw operands before
effects and JSON-escape the resulting literal canonical identity. Ban/overflow
target active users just as their contracts describe, not unchecked caller
spelling or ignored extra operands. This is identity preservation, not semantic
task planning or inference that a sent request was applied by the server.

### Receipt evidence boundaries

A synchronous protocol write is evidence that a request was submitted, not
that the chat server applied moderation. Descriptions and acknowledgments must
state that boundary without inventing a correlated server response. Completed
local persistence remains a verified local mutation; successful workflow cleanup
does not prove every remote operation was applied. The LLM interprets and
summarizes these explicit facts. No semantic completion classifier, roster-based
guess or automatic retry is introduced to fill missing server evidence.

### Utility input and presence ownership

Destructive utility grammar is validated completely before effects. Notes accepts
no operands for private listing or exactly one purge/clear operand for deletion;
unexpected trailing words never broaden a deletion. AFK belongs to the active
caller nickname with its exact trip, not every nickname sharing that credential.
An absent or mismatched caller is not a successful presence change.

List/msgchannel raw room operands may remove exactly one leading room-link marker
`?`; all remaining room bytes are literal. Do not silently redirect messages by
deleting interior/trailing punctuation. Source-owned channel metadata is never
passed back through raw-operand normalization. These are syntax and identity
boundaries, not interpretation of the model's intended task.

### Historical observations are not current sessions

Nickname history uses exact trip credentials and public observation sources, not
registered ownership or private-only aliases. Last-online keeps the latest public
message and recognized presence event as independent timestamped facts across
stored rooms. A missing message does not erase a known join/leave. An unmatched
join cannot prove continuous activity or justify a running session duration.

All configured last-online source paths share the same record and renderer and
return a truthful not-found result when neither fact exists. Exact nickname-or-trip
selection cannot silently combine unrelated identities: reject cross-kind overlap
when the public matching row sets differ. No private evidence or model intent
classifier participates in that structural ambiguity check.

## Final composition boundaries

Canonical reviewed authors and snapshot nicknames are source identities, not raw
operands: never strip their leading mention markers. Normalize raw selectors once
and preserve canonical wire bytes after source resolution. The concrete current
roster updates by exact observed nickname, not shared trip/hash. AFK replaces only
the same exact nickname's state; chat removal additionally requires the exact
trip, including blank-trip equality. Global credential identity remains unchanged.

Contains-mode shadowban sends one truthful batch acknowledgment after its local
writes and kick submissions, without claiming remote application. Failed final
delivery preserves those earlier receipts. Native list's remote-only restriction
uses the same single-marker raw room interpretation as the source handler; trusted
room metadata is never normalized as an operand.

Final-review test gaps use existing bounded synchronization/commit hooks and
owned cleanup, not new lifecycle machinery. The retained prefix setter follows
the existing lock-owned setter before callbacks. Static help documents only the
supported nickname/contains shadowban grammar.

## Verification

Each fix starts with an offline failing regression. Use H2 fixtures, controlled transport seams, and in-process HTTP responses; no real user actions. Review fixes independently, then run the full uncached Go suite, affected race suites, and vet. Maintain per-command coverage and unresolved findings in the audit ledger. The audit is complete only when all command paths have an explicit disposition and cross-command regressions have been checked.
