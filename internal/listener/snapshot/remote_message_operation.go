package snapshot

import (
	"encoding/json"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type RemoteMessageOperation struct{ message string }

func NewRemoteMessageOperation(message string) RemoteMessageOperation {
	return RemoteMessageOperation{message: message}
}

func (o RemoteMessageOperation) Apply(context RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	if err := executionContext(context).Err(); err != nil {
		return Failed(), err
	}
	for _, user := range snapshot.Users {
		if user == nil {
			return Failed(), fmt.Errorf("snapshot contains nil user")
		}
	}
	if strings.TrimSpace(o.message) == "" {
		return Empty(), nil
	}
	if snapshotContainsOnlyMe(snapshot.Users) {
		return Empty(" " + context.TargetChannel + " is empty"), nil
	}
	if context.SendRaw == nil {
		return Failed(), fmt.Errorf("snapshot raw sender is not configured")
	}
	payload, err := json.Marshal(struct {
		Cmd  string `json:"cmd"`
		Nick string `json:"nick"`
		Text string `json:"text"`
	}{Cmd: "chat", Nick: "*", Text: o.message})
	if err != nil {
		return Failed(), err
	}
	if err := context.SendRaw(string(payload)); err != nil {
		result := Failed()
		result.OutcomeUnknown = true
		return result, err
	}
	result := Success("sent successfully.")
	result.ActionCount = 1
	result.DeliveryCount = 1
	return result, nil
}

func snapshotContainsOnlyMe(users []*model.User) bool {
	for _, user := range users {
		if user != nil && !user.Isme {
			return false
		}
	}
	return true
}
