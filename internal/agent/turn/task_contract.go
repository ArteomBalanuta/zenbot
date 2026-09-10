package turn

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

type ObligationKind string

const (
	ObligationAnswer ObligationKind = "ANSWER"
	ObligationTool   ObligationKind = "TOOL"
)

var primaryIntentRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Constraint struct {
	Text string `json:"text"`
}

type Obligation struct {
	ID              string          `json:"id"`
	Kind            ObligationKind  `json:"kind"`
	PrimaryIntent   string          `json:"primaryIntent,omitempty"`
	Subject         string          `json:"subject,omitempty"`
	Required        bool            `json:"required"`
	DependsOn       []string        `json:"dependsOn,omitempty"`
	Effect          contract.Effect `json:"effect,omitempty"`
	RequiresReceipt bool            `json:"requiresReceipt,omitempty"`
}

type TaskContract struct {
	RequestID   string
	RequestHash string
	Objective   string
	constraints []Constraint
	obligations []Obligation
}

func NewTaskContract(requestID, objective string, constraints []Constraint, obligations []Obligation) (TaskContract, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return TaskContract{}, errors.New("task request ID must not be blank")
	}
	if strings.TrimSpace(objective) == "" {
		return TaskContract{}, errors.New("task objective must not be blank")
	}
	ownedConstraints := append([]Constraint(nil), constraints...)
	for index := range ownedConstraints {
		ownedConstraints[index].Text = strings.TrimSpace(ownedConstraints[index].Text)
		if ownedConstraints[index].Text == "" {
			return TaskContract{}, errors.New("task constraint must not be blank")
		}
	}
	ownedObligations := cloneObligations(obligations)
	if err := validateObligations(ownedObligations); err != nil {
		return TaskContract{}, err
	}
	hash := sha256.Sum256([]byte(objective))
	return TaskContract{
		RequestID:   requestID,
		RequestHash: hex.EncodeToString(hash[:]),
		Objective:   objective,
		constraints: ownedConstraints,
		obligations: ownedObligations,
	}, nil
}

func (t TaskContract) Constraints() []Constraint { return append([]Constraint(nil), t.constraints...) }
func (t TaskContract) Obligations() []Obligation { return cloneObligations(t.obligations) }

func cloneObligations(source []Obligation) []Obligation {
	cloned := append([]Obligation(nil), source...)
	for index := range cloned {
		cloned[index].DependsOn = append([]string(nil), source[index].DependsOn...)
	}
	return cloned
}

func validateObligations(obligations []Obligation) error {
	byID := make(map[string]int, len(obligations))
	for index := range obligations {
		obligation := &obligations[index]
		obligation.ID = strings.TrimSpace(obligation.ID)
		obligation.PrimaryIntent = strings.TrimSpace(obligation.PrimaryIntent)
		obligation.Subject = NormalizeSubject(obligation.Subject)
		if obligation.ID == "" {
			return errors.New("task obligation ID must not be blank")
		}
		if _, duplicate := byID[obligation.ID]; duplicate {
			return fmt.Errorf("duplicate task obligation ID %q", obligation.ID)
		}
		byID[obligation.ID] = index
		switch obligation.Kind {
		case ObligationAnswer:
			if obligation.PrimaryIntent != "" || obligation.Effect != "" || obligation.RequiresReceipt {
				return fmt.Errorf("answer obligation %q has tool-only fields", obligation.ID)
			}
		case ObligationTool:
			if !primaryIntentRE.MatchString(obligation.PrimaryIntent) {
				return fmt.Errorf("tool obligation %q has invalid primary intent", obligation.ID)
			}
			if obligation.Effect != contract.ReadOnly && obligation.Effect != contract.Action {
				return fmt.Errorf("tool obligation %q has invalid effect", obligation.ID)
			}
			if obligation.RequiresReceipt && obligation.Effect != contract.Action {
				return fmt.Errorf("tool obligation %q requires a receipt for a non-action", obligation.ID)
			}
		default:
			return fmt.Errorf("task obligation %q has invalid kind", obligation.ID)
		}
		seenDependencies := make(map[string]struct{}, len(obligation.DependsOn))
		for dependencyIndex, dependency := range obligation.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" {
				return fmt.Errorf("task obligation %q has blank dependency", obligation.ID)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("task obligation %q repeats dependency %q", obligation.ID, dependency)
			}
			seenDependencies[dependency] = struct{}{}
			obligation.DependsOn[dependencyIndex] = dependency
		}
	}
	for _, obligation := range obligations {
		for _, dependency := range obligation.DependsOn {
			if _, found := byID[dependency]; !found {
				return fmt.Errorf("task obligation %q has unknown dependency %q", obligation.ID, dependency)
			}
		}
	}
	visiting := make(map[string]bool, len(obligations))
	visited := make(map[string]bool, len(obligations))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("task obligation dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range obligations[byID[id]].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for _, obligation := range obligations {
		if err := visit(obligation.ID); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeSubject(subject string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(subject), "@"))
}

