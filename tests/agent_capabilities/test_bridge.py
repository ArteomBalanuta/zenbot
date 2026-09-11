import json

import pytest

from . import bridge
from .bridge import BridgeError, extract_summary


def test_extract_summary_uses_single_machine_record():
    expected = {"case": "weather", "answer": "{}"}
    output = "=== RUN TestAgentCapabilityBridge\nCAPABILITY_RESULT " + json.dumps(expected) + "\n--- PASS"
    assert extract_summary(output) == expected


def test_extract_summary_rejects_missing_record():
    with pytest.raises(BridgeError, match="did not emit"):
        extract_summary("PASS")


def test_extract_summary_rejects_duplicate_records():
    line = "CAPABILITY_RESULT {}"
    with pytest.raises(BridgeError, match="exactly one"):
        extract_summary(f"{line}\n{line}")


def test_run_bridge_terminates_process_group_on_interrupt(monkeypatch):
    class Process:
        pid = 43210

        def __init__(self):
            self.waits = 0

        def wait(self, timeout=None):
            self.waits += 1
            if self.waits == 1:
                raise KeyboardInterrupt
            return -9

        def poll(self):
            return None if self.waits < 2 else -9

    process = Process()
    killed = []
    monkeypatch.setattr(bridge.subprocess, "Popen", lambda *args, **kwargs: process)
    monkeypatch.setattr(bridge.os, "killpg", lambda pid, sig: killed.append((pid, sig)))
    with pytest.raises(KeyboardInterrupt):
        bridge.run_bridge({"name": "interrupted", "request": "test"})
    assert killed == [(process.pid, bridge.signal.SIGKILL)]
    assert process.waits == 2
