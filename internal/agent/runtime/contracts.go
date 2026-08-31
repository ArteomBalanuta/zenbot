// Package runtime contains the private, unwired agent execution foundation.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
)

type Mode string

// InvocationMode is the source-compatible Saturn name.
type InvocationMode = Mode

const (
	DIRECT     Mode = "DIRECT"
	MENTION    Mode = "MENTION"
	AMBIENT    Mode = "AMBIENT"
	MODERATION Mode = "MODERATION"
)

func (m Mode) RequiresReply() bool { return m == DIRECT || m == MENTION }

type Capability string

const (
	DynamicSQL         Capability = "DYNAMIC_SQL"
	ModerationCommands Capability = "MODERATION_COMMANDS"
	PermanentBan       Capability = "PERMANENT_BAN"
	AdminCommands      Capability = "ADMIN_COMMANDS"
)

type Context struct {
	room, nick, trip, hash string
	whisper                bool
	roomUsers              []string
	capabilities           []Capability
	moderationTarget       string
}

func NewContext(room, nick, trip, hash string, whisper bool, roomUsers []string) Context {
	return NewContextWithCapabilities(room, nick, trip, hash, whisper, roomUsers, nil, "")
}

// NewContextWithCapabilities creates an immutable context carrying trusted capabilities.
// The legacy NewContext constructor remains unchanged and produces an empty capability set.
func NewContextWithCapabilities(room, nick, trip, hash string, whisper bool, roomUsers []string, capabilities []Capability, moderationTarget string) Context {
	return Context{
		room: room, nick: nick, trip: trip, hash: hash, whisper: whisper,
		roomUsers:        append([]string(nil), roomUsers...),
		capabilities:     append([]Capability(nil), capabilities...),
		moderationTarget: moderationTarget,
	}
}
func (c Context) Room() string               { return c.room }
func (c Context) Nick() string               { return c.nick }
func (c Context) Trip() string               { return c.trip }
func (c Context) Hash() string               { return c.hash }
func (c Context) Whisper() bool              { return c.whisper }
func (c Context) RoomUsers() []string        { return append([]string(nil), c.roomUsers...) }
func (c Context) Capabilities() []Capability { return append([]Capability(nil), c.capabilities...) }
func (c Context) HasCapability(capability Capability) bool {
	for _, item := range c.capabilities {
		if item == capability {
			return true
		}
	}
	return false
}
func (c Context) ModerationTarget() string { return c.moderationTarget }

// MemoryKey separates public history from whisper sessions and uses UTF-16
// length to remain compatible with the API contract.
func (c Context) MemoryKey() string {
	key := fmt.Sprintf("%d:%s", len(utf16.Encode([]rune(c.room))), c.room)
	if !c.whisper {
		return key + "|public"
	}
	if strings.TrimSpace(c.trip) != "" {
		return key + "|whisper|trip:" + c.trip
	}
	if strings.TrimSpace(c.hash) != "" {
		return key + "|whisper|hash:" + c.hash
	}
	return key + "|whisper|nick:" + c.nick
}

// Invocation is the immutable request handed to a Runner.
type Invocation struct {
	requestID, prompt, currentMessageText string
	context                               Context
	mode                                  Mode
	commandOriginated                     bool
	createdOn                             time.Time
}

type AgentInvocation = Invocation
type AgentContext = Context
type AgentResult = Result

func NewInvocation(requestID string, context Context, prompt string, mode Mode, currentMessageText string, commandOriginated bool) Invocation {
	return Invocation{requestID: requestID, context: context, prompt: prompt, mode: mode, currentMessageText: currentMessageText, commandOriginated: commandOriginated, createdOn: time.Now()}
}
func (i Invocation) RequestID() string          { return i.requestID }
func (i Invocation) Context() Context           { return i.context }
func (i Invocation) Prompt() string             { return i.prompt }
func (i Invocation) Mode() Mode                 { return i.mode }
func (i Invocation) CurrentMessageText() string { return i.currentMessageText }
func (i Invocation) CommandOriginated() bool    { return i.commandOriginated }
func (i Invocation) CreatedOn() time.Time       { return i.createdOn }

// Result is the runner's final outcome. ErrorCode is data, not provider policy.
type Result struct {
	correlationID, text, errorCode string
	shouldReply                    bool
}

func NewResult(correlationID, text string, shouldReply bool) Result {
	return Result{correlationID: correlationID, text: text, shouldReply: shouldReply}
}
func NewErrorResult(correlationID, errorCode string) Result {
	return Result{correlationID: correlationID, errorCode: errorCode}
}
func (r Result) CorrelationID() string { return r.correlationID }
func (r Result) Text() string          { return r.text }
func (r Result) ShouldReply() bool     { return r.shouldReply }
func (r Result) ErrorCode() string     { return r.errorCode }

// Runner performs one bounded invocation. Implementations must honor ctx.
type Runner interface {
	Run(ctx context.Context, invocation Invocation) (Result, error)
}

// InvocationFactory describes the source-grounded construction seam without wiring it to chat.
type InvocationFactory interface {
	Create(context Context, prompt string, mode Mode, currentMessageText string, commandOriginated bool) Invocation
}

// Sink receives only successful results marked for reply.
type Sink interface {
	Deliver(ctx context.Context, invocation Invocation, result Result) error
}

type RunnerFunc func(context.Context, Invocation) (Result, error)

func (f RunnerFunc) Run(ctx context.Context, invocation Invocation) (Result, error) {
	return f(ctx, invocation)
}

type SinkFunc func(context.Context, Invocation, Result) error

func (f SinkFunc) Deliver(ctx context.Context, invocation Invocation, result Result) error {
	return f(ctx, invocation, result)
}

func validateInvocation(i Invocation) error {
	if strings.TrimSpace(i.requestID) == "" {
		return errors.New("request id must not be blank")
	}
	if strings.TrimSpace(i.context.room) == "" {
		return errors.New("room must not be blank")
	}
	if strings.TrimSpace(i.prompt) == "" {
		return errors.New("prompt must not be blank")
	}
	switch i.mode {
	case DIRECT, MENTION, AMBIENT, MODERATION:
	default:
		return errors.New("invalid invocation mode")
	}
	return nil
}
