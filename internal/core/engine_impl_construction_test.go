package core

import "testing"

func TestNewEngineImplRetainsCallerOwnedPointerWithoutCopyingLocks(t *testing.T) {
	input := &EngineImpl{Prefix: "!"}
	input.prefixMu.Lock()
	defer input.prefixMu.Unlock()

	got := NewEngineImpl(input, nil)
	if got != input {
		t.Fatal("constructor must retain the caller-owned engine pointer")
	}
	if got.Prefix != "!" {
		t.Fatalf("prefix = %q", got.Prefix)
	}
}
