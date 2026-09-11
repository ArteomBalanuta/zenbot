package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/model"
)

func TestLifecycleRequestAuditAcceptedRequestRetainsAdmissionEvidence(t *testing.T) {
	for _, canonical := range []string{"restart", "shutdown"} {
		t.Run(canonical, func(t *testing.T) {
			called := make(chan struct{}, 1)
			callback := func(context.Context) error { called <- struct{}{}; return nil }
			owner := core.NewHostLifecycle(callback, callback)
			t.Cleanup(owner.Close)
			// An existing dispatch keeps retirement pending while the command
			// submits its request and returns its synchronous acknowledgment.
			release, err := owner.BeginDispatch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: owner}
			caller, err := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := commandcatalog.AgentEntry(canonical)
			if !ok {
				t.Fatal("lifecycle entry absent")
			}
			commandTool := tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}
			result, err := commandTool.Execute(context.Background(), caller, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || result.ActionCount != 2 || result.DeliveryCount != 1 || engine.sends != 1 {
				t.Errorf("accepted request lacks truthful request/delivery evidence: result=%+v sends=%d", result, engine.sends)
			}
			var envelope struct {
				Data struct {
					Data map[string]any `json:"data"`
				} `json:"data"`
			}
			if err := json.Unmarshal(result.Envelope(), &envelope); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"operation": canonical, "scope": "host", "requestStatus": "accepted", "completionStatus": "not_observed"} {
				if envelope.Data.Data[key] != want {
					t.Errorf("admission %s=%v want %q; envelope=%s", key, envelope.Data.Data[key], want, result.Envelope())
				}
			}
			if !strings.Contains(result.Content, "accepted") || !strings.Contains(result.Content, "completion is unconfirmed") {
				t.Errorf("acknowledgment does not distinguish admission and completion: %s", result.Content)
			}
			descriptor, err := commandTool.Descriptor(caller)
			if err != nil || contract.ValidateResult(descriptor.ResultSchema(), []byte(result.Content)) != nil {
				t.Errorf("admission result does not validate: %s err=%v", result.Content, err)
			}
			select {
			case <-called:
				t.Error("retirement ran while an existing dispatch was still active")
			default:
			}
		})
	}
}

type lifecycleAckAuditEngine struct {
	*leasedAuditEngine
	afterSend func()
}

func (e *lifecycleAckAuditEngine) SendChatMessage(author, message string, whisper bool) (string, error) {
	result, err := e.gatewayEngine.SendChatMessage(author, message, whisper)
	if e.afterSend != nil {
		e.afterSend()
	}
	return result, err
}

func TestLifecycleRequestAuditAdmissionSurvivesAcknowledgmentFailureAndCancellation(t *testing.T) {
	for _, canonical := range []string{"restart", "shutdown"} {
		for _, cancelAfterAck := range []bool{false, true} {
			t.Run(fmtLifecycleCase(canonical, cancelAfterAck), func(t *testing.T) {
				owner := core.NewHostLifecycle(func(context.Context) error { return nil }, func(context.Context) error { return nil })
				t.Cleanup(owner.Close)
				release, err := owner.BeginDispatch(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				engine := &lifecycleAckAuditEngine{leasedAuditEngine: &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: owner}}
				wantActions, wantDeliveries := 1, 0
				if cancelAfterAck {
					engine.afterSend = cancel
					wantActions, wantDeliveries = 2, 1
				} else {
					engine.sendErr = errors.New("private transport credential diagnostic")
				}
				caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
				entry, _ := commandcatalog.AgentEntry(canonical)
				result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, []byte(`{}`))
				if err != nil || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.ActionCount != wantActions || result.DeliveryCount != wantDeliveries || !result.EffectsCommitted {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				if !strings.Contains(string(result.Envelope()), `"requestStatus":"accepted"`) || !strings.Contains(string(result.Envelope()), `"completionStatus":"not_observed"`) || strings.Contains(string(result.Envelope()), "credential diagnostic") {
					t.Fatalf("admission evidence missing or unsafe: %s", result.Envelope())
				}
			})
		}
	}
}

func fmtLifecycleCase(operation string, cancelled bool) string {
	if cancelled {
		return operation + "/cancel_after_ack"
	}
	return operation + "/failed_ack"
}

func TestLifecycleRequestAuditCancelledBeforeAdmissionHasNoEffects(t *testing.T) {
	owner := core.NewHostLifecycle(func(context.Context) error { t.Error("cancelled restart ran"); return nil }, nil)
	t.Cleanup(owner.Close)
	engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: owner}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
	entry, _ := commandcatalog.AgentEntry("restart")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, []byte(`{}`))
	if err != nil || result.EffectState != contract.EffectNotStarted || result.ActionCount != 0 || result.DeliveryCount != 0 || engine.sends != 0 {
		t.Fatalf("cancelled request=%+v err=%v", result, err)
	}
}

