package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type utilityBoundaryPrefixEngine struct {
	*commandEngineStub
	prefix string
}

func (e *utilityBoundaryPrefixEngine) GetPrefix() string { return e.prefix }

type utilityBoundaryLookupEngine struct {
	*commandEngineStub
	lookup func(string) *model.User
}

func (e *utilityBoundaryLookupEngine) GetActiveUserByName(name string) *model.User {
	if e.lookup != nil {
		return e.lookup(name)
	}
	return e.commandEngineStub.GetActiveUserByName(name)
}

type utilityBoundaryGatewayEngine struct {
	*gatewayEngine
	lookup func(string) *model.User
}

func (e *utilityBoundaryGatewayEngine) GetActiveUserByName(name string) *model.User {
	if e.lookup != nil {
		return e.lookup(name)
	}
	return e.gatewayEngine.GetActiveUserByName(name)
}

type utilityBoundarySendErrorEngine struct {
	*commandEngineStub
	err error
}

func (e *utilityBoundarySendErrorEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	e.chats = append(e.chats, author+"|"+text+"|"+boolString(whisper))
	return "", e.err
}

type utilityBoundaryRoomEngine struct {
	*remoteMsgChannelEngine
	channel string
}

func (e *utilityBoundaryRoomEngine) GetChannel() string { return e.channel }

func TestUtilityBoundaryAuditNotesRejectsTrailingPurgeOperands(t *testing.T) {
	for _, text := range []string{"!notes purge keep-this", "!notes clear unexpected"} {
		t.Run(text, func(t *testing.T) {
			engine := openNotesParityEngine(t)
			ctx := context.Background()
			if err := engine.bundle.Notes.Save(ctx, "Trip", "must survive malformed input"); err != nil {
				t.Fatal(err)
			}
			definition, ok := commandDefinitionFor("notes")
			if !ok {
				t.Fatal("registered notes definition missing")
			}
			status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: text}).Execute(ctx)
			remaining, readErr := engine.bundle.Notes.List(ctx, "Trip")
			if readErr != nil || len(remaining) != 1 {
				t.Errorf("malformed purge changed stored notes: remaining=%v err=%v", remaining, readErr)
			}
			if status == model.SUCCESSFUL {
				t.Errorf("malformed purge reported success: status=%v err=%v", status, err)
			}
			if len(engine.chats) != 1 || !strings.Contains(engine.chats[0], engine.GetPrefix()+"notes") {
				t.Errorf("malformed purge did not explain configured syntax: chats=%v", engine.chats)
			}
		})
	}
}

func TestUtilityBoundaryAuditNotesValidGrammarAndExactOwner(t *testing.T) {
	engine := openNotesParityEngine(t)
	ctx := context.Background()
	for trip, note := range map[string]string{"ExactTrip": "owned", "exacttrip": "other"} {
		if err := engine.bundle.Notes.Save(ctx, trip, note); err != nil {
			t.Fatal(err)
		}
	}
	definition, _ := commandDefinitionFor("notes")
	status, err := definition.New(engine, &model.ChatMessage{Name: "Caller", Trip: "ExactTrip", Text: "!notes"}).Execute(ctx)
	if status != model.SUCCESSFUL || err != nil || len(engine.chats) != 1 || !strings.Contains(engine.chats[0], "[owned]") || strings.Contains(engine.chats[0], "other") {
		t.Fatalf("exact-owner list status=%v err=%v chats=%v", status, err, engine.chats)
	}
	for _, operation := range []string{"purge", "clear"} {
		engine.chats = nil
		if err := engine.bundle.Notes.Save(ctx, "ExactTrip", operation); err != nil {
			t.Fatal(err)
		}
		status, err = definition.New(engine, &model.ChatMessage{Name: "Caller", Trip: "ExactTrip", Text: "!notes " + operation}).Execute(ctx)
		remaining, listErr := engine.bundle.Notes.List(ctx, "ExactTrip")
		other, otherErr := engine.bundle.Notes.List(ctx, "exacttrip")
		if status != model.SUCCESSFUL || err != nil || listErr != nil || len(remaining) != 0 || otherErr != nil || len(other) != 1 || other[0] != "other" {
			t.Fatalf("%s status=%v err=%v owned=%v/%v other=%v/%v", operation, status, err, remaining, listErr, other, otherErr)
		}
	}
}

