package command

import (
	"context"
	"testing"
)

type fakeRC struct{ removed string }

func (f *fakeRC) AddReplica(context.Context, string) error        { return nil }
func (f *fakeRC) RemoveReplica(_ context.Context, s string) error { f.removed = s; return nil }
func (f *fakeRC) ReplicaChannels() []string                       { return []string{"z", "a"} }
func TestReplicaBoundaries(t *testing.T) {
	if got, _ := ParseReplicaChannel([]string{"replica", " x "}); got != "x" {
		t.Fatal(got)
	}
	f := &fakeRC{}
	if err := ReplicaOff(context.Background(), f, []string{"replicaoff", "room"}); err != nil || f.removed != "room" {
		t.Fatal(err, f.removed)
	}
	if got := ReplicaStatusReply("main", f.ReplicaChannels()); got != " channel: main replicas: a,z" {
		t.Fatal(got)
	}
}
func TestMsgAndWhiskeyParsing(t *testing.T) {
	r, m, e := ParseMsgChannel([]string{"msgroom", "?target", "hello", "world"})
	if e != nil || r != "target" || m != "hello world" {
		t.Fatal(r, m, e)
	}
	p, back, e := WhiskeyProxyOrder(context.Background(), []string{"a", "b"}, func(_ context.Context, s string) error {
		if s == "a" {
			return context.DeadlineExceeded
		}
		return nil
	}, 0)
	if e != nil || p != "b" || len(back) != 0 {
		t.Fatal(p, back, e)
	}
}
