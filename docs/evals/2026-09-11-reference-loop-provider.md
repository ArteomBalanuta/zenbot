# Reference-informed loop: provider evaluation

## Verdict

The retained implementation has stronger tested information/receipt guarantees, but
these trials do **not** establish improved aggregate model accuracy or reliable arbitrary
multi-step completion. Its full eight-case run passed 7/8; three unchanged recovery
repetitions passed 3/3. That does not erase the failed recovery trace.

Two additional freshness-guidance experiments did not demonstrate a benefit and were
removed. Every trial, including failures, is retained below. No assertion was relaxed,
no scenario was removed, and no sampling setting was tuned to obtain a green result.

## Environment and isolation

- Production `newAgentToolLoop`, registry/manifest, assembler, SDK validation/execution,
  context projection, provider adapter, and output finalizer.
- Synthetic current room `evaluation`, synthetic creator/caller and nicknames only.
- Command, directory, history, and database I/O are fixtures. No live rooms, sockets,
  moderation, production database or command handlers were invoked.
- Remote-list fixtures now use the actual `snapshot.NewListRoomOperation().Apply`
  with synthetic source users. Formatting and typed roster data therefore match the
  production source contract; the external snapshot and delivery remain simulated.
- Provider-reported model: `Gemma4-26B-A4B-QAT-Uncensored-HauhauCS-Balanced-Q4_K_M.gguf`.
  The configured model name is empty, so the endpoint selects its loaded model.
- Thinking enabled; server-default temperature; no temperature override.
- 4,096 completion tokens; 10 model steps; 10 tool calls; 3 attempts per ordinary tool;
  3 invoked failures; 32,000 context tokens; 16,132 reserved tokens; SQL capability enabled.
- The initial/full inventory offers 65 tools and 40,852 serialized manifest bytes.
  Later offerings can shrink when ledger limits remove a tool (the focused trace
  includes a 40,242-byte manifest). All recorded HTTP attempts equalled model calls;
  no observed transport retry.
- Configuration, endpoint and credentials were not changed. Trace logs redact endpoint
  and credentials and include policy hashes, requests, calls/results and final text.
  Reasoning content is not recorded; optional diagnostic character counts are retained.

The base creator inventory is 39,793 bytes / 64 tools versus 32,513 before hints:
+7,280 bytes (+22.39%). Including request-local retrieval, the comparison is
40,852 versus 33,534 bytes (+7,318 / +21.82%). Exact manifests are budgeted; the base
size guard is 40 KiB. This is an intentional information cost, not a token reduction.

## Scenarios and oracles

1. Ordinary answer: one model round, no tools.
2. Exact kick: flat nick argument, one authorized fixture action.
3. Remote counts then kick: list lounge (3), list programming (4), product 12,
   then kick in a later model round after both observations.
4. NOT_FOUND recovery: try the requested nick; on absence inspect current room users,
   then act on the explicitly authorized alternative only if present.
5. Weather: `saturn_weather({"location":"Tokyo"})`, through the command fixture.
6. Ping: `saturn_ping({})`, fixed runtime endpoint, no invented arguments.
7. False condition: product 12 is not greater than 20; no kick.
8. True condition: product 12 is greater than 10; kick after observing both counts.

Every round asserts retention of the exact original request. Commands are checked at
the gateway boundary after schema validation; room lookup order may vary. The compound
test rejects a same-response list/list/kick batch even when commands execute in order
and final prose is correct: it requires intermediate observation before the dependent
action. Recovery includes a changing fixture: a preflight snapshot contains the old
nick, whereas a read after NOT_FOUND returns the alternative. A stale snapshot is not
sufficient evidence of its absence.

The product oracle checks for `12` and command/order assertions; it is not a general
semantic grader. Reported final answers were also inspected. These small hand-authored
cases do not cover all tools, languages, domains, real transport outcomes or long histories.

## Complete trial record

