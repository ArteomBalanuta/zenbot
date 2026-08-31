package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func context(t *testing.T) Context {
	t.Helper()
	c, err := NewContextWithCapabilities("room", " Alice ", " Trip ", " Hash ", false, []string{"a"}, []Capability{DynamicSQL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestInvocationConstructorsValidationAndDefaults(t *testing.T) {
	c := context(t)
	i, err := NewInvocation(c, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if i.RequestID() == "" || i.Mode() != DIRECT || i.CurrentMessageText() != nil || i.CommandOriginated() {
		t.Fatalf("defaults: %#v", i)
	}
	cur := "current"
	i, err = NewInvocation("req", c, "hello", MENTION, cur, true)
	if err != nil || i.Mode() != MENTION || i.CurrentMessageText() == nil || *i.CurrentMessageText() != "current" || !i.CommandOriginated() {
		t.Fatal(i, err)
	}
	generated, err := NewInvocation(c, "hello", MENTION)
	if err != nil || generated.RequestID() == "" || generated.Mode() != MENTION {
		t.Fatalf("generated mode overload: %#v %v", generated, err)
	}
	roomless, err := NewContextWithCapabilities("", "n", "", "", false, []string{}, []Capability{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewInvocation("roomless", roomless, "hello"); err != nil {
		t.Fatalf("blank room is valid in Saturn context: %v", err)
	}
	for _, args := range [][]any{{" ", c, "x", DIRECT}, {"id", c, " ", DIRECT}, {"id", c, "x", InvocationMode("BAD")}, {"id", c, "x", nil}} {
		if _, err := NewInvocation(args...); err == nil {
			t.Fatal("expected validation")
		}
	}
}
func TestContextCopiesCapabilitiesAndMemoryKey(t *testing.T) {
	users := []string{"a"}
	caps := []Capability{DynamicSQL}
	c, err := NewContextWithCapabilities("r", "n", " trip ", "hash", true, users, caps)
	if err != nil {
		t.Fatal(err)
	}
	users[0] = "x"
	caps[0] = PermanentBan
	if c.RoomUsers()[0] != "a" || !c.HasCapability(DynamicSQL) || c.MemoryKey() != "1:r|whisper|trip: trip " {
		t.Fatal(c.MemoryKey())
	}
	if _, err := NewContext("r", "n", "", "", false, nil); err == nil {
		t.Fatal("nil users accepted")
	}
}
func TestContextPreservesEmptyCollectionsAndJSONNullableEmptyStrings(t *testing.T) {
	c, err := NewContextWithCapabilities("r", "n", "", "", false, []string{}, []Capability{})
	if err != nil || c.RoomUsers() == nil || c.Capabilities() == nil {
		t.Fatalf("empty collections must remain non-nil: %#v %v", c, err)
	}
	data := []byte(`{"room":"r","nick":"n","trip":"","hash":"","whisper":false,"roomUsers":[],"capabilities":[],"moderationTarget":null}`)
	var decoded Context
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Trip() == nil || *decoded.Trip() != "" || decoded.Hash() == nil || *decoded.Hash() != "" {
		t.Fatalf("explicit empty nullable strings lost: %#v %#v", decoded.Trip(), decoded.Hash())
	}
}
func TestIdentityPrecedenceAndNormalization(t *testing.T) {
	v, err := FromValues(" TRIP ", "hash", "Nick")
	if err != nil || v.Value() != "trip:trip" {
		t.Fatal(v, err)
	}
	v, err = FromValues(" ", " HASH ", "Nick")
	if err != nil || v.Value() != "hash:hash" {
		t.Fatal(v, err)
	}
	v, err = FromValues("", "", " NICK ")
	if err != nil || v.Value() != "nick:nick" {
		t.Fatal(v, err)
	}
	v, err = FromValues("", "", "a	b")
	if err != nil || v.Value() != "nick:a	b" {
		t.Fatalf("internal whitespace changed: %q %v", v.Value(), err)
	}
}
func TestJSONGoldenNullAndMissing(t *testing.T) {
	c := context(t)
	i, err := NewInvocation("req", c, "hello", DIRECT)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, field := range []string{`"requestId":"req"`, `"currentMessageText":null`, `"commandOriginated":false`} {
		if !strings.Contains(s, field) {
			t.Fatalf("%s missing in %s", field, s)
		}
	}
	var round Invocation
	if err := json.Unmarshal(b, &round); err != nil || round.CurrentMessageText() != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"requestId":"x","context":null,"prompt":"p","mode":"DIRECT","commandOriginated":false}`), &round); err == nil {
		t.Fatal("missing context accepted")
	}
	if err := json.Unmarshal([]byte(`{"requestId":"x","context":`+string(mustJSON(c))+`,"prompt":"p","mode":"UNKNOWN","commandOriginated":false}`), &round); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
func mustJSON(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func TestResultFactoriesAndJSON(t *testing.T) {
	r, err := Reply("c", " ")
	if err != nil || !r.ShouldReply() || r.Content() != " " {
		t.Fatal(r, err)
	}
	r, err = Silent("c")
	if err != nil || r.Content() != "" || r.ShouldReply() {
		t.Fatal(r, err)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "errorCode") {
		t.Fatal(string(b))
	}
	if _, err := NewResult(" ", "x"); err == nil {
		t.Fatal("blank correlation accepted")
	}
}
