package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// InvocationMode is the closed set of Saturn invocation modes.
type InvocationMode string

const (
	DIRECT     InvocationMode = "DIRECT"
	MENTION    InvocationMode = "MENTION"
	AMBIENT    InvocationMode = "AMBIENT"
	MODERATION InvocationMode = "MODERATION"
)

func (m InvocationMode) Valid() bool {
	switch m {
	case DIRECT, MENTION, AMBIENT, MODERATION:
		return true
	default:
		return false
	}
}
func (m InvocationMode) RequiresReply() bool { return m == DIRECT || m == MENTION }

// Capability is an extensible capability name. Membership is exact set membership.
type Capability string

const (
	DynamicSQL         Capability = "DYNAMIC_SQL"
	ModerationCommands Capability = "MODERATION_COMMANDS"
	PermanentBan       Capability = "PERMANENT_BAN"
	AdminCommands      Capability = "ADMIN_COMMANDS"
)

// Context is the exact eight-component Saturn context contract.
type Context struct {
	room, nick       string
	trip, hash       *string
	whisper          bool
	roomUsers        []string
	capabilities     []Capability
	moderationTarget *string
}

func NewContext(room, nick, trip, hash string, whisper bool, roomUsers []string) (Context, error) {
	return newContext(room, nick, trip, hash, whisper, roomUsers, []Capability{}, nil)
}
func NewContextWithCapabilities(room, nick, trip, hash string, whisper bool, roomUsers []string, capabilities []Capability, moderationTarget ...string) (Context, error) {
	var target *string
	if len(moderationTarget) > 1 {
		return Context{}, errors.New("moderation target accepts at most one value")
	}
	if len(moderationTarget) == 1 {
		target = &moderationTarget[0]
	}
	return newContext(room, nick, trip, hash, whisper, roomUsers, capabilities, target)
}
func NewContextWithModerationTarget(room, nick, trip, hash string, whisper bool, roomUsers []string, capabilities []Capability, moderationTarget string) (Context, error) {
	return newContext(room, nick, trip, hash, whisper, roomUsers, capabilities, &moderationTarget)
}
func newContext(room, nick, trip, hash string, whisper bool, roomUsers []string, capabilities []Capability, target *string) (Context, error) {
	if roomUsers == nil {
		return Context{}, errors.New("room users must not be nil")
	}
	if capabilities == nil {
		return Context{}, errors.New("capabilities must not be nil")
	}
	users := make([]string, len(roomUsers))
	copy(users, roomUsers)
	caps := make([]Capability, 0, len(capabilities))
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		caps = append(caps, capability)
	}
	return Context{room: room, nick: nick, trip: nullableString(trip), hash: nullableString(hash), whisper: whisper,
		roomUsers: users, capabilities: caps, moderationTarget: cloneString(target)}, nil
}
func (c Context) Room() string  { return c.room }
func (c Context) Nick() string  { return c.nick }
func (c Context) Trip() *string { return cloneString(c.trip) }
func (c Context) Hash() *string { return cloneString(c.hash) }
func (c Context) Whisper() bool { return c.whisper }
func (c Context) RoomUsers() []string {
	v := make([]string, len(c.roomUsers))
	copy(v, c.roomUsers)
	return v
}
func (c Context) Capabilities() []Capability {
	v := make([]Capability, len(c.capabilities))
	copy(v, c.capabilities)
	return v
}
func (c Context) HasCapability(capability Capability) bool {
	for _, v := range c.capabilities {
		if v == capability {
			return true
		}
	}
	return false
}
func (c Context) ModerationTarget() *string { return cloneString(c.moderationTarget) }
func (c Context) MemoryKey() string {
	key := fmt.Sprintf("%d:%s", len(utf16.Encode([]rune(c.room))), c.room)
	if !c.whisper {
		return key + "|public"
	}
	if c.trip != nil && strings.TrimSpace(*c.trip) != "" {
		return key + "|whisper|trip:" + *c.trip
	}
	if c.hash != nil && strings.TrimSpace(*c.hash) != "" {
		return key + "|whisper|hash:" + *c.hash
	}
	return key + "|whisper|nick:" + c.nick
}
func stringPtr(s string) *string { return &s }
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func cloneString(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

// Invocation is the exact Saturn invocation record.
type Invocation struct {
	requestID, prompt  string
	context            Context
	mode               InvocationMode
	currentMessageText *string
	commandOriginated  bool
}
type AgentInvocation = Invocation
type AgentContext = Context

// NewInvocation mirrors Saturn's overloads: (context,prompt), then request ID,
// with optional mode, current message text, and command-origin flag.
func NewInvocation(args ...any) (Invocation, error) {
	if len(args) == 2 {
		ctx, ok := args[0].(Context)
		if !ok {
			return Invocation{}, errors.New("context must be api.Context")
		}
		prompt, ok := args[1].(string)
		if !ok {
			return Invocation{}, errors.New("prompt must be string")
		}
		return newInvocation("", ctx, prompt, DIRECT, nil, false, true)
	}
	if len(args) == 3 {
		if ctx, ok := args[0].(Context); ok {
			prompt, promptOK := args[1].(string)
			mode, modeOK := args[2].(InvocationMode)
			if !promptOK || !modeOK {
				return Invocation{}, errors.New("generated invocation requires prompt and mode")
			}
			return newInvocation("", ctx, prompt, mode, nil, false, true)
		}
	}
	if len(args) < 3 || len(args) > 6 {
		return Invocation{}, errors.New("invalid invocation constructor arity")
	}
	requestID, ok := args[0].(string)
	if !ok {
		return Invocation{}, errors.New("request ID must be string")
	}
	ctx, ok := args[1].(Context)
	if !ok {
		return Invocation{}, errors.New("context must be api.Context")
	}
	prompt, ok := args[2].(string)
	if !ok {
		return Invocation{}, errors.New("prompt must be string")
	}
	mode := DIRECT
	if len(args) >= 4 {
		mode, ok = args[3].(InvocationMode)
		if !ok {
			return Invocation{}, errors.New("mode must be api.InvocationMode")
		}
	}
	var current *string
	if len(args) >= 5 && args[4] != nil {
		v, ok := args[4].(string)
		if !ok {
			return Invocation{}, errors.New("current message text must be string")
		}
		current = &v
	}
	command := false
	if len(args) == 6 {
		command, ok = args[5].(bool)
		if !ok {
			return Invocation{}, errors.New("command originated must be bool")
		}
	}
	return newInvocation(requestID, ctx, prompt, mode, current, command, false)
}
func newInvocation(id string, ctx Context, prompt string, mode InvocationMode, current *string, command, generated bool) (Invocation, error) {
	if generated {
		id = newUUID()
	}
	v := Invocation{requestID: id, context: ctx, prompt: prompt, mode: mode, currentMessageText: cloneString(current), commandOriginated: command}
	if err := v.Validate(); err != nil {
		return Invocation{}, err
	}
	return v, nil
}
func (i Invocation) RequestID() string           { return i.requestID }
func (i Invocation) Context() Context            { return i.context }
func (i Invocation) Prompt() string              { return i.prompt }
func (i Invocation) Mode() InvocationMode        { return i.mode }
func (i Invocation) CurrentMessageText() *string { return cloneString(i.currentMessageText) }
func (i Invocation) CommandOriginated() bool     { return i.commandOriginated }
func (i Invocation) Validate() error {
	if javaStrip(i.requestID) == "" {
		return errors.New("request ID must not be blank")
	}
	if javaStrip(i.prompt) == "" {
		return errors.New("prompt must not be blank")
	}
	if !i.mode.Valid() {
		return fmt.Errorf("invalid invocation mode %q", i.mode)
	}
	return nil
}
func ValidateInvocation(i Invocation) error { return i.Validate() }

func (i Invocation) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RequestID string         `json:"requestId"`
		Context   Context        `json:"context"`
		Prompt    string         `json:"prompt"`
		Mode      InvocationMode `json:"mode"`
		Current   *string        `json:"currentMessageText"`
		Command   bool           `json:"commandOriginated"`
	}{i.requestID, i.context, i.prompt, i.mode, i.currentMessageText, i.commandOriginated})
}
