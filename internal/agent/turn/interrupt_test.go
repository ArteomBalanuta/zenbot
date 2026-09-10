package turn

import (
	"context"
	"testing"

	"zenbot/internal/agent/tool/execution"
)

func TestAllowAllInterruptHookPreservesAuthorizedExecution(t *testing.T) {
	decision, err := (AllowAllInterruptHook{}).Review(context.Background(), ActionRequest{Call: execution.Call{ID: "one", Name: "run_command"}})
	if err != nil || decision.Outcome != InterruptAllow {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}

func TestInterruptDecisionConstructorsRequireActionableData(t *testing.T) {
	if _, err := NewInterruptDecision(InterruptDeny, "", ""); err == nil {
		t.Fatal("denial without reason was accepted")
	}
	if _, err := NewInterruptDecision(InterruptPause, "approval required", ""); err == nil {
		t.Fatal("pause without resume token was accepted")
	}
	if decision, err := NewInterruptDecision(InterruptDeny, "operator denied", ""); err != nil || decision.Outcome != InterruptDeny {
		t.Fatalf("valid denial rejected: %#v %v", decision, err)
	}
}
