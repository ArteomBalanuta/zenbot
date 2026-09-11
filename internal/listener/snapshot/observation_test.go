package snapshot

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestObservedListDataSurvivesFlushFailureAndIndependentCallbacks(t *testing.T) {
	session := &controlledSession{fakeSession: fakeSession{id: "observed"}, flushErr: errors.New("flush failed")}
	var first OperationResult
	completed := make(chan OperationResult, 1)
	coordinator := NewRoomSnapshotCoordinatorWithOutcome(fakeFactory{session: session}, nil, ParseUsers, time.Second, func(_ RoomSnapshotRequest, r OperationResult) {
		first = r
		r.Data[0] = 'x'
	})
	req := request(NewListRoomOperation())
	req.OnComplete = func(r OperationResult) { completed <- r }
	if err := coordinator.Submit(req); err != nil {
		t.Fatal(err)
	}
	coordinator.OnSnapshot(session.id, `{"cmd":"onlineSet","users":[{"nick":"alice"},{"nick":"bob"}]}`)
	r := <-completed
	if !first.DataObserved || !r.DataObserved || !json.Valid(r.Data) || !strings.Contains(string(r.Data), `"count":2`) || r.Outcome != OutcomeFailed || !r.OutcomeUnknown || r.ActionCount != 0 || r.DeliveryCount != 0 || coordinator.ActiveWorkflowCount() != 0 {
		t.Fatalf("result=%+v data=%s", r, r.Data)
	}
}
