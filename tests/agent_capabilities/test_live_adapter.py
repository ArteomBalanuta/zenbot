"""Offline regressions for reply correlation and bounded live collection."""

import json
import base64
import hashlib
import socketserver
import struct
import threading
import time
from types import SimpleNamespace

import pytest

from . import live as live_module
from . import test_live as live_tests
from .live import parse_final, run_live_case
from .test_live import check_visible_answer


def test_final_requires_marker_and_complete_json():
    assert parse_final('unrelated {"product":12}', 'EVAL_ab') is None
    assert parse_final('EVAL_ab {"product":', 'EVAL_ab') is None
    assert parse_final('summary EVAL_ab {"product":12}', 'EVAL_ab') == {"product": 12}


def test_final_accepts_fenced_json_after_marker():
    assert parse_final('EVAL_ab\n```json\n{"latency_ms":37}\n```', 'EVAL_ab') == {"latency_ms": 37}


def test_final_rejects_non_object_and_nonfinite_numbers():
    for value in ('[]', 'true', '{"latency_ms":NaN}', '{"latency_ms":Infinity}'):
        assert parse_final('EVAL_ab ' + value, 'EVAL_ab') is None


class ChatWire:
    """The external chat socket only; collector/parser under test remain real."""

    def __init__(self, reply=True, bot_present=True, oversized=False):
        self.events = []
        self.closed = False
        self.reply = reply
        self.bot_present = bot_present
        self.oversized = oversized

    def send(self, value):
        message = json.loads(value)
        if message['cmd'] == 'join':
            self.events.append({'cmd': 'onlineSet', 'nicks': ['Bot'] if self.bot_present else []})
        elif message['cmd'] == 'chat':
            marker = message['text'].split('EVAL_', 1)[1].split()[0]
            marker = 'EVAL_' + marker
            # Unrelated user's valid JSON must not satisfy the bot reply.
            self.events.append({'cmd': 'chat', 'nick': 'visitor', 'text': marker + ' {"latency_ms":999}'})
            if self.reply:
                self.events.extend([
                    {'cmd': 'chat', 'nick': 'Bot', 'text': 'ping result: 37 ms'},
                    {'cmd': 'chat', 'nick': 'Bot',
                     'text': marker + ' {"latency_ms":37}'},
                ])
            if self.oversized:
                self.events.insert(0, {'cmd': 'chat', 'nick': 'Bot', 'text': 'x' * 70000})

    def recv(self):
        if not self.events:
            raise TimeoutError('no final reply')
        return json.dumps(self.events.pop(0))

    def settimeout(self, seconds):
        assert seconds > 0

    def close(self):
        self.closed = True


class CrossReplyWire(ChatWire):
    """A marked partial answer followed by an unrelated bot message."""

    def send(self, value):
        message = json.loads(value)
        if message['cmd'] == 'join':
            self.events.append({'cmd': 'onlineSet', 'nicks': ['Bot']})
        elif message['cmd'] == 'chat':
            marker = 'EVAL_' + message['text'].split('EVAL_', 1)[1].split()[0]
            self.events.extend([
                {'cmd': 'chat', 'nick': 'Bot', 'text': marker + ' {"latency_ms":'},
                {'cmd': 'chat', 'nick': 'Bot', 'text': '37}'},
            ])


def run(wire, **options):
    return run_live_case(url='ws://127.0.0.1:1234', room='test', bot='Bot',
                         prompt='Check ping.', connect=lambda *a, **k: wire, **options)


def test_deadline_interrupts_blocked_receive_and_retains_partial_evidence():
    class BlockedWire(ChatWire):
        def __init__(self):
            super().__init__(reply=False)
            self.release = threading.Event()
        def recv(self):
            if self.events:
                return super().recv()
            self.release.wait(0.7)
            return ''
        def abort(self):
            self.release.set()
    wire = BlockedWire()
    started = time.monotonic()
    with pytest.raises(TimeoutError) as error:
        run(wire, timeout=0.08)
    assert time.monotonic() - started < 0.5
    assert hasattr(error.value, 'evidence')
    assert wire.closed


def test_correlated_observation_cannot_be_discarded_as_null():
    from .test_live import check_observed_values
    with pytest.raises(AssertionError, match='latency_ms'):
        check_observed_values({'latency_ms': None}, {'latency_ms': 190})
    check_observed_values({'latency_ms': None}, {})


def test_collector_ignores_other_users_and_accepts_correlated_reply():
    wire = ChatWire()
    result = run(wire)
    assert result['answer'] == {'latency_ms': 37}
    assert result['status'] == 'unverifiable'
    assert all('999' not in message for message in result['messages'])
    assert wire.closed


