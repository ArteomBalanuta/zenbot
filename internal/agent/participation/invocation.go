package participation

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"zenbot/internal/agent/api"
	"zenbot/internal/model"
)

type Role string

const (
	RoleAdmin     Role = "ADMIN"
	RoleModerator Role = "MODERATOR"
)

type TrustedSnapshot struct {
	Room        string
	Users       []string
	CreatorTrip string
	AdminTrips  []string
	Roles       map[string]Role
}
type InvocationFactory struct{ Snapshot func() TrustedSnapshot }

func NewInvocationFactory(snapshot func() TrustedSnapshot) *InvocationFactory {
	return &InvocationFactory{Snapshot: snapshot}
}
func (f *InvocationFactory) Create(s TrustedSnapshot, message model.ChatMessage, prompt string, mode api.InvocationMode, commandOriginated bool) (api.Invocation, error) {
	caps := []api.Capability{}
	creator := s.CreatorTrip != "" && s.CreatorTrip == message.Trip
	admin := contains(s.AdminTrips, message.Trip) || (s.Roles != nil && s.Roles[message.Trip] == RoleAdmin)
	if creator || (admin && mode != api.AMBIENT && mode != api.MODERATION) {
		caps = append(caps, api.DynamicSQL)
	}
	moderator := s.Roles != nil && s.Roles[message.Trip] == RoleModerator
	if creator || ((admin || moderator) && mode != api.AMBIENT && mode != api.MODERATION) {
		caps = append(caps, api.ModerationCommands)
	}
	if creator && mode == api.DIRECT {
		caps = append(caps, api.PermanentBan, api.AdminCommands)
	}
	whisper := message.Whisper || message.IsWhisper
	ctx, e := api.NewContextWithCapabilities(s.Room, message.Name, message.Trip, message.Hash, whisper, append([]string(nil), s.Users...), caps)
	if e != nil {
		return api.Invocation{}, e
	}
	id := requestID()
	return api.NewInvocation(id, ctx, prompt, mode, message.Text, commandOriginated)
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func requestID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return fmt.Sprintf("request-%d", len(strings.TrimSpace(string(b))))
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Submitter interface{ Submit(api.Invocation) error }
type Decision string

const (
	Pass    Decision = "PASS"
	Claimed Decision = "CLAIMED"
)

type Event struct {
	Message             model.ChatMessage
	Snapshot            TrustedSnapshot
	BotNick             string
	Prefix              string
	AmbientEnabled      bool
	AmbientEvery        uint64
	EligibleCount       uint64
	ModerationCandidate bool
}
type Pipeline struct {
	Factory *InvocationFactory
	Quiet   *QuietRegistry
	Parser  MentionParser
	Submit  Submitter
	// Monitor observes eligible/ineligible events for moderation telemetry. It
	// never claims the event and runs before the eligibility filter.
	Monitor func(Event)
}
type Outcome struct {
	Decision  Decision
	Submitted bool
	Mode      api.InvocationMode
	Err       error
}

func (p *Pipeline) Handle(e Event) Outcome {
	if p.Monitor != nil {
		p.Monitor(e)
	}
	text := strings.TrimSpace(e.Message.Text)
	if text == "" || e.Message.Whisper || e.Message.IsWhisper || strings.EqualFold(e.Message.Name, e.BotNick) || isConventionalBot(e.Message.Name) || (e.Prefix != "" && strings.HasPrefix(text, e.Prefix)) {
		return Outcome{Decision: Pass}
	}
	ctx, e1 := api.NewContext(e.Snapshot.Room, e.Message.Name, e.Message.Trip, e.Message.Hash, false, e.Snapshot.Users)
	if e1 != nil {
		return Outcome{Decision: Pass, Err: e1}
	}
	if p.Quiet != nil && p.Quiet.IsPoliteQuietRequest(text, e.BotNick) {
		p.Quiet.Silence(ctx)
		return Outcome{Decision: Pass}
	}
	if prompt, ok := p.Parser.Parse(text, e.BotNick); ok {
		return p.submit(e, e.Snapshot, prompt, api.MENTION, true, Claimed)
	}
	if e.ModerationCandidate {
		return p.submit(e, e.Snapshot, text, api.MODERATION, false, Pass)
	}
	if e.AmbientEnabled && e.AmbientEvery > 0 && e.EligibleCount%e.AmbientEvery == 0 {
		return p.submit(e, e.Snapshot, text, api.AMBIENT, false, Pass)
	}
	return Outcome{Decision: Pass}
}
func isConventionalBot(name string) bool {
	return regexp.MustCompile(`(?i)^(?:bot(?:[_-]?\d+)?|[\p{L}\p{N}_-]*(?:Bot|[_-]bot)(?:[_-]?\d+)?)$`).MatchString(name)
}
func (p *Pipeline) submit(e Event, s TrustedSnapshot, prompt string, mode api.InvocationMode, origin bool, d Decision) Outcome {
	if p.Submit == nil || p.Factory == nil {
		return Outcome{Decision: d, Mode: mode, Err: fmt.Errorf("agent submission is unsupported")}
	}
	inv, err := p.Factory.Create(s, e.Message, prompt, mode, origin)
	if err != nil {
		return Outcome{Decision: d, Mode: mode, Err: err}
	}
	err = p.Submit.Submit(inv)
	return Outcome{Decision: d, Submitted: err == nil, Mode: mode, Err: err}
}
