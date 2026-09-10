package execution

import (
	"fmt"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

// Stage is an ordered execution unit. Parallel stages contain only mutually
// compatible read-only, idempotent calls; all other stages contain one call.
type Stage struct {
	Calls    []Call
	Parallel bool
}

// BatchPlanner preserves provider order while grouping only safe adjacent
// reads. Actions, invalid descriptors, and unknown tools are hard barriers.
type BatchPlanner struct {
	Registry *tool.Registry
}

func (p BatchPlanner) Plan(agent api.Context, calls []Call) ([]Stage, error) {
	if p.Registry == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	if err := ValidateBatchIdentity(calls); err != nil {
		return nil, err
	}

	stages := make([]Stage, 0, len(calls))
	parallelCalls := make([]Call, 0, len(calls))
	parallelDescriptors := make([]contract.Descriptor, 0, len(calls))
	flushParallel := func() {
		if len(parallelCalls) == 0 {
			return
		}
		stages = append(stages, Stage{Calls: append([]Call(nil), parallelCalls...), Parallel: len(parallelCalls) > 1})
		parallelCalls = parallelCalls[:0]
		parallelDescriptors = parallelDescriptors[:0]
	}

	for _, call := range calls {
		descriptor, safe := p.safeDescriptor(agent, call)
		if !safe {
			flushParallel()
			stages = append(stages, Stage{Calls: []Call{call}})
			continue
		}
		compatible := true
		for _, existing := range parallelDescriptors {
			if Conflict(existing, descriptor) {
				compatible = false
				break
			}
		}
		if !compatible {
			flushParallel()
		}
		parallelCalls = append(parallelCalls, call)
		parallelDescriptors = append(parallelDescriptors, descriptor)
	}
	flushParallel()
	return stages, nil
}

func (p BatchPlanner) safeDescriptor(agent api.Context, call Call) (contract.Descriptor, bool) {
	registered, ok := p.Registry.Find(agent, call.Name)
	if !ok {
		return contract.Descriptor{}, false
	}
	descriptor, err := registered.Descriptor(agent)
	if err != nil || !Safe(descriptor) {
		return contract.Descriptor{}, false
	}
	return descriptor, true
}
