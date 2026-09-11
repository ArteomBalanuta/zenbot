from types import SimpleNamespace

import pytest

from . import test_provider as provider_tests
from . import conftest
import json


@pytest.mark.parametrize('summary,status', [
    ({'configuration_error': 'configured limit too small'}, 'blocked'),
    ({'error': 'provider error code=http http_status=429'}, 'provider_error'),
])
def test_provider_and_limit_failures_are_not_capability_failures(monkeypatch, summary, status):
    request = SimpleNamespace(node=SimpleNamespace(), config=SimpleNamespace(getoption=lambda _: 10))
    frozen = SimpleNamespace(path='/not/read/config.toml', environment={}, fingerprint='snapshot')
    monkeypatch.setattr(provider_tests, 'run_bridge', lambda *a, **k: summary)
    with pytest.raises(pytest.skip.Exception):
        provider_tests.test_configured_model_capability({'name': 'case', 'responses': []}, 0, request, frozen)
    assert request.node.agent_result['status'] == status
    assert request.node.agent_result['config_sha256'] == 'snapshot'


def test_interrupted_live_result_is_written_once_with_partial_evidence(tmp_path):
    path = tmp_path / 'report.json'
    active = SimpleNamespace(nodeid='live/case', agent_result={
        'status': 'interrupted', 'messages': ['partial'], 'mode': 'live',
    })
    config = SimpleNamespace(getoption=lambda _: str(path),
                             _agent_active_item=active, _agent_capability_results=[])
    conftest.pytest_sessionfinish(SimpleNamespace(config=config), 2)
    conftest.pytest_sessionfinish(SimpleNamespace(config=config), 2)
    report = json.loads(path.read_text())
    assert report['counts']['interrupted'] == 1
    assert report['results'][0]['messages'] == ['partial']
