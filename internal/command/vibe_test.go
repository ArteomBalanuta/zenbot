package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/model"
)

type recordingVibeSubmitter struct {
	recordingDirectAgentSubmitter
	vibeCalls   int
	vibeMessage *model.ChatMessage
	vibeErr     error
}

func (s *recordingVibeSubmitter) SubmitVibe(_ context.Context, m *model.ChatMessage) error {
	s.vibeCalls++
	s.vibeMessage = m
	return s.vibeErr
}

func TestVibeProductionRegistrationAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, text                             string
		whisper, disabled, cancelled, rejected bool
		wantCalls                              int
		wantSuccess                            bool
	}{
		{name: "public", text: "!vibe", wantCalls: 1, wantSuccess: true},
		{name: "arguments", text: "!vibe @merc"},
		{name: "whisper", text: "!vibe", whisper: true},
		{name: "disabled", text: "!vibe", disabled: true},
		{name: "cancelled", text: "!vibe", cancelled: true},
		{name: "busy", text: "!vibe", rejected: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &commandEngineStub{}
			s := &recordingVibeSubmitter{}
			if tc.rejected {
				s.vibeErr = errors.New("runtime busy")
			}
			var submitter DirectAgentSubmitter = s
			if tc.disabled {
				submitter = nil
			}
			if err := RegisterUserUtilitiesWithDirectAgent(e, submitter); err != nil {
				t.Fatal(err)
			}
			metadata, ok := (*e.GetEnabledCommands())["vibe"]
			if !ok {
				t.Fatal("vibe not registered")
			}
			m := &model.ChatMessage{Name: "merc", Text: tc.text, IsWhisper: tc.whisper}
			handler := metadata.Command(m).(*legacyAdapter)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			status, err := handler.ExecuteResult(ctx)
			if (status == model.SUCCESSFUL) != tc.wantSuccess || s.vibeCalls != tc.wantCalls || s.calls != 0 {
				t.Fatalf("status=%v err=%v calls=%d direct=%d", status, err, s.vibeCalls, s.calls)
			}
			if tc.rejected && !errors.Is(err, s.vibeErr) {
				t.Fatalf("lost admission error: %v", err)
			}
			if tc.wantCalls > 0 && s.vibeMessage != m {
				t.Fatal("request identity changed")
			}
			if (tc.whisper || tc.disabled || tc.name == "arguments" || tc.rejected) && len(e.chats) == 0 {
				t.Fatal("missing explanatory reply")
			}
			if tc.wantSuccess && len(e.chats) != 0 {
				t.Fatal("command delivered before model completion")
			}
		})
	}
}
