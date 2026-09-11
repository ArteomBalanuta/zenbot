"""Deterministic grading of model-visible tool-loop evidence."""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
import json
import math
import re
from typing import Any


@dataclass(frozen=True)
class Grade:
    passed: bool
    failures: tuple[str, ...]
    call_count: int
    optional_call_count: int = 0


def bounded_trace(summary: dict[str, Any], *, max_answer_chars: int = 4000, max_items: int = 30) -> dict[str, Any]:
    """Return diagnostic synthetic evidence without raw subprocess/config data."""
    answer = str(summary.get("answer", ""))
    final_text = str(summary.get("final_text", ""))
    calls = []
    observations = []
    for round_index, round_data in enumerate(summary.get("rounds") or [], start=1):
        for item in round_data.get("calls", []):
            if len(calls) >= max_items:
                break
            arguments = item.get("arguments", {})
            encoded = _canonical(arguments)
            if len(encoded) > 1000:
                arguments = {"preview": encoded[:1000], "truncated": True}
            calls.append({"round": round_index, "id": item.get("id"), "name": item.get("name"), "arguments": arguments})
        for result in round_data.get("tool_results", []):
            if len(observations) >= max_items:
                break
            content = _parse_content(result)
            status = content.get("status") if isinstance(content, dict) else None
            code = None
            if isinstance(content, dict) and isinstance(content.get("error"), dict):
                code = content["error"].get("code")
            observations.append({"round": round_index, "call_id": result.get("call_id"), "status": status, "code": code})
    return {
        "answer": answer[:max_answer_chars],
        "answer_truncated": len(answer) > max_answer_chars,
        "final_text": final_text[:max_answer_chars],
        "final_text_truncated": len(final_text) > max_answer_chars,
        "calls": calls,
        "calls_truncated": sum(len(item.get("calls", [])) for item in summary.get("rounds") or []) > len(calls),
        "observations": observations,
        "observations_truncated": sum(len(item.get("tool_results", [])) for item in summary.get("rounds") or []) > len(observations),
    }


