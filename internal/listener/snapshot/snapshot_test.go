package snapshot

import "testing"

func TestParseRejectsMalformedPayload(t *testing.T) {
	for _, p := range []string{"not-json", `{"cmd":"chat"}`, `{"cmd":"onlineSet","users":[null]}`, `{"cmd":"onlineSet","users":[{"nick":""}]}`} {
		if _, err := Parse(p, false); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}
func TestCoordinatorReplacesSnapshot(t *testing.T) {
	s := NewStore()
	c := NewCoordinator(s, 0)
	if err := c.Apply(`{"cmd":"onlineSet","users":[{"nick":"Alice","trip":"t1"},{"nick":"Bob"}]}`, false); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Snapshot()); got != 2 {
		t.Fatal(got)
	}
	if err := c.Apply(`{"cmd":"onlineSet","users":[{"nick":"Carol","trip":"t3"}]}`, false); err != nil {
		t.Fatal(err)
	}
	u := s.Snapshot()
	if len(u) != 1 || u[0].Name != "Carol" {
		t.Fatalf("%v", u)
	}
}