type TaskEvidence struct {
	ObligationID  string
	Tool          string
	PrimaryIntent string
	CallID        string
	Subject       string
	Result        contract.Result
}

type TaskState struct {
	mu                   sync.RWMutex
	contract             TaskContract
	satisfied            map[string]bool
	evidence             []TaskEvidence
	unknownActionOutcome bool
}

func NewTaskState(task TaskContract) *TaskState {
	owned, _ := NewTaskContract(task.RequestID, task.Objective, task.Constraints(), task.Obligations())
	return &TaskState{contract: owned, satisfied: make(map[string]bool, len(task.obligations))}
}

func (s *TaskState) Contract() TaskContract {
	s.mu.RLock()
	defer s.mu.RUnlock()
	owned, _ := NewTaskContract(s.contract.RequestID, s.contract.Objective, s.contract.Constraints(), s.contract.Obligations())
	return owned
}

func (s *TaskState) Observe(primaryIntent string, call execution.Call, result contract.Result) error {
	if s == nil {
		return errors.New("task state is nil")
	}
	primaryIntent = strings.TrimSpace(primaryIntent)
	toolName := strings.TrimSpace(call.Name)
	if !primaryIntentRE.MatchString(primaryIntent) || toolName == "" || call.ID == "" {
		return errors.New("task observation identity is invalid")
	}
	if result.ToolName != "" && result.ToolName != toolName {
		return errors.New("task result tool identity does not match call")
	}
	if result.CallID != "" && result.CallID != call.ID {
		return errors.New("task result call identity does not match call")
	}
	subject := subjectFromArguments(call.Arguments)
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.ErrorCode == "ACTION_OUTCOME_UNKNOWN" {
		s.unknownActionOutcome = true
	}
	record := TaskEvidence{Tool: toolName, PrimaryIntent: primaryIntent, CallID: call.ID, Subject: subject, Result: result}
	for _, obligation := range s.contract.obligations {
		if obligation.Kind != ObligationTool || s.satisfied[obligation.ID] || obligation.PrimaryIntent != primaryIntent || obligation.Subject != subject || !s.dependenciesSatisfied(obligation) {
			continue
		}
		if result.IsError {
			break
		}
		if obligation.Effect == contract.Action && !result.EffectsCommitted {
			break
		}
		if obligation.RequiresReceipt && !result.VerifiedRoomDelivery() {
			break
		}
		s.satisfied[obligation.ID] = true
		record.ObligationID = obligation.ID
		break
	}
	s.evidence = append(s.evidence, record)
	return nil
}

func subjectFromArguments(arguments json.RawMessage) string {
	var values map[string]any
	if json.Unmarshal(arguments, &values) != nil {
		return ""
	}
	for _, key := range []string{"subject", "nick", "trip", "room", "identity", "recipient", "target"} {
		if value, ok := values[key].(string); ok {
			return NormalizeSubject(value)
		}
	}
	if targets, ok := values["targets"].([]any); ok && len(targets) == 1 {
		if target, ok := targets[0].(string); ok {
			return NormalizeSubject(target)
		}
	}
	return ""
}

func (s *TaskState) ObserveAnswer(answer string) bool {
	if s == nil || strings.TrimSpace(answer) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, obligation := range s.contract.obligations {
		if obligation.Kind == ObligationAnswer && !s.satisfied[obligation.ID] && s.dependenciesSatisfied(obligation) {
			s.satisfied[obligation.ID] = true
			changed = true
		}
	}
	return changed
}

func (s *TaskState) dependenciesSatisfied(obligation Obligation) bool {
	for _, dependency := range obligation.DependsOn {
		if !s.satisfied[dependency] {
			return false
		}
	}
	return true
}

func (s *TaskState) Satisfied(id string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.satisfied[id]
}

func (s *TaskState) Pending() []Obligation {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	pending := make([]Obligation, 0, len(s.contract.obligations))
	for _, obligation := range s.contract.obligations {
		if obligation.Required && !s.satisfied[obligation.ID] {
			pending = append(pending, obligation)
		}
	}
	return cloneObligations(pending)
}

func (s *TaskState) Evidence() []TaskEvidence {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]TaskEvidence(nil), s.evidence...)
}

func (s *TaskState) HasUnknownActionOutcome() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unknownActionOutcome
}
