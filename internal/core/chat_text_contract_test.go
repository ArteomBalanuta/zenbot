package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// Changing text before serialization, or serializing it twice, must fail this
// test: the peer's decoded text and the sender's receipt must both be exact.
func TestChatSendersPreservePlainTextAtJSONBoundary(t *testing.T) {
	text := "real\nnewline; literal \\n; quote \"; path C:\\new; CR\rLF\r\nTAB\tNUL\x00; Chișinău 🌦️"
	for _, tc := range []struct {
		name   string
		send   func(*EngineImpl, string) (string, error)
		prefix string
	}{
		{"public", func(e *EngineImpl, s string) (string, error) { return e.SendChatMessage("", s, false) }, ""},
		{"addressed-chat", func(e *EngineImpl, s string) (string, error) { return e.SendChatMessage("merc", s, false) }, "@merc "},
		{"chat-whisper", func(e *EngineImpl, s string) (string, error) { return e.SendChatMessage("merc", s, true) }, "/whisper @merc .\n"},
		{"whisper", func(e *EngineImpl, s string) (string, error) { return e.SendWhisperMessage("merc", s) }, "/whisper @merc "},
		{"addressed", func(e *EngineImpl, s string) (string, error) { return e.SendAddressedMessage("merc", s, false) }, "@merc "},
		{"addressed-whisper", func(e *EngineImpl, s string) (string, error) { return e.SendAddressedMessage("merc", s, true) }, "/whisper @merc "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
			receipt, err := tc.send(e, text)
			if err != nil {
				t.Fatal(err)
			}
			frame := <-e.OutMessageQueue
			if strings.ContainsAny(frame, "\n\r\t\x00") {
				t.Fatalf("unescaped control in frame: %q", frame)
			}
			var decoded struct {
				Cmd  string `json:"cmd"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(frame), &decoded); err != nil {
				t.Fatal(err)
			}
			want := tc.prefix + text
			if decoded.Cmd != "chat" || decoded.Text != want || receipt != want {
				t.Fatalf("receipt=%q decoded=%q want=%q", receipt, decoded.Text, want)
			}
		})
	}
}
