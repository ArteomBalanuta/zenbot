package sqlite

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNamedQueriesSQLiteCountsVisibilityAndNullableHistory(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	_, err := d.DB.Exec(`INSERT INTO messages(name,trip,message,created_on,channel,visibility) VALUES
	('alice','Trip',NULL,0,NULL,'PUBLIC'),
	('alice','Trip','private',1,'room','WHISPER'),
	('alice','Trip','public',2,'room','PUBLIC');
	INSERT INTO trips(type,trip,created_on) VALUES('USER','Trip',0)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, args, want string }{
		{"message_count", `{}`, `{"count":2}`},
		{"registered_user_count", `{}`, `{"count":1}`},
		{"recent_messages_for_requester", `{"limit":1}`, `{"rows":[{"channel":"room","createdOn":2,"message":"public","name":"alice"}]}`},
		{"recent_messages_for_requester", `{"limit":2}`, `{"rows":[{"channel":"room","createdOn":2,"message":"public","name":"alice"},{"channel":"","createdOn":0,"message":"","name":"alice"}]}`},
		{"recent_messages_for_room", `{"room":"room"}`, `{"rows":[{"channel":"room","createdOn":2,"message":"public","name":"alice"}]}`},
	} {
		got, err := d.ExecuteAgentQuery(ctx, tc.name, json.RawMessage(tc.args), "other", "Trip")
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: got %s err=%v want %s", tc.name, got, err, tc.want)
		}
	}
}
