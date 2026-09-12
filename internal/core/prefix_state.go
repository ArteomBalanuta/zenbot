package core

import (
	"strings"
	"zenbot/internal/common"
)

// UpdatePrefix replaces the prefix list on this host and the point-in-time snapshot of
// concrete managed replicas. It intentionally does not persist the value or
// affect replicas created after this call.
func (e *EngineImpl) UpdatePrefix(prefixes ...string) (previous string, err error) {
	normalized, err := common.NormalizePrefixes(prefixes)
	if err != nil {
		return "", err
	}
	e.prefixMu.Lock()
	defer e.prefixMu.Unlock()

	previous = strings.Join(e.prefixesLocked(), " ")
	e.Prefix = normalized[0]
	e.Prefixes = normalized
	if e.replicaController == nil || e.replicaController.manager == nil {
		return previous, nil
	}
	for _, managed := range e.replicaController.manager.ManagedEngines() {
		if replica, ok := managed.(*EngineImpl); ok && replica != e {
			replica.setPrefixes(normalized)
		}
	}
	return previous, nil
}

func (e *EngineImpl) setPrefix(prefix string) {
	e.setPrefixes([]string{prefix})
}

func (e *EngineImpl) setPrefixes(prefixes []string) {
	e.prefixMu.Lock()
	e.Prefixes = append([]string(nil), prefixes...)
	e.Prefix = prefixes[0]
	e.prefixMu.Unlock()
}

func (e *EngineImpl) prefixesLocked() []string {
	if len(e.Prefixes) > 0 {
		return e.Prefixes
	}
	return []string{e.Prefix}
}

func (e *EngineImpl) GetPrefixes() []string {
	e.prefixMu.RLock()
	defer e.prefixMu.RUnlock()
	return append([]string(nil), e.prefixesLocked()...)
}
