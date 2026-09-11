package live

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/tool/contract"
)

const failureReplySecret = "result-secret-that-must-not-leak"

func TestFailureReplyFallsBackWithoutExecutionEvidence(t *testing.T) {
	const generic = "failed: the agent could not answer that request."
	for _, cause := range []error{
		errors.New("plain provider failure containing " + failureReplySecret),
		fmt.Errorf("wrapped: %w", &IncompleteTurnError{
			Cause:      errors.New("empty incomplete turn containing " + failureReplySecret),
			Completion: Completion{Observations: assemble.NewObservationStore()},
		}),
	} {
		if got := FailureReply(cause); got != generic {
			t.Fatalf("reply=%q, want %q", got, generic)
		}
	}
}

func TestFailureReplyRendersReceiptStatesFromWrappedIncompleteTurn(t *testing.T) {
	committed := contract.ActionSuccessResult("committed-call", "notify", map[string]any{"message": "delivered " + failureReplySecret}, 2)
	committed.ActionCount = 3
	partial := contract.ActionErrorResult("partial-call", "multi_send", "DELIVERY_FAILED", failureReplySecret, contract.EffectPartial)
	partial.DeliveryCount, partial.ActionCount = 1, 2
	unknown := contract.ActionErrorResult("unknown-call", "remote_action", "ACTION_OUTCOME_UNKNOWN", failureReplySecret, contract.EffectUnknown)
	unknown.ActionCount = 1
	// A pre-execution policy rejection is rejected, not a skipped dependent
	// action, even though both correctly report that effects never started.
	rejected := contract.ActionErrorResult("rejected-call", "moderate", "COMMAND_REJECTED", failureReplySecret, contract.EffectNotStarted)
	skipped := contract.ActionErrorResult("skipped-call", "moderate", "ACTION_NOT_EXECUTED", failureReplySecret, contract.EffectNotStarted)
	skipped.RelatedCallID = "rejected-call"

	for _, tc := range []struct {
		name       string
		result     contract.Result
		wantStatus string
		wantEffect string
		wantCounts string
	}{
		{name: "committed", result: committed, wantStatus: "status=succeeded", wantEffect: "effect=committed", wantCounts: "deliveries=2, actions=3"},
		{name: "partial", result: partial, wantStatus: "status=failed", wantEffect: "effect=partial", wantCounts: "deliveries=1, actions=2"},
		{name: "unknown", result: unknown, wantStatus: "status=failed", wantEffect: "effect=unknown", wantCounts: "deliveries=0, actions=1"},
		{name: "rejected", result: rejected, wantStatus: "status=rejected", wantEffect: "effect=not-started", wantCounts: "deliveries=0, actions=0"},
		{name: "skipped", result: skipped, wantStatus: "status=skipped", wantEffect: "effect=not-started", wantCounts: "deliveries=0, actions=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := FailureReply(wrappedIncompleteFailure(tc.result))
			for _, want := range []string{"failed:", "answer is incomplete", tc.result.CallID, tc.result.ToolName, tc.wantStatus, tc.wantEffect, tc.wantCounts} {
				if !strings.Contains(reply, want) {
					t.Fatalf("reply=%q does not contain %q", reply, want)
				}
			}
			if strings.Contains(reply, failureReplySecret) {
				t.Fatalf("failure reply leaked a result, argument, or cause payload: %q", reply)
			}
			for _, forbidden := range []string{"replay", "repeat the whole", "task is complete", "request is complete"} {
				if strings.Contains(strings.ToLower(reply), forbidden) {
					t.Fatalf("failure reply made an unsafe completion or replay claim: %q", reply)
				}
			}
		})
	}
}

