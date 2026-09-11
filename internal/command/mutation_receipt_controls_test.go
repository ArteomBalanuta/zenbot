package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type receiptIdentity struct {
	repository.IdentityRepository
	afterCommit context.CancelFunc
	sourceErr   error
}

func (r receiptIdentity) Register(ctx context.Context, name, trip string, role model.Role) error {
	if r.sourceErr != nil {
		return r.sourceErr
	}
	err := r.IdentityRepository.Register(ctx, name, trip, role)
	if err == nil && r.afterCommit != nil {
		r.afterCommit()
	}
	return err
}

func TestMutationReceiptSourceFailuresAndCancellation(t *testing.T) {
	db := h2fixture.Open(t, "mutation-receipt-controls")
	if _, err := db.DB.Exec("ALTER TABLE trips ADD CONSTRAINT reject_receipt_trip CHECK (trip <> 'RollbackTrip')"); err != nil {
		t.Fatal(err)
	}
	users := &service.UserService{Identity: db, GroupB: db}
	ctx, receipt := common.WithMutationRecorder(context.Background())
	if err := users.Register(ctx, "RollbackName", "RollbackTrip", model.REGULAR); err == nil {
		t.Fatal("expected constraint failure after name insertion")
	}
	var rows int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM names WHERE name='RollbackName'").Scan(&rows); err != nil || rows != 0 || receipt.Count() != 0 {
		t.Fatalf("rollback rows=%d receipts=%d err=%v", rows, receipt.Count(), err)
	}
	if err := users.RegisterNameByTrip(ctx, "MissingAlias", "MissingTrip"); err == nil || receipt.Count() != 0 {
		t.Fatalf("missing parent err=%v count=%d", err, receipt.Count())
	}
	if _, err := users.DeleteIdentity(ctx, "MissingName"); err == nil || receipt.Count() != 0 {
		t.Fatalf("missing identity err=%v count=%d", err, receipt.Count())
	}
	for _, sourceErr := range []error{errors.New("private storage failure"), repository.ErrCommitOutcomeUnknown, context.Canceled} {
		users.Identity = receiptIdentity{IdentityRepository: db, sourceErr: sourceErr}
		if err := users.Register(ctx, "Rejected", "RejectedTrip", model.REGULAR); !errors.Is(err, sourceErr) || receipt.Count() != 0 {
			t.Fatalf("err=%v count=%d", err, receipt.Count())
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	notes := &service.NoteService{DB: db.DB}
	if err := notes.Save(canceled, "Trip", "private"); !errors.Is(err, context.Canceled) || receipt.Count() != 0 {
		t.Fatalf("err=%v count=%d", err, receipt.Count())
	}
	if err := notes.Clear(ctx, "MissingTrip"); err != nil || receipt.Count() != 0 {
		t.Fatalf("empty purge err=%v count=%d", err, receipt.Count())
	}
	mail := &service.MailService{DB: db.DB}
	if err := mail.Queue(ctx, "private", "caller", "UnknownRecipient", true); !errors.Is(err, service.ErrMailRecipientUnregistered) || receipt.Count() != 0 {
		t.Fatalf("err=%v count=%d", err, receipt.Count())
	}
}

func TestMutationReceiptAliasAndCancellationAfterCommit(t *testing.T) {
	db := h2fixture.Open(t, "mutation-receipt-alias")
	ctx, receipt := common.WithMutationRecorder(context.Background())
	users := &service.UserService{Identity: db}
	if err := users.Register(ctx, "Name", "ExactTrip", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	if err := users.RegisterNameByTrip(ctx, "Alias", "ExactTrip"); err != nil {
		t.Fatal(err)
	}
	if err := users.RegisterTripByName(ctx, "Name", "SecondTrip"); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM trip_names").Scan(&rows); err != nil || rows != 3 || receipt.Count() != 3 {
		t.Fatalf("rows=%d receipts=%d err=%v", rows, receipt.Count(), err)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	users.Identity = receiptIdentity{IdentityRepository: db, afterCommit: cancel}
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: users}}}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "Trip", "", false, []string{}, []api.Capability{api.AdminCommands, api.ModerationCommands})
	got, err := NewAgentCommandGateway(e).Execute(cancelCtx, caller, "register", "CanceledName CanceledTrip")
	if err != nil || got.Status != commandgateway.OutcomeUnknown || got.Action == nil || got.Action.Count != 1 || e.sends != 0 {
		t.Fatalf("result=%+v sends=%d err=%v", got, e.sends, err)
	}
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM names WHERE name='CanceledName'").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
}

