package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c Context) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Room             string       `json:"room"`
		Nick             string       `json:"nick"`
		Trip             *string      `json:"trip"`
		Hash             *string      `json:"hash"`
		Whisper          bool         `json:"whisper"`
		RoomUsers        []string     `json:"roomUsers"`
		Capabilities     []Capability `json:"capabilities"`
		ModerationTarget *string      `json:"moderationTarget"`
	}{c.room, c.nick, c.trip, c.hash, c.whisper, append([]string(nil), c.roomUsers...), append([]Capability(nil), c.capabilities...), c.moderationTarget})
}
func (c *Context) UnmarshalJSON(data []byte) error {
	var w struct {
		Room             *string       `json:"room"`
		Nick             *string       `json:"nick"`
		Trip             *string       `json:"trip"`
		Hash             *string       `json:"hash"`
		Whisper          bool          `json:"whisper"`
		RoomUsers        *[]string     `json:"roomUsers"`
		Capabilities     *[]Capability `json:"capabilities"`
		ModerationTarget *string       `json:"moderationTarget"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.Room == nil || w.Nick == nil || w.RoomUsers == nil || w.Capabilities == nil {
		return errors.New("context required field missing or null")
	}
	v, err := newContext(*w.Room, *w.Nick, valueOrEmpty(w.Trip), valueOrEmpty(w.Hash), w.Whisper, *w.RoomUsers, *w.Capabilities, w.ModerationTarget)
	if err != nil {
		return err
	}
	v.trip = cloneString(w.Trip)
	v.hash = cloneString(w.Hash)
	*c = v
	return nil
}
func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (i *Invocation) UnmarshalJSON(data []byte) error {
	var w struct {
		RequestID *string         `json:"requestId"`
		Context   *Context        `json:"context"`
		Prompt    *string         `json:"prompt"`
		Mode      *InvocationMode `json:"mode"`
		Current   *string         `json:"currentMessageText"`
		Command   *bool           `json:"commandOriginated"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.RequestID == nil || w.Context == nil || w.Prompt == nil || w.Mode == nil || w.Command == nil {
		return errors.New("invocation required field missing or null")
	}
	v := Invocation{requestID: *w.RequestID, context: *w.Context, prompt: *w.Prompt, mode: *w.Mode, currentMessageText: w.Current, commandOriginated: *w.Command}
	if err := v.Validate(); err != nil {
		return err
	}
	*i = v
	return nil
}

// Result is the exact Saturn three-component result.
type Result struct {
	correlationID, content string
	shouldReply            bool
}
type AgentResult = Result

func NewResult(correlationID, content string, shouldReply ...bool) (Result, error) {
	reply := true
	if len(shouldReply) > 1 {
		return Result{}, errors.New("shouldReply accepts at most one value")
	}
	if len(shouldReply) == 1 {
		reply = shouldReply[0]
	}
	r := Result{correlationID: correlationID, content: content, shouldReply: reply}
	if err := r.Validate(); err != nil {
		return Result{}, err
	}
	return r, nil
}
func Reply(correlationID, content string) (Result, error) {
	return NewResult(correlationID, content, true)
}
func Silent(correlationID string) (Result, error) { return NewResult(correlationID, "", false) }
func (r Result) CorrelationID() string            { return r.correlationID }
func (r Result) Content() string                  { return r.content }
func (r Result) ShouldReply() bool                { return r.shouldReply }
func (r Result) Validate() error {
	if strings.TrimSpace(r.correlationID) == "" {
		return errors.New("correlation ID must not be blank")
	}
	return nil
}
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		CorrelationID string `json:"correlationId"`
		Content       string `json:"content"`
		ShouldReply   bool   `json:"shouldReply"`
	}{r.correlationID, r.content, r.shouldReply})
}
func (r *Result) UnmarshalJSON(data []byte) error {
	var w struct {
		CorrelationID *string `json:"correlationId"`
		Content       *string `json:"content"`
		ShouldReply   *bool   `json:"shouldReply"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.CorrelationID == nil || w.Content == nil || w.ShouldReply == nil {
		return errors.New("result required field missing or null")
	}
	v := Result{correlationID: *w.CorrelationID, content: *w.Content, shouldReply: *w.ShouldReply}
	if err := v.Validate(); err != nil {
		return err
	}
	*r = v
	return nil
}
