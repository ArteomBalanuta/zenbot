"""Opt-in configured-model capability evaluation."""

from __future__ import annotations

import copy

import pytest

from .bridge import BridgeError, run_bridge
from .cases import CASES
from .evidence import bounded_trace, grade_evidence


@pytest.mark.provider
@pytest.mark.parametrize("case", CASES, ids=[item["name"] for item in CASES])
def test_configured_model_capability(case, agent_repeat, request, frozen_provider):
    result = {"case": case["name"], "mode": "provider", "repeat": agent_repeat + 1, "status": "failed"}
    request.node.agent_result = result
    if frozen_provider is None:
        pytest.skip("provider evaluation requires --agent-provider-config")
    config_path = str(frozen_provider.path)
    result['config_sha256'] = frozen_provider.fingerprint
    bridge_case = copy.deepcopy(case)
    bridge_case.pop("responses")
    try:
        summary = run_bridge(
            bridge_case,
            provider_config=config_path,
            timeout=request.config.getoption("--agent-timeout"),
            provider_environment=frozen_provider.environment,
        )
    except BridgeError as error:
        result["failure_reason"] = str(error)[:1000]
        raise
    if summary.get("configuration_error"):
        result["failure_reason"] = summary["configuration_error"]
        result['status'] = 'blocked'
        pytest.skip(summary['configuration_error'])
    grade = grade_evidence(case, summary)
    result.update({
        "status": "passed" if grade.passed else "failed", "duration_ms": summary.get("duration_ms"),
        "call_count": grade.call_count, "optional_call_count": grade.optional_call_count,
        "provider_calls": summary.get("provider_calls"), "model": summary.get("model"),
        "thinking_enabled": summary.get("thinking_enabled"), "policy_sha256": summary.get("policy_sha256"),
        "limits": summary.get("limits"), "failures": list(grade.failures),
        "trace": bounded_trace(summary),
    })
    if str(summary.get('error', '')).startswith('provider error'):
        result['status'] = 'provider_error'
        pytest.skip(summary['error'])
    assert grade.passed, "; ".join(grade.failures)
