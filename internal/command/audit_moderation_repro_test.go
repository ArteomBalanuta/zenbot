package command

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

// Temporary audit regressions: assert intended outcomes, currently failing.
func TestAuditModerationUnshadowAllReportsSuccessfulDeletion(t *testing.T) {
	repo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Name: "raider"}}, removeAllCount: 1}
	e := newShadowBanEngine(repo, nil)
	d, _ := commandDefinitionFor("unshadowban")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!unshadowban -all"}).Execute(context.Background())
	if repo.removeAlls != 1 || status != model.SUCCESSFUL || err != nil {
		t.Fatalf("deleted=%d status=%v err=%v", repo.removeAlls, status, err)
	}
}

type auditNukeGatewayEngine struct{ *commandEngineStub }

func (e *auditNukeGatewayEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	result, err := request.Operation.Apply(snapshot.RoomSnapshotContext{TargetChannel: request.TargetChannel, SendRaw: func(raw string) error {
		e.raws = append(e.raws, raw)
		return nil
	}}, snapshot.Snapshot{Users: []*model.User{{Name: "innocent"}}})
	if request.OnComplete != nil {
		request.OnComplete(result)
	}
	return err
}

func TestAuditModerationNukeRequiresPermanentBanCapability(t *testing.T) {
	e := &auditNukeGatewayEngine{commandEngineStub: &commandEngineStub{}}
	caller, err := api.NewContextWithModerationTarget("room", "bot", "creator-trip", "", false, []string{"reviewed", "innocent"}, []api.Capability{api.ModerationCommands}, "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "nuke", "other-room")
	if result.Status != commandgateway.OutcomeRejected || len(e.raws) != 0 {
		t.Fatalf("status=%v err=%v raws=%v", result.Status, err, e.raws)
	}
}

type auditCancellingIdentity struct {
	*identityFake
	cancel context.CancelFunc
}

func (r *auditCancellingIdentity) IsTripRegistered(context.Context, string) (bool, error) {
	r.cancel()
	return false, nil
}

func TestAuditModerationRegisterDoesNotMutateAfterLookupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &auditCancellingIdentity{identityFake: &identityFake{}, cancel: cancel}
	e := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: r}}}
	d, _ := commandDefinitionFor("register")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!register alice trip"}).Execute(ctx)
	if len(r.registered) != 0 || status != model.FAILED || err != context.Canceled {
		t.Fatalf("writes=%v status=%v err=%v", r.registered, status, err)
	}
}
