package live

import "testing"

func TestInternalToolEvidenceGuardUsesExactCaseSensitiveMarker(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    bool
	}{
		{name: "marker at start", content: "[Internal tool evidence from room_users] data", want: true},
		{name: "marker in ordinary prose", content: "Here is [Internal tool evidence from room_users] data", want: true},
		{name: "marker at end", content: "data [Internal tool evidence from ", want: true},
		{name: "empty", content: "", want: false},
		{name: "case variant", content: "[internal tool evidence from room_users]", want: false},
		{name: "incomplete marker", content: "[Internal tool evidence]", want: false},
		{name: "tool result json", content: `{"tool":"room_users","result":"data"}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := containsInternalToolEvidence(tc.content); got != tc.want {
				t.Fatalf("containsInternalToolEvidence(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}
