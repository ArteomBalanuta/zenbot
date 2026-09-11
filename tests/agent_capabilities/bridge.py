"""Subprocess adapter for the test-only Go capability bridge."""

from __future__ import annotations

import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
from typing import Any


RESULT_PREFIX = "CAPABILITY_RESULT "
REPOSITORY_ROOT = Path(__file__).resolve().parents[2]


class BridgeError(RuntimeError):
    pass


def extract_summary(output: str) -> dict[str, Any]:
    records = [line[len(RESULT_PREFIX) :] for line in output.splitlines() if line.startswith(RESULT_PREFIX)]
    if not records:
        raise BridgeError("Go bridge did not emit a capability result")
    if len(records) != 1:
        raise BridgeError("Go bridge must emit exactly one capability result")
    try:
        value = json.loads(records[0])
    except json.JSONDecodeError as error:
        raise BridgeError("Go bridge emitted malformed capability JSON") from error
    if not isinstance(value, dict):
        raise BridgeError("Go bridge capability result must be an object")
    return value


def run_bridge(case: dict[str, Any], *, provider_config: str | None = None, timeout: float = 240,
               provider_environment: dict | None = None) -> dict[str, Any]:
    environment = dict(os.environ if provider_environment is None else provider_environment)
    environment["ZENBOT_CAPABILITY_CASE_JSON"] = json.dumps(case, ensure_ascii=False, separators=(",", ":"))
    if provider_config is not None:
        environment["ZENBOT_CAPABILITY_PROVIDER_CONFIG"] = str(Path(provider_config).resolve())
    else:
        environment.pop("ZENBOT_CAPABILITY_PROVIDER_CONFIG", None)
    with tempfile.TemporaryFile(mode="w+b") as output_file:
        process = subprocess.Popen(
            ["go", "test", "./cmd/zenbot", "-run", "^TestAgentCapabilityBridge$", "-count=1", "-v"],
            cwd=REPOSITORY_ROOT,
            env=environment,
            stdout=output_file,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        try:
            try:
                return_code = process.wait(timeout=timeout)
            except subprocess.TimeoutExpired as error:
                raise BridgeError(f"Go capability bridge exceeded {timeout:g}s and its process group was terminated") from error
        finally:
            if process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()
        size = output_file.tell()
        output_file.seek(max(0, size - 262_144))
        output = output_file.read().decode("utf-8", errors="replace")
    try:
        summary = extract_summary(output)
    except BridgeError as error:
        raise BridgeError(f"{error}; go test exit={return_code}\n{output[-4000:]}") from error
    if return_code != 0 and not summary.get("configuration_error"):
        raise BridgeError(f"Go capability bridge failed with exit {return_code}: {summary.get('error', 'unknown error')}")
    return summary
