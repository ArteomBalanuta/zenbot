"""Read-only, opt-in chat transport for a running bot."""

import json
import math
import re
import time
import uuid
import threading

import websocket

MAX_CAPTURE_CHARS = 65536


def addressed_observation(message: str, nick: str) -> dict:
    """Conservative command-output evidence for this run's unique identity.

    This is visible consistency evidence, not proof of internal tool execution.
    Do not infer observations from free-form prose or messages to other users.
    """
    match = re.fullmatch(r'\s*@' + re.escape(nick) + r'\s+response time: (\d+) milliseconds\s*', message)
    return {'latency_ms': int(match.group(1))} if match else {}


def _reject_constant(value):
    raise ValueError(f"non-JSON number: {value}")


def parse_final(text: str, marker: str) -> dict | None:
    """Only accept a complete JSON object following this request's marker."""
    if marker not in text:
        return None
    suffix = text.split(marker, 1)[1].lstrip()
    suffix = re.sub(r"^```(?:json)?\s*", "", suffix)
    try:
        value, _ = json.JSONDecoder(parse_constant=_reject_constant).raw_decode(suffix)
    except (ValueError, json.JSONDecodeError):
        return None
    return value if isinstance(value, dict) else None


def run_live_case(*, url: str, room: str, bot: str, prompt: str,
                  timeout: float = 180, prefix: str = "*", connect=None) -> dict:
    """Send one fixed read-only test prompt; never infer internal tool success.

    ``connect`` substitutes only the external wire in offline collector tests.
    No production database, provider config, bot process, or credentials are read.
    """
    if not math.isfinite(timeout) or not 0 < timeout <= 600:
        raise ValueError("live timeout must be finite and between 0 and 600 seconds")
    if not room.strip() or not bot.strip() or not prefix.strip():
        raise ValueError("live room, bot and prefix must be explicit and nonempty")
    marker = "EVAL_" + uuid.uuid4().hex
    nick = "eval_" + uuid.uuid4().hex[:12]
    started = time.monotonic()
    deadline = started + timeout
    wire = (connect or websocket.create_connection)(url, timeout=timeout)
    messages: list[str] = []
    observations: dict = {}
    captured = 0
    expired = threading.Event()
    def abort_at_deadline():
        expired.set()
        abort = getattr(wire, 'abort', None)
        if abort:
            abort()
    watchdog = threading.Timer(max(0, deadline - time.monotonic()), abort_at_deadline)
    watchdog.daemon = True
    watchdog.start()
    try:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("no correlated final reply before live deadline")
        wire.settimeout(remaining)
        wire.send(json.dumps({"cmd": "join", "channel": room, "nick": nick}))
        sent = False
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("no correlated final reply before live deadline")
            wire.settimeout(remaining)
            try:
                raw = wire.recv()
            except websocket.WebSocketTimeoutException as error:
                raise TimeoutError("no correlated final reply before live deadline") from error
            if expired.is_set() or time.monotonic() >= deadline:
                raise TimeoutError("no correlated final reply before live deadline")
            if not raw:
                raise ConnectionError("chat connection closed before final reply")
            if len(raw) > MAX_CAPTURE_CHARS:
                raise ValueError("chat event exceeded live capture limit")
            event = json.loads(raw)
            if not isinstance(event, dict):
                continue
            if event.get("cmd") == "warn":
                # Do not copy unknown server payloads into test reports.
                raise RuntimeError("chat server warning; join/request may have been rejected")
            if event.get("cmd") == "onlineSet" and not sent:
                names = event.get("nicks", [])
                if bot not in names:
                    raise ValueError("configured bot is not present in the requested room")
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError("no correlated final reply before live deadline")
                wire.settimeout(remaining)
                wire.send(json.dumps({
                    "cmd": "chat",
                    "text": f"{prefix}l {prompt} In your final reply put {marker} "
                            "followed by the requested JSON object. Do not use that marker in intermediate replies.",
                }))
                sent = True
            if not sent or event.get("cmd") != "chat" or event.get("nick") != bot:
                continue
            message = event.get("text")
            if not isinstance(message, str):
                continue
            captured += len(message)
            if captured > MAX_CAPTURE_CHARS or len(messages) >= 100:
                raise ValueError("bot output exceeded live capture limit")
            messages.append(message)
            observations.update(addressed_observation(message, nick))
            answer = parse_final(message, marker)
            if answer is not None:
                return {
                    "mode": "live", "status": "unverifiable",
                    "reason": "Chat output alone does not prove tool execution or arguments.",
                    "answer": answer, "messages": messages,
                    "observations": observations,
                    "duration_ms": round((time.monotonic() - started) * 1000),
                }
    except BaseException as error:
        if expired.is_set() and isinstance(error, Exception) and not isinstance(error, TimeoutError):
            error = TimeoutError('no correlated final reply before live deadline')
        error.evidence = {'messages': messages, 'observations': observations,
                          'duration_ms': round((time.monotonic() - started) * 1000)}
        raise error
    finally:
        watchdog.cancel()
        if isinstance(wire, websocket.WebSocket):
            wire.close(timeout=0)
        else:
            wire.close()
