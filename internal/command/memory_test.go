package command

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"zenbot/internal/model"
)

func TestFormatMemoryReportUsesTruthfulGoLabelsAndExactMiBValues(t *testing.T) {
	stats := runtime.MemStats{
		Alloc:    0,
		HeapIdle: memoryMiB,
		HeapSys:  3*memoryMiB - 1,
		Sys:      4 * memoryMiB,
	}
	want := "Go Alloc: 0 MiB \nGo HeapIdle: 1 MiB \nGo HeapSys: 2 MiB \nGo Sys: 4 MiB \n"
	if got := formatMemoryReport(stats); got != want {
		t.Fatalf("formatMemoryReport() = %q, want %q", got, want)
	}
}

func TestMemoryAliasesRegisterAndRenderGoRuntimeReport(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"admin": {Name: "admin", Hash: "admin-hash"},
	}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"mem", "memory", "memstats"} {
		if _, ok := (*engine.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("alias %q was not registered", alias)
		}
	}

	definition, ok := commandDefinitionFor("memstats")
	if !ok {
		t.Fatal("missing memstats definition")
	}
	message := &model.ChatMessage{Name: "admin", Text: "!memstats ignored arguments", IsWhisper: true}
	status, err := definition.New(engine, message).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("Execute() = (%s, %v), want (%s, nil)", status, err, model.SUCCESSFUL)
	}
	if len(engine.chats) != 1 {
		t.Fatalf("reply count = %d, want 1: %v", len(engine.chats), engine.chats)
	}

	const prefix = "admin|"
	const suffix = "|true"
	chat := engine.chats[0]
	if !strings.HasPrefix(chat, prefix) || !strings.HasSuffix(chat, suffix) {
		t.Fatalf("reply = %q, want whisper to admin", chat)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(chat, prefix), suffix)
	report := regexp.MustCompile(`^Go Alloc: [0-9]+ MiB \nGo HeapIdle: [0-9]+ MiB \nGo HeapSys: [0-9]+ MiB \nGo Sys: [0-9]+ MiB \n$`)
	if !report.MatchString(payload) {
		t.Fatalf("report = %q, want Go runtime report", payload)
	}

	engine.chats = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err = definition.New(engine, message).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Execute() = (%s, %v), want (%s, context.Canceled)", status, err, model.FAILED)
	}
	if len(engine.chats) != 0 {
		t.Fatalf("canceled Execute() sent replies: %v", engine.chats)
	}
}
