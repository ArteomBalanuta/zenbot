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
		{Name: "same-trip", Hash: "a", Trip: "trip"},
		nil,
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "\nUsers online: \na - trip - alpha\na - trip - same-trip\nz - ------ - zulu\n\n"
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
	if data.Room != "lounge" || !reflect.DeepEqual(data.Users, []string{"alpha", "same-trip", "zulu"}) || data.Count != 3 || data.ReturnedCount != 3 || data.Truncated {
		t.Fatalf("data=%+v", data)
	}
}

func TestListRoomOperationCountsExactNicknamesFromParsedRoster(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantData  string
		wantReply string
	}{
		{
			name:      "distinct nicknames sharing trip",
			payload:   `{"cmd":"onlineSet","users":[{"nick":"alpha","trip":"shared","hash":"b"},{"nick":"beta","trip":"shared","hash":"a"}]}`,
			wantData:  `{"room":"lounge","users":["beta","alpha"],"count":2,"returnedCount":2,"truncated":false}`,
			wantReply: "\nUsers online: \na - shared - beta\nb - shared - alpha\n\n",
		},
		{
			name:      "distinct nicknames sharing hash without trips",
			payload:   `{"cmd":"onlineSet","users":[{"nick":"alpha","hash":"shared"},{"nick":"beta","hash":"shared"}]}`,
			wantData:  `{"room":"lounge","users":["alpha","beta"],"count":2,"returnedCount":2,"truncated":false}`,
			wantReply: "\nUsers online: \nshared - ------ - alpha\nshared - ------ - beta\n\n",
		},
		{
			name:      "exact repeated nickname keeps first source record",
			payload:   `{"cmd":"onlineSet","users":[{"nick":"exact","trip":"first-trip","hash":"z"},{"nick":"exact","trip":"later-trip","hash":"a"}]}`,
			wantData:  `{"room":"lounge","users":["exact"],"count":1,"returnedCount":1,"truncated":false}`,
			wantReply: "\nUsers online: \nz - first-trip - exact\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := Parse(test.payload, false)
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewListRoomOperation().Apply(RoomSnapshotContext{TargetChannel: "lounge"}, source)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(operationResultData(t, result)); got != test.wantData {
				t.Fatalf("data=%s, want %s", got, test.wantData)
			}
			if result.Reply != test.wantReply {
				t.Fatalf("reply=%q, want %q", result.Reply, test.wantReply)
			}
		})
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
