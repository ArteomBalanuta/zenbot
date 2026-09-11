package execution

import (
	"context"
	"sync"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
)

func parallelRead(d contract.Descriptor) bool {
	return d.IsReadOnly() && d.Idempotent() && len(d.ResourceWrites()) == 0 && len(d.ResourceReads()) > 0
}

// ExecuteAll executes the model's batch without selecting tools or retries.
// Independent declared-safe reads overlap; actions and undeclared resource
// access are execution barriers. Conditional work belongs in a later model
// response after its inputs and the preceding action's outcome are known.
func ExecuteAll(ctx context.Context, e *Executor, agent api.Context, calls []Call) []contract.Result {
	out := make([]contract.Result, len(calls))
	if err := ValidateBatchIdentity(calls); err != nil {
		for index, call := range calls {
			out[index] = notStarted(call, "INVALID_TOOL_PROTOCOL", err.Error())
		}
		return out
	}
	prepared := make([]preparedCall, len(calls))
	for index, call := range calls {
		prepared[index] = e.prepare(agent, call)
	}
	pending := make([]int, 0, len(calls))
	flush := func() {
		var workers sync.WaitGroup
		for _, index := range pending {
			workers.Add(1)
			go func() {
				defer workers.Done()
				out[index] = e.executePrepared(ctx, agent, prepared[index])
			}()
		}
		workers.Wait()
		pending = pending[:0]
	}
	failedAction := ""
	for index, p := range prepared {
		if parallelRead(p.descriptor) && p.rejection == nil {
			if e.Ledger != nil && len(e.Ledger.missing(p.descriptor.RequiredSuccessfulTools())) > 0 {
				flush()
			}
			pending = append(pending, index)
			continue
		}
		flush()
		if p.descriptor.Effect() == contract.Action && failedAction != "" {
			r := notStarted(p.call, "ACTION_NOT_EXECUTED", "a preceding action in this batch failed; inspect its observation and choose the next action in a new call")
			r.RelatedCallID = failedAction
			p.rejection = &r
		}
		out[index] = e.executePrepared(ctx, agent, p)
		if p.descriptor.Effect() == contract.Action && out[index].IsError && failedAction == "" {
			failedAction = p.call.ID
		}
	}
	flush()
	return out
}
