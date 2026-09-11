"""CLI and concise machine-report support for capability evaluations."""

from __future__ import annotations

from datetime import datetime, timezone
import json
import os
from pathlib import Path

import pytest
from .snapshot import freeze_provider


@pytest.fixture(scope='session')
def frozen_provider(request):
    path = request.config.getoption('--agent-provider-config')
    if not path:
        yield None
        return
    with freeze_provider(path, os.environ) as frozen:
        yield frozen


def pytest_addoption(parser):
    group = parser.getgroup("agent capabilities")
    group.addoption("--agent-provider-config", help="explicit TOML path for opt-in provider evaluation")
    group.addoption("--agent-repeat", type=int, default=1, help="provider repeat count (default: 1)")
    group.addoption("--agent-timeout", type=float, default=240, help="Go provider bridge subprocess deadline in seconds")
    group.addoption("--agent-report", help="write concise capability JSON report")
    group.addoption("--agent-live", action="store_true", help="enable fixed read-only live chat checks")
    group.addoption("--agent-room", help="explicit room for live chat checks")
    group.addoption("--agent-bot", help="explicit bot nickname for live chat checks")
    group.addoption("--agent-url", default="wss://hack.chat/chat-ws", help="live chat WebSocket URL")
    group.addoption("--agent-live-timeout", type=float, default=180, help="per-case live reply deadline in seconds")
    group.addoption("--agent-prefix", default="*", help="configured bot command prefix")


def pytest_configure(config):
    config.addinivalue_line("markers", "provider: opt-in configured-model capability evaluation")
    config.addinivalue_line("markers", "live: opt-in black-box chat check with unverifiable internal execution")
    repeat = config.getoption("--agent-repeat")
    timeout = config.getoption("--agent-timeout")
    if repeat < 1:
        raise pytest.UsageError("--agent-repeat must be at least 1")
    if not 0 < timeout <= 1800:
        raise pytest.UsageError("--agent-timeout must be between 0 and 1800 seconds")
    external = config.getoption("--agent-live") or config.getoption("--agent-provider-config")
    workers = getattr(config.option, "numprocesses", None)
    if external and (os.getenv("PYTEST_XDIST_WORKER") or workers not in (None, 0, 1, "0", "1")):
        raise pytest.UsageError("provider and live capability evaluations must run serially without pytest-xdist")
    config._agent_capability_results = []


def pytest_generate_tests(metafunc):
    if "agent_repeat" in metafunc.fixturenames:
        repeat = metafunc.config.getoption("--agent-repeat")
        metafunc.parametrize("agent_repeat", range(repeat), ids=[f"repeat-{index + 1}" for index in range(repeat)])


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    report = outcome.get_result()
    if report.when != "call":
        return
    if hasattr(item, "agent_result"):
        result = dict(item.agent_result)
    elif item.get_closest_marker("provider") or item.get_closest_marker("live"):
        result = {
            "mode": "provider" if item.get_closest_marker("provider") else "live",
            "status": "skipped" if report.skipped else report.outcome,
        }
    else:
        return
    if report.failed:
        result["status"] = "failed"
    elif report.skipped and result.get("status") not in ('unverifiable', 'blocked', 'provider_error'):
        result["status"] = "skipped"
    result.setdefault("pytest_outcome", report.outcome)
    result.setdefault("test", item.nodeid)
    result.setdefault("duration_ms", round(report.duration * 1000))
    item.config._agent_capability_results.append(result)


def pytest_runtest_setup(item):
    item.config._agent_active_item = item


def pytest_sessionfinish(session, exitstatus):
    report_path = session.config.getoption("--agent-report")
    if not report_path:
        return
    path = Path(report_path)
    if not path.is_absolute():
        path = Path.cwd() / path
    path.parent.mkdir(parents=True, exist_ok=True)
    results = session.config._agent_capability_results
    active = getattr(session.config, '_agent_active_item', None)
    if active is not None and hasattr(active, 'agent_result') and not any(x.get('test') == active.nodeid for x in results):
        results.append({**active.agent_result, 'test': active.nodeid, 'status': 'interrupted'})
    payload = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "exit_status": exitstatus,
        "counts": {
            "passed": sum(item.get("status") == "passed" for item in results),
            "failed": sum(item.get("status") == "failed" for item in results),
            "unverifiable": sum(item.get("status") == "unverifiable" for item in results),
            "skipped": sum(item.get("status") == "skipped" for item in results),
            "blocked": sum(item.get("status") == "blocked" for item in results),
            "provider_error": sum(item.get("status") == "provider_error" for item in results),
            "interrupted": sum(item.get("status") == "interrupted" for item in results),
        },
        "results": results,
    }
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
