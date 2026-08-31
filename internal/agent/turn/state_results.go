package turn

import (
	"errors"
	"strings"
	"zenbot/internal/agent/tool/contract"
)

func (s *State) RecordSuccessfulToolResult(r contract.Result) error {
	if r.IsError || r.ToolName == "" || strings.TrimSpace(r.Content) == "" {
		return errors.New("invalid successful tool result")
	}
	s.results = append(s.results, r)
	return nil
}
func (s *State) SuccessfulToolResults() []contract.Result {
	return append([]contract.Result(nil), s.results...)
}
