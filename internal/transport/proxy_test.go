package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

type fakeDialer struct {
	calls  int
	ctx    context.Context
	url    string
	header http.Header
	err    error
}

func (d *fakeDialer) DialContext(ctx context.Context, url string, header http.Header) (*websocket.Conn, *http.Response, error) {
	d.calls++
	d.ctx = ctx
	d.url = url
	d.header = header
	return nil, nil, d.err
}

func TestNewProxyConnectionRejectsMissingProxyRefWithoutCallingSelector(t *testing.T) {
	selectorCalls := 0
	selector := ProxyDialerSelectorFunc(func(ProxyRef) (Dialer, error) {
		selectorCalls++
		return nil, nil
	})

	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid"}, "", selector)
	if conn != nil || err == nil || !strings.Contains(err.Error(), "proxy reference is required") {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
	if selectorCalls != 0 {
		t.Fatalf("selector calls=%d", selectorCalls)
	}
}

func TestNewProxyConnectionRejectsNilSelector(t *testing.T) {
	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid"}, "proxy-a", nil)
	if conn != nil || err == nil || !strings.Contains(err.Error(), "proxy dialer selector is required") {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
}

func TestNewProxyConnectionSelectorFailureDoesNotFallBackOrLeakSelectionDetails(t *testing.T) {
	selectorErr := errors.New("proxy-a credential rejected")
	directDialer := &fakeDialer{err: errors.New("direct dialed")}
	selector := ProxyDialerSelectorFunc(func(ProxyRef) (Dialer, error) {
		return nil, selectorErr
	})

	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid", Dialer: directDialer}, "proxy-a", selector)
	if conn != nil || err == nil || !strings.Contains(err.Error(), "select proxy dialer") {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
	if strings.Contains(err.Error(), "proxy-a") || strings.Contains(err.Error(), selectorErr.Error()) || errors.Is(err, selectorErr) {
		t.Fatalf("selection details leaked in error: %v", err)
	}
	if directDialer.calls != 0 {
		t.Fatalf("direct dialer calls=%d", directDialer.calls)
	}
}

func TestNewProxyConnectionRejectsNilSelectedDialer(t *testing.T) {
	directDialer := &fakeDialer{err: errors.New("direct dialed")}
	selector := ProxyDialerSelectorFunc(func(ProxyRef) (Dialer, error) {
		return nil, nil
	})

	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid", Dialer: directDialer}, "proxy-a", selector)
	if conn != nil || err == nil || !strings.Contains(err.Error(), "selector returned nil dialer") {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
	if directDialer.calls != 0 {
		t.Fatalf("direct dialer calls=%d", directDialer.calls)
	}
}

func TestNewProxyConnectionRejectsTypedNilSelectedDialer(t *testing.T) {
	var typedNil *fakeDialer
	selector := ProxyDialerSelectorFunc(func(ProxyRef) (Dialer, error) {
		return typedNil, nil
	})

	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid"}, "proxy-a", selector)
	if conn != nil || err == nil || !strings.Contains(err.Error(), "selector returned nil dialer") {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
}

func TestNewProxyConnectionUsesSelectedDialerOnly(t *testing.T) {
	selectedErr := errors.New("selected dialer failed")
	selectedDialer := &fakeDialer{err: selectedErr}
	directDialer := &fakeDialer{err: errors.New("direct dialed")}
	proxy := ProxyRef("proxy-a")
	selectorCalls := 0
	selector := ProxyDialerSelectorFunc(func(got ProxyRef) (Dialer, error) {
		selectorCalls++
		if got != proxy {
			t.Fatalf("proxy=%q", got)
		}
		return selectedDialer, nil
	})
	ctx := context.WithValue(context.Background(), struct{}{}, "context value")

	conn, err := NewProxyConnection(Config{URL: "ws://example.invalid", Dialer: directDialer}, proxy, selector)
	if err != nil || conn == nil {
		t.Fatalf("conn=%v err=%v", conn, err)
	}
	if err := conn.Start(ctx); !errors.Is(err, selectedErr) || !strings.Contains(err.Error(), "websocket dial: selected dialer failed") {
		t.Fatalf("start error=%v", err)
	}
	if selectorCalls != 1 {
		t.Fatalf("selector calls=%d", selectorCalls)
	}
	if selectedDialer.calls != 1 || selectedDialer.ctx != ctx || selectedDialer.url != "ws://example.invalid" || selectedDialer.header != nil {
		t.Fatalf("selected calls=%d ctx=%v url=%q header=%v", selectedDialer.calls, selectedDialer.ctx, selectedDialer.url, selectedDialer.header)
	}
	if directDialer.calls != 0 {
		t.Fatalf("direct dialer calls=%d", directDialer.calls)
	}
}
