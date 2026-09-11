package command

import (
	"context"
	"encoding/json"
)

// commandDataObserver receives a command-produced, bounded source snapshot,
// before acknowledgment can fail. The visibility flag belongs to the command;
// the observer also checks the trusted invocation visibility.
type commandDataObserver interface {
	ObserveCommandData(json.RawMessage, bool)
}

type commandTextObservation struct {
	Text string `json:"text"`
}

func observeCommandData(c *commandBase, data any, whisper bool) {
	observer, ok := c.engine.(commandDataObserver)
	if !ok {
		return
	}
	encoded, err := json.Marshal(data)
	if err == nil {
		observer.ObserveCommandData(encoded, whisper)
	}
}

// observeAndReply records source text before the context or outward delivery
// can fail. Callers choose whisper from the actual output channel rather than
// the invocation alone.
func observeAndReply(ctx context.Context, c *commandBase, text string, whisper bool) error {
	observeCommandData(c, commandTextObservation{Text: text}, whisper)
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := c.engine.SendChatMessage(c.message.Name, text, whisper)
	return err
}

func (e *agentCaptureEngine) ObserveCommandData(data json.RawMessage, whisper bool) {
	if !json.Valid(data) || (whisper && !e.invocationWhisper) {
		return
	}
	e.data = append(json.RawMessage(nil), data...)
	e.dataObserved = true
}
