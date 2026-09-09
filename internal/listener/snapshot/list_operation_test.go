package snapshot

import (
	"testing"

	"zenbot/internal/model"
)

func TestListRoomOperationFormatsStableSaturnUserList(t *testing.T) {
	operation := NewListRoomOperation()
	result, err := operation.Apply(RoomSnapshotContext{}, Snapshot{Users: []*model.User{
		{Name: "zulu", Hash: "z", Trip: ""},
		{Name: "alpha", Hash: "a", Trip: "trip"},
		{Name: "duplicate", Hash: "a", Trip: "trip"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "\\nUsers online: \\na - trip - alpha\\nz - ------ - zulu\\n\\n"
	if result.Outcome != OutcomeSuccess || result.Reply != want {
		t.Fatalf("result=%+v, want reply %q", result, want)
	}
}
