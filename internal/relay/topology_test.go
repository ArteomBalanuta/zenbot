package relay

import (
	"context"
	"errors"
	"testing"
)

type recordingSender struct {
	author  string
	text    string
	whisper bool
	calls   int
	err     error
}

func (s *recordingSender) SendChatMessage(author, text string, whisper bool) (string, error) {
	s.author, s.text, s.whisper = author, text, whisper
	s.calls++
	return text, s.err
}

func TestHostRelayDeliversExactPublicAgentTextOnce(t *testing.T) {
	sender := &recordingSender{}
	host := NewHost(sender)
	text := "first\nquote: \" backslash: \\ non-ASCII: Ž"
	if err := host.RelayAgentMessage(context.Background(), "alice", text); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || sender.author != "" || sender.text != "alice: "+text || sender.whisper {
		t.Fatalf("calls=%d author=%q text=%q whisper=%v", sender.calls, sender.author, sender.text, sender.whisper)
	}
}

func TestHostRelayReturnsTransportErrorWithoutRetry(t *testing.T) {
	want := errors.New("transport down")
	sender := &recordingSender{err: want}
	if err := NewHost(sender).RelayAgentMessage(context.Background(), "alice", "hello"); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
	if sender.calls != 1 {
		t.Fatalf("calls=%d, want 1", sender.calls)
	}
}

func TestHostRelayDoesNotSendAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sender := &recordingSender{}
	if err := NewHost(sender).RelayAgentMessage(ctx, "alice", "hello"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("calls=%d, want 0", sender.calls)
	}
}
