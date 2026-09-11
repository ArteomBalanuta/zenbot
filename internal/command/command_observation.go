package command

import "encoding/json"

// commandDataObserver receives a command-produced, bounded source snapshot,
// before acknowledgment can fail. The visibility flag belongs to the command;
// the observer also checks the trusted invocation visibility.
type commandDataObserver interface {
	ObserveCommandData(json.RawMessage, bool)
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

func (e *agentCaptureEngine) ObserveCommandData(data json.RawMessage, whisper bool) {
	if !json.Valid(data) || (whisper && !e.invocationWhisper) {
		return
	}
	e.data = append(json.RawMessage(nil), data...)
	e.dataObserved = true
}
