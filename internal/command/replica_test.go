package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"zenbot/internal/listener"
	"zenbot/internal/model"
)

type fakeRC struct {
	added, removed    string
	addErr, removeErr error
	channels          []string
}

func (f *fakeRC) AddReplica(_ context.Context, channel string) error {
	f.added = channel
	if f.addErr != nil {
		return f.addErr
	}
	f.channels = append(f.channels, channel)
	return nil
}
func (f *fakeRC) RemoveReplica(_ context.Context, channel string) error {
	f.removed = channel
	if f.removeErr != nil {
		return f.removeErr
	}
	return nil
}
func (f *fakeRC) ReplicaChannels() []string {
	if f.channels != nil {
		return append([]string(nil), f.channels...)
	}
	return []string{"z", "a"}
}
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

type replicaRegistrationEngine struct {
	*commandEngineStub
	fakeRC
}

func TestReplicaRegistersOnlyWithReplicaController(t *testing.T) {
	without := &commandEngineStub{}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	with := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"replica", "bot", "agent", "replicaoff", "offline", "botoff", "agentoff", "replicastatus", "status"} {
		if _, ok := (*without.GetEnabledCommands())[alias]; ok {
			t.Errorf("%q registered without ReplicaController", alias)
		}
		metadata, ok := (*with.GetEnabledCommands())[alias]
		if !ok {
			t.Errorf("%q not registered with ReplicaController", alias)
			continue
		}
		if role := metadata.Command(&model.ChatMessage{}).GetRole(); role == nil || *role != model.ADMIN {
			t.Errorf("%q role=%v, want ADMIN", alias, role)
		}
		adapter, ok := metadata.Command(&model.ChatMessage{}).(*legacyAdapter)
		if !ok {
			t.Errorf("%q command=%T, want legacy adapter", alias, metadata.Command(&model.ChatMessage{}))
			continue
		}
		if _, generic := adapter.def.New(with, &model.ChatMessage{}).(*saturnCommand); generic {
			t.Errorf("%q remains a generic fallback", alias)
		}
	}
}

type deniedReplicaEngine struct{ *replicaRegistrationEngine }

func (*deniedReplicaEngine) IsUserAuthorized(_ *model.User, _ *model.Role) bool { return false }

func TestReplicaCommandIsBlockedForUnauthorizedAuthor(t *testing.T) {
	e := &deniedReplicaEngine{replicaRegistrationEngine: &replicaRegistrationEngine{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}},
		fakeRC:            fakeRC{channels: []string{}},
	}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!replica lounge"})
	if err != nil {
		t.Fatal(err)
	}
	listener.NewUserChatListener(e).Notify(string(payload))
	if e.added != "" {
		t.Fatalf("unauthorized replica add=%q", e.added)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice| you are not authorized to run: replica command.|false" {
		t.Fatalf("authorization response=%v", e.chats)
	}
}

func TestReplicaAliasesAndFirstArgumentUseSaturnContract(t *testing.T) {
	for _, alias := range []string{"replica", "bot", "agent"} {
		t.Run(alias, func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: []string{}}}
			d, ok := commandDefinitionFor(alias)
			if !ok {
				t.Fatalf("missing %s definition", alias)
			}
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias + " lounge ignored", IsWhisper: true}).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if e.added != "lounge" {
				t.Fatalf("added=%q, want lounge", e.added)
			}
			want := "alice|started replica in channel: lounge successfully. Number of replicas: 1|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%v, want %q", e.chats, want)
			}
		})
	}
}

func TestReplicaSaturnFailuresReplyAndFail(t *testing.T) {
	cases := []struct {
		name, text, want string
		channels         []string
	}{
		{"missing", "!replica", "Example: !replica lounge", nil},
		{"blank", "!replica   ", "I'm the host bot serving current channel. Example: !replica lounge", nil},
		{"host", "!replica programming", "I'm the host bot serving current channel. Example: !replica lounge", nil},
		{"duplicate", "!replica lounge", "Channel lounge already has a replica running.", []string{"lounge"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: tc.channels}}
			d, _ := commandDefinitionFor("replica")
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: tc.text, IsWhisper: true}).Execute(context.Background())
			if err != nil || status != model.FAILED {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if e.added != "" {
				t.Fatalf("unexpected add %q", e.added)
			}
			want := "alice|" + tc.want + "|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%v, want %q", e.chats, want)
			}
		})
	}
}

func TestReplicaOffAliasesRepliesAndFailures(t *testing.T) {
	for _, alias := range []string{"replicaoff", "offline", "botoff", "agentoff"} {
		t.Run(alias+" success", func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: []string{"lounge"}}}
			d, _ := commandDefinitionFor(alias)
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias + " lounge ignored", IsWhisper: true}).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL || e.removed != "lounge" {
				t.Fatalf("status=%v err=%v removed=%q", status, err, e.removed)
			}
			want := "alice|Successfully shut down replica in channel: lounge|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%v want %q", e.chats, want)
			}
		})
	}
	for _, tc := range []struct {
		name, text, want string
		channels         []string
	}{
		{"missing", "!replicaoff", "Example: !replicaoff lounge", nil},
		{"blank", "!replicaoff   ", "I'm the host bot serving current channel, not a replica.", nil},
		{"host", "!replicaoff programming", "I'm the host bot serving current channel, not a replica.", nil},
		{"absent", "!replicaoff lounge", "No replica in channel: lounge", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: tc.channels}}
			d, _ := commandDefinitionFor("replicaoff")
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: tc.text, IsWhisper: true}).Execute(context.Background())
			if err != nil || status != model.FAILED || e.removed != "" {
				t.Fatalf("status=%v err=%v removed=%q", status, err, e.removed)
			}
			want := "alice|" + tc.want + "|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%v want %q", e.chats, want)
			}
		})
	}
}

func TestReplicaStatusExactReplyForNoneAndSortedChannels(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		channels   []string
	}{
		{"none", "Host room:programming, replicas active: 0 \\nServing channels: none", []string{}},
		{"sorted", "Host room:programming, replicas active: 2 \\nServing channels: a, z", []string{"z", "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: tc.channels}}
			d, _ := commandDefinitionFor("status")
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!status ignored", IsWhisper: true}).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL {
				t.Fatalf("status=%v err=%v", status, err)
			}
			want := "alice|" + tc.want + "|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%q want %q", e.chats, want)
			}
		})
	}
}

func TestConcreteReplicaCommandsAreNotGenericFallbacks(t *testing.T) {
	for _, canonical := range []string{"replica", "replicaoff", "replicastatus"} {
		d, ok := commandDefinitionFor(canonical)
		if !ok {
			t.Fatalf("missing %s definition", canonical)
		}
		if _, generic := d.New(&replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}}, &model.ChatMessage{}).(*saturnCommand); generic {
			t.Errorf("%s remains generic", canonical)
		}
	}
}

func TestReplicaLifecycleFailureDoesNotClaimSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, canonical, text string
		controller            fakeRC
	}{
		{"add", "replica", "!replica lounge", fakeRC{channels: []string{}, addErr: errors.New("start failed")}},
		{"remove", "replicaoff", "!replicaoff lounge", fakeRC{channels: []string{"lounge"}, removeErr: errors.New("stop failed")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: tc.controller}
			d, _ := commandDefinitionFor(tc.canonical)
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: tc.text}).Execute(context.Background())
			if status != model.FAILED || err == nil {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if len(e.chats) != 0 {
				t.Fatalf("unexpected success chat=%v", e.chats)
			}
		})
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
