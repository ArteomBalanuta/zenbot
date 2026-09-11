package command

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type unavailableAuditEngine struct{ *gatewayEngine }

func (*unavailableAuditEngine) CommandAvailable(string) bool { return false }

type admissionAuditController struct{ entered int }

func (*admissionAuditController) RequestRestart(context.Context) error  { return nil }
func (*admissionAuditController) RequestShutdown(context.Context) error { return nil }
func (c *admissionAuditController) BeginDispatch(context.Context) (func(), error) {
	c.entered++
	return nil, errors.New("host is retiring")
}

func TestGatewayAdmissionAuditMissingServiceHiddenAndNotRegistered(t *testing.T) {
	engine := &gatewayEngine{}
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	entry, _ := commandcatalog.AgentEntry("ping")
	commandTool := tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*engine.GetEnabledCommands())["ping"]; ok {
		t.Error("missing ping service was registered")
	}
	if commandTool.Authorized(caller) {
		t.Error("missing ping service remains visible")
	}
	result, err := commandTool.Execute(context.Background(), caller, []byte(`{}`))
	if err != nil || !result.IsError || result.ErrorCode == "ACTION_OUTCOME_UNKNOWN" || engine.sends != 0 {
		t.Fatalf("missing service not rejected before work: result=%+v err=%v sends=%d", result, err, engine.sends)
	}
	engine.bundle = &service.Bundle{Ping: auditPingService(t)}
	if !commandTool.Authorized(caller) {
		t.Fatal("configured service hidden")
	}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*engine.GetEnabledCommands())["ping"]; !ok {
		t.Fatal("configured ping not registered")
	}
}

func TestGatewayAdmissionAuditConfiguredControllersRemainUsable(t *testing.T) {
	caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands, api.ModerationCommands})
	replica := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}}
	lifecycle := &recordingLifecycleController{}
	host := &lifecycleCommandEngineStub{commandEngineStub: &commandEngineStub{}, controller: lifecycle}
	moderation := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"target": {Name: "target"}}}}
	for _, tc := range []struct {
		canonical, arguments string
		engine               common.Engine
	}{
		{"replica", "elsewhere", replica},
		{"restart", "", host},
		{"kick", "target", moderation},
	} {
		t.Run(tc.canonical, func(t *testing.T) {
			if err := RegisterUserUtilities(tc.engine); err != nil {
				t.Fatal(err)
			}
			if _, ok := (*tc.engine.GetEnabledCommands())[tc.canonical]; !ok {
				t.Fatal("configured command not registered")
			}
			entry, _ := commandcatalog.AgentEntry(tc.canonical)
			gateway := NewAgentCommandGateway(tc.engine)
			if !(tool.SaturnCommand{Definition: entry, Gateway: gateway}).Authorized(caller) {
				t.Fatal("configured controller hidden")
			}
			result, err := gateway.Execute(context.Background(), caller, tc.canonical, tc.arguments)
			if err != nil || result.Status != commandgateway.OutcomeSucceeded {
				t.Fatalf("configured source rejected: %+v err=%v", result, err)
			}
		})
	}
	if replica.added != "elsewhere" || lifecycle.restartCalls != 1 || len(moderation.raws) != 1 {
		t.Fatalf("source calls: replica=%q restart=%d moderation=%v", replica.added, lifecycle.restartCalls, moderation.raws)
	}
}

