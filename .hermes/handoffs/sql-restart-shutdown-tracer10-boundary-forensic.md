# SQL tracer 10 boundary forensic — cancellation and best-effort delivery

**Verdict:** accept SQL pre-cancellation as an **inherited target-contract regression-only assertion**, not a tracer-10 RED→GREEN result and not a prior-tracer recovery trigger. The next send-failure assertion has the same disposition: it specifies already-present best-effort delivery and **will be green today**. Do not remove or weaken either existing behavior merely to manufacture RED evidence.

This document records an inspection-only decision. It makes no Go source/test edits and does not stage, reset, restore, clean, or commit the dirty worktree.

## Binding evidence

### Cancellation is inherited, not a new tracer-10 behavior

`sql-restart-shutdown-parity-architecture.md` is explicit at §1, **TARGET-ONLY CANCELLATION RULE**: every concrete command must check `ctx.Err()` before any side effect, returning `FAILED, ctx.Err()` with no reply/call. It names SQL specifically: pass the received context to the database without adding a deadline. Its SQL failure matrix likewise requires `FAILED, context.Canceled` and no DB call/reply for a cancelled context.

That rule precedes the sequential tracer plan and applies to the concrete SQL command as a target-wide Go adaptation; it is not a new source-parity behavior introduced by tracer 10. The same binding plan describes tracer 10 as a combined error/cancellation/delivery validation pass, but a test that is first observed green cannot retroactively serve as its GREEN phase.

The current implementation follows the inherited rule exactly:

- `internal/command/sql.go:15-18` checks `ctx.Err()` before raw parsing, service lookup, query, rendering, or reply.
- `internal/command/sql.go:32` passes that received context to `RawSQLQuery.Query`.
- `internal/command/sql_test.go:139-156` asserts `FAILED`/`context.Canceled` with zero query and chat effects.
- The implementation handoff records that this test's first execution was green because the guard already existed from earlier SQL parser work (`sql-restart-shutdown-parity-implementation.md`, “Tracer 10b stop condition”).

### Strict-TDD classification

The pre-cancelled test is valid regression coverage, but it is not witnessed RED evidence for tracer 10b. The historical record does not show a separate earlier cancellation RED for the initial guard; therefore this review does **not** claim a clean standalone cancellation RED→GREEN cycle.

That provenance gap does not justify a recovery that removes the guard: doing so would deliberately violate the binding target contract and create an unsafe transient state only to make a test fail. Record the assertion as inherited regression coverage, exclude it from tracer-10 RED→GREEN accounting, and continue without cancellation production changes. If an owner requires every inherited guard to have original witnessed RED evidence, the appropriate action is an explicit TDD-history rebaseline, not code rollback/recovery in this shared dirty tree.

## Send-failure boundary

### Existing delivery contract

`internal/command/handlers.go:70-72` defines the command reply seam:

```go
func reply(c *commandBase, text string) {
    _, _ = c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper")
}
```

It intentionally discards the returned delivery error. SQL uses this helper for both result paths:

- success: `internal/command/sql.go:37-38`;
- raw query/driver error rendered as source-shaped result: `internal/command/sql.go:33-35`.

The binding architecture's SQL failure matrix independently requires: **“Chat delivery fails | Preserve existing `reply` best-effort behavior: status remains successful and delivery error is discarded.”** The tracer-10a GREEN change was deliberately limited to routing a raw query error through this existing helper, and its handoff expressly states that it retained best-effort delivery unchanged.

The underlying engine can return a real send error (`internal/core/engine_impl.go:443-452`); that does not alter command status because the helper ignores it. `internal/command/msg_channel_test.go:175-210` is a contrasting command-local seam: it explicitly returns `SendChatMessage` errors and therefore has a failing-send test that expects `FAILED`. That is not the SQL contract.

### Minimal regression test shape

The smallest precise SQL send-failure assertion is a **direct command test**, not listener integration and not a real transport test:

1. Define a local `sendFailureSQLCommandEngine` in `internal/command/sql_test.go` that embeds `*commandEngineStub`, records each `SendChatMessage` invocation, and returns a stable sentinel error.
2. Use the existing `recordingRawSQLQuery` capability and a valid `!sql SELECT 1` message. Its nil/empty `SQLTable` is sufficient; no H2 fixture, auth/listener, renderer golden, or driver-error setup is needed.
3. Execute with `context.Background()`.
4. Assert all of the following, and nothing transport-specific:
   - `status == model.SUCCESSFUL` and `err == nil`;
   - exactly one raw-query call with `SELECT 1`;
   - exactly one attempted `SendChatMessage` call addressed to `alice`, public unless the test intentionally selects whisper;
   - no retry and no raw-message side effect.

A separate variant is unnecessary: `sqlCommand` has a single `reply` seam, so a successful-query send-failure case proves the delivery contract without coupling this tracer to the already-covered raw-driver-error behavior. Do not assert a delivered chat record: a failed send is still an attempted call, not a successful delivery.

### RED feasibility and correct disposition

That test **will pass immediately today**:

- successful SQL reaches `reply` (`sql.go:37`);
- `reply` invokes `SendChatMessage` once and discards its error (`handlers.go:70-72`);
- SQL then returns `SUCCESSFUL, nil` (`sql.go:38`).

It therefore cannot be a valid new strict-TDD tracer whose required first observation is RED. A listener test would be weaker because `legacyAdapter` does not expose status/error and the same helper still consumes the send failure. A test expecting `FAILED` or propagated send error would RED, but it would contradict both the binding delivery matrix and established helper contract, so it is invalid.

Classify this as a regression-only contract test if desired for future protection. It does not require SQL production code, a new reply helper, a transport retry, error logging, a listener change, or any catalog/factory/lifecycle work.

## Safe resume recommendation

1. Keep `TestSQLCommandPreCancelledContextDoesNotQueryOrReply` as inherited regression coverage. Mark tracer 10b **not witnessed RED→GREEN**, but do not reopen parser work or remove the guard.
2. If adding coverage is useful, add only the minimal direct successful-query/send-failure regression above and record its expected first-run green result. Do not call it a new tracer 10c GREEN.
3. Treat tracer 10 as complete only in the limited behavioral sense already evidenced by tracer 10a plus the two inherited regression assertions; its strict-TDD ledger must say that 10b/10c are inherited/regression-only.
4. Resume the next independently RED-capable slice rather than modifying already-correct SQL delivery/cancellation behavior to force a failure. Any demand for uninterrupted witnessed RED history must be resolved by owner-approved rebaseline, not speculative recovery.

## Focused verification in the inspected state

From `/Users/ab/workspace/go-projects/zenbot`:

```text
go test ./internal/command -run '^(TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper|TestSQLCommandPreCancelledContextDoesNotQueryOrReply)$' -count=1
ok   zenbot/internal/command  1.210s

go test -race ./internal/command -run '^(TestUserChatListenerSQLRepliesWithRawH2DriverErrorAndWhisper|TestSQLCommandPreCancelledContextDoesNotQueryOrReply)$' -count=1
ok   zenbot/internal/command  3.110s

git diff --check
# exit 0
```

The repository remains broadly dirty and unstaged. The SQL implementation files and prior handoffs are untracked; this forensic handoff is the sole file created by this review.
