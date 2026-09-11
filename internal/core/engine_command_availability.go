package core

// CommandAvailable reports whether controller-backed commands can operate on
// this engine. Ordinary utility capability checks remain with registration.
func (e *EngineImpl) CommandAvailable(canonical string) bool {
	switch canonical {
	case "replica", "replicaoff", "replicastatus":
		return e.replicaController != nil
	case "ws", "wsa":
		return e.supportRelay != nil
	case "automove":
		return e.autoMove != nil && e.replicaController != nil
	case "restart", "shutdown":
		return e.HostLifecycle != nil
	default:
		return true
	}
}