func TestGatewayAdmissionAuditVisibilityWithdrawalAndSingleResolution(t *testing.T) {
	first, second := &gatewayEngine{}, &gatewayEngine{}
	current := common.Engine(first)
	resolves := 0
	gateway := NewResolvingAgentCommandGateway(func() common.Engine { resolves++; return current })
	entry, _ := commandcatalog.AgentEntry("say")
	commandTool := tool.SaturnCommand{Definition: entry, Gateway: gateway}
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	if !commandTool.Authorized(caller) {
		t.Fatal("initial host hidden")
	}
	current = nil
	result, err := commandTool.Execute(context.Background(), caller, []byte(`{"message":"hello"}`))
	if err != nil || !result.IsError || result.ErrorCode == "ACTION_OUTCOME_UNKNOWN" || first.sends != 0 {
		t.Fatalf("withdrawal execution=%+v err=%v sends=%d", result, err, first.sends)
	}
	current = second
	resolves = 0
	result, err = commandTool.Execute(context.Background(), caller, []byte(`{"message":"hello"}`))
	if err != nil || result.IsError || resolves != 1 || first.sends != 0 || second.sends != 1 {
		t.Fatalf("replacement result=%+v err=%v resolves=%d sends=%d/%d", result, err, resolves, first.sends, second.sends)
	}
}

type leasedAuditEngine struct {
	*gatewayEngine
	controller common.HostLifecycleController
	onSend     func()
	onAudit    func()
}

func (e *leasedAuditEngine) HostLifecycleController() common.HostLifecycleController {
	return e.controller
}
func (e *leasedAuditEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	if e.onSend != nil {
		e.onSend()
	}
	return e.gatewayEngine.SendChatMessage(author, text, whisper)
}
func (e *leasedAuditEngine) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	if e.onAudit != nil {
		e.onAudit()
	}
	return 1, nil
}

type countingAuditLease struct {
	active, entered, released atomic.Int32
	onBegin                   func()
}

func (*countingAuditLease) RequestRestart(context.Context) error  { return nil }
func (*countingAuditLease) RequestShutdown(context.Context) error { return nil }
func (l *countingAuditLease) BeginDispatch(context.Context) (func(), error) {
	l.entered.Add(1)
	l.active.Add(1)
	if l.onBegin != nil {
		l.onBegin()
	}
	return func() { l.active.Add(-1); l.released.Add(1) }, nil
}

func TestGatewayAdmissionAuditLeaseCoversWorkAndAuditAndUnwinds(t *testing.T) {
	for _, outcome := range []string{"success", "source error", "cancel at admission", "cancel after send", "panic in audit"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			lease := &countingAuditLease{}
			engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{}, controller: lease}
			engine.onSend = func() {
				if lease.active.Load() != 1 {
					t.Error("send outside lease")
				}
				if outcome == "cancel after send" {
					cancel()
				}
			}
			engine.onAudit = func() {
				if lease.active.Load() != 1 {
					t.Error("audit outside lease")
				}
				if outcome == "panic in audit" {
					panic("audit panic")
				}
			}
			if outcome == "source error" {
				engine.sendErr = errors.New("socket failed")
			}
			if outcome == "cancel at admission" {
				lease.onBegin = cancel
			}
			caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
			result, _ := NewAgentCommandGateway(engine).Execute(ctx, caller, "say", "hello")
			if lease.entered.Load() != 1 || lease.active.Load() != 0 || lease.released.Load() != 1 {
				t.Fatalf("lease acquired=%d active=%d released=%d", lease.entered.Load(), lease.active.Load(), lease.released.Load())
			}
			if outcome == "cancel at admission" && (engine.sends != 0 || result.Status != commandgateway.OutcomeRejected) {
				t.Fatalf("canceled admission entered work: %+v sends=%d", result, engine.sends)
			}
			if outcome == "cancel after send" && (result.Status != commandgateway.OutcomeUnknown || result.Action == nil || result.Action.Count != 1) {
				t.Fatalf("source action lost after cancellation: %+v", result)
			}
		})
	}
}

type leasedSnapshotAuditEngine struct {
	*snapshotGatewayEngine
	lease *countingAuditLease
}

