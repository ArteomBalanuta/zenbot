package core

import "testing"

func TestSubscriptionSetIsIdempotentAndUnsubscribeIsCaseInsensitive(t *testing.T) {
	e := &EngineImpl{}
	if !e.SubscribeTrip(" Trip-A ") {
		t.Fatal("first subscribe should succeed")
	}
	if e.SubscribeTrip("Trip-A") {
		t.Fatal("duplicate subscribe should be idempotent")
	}
	if !e.IsSubscribedTrip("trip-a") {
		t.Fatal("subscription lookup should be case-insensitive")
	}
	if !e.UnsubscribeTrip("TRIP-A") {
		t.Fatal("matching unsubscribe should succeed")
	}
	if e.UnsubscribeTrip("trip-a") {
		t.Fatal("second unsubscribe should fail")
	}
	if e.IsSubscribedTrip("trip-a") {
		t.Fatal("unsubscription should suppress notifications")
	}
}

func TestSendAddressedMessageRendersForcedWhisperPayloadExactly(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	payload := ` -\n\nHashes: \nh1 \nNicks: \nn1 \n`
	got, err := e.SendAddressedMessage("alice", payload, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "/whisper @alice  -\n\nHashes: \nh1 \nNicks: \nn1 \n"
	if got != want {
		t.Fatalf("returned=%q, want %q", got, want)
	}
	wantJSON := `{ "cmd": "chat", "text": "/whisper @alice  -\n\nHashes: \nh1 \nNicks: \nn1 \n"}`
	if queued := <-e.OutMessageQueue; queued != wantJSON {
		t.Fatalf("queued=%q, want %q", queued, wantJSON)
	}
}
