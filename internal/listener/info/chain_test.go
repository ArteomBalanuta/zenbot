package info

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDefaultChainOrder(t *testing.T) {
	want := []string{"CaptureBanishedUser", "IgnoreSelfWhisperInfo", "RenameAfkUsers", "ConvertWhisperToChatMessage", "AuditWhisperCommand", "DispatchWhisperCommand"}
	got := []string{}
	for _, h := range DefaultChain().Handlers() {
		got = append(got, reflect.TypeOf(h).Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
func TestChainShortCircuitAndError(t *testing.T) {
	calls := 0
	c := NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { calls++; return false, nil }), HandlerFunc(func(context.Context, *Context) (bool, error) { calls++; return true, nil }))
	if err := c.Process(context.Background(), nil, nil); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	want := errors.New("boom")
	c = NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { return true, want }))
	if !errors.Is(c.Process(context.Background(), nil, nil), want) {
		t.Fatal("error not preserved")
	}
}
