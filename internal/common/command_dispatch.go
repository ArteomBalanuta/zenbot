package common

import (
	"context"
	"fmt"

	"zenbot/internal/model"
	"zenbot/internal/profiling"
)

// CommandRejectedError distinguishes a known handler rejection from a legacy
// command whose void return provides no execution status.
type CommandRejectedError struct{ Status model.Status }

func (e *CommandRejectedError) Error() string {
	return fmt.Sprintf("command rejected with status %s", e.Status)
}

// InvokeCommand runs an authorized command synchronously under the host's
// existing dispatch lease. A legacy void return has an unknown (empty) status;
// it is not evidence that an action succeeded.
func InvokeCommand(ctx context.Context, engine Engine, command Command) (model.Status, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	lifecycleDone := profiling.Measure(ctx, "command.lifecycle_gate")
	release, err := BeginCommandDispatch(ctx, engine)
	lifecycleDone()
	if err != nil {
		return model.FAILED, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	executionDone := profiling.Measure(ctx, "command.execute")
	defer executionDone()
	if result, ok := command.(interface {
		ExecuteResult(context.Context) (model.Status, error)
	}); ok {
		status, err := result.ExecuteResult(ctx)
		if status == model.FAILED && err == nil {
			err = &CommandRejectedError{Status: status}
		}
		return status, err
	}
	if contextual, ok := command.(interface{ ExecuteContext(context.Context) }); ok {
		contextual.ExecuteContext(ctx)
	} else {
		command.Execute()
	}
	return "", nil
}

// BeginCommandDispatch is the shared, nonblocking host admission boundary.
// The returned lease must cover execution and command auditing.
func BeginCommandDispatch(ctx context.Context, engine Engine) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if provider, ok := engine.(interface {
		HostLifecycleController() HostLifecycleController
	}); ok {
		if controller, ok := provider.HostLifecycleController().(interface {
			BeginDispatch(context.Context) (func(), error)
		}); ok {
			return controller.BeginDispatch(ctx)
		}
	}
	return func() {}, nil
}