func TestUtilityBoundaryAuditMalformedNotesUsesConfiguredPrefixAndFailedDeliveryHasNoReceipt(t *testing.T) {
	base := openNotesParityEngine(t)
	if err := base.bundle.Notes.Save(context.Background(), "Trip", "keep"); err != nil {
		t.Fatal(err)
	}
	prefixed := &utilityBoundaryPrefixEngine{commandEngineStub: base, prefix: "$"}
	definition, _ := commandDefinitionFor("notes")
	status, err := definition.New(prefixed, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: "!notes purge extra"}).Execute(context.Background())
	if status != model.FAILED || err != nil || len(prefixed.chats) != 1 || !strings.Contains(prefixed.chats[0], "$notes") {
		t.Fatalf("status=%v err=%v chats=%v", status, err, prefixed.chats)
	}
	prefixed.chats = nil
	status, err = definition.New(prefixed, &model.ChatMessage{Name: "caller", Text: "!notes"}).Execute(context.Background())
	if status != model.FAILED || err != nil || len(prefixed.chats) != 1 || !strings.Contains(prefixed.chats[0], "$notes") {
		t.Fatalf("trip explanation status=%v err=%v chats=%v", status, err, prefixed.chats)
	}

	sendErr := errors.New("usage delivery failed")
	failing := &utilityBoundarySendErrorEngine{commandEngineStub: base, err: sendErr}
	ctx, receipt := common.WithMutationRecorder(context.Background())
	status, err = definition.New(failing, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: "!notes clear extra"}).Execute(ctx)
	remaining, listErr := base.bundle.Notes.List(context.Background(), "Trip")
	if status != model.FAILED || !errors.Is(err, sendErr) || receipt.Count() != 0 || listErr != nil || len(remaining) != 1 || remaining[0] != "keep" {
		t.Fatalf("status=%v err=%v receipts=%d remaining=%v listErr=%v", status, err, receipt.Count(), remaining, listErr)
	}
}

func TestUtilityBoundaryAuditAFKRequiresTheActiveCaller(t *testing.T) {
	for _, active := range []map[string]*model.User{
		{},
		{"other": {Name: "other", Trip: "Trip"}},
		{"caller": {Name: "caller", Trip: "DifferentTrip"}},
		{"caller": {Name: "caller", Trip: " "}},
	} {
		engine := &commandEngineStub{users: active}
		definition, ok := commandDefinitionFor("afk")
		if !ok {
			t.Fatal("registered afk definition missing")
		}
		status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: "!afk lunch"}).Execute(context.Background())
		if len(*engine.GetAfkUsers()) != 0 {
			t.Errorf("absent/mismatched caller changed another occupant's AFK state: users=%v", active)
		}
		if status == model.SUCCESSFUL {
			t.Errorf("absent/mismatched caller reported AFK success: users=%v status=%v err=%v", active, status, err)
		}
	}
}

func TestUtilityBoundaryAuditAFKUsesResolvedCanonicalCallerOnly(t *testing.T) {
	caller := &model.User{Name: "CaLlEr", Trip: "ExactTrip", Hash: "caller-hash"}
	other := &model.User{Name: "other", Trip: "ExactTrip", Hash: "other-hash"}
	base := &commandEngineStub{users: map[string]*model.User{"canonical": caller, "other": other}}
	engine := &utilityBoundaryLookupEngine{commandEngineStub: base}
	engine.lookup = func(name string) *model.User {
		for _, user := range base.users {
			if strings.EqualFold(user.Name, strings.TrimSpace(name)) {
				return user
			}
		}
		return nil
	}
	definition, _ := commandDefinitionFor("afk")
	status, err := definition.New(engine, &model.ChatMessage{Name: "CALLER", Trip: "ExactTrip", Text: "!afk line  one"}).Execute(context.Background())
	afks := *engine.GetAfkUsers()
	if status != model.SUCCESSFUL || err != nil || len(afks) != 1 || afks[caller] != "line  one" {
		t.Fatalf("status=%v err=%v afks=%v", status, err, afks)
	}
	if _, changed := afks[other]; changed {
		t.Fatalf("same-trip non-caller was marked AFK: %v", afks)
	}
}

