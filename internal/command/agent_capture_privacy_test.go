package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type capturePrivacySnapshotEngine struct {
	*commandEngineStub
	request snapshot.RoomSnapshotRequest
}

type capturePrivacyFailingEngine struct {
	*commandEngineStub
	err error
}

func (e *capturePrivacyFailingEngine) SendChatMessage(string, string, bool) (string, error) {
	return "", e.err
}

func (e *capturePrivacyFailingEngine) SendWhisperMessage(string, string) (string, error) {
	return "", e.err
}

func (e *capturePrivacyFailingEngine) SendAddressedMessage(string, string, bool) (string, error) {
	return "", e.err
}

func (e *capturePrivacySnapshotEngine) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.request = request
	return nil
}

func TestAgentCapturePrivacyProjectsPrivateDeliveriesForPublicInvocation(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{}}
	capture := &agentCaptureEngine{Engine: engine}
	secrets := []string{
		"direct private secret",
		"explicit whisper secret",
		"addressed private secret",
	}

	if _, err := capture.SendChatMessage("alice", secrets[0], true); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.SendWhisperMessage("alice", secrets[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.SendAddressedMessage("alice", secrets[2], true); err != nil {
		t.Fatal(err)
	}

	if len(engine.chats) != len(secrets) {
		t.Fatalf("private deliveries=%#v", engine.chats)
	}
	for _, secret := range secrets {
		if !strings.Contains(strings.Join(engine.chats, "\n"), secret) {
			t.Fatalf("private delivery lost %q: %#v", secret, engine.chats)
		}
	}
	if capture.deliveryCount != len(secrets) || capture.actionCount != len(secrets) {
		t.Fatalf("deliveryCount=%d actionCount=%d", capture.deliveryCount, capture.actionCount)
	}
	if len(capture.messages) != len(secrets) {
		t.Fatalf("observations=%#v", capture.messages)
	}
	for index, observation := range capture.messages {
		if observation == "" || observation != capture.messages[0] {
			t.Fatalf("observation[%d]=%q, want one fixed non-sensitive notice", index, observation)
		}
		for _, secret := range secrets {
			if strings.Contains(observation, secret) {
				t.Fatalf("private output entered public observation: %q", observation)
			}
		}
	}
}

func TestAgentCapturePrivacyWithholdsPrivateSnapshotReplyAndDataFromPublicInvocation(t *testing.T) {
	const secret = "private snapshot secret"
	engine := &capturePrivacySnapshotEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	capture := &agentCaptureEngine{Engine: engine}
	var callbackResult snapshot.OperationResult
	request := snapshot.RoomSnapshotRequest{
		Whisper: true,
		OnComplete: func(result snapshot.OperationResult) {
			callbackResult = result
		},
	}
	if err := capture.SubmitRoomSnapshot(request); err != nil {
		t.Fatal(err)
	}
	result := snapshot.OperationResult{
		ActionCount: 1, DeliveryCount: 1,
		Outcome: snapshot.OutcomeSuccess,
		Reply:   secret,
		Data:    json.RawMessage(`{"value":"private snapshot secret"}`),
	}
	engine.request.OnComplete(result)
	if err := capture.awaitSnapshotCompletions(context.Background()); err != nil {
		t.Fatal(err)
	}

	if callbackResult.Reply != secret || !bytes.Contains(callbackResult.Data, []byte(secret)) {
		t.Fatalf("real private callback was redacted: %#v", callbackResult)
	}
	if capture.deliveryCount != 1 || capture.actionCount != 1 {
		t.Fatalf("deliveryCount=%d actionCount=%d", capture.deliveryCount, capture.actionCount)
	}
	if len(capture.messages) != 1 || capture.messages[0] != privateDeliveryObservation {
		t.Fatalf("messages=%#v", capture.messages)
	}
	if bytes.Contains(capture.data, []byte(secret)) || len(capture.data) != 0 {
		t.Fatalf("private snapshot data entered public observation: %s", capture.data)
	}
}

func TestAgentCapturePrivacyRetainsAllowedDeliveryObservations(t *testing.T) {
	publicEngine := &commandEngineStub{users: map[string]*model.User{}}
	publicCapture := &agentCaptureEngine{Engine: publicEngine}
	if _, err := publicCapture.SendChatMessage("alice", "ordinary public result", false); err != nil {
		t.Fatal(err)
	}
	if len(publicCapture.messages) != 1 || publicCapture.messages[0] != "ordinary public result" {
		t.Fatalf("public observation=%#v", publicCapture.messages)
	}

	privateEngine := &commandEngineStub{users: map[string]*model.User{}}
	privateCapture := &agentCaptureEngine{Engine: privateEngine, invocationWhisper: true}
	privateMessages := []string{"direct private result", "explicit whisper result", "addressed private result"}
	if _, err := privateCapture.SendChatMessage("alice", privateMessages[0], true); err != nil {
		t.Fatal(err)
	}
	if _, err := privateCapture.SendWhisperMessage("alice", privateMessages[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := privateCapture.SendAddressedMessage("alice", privateMessages[2], true); err != nil {
		t.Fatal(err)
	}
	if len(privateCapture.messages) != len(privateMessages) {
		t.Fatalf("private observations=%#v", privateCapture.messages)
	}
	for index, want := range privateMessages {
		if privateCapture.messages[index] != want {
			t.Fatalf("private observation[%d]=%q want %q", index, privateCapture.messages[index], want)
		}
	}
}

func TestAgentCapturePrivacyFailedSendsCreateNoReceipts(t *testing.T) {
	sendErr := errors.New("delivery failed")
	engine := &capturePrivacyFailingEngine{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		err:               sendErr,
	}
	capture := &agentCaptureEngine{Engine: engine}

	for _, send := range []func() error{
		func() error { _, err := capture.SendChatMessage("alice", "direct secret", true); return err },
		func() error { _, err := capture.SendWhisperMessage("alice", "whisper secret"); return err },
		func() error { _, err := capture.SendAddressedMessage("alice", "addressed secret", true); return err },
	} {
		if err := send(); !errors.Is(err, sendErr) {
			t.Fatalf("send error=%v want %v", err, sendErr)
		}
	}
	if capture.deliveryCount != 0 || capture.actionCount != 0 || len(capture.messages) != 0 {
		t.Fatalf("failed sends produced receipts: deliveryCount=%d actionCount=%d messages=%#v", capture.deliveryCount, capture.actionCount, capture.messages)
	}
}

func TestAgentCapturePrivacyRetainsPrivateSnapshotForPrivateInvocation(t *testing.T) {
	const secret = "private snapshot result"
	engine := &capturePrivacySnapshotEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	capture := &agentCaptureEngine{Engine: engine, invocationWhisper: true}
	if err := capture.SubmitRoomSnapshot(snapshot.RoomSnapshotRequest{Whisper: true}); err != nil {
		t.Fatal(err)
	}
	engine.request.OnComplete(snapshot.OperationResult{
		ActionCount: 1, DeliveryCount: 1,
		Outcome: snapshot.OutcomeSuccess,
		Reply:   secret,
		Data:    json.RawMessage(`{"value":"private snapshot result"}`),
	})
	if err := capture.awaitSnapshotCompletions(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(capture.messages) != 1 || capture.messages[0] != secret || !bytes.Contains(capture.data, []byte(secret)) {
		t.Fatalf("private snapshot observation: messages=%#v data=%s", capture.messages, capture.data)
	}
	if capture.deliveryCount != 1 || capture.actionCount != 1 {
		t.Fatalf("deliveryCount=%d actionCount=%d", capture.deliveryCount, capture.actionCount)
	}
}

func TestAgentCommandGatewayPrivateObservationFollowsTrustedCallerVisibility(t *testing.T) {
	const secret = "private secret from notes"
	for _, test := range []struct {
		name           string
		callerWhisper  bool
		wantFullOutput bool
	}{
		{name: "public caller", callerWhisper: false, wantFullOutput: false},
		{name: "private caller", callerWhisper: true, wantFullOutput: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := openNotesParityEngine(t)
			if err := engine.bundle.Notes.Save(context.Background(), "trip", secret); err != nil {
				t.Fatal(err)
			}
			caller, err := api.NewContext("programming", "alice", "trip", "", test.callerWhisper, []string{"alice"})
			if err != nil {
				t.Fatal(err)
			}

			result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "notes", "")
			if err != nil || result.Status != commandgateway.OutcomeSucceeded || !result.EffectsCommitted || result.Delivery == nil || result.Delivery.Count != 1 || result.Action == nil || result.Action.Count != 1 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if len(engine.chats) != 1 || !strings.Contains(engine.chats[0], secret) || !strings.HasSuffix(engine.chats[0], "|true") {
				t.Fatalf("private delivery=%#v", engine.chats)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			containsSecret := bytes.Contains(encoded, []byte(secret))
			if containsSecret != test.wantFullOutput {
				t.Fatalf("private output visibility=%v want %v: %s", containsSecret, test.wantFullOutput, encoded)
			}
			if len(result.Messages) != 1 {
				t.Fatalf("messages=%#v", result.Messages)
			}
			if !test.wantFullOutput && result.Messages[0] != privateDeliveryObservation {
				t.Fatalf("public observation=%#v", result.Messages)
			}
		})
	}
}
