package common

import (
	"context"
	"sync"
	"testing"
)

func TestMutationRecorderIsolatedConcurrentAndCancellationSafe(t *testing.T) {
	parent, parentReceipt := WithMutationRecorder(context.Background())
	child, childReceipt := WithMutationRecorder(parent)
	canceled, cancel := context.WithCancel(child)
	cancel()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); RecordCommittedMutation(canceled) }()
	}
	wg.Wait()
	if childReceipt.Count() != 20 || parentReceipt.Count() != 0 {
		t.Fatalf("child=%d parent=%d", childReceipt.Count(), parentReceipt.Count())
	}
	detached := childReceipt.Count()
	RecordCommittedMutation(child)
	if detached != 20 || childReceipt.Count() != 21 {
		t.Fatalf("detached=%d latest=%d", detached, childReceipt.Count())
	}
	RecordCommittedMutation(context.Background())
	RecordCommittedMutation(nil)
}
