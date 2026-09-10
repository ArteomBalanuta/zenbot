package turn

// BudgetDecision describes whether a tool batch may be submitted.
type BudgetDecision struct{ ExecuteTools, FinalizeWithoutTools bool }

func ReserveToolBatch(s *State, requested int) BudgetDecision {
	if s == nil || !s.ToolsEnabled() || !s.ReserveToolCalls(requested) {
		if s != nil {
			s.DisableTools()
		}
		return BudgetDecision{false, true}
	}
	return BudgetDecision{true, false}
}