def _canonical(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def _normalize_nick(value: str) -> str:
    return value.strip().removeprefix("@").strip()


def _key(call: dict[str, Any]) -> tuple[str, str]:
    name = call.get("name", "")
    arguments = dict(call.get("arguments", {}))
    if name == "user_message_history":
        if isinstance(arguments.get("nick"), str):
            arguments["nick"] = _normalize_nick(arguments["nick"])
        arguments.setdefault("room", "")
        arguments.setdefault("limit", 500)
    elif name == "saturn_kick" and isinstance(arguments.get("nick"), str):
        arguments["nick"] = _normalize_nick(arguments["nick"])
    return name, _canonical(arguments)


def _parse_content(result: dict[str, Any]) -> Any:
    content = result.get("content")
    if isinstance(content, str):
        try:
            return json.loads(content)
        except json.JSONDecodeError:
            return content
    return content


def _contains_error(value: Any, code: str) -> bool:
    if isinstance(value, dict):
        return any(
            (key in {"errorCode", "error_code", "code"} and item == code) or _contains_error(item, code)
            for key, item in value.items()
        )
    if isinstance(value, list):
        return any(_contains_error(item, code) for item in value)
    return False


def _compare_expected(expected: Any, actual: Any, path: str, failures: list[str]) -> None:
    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            failures.append(f"final {path} must be an object")
            return
        for key, value in expected.items():
            if key not in actual:
                failures.append(f"final {path}.{key} is missing")
            else:
                _compare_expected(value, actual[key], f"{path}.{key}", failures)
        return
    if isinstance(expected, list):
        if not isinstance(actual, list) or len(actual) != len(expected):
            failures.append(f"final {path}={actual!r}; expected {expected!r}")
        else:
            for index, value in enumerate(expected):
                _compare_expected(value, actual[index], f"{path}[{index}]", failures)
        return
    if type(expected) in (int, float) and type(actual) in (int, float):
        if not math.isfinite(actual) or actual != expected:
            failures.append(f"final {path}={actual!r}; expected {expected!r}")
        return
    if type(actual) is not type(expected) or actual != expected:
        failures.append(f"final {path}={actual!r}; expected {expected!r}")


def _fixture_mismatch(case: dict[str, Any], wanted: dict[str, Any], observation: Any) -> str | None:
    if not isinstance(observation, dict) or observation.get("status") != "success":
        return None
    fixture = case.get("fixture", {})
    name = wanted.get("name")
    arguments = wanted.get("arguments", {})
    data = observation.get("data")
    if name == "saturn_list":
        room = arguments.get("room")
        expected_users = fixture.get("rooms", {}).get(room)
        observed = data.get("data") if isinstance(data, dict) else None
        if expected_users is not None and (not isinstance(observed, dict) or observed.get("room") != room or observed.get("count") != len(expected_users) or sorted(observed.get("users", []), key=str.casefold) != sorted(expected_users, key=str.casefold)):
            return f"fixture remote-room result mismatch for {room}"
    elif name == "room_users":
        expected_users = fixture.get("current_users", [])
        if not isinstance(data, dict) or data.get("count") != len(expected_users) or sorted(data.get("users", []), key=str.casefold) != sorted(expected_users, key=str.casefold):
            return "fixture current-room result mismatch"
    elif name == "saturn_weather":
        expected = fixture.get("weather", {}).get(arguments.get("location"), {}).get("message")
        messages = data.get("messages", []) if isinstance(data, dict) else []
        if expected is not None and messages != [expected]:
            return f"fixture weather result mismatch for {arguments.get('location')}"
    elif name == "saturn_ping":
        expected = fixture.get("ping", {}).get("message") or "Ping to hack.chat:80: 37 ms."
        messages = data.get("messages", []) if isinstance(data, dict) else []
        if messages != [expected]:
            return "fixture ping result mismatch"
    elif name == "user_message_history":
        nick = _normalize_nick(str(arguments.get("nick", "")))
        expected_messages = fixture.get("history", {}).get(nick, [])
        if not isinstance(data, dict) or data.get("returnedCount") != len(expected_messages) or [row.get("message") for row in data.get("rows", [])] != expected_messages:
            return f"fixture history result mismatch for {nick}"
    return None


def grade_evidence(case: dict[str, Any], summary: dict[str, Any]) -> Grade:
    """Grade observable behavior without inferring private model reasoning."""
    failures: list[str] = []
    rounds = summary.get("rounds") or []
    flattened: list[tuple[int, dict[str, Any]]] = [
        (round_index, call)
        for round_index, round_data in enumerate(rounds)
        for call in round_data.get("calls", [])
    ]
    expected = case.get("expected_calls", [])
    optional = case.get("optional_calls", [])

    if summary.get("error"):
        failures.append(f"loop error: {summary['error']}")
    if not summary.get("finalizer_ok", False):
        failures.append("final response was rejected by the production finalizer")
    if not rounds or not any(item.get("answer") for item in rounds):
        failures.append("no observable final provider round")
    if summary.get("suppressed", False) or not summary.get("should_reply", False):
        failures.append("final answer was not selected for reply")
    if any(not item.get("original_request_present", False) for item in rounds):
        failures.append("a provider round dropped the original request")

    actual_counts = Counter(_key(call) for _, call in flattened)
    expected_counts = Counter(_key(call) for call in expected)
    for key, count in (expected_counts - actual_counts).items():
        failures.append(f"missing required call {key[0]} arguments={key[1]} count={count}")
    extras = actual_counts - expected_counts
    optional_count = 0
    for allowed in optional:
        key = _key(allowed)
        used = min(extras[key], int(allowed.get("max", 1)))
        optional_count += used
        extras[key] -= used
        if extras[key] <= 0:
            del extras[key]
    for key, count in extras.items():
        matching_name = [want for want in expected if want.get("name") == key[0]]
        if matching_name:
            failures.append(f"wrong arguments or extra call {key[0]} arguments={key[1]} count={count}")
        else:
            failures.append(f"extra call {key[0]} arguments={key[1]} count={count}")

    actual_ids = [call.get("id") for _, call in flattened]
    if any(not value for value in actual_ids) or len(actual_ids) != len(set(actual_ids)):
        failures.append("tool call ids must be nonempty and unique")

    matches: dict[str, tuple[int, dict[str, Any]]] = {}
    available = list(flattened)
    for wanted in expected:
        for index, candidate in enumerate(available):
            if _key(candidate[1]) == _key(wanted):
                matches[wanted["id"]] = candidate
                available.pop(index)
                break

    observed_by_round: list[dict[str, Any]] = []
    for round_data in rounds:
        observed_by_round.append(
            {
                result.get("call_id"): _parse_content(result)
                for result in round_data.get("tool_results", [])
                if result.get("call_id")
            }
        )

    for logical_id, (call_round, call) in matches.items():
        actual_id = call.get("id")
        observations = [observed[actual_id] for observed in observed_by_round[call_round + 1 :] if actual_id in observed]
        expected_error = next((item.get("expected_error") for item in expected if item.get("id") == logical_id), None)
        if not observations:
            failures.append(f"missing observable result evidence for {logical_id}")
        elif expected_error:
            if not any(_contains_error(observation, expected_error) for observation in observations):
                failures.append(f"{logical_id} lacked expected {expected_error} observation")
        elif not any(isinstance(observation, dict) and observation.get("status") == "success" for observation in observations):
            failures.append(f"{logical_id} lacked a successful observation")
        else:
            wanted = next(item for item in expected if item.get("id") == logical_id)
            mismatch = _fixture_mismatch(case, wanted, observations[0])
            if mismatch:
                failures.append(mismatch)

    for dependency in case.get("dependencies", []):
        dependent = matches.get(dependency["call"])
        sources = [matches.get(source) for source in dependency.get("after", [])]
        if dependent is None or any(source is None for source in sources):
            continue
        dependent_round = dependent[0]
        source_ids = [source[1].get("id") for source in sources if source]
        observed = observed_by_round[dependent_round] if dependent_round < len(observed_by_round) else {}
        if any(source[0] >= dependent_round for source in sources if source) or any(source_id not in observed for source_id in source_ids):
            failures.append(f"{dependency['call']} ran without all required prior observations")

    action_ids = set(case.get("action_calls", []))
    for logical_id in action_ids:
        wanted = next((item for item in expected if item.get("id") == logical_id), None)
        if wanted is not None and actual_counts[_key(wanted)] > 1:
            failures.append(f"duplicate action {logical_id}")
        match = matches.get(logical_id)
        if wanted is not None and not wanted.get("expected_error") and match is not None:
            actual_id = match[1].get("id")
            observations = [observed[actual_id] for observed in observed_by_round[match[0] + 1 :] if actual_id in observed]
            if not any(isinstance(item, dict) and item.get("status") == "success" and item.get("effectsCommitted") is True and item.get("effectState") == "COMMITTED" and item.get("actionCount", 0) > 0 for item in observations):
                failures.append(f"{logical_id} lacked a committed action receipt")

    for recovery in case.get("recovery", []):
        repaired = matches.get(recovery["call"])
        source = matches.get(recovery["from"])
        if repaired is None or source is None:
            continue
        repaired_round = repaired[0]
        source_actual_id = source[1].get("id")
        observed = observed_by_round[repaired_round] if repaired_round < len(observed_by_round) else {}
        code = recovery["after_error"]
        if source[0] >= repaired_round or source_actual_id not in observed or not _contains_error(observed[source_actual_id], code):
            failures.append(f"recovery call {recovery['call']} lacked prior {code} observation")

    expected_commands = []
    for _, actual_call in flattened:
        name = actual_call.get("name", "")
        arguments = actual_call.get("arguments", {})
        if name.startswith("saturn_"):
            command = name.removeprefix("saturn_")
            value = ""
            if command == "list":
                value = arguments.get("room", "")
            elif command == "weather":
                value = arguments.get("location", "")
            elif command == "kick":
                value = arguments.get("nick", "")
            expected_commands.append((command + " " + str(value)).strip())
    if Counter(summary.get("commands") or []) != Counter(expected_commands):
        failures.append("gateway command evidence does not match model calls")
    expected_room_lookups = sum(1 for _, item in flattened if item.get("name") == "room_users")
    if summary.get("room_lookups", 0) != expected_room_lookups:
        failures.append("room directory evidence does not match room_users calls")
    expected_history = [item.get("arguments", {}) for _, item in flattened if item.get("name") == "user_message_history"]
    actual_history = summary.get("history_lookups") or []
    if len(actual_history) != len(expected_history):
        failures.append("history repository evidence does not match history calls")
    else:
        for wanted, actual in zip(expected_history, actual_history):
            if actual.get("nick") != _normalize_nick(str(wanted.get("nick", ""))) or actual.get("room", "") != str(wanted.get("room", "")).strip() or actual.get("limit") != wanted.get("limit", 500):
                failures.append("history repository arguments do not match model call")
                break

    answer = summary.get("final_text", "")
    try:
        if isinstance(answer, str):
            fenced = re.fullmatch(r'\s*```(?:json)?\s*\n(.*?)\n```\s*', answer, re.DOTALL)
            if fenced:
                answer = fenced.group(1)
        structured = json.loads(answer)
    except (TypeError, json.JSONDecodeError):
        failures.append("final answer is not valid structured JSON")
    else:
        if isinstance(structured, dict):
            for field in case.get('nickname_fields', []):
                if isinstance(structured.get(field), str):
                    structured[field] = _normalize_nick(structured[field])
        _compare_expected(case.get("expected_final", {}), structured, "$", failures)

    return Grade(not failures, tuple(failures), len(flattened), optional_count)