def test_unrelated_later_bot_message_cannot_finish_partial_marked_json():
    wire = CrossReplyWire()
    with pytest.raises(TimeoutError):
        run(wire)
    assert wire.closed


def test_missing_final_reply_is_failure_and_closes_socket():
    wire = ChatWire(reply=False)
    with pytest.raises(TimeoutError):
        run(wire)
    assert wire.closed


def test_missing_bot_fails_before_sending_prompt():
    wire = ChatWire(bot_present=False)
    with pytest.raises(ValueError, match='not present'):
        run(wire)
    assert wire.closed


def test_oversized_output_fails_instead_of_unbounded_capture():
    wire = ChatWire(oversized=True)
    with pytest.raises(ValueError, match='limit'):
        run(wire)
    assert wire.closed


@pytest.mark.parametrize('timeout', [0, -1, float('inf'), float('nan')])
def test_invalid_deadline_fails_before_connect(timeout):
    def unexpected(*args, **kwargs):
        pytest.fail('connected with invalid timeout')
    with pytest.raises(ValueError):
        run_live_case(url='ws://localhost', room='test', bot='Bot', prompt='ping',
                      timeout=timeout, connect=unexpected)


def test_total_deadline_is_reapplied_before_each_send(monkeypatch):
    clock = SimpleNamespace(now=0.0)

    class DeadlineWire:
        def __init__(self):
            self.events = []
            self.timeout = None
            self.send_timeouts = []

        def settimeout(self, seconds):
            self.timeout = seconds

        def send(self, value):
            self.send_timeouts.append(self.timeout)
            clock.now += 2
            message = json.loads(value)
            if message['cmd'] == 'join':
                self.events.append({'cmd': 'onlineSet', 'nicks': ['Bot']})
            else:
                marker = 'EVAL_' + message['text'].split('EVAL_', 1)[1].split()[0]
                self.events.append({'cmd': 'chat', 'nick': 'Bot',
                                    'text': marker + ' {"latency_ms":37}'})

        def recv(self):
            clock.now += 2
            return json.dumps(self.events.pop(0))

        def close(self):
            pass

    wire = DeadlineWire()
    monkeypatch.setattr(live_module.time, 'monotonic', lambda: clock.now)
    result = run_live_case(url='ws://localhost', room='test', bot='Bot', prompt='ping',
                           timeout=10, connect=lambda *args, **kwargs: wire)
    assert result['answer'] == {'latency_ms': 37}
    assert wire.send_timeouts == [10, 6]


@pytest.mark.parametrize('answer', [
    {'lounge': 3, 'programming': 4, 'product': 112},
    {'lounge': 3, 'programming': 4},
    {'lounge': True, 'programming': 4, 'product': 4},
    {'lounge': 3.5, 'programming': 4, 'product': 14},
    {'lounge': -3, 'programming': 4, 'product': -12},
    {'lounge': None, 'programming': 4, 'product': 0},
    {'lounge': float('nan'), 'programming': 4, 'product': None},
])
def test_visible_grader_rejects_inconsistent_or_missing_results(answer):
    with pytest.raises(AssertionError):
        check_visible_answer('counts_product', answer)


@pytest.mark.parametrize('answer', [
    {'lounge': 3, 'programming': 4, 'product': 12},
    {'lounge': 0, 'programming': 4, 'product': 0},
    {'lounge': None, 'programming': 4, 'product': None},
])
def test_visible_grader_distinguishes_zero_from_unavailable(answer):
    check_visible_answer('counts_product', answer)


def test_visible_grader_handles_arbitrary_precision_integer_without_float_conversion():
    check_visible_answer('ping', {'latency_ms': 10 ** 10000})


def test_weather_grader_normalizes_accented_city_with_country():
    check_visible_answer('weather', {'location': 'Chișinău, Moldova', 'temperature_c': 20})


def test_weather_grader_rejects_another_city():
    with pytest.raises(AssertionError, match='location'):
        check_visible_answer('weather', {'location': 'London', 'temperature_c': 20})


def test_live_grading_exception_records_failed_status(monkeypatch):
    options = {
        '--agent-live': True,
        '--agent-room': 'test',
        '--agent-bot': 'Bot',
        '--agent-url': 'ws://localhost',
        '--agent-live-timeout': 10,
        '--agent-prefix': '*',
    }
    request = SimpleNamespace(
        config=SimpleNamespace(getoption=options.__getitem__),
        node=SimpleNamespace(),
    )
    monkeypatch.setattr(live_tests, 'run_live_case', lambda **kwargs: {
        'mode': 'live', 'status': 'unverifiable', 'answer': {'latency_ms': 1},
    })

    def fail_grading(*args):
        raise RuntimeError('grader failed')

    monkeypatch.setattr(live_tests, 'check_visible_answer', fail_grading)

    with pytest.raises(RuntimeError, match='grader failed'):
        live_tests.test_connected_bot('ping', 'Check ping.', request)
    assert request.node.agent_result['status'] == 'failed'


