package snapshot

import (
	"context"
	"testing"
)

func TestTemporarySessionsAreIsolated(t *testing.T) {
	r := NewTemporarySessionRegistry()
	id, ctx, err := r.Open(context.Background())
	if err != nil || id == "" {
		t.Fatal(err)
	}
	if r.Len() != 1 {
		t.Fatal(r.Len())
	}
	if err := r.Close(id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("session context still active")
	}
	if r.Len() != 0 {
		t.Fatal("session leaked")
	}
	if r.Close(id) == nil {
		t.Fatal("double close accepted")
	}
}
