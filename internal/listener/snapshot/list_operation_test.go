package snapshot

import (
	"encoding/json"
	"reflect"
	"testing"

	"zenbot/internal/model"
)

func TestListRoomOperationFormatsStableSaturnUserList(t *testing.T) {
	operation := NewListRoomOperation()
	result, err := operation.Apply(RoomSnapshotContext{TargetChannel: "lounge"}, Snapshot{Users: []*model.User{
		{Name: "zulu", Hash: "z", Trip: ""},
		{Name: "alpha", Hash: "a", Trip: "trip"},
		{Name: "duplicate", Hash: "a", Trip: "trip"},
		nil,
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "\\nUsers online: \\na - trip - alpha\\nz - ------ - zulu\\n\\n"
	if result.Outcome != OutcomeSuccess || result.Reply != want {
		t.Fatalf("result=%+v, want reply %q", result, want)
	}
	var data struct {
		Room          string   `json:"room"`
		Users         []string `json:"users"`
		Count         int      `json:"count"`
		ReturnedCount int      `json:"returnedCount"`
		Truncated     bool     `json:"truncated"`
	}
	if err := json.Unmarshal(operationResultData(t, result), &data); err != nil {
		t.Fatal(err)
	}
	if data.Room != "lounge" || !reflect.DeepEqual(data.Users, []string{"alpha", "zulu"}) || data.Count != 2 || data.ReturnedCount != 2 || data.Truncated {
		t.Fatalf("data=%+v", data)
	}
}

func TestListRoomOperationReturnsTypedEmptyRoster(t *testing.T) {
	result, err := NewListRoomOperation().Apply(RoomSnapshotContext{TargetChannel: "quiet"}, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	data := operationResultData(t, result)
	if string(data) != `{"room":"quiet","users":[],"count":0,"returnedCount":0,"truncated":false}` {
		t.Fatalf("data=%s", data)
	}
}

func operationResultData(t *testing.T, result OperationResult) json.RawMessage {
	t.Helper()
	field := reflect.ValueOf(result).FieldByName("Data")
	if !field.IsValid() {
		t.Fatal("OperationResult.Data is missing")
	}
	data, ok := field.Interface().(json.RawMessage)
	if !ok {
		t.Fatalf("OperationResult.Data has type %s, want json.RawMessage", field.Type())
	}
	return append(json.RawMessage(nil), data...)
}
