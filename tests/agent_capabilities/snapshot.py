"""One immutable provider configuration per evaluation, with secrets in memory."""
from contextlib import contextmanager
from dataclasses import dataclass, field
import hashlib
import json
from pathlib import Path
import re
import tempfile
import tomllib


@dataclass(frozen=True)
class FrozenProvider:
    path: Path
    environment: dict = field(repr=False)
    fingerprint: str


def _toml(values, section='agent'):
    lines = ['[' + section + ']']
    for key, value in values.items():
        if not isinstance(value, dict):
            lines.append(json.dumps(key) + ' = ' + json.dumps(value, ensure_ascii=False))
    for key, value in values.items():
        if isinstance(value, dict):
            lines.append(_toml(value, section + '.' + json.dumps(key)))
    return '\n'.join(lines) + '\n'


@contextmanager
def freeze_provider(path, environment):
    agent = tomllib.loads(Path(path).read_text())['agent']
    env = dict(environment)
    key_name = env.get('SATURN_AGENT_API_KEY_ENV', agent.get('apiKeyEnv', 'SATURN_AGENT_API_KEY'))
    if key_name and not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]*', key_name):
        raise ValueError('apiKeyEnv must name an environment variable, not contain a credential')
    if any(key in agent for key in ('apiKey', 'token', 'password', 'secret')):
        raise ValueError('provider credentials must be environment variables')
    agent['creatorTrip'] = 'capability-eval-creator'
    encoded = _toml(agent)
    overrides = {key: value for key, value in env.items()
                 if key.startswith('SATURN_AGENT_') and key != key_name
                 and not key.endswith(('_KEY', '_TOKEN', '_SECRET', '_PASSWORD'))}
    fingerprint = hashlib.sha256((encoded + json.dumps(overrides, sort_keys=True)).encode()).hexdigest()
    with tempfile.TemporaryDirectory(prefix='zenbot-eval-') as directory:
        frozen_path = Path(directory) / 'provider.toml'
        frozen_path.write_text(encoded)
        yield FrozenProvider(frozen_path, env, fingerprint)
