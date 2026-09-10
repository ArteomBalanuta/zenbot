// Package profiling captures latency evidence for regular command processing.
package profiling

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const excludedAgentCommand = "l"

type ingressKey struct{}
type traceKey struct{}

// Settings controls command and transport latency diagnostics.
type Settings struct {
	Enabled                bool
	SlowCommandThreshold   time.Duration
	SlowStageThreshold     time.Duration
	SlowTransportThreshold time.Duration
}

// Ingress records when a websocket frame entered and left the transport queue.
type Ingress struct {
	ReceivedAt time.Time
	DequeuedAt time.Time
	QueueDepth int
}

// Command is the non-sensitive metadata needed to start a command trace.
type Command struct {
	Prefix        string
	Text          string
	Room          string
	Nick          string
	Whisper       bool
	ParseDuration time.Duration
}

// Profiler creates request-local command traces.
type Profiler struct {
	settings Settings
	logger   *slog.Logger
	now      func() time.Time
	sequence atomic.Uint64
}

type stageTiming struct {
	name     string
	duration time.Duration
}

// Trace records ordered stages for one regular command.
type Trace struct {
	profiler   *Profiler
	id         string
	command    string
	canonical  string
	room       string
	nick       string
	whisper    bool
	receivedAt time.Time
	dequeuedAt time.Time
	startedAt  time.Time
	mu         sync.Mutex
	stages     []stageTiming
	finishOnce sync.Once
}

// New returns a profiler with production-safe latency thresholds.
func New(settings Settings, logger *slog.Logger) *Profiler {
	if settings.SlowCommandThreshold <= 0 {
		settings.SlowCommandThreshold = 250 * time.Millisecond
	}
	if settings.SlowStageThreshold <= 0 {
		settings.SlowStageThreshold = 25 * time.Millisecond
	}
	if settings.SlowTransportThreshold <= 0 {
		settings.SlowTransportThreshold = 25 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Profiler{settings: settings, logger: logger, now: time.Now}
}

// Enabled reports whether profiling is active.
func (p *Profiler) Enabled() bool { return p != nil && p.settings.Enabled }

// Active reports whether ctx carries a regular-command trace.
func Active(ctx context.Context) bool { return traceFrom(ctx) != nil }

// Run applies low-cardinality pprof labels while executing a regular command.
func Run(ctx context.Context, operation func(context.Context) error) (operationErr error) {
	trace := traceFrom(ctx)
	if trace == nil {
		return operation(ctx)
	}
	pprof.Do(ctx, pprof.Labels(
		"zenbot.command", trace.command,
		"zenbot.room", trace.room,
	), func(profiled context.Context) {
		operationErr = operation(profiled)
	})
	return operationErr
}

// ObserveInboundEnqueue reports transport backpressure when the websocket
// reader cannot promptly hand a frame to the bounded inbound channel.
func (p *Profiler) ObserveInboundEnqueue(wait time.Duration, queueDepth int) {
	if !p.Enabled() || wait < p.settings.SlowTransportThreshold {
		return
	}
	p.logger.Info("transport.profile.inbound_enqueue",
		"wait_ms", durationMS(wait),
		"queue_depth", queueDepth,
	)
}

// ObserveWrite separates time spent waiting for the serialized writer from
// time spent in the websocket write itself. Payload contents are never logged.
func (p *Profiler) ObserveWrite(lockWait, networkWrite time.Duration, bytes int, kind string, cause error) {
	if !p.Enabled() {
		return
	}
	total := nonNegative(lockWait) + nonNegative(networkWrite)
	if cause == nil && total < p.settings.SlowTransportThreshold {
		return
	}
	attrs := []any{
		"total_ms", durationMS(total),
		"lock_wait_ms", durationMS(lockWait),
		"network_write_ms", durationMS(networkWrite),
		"bytes", bytes,
		"kind", kind,
	}
	if cause != nil {
		attrs = append(attrs, "error", cause.Error())
	}
	p.logger.Info("transport.profile.write", attrs...)
}

// WithIngress adds transport timing without changing cancellation semantics.
func WithIngress(ctx context.Context, ingress Ingress) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ingressKey{}, ingress)
}

