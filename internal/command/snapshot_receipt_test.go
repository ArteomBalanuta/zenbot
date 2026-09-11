package command

import (
	"context"
	"testing"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

func TestSnapshotReceiptUsesExplicitEvidenceAndOutcome(t *testing.T) {
	caller, _ := api.NewContextWithCapabilities("programming", "caller", "trip", "hash", false, []string{}, []api.Capability{api.PermanentBan})
	for _, tc := range []struct {
		name                string
		operation           snapshot.OperationResult
		status              commandgateway.OutcomeStatus
		actions, deliveries int
	}{
		{"silent successful nuke", snapshot.OperationResult{Outcome: snapshot.OutcomeSuccess, ActionCount: 3}, commandgateway.OutcomeSucceeded, 3, 0},
		{"absent target", snapshot.Absent("target missing"), commandgateway.OutcomeNotFound, 0, 0},
		{"empty unsent message", snapshot.Empty("empty"), commandgateway.OutcomeRejected, 0, 0},
		{"partial send", snapshot.OperationResult{Outcome: snapshot.OutcomeFailed, ActionCount: 1, OutcomeUnknown: true}, commandgateway.OutcomeUnknown, 1, 0},
		{"failed reply delivery", snapshot.OperationResult{Outcome: snapshot.OutcomeFailed, OutcomeUnknown: true}, commandgateway.OutcomeUnknown, 0, 0},
		{"callback is not delivery", snapshot.Success("callback text"), commandgateway.OutcomeSucceeded, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &snapshotGatewayEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: true}, requests: make(chan snapshot.RoomSnapshotRequest, 1)}
			done := make(chan CommandExecution, 1)
			go func() {
				r, _ := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "nuke", "lounge")
				done <- r
			}()
			req := <-engine.requests
			req.OnComplete(tc.operation)
			r := <-done
			actions, deliveries := 0, 0
			if r.Action != nil {
				actions = r.Action.Count
			}
			if r.Delivery != nil {
				deliveries = r.Delivery.Count
			}
			if r.Status != tc.status || actions != tc.actions || deliveries != tc.deliveries {
				t.Fatalf("execution=%+v actions=%d deliveries=%d", r, actions, deliveries)
			}
			if tc.name == "callback is not delivery" && len(r.Messages) != 0 {
				t.Fatalf("unsent callback became delivered output: %+v", r)
			}
		})
	}
}

func TestSnapshotReceiptCancellationWaitsForTerminalOwner(t *testing.T) {
	caller, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	engine := &snapshotGatewayEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: true}, requests: make(chan snapshot.RoomSnapshotRequest, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan CommandExecution, 1)
	go func() { r, _ := NewAgentCommandGateway(engine).Execute(ctx, caller, "list", "lounge"); done <- r }()
	req := <-engine.requests
	cancel()
	select {
	case r := <-done:
		t.Fatalf("gateway returned before owner cleanup: %+v", r)
	case <-time.After(20 * time.Millisecond):
	}
	req.OnComplete(snapshot.OperationResult{Outcome: snapshot.OutcomeFailed, ActionCount: 1, OutcomeUnknown: true})
	r := <-done
	if r.Status != commandgateway.OutcomeUnknown || r.Action == nil || r.Action.Count != 1 {
		t.Fatalf("execution=%+v", r)
	}
}
