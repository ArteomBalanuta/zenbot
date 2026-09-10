package observability

import (
	"context"
	"log/slog"
	"strings"
)

type requestKey struct{}
type stageKey struct{}

// Request identifies one agent turn without carrying user or provider payloads.
type Request struct {
	ID   string
	Mode string
	Room string
	Nick string
}

func WithRequest(ctx context.Context, request Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestKey{}, request)
}

func WithStage(ctx context.Context, stage string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stageKey{}, strings.TrimSpace(stage))
}

func Info(ctx context.Context, event string, attributes ...any) {
	log(ctx, slog.LevelInfo, event, nil, attributes...)
}

func Debug(ctx context.Context, event string, attributes ...any) {
	log(ctx, slog.LevelDebug, event, nil, attributes...)
}

func Error(ctx context.Context, event string, cause error, attributes ...any) {
	log(ctx, slog.LevelError, event, cause, attributes...)
}

func log(ctx context.Context, level slog.Level, event string, cause error, attributes ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	values := make([]any, 0, len(attributes)+12)
	if request, ok := ctx.Value(requestKey{}).(Request); ok {
		values = appendNonEmpty(values, "request_id", request.ID)
		values = appendNonEmpty(values, "mode", request.Mode)
		values = appendNonEmpty(values, "room", request.Room)
		values = appendNonEmpty(values, "nick", request.Nick)
	}
	if stage, ok := ctx.Value(stageKey{}).(string); ok {
		values = appendNonEmpty(values, "stage", stage)
	}
	values = append(values, safeAttributes(attributes...)...)
	if cause != nil {
		values = append(values, "error", cause.Error())
	}
	slog.Log(ctx, level, event, values...)
}

func appendNonEmpty(values []any, key, value string) []any {
	if value == "" {
		return values
	}
	return append(values, key, value)
}

func safeAttributes(attributes ...any) []any {
	values := make([]any, 0, len(attributes))
	for index := 0; index+1 < len(attributes); index += 2 {
		key, ok := attributes[index].(string)
		if !ok || sensitiveKey(key) {
			continue
		}
		values = append(values, key, attributes[index+1])
	}
	return values
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, accounting := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
		if normalized == accounting {
			return false
		}
	}
	for _, fragment := range []string{"api_key", "token", "secret", "password", "prompt", "arguments", "result", "content", "message", "history", "sql"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
