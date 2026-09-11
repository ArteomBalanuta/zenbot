package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	agenttool "zenbot/internal/agent/tool"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/core"
	messagehandler "zenbot/internal/listener/message"
	"zenbot/internal/model"
)

func TestFinalIntegrationReviewedCanonicalAuthor(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, operand := range []string{"alice", "@@ALICE"} {
			t.Run(operand+map[bool]string{false: "/gateway", true: "/native"}[native], func(t *testing.T) {
				engine := newTargetBoundaryAuditWire("room", "alice", "@alice")
				caller, err := api.NewContextWithModerationTarget("room", "bot", "creator", "", false, []string{"alice", "@alice"}, []api.Capability{api.ModerationCommands}, "@alice")
				if err != nil {
					t.Fatal(err)
				}
				allowed := operand == "@@ALICE"
				if native {
					definition, _ := commandcatalog.AgentEntry("mute")
					arguments, _ := json.Marshal(map[string]string{"nick": operand})
					result, err := (agenttool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(engine)}).Execute(context.Background(), caller, arguments)
					if err != nil || result.IsError == allowed {
						t.Errorf("result=%+v err=%v allowed=%t", result, err, allowed)
					}
				} else {
					result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "mute", operand)
					if (result.Status == commandgateway.OutcomeSucceeded) != allowed {
						t.Errorf("result=%+v err=%v allowed=%t", result, err, allowed)
					}
				}
				nicks := targetBoundaryAuditWireNicks(t, engine)
				if allowed && !reflect.DeepEqual(nicks, []string{"@alice"}) || !allowed && len(nicks) != 0 {
					t.Fatalf("moderation targets=%v allowed=%t", nicks, allowed)
				}
			})
		}
	}
}

func TestFinalIntegrationActiveRosterOwnsExactNicknames(t *testing.T) {
	initial := []*model.User{{Name: "alice", Trip: "shared"}, {Name: "anon", Hash: "shared-hash"}, {Name: "@alice", Trip: "shared"}, {Name: "Alice", Trip: "shared"}, {Name: "alice", Trip: "new-trip"}}
	want := []string{"@alice", "Alice", "alice", "anon", "bob", "guest"}
	for _, replace := range []bool{true, false} {
		engine := &core.EngineImpl{Channel: "room", Prefix: "!", OutMessageQueue: make(chan string, 16)}
		if replace {
			engine.ReplaceActiveUsers(initial)
		} else {
			for _, u := range initial {
				engine.AddActiveUser(u)
			}
		}
		if len(*engine.GetActiveUsers()) != 4 {
			t.Errorf("replace=%t initial exact roster count=%d, want 4", replace, len(*engine.GetActiveUsers()))
		}
		engine.AddActiveUser(&model.User{Name: "bob", Trip: "shared"})
		engine.AddActiveUser(&model.User{Name: "guest", Hash: "shared-hash"})
		engine.AddActiveUser(&model.User{Name: "anon", Hash: "updated-hash"})
		users := *engine.GetActiveUsers()
		got := []string{}
		for u := range users {
			got = append(got, u.Name)
			if u.Name == "alice" && u.Trip != "new-trip" || u.Name == "anon" && u.Hash != "updated-hash" {
				t.Errorf("stale metadata: %+v", u)
			}
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("replace=%t roster=%v want=%v", replace, got, want)
		}
		directory := core.NewEngineRoomUserDirectory(engine, nil)
		caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
		result, err := (agenttool.RoomUsers{Directory: directory}).Execute(context.Background(), caller, json.RawMessage(`{}`))
		var observation struct {
			Users         []string `json:"users"`
			Count         int      `json:"count"`
			ReturnedCount int      `json:"returnedCount"`
		}
		if err != nil || json.Unmarshal([]byte(result.Content), &observation) != nil {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		sort.Strings(observation.Users)
		if observation.Count != 6 || observation.ReturnedCount != 6 || !reflect.DeepEqual(observation.Users, want) {
			t.Errorf("directory/tool roster=%+v", observation)
		}
		definition, _ := commandDefinitionFor("list")
		status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Text: "!list room"}).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil {
			t.Fatalf("local list status=%v err=%v", status, err)
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(<-engine.OutMessageQueue), &payload); err != nil {
			t.Fatal(err)
		}
		for _, name := range want {
			if !containsRosterName(payload.Text, name) {
				t.Errorf("local list missing %q: %s", name, payload.Text)
			}
		}
	}
}

func containsRosterName(text, name string) bool { // Exact output tokens, ignoring display punctuation.
	for _, field := range strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '`' }) {
		if field == name {
			return true
		}
	}
	return false
}

