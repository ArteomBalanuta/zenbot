package listener

import "testing"

func TestSnapshotOnlineSetListenerForwardsValidPayloadWithoutEngineState(t *testing.T) {
	payload := `{"cmd":"onlineSet","users":[{"nick":"alice","trip":"t","hash":"h"}]}`
	var got string
	l := NewSnapshotOnlineSetListener(func(v string) { got = v })
	l.Notify(payload)
	if got != payload {
		t.Fatalf("got %q", got)
	}
}
