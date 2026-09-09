package transport

import (
	"fmt"
	"reflect"
)

// ProxyRef is an opaque, non-secret caller-selected proxy identity.
// It is deliberately not a URL parser or a configuration format.
type ProxyRef string

// ProxyDialerSelector resolves one explicit proxy identity to the dialer that
// must be used for its WebSocket connection.
type ProxyDialerSelector interface {
	DialerForProxy(ProxyRef) (Dialer, error)
}

// ProxyDialerSelectorFunc adapts a function for tests and composition.
type ProxyDialerSelectorFunc func(ProxyRef) (Dialer, error)

func (f ProxyDialerSelectorFunc) DialerForProxy(proxy ProxyRef) (Dialer, error) {
	return f(proxy)
}

// NewProxyConnection constructs a connection bound to one selected proxy.
// It fails closed: absent/invalid selection or a failed/nil result never
// reaches NewConnection and never receives the direct default dialer.
func NewProxyConnection(cfg Config, proxy ProxyRef, selector ProxyDialerSelector) (*Connection, error) {
	if proxy == "" {
		return nil, fmt.Errorf("proxy reference is required")
	}
	if selector == nil {
		return nil, fmt.Errorf("proxy dialer selector is required")
	}
	dialer, err := selector.DialerForProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("select proxy dialer: selection failed")
	}
	if isNilDialer(dialer) {
		return nil, fmt.Errorf("select proxy dialer: selector returned nil dialer")
	}
	cfg.Dialer = dialer
	return NewConnection(cfg), nil
}

func isNilDialer(dialer Dialer) bool {
	if dialer == nil {
		return true
	}
	value := reflect.ValueOf(dialer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
