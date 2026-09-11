import tomllib

from .snapshot import freeze_provider


def test_freeze_config_and_environment_without_persisting_credentials(tmp_path):
    source = tmp_path / 'config.toml'
    source.write_text('[agent]\nmodel="first"\napiKeyEnv="TEST_KEY"\nmaxCompletionTokens=2048\n')
    env = {'TEST_KEY': 'private-value', 'SATURN_AGENT_MAX_STEPS': '8'}
    with freeze_provider(source, env) as frozen:
        source.write_text('[agent]\nmodel="second"\n')
        env['TEST_KEY'] = 'changed'
        assert tomllib.loads(frozen.path.read_text())['agent']['model'] == 'first'
        assert frozen.environment['TEST_KEY'] == 'private-value'
        assert 'private-value' not in frozen.path.read_text()
        assert 'private-value' not in repr(frozen)
        assert frozen.environment['SATURN_AGENT_MAX_STEPS'] == '8'
        assert len(frozen.fingerprint) == 64
    assert not frozen.path.exists()
