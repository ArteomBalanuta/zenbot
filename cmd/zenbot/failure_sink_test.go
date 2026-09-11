package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/common"
)

type failureSinkEngine struct {
	common.Engine
	messages []string
}

func (e *failureSinkEngine) SendChatMessage(_ string, message string, _ bool) (string, error) {
	e.messages = append(e.messages, message)
	return message, nil
}

func TestAgentFailureSinkUsesReceiptOnlyIncompleteReply(t *testing.T) {
	const secret = "sink-result-secret"
	engine := &failureSinkEngine{}
	observations := assemble.NewObservationStore()
	result := contract.ActionErrorResult("uncertain-call", "notify", "ACTION_OUTCOME_UNKNOWN", secret, contract.EffectUnknown)
	observations.RecordCall(result.CallID, result.ToolName, []byte(`{"payload":"`+secret+`"}`))
	observations.Store(result, contract.DefaultMaxModelResultBytes)
	cause := errors.New(secret)
	failure := &live.IncompleteTurnError{Cause: cause, Completion: live.Completion{Observations: observations}}
	sink := newAgentFailureSink(func() common.Engine { return engine })
	inv := runtime.NewInvocation("failure", runtime.NewContext("room", "alice", "", "", false, nil), "do it", runtime.MENTION, "", true)

	sink.DeliverFailure(context.Background(), inv, failure)

	if len(engine.messages) != 1 {
		t.Fatalf("messages=%q", engine.messages)
	}
	reply := engine.messages[0]
	for _, want := range []string{"answer is incomplete", "uncertain-call", "notify", "status=failed", "effect=unknown"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply=%q does not contain %q", reply, want)
		}
	}
	if strings.Contains(reply, secret) {
		t.Fatalf("production failure sink leaked payload or cause: %q", reply)
	}
}

func TestAgentFailureSinkDoesNotDeliverAfterCancellation(t *testing.T) {
	engine := &failureSinkEngine{}
	sink := newAgentFailureSink(func() common.Engine { return engine })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inv := runtime.NewInvocation("cancelled", runtime.NewContext("room", "alice", "", "", false, nil), "do it", runtime.MENTION, "", true)

	sink.DeliverFailure(ctx, inv, errors.New("cancelled"))

	if len(engine.messages) != 0 {
		t.Fatalf("canceled failure delivered messages=%q", engine.messages)
	}
}
