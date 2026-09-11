package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

// Only the external transport is controlled; selection, persistence receipts,
// and serialization run through the real engine, service, gateway and tool.
type finalShadowTransport struct {
	requestReceiptAuditTransport
	kicks, chats []string
	failKickAt   int
	failAck      bool
}

func (s *finalShadowTransport) SendText(ctx context.Context, raw string) error {
	var payload struct {
		Command string `json:"cmd"`
		Nick    string `json:"nick"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return err
	}
	if payload.Command == "kick" {
		s.kicks = append(s.kicks, payload.Nick)
		if len(s.kicks) == s.failKickAt {
			return errors.New("private kick failure")
		}
	}
	if payload.Command == "chat" {
		s.chats = append(s.chats, payload.Text)
		if s.failAck {
			return errors.New("private ack failure")
		}
	}
	return s.requestReceiptAuditTransport.SendText(ctx, raw)
}

func TestFinalIntegrationShadowContainsNativeBatchReceipts(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		failure                               string
		code                                  string
		persisted, kicks, actions, deliveries int
	}{
		{"success", "", "", 2, 2, 5, 1},
		{"no matches", "absent", "COMMAND_REJECTED", 0, 0, 0, 0},
		{"second persist fails", "persist", "ACTION_OUTCOME_UNKNOWN", 1, 1, 2, 0},
		{"second kick fails", "kick", "ACTION_OUTCOME_UNKNOWN", 2, 2, 3, 0},
		{"cancel after persistence", "cancel", "ACTION_OUTCOME_UNKNOWN", 1, 0, 1, 0},
		{"final ack fails", "ack", "ACTION_OUTCOME_UNKNOWN", 2, 2, 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &shadowBanRepositoryStub{}
			wire := &finalShadowTransport{}
			engine := &core.EngineImpl{Channel: "room", Prefix: "!", Transport: wire, Services: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: repo}}}
			engine.ReplaceActiveUsers([]*model.User{{Name: "raider-a", Trip: "a"}, {Name: "raider-b", Trip: "b"}, {Name: "other"}})
			pattern := "raider"
			switch tc.failure {
			case "absent":
				pattern = "missing"
			case "persist":
				repo.afterPersist = func() { repo.err = errors.New("private persistence failure") }
			case "kick":
				wire.failKickAt = 2
			case "cancel":
				repo.afterPersist = cancel
			case "ack":
				wire.failAck = true
			}
			caller, _ := api.NewContextWithCapabilities("room", "mod", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
			definition, _ := commandcatalog.AgentEntry("shadowban")
			arguments, _ := json.Marshal(map[string]string{"mode": "contains", "target": pattern})
			result, err := (agenttool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, arguments)
			if err != nil || result.ErrorCode != tc.code || result.ActionCount != tc.actions || result.DeliveryCount != tc.deliveries || len(repo.persisted) != tc.persisted || len(wire.kicks) != tc.kicks {
				t.Fatalf("result=%+v err=%v persisted=%v kicks=%v", result, err, repo.persisted, wire.kicks)
			}
			if tc.code == "" {
				if !result.VerifiedRoomDelivery() || len(wire.chats) != 1 || !strings.Contains(result.Content, "2") || !strings.Contains(result.Content, "record") || !strings.Contains(result.Content, "unconfirmed") {
					t.Errorf("untruthful/missing batch acknowledgement: %+v chats=%v", result, wire.chats)
				}
			} else if tc.code == "ACTION_OUTCOME_UNKNOWN" {
				if !result.EffectsCommitted || result.EffectState != contract.EffectUnknown || !strings.Contains(result.Content, "do not repeat") {
					t.Errorf("lost partial receipts: %+v", result)
				}
			}
			if strings.Contains(string(result.Envelope()), "private ") {
				t.Errorf("diagnostic leaked: %s", result.Envelope())
			}
		})
	}
}
