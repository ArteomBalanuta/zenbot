package core

// UpdatePrefix applies prefix to this host and the point-in-time snapshot of
// concrete managed replicas. It intentionally does not persist the value or
// affect replicas created after this call.
func (e *EngineImpl) UpdatePrefix(prefix string) (previous string, err error) {
	e.prefixMu.Lock()
	defer e.prefixMu.Unlock()

	previous = e.Prefix
	e.Prefix = prefix
	if e.replicaController == nil || e.replicaController.manager == nil {
		return previous, nil
	}
	for _, managed := range e.replicaController.manager.ManagedEngines() {
		if replica, ok := managed.(*EngineImpl); ok && replica != e {
			replica.setPrefix(prefix)
		}
	}
	return previous, nil
}

func (e *EngineImpl) setPrefix(prefix string) {
	e.prefixMu.Lock()
	e.Prefix = prefix
	e.prefixMu.Unlock()
}
