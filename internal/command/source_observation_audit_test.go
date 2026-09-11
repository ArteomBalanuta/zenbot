package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/listener/snapshot"
)

func TestSourceObservationAuditRetainsObservedRoomCountAfterFailedDelivery(t *testing.T) {
	session := &ownedSnapshotSession{payload: `{"cmd":"onlineSet","users":[{"nick":"alice"},{"nick":"bob"}]}`}
	engine := snapshotToolFixture(session, errors.New("private transport diagnostic"), nil)
	result := executeSnapshotTool(t, context.Background(), engine, "list", `{"room":"lounge"}`)
	if result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.DeliveryCount != 0 || session.closed != 1 {
		t.Fatalf("failed-delivery fixture did not retain its terminal state: result=%+v closed=%d", result, session.closed)
	}
	observation := string(result.Envelope())
	if !strings.Contains(observation, `"count":2`) || !strings.Contains(observation, `"room":"lounge"`) {
		t.Fatalf("valid source observation was discarded with failed acknowledgment: %s", observation)
	}
	if strings.Contains(observation, "private transport diagnostic") {
		t.Fatalf("raw transport diagnostic entered model data: %s", observation)
	}
}

func TestSourceObservationAuditEmptyAndMalformedSnapshotsAfterFailedDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		observed      bool
	}{
		{"empty", `{"cmd":"onlineSet","users":[]}`, true},
		{"absent", `{"cmd":"onlineSet"}`, false},
		{"malformed", `{"cmd":"onlineSet","users":[{}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &ownedSnapshotSession{payload: tc.payload}
			result := executeSnapshotTool(t, context.Background(), snapshotToolFixture(session, errors.New("private diagnostic"), nil), "list", `{"room":"lounge"}`)
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(result.Envelope(), &envelope); err != nil {
				t.Fatal(err)
			}
			if observed := len(envelope.Data) > 0 && string(envelope.Data) != "null"; observed != tc.observed {
				t.Fatalf("observed=%v result=%s", observed, result.Envelope())
			}
			if tc.observed && (!strings.Contains(string(envelope.Data), `"count":0`) || !strings.Contains(string(envelope.Data), `"users":[]`)) {
				t.Fatalf("empty observation lost: %s", result.Envelope())
			}
			if !result.IsError || result.DeliveryCount != 0 || result.ActionCount != 0 || result.VerifiedRoomDelivery() || session.closed != 1 {
				t.Fatalf("receipts=%+v closed=%d", result, session.closed)
			}
		})
	}
}

func TestSourceObservationAuditCancellationAfterObservationKeepsFacts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	operation := snapshot.OperationFunc(func(ctx snapshot.RoomSnapshotContext, source snapshot.Snapshot) (snapshot.OperationResult, error) {
		result, err := snapshot.NewListRoomOperation().Apply(ctx, source)
		cancel()
		return result, err
	})
	session := &ownedSnapshotSession{payload: `{"cmd":"onlineSet","users":[{"nick":"alice"}]}`}
	result := executeSnapshotTool(t, ctx, snapshotToolFixture(session, context.Canceled, operation), "list", `{"room":"lounge"}`)
	if !strings.Contains(string(result.Envelope()), `"count":1`) || result.EffectState != contract.EffectUnknown || result.DeliveryCount != 0 || session.closed != 1 {
		t.Fatalf("cancelled observation=%s receipt=%+v closed=%d", result.Envelope(), result, session.closed)
	}
}
