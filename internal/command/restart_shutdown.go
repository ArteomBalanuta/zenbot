package command

import (
	"context"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type lifecycleControllerProvider interface {
	HostLifecycleController() common.HostLifecycleController
}

func lifecycleController(engine common.Engine) common.HostLifecycleController {
	if provider, ok := engine.(lifecycleControllerProvider); ok {
		return provider.HostLifecycleController()
	}
	return nil
}

type restartCommand struct {
	commandBase
}

func (c *restartCommand) Execute(ctx context.Context) (model.Status, error) {
	return requestHostLifecycle(ctx, &c.commandBase, "restart")
}

type shutdownCommand struct {
	commandBase
}

func (c *shutdownCommand) Execute(ctx context.Context) (model.Status, error) {
	return requestHostLifecycle(ctx, &c.commandBase, "shutdown")
}

type lifecycleRequestObservation struct {
	Operation        string `json:"operation"`
	Scope            string `json:"scope"`
	RequestStatus    string `json:"requestStatus"`
	CompletionStatus string `json:"completionStatus"`
	FailureCode      string `json:"failureCode,omitempty"`
}

func requestHostLifecycle(ctx context.Context, c *commandBase, operation string) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, &common.LifecycleRequestRejectedError{Err: err}
	}
	controller := lifecycleController(c.engine)
	if controller == nil {
		return model.FAILED, fmt.Errorf("host lifecycle controller is not configured")
	}
	requests, structured := controller.(common.HostLifecycleRequests)
	if !structured {
		// Preserve error-only controller compatibility. It provides no facts
		// sufficient to distinguish acceptance from coalescing.
		var err error
		if operation == "restart" {
			err = controller.RequestRestart(ctx)
		} else {
			err = controller.RequestShutdown(ctx)
		}
		if err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	var admission common.LifecycleAdmission
	var err error
	if operation == "restart" {
		admission, err = requests.RequestRestartResult(ctx)
	} else {
		admission, err = requests.RequestShutdownResult(ctx)
	}
	if err != nil {
		return model.FAILED, &common.LifecycleRequestRejectedError{Err: err}
	}
	data := lifecycleRequestObservation{Operation: operation, Scope: "host", RequestStatus: "accepted", CompletionStatus: "not_observed"}
	if admission.Coalesced {
		data.RequestStatus = "coalesced"
	}
	if admission.Operation != nil {
		if result, finished := admission.Operation.Result(); finished {
			data.CompletionStatus = "succeeded"
			if result.Err != nil {
				data.CompletionStatus = "failed"
				data.FailureCode = "HOST_LIFECYCLE_FAILED"
			}
		}
	}
	whisper := c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"
	observeCommandData(c, data, whisper)
	request := "accepted"
	if admission.Coalesced {
		request = "coalesced with an existing request"
	}
	completion := "completion is unconfirmed"
	if data.CompletionStatus != "not_observed" {
		completion = "source reports completion " + data.CompletionStatus
	}
	ack := fmt.Sprintf("Host %s request %s; %s.", operation, request, completion)
	if err := replyContext(ctx, c, ack); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
