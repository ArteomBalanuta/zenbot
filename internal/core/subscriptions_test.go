package core

import "testing"

func TestSubscriptionSetUsesNormalizedExactTripCredentials(t *testing.T) {
	e := &EngineImpl{}
	if !e.SubscribeTrip(" Trip-A ") {
		t.Fatal("first subscribe should succeed")
	}
	if e.SubscribeTrip("Trip-A") {
		t.Fatal("duplicate subscribe should be idempotent")
	}
	if e.IsSubscribedTrip("trip-a") {
		t.Fatal("case-variant trip read another credential's subscription")
	}
	if e.UnsubscribeTrip("TRIP-A") {
		t.Fatal("case-variant trip removed another credential's subscription")
	}
	if !e.SubscribeTrip("trip-a") || e.SubscribeTrip("trip-a") {
		t.Fatal("distinct case-variant credential did not retain exact duplicate semantics")
	}
	if !e.UnsubscribeTrip("trip-a") {
		t.Fatal("case-variant credential failed to remove its own subscription")
	}
	if !e.IsSubscribedTrip(" Trip-A ") {
		t.Fatal("removing case-variant credential removed exact subscription")
	}
	if !e.UnsubscribeTrip(" Trip-A ") {
		t.Fatal("trimmed exact unsubscribe should succeed")
	}
	if e.UnsubscribeTrip("Trip-A") {
		t.Fatal("second unsubscribe should fail")
	}
	if e.IsSubscribedTrip("Trip-A") {
		t.Fatal("unsubscription should suppress notifications")
	}
}

func TestSendAddressedMessageRendersForcedWhisperPayloadExactly(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	payload := " -\n\nHashes: \nh1 \nNicks: \nn1 \n"
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
