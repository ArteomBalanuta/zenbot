package execution

import (
	"sync"

	"zenbot/internal/agent/tool/contract"
)

type attemptStatus uint8

const (
	attemptAdmitted attemptStatus = iota
	attemptRunning
	attemptFinished
)

type attempt struct {
	status attemptStatus
	result contract.Result
}

// Ledger records attempts separately from action execution. Reads are never
// cached: the model may retry or refresh them within the request budget.
type Ledger struct {
	mu          sync.Mutex
	counts      map[string]int
	failures    map[string]int
	successful  map[string]bool
	limits      map[string]int
	maxFailures int
	attempts    map[string]attempt
	actions     map[string]string
	unknownCall string
}

func NewLedger(limits map[string]int, maxFailures int) *Ledger {
	ownedLimits := make(map[string]int, len(limits))
	for name, limit := range limits {
		ownedLimits[name] = limit
	}
	return &Ledger{counts: map[string]int{}, failures: map[string]int{}, successful: map[string]bool{},
		limits: ownedLimits, maxFailures: maxFailures, attempts: map[string]attempt{}, actions: map[string]string{}}
}

func (l *Ledger) admit(c Call) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.attempts[c.ID]; exists {
		return "INVALID_TOOL_PROTOCOL"
	}
	if limit := l.limits[c.Name]; limit > 0 && l.counts[c.Name] >= limit {
		return "TOOL_CALL_LIMIT_REACHED"
	}
	if l.maxFailures > 0 && l.failures[c.Name] >= l.maxFailures {
		return "TOOL_DISABLED"
	}
	l.counts[c.Name]++
	l.attempts[c.ID] = attempt{status: attemptAdmitted}
	return ""
}

func (l *Ledger) start(c Call, d contract.Descriptor) *contract.Result {
	l.mu.Lock()
	defer l.mu.Unlock()
	if d.Effect() == contract.Action {
		if l.unknownCall != "" {
			r := notStarted(c, "ACTION_NOT_EXECUTED", "a previous action has an unknown outcome; actions are blocked for this turn, but read-only tools remain available")
			r.RelatedCallID = l.unknownCall
			return &r
		}
		if previous, exists := l.actions[Key(c)]; exists {
			r := notStarted(c, "DUPLICATE_TOOL_CALL", "this action already started or committed effects; use the earlier observation instead of repeating it")
			r.RelatedCallID = previous
			return &r
		}
		l.actions[Key(c)] = c.ID
	}
	l.attempts[c.ID] = attempt{status: attemptRunning}
	return nil
}

func (l *Ledger) finish(c Call, d contract.Descriptor, result contract.Result, invoked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[c.ID] = attempt{status: attemptFinished, result: result}
	if !invoked {
		return
	}
	if result.IsError && result.EffectState != contract.EffectNotStarted {
		l.failures[c.Name]++
	} else if !result.IsError {
		l.successful[c.Name] = true
	}
	if d.Effect() == contract.Action {
		switch result.EffectState {
		case contract.EffectNotStarted, contract.EffectNotCommitted:
			delete(l.actions, Key(c))
		case contract.EffectUnknown:
			l.unknownCall = c.ID
		}
	}
}

func (l *Ledger) missing(prerequisites []string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	missing := make([]string, 0, len(prerequisites))
	for _, prerequisite := range prerequisites {
		if !l.successful[prerequisite] {
			missing = append(missing, prerequisite)
		}
	}
	return missing
}

func (l *Ledger) Available(name string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit := l.limits[name]; limit > 0 && l.counts[name] >= limit {
		return false
	}
	return l.maxFailures <= 0 || l.failures[name] < l.maxFailures
}

// UnknownActionCallID identifies the unresolved action blocking further
// effects in this turn. Read-only observations cannot prove a generic action
// safe to repeat, so the block persists for the lifetime of the ledger.
func (l *Ledger) UnknownActionCallID() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.unknownCall
}