func TestFinalIntegrationAFKCommandPreservesSameTripAlias(t *testing.T) {
	for _, name := range []string{"bob", "@bob"} {
		engine := newTargetBoundaryAuditWire("room", name)
		engine.ReplaceActiveUsers([]*model.User{{Name: name, Trip: "Shared"}, {Name: "alice", Trip: "Shared"}})
		engine.AddAfkUser(&model.User{Name: "alice", Trip: "Shared"}, "alice reason")
		definition, _ := commandDefinitionFor("afk")
		status, err := definition.New(engine, &model.ChatMessage{Name: name, Trip: "Shared", Text: "!afk bob reason"}).Execute(context.Background())
		got := map[string]string{}
		for u, reason := range *engine.GetAfkUsers() {
			got[u.Name] = reason
		}
		if status != model.SUCCESSFUL || err != nil || !reflect.DeepEqual(got, map[string]string{"alice": "alice reason", name: "bob reason"}) {
			t.Fatalf("status=%v err=%v AFK=%v", status, err, got)
		}
		engine.AddAfkUser(&model.User{Name: name, Trip: "changed-trip"}, "updated reason")
		got = map[string]string{}
		for u, reason := range *engine.GetAfkUsers() {
			got[u.Name] = reason
			if u.Name == name && u.Trip != "changed-trip" {
				t.Errorf("stale AFK metadata=%+v", u)
			}
		}
		if !reflect.DeepEqual(got, map[string]string{"alice": "alice reason", name: "updated reason"}) {
			t.Errorf("exact nickname replacement changed alias: %v", got)
		}
	}
}

type finalAFKEngine struct {
	*core.EngineImpl
	afterAdd func()
	ackErr   error
}

func (e *finalAFKEngine) AddAfkUser(u *model.User, reason string) {
	e.EngineImpl.AddAfkUser(u, reason)
	if e.afterAdd != nil {
		e.afterAdd()
	}
}
func (e *finalAFKEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	if e.ackErr != nil {
		return "", e.ackErr
	}
	return e.EngineImpl.SendChatMessage(author, text, whisper)
}

func TestFinalIntegrationAFKRealOwnerRetainsReceiptsOnInterruptedAcknowledgment(t *testing.T) {
	for _, phase := range []string{"ack failure", "cancel after mutation", "credential case mismatch"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			engine := &finalAFKEngine{EngineImpl: newTargetBoundaryAuditWire("room")}
			engine.ReplaceActiveUsers([]*model.User{{Name: "@bob", Trip: "Shared"}, {Name: "alice", Trip: "Shared"}})
			engine.AddAfkUser(&model.User{Name: "alice", Trip: "Shared"}, "alice reason")
			trip := "Shared"
			switch phase {
			case "ack failure":
				engine.ackErr = errors.New("private ack error")
			case "cancel after mutation":
				engine.afterAdd = cancel
			case "credential case mismatch":
				trip = "shared"
			}
			caller, _ := api.NewContext("room", "@bob", trip, "", false, []string{})
			result, err := NewAgentCommandGateway(engine).Execute(ctx, caller, "afk", "bob reason")
			got := map[string]string{}
			for u, reason := range *engine.GetAfkUsers() {
				got[u.Name] = reason
			}
			if got["alice"] != "alice reason" {
				t.Fatalf("alias reason lost: %v", got)
			}
			if phase == "credential case mismatch" {
				if len(got) != 1 || result.Status != commandgateway.OutcomeRejected {
					t.Fatalf("mismatched credential changed state: result=%+v AFK=%v", result, got)
				}
			} else if err != nil || got["@bob"] != "bob reason" || len(got) != 2 || result.Status != commandgateway.OutcomeUnknown || result.Action == nil || result.Action.Count != 1 || result.Delivery != nil {
				t.Fatalf("mutation receipt/state lost: result=%+v err=%v AFK=%v", result, err, got)
			}
		})
	}
}

func TestFinalIntegrationAFKChatRequiresExactNameAndTrip(t *testing.T) {
	for _, tc := range []struct {
		name, trip string
		remove     bool
	}{
		{"bob", "Shared", false}, {"alice", "shared", false}, {"Alice", "Shared", false}, {"@alice", "Shared", false}, {"alice", "", false}, {"alice", "Shared", true},
	} {
		engine := newTargetBoundaryAuditWire("room")
		engine.AddAfkUser(&model.User{Name: "alice", Trip: "Shared"}, "original reason")
		_, err := (messagehandler.UpdateAfkState{}).Handle(context.Background(), &messagehandler.Context{Engine: engine, Author: &model.User{Name: tc.name, Trip: tc.trip}, Message: &model.ChatMessage{Name: tc.name, Text: "hello"}})
		if err != nil || (len(*engine.GetAfkUsers()) == 0) != tc.remove {
			t.Errorf("chat=%+v AFK=%v err=%v", tc, *engine.GetAfkUsers(), err)
		}
	}
	for _, trip := range []string{"", "different"} {
		engine := newTargetBoundaryAuditWire("room")
		engine.AddAfkUser(&model.User{Name: "@anon"}, "anonymous reason")
		_, _ = (messagehandler.UpdateAfkState{}).Handle(context.Background(), &messagehandler.Context{Engine: engine, Author: &model.User{Name: "@anon", Trip: trip}, Message: &model.ChatMessage{Name: "@anon", Text: "hello"}})
		if (len(*engine.GetAfkUsers()) == 0) != (trip == "") {
			t.Errorf("blank credential comparison trip=%q AFK=%v", trip, *engine.GetAfkUsers())
		}
	}
}
