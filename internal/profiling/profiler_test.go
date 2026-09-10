package profiling

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type capturedRecord struct {
	event string
	attrs map[string]any
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (*captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, capturedRecord{event: record.Message, attrs: attrs})
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) snapshot() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]capturedRecord(nil), h.records...)
}

func TestProfilerTracesRegularCommandCriticalPath(t *testing.T) {
	base := time.Unix(100, 0)
	clock := newSequenceClock(
		base.Add(25*time.Millisecond),
		base.Add(30*time.Millisecond),
		base.Add(42*time.Millisecond),
		base.Add(60*time.Millisecond),
	)
	handler := &captureHandler{}
	profiler := New(Settings{
		Enabled:              true,
		SlowCommandThreshold: 30 * time.Millisecond,
		SlowStageThreshold:   10 * time.Millisecond,
	}, slog.New(handler))
	profiler.now = clock

	ctx := WithIngress(context.Background(), Ingress{
		ReceivedAt: base,
		DequeuedAt: base.Add(20 * time.Millisecond),
		QueueDepth: 7,
	})
	ctx, finish := profiler.StartCommand(ctx, Command{
		Prefix:        "*",
		Text:          "*ping now",
		Room:          "programming",
		Nick:          "alice",
		ParseDuration: 5 * time.Millisecond,
	})
	stageDone := Measure(ctx, "handler.audit_chat_message")
	stageDone()
	finish(nil)

	records := handler.snapshot()
	if len(records) != 3 {
		t.Fatalf("records=%#v, want started, slow stage, and completed", records)
	}
	if records[0].event != "command.profile.started" || records[0].attrs["command"] != "ping" || records[0].attrs["ws_queue_ms"] != float64(20) || records[0].attrs["parse_ms"] != float64(5) || records[0].attrs["queue_depth"] != int64(7) {
		t.Fatalf("started record=%#v", records[0])
	}
	if records[1].event != "command.profile.stage" || records[1].attrs["stage"] != "handler.audit_chat_message" || records[1].attrs["duration_ms"] != float64(12) {
		t.Fatalf("stage record=%#v", records[1])
	}
	if records[2].event != "command.profile.completed" || records[2].attrs["total_ms"] != float64(60) || records[2].attrs["processing_ms"] != float64(35) || records[2].attrs["slow"] != true || records[2].attrs["slowest_stage"] != "handler.audit_chat_message" {
		t.Fatalf("completed record=%#v", records[2])
	}
}

func TestProfilerExcludesDirectAgentCommand(t *testing.T) {
	handler := &captureHandler{}
	profiler := New(Settings{Enabled: true}, slog.New(handler))

	ctx, finish := profiler.StartCommand(context.Background(), Command{Prefix: "*", Text: "*l explain this"})
	Measure(ctx, "handler.audit")()
	finish(nil)

	if records := handler.snapshot(); len(records) != 0 {
		t.Fatalf("direct agent command emitted profiling records: %#v", records)
	}
}

func TestProfilerIgnoresOrdinaryChat(t *testing.T) {
	handler := &captureHandler{}
	profiler := New(Settings{Enabled: true}, slog.New(handler))

	ctx, finish := profiler.StartCommand(context.Background(), Command{Prefix: "*", Text: "hello room"})
	Measure(ctx, "handler.audit")()
	finish(nil)

	if records := handler.snapshot(); len(records) != 0 {
		t.Fatalf("ordinary chat emitted command profiling records: %#v", records)
	}
}

func TestProfilerReportsSlowTransportOperationsWithoutPayloadData(t *testing.T) {
	handler := &captureHandler{}
	profiler := New(Settings{Enabled: true, SlowTransportThreshold: 10 * time.Millisecond}, slog.New(handler))

	profiler.ObserveInboundEnqueue(15*time.Millisecond, 32)
	profiler.ObserveWrite(4*time.Millisecond, 8*time.Millisecond, 128, "text", errors.New("deadline exceeded"))

	records := handler.snapshot()
	if len(records) != 2 {
		t.Fatalf("records=%#v, want inbound enqueue and write", records)
	}
	if records[0].event != "transport.profile.inbound_enqueue" || records[0].attrs["wait_ms"] != float64(15) || records[0].attrs["queue_depth"] != int64(32) {
		t.Fatalf("inbound record=%#v", records[0])
	}
	if records[1].event != "transport.profile.write" || records[1].attrs["lock_wait_ms"] != float64(4) || records[1].attrs["network_write_ms"] != float64(8) || records[1].attrs["bytes"] != int64(128) || records[1].attrs["kind"] != "text" || records[1].attrs["error"] != "deadline exceeded" {
		t.Fatalf("write record=%#v", records[1])
	}
	if _, leaked := records[1].attrs["payload"]; leaked {
		t.Fatalf("transport record leaked payload metadata: %#v", records[1])
	}
}

func TestProfilerSuppressesFastSuccessfulTransportOperations(t *testing.T) {
	handler := &captureHandler{}
	profiler := New(Settings{Enabled: true, SlowTransportThreshold: 10 * time.Millisecond}, slog.New(handler))

	profiler.ObserveInboundEnqueue(time.Millisecond, 1)
	profiler.ObserveWrite(time.Millisecond, time.Millisecond, 12, "ping", nil)

	if records := handler.snapshot(); len(records) != 0 {
		t.Fatalf("fast transport operations emitted profiling records: %#v", records)
	}
}

func newSequenceClock(times ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if index >= len(times) {
			panic("test clock exhausted")
		}
		value := times[index]
		index++
		return value
	}
}