// StartCommand starts a trace for a prefixed command other than the direct l command.
func (p *Profiler) StartCommand(ctx context.Context, command Command) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	alias, ok := commandAlias(command.Text, command.Prefix)
	if !p.Enabled() || !ok || strings.EqualFold(alias, excludedAgentCommand) {
		return ctx, func(error) {}
	}
	startedAt := p.now()
	ingress, _ := ctx.Value(ingressKey{}).(Ingress)
	if ingress.DequeuedAt.IsZero() {
		ingress.DequeuedAt = startedAt
	}
	if ingress.ReceivedAt.IsZero() {
		ingress.ReceivedAt = ingress.DequeuedAt
	}
	trace := &Trace{
		profiler: p, id: fmt.Sprintf("cmd-%d", p.sequence.Add(1)), command: strings.ToLower(alias),
		room: command.Room, nick: command.Nick, whisper: command.Whisper,
		receivedAt: ingress.ReceivedAt, dequeuedAt: ingress.DequeuedAt, startedAt: startedAt,
	}
	trace.log("command.profile.started",
		"ws_queue_ms", durationMS(ingress.DequeuedAt.Sub(ingress.ReceivedAt)),
		"pre_trace_ms", durationMS(startedAt.Sub(ingress.DequeuedAt)),
		"parse_ms", durationMS(command.ParseDuration),
		"queue_depth", ingress.QueueDepth,
	)
	profiled := context.WithValue(ctx, traceKey{}, trace)
	return profiled, trace.finish
}

func commandAlias(text, prefix string) (string, bool) {
	text = strings.TrimSpace(text)
	if prefix == "" || !strings.HasPrefix(text, prefix) {
		return "", false
	}
	fields := strings.Fields(strings.TrimPrefix(text, prefix))
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

// Measure records one synchronous stage associated with the command in ctx.
func Measure(ctx context.Context, stage string) func() {
	trace := traceFrom(ctx)
	if trace == nil || strings.TrimSpace(stage) == "" {
		return func() {}
	}
	startedAt := trace.profiler.now()
	var once sync.Once
	return func() {
		once.Do(func() {
			duration := nonNegative(trace.profiler.now().Sub(startedAt))
			trace.mu.Lock()
			trace.stages = append(trace.stages, stageTiming{name: stage, duration: duration})
			trace.mu.Unlock()
			if duration >= trace.profiler.settings.SlowStageThreshold {
				trace.log("command.profile.stage", "stage", stage, "duration_ms", durationMS(duration))
			}
		})
	}
}

// SetCommandName records the canonical command selected by the registry.
func SetCommandName(ctx context.Context, canonical string) {
	if trace := traceFrom(ctx); trace != nil {
		trace.mu.Lock()
		trace.canonical = strings.TrimSpace(canonical)
		trace.mu.Unlock()
	}
}

func traceFrom(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceKey{}).(*Trace)
	return trace
}

func (t *Trace) finish(cause error) {
	t.finishOnce.Do(func() {
		finishedAt := t.profiler.now()
		total := nonNegative(finishedAt.Sub(t.receivedAt))
		processing := nonNegative(finishedAt.Sub(t.startedAt))
		t.mu.Lock()
		stages := append([]stageTiming(nil), t.stages...)
		canonical := t.canonical
		t.mu.Unlock()
		slowest := stageTiming{}
		for _, stage := range stages {
			if stage.duration > slowest.duration {
				slowest = stage
			}
		}
		attrs := []any{
			"total_ms", durationMS(total),
			"processing_ms", durationMS(processing),
			"stage_count", len(stages),
			"slow", total >= t.profiler.settings.SlowCommandThreshold,
		}
		if canonical != "" {
			attrs = append(attrs, "canonical", canonical)
		}
		if slowest.name != "" {
			attrs = append(attrs, "slowest_stage", slowest.name, "slowest_stage_ms", durationMS(slowest.duration))
		}
		if cause != nil {
			attrs = append(attrs, "error", cause.Error())
		}
		t.log("command.profile.completed", attrs...)
	})
}

func (t *Trace) log(event string, attrs ...any) {
	base := []any{
		"trace_id", t.id,
		"command", t.command,
		"room", t.room,
		"nick", t.nick,
		"whisper", t.whisper,
	}
	t.profiler.logger.Info(event, append(base, attrs...)...)
}

func durationMS(duration time.Duration) float64 {
	return float64(nonNegative(duration).Microseconds()) / 1000
}

func nonNegative(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}
