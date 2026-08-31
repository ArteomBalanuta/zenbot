package commandgateway

import (
	"context"

	"zenbot/internal/agent/api"
)

type Execution struct {
	Executed bool
	Messages []string
}

type Gateway interface {
	Execute(context.Context, api.Context, string, string) (Execution, error)
}
