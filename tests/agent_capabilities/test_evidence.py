import copy
import json

import pytest

from .evidence import bounded_trace, grade_evidence


@pytest.fixture
def case():
    return {
        "name": "dependent_product",
        "expected_calls": [
            {"id": "lounge", "name": "saturn_list", "arguments": {"room": "lounge"}},
            {"id": "code", "name": "saturn_list", "arguments": {"room": "programming"}},
            {"id": "kick", "name": "saturn_kick", "arguments": {"nick": "raider"}},
        ],
        "dependencies": [{"call": "kick", "after": ["lounge", "code"]}],
        "expected_final": {"counts": {"lounge": 3, "programming": 4}, "product": 12, "kicked": True},
        "action_calls": ["kick"],
        "fixture": {"rooms": {"lounge": ["a", "b", "c"], "programming": ["d", "e", "f", "g"]}},
    }


@pytest.fixture
def evidence():
    return {
        "case": "dependent_product",
        "rounds": [
            {
                "calls": [
                    {"id": "code", "name": "saturn_list", "arguments": {"room": "programming"}},
                    {"id": "lounge", "name": "saturn_list", "arguments": {"room": "lounge"}},
                ],
                "tool_results": [],
                "original_request_present": True,
            },
            {
                "calls": [{"id": "kick", "name": "saturn_kick", "arguments": {"nick": "raider"}}],
                "tool_results": [
                    {"call_id": "code", "content": {"status": "success", "data": {"data": {"room": "programming", "count": 4, "users": ["d", "e", "f", "g"]}}}},
                    {"call_id": "lounge", "content": {"status": "success", "data": {"data": {"room": "lounge", "count": 3, "users": ["a", "b", "c"]}}}},
                ],
                "original_request_present": True,
            },
            {
                "calls": [],
                "tool_results": [{"call_id": "kick", "content": {"status": "success", "effectsCommitted": True, "effectState": "COMMITTED", "actionCount": 1}}],
                "answer": '{"counts":{"lounge":3,"programming":4},"product":12,"kicked":true}',
                "original_request_present": True,
            },
        ],
        "commands": ["list programming", "list lounge", "kick raider"],
        "answer": '{"counts":{"lounge":3,"programming":4},"product":12,"kicked":true}',
        "final_text": '{"counts":{"lounge":3,"programming":4},"product":12,"kicked":true}',
        "finalizer_ok": True,
        "should_reply": True,
        "suppressed": False,
        "error": None,
    }


def assert_rejected(case, evidence, phrase):
    grade = grade_evidence(case, evidence)
    assert not grade.passed
    assert any(phrase in failure for failure in grade.failures), grade.failures


def test_accepts_independent_reads_in_reverse_order(case, evidence):
    assert grade_evidence(case, evidence).passed


def test_accepts_complete_fenced_json_not_surrounding_prose(case, evidence):
    original = evidence['final_text']
    evidence['final_text'] = '```json\n' + original + '\n```'
    assert grade_evidence(case, evidence).passed
    evidence['final_text'] = 'Here is the answer:\n' + evidence['final_text']
    assert_rejected(case, evidence, 'structured JSON')


def test_nickname_equivalence_requires_explicit_field_contract(case, evidence):
    case['expected_final']['target'] = 'raider'
    answer = json.loads(evidence['final_text'])
    answer['target'] = '@raider'
    evidence['final_text'] = json.dumps(answer)
    assert_rejected(case, evidence, 'target')
    case['nickname_fields'] = ['target']
    assert grade_evidence(case, evidence).passed
    answer['target'] = '@@raider'
    evidence['final_text'] = json.dumps(answer)
    assert_rejected(case, evidence, 'target')


def test_rejects_wrong_product(case, evidence):
    evidence["final_text"] = '{"counts":{"lounge":3,"programming":4},"product":7,"kicked":true}'
    assert_rejected(case, evidence, "final")


def test_rejects_missing_tool(case, evidence):
    evidence["rounds"][0]["calls"].pop()
    assert_rejected(case, evidence, "missing")


def test_rejects_extra_tool(case, evidence):
    evidence["rounds"][0]["calls"].append({"id": "extra", "name": "saturn_ping", "arguments": {}})
    assert_rejected(case, evidence, "extra")


def test_rejects_wrong_arguments(case, evidence):
    evidence["rounds"][0]["calls"][0]["arguments"] = {"room": "wrong"}
    assert_rejected(case, evidence, "arguments")


def test_rejects_dependency_in_same_round(case, evidence):
    evidence["rounds"][0]["calls"].append(evidence["rounds"][1]["calls"].pop())
    assert_rejected(case, evidence, "prior observations")


