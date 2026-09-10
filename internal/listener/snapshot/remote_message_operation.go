package snapshot

import (
	"encoding/json"

	"zenbot/internal/model"
)

type RemoteMessageOperation struct{ message string }

func NewRemoteMessageOperation(message string) RemoteMessageOperation {
	return RemoteMessageOperation{message: message}
}

func (o RemoteMessageOperation) Apply(context RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	if snapshotContainsOnlyMe(snapshot.Users) {
		return Empty(" " + context.TargetChannel + " is empty"), nil
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
		return Failed(), err
	}
	return Success("sent successfully."), nil
}

func snapshotContainsOnlyMe(users []*model.User) bool {
	for _, user := range users {
		if !user.Isme {
			return false
		}
	}
	return true
}
