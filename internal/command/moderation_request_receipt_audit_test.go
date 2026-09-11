package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/transport"
)

// This boundary accepts outbound frames but supplies no server application
// acknowledgment. The command still traverses the real core serializer.
type requestReceiptAuditTransport struct {
	frames       []string
	inboundReads int
}

func (*requestReceiptAuditTransport) Start(context.Context) error { return nil }
func (*requestReceiptAuditTransport) Connected() bool             { return true }
func (*requestReceiptAuditTransport) Close(context.Context) error { return nil }
func (s *requestReceiptAuditTransport) Messages() <-chan transport.InboundMessage {
	s.inboundReads++
	return nil
}
func (s *requestReceiptAuditTransport) Errors() <-chan error {
	s.inboundReads++
	return nil
}
func (s *requestReceiptAuditTransport) SendText(ctx context.Context, payload string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.frames = append(s.frames, payload)
	return nil
}
func (*requestReceiptAuditTransport) SendRaw(context.Context, []byte) error {
	return errors.New("unexpected raw transport entry")
}

type requestReceiptAuditEngine struct {
	*gatewayEngine
	wire *core.EngineImpl
}

func (e *requestReceiptAuditEngine) BanNick(ctx context.Context, nick common.NickTarget) error {
	return e.wire.BanNick(ctx, nick)
}

func TestModerationRequestReceiptAuditWriteOnlyBanClaimsSubmissionNotApplication(t *testing.T) {
	wire := &requestReceiptAuditTransport{}
	engine := &requestReceiptAuditEngine{
		gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"Merc": {Name: "Merc"}}}, authorized: true},
		wire:          &core.EngineImpl{Transport: wire},
	}
	caller, err := api.NewContextWithCapabilities("room", "mod", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "ban", "Merc")
	if err != nil || result.Status != commandgateway.OutcomeSucceeded || result.Action == nil || result.Action.Count != 2 || result.Delivery == nil || result.Delivery.Count != 1 {
		t.Fatalf("write-only operation did not return its actual request/delivery receipts: result=%+v err=%v", result, err)
	}
	if len(wire.frames) != 1 || wire.frames[0] != `{"cmd":"ban","nick":"Merc"}` || wire.inboundReads != 0 {
		t.Fatalf("unexpected source boundary: frames=%v inboundReads=%d", wire.frames, wire.inboundReads)
	}
	const want = "Permanent-ban request sent for Merc; server application is unconfirmed."
	if len(result.Messages) != 1 || result.Messages[0] != want {
		t.Errorf("outbound write alone claimed remote application: messages=%q want=%q", result.Messages, want)
	}
	for _, completedClaim := range []string{"has been banned", "was banned", "banned successfully"} {
		if strings.Contains(strings.ToLower(strings.Join(result.Messages, " ")), completedClaim) {
			t.Errorf("write-only receipt contains completed application claim %q: %q", completedClaim, result.Messages)
		}
	}
}
