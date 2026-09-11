"""Scripted-provider integration through the real Go agent loop."""

from __future__ import annotations

import copy
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import threading

import pytest

from .bridge import BridgeError, run_bridge
from .cases import CASES
from .evidence import bounded_trace, grade_evidence


class ScriptedProvider:
    def __init__(self, responses):
        self.responses = list(responses)
        self.requests = []
        self.lock = threading.Lock()

        owner = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                if self.path != "/v1/chat/completions":
                    self.send_error(404)
                    return
                length = int(self.headers.get("Content-Length", "0"))
                request = json.loads(self.rfile.read(length))
                with owner.lock:
                    owner.requests.append(request)
                    if not owner.responses:
                        self.send_error(500, "script exhausted")
                        return
                    response = owner.responses.pop(0)
                encoded = json.dumps(response).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)

            def log_message(self, *_):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)

    @property
    def endpoint(self):
        return f"http://127.0.0.1:{self.server.server_address[1]}"


@pytest.mark.parametrize("case", CASES, ids=[item["name"] for item in CASES])
def test_scripted_trace_traverses_go_loop_and_grades(case, request):
    result = {"case": case["name"], "mode": "offline", "status": "failed"}
    request.node.agent_result = result
    bridge_case = copy.deepcopy(case)
    responses = bridge_case.pop("responses")
    try:
        with ScriptedProvider(responses) as provider:
            bridge_case["endpoint"] = provider.endpoint
            summary = run_bridge(bridge_case, timeout=45)
    except BridgeError as error:
        result["failure_reason"] = str(error)[:1000]
        raise
    grade = grade_evidence(case, summary)
    result.update({
        "status": "passed" if grade.passed else "failed",
        "duration_ms": summary.get("duration_ms"), "call_count": grade.call_count,
        "optional_call_count": grade.optional_call_count, "provider_calls": summary.get("provider_calls"),
        "failures": list(grade.failures),
        "trace": bounded_trace(summary),
    })
    if provider.responses:
        result["failure_reason"] = "scripted provider responses were not all consumed"
    assert not provider.responses, result.get("failure_reason")
    if len(provider.requests) != summary.get("provider_calls"):
        result["failure_reason"] = "provider request count disagrees with Go bridge evidence"
    assert len(provider.requests) == summary.get("provider_calls"), result.get("failure_reason")
    assert grade.passed, "; ".join(grade.failures)


def test_corpus_prompts_do_not_embed_structured_answer_or_fixture_messages():
    for case in CASES:
        prompt = case["request"]
        assert json.dumps(case["expected_final"], ensure_ascii=False, separators=(",", ":")) not in prompt
        for outcome in case.get("fixture", {}).get("weather", {}).values():
            assert outcome.get("message", "") not in prompt


def test_corpus_has_purposeful_depth_and_scope():
    assert len(CASES) >= 20
    assert any(len(case["expected_calls"]) >= 8 for case in CASES)
    assert any(len({round_index for round_index, batch in enumerate(case["responses"][:-1]) if batch}) >= 7 for case in CASES)
    forbidden = ("prompt injection", "ignore previous", "system prompt", "chain of thought")
    assert all(not any(term in case["request"].lower() for term in forbidden) for case in CASES)
