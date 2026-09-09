package participation

import (
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/model"
)

func TestSemanticModerationSignalMatchesOnlySourceSevereAbuseCandidates(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"kys", "KYS", true},
		{"kill yourself", "kill yourself", true},
		{"kill urself", "kill urself", true},
		{"kill u", "kill u", true},
		{"kill you", "kill you", true},
		{"hang yourself", "hang yourself", true},
		{"hang urself", "hang urself", true},
		{"dox", "I will dox you", true},
		{"doxx", "I will doxx you", true},
		{"swat", "I will swat you", true},
		{"swatting", "I am swatting you", true},
		{"rape", "I will rape you", true},
		{"shoot", "I will shoot you", true},
		{"stab", "I will stab you", true},
		{"bomb", "bomb the room", true},
		{"ordinary", "please pass the salt", false},
		{"word boundary", "doxxing", false},
		// Saturn compiles with Java's Unicode character classes. A Unicode
		// letter immediately before the candidate is not a word boundary.
		{"unicode word boundary", "猫doxx", false},
		{"near miss", "kill myself", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SemanticModerationCandidate(model.ChatMessage{Text: tc.text}); got != tc.want {
				t.Fatalf("SemanticModerationCandidate(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestPipelineCreatesBotModerationContextAndContinuesToAmbient(t *testing.T) {
	submitter := &recordingPipelineSubmitter{}
	p := Pipeline{
		Factory:                 NewInvocationFactory(nil),
		Parser:                  MentionParser{},
		Submit:                  submitter,
		SemanticModerationReady: true,
	}
	out := p.Handle(Event{
		Message:             model.ChatMessage{Name: "alleged-author", Trip: "author-trip", Hash: "author-hash", Text: "I will doxx you"},
		Snapshot:            TrustedSnapshot{Room: "room", Users: []string{"alleged-author", "other"}, CreatorTrip: "creator"},
		BotNick:             "bot",
		ModerationCandidate: true,
		ModerationTarget:    "alleged-author",
		AmbientEnabled:      true,
		AmbientEvery:        1,
	})
	if out.Decision != Pass || !out.Submitted || out.Mode != api.AMBIENT || out.Err != nil {
		t.Fatalf("outcome = %+v", out)
	}
	if len(submitter.invocations) != 2 {
		t.Fatalf("invocations = %d, want moderation then ambient", len(submitter.invocations))
	}
	moderation := submitter.invocations[0]
	if moderation.Mode() != api.MODERATION || moderation.CommandOriginated() || moderation.Prompt() != "I will doxx you" || moderation.CurrentMessageText() == nil || *moderation.CurrentMessageText() != "I will doxx you" {
		t.Fatalf("moderation invocation = %#v", moderation)
	}
	ctx := moderation.Context()
	if ctx.Nick() != "bot" || ctx.Trip() == nil || *ctx.Trip() != "creator" || ctx.Hash() != nil || ctx.Whisper() || !ctx.HasCapability(api.ModerationCommands) || len(ctx.Capabilities()) != 1 || ctx.ModerationTarget() == nil || *ctx.ModerationTarget() != "alleged-author" {
		t.Fatalf("moderation context = %#v", ctx)
	}
	if ambient := submitter.invocations[1]; ambient.Mode() != api.AMBIENT || ambient.Context().Nick() != "alleged-author" {
		t.Fatalf("ambient invocation = %#v", ambient)
	}
}

func TestPipelineKeepsSemanticModerationFailClosedUntilReady(t *testing.T) {
	submitter := &recordingPipelineSubmitter{}
	p := Pipeline{Factory: NewInvocationFactory(nil), Parser: MentionParser{}, Submit: submitter}
	out := p.Handle(Event{
		Message:             model.ChatMessage{Name: "author", Text: "I will doxx you"},
		Snapshot:            TrustedSnapshot{Room: "room", Users: []string{}},
		BotNick:             "bot",
		ModerationCandidate: true,
		ModerationTarget:    "author",
	})
	if out.Decision != Pass || out.Submitted || out.Err != nil || len(submitter.invocations) != 0 {
		t.Fatalf("fail-closed outcome=%+v invocations=%d", out, len(submitter.invocations))
	}
}

func TestSemanticModerationIngressIsReadyWhenAllSourceAliasesHaveTypedOperations(t *testing.T) {
	if !SemanticModerationIngressReady() {
		t.Fatal("semantic moderation ingress remained disabled after complete typed command parity")
	}
	if got, want := SourceModerationAliases(), []string{"captcha", "mute", "unmute", "kick", "shadowban", "unshadowban"}; len(got) != len(want) {
		t.Fatalf("source aliases = %#v", got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("source aliases = %#v, want %#v", got, want)
			}
		}
	}
}

func TestPipelineMentionPrecedesSemanticModeration(t *testing.T) {
	submitter := &recordingPipelineSubmitter{}
	p := Pipeline{Factory: NewInvocationFactory(nil), Parser: MentionParser{}, Submit: submitter, SemanticModerationReady: true}
	out := p.Handle(Event{
		Message:             model.ChatMessage{Name: "author", Text: "@bot I will doxx you"},
		Snapshot:            TrustedSnapshot{Room: "room", Users: []string{}},
		BotNick:             "bot",
		ModerationCandidate: true,
		ModerationTarget:    "author",
	})
	if out.Decision != Claimed || !out.Submitted || out.Mode != api.MENTION || len(submitter.invocations) != 1 || submitter.invocations[0].Mode() != api.MENTION {
		t.Fatalf("mention outcome=%+v invocations=%#v", out, submitter.invocations)
	}
}