func TestUtilityBoundaryAuditAFKChecksCancellationAroundLookup(t *testing.T) {
	definition, _ := commandDefinitionFor("afk")
	t.Run("before lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		lookups := 0
		engine := &utilityBoundaryLookupEngine{commandEngineStub: &commandEngineStub{}}
		engine.lookup = func(string) *model.User { lookups++; return nil }
		status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: "!afk"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || lookups != 0 || len(*engine.GetAfkUsers()) != 0 || len(engine.chats) != 0 {
			t.Fatalf("status=%v err=%v lookups=%d afks=%v chats=%v", status, err, lookups, *engine.GetAfkUsers(), engine.chats)
		}
	})
	t.Run("after lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		caller := &model.User{Name: "caller", Trip: "Trip"}
		engine := &utilityBoundaryLookupEngine{commandEngineStub: &commandEngineStub{}}
		engine.lookup = func(string) *model.User { cancel(); return caller }
		status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Trip: "Trip", Text: "!afk"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || len(*engine.GetAfkUsers()) != 0 || len(engine.chats) != 0 {
			t.Fatalf("status=%v err=%v afks=%v chats=%v", status, err, *engine.GetAfkUsers(), engine.chats)
		}
	})
}

func TestUtilityBoundaryAuditFailedAFKExplanationPropagatesWithoutMutation(t *testing.T) {
	sendErr := errors.New("explanation failed")
	engine := &utilityBoundarySendErrorEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"other": {Name: "other", Trip: "Trip"}}}, err: sendErr}
	definition, _ := commandDefinitionFor("afk")
	ctx, receipt := common.WithMutationRecorder(context.Background())
	status, err := definition.New(engine, &model.ChatMessage{Name: "missing", Trip: "Trip", Text: "!afk"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, sendErr) || receipt.Count() != 0 || len(*engine.GetAfkUsers()) != 0 {
		t.Fatalf("status=%v err=%v receipts=%d afks=%v", status, err, receipt.Count(), *engine.GetAfkUsers())
	}
}

func TestUtilityBoundaryAuditGatewayKeepsActualMutationReceiptsAfterAckFailure(t *testing.T) {
	callerContext, err := api.NewContext("programming", "caller", "Trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("notes purge", func(t *testing.T) {
		base := openNotesParityEngine(t)
		if err := base.bundle.Notes.Save(context.Background(), "Trip", "delete"); err != nil {
			t.Fatal(err)
		}
		engine := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: base.bundle}, authorized: true, sendErr: errors.New("ack failed")}
		result, executeErr := NewAgentCommandGateway(engine).Execute(context.Background(), callerContext, "notes", "purge")
		remaining, listErr := base.bundle.Notes.List(context.Background(), "Trip")
		if executeErr != nil || result.Status != commandgateway.OutcomeUnknown || result.Action == nil || result.Action.Count != 1 || result.Delivery != nil || listErr != nil || len(remaining) != 0 {
			t.Fatalf("result=%+v executeErr=%v remaining=%v listErr=%v", result, executeErr, remaining, listErr)
		}
	})
	t.Run("afk canonical caller", func(t *testing.T) {
		caller := &model.User{Name: "caller", Trip: "Trip", Hash: "canonical"}
		other := &model.User{Name: "other", Trip: "Trip", Hash: "other"}
		base := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": caller, "other": other}}, authorized: true, sendErr: errors.New("ack failed")}
		engine := &utilityBoundaryGatewayEngine{gatewayEngine: base, lookup: func(name string) *model.User {
			if strings.EqualFold(name, caller.Name) {
				return caller
			}
			return nil
		}}
		result, executeErr := NewAgentCommandGateway(engine).Execute(context.Background(), callerContext, "afk", "away")
		afks := *engine.GetAfkUsers()
		if executeErr != nil || result.Status != commandgateway.OutcomeUnknown || result.Action == nil || result.Action.Count != 1 || result.Delivery != nil || len(afks) != 1 || afks[caller] != "away" {
			t.Fatalf("result=%+v executeErr=%v afks=%v", result, executeErr, afks)
		}
		if _, changed := afks[other]; changed {
			t.Fatalf("same-trip non-caller was marked AFK: %v", afks)
		}
	})
}

