package runtime

import agentapi "zenbot/internal/agent/api"

// APIBridge is the explicit caller-to-runtime submission boundary. It performs
// no provider work and preserves runtime admission/cancellation semantics.
type APIBridge struct{ Runtime *Runtime }

func (b APIBridge) Submit(inv agentapi.Invocation) error {
	if b.Runtime == nil {
		return ErrClosed
	}
	v, err := FromAPIInvocation(inv)
	if err != nil {
		return err
	}
	if v.Mode() == AMBIENT {
		return b.Runtime.SubmitAmbient(v)
	}
	return b.Runtime.Submit(v)
}
