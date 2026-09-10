package live

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/participation"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
)

func TestRoomParticipationDerivesSemanticTargetOnlyFromResolvedAuthor(t *testing.T) {
	submitter := &recordingSubmitter{}
	p := RoomParticipation{
		Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Parser: participation.MentionParser{}, Submit: submitter, SemanticModerationReady: true},
		Snapshot: func(*message.Context) participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "room", Users: []string{"canonical-author"}, CreatorTrip: "creator"}
		},
		SemanticCandidate: func(model.ChatMessage) bool { return true },
	}

	claimed, err := p.Handle(context.Background(), &message.Context{
		Engine:  participationEngine{},
		Message: &model.ChatMessage{Name: "chat-asserted-name", Text: "I will doxx you"},
		Author:  &model.User{Name: "canonical-author", IsBot: false},
	})
	if err != nil || claimed || len(submitter.invocations) != 1 {
		t.Fatalf("resolved semantic candidate claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}
	if submitter.invocations[0].Mode() != api.MODERATION {
		t.Fatalf("mode = %s", submitter.invocations[0].Mode())
	}
	if got := submitter.invocations[0].Context().ModerationTarget(); got == nil || *got != "canonical-author" {
		t.Fatalf("moderation target = %v", got)
	}

	claimed, err = p.Handle(context.Background(), &message.Context{
		Engine:  participationEngine{},
		Message: &model.ChatMessage{Name: "unresolved-chat-name", Text: "I will doxx you"},
		Author:  nil,
	})
	if err != nil || claimed || len(submitter.invocations) != 1 {
		t.Fatalf("unresolved semantic candidate claimed=%v err=%v invocations=%d", claimed, err, len(submitter.invocations))
	}
}