def test_rejects_dependency_without_all_observations(case, evidence):
    evidence["rounds"][1]["tool_results"].pop()
    assert_rejected(case, evidence, "prior observations")


def test_rejects_duplicate_action(case, evidence):
    evidence["rounds"][2]["calls"].append({"id": "kick-again", "name": "saturn_kick", "arguments": {"nick": "raider"}})
    assert_rejected(case, evidence, "duplicate action")


def test_rejects_dropped_request(case, evidence):
    evidence["rounds"][1]["original_request_present"] = False
    assert_rejected(case, evidence, "original request")


def test_rejects_absent_evidence(case, evidence):
    evidence["rounds"][1]["tool_results"] = []
    assert_rejected(case, evidence, "prior observations")


def test_rejects_error_result_for_required_success(case, evidence):
    evidence["rounds"][1]["tool_results"][0]["content"] = {
        "status": "error", "error": {"code": "INVALID_ARGUMENTS", "message": "bad"}
    }
    assert_rejected(case, evidence, "successful observation")


def test_rejects_vacuous_empty_summary():
    assert not grade_evidence({"expected_calls": [], "expected_final": {}}, {"answer": "{}", "finalizer_ok": True}).passed


def test_accepts_equal_finite_numeric_types(case, evidence):
    evidence["final_text"] = '{"counts":{"lounge":3.0,"programming":4.0},"product":12.0,"kicked":true}'
    assert grade_evidence(case, evidence).passed


def test_rejects_truncated_finalized_text(case, evidence):
    evidence["final_text"] = '{"counts":'
    assert_rejected(case, evidence, "final")


def test_rejects_observation_that_disagrees_with_fixture(case, evidence):
    evidence["rounds"][1]["tool_results"][0]["content"]["data"]["data"]["count"] = 400
    assert_rejected(case, evidence, "fixture")


def test_rejects_successful_action_without_committed_receipt(case, evidence):
    result = evidence["rounds"][2]["tool_results"][0]["content"]
    result.update({"effectsCommitted": False, "effectState": "NOT_COMMITTED", "actionCount": 0})
    assert_rejected(case, evidence, "committed action receipt")


def test_history_matching_accepts_documented_defaults_and_nick_normalization():
    case = {
        "expected_calls": [{"id": "history", "name": "user_message_history", "arguments": {"nick": "Ghost"}}],
        "expected_final": {"count": 0}, "fixture": {"history": {"Ghost": []}},
    }
    evidence = {
        "rounds": [
            {"calls": [{"id": "actual", "name": "user_message_history", "arguments": {"nick": " @ Ghost ", "room": "", "limit": 500}}], "original_request_present": True},
            {"calls": [], "tool_results": [{"call_id": "actual", "content": {"status": "success", "data": {"rows": [], "returnedCount": 0}}}], "answer": '{"count":0}', "original_request_present": True},
        ],
        "commands": [], "room_lookups": 0, "history_lookups": [{"nick": "Ghost", "room": "", "limit": 500}],
        "answer": '{"count":0}', "final_text": '{"count":0}', "finalizer_ok": True, "should_reply": True, "suppressed": False,
    }
    assert grade_evidence(case, evidence).passed
    evidence["rounds"][0]["calls"][0]["arguments"]["nick"] = "Other"
    assert_rejected(case, evidence, "missing")
    evidence["rounds"][0]["calls"][0]["arguments"]["nick"] = "@@Ghost"
    assert_rejected(case, evidence, "missing")


def test_recovery_requires_error_observation_before_repair(case, evidence):
    case = copy.deepcopy(case)
    case["recovery"] = [{"call": "kick", "after_error": "NOT_FOUND", "from": "first-kick"}]
    case["expected_calls"].insert(2, {"id": "first-kick", "name": "saturn_kick", "arguments": {"nick": "raider"}})
    evidence["rounds"][0]["calls"].append({"id": "first-kick", "name": "saturn_kick", "arguments": {"nick": "raider"}})
    assert_rejected(case, evidence, "NOT_FOUND")


def test_bounded_trace_keeps_calls_outcomes_and_truncates_answer(evidence):
    evidence["answer"] = "x" * 5000
    trace = bounded_trace(evidence, max_answer_chars=20)
    assert trace["answer"] == "x" * 20
    assert trace["answer_truncated"] is True
    assert trace["calls"][0] == {"round": 1, "id": "code", "name": "saturn_list", "arguments": {"room": "programming"}}
    assert trace["observations"][0]["status"] == "error" or trace["observations"][0]["status"] == "success"
