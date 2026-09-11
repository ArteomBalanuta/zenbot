# Agent capability evaluation

This Python suite tests tool selection, arguments, multi-tool dependencies,
result interpretation, arithmetic, recovery, and final synthesis. It evaluates
observable behavior, not private chain-of-thought. It does not add security or
prompt-injection scenarios. Existing Go regression tests remain unchanged in
purpose.

## Three different kinds of evidence

| Mode | What runs | What the result establishes |
| --- | --- | --- |
| Offline | Python graders and scripted provider responses through the actual Go loop | The harness executes and grades the tested traces correctly; **not** proof of a model's reasoning ability |
| Provider | Real configured model, actual Go loop, controlled tool fixtures | The model chose calls and interpreted known observations correctly for the tested scenario/settings |
| Live | Separate chat client talks to an already-connected bot | Visible-output checks may succeed, but internal tool execution remains **unverifiable**, reported as skipped rather than passed |

The provider bridge constructs the production `newAgentToolLoop` and output
finalizer. Tool schemas and execution machinery are real; room snapshots,
command outcomes, and action effects are fixtures. No actual users are kicked.
This mode does not test WebSocket delivery, database persistence, or the real
weather service. Live mode complements it rather than replacing it.

## Install and run offline

From the repository or worktree root, with Python 3.11+ and the project's Go
toolchain available:

```sh
python3 -m venv target/agent-tests-venv
target/agent-tests-venv/bin/python -m pip install -r tests/agent_capabilities/requirements.txt
target/agent-tests-venv/bin/python -m pytest tests/agent_capabilities -q
```

The default run must not contact model providers or Hack.Chat. External cases
are opt-in and reported as skipped. Offline integration tests use a loopback
provider and synthetic configuration, not your private `config.toml`.

## Evaluate the real model

Select an explicit configuration file. In a worktree, the private config from
your main checkout is not automatically copied or discovered.

```sh
target/agent-tests-venv/bin/python -m pytest tests/agent_capabilities -m provider \
  --agent-provider-config /absolute/path/to/evaluation-config.toml \
  --agent-repeat 3 \
  --agent-timeout 240 \
  --agent-report target/agent-capabilities.json \
  --junitxml target/agent-capabilities.xml
```

This makes real provider requests and can incur costs. Supply credentials via
the environment variable named by the configuration; the bot's `.env` file is
not loaded automatically. Prefer a separate evaluation configuration with
synthetic identity values. The provider receives synthetic requests and tool
observations, not the live room history or database.

Use `--collect-only` to list cases and `-k` to select a small subset before a
repeated run. Repeated trials measure consistency; do not retry failed trials
until they pass and discard the failures.

Configured execution limits matter. A ten-call task cannot complete with
`maxToolCallsPerTurn = 4`. Use an evaluation-only configuration when testing
larger budgets; do not change the live bot's limits merely to make a test pass.
Record the configuration used when comparing model versions or prompts. The
suite must not silently enlarge your configured budgets.

The scripted offline bridge uses a 64,000-token context ceiling so deep traces
can exercise the full chain without pruning their prerequisite observations.
The long provider cases require at least 32,000 context tokens, in addition to
their declared tool/round limits. These are evaluation settings, not a change
to Zenbot's defaults, and a passing high-budget run does not establish that the
same task works under a smaller production budget.

`--agent-timeout` bounds the provider-test subprocess, including Go startup;
choose it above the configured request deadline when you want the agent's own
deadline behavior to be observed. It does not override that runtime deadline.
The process-group cleanup currently targets macOS/Linux, matching the documented
shell workflow. Output limits still apply: grading uses the finalized reply,
so a correct internal answer truncated before delivery is not a success.

## Use an already-connected bot

No restart or deployment is necessary. The suite joins as another chat user
and sends fixed read-only requests to the named bot. Tools are unavailable to
whisper invocations, so these tests use public chat messages.

```sh
target/agent-tests-venv/bin/python -m pytest tests/agent_capabilities -m live \
  --agent-live \
  --agent-url wss://hack.chat/chat-ws \
  --agent-room YOUR_TEST_ROOM \
  --agent-bot YOUR_BOT_NICK \
  --agent-prefix '*' \
  --agent-live-timeout 180 \
  --agent-report target/agent-live.json
```

Use a room you control and an unprivileged test identity. Even intended read-only
requests produce chat traffic and may consume provider credits; they are not a
guarantee against model mistakes. No moderation/action prompts belong in live
mode. Test runs should be serial, not parallelized with pytest-xdist. Ordinary
room activity and a busy bot can delay replies or change room counts.

Live cases correlate replies using a unique marker. A timeout or absent final
reply is not success. A bot's claim that it called a tool is not execution
evidence. Without a correlated internal trace, tool selection/arguments/order
must remain unverifiable even when the visible answer looks correct. Do not
interpret a skipped/unverifiable test as a passed capability check.

## Reading results

Assertions should distinguish three questions:

1. Did the required calls execute with the right normalized arguments?
2. Were prerequisite observations available before dependent decisions?
3. Does the final answer match independently known results and action outcomes?

Independent reads may happen in either order. A dependent action must follow
the observations on which the decision rests, not merely appear later in a
flattened list of calls. Numeric answers are checked as values, not substrings
(`112` does not prove a product of `12`). Structured final-answer requests make
these checks reproducible; they also test formatting compliance and do not
cover every valid free-form explanation.

Keep JSON/JUnit artifacts under ignored `target/`. Reports are local debugging
artifacts, not automatically safe to publish: bot replies or provider errors
can still contain sensitive text. Review/redact them before sharing. Compare
pass/fail/unverifiable counts together with elapsed time, calls, and settings;
a single successful trial is not a general guarantee of agent capability.

## Reproducibility and grading boundaries

Prompts explicitly request currently online users, quote room names, and define
output-field meanings without providing fixture answers. The grader accepts a
whole JSON object or a complete backtick-fenced JSON object, not surrounding
prose. Only declared nickname fields permit one leading `@`; booleans, null, and
status strings are not interchangeable. Extra tools and premature dependent or
recovery calls still fail grading.

At the first provider case, the session snapshots agent configuration and the
environment. Later source-config edits do not affect the run. Temporary TOML
contains no API-key values and is removed at session end; credentials remain in
memory. Reports include the effective model and a configuration fingerprint
excluding key values. Runtime limits are never increased automatically.

JSON results distinguish `blocked` configuration/limit cases, `provider_error`
upstream failures, `interrupted` runs, and capability `failed` cases. The first
two appear as skips in pytest/JUnit, not model successes. Provider failures retain
their partial traces and grading diagnostics. Review JSON categories alongside
pytest exit status; a skip does not demonstrate that a capability works.

Live collection uses an independent deadline that aborts receives even when
WebSocket ping frames keep arriving. Late replies are rejected. Errors and
interruptions retain bounded partial bot messages. The real socket close does
not wait for a peer close handshake.

An exact ping-command reply addressed to the run's unique tester nickname cannot
become a different value or `null` in the final answer. This is visible
consistency evidence, not an internal tool receipt. Other addresses and free-form
assertions do not qualify. Weather and roster output lack equally strict source
correlation and are not promoted to trusted observations. Custom transports must
preserve the generated unique nickname to use the address-correlation check.