func TestFailureReplySeparatesOutcomeStatusFromEffectState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		result     contract.Result
		wantStatus string
		wantEffect string
	}{
		{
			name:       "rejected with partial effects",
			result:     contract.ActionErrorResult("rejected-partial", "captcha", "COMMAND_REJECTED", failureReplySecret, contract.EffectPartial),
			wantStatus: "status=rejected",
			wantEffect: "effect=partial",
		},
		{
			name:       "rejected with unknown effects",
			result:     contract.ActionErrorResult("rejected-unknown", "remote_action", "COMMAND_REJECTED", failureReplySecret, contract.EffectUnknown),
			wantStatus: "status=rejected",
			wantEffect: "effect=unknown",
		},
		{
			name:       "failed after committed effect",
			result:     contract.ActionErrorResult("failed-committed", "notify", "UNVERIFIED_ROOM_DELIVERY", failureReplySecret, contract.EffectCommitted),
			wantStatus: "status=failed",
			wantEffect: "effect=committed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := FailureReply(wrappedIncompleteFailure(tc.result))
			for _, want := range []string{tc.wantStatus, tc.wantEffect} {
				if !strings.Contains(reply, want) {
					t.Fatalf("reply=%q does not contain %q", reply, want)
				}
			}
		})
	}
}

func TestFailureReplyPreservesMixedReceiptOrder(t *testing.T) {
	read := contract.SuccessResult("read-call", "room_users", map[string]any{"secret": failureReplySecret})
	rejected := contract.ActionErrorResult("rejected-call", "ban", "COMMAND_REJECTED", failureReplySecret, contract.EffectNotCommitted)
	unknown := contract.ActionErrorResult("unknown-call", "send", "ACTION_OUTCOME_UNKNOWN", failureReplySecret, contract.EffectUnknown)

	reply := FailureReply(wrappedIncompleteFailure(read, rejected, unknown))
	positions := []int{
		strings.Index(reply, "read-call"),
		strings.Index(reply, "rejected-call"),
		strings.Index(reply, "unknown-call"),
	}
	if positions[0] < 0 || positions[0] >= positions[1] || positions[1] >= positions[2] {
		t.Fatalf("mixed receipts were missing or reordered: %q", reply)
	}
	for _, want := range []string{"status=succeeded", "effect=unspecified", "status=rejected", "effect=not-committed", "status=failed", "effect=unknown"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply=%q does not contain %q", reply, want)
		}
	}
}

func TestFailureReplyBoundsHostileReceiptIdentitiesAndReportsOmissions(t *testing.T) {
	results := make([]contract.Result, 0, 100)
	results = append(results, contract.ActionSuccessResult(
		"hostile\ncall\t"+strings.Repeat("😀", 500),
		"tool\rname\x00"+strings.Repeat("界", 500),
		failureReplySecret,
		1,
	))
	for index := 1; index < 100; index++ {
		results = append(results, contract.ActionSuccessResult(fmt.Sprintf("call-%03d", index), "notify", failureReplySecret, 1))
	}

	reply := FailureReply(wrappedIncompleteFailure(results...))
	if !utf8.ValidString(reply) || len([]rune(reply)) > 2000 {
		t.Fatalf("reply is invalid or exceeds 2000 runes: valid=%t runes=%d", utf8.ValidString(reply), len([]rune(reply)))
	}
	if strings.Contains(reply, "\n") || strings.Contains(reply, "\r") || strings.Contains(reply, "\x00") {
		t.Fatalf("hostile receipt identity broke readable single-line output: %q", reply)
	}
	if !strings.Contains(reply, "omitted receipts:") {
		t.Fatalf("bounded reply did not report omitted receipts: %q", reply)
	}
	if strings.Contains(reply, failureReplySecret) || strings.Contains(reply, strings.Repeat("😀", 100)) || strings.Contains(reply, strings.Repeat("界", 100)) {
		t.Fatalf("reply leaked payload or failed to bound receipt identity: %q", reply)
	}
}

func wrappedIncompleteFailure(results ...contract.Result) error {
	observations := assemble.NewObservationStore()
	for _, result := range results {
		observations.RecordCall(result.CallID, result.ToolName, []byte(`{"secret":"`+failureReplySecret+`"}`))
		observations.Store(result, contract.DefaultMaxModelResultBytes)
	}
	incomplete := &IncompleteTurnError{
		Cause:      errors.New("provider cause containing " + failureReplySecret),
		Completion: Completion{Observations: observations},
	}
	return fmt.Errorf("outer cause containing %s: %w", failureReplySecret, incomplete)
}
