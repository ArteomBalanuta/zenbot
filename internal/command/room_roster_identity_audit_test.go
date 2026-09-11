package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"zenbot/internal/agent/tool/contract"
)

func TestRoomRosterIdentityAuditPreservesExactNicknamesThroughGatewayAndTool(t *testing.T) {
	const payload = `{"cmd":"onlineSet","users":[{"nick":"trip-beta","trip":"shared-trip","hash":"a"},{"nick":"hash-alpha","hash":"shared-hash"},{"nick":"hash-beta","hash":"shared-hash"},{"nick":"trip-alpha","trip":"shared-trip","hash":"z"},{"nick":"trip-alpha","trip":"later-trip","hash":"0"}]}`
	wantUsers := []string{"trip-beta", "hash-alpha", "hash-beta", "trip-alpha"}
	tests := []struct {
		name     string
		replyErr error
		failed   bool
	}{
		{name: "successful delivery"},
		{name: "failed delivery retains observation", replyErr: errors.New("private transport diagnostic"), failed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &ownedSnapshotSession{payload: payload}
			result := executeSnapshotTool(t, context.Background(), snapshotToolFixture(session, test.replyErr, nil), "list", `{"room":"lounge"}`)

			var raw []byte
			if test.failed {
				if !result.IsError || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.EffectsCommitted || result.ActionCount != 0 || result.DeliveryCount != 0 || result.VerifiedRoomDelivery() {
					t.Fatalf("failed delivery receipts=%+v", result)
				}
				if !strings.Contains(result.Content, "do not repeat it") || strings.Contains(string(result.Envelope()), "private transport diagnostic") {
					t.Fatalf("failed delivery status/privacy=%s", result.Envelope())
				}
				raw = result.ObservedData
			} else {
				if result.IsError || !result.EffectsCommitted || result.EffectState != contract.EffectCommitted || result.ActionCount != 1 || result.DeliveryCount != 1 || !result.VerifiedRoomDelivery() {
					t.Fatalf("successful delivery receipts=%+v", result)
				}
				raw = []byte(result.Content)
			}

			var observation struct {
				Data struct {
					Room          string   `json:"room"`
					Users         []string `json:"users"`
					Count         int      `json:"count"`
					ReturnedCount int      `json:"returnedCount"`
					Truncated     bool     `json:"truncated"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &observation); err != nil {
				t.Fatalf("decode observation %s: %v", raw, err)
			}
			if observation.Data.Room != "lounge" || !reflect.DeepEqual(observation.Data.Users, wantUsers) || observation.Data.Count != len(wantUsers) || observation.Data.ReturnedCount != len(wantUsers) || observation.Data.Truncated {
				t.Fatalf("observation=%s", raw)
			}
			if session.closed != 1 {
				t.Fatalf("session closed=%d, want 1", session.closed)
			}
		})
	}
}
