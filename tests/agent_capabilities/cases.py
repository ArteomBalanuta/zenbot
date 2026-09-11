"""Purposeful model capability scenarios shared by offline and provider runs."""

from __future__ import annotations

import json
from typing import Any


def call(call_id: str, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
    return {"id": call_id, "name": name, "arguments": arguments}


def _tool_round(*calls: dict[str, Any]) -> dict[str, Any]:
    return {
        "choices": [{
            "message": {
                "content": None,
                "tool_calls": [
                    {
                        "id": item["id"],
                        "type": "function",
                        "function": {"name": item["name"], "arguments": json.dumps(item["arguments"], ensure_ascii=False)},
                    }
                    for item in calls
                ],
            },
            "finish_reason": "tool_calls",
        }],
        "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
    }


def _final(value: dict[str, Any]) -> dict[str, Any]:
    return {
        "choices": [{"message": {"content": json.dumps(value, ensure_ascii=False, separators=(",", ":"))}, "finish_reason": "stop"}],
        "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
    }


def scenario(
    name: str,
    request: str,
    rounds: list[list[dict[str, Any]]],
    final: dict[str, Any],
    *,
    fixture: dict[str, Any] | None = None,
    dependencies: list[dict[str, Any]] | None = None,
    recovery: list[dict[str, Any]] | None = None,
    action_calls: list[str] | None = None,
    optional_calls: list[dict[str, Any]] | None = None,
    expected_errors: dict[str, str] | None = None,
) -> dict[str, Any]:
    expected_calls = [item for batch in rounds for item in batch]
    for recovery_rule in recovery or []:
        source = next(item for item in expected_calls if item["id"] == recovery_rule["from"])
        source["expected_error"] = recovery_rule["after_error"]
    for logical_id, code in (expected_errors or {}).items():
        source = next(item for item in expected_calls if item["id"] == logical_id)
        source["expected_error"] = code
    per_tool: dict[str, int] = {}
    for item in expected_calls:
        per_tool[item["name"]] = per_tool.get(item["name"], 0) + 1
    optional_budget = 0
    for item in optional_calls or []:
        maximum = int(item.get("max", 1))
        optional_budget += maximum
        per_tool[item["name"]] = per_tool.get(item["name"], 0) + maximum
    conventions = {
        'warmer': '"warmer" is the warmer city name, not a boolean.',
        'first': '"first" is a lowercase outcome code.',
        'history': '"history" is the string "unavailable" on lookup failure.',
        'target': '"target" is the nickname, optionally with one leading @.',
        'alternative': '"alternative" is the nickname, optionally with one leading @.',
    }
    field_help = ' '.join(text for key, text in conventions.items() if key in final)
    return {
        "name": name,
        "nickname_fields": [key for key in ("target", "alternative") if key in final],
        "request": request + ' ' + field_help + " Return only the requested JSON object, with numeric and boolean values as JSON numbers and booleans.",
        "fixture": fixture or {},
        "expected_calls": expected_calls,
        "dependencies": dependencies or [],
        "recovery": recovery or [],
        "action_calls": action_calls or [],
        "optional_calls": optional_calls or [],
        "expected_final": final,
        "required_limits": {
            "max_steps": max(1, len(rounds) + 1),
            "max_tools": len(expected_calls) + optional_budget,
            "max_calls_per_tool": max(per_tool.values(), default=1),
            "max_tool_failures": 1 if recovery else 0,
            "max_context_tokens": 32000 if len(expected_calls) >= 5 else 16000,
        },
        "responses": [_tool_round(*batch) for batch in rounds] + [_final(final)],
    }


ROOMS = {
    "alpha": ["a1", "a2"],
    "beta": ["b1", "b2", "b3"],
    "gamma": ["g1", "g2", "g3", "g4", "g5"],
    "delta": [f"d{i}" for i in range(1, 8)],
    "epsilon": [f"e{i}" for i in range(1, 12)],
    "empty": [],
}
CURRENT = ["operator", "raider", "friend"]
WEATHER = {
    "Tokyo": {"message": "Tokyo: clear, 24 °C, humidity 65%, wind 8 km/h."},
    "Oslo": {"message": "Oslo: cloudy, 9 °C, humidity 71%, wind 11 km/h."},
    "Chișinău": {"message": "Chișinău: sunny, 18 °C, humidity 52%, wind 6 km/h."},
}


CASES = [
    scenario("arithmetic_without_tools", 'Calculate six multiplied by seven. Do not use a tool. Use the key "result".', [], {"result": 42}),
    scenario("weather_tokyo", 'Get current weather for Tokyo. Use keys "location" and "temperature_c".', [[call("weather", "saturn_weather", {"location": "Tokyo"})]], {"location": "Tokyo", "temperature_c": 24}, fixture={"weather": WEATHER}),
    scenario("weather_unicode_location", 'Get current weather for Chișinău. Use keys "location" and "temperature_c".', [[call("weather", "saturn_weather", {"location": "Chișinău"})]], {"location": "Chișinău", "temperature_c": 18}, fixture={"weather": WEATHER}),
    scenario("ping_no_arguments", 'Measure the bot connection ping. Use the key "latency_ms".', [[call("ping", "saturn_ping", {})]], {"latency_ms": 37}),
    scenario("current_room_count", 'Inspect current-room users. Use the key "count".', [[call("current", "room_users", {})]], {"count": 3}, fixture={"current_users": CURRENT}),
    scenario("history_limit_and_latest", 'Read Alice public history with limit two. Use keys "count" (number of returned messages) and "latest" (text of the most recent returned message, not its ID).', [[call("history", "user_message_history", {"nick": "Alice", "limit": 2})]], {"count": 2, "latest": "beta"}, fixture={"history": {"Alice": ["alpha", "beta"]}}),
    scenario("history_empty", 'Read Ghost public history using the default limit. Use keys "count" and "empty".', [[call("history", "user_message_history", {"nick": "Ghost"})]], {"count": 0, "empty": True}, fixture={"history": {"Ghost": []}}),
    scenario("remote_room_count", 'Count currently online users in the room named "alpha". Use keys "room" and "count".', [[call("alpha", "saturn_list", {"room": "alpha"})]], {"room": "alpha", "count": 2}, fixture={"rooms": ROOMS}),
    scenario("remote_room_zero_not_unavailable", 'Count currently online users in the room named "empty" and distinguish an available empty room from an unavailable room. Use keys "available" and "count".', [[call("empty", "saturn_list", {"room": "empty"})]], {"available": True, "count": 0}, fixture={"rooms": ROOMS}),
    scenario(
        "unavailable_room_recovery",
        'Fetch the online-user roster of the remote room literally named "missing". If unavailable, inspect current-room users. Use keys "remote_available" and "current_count".',
        [[call("missing", "saturn_list", {"room": "missing"})], [call("current", "room_users", {})]],
        {"remote_available": False, "current_count": 3},
        fixture={"rooms": ROOMS, "current_users": CURRENT},
        recovery=[{"call": "current", "from": "missing", "after_error": "COMMAND_REJECTED"}],
    ),
    scenario("two_room_sum", 'Count currently online users in rooms "alpha" and "beta", then compute their sum. Use an object "counts" keyed by room and the key "sum".', [[call("alpha", "saturn_list", {"room": "alpha"}), call("beta", "saturn_list", {"room": "beta"})]], {"counts": {"alpha": 2, "beta": 3}, "sum": 5}, fixture={"rooms": ROOMS}),
    scenario("two_room_product", 'Count currently online users in rooms "beta" and "gamma", then compute their product. Use an object "counts" keyed by room and the key "product".', [[call("beta", "saturn_list", {"room": "beta"}), call("gamma", "saturn_list", {"room": "gamma"})]], {"counts": {"beta": 3, "gamma": 5}, "product": 15}, fixture={"rooms": ROOMS}),
    scenario("three_source_total", 'Count currently online users in rooms "alpha" and "beta" plus the number of currently online users in this room, then compute the total. Use keys "remote", "current", and "total".', [[call("alpha", "saturn_list", {"room": "alpha"}), call("beta", "saturn_list", {"room": "beta"}), call("current", "room_users", {})]], {"remote": 5, "current": 3, "total": 8}, fixture={"rooms": ROOMS, "current_users": CURRENT}),
    scenario(
        "conditional_kick_true",
        'Count currently online users in rooms "beta" and "gamma". Kick raider only if their product is greater than ten. Use keys "product" and "kicked".',
        [[call("beta", "saturn_list", {"room": "beta"}), call("gamma", "saturn_list", {"room": "gamma"})], [call("kick", "saturn_kick", {"nick": "raider"})]],
        {"product": 15, "kicked": True}, fixture={"rooms": ROOMS, "current_users": CURRENT, "kick": {"raider": ["succeeded"]}},
        dependencies=[{"call": "kick", "after": ["beta", "gamma"]}], action_calls=["kick"],
        optional_calls=[{"name": "room_users", "arguments": {}, "max": 1}],
    ),
    scenario("conditional_kick_false", 'Count currently online users in rooms "alpha" and "beta". Kick raider only if their product is greater than ten. Use keys "product" and "kicked".', [[call("alpha", "saturn_list", {"room": "alpha"}), call("beta", "saturn_list", {"room": "beta"})]], {"product": 6, "kicked": False}, fixture={"rooms": ROOMS, "kick": {"raider": ["succeeded"]}}),
    scenario("exact_kick_argument", 'Kick exactly raider. Use keys "target" and "kicked".', [[call("kick", "saturn_kick", {"nick": "raider"})]], {"target": "raider", "kicked": True}, fixture={"current_users": ["raider"], "kick": {"raider": ["succeeded"]}}, action_calls=["kick"], optional_calls=[{"name": "room_users", "arguments": {}, "max": 1}]),
    scenario(
        "not_found_alternative",
        'Try to kick raider. If NOT_FOUND, inspect current users, then kick raider_alt only if present. Use keys "first", "alternative", and "kicked".',
        [[call("first", "saturn_kick", {"nick": "raider"})], [call("inspect", "room_users", {})], [call("alternative", "saturn_kick", {"nick": "raider_alt"})]],
        {"first": "not_found", "alternative": "raider_alt", "kicked": True},
        fixture={"current_users": ["operator", "raider_alt", "friend"], "kick": {"raider": ["not_found"], "raider_alt": ["succeeded"]}},
        recovery=[{"call": "inspect", "from": "first", "after_error": "NOT_FOUND"}],
        dependencies=[{"call": "alternative", "after": ["inspect"]}], action_calls=["first", "alternative"],
    ),
    scenario(
        "not_found_no_alternative",
        'Try to kick raider. If NOT_FOUND, inspect current users. Do not kick anyone else when raider_alt is absent. Use keys "first", "alternative_present", and "kicked".',
        [[call("first", "saturn_kick", {"nick": "raider"})], [call("inspect", "room_users", {})]],
        {"first": "not_found", "alternative_present": False, "kicked": False},
        fixture={"current_users": ["operator", "friend"], "kick": {"raider": ["not_found"]}},
        recovery=[{"call": "inspect", "from": "first", "after_error": "NOT_FOUND"}], action_calls=["first"],
    ),
    scenario(
        "five_room_aggregates",
        'Count currently online users in rooms "alpha", "beta", "gamma", "delta" and "epsilon". In that order, report the count list, sum, product, maximum, and maximum-minus-minimum spread using keys "counts", "sum", "product", "max", and "spread".',
        [[call(name, "saturn_list", {"room": name}) for name in ["alpha", "beta", "gamma", "delta", "epsilon"]]],
        {"counts": [2, 3, 5, 7, 11], "sum": 28, "product": 2310, "max": 11, "spread": 9}, fixture={"rooms": ROOMS},
    ),
    scenario(
        "weather_comparison_then_ping",
        'Get Tokyo and Oslo weather. Ping only after observing both, and only if Tokyo is warmer. Use keys "warmer", "difference_c", and "ping_ms".',
        [[call("tokyo", "saturn_weather", {"location": "Tokyo"}), call("oslo", "saturn_weather", {"location": "Oslo"})], [call("ping", "saturn_ping", {})]],
        {"warmer": "Tokyo", "difference_c": 15, "ping_ms": 37}, fixture={"weather": WEATHER},
        dependencies=[{"call": "ping", "after": ["tokyo", "oslo"]}],
    ),
    scenario(
        "history_error_recovery",
        'Read Broken public history using the default limit. If the tool fails, inspect current users. Use keys "history" and "current_count".',
        [[call("history", "user_message_history", {"nick": "Broken"})], [call("current", "room_users", {})]],
        {"history": "unavailable", "current_count": 3}, fixture={"history_error": ["Broken"], "current_users": CURRENT},
        recovery=[{"call": "current", "from": "history", "after_error": "TOOL_EXECUTION_FAILED"}],
    ),
    scenario(
        "nine_call_mixed_synthesis",
        'Get current online-user counts for rooms "alpha", "beta", "gamma", "delta" and "epsilon", Tokyo and Oslo weather, and Alice public history with limit two. After those results, ping only if the largest room count equals eleven and Tokyo is warmer. Use keys "room_sum", "room_product", "warmer", "history_count", and "ping_ms".',
        [[
            *[call(name, "saturn_list", {"room": name}) for name in ["alpha", "beta", "gamma", "delta", "epsilon"]],
            call("tokyo", "saturn_weather", {"location": "Tokyo"}), call("oslo", "saturn_weather", {"location": "Oslo"}),
            call("history", "user_message_history", {"nick": "Alice", "limit": 2}),
        ], [call("ping", "saturn_ping", {})]],
        {"room_sum": 28, "room_product": 2310, "warmer": "Tokyo", "history_count": 2, "ping_ms": 37},
        fixture={"rooms": ROOMS, "weather": WEATHER, "history": {"Alice": ["alpha", "beta"]}},
        dependencies=[{"call": "ping", "after": ["alpha", "beta", "gamma", "delta", "epsilon", "tokyo", "oslo", "history"]}],
    ),
    scenario(
        "unknown_kick_not_repeated",
        'Kick raider once. If the outcome is unknown, do not retry and report it as unconfirmed. Use keys "target", "status", and "retried".',
        [[call("kick", "saturn_kick", {"nick": "raider"})]],
        {"target": "raider", "status": "unconfirmed", "retried": False},
        fixture={"current_users": ["raider"], "kick": {"raider": ["unknown"]}},
        action_calls=["kick"], expected_errors={"kick": "ACTION_OUTCOME_UNKNOWN"},
    ),
    scenario(
        "sequential_eight_call_retention",
        'Count currently online users in room "alpha", then only after seeing it count users in "beta", then "gamma", then "delta", then "epsilon". After retaining all five counts, compare Tokyo and Oslo weather. Ping only after observing both weather results and only if Tokyo is warmer. Use keys "counts", "sum", "product", "warmer", and "ping_ms".',
        [
            [call("alpha", "saturn_list", {"room": "alpha"})],
            [call("beta", "saturn_list", {"room": "beta"})],
            [call("gamma", "saturn_list", {"room": "gamma"})],
            [call("delta", "saturn_list", {"room": "delta"})],
            [call("epsilon", "saturn_list", {"room": "epsilon"})],
            [call("tokyo", "saturn_weather", {"location": "Tokyo"}), call("oslo", "saturn_weather", {"location": "Oslo"})],
            [call("ping", "saturn_ping", {})],
        ],
        {"counts": [2, 3, 5, 7, 11], "sum": 28, "product": 2310, "warmer": "Tokyo", "ping_ms": 37},
        fixture={"rooms": ROOMS, "weather": WEATHER},
        dependencies=[
            {"call": "beta", "after": ["alpha"]},
            {"call": "gamma", "after": ["beta"]},
            {"call": "delta", "after": ["gamma"]},
            {"call": "epsilon", "after": ["delta"]},
            {"call": "tokyo", "after": ["alpha", "beta", "gamma", "delta", "epsilon"]},
            {"call": "oslo", "after": ["alpha", "beta", "gamma", "delta", "epsilon"]},
            {"call": "ping", "after": ["tokyo", "oslo"]},
        ],
    ),
]


CASES_BY_NAME = {item["name"]: item for item in CASES}