func (e *leasedSnapshotAuditEngine) HostLifecycleController() common.HostLifecycleController {
	return e.lease
}
func TestGatewayAdmissionAuditLeaseWaitsForSnapshotCompletion(t *testing.T) {
	lease := &countingAuditLease{}
	engine := &leasedSnapshotAuditEngine{snapshotGatewayEngine: &snapshotGatewayEngine{gatewayEngine: &gatewayEngine{}, requests: make(chan snapshot.RoomSnapshotRequest, 1)}, lease: lease}
	caller, _ := api.NewContext("programming", "caller", "trip", "hash", false, []string{})
	completed := make(chan CommandExecution, 1)
	go func() {
		result, _ := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "list", "elsewhere")
		completed <- result
	}()
	request := <-engine.requests
	if lease.active.Load() != 1 || lease.released.Load() != 0 {
		t.Error("snapshot operation escaped lease")
	}
	request.OnComplete(snapshot.OperationResult{Outcome: snapshot.OutcomeSuccess, DeliveryCount: 1, ActionCount: 1})
	<-completed
	if lease.active.Load() != 0 || lease.released.Load() != 1 {
		t.Fatal("snapshot lease was not released once")
	}
}

func TestGatewayAdmissionAuditRejectsRealOwnerLifecycleStates(t *testing.T) {
	for _, state := range []string{"pending", "active", "terminal", "closed"} {
		t.Run(state, func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			owner := core.NewHostLifecycle(func(context.Context) error { close(started); <-finish; return nil }, func(context.Context) error { return nil })
			defer owner.Close()
			defer close(finish)
			switch state {
			case "pending":
				release, _ := owner.BeginDispatch(context.Background())
				defer release()
				if err := owner.RequestRestart(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "active":
				if err := owner.RequestRestart(context.Background()); err != nil {
					t.Fatal(err)
				}
				<-started
			case "terminal":
				if err := owner.RequestShutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
				owner.Wait()
			case "closed":
				owner.Close()
			}
			engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{}, controller: owner}
			caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
			result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "say", "hello")
			if err == nil || result.Status != commandgateway.OutcomeRejected || engine.sends != 0 {
				t.Fatalf("%s host dispatched: %+v err=%v sends=%d", state, result, err, engine.sends)
			}
		})
	}
}

type admissionAuditEngine struct {
	*gatewayEngine
	controller *admissionAuditController
}

func (e *admissionAuditEngine) HostLifecycleController() common.HostLifecycleController {
	return e.controller
}

func TestGatewayAdmissionAuditUnavailableOwner(t *testing.T) {
	engine := &unavailableAuditEngine{&gatewayEngine{authorized: true}}
	gateway := NewAgentCommandGateway(engine)
	caller, err := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := commandcatalog.AgentEntry("say")
	if !ok {
		t.Fatal("missing say catalog entry")
	}
	if (tool.SaturnCommand{Definition: entry, Gateway: gateway}).Authorized(caller) {
		t.Error("tool remains visible when owning engine rejects availability")
	}
	result, err := gateway.Execute(context.Background(), caller, "say", "should not send")
	if result.Status != commandgateway.OutcomeRejected || err == nil || engine.sends != 0 {
		t.Fatalf("unavailable command executed: status=%s err=%v sends=%d", result.Status, err, engine.sends)
	}
}

func TestGatewayAdmissionAuditRetiringHost(t *testing.T) {
	controller := &admissionAuditController{}
	engine := &admissionAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: controller}
	caller, err := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "say", "should not send")
	if result.Status != commandgateway.OutcomeRejected || err == nil || engine.sends != 0 || controller.entered != 1 {
		t.Fatalf("retiring host entered: status=%s err=%v sends=%d admissions=%d", result.Status, err, engine.sends, controller.entered)
	}
}

func TestGatewayAdmissionAuditMissingResolvedHostHidden(t *testing.T) {
	caller, err := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := commandcatalog.AgentEntry("say")
	if !ok {
		t.Fatal("missing say catalog entry")
	}
	gateway := NewResolvingAgentCommandGateway(func() common.Engine { return nil })
	if (tool.SaturnCommand{Definition: entry, Gateway: gateway}).Authorized(caller) {
		t.Fatal("tool remains visible without a currently resolved host")
	}
}