func TestLifecycleRequestAuditQueueMutationCountsCoalescingAndSupersession(t *testing.T) {
	owner := core.NewHostLifecycle(func(context.Context) error { return nil }, func(context.Context) error { return nil })
	t.Cleanup(owner.Close)
	release, err := owner.BeginDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, receipt := common.WithMutationRecorder(context.Background())
	first, err := owner.RequestRestartResult(ctx)
	if err != nil || first.Coalesced || receipt.Count() != 1 {
		t.Fatalf("first=%+v receipt=%d err=%v", first, receipt.Count(), err)
	}
	ctx, receipt = common.WithMutationRecorder(context.Background())
	second, err := owner.RequestRestartResult(ctx)
	if err != nil || !second.Coalesced || first.Operation != second.Operation || receipt.Count() != 0 {
		t.Fatalf("coalesced=%+v receipt=%d err=%v", second, receipt.Count(), err)
	}
	ctx, receipt = common.WithMutationRecorder(context.Background())
	shutdown, err := owner.RequestShutdownResult(ctx)
	if err != nil || shutdown.Coalesced || receipt.Count() != 1 {
		t.Fatalf("supersession=%+v receipt=%d err=%v", shutdown, receipt.Count(), err)
	}
	if result, finished := first.Operation.Result(); !finished || !errors.Is(result.Err, core.ErrHostLifecycleSuperseded) {
		t.Fatalf("superseded result=%+v finished=%v", result, finished)
	}
	ctx, receipt = common.WithMutationRecorder(context.Background())
	secondShutdown, err := owner.RequestShutdownResult(ctx)
	if err != nil || !secondShutdown.Coalesced || secondShutdown.Operation != shutdown.Operation || receipt.Count() != 0 {
		t.Fatalf("coalesced shutdown=%+v receipt=%d err=%v", secondShutdown, receipt.Count(), err)
	}
}

// Both gateways acquire real owner leases before either command can submit.
// This models concurrent requests without bypassing the pending-host gate.
type concurrentLifecycleAuditEngine struct {
	*leasedAuditEngine
	mu       sync.Mutex
	arrivals int
	ready    chan struct{}
}

func (e *concurrentLifecycleAuditEngine) GetPrefix() string {
	e.mu.Lock()
	e.arrivals++
	if e.arrivals == 2 {
		close(e.ready)
	}
	e.mu.Unlock()
	<-e.ready
	return "!"
}

func (e *concurrentLifecycleAuditEngine) SendChatMessage(author, message string, whisper bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gatewayEngine.SendChatMessage(author, message, whisper)
}

func TestLifecycleRequestAuditConcurrentAdmissionsCoalesceWithSeparateAcknowledgments(t *testing.T) {
	for _, canonical := range []string{"restart", "shutdown"} {
		t.Run(canonical, func(t *testing.T) {
			owner := core.NewHostLifecycle(func(context.Context) error { return nil }, func(context.Context) error { return nil })
			t.Cleanup(owner.Close)
			release, err := owner.BeginDispatch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			engine := &concurrentLifecycleAuditEngine{leasedAuditEngine: &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: owner}, ready: make(chan struct{})}
			caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
			entry, _ := commandcatalog.AgentEntry(canonical)
			results := make(chan contract.Result, 2)
			for range 2 {
				go func() {
					result, _ := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(context.Background(), caller, []byte(`{}`))
					results <- result
				}()
			}
			statuses := map[string]int{}
			for range 2 {
				result := <-results
				var content struct {
					Data struct {
						RequestStatus string `json:"requestStatus"`
					} `json:"data"`
				}
				if err := json.Unmarshal([]byte(result.Content), &content); err != nil {
					t.Fatal(err)
				}
				statuses[content.Data.RequestStatus]++
				wantActions := 2
				if content.Data.RequestStatus == "coalesced" {
					wantActions = 1
				}
				if result.IsError || result.ActionCount != wantActions || result.DeliveryCount != 1 || !strings.Contains(result.Content, content.Data.RequestStatus) || !strings.Contains(result.Content, "completion is unconfirmed") {
					t.Fatalf("concurrent result=%+v", result)
				}
			}
			if statuses["accepted"] != 1 || statuses["coalesced"] != 1 || engine.sends != 2 {
				t.Fatalf("admissions=%v sends=%d", statuses, engine.sends)
			}
		})
	}
}

type lifecycleSnapshotAuditOperation struct {
	result   common.LifecycleResult
	finished bool
	reads    int
}

func (o *lifecycleSnapshotAuditOperation) Done() <-chan struct{} {
	panic("dispatcher must not wait for lifecycle completion")
}
func (o *lifecycleSnapshotAuditOperation) Result() (common.LifecycleResult, bool) {
	o.reads++
	return o.result, o.finished
}

type lifecycleSnapshotAuditController struct {
	*recordingLifecycleController
	admission      common.LifecycleAdmission
	afterAdmission func()
}

func (c *lifecycleSnapshotAuditController) RequestRestartResult(context.Context) (common.LifecycleAdmission, error) {
	if c.afterAdmission != nil {
		c.afterAdmission()
	}
	return c.admission, nil
}
func (c *lifecycleSnapshotAuditController) RequestShutdownResult(ctx context.Context) (common.LifecycleAdmission, error) {
	return c.RequestRestartResult(ctx)
}