@pytest.mark.parametrize('heartbeat_only', [False, True])
def test_real_websocket_connection_to_loopback_chat_server(heartbeat_only):
    """Exercise the actual websocket-client path without contacting Hack.Chat."""
    failures = []

    class Server(socketserver.StreamRequestHandler):
        def handle(self):
            self.connection.settimeout(3)
            try:
                headers = {}
                self.rfile.readline()  # GET request
                while (line := self.rfile.readline().strip()):
                    name, value = line.decode().split(':', 1)
                    headers[name.lower()] = value.strip()
                key = headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'
                accept = base64.b64encode(hashlib.sha1(key.encode()).digest())
                self.wfile.write(b'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ' + accept + b'\r\n\r\n')

                def receive():
                    head = self.rfile.read(2)
                    length = head[1] & 127
                    if length == 126:
                        length = struct.unpack('!H', self.rfile.read(2))[0]
                    elif length == 127:
                        length = struct.unpack('!Q', self.rfile.read(8))[0]
                    assert length < 4096
                    mask = self.rfile.read(4)
                    data = self.rfile.read(length)
                    return json.loads(bytes(byte ^ mask[i % 4] for i, byte in enumerate(data)))

                def send(event):
                    data = json.dumps(event).encode()
                    header = bytes([0x81, len(data)]) if len(data) < 126 else bytes([0x81, 126]) + struct.pack('!H', len(data))
                    self.wfile.write(header + data)

                assert receive()['cmd'] == 'join'
                send({'cmd': 'onlineSet', 'nicks': ['Bot']})
                prompt = receive()['text']
                marker = 'EVAL_' + prompt.split('EVAL_', 1)[1].split()[0]
                if heartbeat_only:
                    stop = time.monotonic() + 0.8
                    while time.monotonic() < stop:
                        self.wfile.write(b'\x89\x00')
                        time.sleep(0.01)
                else:
                    send({'cmd': 'chat', 'nick': 'Bot', 'text': marker + ' {"latency_ms":37}'})
            except Exception as error:
                if not heartbeat_only or not isinstance(error, OSError):
                    failures.append(error)

    with socketserver.TCPServer(('127.0.0.1', 0), Server) as server:
        thread = threading.Thread(target=server.handle_request, daemon=True)
        thread.start()
        kwargs = dict(url=f'ws://127.0.0.1:{server.server_address[1]}',
                      room='test', bot='Bot', prompt='Check ping.', timeout=0.15 if heartbeat_only else 3)
        started = time.monotonic()
        if heartbeat_only:
            with pytest.raises(TimeoutError):
                run_live_case(**kwargs)
            assert time.monotonic() - started < 0.6
        else:
            result = run_live_case(**kwargs)
        thread.join(timeout=4)
    assert not thread.is_alive()
    assert not failures
    if not heartbeat_only:
        assert result['answer'] == {'latency_ms': 37}


def test_observation_correlation_requires_exact_address_and_command_format():
    from .live import addressed_observation
    assert addressed_observation('@eval_a response time: 190 milliseconds', 'eval_a') == {'latency_ms': 190}
    assert addressed_observation('@other response time: 190 milliseconds', 'eval_a') == {}
    assert addressed_observation('I think @eval_a response time: 190 milliseconds', 'eval_a') == {}


def test_live_result_retains_addressed_ping_for_final_consistency_check():
    class MissingPingWire(ChatWire):
        def send(self, raw):
            event = json.loads(raw)
            if event['cmd'] == 'join':
                self.nick = event['nick']
                self.events.append({'cmd': 'onlineSet', 'nicks': ['Bot']})
            else:
                marker = 'EVAL_' + event['text'].split('EVAL_', 1)[1].split()[0]
                self.events.extend([
                    {'cmd': 'chat', 'nick': 'Bot', 'text': f'@{self.nick} response time: 190 milliseconds'},
                    {'cmd': 'chat', 'nick': 'Bot', 'text': marker + ' {"latency_ms":null}'},
                ])
    result = run(MissingPingWire())
    assert result['observations'] == {'latency_ms': 190}
    with pytest.raises(AssertionError, match='latency_ms'):
        live_tests.check_observed_values(result['answer'], result['observations'])