func TestGatewayAdmissionAuditSourceSnapshotConfigurationIsRequired(t *testing.T) {
	master := &core.EngineImpl{Type: model.MASTER, EnabledCommands: map[string]common.CommandMetadata{}}
	engine := core.BindCredentialedRoomSnapshotMaster(master)
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	for _, canonical := range []string{"list", "msgchannel"} {
		if _, ok := (*engine.GetEnabledCommands())[canonical]; !ok {
			t.Errorf("local manual %s hidden without coordinator", canonical)
		}
		entry, _ := commandcatalog.AgentEntry(canonical)
		if (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Authorized(caller) {
			t.Errorf("remote %s visible without source coordinator", canonical)
		}
		result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, canonical, "elsewhere hello")
		if result.Status != commandgateway.OutcomeRejected || err == nil {
			t.Errorf("remote %s was not rejected before work: %+v err=%v", canonical, result, err)
		}
	}
	if _, ok := (*engine.GetEnabledCommands())["nuke"]; ok {
		t.Error("remote-only nuke registered without coordinator")
	}
}

type minimalAvailabilityEngine struct{ common.Engine }

func TestGatewayAdmissionAuditDoesNotTrustCaptureCapabilities(t *testing.T) {
	base := &gatewayEngine{}
	minimal := &minimalAvailabilityEngine{Engine: base}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands})
	for _, canonical := range []string{"kick", "replica", "nuke", "restart"} {
		capture := &agentCaptureEngine{Engine: minimal}
		entry, _ := commandcatalog.AgentEntry(canonical)
		gateway := NewAgentCommandGateway(capture)
		if (tool.SaturnCommand{Definition: entry, Gateway: gateway}).Authorized(caller) {
			t.Errorf("capture advertised unavailable %s", canonical)
		}
		result, err := gateway.Execute(context.Background(), caller, canonical, "target")
		if result.Status != commandgateway.OutcomeRejected || err == nil || base.sends != 0 {
			t.Errorf("capture dispatched unavailable %s: %+v err=%v sends=%d", canonical, result, err, base.sends)
		}
	}
	if err := RegisterUserUtilities(minimal); err != nil {
		t.Fatal(err)
	}
	for _, canonical := range []string{"kick", "replica", "nuke", "restart"} {
		if _, ok := (*minimal.GetEnabledCommands())[canonical]; ok {
			t.Errorf("minimal registered %s", canonical)
		}
	}
}

func TestGatewayAdmissionAuditRechecksDependencyAfterVisibilityAndLease(t *testing.T) {
	lease := &countingAuditLease{}
	engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{}, controller: lease}
	engine.bundle = &service.Bundle{Ping: auditPingService(t)}
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	entry, _ := commandcatalog.AgentEntry("ping")
	commandTool := tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}
	if !commandTool.Authorized(caller) {
		t.Fatal("configured ping invisible")
	}
	lease.onBegin = func() { engine.bundle = nil }
	result, err := commandTool.Execute(context.Background(), caller, []byte(`{}`))
	if err != nil || !result.IsError || result.ErrorCode == "ACTION_OUTCOME_UNKNOWN" || engine.sends != 0 || lease.released.Load() != 1 {
		t.Fatalf("dependency withdrawal ignored: %+v err=%v sends=%d released=%d", result, err, engine.sends, lease.released.Load())
	}
}

func TestGatewayAdmissionAuditPinsHostWhileLeaseReplacesResolution(t *testing.T) {
	lease := &countingAuditLease{}
	first := &leasedAuditEngine{gatewayEngine: &gatewayEngine{}, controller: lease}
	second := &gatewayEngine{}
	current := common.Engine(first)
	lease.onBegin = func() { current = second }
	resolves := 0
	gateway := NewResolvingAgentCommandGateway(func() common.Engine { resolves++; return current })
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	result, err := gateway.Execute(context.Background(), caller, "say", "hello")
	if err != nil || result.Status != commandgateway.OutcomeSucceeded || first.sends != 1 || second.sends != 0 || resolves != 1 {
		t.Fatalf("host changed within execution: %+v err=%v sends=%d/%d resolves=%d", result, err, first.sends, second.sends, resolves)
	}
}