func TestLifecycleRequestAuditReadsCompletionOnceAndOnlyProjectsFinishedResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		finished  bool
		sourceErr error
		want      string
	}{
		{"pending", false, nil, "not_observed"},
		{"pending error is not completion", false, errors.New("credential secret"), "not_observed"},
		{"finished", true, nil, "succeeded"},
		{"failed", true, errors.New("credential secret"), "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation := &lifecycleSnapshotAuditOperation{result: common.LifecycleResult{Kind: "restart", Err: tc.sourceErr}, finished: tc.finished}
			controller := &lifecycleSnapshotAuditController{recordingLifecycleController: &recordingLifecycleController{}, admission: common.LifecycleAdmission{Operation: operation}}
			capture := &agentCaptureEngine{Engine: &leasedAuditEngine{gatewayEngine: &gatewayEngine{}, controller: controller}}
			status, err := newCommand("restart", nil, model.ADMIN, capture, &model.ChatMessage{Name: "caller"}).Execute(context.Background())
			if status != model.SUCCESSFUL || err != nil || operation.reads != 1 || !capture.dataObserved {
				t.Fatalf("status=%s err=%v reads=%d observed=%v", status, err, operation.reads, capture.dataObserved)
			}
			var data map[string]any
			if err := json.Unmarshal(capture.data, &data); err != nil {
				t.Fatal(err)
			}
			if data["completionStatus"] != tc.want || strings.Contains(string(capture.data), "secret") || (data["failureCode"] != nil) != (tc.want == "failed") {
				t.Fatalf("data=%s", capture.data)
			}
			if tc.finished && (strings.Contains(strings.Join(capture.messages, " "), "unconfirmed") || !strings.Contains(strings.Join(capture.messages, " "), tc.want)) {
				t.Fatalf("acknowledgment contradicts observed completion: %v", capture.messages)
			}
		})
	}
}

func TestLifecycleRequestAuditCancellationAfterAdmissionBeforeAcknowledgmentRetainsFacts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := &lifecycleSnapshotAuditController{recordingLifecycleController: &recordingLifecycleController{}, afterAdmission: cancel}
	engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: controller}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
	result, err := NewAgentCommandGateway(engine).Execute(ctx, caller, "restart", "")
	if err != nil || result.Status != commandgateway.OutcomeUnknown || !result.DataObserved || !strings.Contains(string(result.Data), `"requestStatus":"accepted"`) || result.Delivery != nil || engine.sends != 0 {
		t.Fatalf("result=%+v err=%v sends=%d", result, err, engine.sends)
	}
}

type lifecycleCancelAuditController struct {
	*core.HostLifecycle
	cancel context.CancelFunc
	before bool
}

func (c *lifecycleCancelAuditController) RequestRestartResult(ctx context.Context) (common.LifecycleAdmission, error) {
	if c.before {
		c.cancel()
	}
	admission, err := c.HostLifecycle.RequestRestartResult(ctx)
	if !c.before {
		c.cancel()
	}
	return admission, err
}

func TestLifecycleRequestAuditCancellationAtSourceAdmissionBoundary(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before admission", false: "after admission"}[before], func(t *testing.T) {
			owner := core.NewHostLifecycle(func(context.Context) error { return nil }, nil)
			t.Cleanup(owner.Close)
			release, err := owner.BeginDispatch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controller := &lifecycleCancelAuditController{HostLifecycle: owner, cancel: cancel, before: before}
			engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: controller}
			caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
			entry, _ := commandcatalog.AgentEntry("restart")
			result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, []byte(`{}`))
			if err != nil || !result.IsError || engine.sends != 0 || result.DeliveryCount != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if before {
				if result.EffectState != contract.EffectNotStarted || result.ActionCount != 0 || len(result.ObservedData) != 0 {
					t.Fatalf("unadmitted cancellation=%+v", result)
				}
			} else if result.EffectState != contract.EffectUnknown || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.ActionCount != 1 || !strings.Contains(string(result.Envelope()), `"requestStatus":"accepted"`) {
				t.Fatalf("admitted cancellation=%+v", result)
			}
		})
	}
}

type uncertainLifecycleCancellation struct {
	*recordingLifecycleController
	cancel context.CancelFunc
}

func (c *uncertainLifecycleCancellation) RequestRestart(context.Context) error {
	c.restartCalls++ // legacy controller has acted, but provides no source receipt
	c.cancel()
	return context.Canceled
}

func TestLifecycleRequestAuditLegacyCancellationWithoutAdmissionProofRemainsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := &uncertainLifecycleCancellation{recordingLifecycleController: &recordingLifecycleController{}, cancel: cancel}
	engine := &leasedAuditEngine{gatewayEngine: &gatewayEngine{authorized: true}, controller: controller}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.AdminCommands})
	entry, _ := commandcatalog.AgentEntry("restart")
	result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, []byte(`{}`))
	if err != nil || controller.restartCalls != 1 || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.ActionCount != 0 || result.DeliveryCount != 0 {
		t.Fatalf("legacy cancellation=%+v err=%v", result, err)
	}
}
