package live

import (
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/runtime"
)

// RuntimeService forwards live-agent submissions and lifecycle to one runtime.
type RuntimeService struct {
	Runtime *runtime.Runtime
}

func (s RuntimeService) Submit(invocation api.Invocation) error {
	return runtime.APIBridge{Runtime: s.Runtime}.Submit(invocation)
}

func (s RuntimeService) Close() {
	if s.Runtime != nil {
		s.Runtime.Close()
	}
}

var _ AgentService = RuntimeService{}
