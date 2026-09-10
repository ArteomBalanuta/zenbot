package turn

import "testing"

func TestRecoveryPolicyChoosesBoundedCorrectionOrDegradation(t *testing.T) {
	policy := RecoveryPolicy{}
	tests := []struct {
		name string
		in   RecoveryInput
		want RecoveryDecision
	}{
		{name: "invalid arguments can self correct", in: RecoveryInput{ErrorCode: "INVALID_ARGUMENTS", ToolRoundsRemaining: 1}, want: RecoveryRetryModel},
		{name: "failed tool can self correct", in: RecoveryInput{ErrorCode: "TOOL_EXECUTION_FAILED", ToolRoundsRemaining: 1}, want: RecoveryRetryModel},
		{name: "exhausted failure degrades", in: RecoveryInput{ErrorCode: "TOOL_DISABLED", ToolRoundsRemaining: 2}, want: RecoveryDegrade},
		{name: "observations reserve synthesis", in: RecoveryInput{HasObservations: true}, want: RecoveryFinalize},
		{name: "nothing usable fails", in: RecoveryInput{}, want: RecoveryFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.Decide(tc.in); got != tc.want {
				t.Fatalf("decision=%s, want %s", got, tc.want)
			}
		})
	}
}