| Configuration | Cases passed | Actual failure | Full trace |
| --- | --- | --- | --- |
| Retained contracts/context, original recovery guidance | 7/8 | NOT_FOUND recovery used a pre-failure snapshot, repeated the failed kick, and falsely concluded the alternative was absent | [Run 1](2026-09-11-reference-loop-run-1.log) |
| Same retained candidate, recovery repeated unchanged | 3/3 | None in these repetitions | [Recovery before experiments](2026-09-11-reference-loop-recovery-before.log) |
| Experiment A: add two global read-freshness sentences | 7/8 | Compound case batched both lists and kick before inspecting results; final counts/product were correct but dependency oracle failed | [Run 2](2026-09-11-reference-loop-run-2.log) |
| Experiment A, four complex cases repeated three times | 10/12 | Compound ordering failed 1/3; stale-read NOT_FOUND recovery failed 1/3; both conditional cases passed 3/3 | [Focused experiment](2026-09-11-reference-loop-focused.log) |
| Experiment B: remove global sentences, extend only NOT_FOUND feedback | 7/8 | Stale-read recovery still failed; compound and both conditional cases passed | [Run 3](2026-09-11-reference-loop-run-3.log) |
| Experiment B, recovery repeated three times | 0/3 | All three relied on a pre-failure snapshot and failed to execute the authorized alternative | [Recovery experiment](2026-09-11-reference-loop-recovery-experiment.log) |

Retained policy SHA-256:
`51ca9aaccca6d3b5e23f9c8d03e71e935801f3a64724a7d852ad680e96846104`.
Experiment A policy SHA-256:
`92ecdb193a9329f81d75c37e2bd4dc6646f1021c3cb626621241f8a81c8f6dae`.
Experiment B used the retained policy hash but different NOT_FOUND observation text,
visible in the trace. Both experiments were removed before handoff. The final normal
model-input behavior is the Run 1 candidate; later receipt-notice review corrections
affect only the production failure sink, not these successful-finalizer trial paths.

The samples are small, unseeded and temporally sequential. They do not prove that either
experiment caused the observed regression. They also supply no reason to retain extra
guidance as an accuracy improvement. Results are separated by configuration rather than
pooled into a misleading overall success percentage. Earlier baseline trials are in
[the original evaluation](2026-09-11-tool-loop-provider.md); those were also variable,
so no statistically supported before/after accuracy claim is made.

## Failure analysis and decision

The stale-read failure is model-side in these traces: the exact user request, previous
read, NOT_FOUND code, effect state and remaining tool availability were present. The
model did not request the available refresh. The ledger does not cache successful reads
or prohibit another room_users call within budget. Retrying the known-uncommitted kick
does not duplicate a committed action, but wastes calls and fails the user's instructions.

The ordering failure also originated in the model's batch choice. The scheduler preserved
its source order. It cannot infer “multiply before kick” or evaluate a user's condition
without becoming the deterministic task orchestrator expressly excluded from this work.

Accordingly, neither a mandatory refresh, hidden dependency classifier, forced planner,
nor semantic completion gate was added to make the test pass. Existing action-uncertainty
blocking and resource limits remain. Better provider/model reasoning or a larger controlled
evaluation is still needed before consequential autonomous conditional tasks can be called
reliable. Broader model/backend changes were not smuggled into a contract/context revision.

## Reproduction

```sh
ZENBOT_PROVIDER_EVAL=1 go test ./cmd/zenbot -run TestProviderToolLoopEvaluation -count=1 -v
ZENBOT_PROVIDER_EVAL=1 go test ./cmd/zenbot -run 'TestProviderToolLoopEvaluation/not_found_recovery$' -count=3 -v
ZENBOT_PROVIDER_EVAL=1 go test ./cmd/zenbot -run 'TestProviderToolLoopEvaluation/(remote_counts_then_kick|not_found_recovery|conditional_no_kick|conditional_kick)$' -count=3 -v
```

Opt-in provider tests are intentionally excluded from ordinary `go test ./...` runs;
their failures are reported separately from deterministic harness regression tests.