func TestMutationReceiptSourceErrorSurvivesErrorAcknowledgmentFailure(t *testing.T) {
	db := h2fixture.Open(t, "mutation-receipt-source-error")
	sourceErr := repository.ErrCommitOutcomeUnknown
	sendErr := errors.New("ack failed")
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: receiptIdentity{IdentityRepository: db, sourceErr: sourceErr}}}}, sendErr: sendErr}
	def, _ := commandDefinitionFor("register")
	ctx, receipt := common.WithMutationRecorder(context.Background())
	status, err := def.New(e, &model.ChatMessage{Name: "caller", Text: "!register Name Trip"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, sourceErr) || !errors.Is(err, sendErr) || receipt.Count() != 0 {
		t.Fatalf("status=%v err=%v count=%d", status, err, receipt.Count())
	}
}

func TestCommandReplyFailurePropagation(t *testing.T) {
	for _, tc := range []struct{ command, args string }{
		{"note", ""}, {"notes", ""}, {"mail", ""}, {"register", ""}, {"access", ""}, {"remove", ""}, {"sub", ""}, {"unsub", ""}, {"afk", ""}, {"info", ""}, {"list", ""}, {"ban", ""}, {"unban", ""}, {"mute", ""}, {"unmute", ""}, {"shadowban", ""}, {"unshadowban", ""}, {"captcha", ""}, {"lock", ""}, {"flair", ""}, {"color", ""}, {"prefix", ""}, {"msgroom", ""}, {"move", ""}, {"nuke", ""}, {"lastseen", ""}, {"t2n", ""}, {"shoot", ""}, {"say", "hello"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			sendErr := errors.New("ack failed")
			e := &gatewayEngine{commandEngineStub: commandEngineStub{}, sendErr: sendErr}
			def, ok := commandDefinitionFor(tc.command)
			if !ok {
				t.Fatalf("missing command %s", tc.command)
			}
			status, err := def.New(e, &model.ChatMessage{Name: "caller", Text: strings.TrimSpace("!" + tc.command + " " + tc.args)}).Execute(context.Background())
			if status != model.FAILED || !errors.Is(err, sendErr) || e.sends != 1 {
				t.Fatalf("status=%v err=%v sends=%d", status, err, e.sends)
			}
		})
	}
}

type receiptSequenceEngine struct {
	*gatewayEngine
	cancel    context.CancelFunc
	snapshots int
}

func (e *receiptSequenceEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.snapshots++
	request.OnComplete(snapshot.OperationResult{Outcome: snapshot.OutcomeSuccess})
	return nil
}

func (e *receiptSequenceEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	if e.sends == 1 {
		e.sendErr = errors.New("second delivery failed")
	}
	result, err := e.gatewayEngine.SendChatMessage(author, text, whisper)
	if e.cancel != nil {
		e.cancel()
	}
	return result, err
}

func TestMutationReceiptMultipleRepliesStopOnFailureOrCancellation(t *testing.T) {
	for _, cancelAfterFirst := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		e := &receiptSequenceEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{}}}
		if cancelAfterFirst {
			e.cancel = cancel
		}
		caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
		got, err := NewAgentCommandGateway(e).Execute(ctx, caller, "list", "")
		cancel()
		wantSends := 2
		if cancelAfterFirst {
			wantSends = 1
		}
		if err != nil || got.Status != commandgateway.OutcomeUnknown || got.Action == nil || got.Action.Count != 1 || got.Delivery == nil || got.Delivery.Count != 1 || len(got.Messages) != 1 || e.sends != wantSends || e.snapshots != 0 {
			t.Fatalf("cancel=%v result=%+v sends=%d err=%v", cancelAfterFirst, got, e.sends, err)
		}
	}
}

func TestMutationReceiptAuditCancellationAfterSuccessfulAcknowledgment(t *testing.T) {
	db := h2fixture.Open(t, "mutation-receipt-cancel-audit")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &mutationExitEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Notes: &service.NoteService{DB: db.DB}}}}, cancelAudit: cancel}
	caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
	got, err := NewAgentCommandGateway(e).Execute(ctx, caller, "note", "secret")
	if err != nil || got.Status != commandgateway.OutcomeUnknown || got.Action == nil || got.Action.Count != 2 || got.Delivery == nil || got.Delivery.Count != 1 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}
