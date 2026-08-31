package core

import "testing"

func TestSendWhisperMessageNormalizesNewlinesAndEnqueuesEscapedJSON(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	got, err := e.SendWhisperMessage("alice", `first\nsecond`)
	if err != nil {
		t.Fatal(err)
	}
	want := "/whisper @alice first\nsecond"
	if got != want {
		t.Fatalf("returned=%q, want %q", got, want)
	}
	if queued := <-e.OutMessageQueue; queued != `{ "cmd": "chat", "text": "/whisper @alice first\nsecond"}` {
		t.Fatalf("queued=%q", queued)
	}
}

func TestSendWhisperMessageHasNoDotSeparator(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	got, err := e.SendWhisperMessage("alice", "payload")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/whisper @alice payload" {
		t.Fatalf("returned=%q", got)
	}
}

func TestSendAddressedMessageNormalizesAndFormatsPublicOutput(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	got, err := e.SendAddressedMessage("alice", "first\r\nsecond\\nthird", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "@alice first\nsecond\nthird"
	if got != want {
		t.Fatalf("returned=%q, want %q", got, want)
	}
	if queued := <-e.OutMessageQueue; queued != `{ "cmd": "chat", "text": "@alice first\nsecond\nthird"}` {
		t.Fatalf("queued=%q", queued)
	}
}

func TestSendAddressedMessageNormalizesAndFormatsWhisperOutput(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	got, err := e.SendAddressedMessage("alice", "first\\nsecond", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/whisper @alice first\nsecond" {
		t.Fatalf("returned=%q", got)
	}
}
