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
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	controller := lifecycleController(c.engine)
	if controller == nil {
		return model.FAILED, fmt.Errorf("host lifecycle controller is not configured")
	}
	if err := controller.RequestRestart(ctx); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type shutdownCommand struct {
	commandBase
}

func (c *shutdownCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	controller := lifecycleController(c.engine)
	if controller == nil {
		return model.FAILED, fmt.Errorf("host lifecycle controller is not configured")
	}
	if err := controller.RequestShutdown(ctx); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
