package command

import (
	"encoding/json"
	"testing"
)

func TestCommandObservationRespectsTrustedVisibilityAndDetachesData(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		invocationPrivate, sourcePrivate, want bool
	}{
		{"public", false, false, true}, {"private withheld", false, true, false},
		{"private allowed", true, true, true}, {"public in private invocation", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &agentCaptureEngine{invocationWhisper: tc.invocationPrivate}
			data := json.RawMessage(`{"value":"source fact"}`)
			observeCommandData(&commandBase{engine: capture}, data, tc.sourcePrivate)
			data[0] = '!'
			if capture.dataObserved != tc.want || (len(capture.data) > 0) != tc.want {
				t.Fatalf("observation=%s observed=%v", capture.data, capture.dataObserved)
			}
			if tc.want && string(capture.data) != `{"value":"source fact"}` {
				t.Fatalf("data mutated: %s", capture.data)
			}
			if capture.actionCount != 0 || capture.deliveryCount != 0 {
				t.Fatal("observation invented action or delivery")
			}
		})
	}
	capture := &agentCaptureEngine{}
	capture.ObserveCommandData([]byte(`{"malformed"`), false)
	if capture.dataObserved || len(capture.data) != 0 {
		t.Fatal("malformed observation admitted")
	}
}
