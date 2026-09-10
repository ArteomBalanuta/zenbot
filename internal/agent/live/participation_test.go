package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/participation"
	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
)

type participationEngine struct{ common.Engine }

func (participationEngine) GetName() string   { return "bot" }
func (participationEngine) GetPrefix() string { return "!" }

type recordingSubmitter struct {
	invocations []api.Invocation
	err         error
}

func (s *recordingSubmitter) Submit(inv api.Invocation) error {
	s.invocations = append(s.invocations, inv)
	return s.err
}

func TestRoomParticipationClaimsPublicMentionWithTrustedSnapshot(t *testing.T) {
	submitter := &recordingSubmitter{}
	users := []string{"alice"}
	p := RoomParticipation{
		Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Parser: participation.MentionParser{}, Submit: submitter},
		Snapshot: func(*message.Context) participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "trusted-room", Users: users, CreatorTrip: "creator", AdminTrips: []string{"admin"}}
		},
	}
	claimed, err := p.Handle(context.Background(), &message.Context{Engine: participationEngine{}, Message: &model.ChatMessage{Name: "alice", Trip: "creator", Hash: "trusted-hash", Text: "@bot: help"}})
	if err != nil || !claimed || len(submitter.invocations) != 1 {
		t.Fatalf("claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}
	inv := submitter.invocations[0]
	if inv.Mode() != api.MENTION || inv.Prompt() != "help" || !inv.CommandOriginated() || inv.Context().Room() != "trusted-room" || inv.Context().Nick() != "alice" || inv.Context().Trip() == nil || *inv.Context().Trip() != "creator" || inv.Context().Hash() == nil || *inv.Context().Hash() != "trusted-hash" || !inv.Context().HasCapability(api.DynamicSQL) || !inv.Context().HasCapability(api.ModerationCommands) {
		t.Fatalf("unexpected invocation: %#v", inv)
	}
	users[0] = "mutated"
	if inv.Context().RoomUsers()[0] != "alice" {
		t.Fatal("invocation retained mutable trusted users")
	}
}

func TestRoomParticipationUsesResolvedBotAuthorMetadataOnly(t *testing.T) {
	submitter := &recordingSubmitter{}
	p := RoomParticipation{
		Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Parser: participation.MentionParser{}, Submit: submitter},
		Snapshot: func(*message.Context) participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "room", Users: []string{}}
		},
	}

	claimed, err := p.Handle(context.Background(), &message.Context{
		Engine:  participationEngine{},
		Message: &model.ChatMessage{Name: "automaton", Text: "@bot help"},
		Author:  &model.User{Name: "automaton", IsBot: true},
	})
	if err != nil || claimed || len(submitter.invocations) != 0 {
		t.Fatalf("resolved bot claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}

	claimed, err = p.Handle(context.Background(), &message.Context{
		Engine:  participationEngine{},
		Message: &model.ChatMessage{Name: "alice", Text: "@bot help"},
		Author:  nil,
	})
	if err != nil || !claimed || len(submitter.invocations) != 1 {
		t.Fatalf("unresolved human claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}

	claimed, err = p.Handle(context.Background(), &message.Context{
		Engine:  participationEngine{},
		Message: &model.ChatMessage{Name: "bob", Text: "@bot help"},
		Author:  &model.User{Name: "bob", IsBot: false},
	})
	if err != nil || !claimed || len(submitter.invocations) != 2 {
		t.Fatalf("resolved non-bot claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}
}

func TestRoomParticipationPassesNonMentionAndClaimsSubmissionError(t *testing.T) {
	submitter := &recordingSubmitter{err: errors.New("busy")}
	p := RoomParticipation{Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Parser: participation.MentionParser{}, Submit: submitter}, Snapshot: func(*message.Context) participation.TrustedSnapshot {
		return participation.TrustedSnapshot{Room: "room", Users: []string{}}
	}}
	for _, text := range []string{"hello", "!help", ""} {
		claimed, err := p.Handle(context.Background(), &message.Context{Engine: participationEngine{}, Message: &model.ChatMessage{Name: "alice", Text: text}})
		if claimed || err != nil {
			t.Fatalf("text=%q claimed=%v err=%v", text, claimed, err)
		}
	}
	claimed, err := p.Handle(context.Background(), &message.Context{Engine: participationEngine{}, Message: &model.ChatMessage{Name: "alice", Text: "@bot: help"}})
	if !claimed || !errors.Is(err, submitter.err) || len(submitter.invocations) != 1 {
		t.Fatalf("claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}
}

func TestRoomParticipationAmbientCadenceSkipsMentionAndQuiet(t *testing.T) {
	submitter := &recordingSubmitter{}
	p := RoomParticipation{
		Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Quiet: participation.NewQuietRegistry(time.Minute), Parser: participation.MentionParser{}, Submit: submitter},
		Snapshot: func(*message.Context) participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "room", Users: []string{}}
		},
		AmbientEnabled: true,
		AmbientEvery:   2,
	}
	for _, item := range []struct{ name, text, trip string }{{"alice", "one", "alice-trip"}, {"alice", "@bot help", "alice-trip"}, {"bob", "please be quiet", "bob-trip"}, {"alice", "two", "alice-trip"}} {
		claimed, err := p.Handle(context.Background(), &message.Context{Engine: participationEngine{}, Message: &model.ChatMessage{Name: item.name, Trip: item.trip, Text: item.text}})
		if err != nil {
			t.Fatalf("text=%q: %v", item.text, err)
		}
		if item.text == "@bot help" && !claimed {
			t.Fatal("mention was not claimed")
		}
	}
	if len(submitter.invocations) != 2 || submitter.invocations[0].Mode() != api.MENTION || submitter.invocations[1].Mode() != api.AMBIENT {
		t.Fatalf("submissions = %#v", submitter.invocations)
	}
}

func TestRoomParticipationQuietSuppressesOnlyAmbientForIdentityAndRoom(t *testing.T) {
	now := time.Unix(1_000, 0)
	submitter := &recordingSubmitter{}
	p := RoomParticipation{
		Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Quiet: participation.NewQuietRegistryAt(time.Minute, func() time.Time { return now }), Parser: participation.MentionParser{}, Submit: submitter},
		Snapshot: func(*message.Context) participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "room", Users: []string{}}
		},
		AmbientEnabled: true,
		AmbientEvery:   1,
	}
	handle := func(name, trip, text string) bool {
		claimed, err := p.Handle(context.Background(), &message.Context{Engine: participationEngine{}, Message: &model.ChatMessage{Name: name, Trip: trip, Text: text}})
		if err != nil {
			t.Fatal(err)
		}
		return claimed
	}
	handle("alice", "same", "please be quiet")
	handle("alice-renamed", "same", "ambient suppressed")
	handle("bob", "other", "ambient accepted")
	if !handle("alice-renamed", "same", "@bot help") {
		t.Fatal("quiet user mention was not claimed")
	}
	now = now.Add(time.Minute)
	handle("alice-renamed", "same", "ambient after expiry")
	if len(submitter.invocations) != 3 || submitter.invocations[0].Mode() != api.AMBIENT || submitter.invocations[1].Mode() != api.MENTION || submitter.invocations[2].Mode() != api.AMBIENT {
		t.Fatalf("submissions = %#v", submitter.invocations)
	}
}
