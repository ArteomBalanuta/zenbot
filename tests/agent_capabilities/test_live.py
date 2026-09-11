"""Explicitly opt-in read-only checks against an already-connected bot.

Passing visible-output checks is recorded separately from unverifiable internal
tool execution. This file contains no action/moderation prompts.
"""

import math
import unicodedata

import pytest

from .live import run_live_case

LIVE_CASES = [
    ("ping", 'Check the bot connection ping for hack.chat. Return JSON with "latency_ms" as a number, or null if unavailable.'),
    ("weather", 'Check the weather for Chisinau. Return JSON with "location" as a string and "temperature_c" as a number, or null if unavailable.'),
    ("counts_product", 'Count current users in lounge and programming, multiply their counts, and summarize. Return JSON with "lounge", "programming", "product" as numbers. Use null for unavailable data and any product that cannot be calculated.'),
    ("counts_weather_ping", 'Count users in lounge and programming, multiply their counts, then check weather for Chisinau, and finally check the bot connection ping for hack.chat. Summarize everything as JSON with keys "lounge", "programming", "product", "temperature_c", "latency_ms". Use null for unavailable values.'),
]


def check_observed_values(answer: dict, observations: dict) -> None:
    for key, value in observations.items():
        if key in answer:
            assert answer[key] == value, f'{key} differs from correlated command output'


def check_visible_answer(case: str, answer: dict) -> None:
    """Check output shape and arithmetic, not unobserved external facts."""
    required = {
        "ping": {"latency_ms"},
        "weather": {"location", "temperature_c"},
        "counts_product": {"lounge", "programming", "product"},
        "counts_weather_ping": {"lounge", "programming", "product", "temperature_c", "latency_ms"},
    }[case]
    assert required <= answer.keys(), "final answer omitted requested fields"
    for key in required - {"location"}:
        value = answer[key]
        assert value is None or type(value) in (int, float), f"{key} must be numeric or null"
        if type(value) is float:
            assert math.isfinite(value), f"{key} must be finite"
        if value is not None and key in {"lounge", "programming", "product"}:
            assert value >= 0 and int(value) == value, f"{key} must be a nonnegative integer"
        if key == "latency_ms" and value is not None:
            assert value >= 0, "latency cannot be negative"
    if "location" in required:
        location = answer["location"]
        assert isinstance(location, str) and location.strip(), "missing location"
        city = location.split(",", 1)[0].strip().casefold()
        city = "".join(character for character in unicodedata.normalize("NFKD", city)
                       if not unicodedata.combining(character))
        assert city == "chisinau", "location must identify Chisinau"
    if "product" in required:
        a, b = answer["lounge"], answer["programming"]
        if a is None or b is None:
            assert answer["product"] is None, "cannot multiply an unavailable count"
        else:
            assert answer["product"] == a * b, "incorrect product of reported counts"


@pytest.mark.live
@pytest.mark.parametrize("case,prompt", LIVE_CASES, ids=[case for case, _ in LIVE_CASES])
def test_connected_bot(case, prompt, request):
    if not request.config.getoption("--agent-live"):
        pytest.skip("read-only live chat checks require --agent-live")
    room = request.config.getoption("--agent-room")
    bot = request.config.getoption("--agent-bot")
    if not room or not bot:
        pytest.fail("--agent-live requires explicit --agent-room and --agent-bot")
    result = {"case": case, "mode": "live", "status": "failed"}
    request.node.agent_result = result
    try:
        collected = run_live_case(
        url=request.config.getoption("--agent-url"), room=room, bot=bot, prompt=prompt,
        timeout=request.config.getoption("--agent-live-timeout"),
        prefix=request.config.getoption("--agent-prefix"),
        )
    except BaseException as error:
        result.update(getattr(error, 'evidence', {}))
        result['status'] = 'interrupted' if isinstance(error, KeyboardInterrupt) else 'failed'
        result['failure_kind'] = type(error).__name__
        raise
    result.update(collected)
    try:
        check_visible_answer(case, result["answer"])
        check_observed_values(result['answer'], result.get('observations', {}))
    except Exception:
        result["status"] = "failed"
        raise
    result["visible_output_checks"] = "passed"
    pytest.skip("UNVERIFIABLE tool execution: visible output checks passed, internal tool trace unavailable")