func TestUtilityBoundaryAuditRoomSelectorPreservesLiteralDestination(t *testing.T) {
	for _, tc := range []struct {
		command, text, room string
	}{
		{"msgchannel", "!msgchannel other?room hello", "other?room"},
		{"msgchannel", "!msgchannel ?other?room hello", "other?room"},
		{"msgchannel", "!msgchannel ?MiXeD?東 hello", "MiXeD?東"},
		{"list", "!list ?lounge", "lounge"},
		{"list", "!list ?Lounge?東", "Lounge?東"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
			definition, ok := commandDefinitionFor(tc.command)
			if !ok {
				t.Fatal("registered definition missing")
			}
			status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Text: tc.text}).Execute(context.Background())
			if status != model.SUCCESSFUL || err != nil || len(engine.requests) != 1 {
				t.Fatalf("status=%v err=%v requests=%v", status, err, engine.requests)
			}
			if got := engine.requests[0].TargetChannel; got != tc.room {
				t.Errorf("room selector changed destination: got %q, want %q", got, tc.room)
			}
		})
	}
}

func TestUtilityBoundaryAuditRoomSelectorLocalAndInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		command, text string
	}{
		{"list", "!list ?"},
		{"list", "!list lounge extra"},
		{"msgchannel", "!msgchannel ? hello"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
			definition, _ := commandDefinitionFor(tc.command)
			status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Text: tc.text}).Execute(context.Background())
			if status != model.FAILED || err != nil || len(engine.requests) != 0 || len(engine.raws) != 0 || len(engine.chats) != 1 {
				t.Fatalf("status=%v err=%v requests=%v raws=%v chats=%v", status, err, engine.requests, engine.raws, engine.chats)
			}
		})
	}

	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	definition, _ := commandDefinitionFor("list")
	status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Text: "!list ?programming"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(engine.requests) != 0 || len(engine.chats) != 1 {
		t.Fatalf("local marker status=%v err=%v requests=%v chats=%v", status, err, engine.requests, engine.chats)
	}
}

func TestUtilityBoundaryAuditMsgChannelPreservesSourceChannelBytes(t *testing.T) {
	base := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	engine := &utilityBoundaryRoomEngine{remoteMsgChannelEngine: base, channel: "Pro?Gram東"}
	definition, _ := commandDefinitionFor("msgchannel")
	status, err := definition.New(engine, &model.ChatMessage{Name: "caller", Text: "!msgchannel ?remote hello"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(engine.requests) != 1 {
		t.Fatalf("status=%v err=%v requests=%v", status, err, engine.requests)
	}
	request := engine.requests[0]
	if request.SourceChannel != "Pro?Gram東" || request.TargetChannel != "remote" || request.RemoteMessage != "anonymous mail from: ?Pro?Gram東 message: hello" {
		t.Fatalf("request=%+v", request)
	}
}

var _ common.Engine = (*utilityBoundaryLookupEngine)(nil)
var _ common.Engine = (*utilityBoundaryRoomEngine)(nil)
