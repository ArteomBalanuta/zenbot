# Whiskey proxy transport foundation — bounded implementation specification

## Decision and boundary

[RECOMMENDED] Implement **only** an injectable proxy-to-dialer construction seam in `internal/transport`. It must construct a connection only when a caller explicitly supplies both a non-empty proxy reference and a non-nil selector that returns a non-nil dialer. It must never substitute `websocket.DefaultDialer`, retry through a direct connection, read application configuration, create an engine/replica, or register a command.

This is a transport prerequisite, not Whiskey activation. The present production composition remains unchanged, so the new path has no caller and is unreachable from inbound/public commands, including `whiskey`.

## Evidence map

| Label | Evidence | Consequence |
|---|---|---|
| [OBSERVED] | `internal/transport/connection.go` `Dialer` already abstracts `DialContext(context.Context, string, http.Header)`, and `Config` already accepts a `Dialer`. | Tests can use a fake dialer without network/proxy infrastructure. |
| [OBSERVED] | `transport.NewConnection` replaces a nil `Config.Dialer` with `websocket.DefaultDialer`; `Connection.start` invokes `cfg.Dialer.DialContext(ctx, cfg.URL, nil)` exactly once. | The existing constructor is intentionally the direct/default path and must not be changed. A proxy path must reject missing proxy selection *before* calling it. |
| [TEST-BACKED] | `internal/transport/connection_test.go` `TestConnectionLocalRoundTripAndConcurrentWrites`, `TestConnectionDialCancellation`, and `TestConnectionCloseBeforeStartPreventsDial` cover current direct behavior and cancellation/close boundaries. | Preserve these tests unchanged and add focused fake-driven tests. |
| [OBSERVED] | `internal/config/config.go` `Config` has no proxy fields; `config.example.toml` contains only agent example settings. | This slice must not add TOML fields, parsing, validation, or example syntax. |
| [OBSERVED] | `internal/factory/engine_factory.go` `EngineOptions.Transport` passes `transport.Config` to `transport.NewConnection`; `NewEngineWithOptions` defaults only the WebSocket URL. | No factory option or construction path is changed in this slice. |
| [OBSERVED] | `cmd/zenbot/main.go` calls `factory.NewEngineWithOptions` for the host and sets only `core.NewManagedReplicaController` with `ReplicaFactory.NewReplica`; neither calls a proxy constructor. | Production remains direct and no proxy connection can be formed through main. |
| [OBSERVED] | `internal/command/dispatch_adapter.go` `RegisterUserUtilitiesWithDirectAgent` adds only `replica`, `replicaoff`, and `replicastatus` for a `ReplicaController`; it does not append `whiskey`. `internal/command/registry.go` `saturnCommand.Execute` returns `whiskey proxy configuration is unavailable` only for a direct catalog invocation. | Do not alter command catalog/dispatch/handler code. Whiskey remains unavailable to public command dispatch. |
| [OBSERVED] | `internal/command/replica.go` `WhiskeyProxyOrder` is an unused string probe helper, not a transport selector, and `ProxyRetryPolicy` is unused by it. | Do not wire, alter, or treat either helper as this transport foundation. |

## Required public transport contract

[RECOMMENDED] Add `internal/transport/proxy.go`. Keep `connection.go` behavior and API byte-for-byte unchanged except that the new file can reuse its exported `Dialer`, `Config`, and `Connection` types.

```go
package transport

import "fmt"

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
        return nil, fmt.Errorf("select proxy dialer: %w", err)
    }
    if dialer == nil {
        return nil, fmt.Errorf("select proxy dialer: selector returned nil dialer")
    }
    cfg.Dialer = dialer
    return NewConnection(cfg), nil
}
```

### Contract semantics

| Condition | Required result | Direct fallback allowed? | Selector/dial behavior |
|---|---|---:|---|
| `proxy == ""` | `(nil, error)` containing `proxy reference is required` | No | Do not call selector or any dialer. |
| `selector == nil` | `(nil, error)` containing `proxy dialer selector is required` | No | Do not dial. |
| selector returns error | `(nil, error)` wrapping it with `select proxy dialer` | No | Do not call any dialer. |
| selector returns `(nil, nil)` | `(nil, error)` containing `selector returned nil dialer` | No | Do not dial. |
| selector returns a non-nil `Dialer` | non-nil `*Connection`, nil error | No implicit fallback | Save that exact dialer in a copied `Config`; it is invoked only by a later `Connection.Start`. |
| `Connection.Start(ctx)` after successful construction | Preserve current `Connection.Start` semantics | No retry/substitution | Call the selected dialer once through the existing `Connection.start`. |

[RECOMMENDED] The constructor must not log `ProxyRef` or the selector error. Callers may choose a reference that embeds a URL/credential, and this foundational package must not leak it. The prescribed errors above intentionally identify only the failure class.

[RECOMMENDED] Do not add a `Proxy` field to `transport.Config`. `Config` is currently reusable direct transport input and a nil `Dialer` deliberately means direct/default. Keeping proxy selection as a separate constructor makes accidental `Config{}`-based direct fallback impossible on the proxy path, while preserving every current caller.

## Ownership and dependency direction

```text
[future private composition code]
   owns: ordered proxy policy, credentials, URL/protocol implementation
   holds: ProxyDialerSelector
                  |
                  | explicit ProxyRef
                  v
transport.NewProxyConnection(Config, ProxyRef, ProxyDialerSelector)
   validates selection -> stores selected Dialer in a Config copy
                  |
                  v
transport.Connection.Start(ctx)
   uses exactly that Dialer.DialContext(ctx, URL, nil)

Existing direct callers
   -> transport.NewConnection(Config{Dialer:nil})
   -> websocket.DefaultDialer                    (UNCHANGED)

cmd/zenbot main -> factory -> NewEngineWithOptions -> NewConnection
                                                   (UNCHANGED; no proxy seam call)
```

[RECOMMENDED] `transport` owns the narrow selection boundary because it owns `Dialer` and `Connection`. The eventual application/composition layer owns real endpoint parsing, proxy protocol support, credential lookup/redaction, ordering, health probes, replica lifecycle, and any proxy list. `factory.EngineOptions` remains an existing pass-through for `transport.Config`; it owns neither proxy policy nor selector state in this slice.

[RECOMMENDED] A selector must create/configure a dialer per proxy decision or return an independently safe immutable/shared implementation. It must not mutate `websocket.DefaultDialer` or reuse a mutable dialer concurrently without its own synchronization. `NewProxyConnection` copies `Config` by value before assigning the selected interface, so it does not mutate the caller's `Config`.

## Explicit deferred user decision: real proxy endpoint format

[LIMITATION] A real proxy endpoint format cannot be chosen safely from current source. Zenbot has no proxy configuration, and `go.mod` has `github.com/gorilla/websocket` but no SOCKS dependency. Gorilla's `Dialer.Proxy` is an HTTP proxy callback; it does not by itself establish a project-approved URL/credential contract for SOCKS or authenticated proxy variants.

**Decision required before any real selector implementation or configuration change:**

1. Which proxy protocols are supported: HTTP CONNECT only, HTTPS proxy, SOCKS5/SOCKS5h, or a defined subset?
2. What canonical endpoint syntax is accepted (for example, URL schemes/host/port requirements), and are credentials embedded in a URL prohibited in favor of a secret environment reference?
3. How are ordered endpoints supplied (TOML list, environment variable, secret manager), validated at startup, redacted in logs/errors, and rotated?
4. Is a proxy reference stable opaque ID, redacted URL label, or another non-secret key?

Until that decision, implement only the interface above and use fake selectors. Do not add `Config.Proxies`, TOML examples, `net/http` proxy parsing, `golang.org/x/net/proxy`, or a third-party dependency. A future real selector may satisfy `ProxyDialerSelector` without changing `Connection` or its direct callers.

## First RED tests (transport only)

[RECOMMENDED] Add `internal/transport/proxy_test.go` before `proxy.go`. The first test must prove the fail-closed boundary without opening a socket:

```go
func TestNewProxyConnectionRejectsMissingSelectionWithoutDial(t *testing.T) {
    selectorCalls := 0
    selector := ProxyDialerSelectorFunc(func(ProxyRef) (Dialer, error) {
        selectorCalls++
        return fakeDialer{}, nil
    })

    conn, err := NewProxyConnection(Config{URL: "ws://example.invalid"}, "", selector)
    if conn != nil || err == nil || !strings.Contains(err.Error(), "proxy reference is required") {
        t.Fatalf("conn=%v err=%v", conn, err)
    }
    if selectorCalls != 0 { t.Fatalf("selector calls=%d", selectorCalls) }
}
```

Use a deterministic `fakeDialer` implementing `DialContext`; it records calls and returns a sentinel error. Do **not** use a live proxy or remote endpoint.

Required RED-to-GREEN test set:

1. `TestNewProxyConnectionRejectsMissingProxyRefWithoutCallingSelector` — exact behavior above.
2. `TestNewProxyConnectionRejectsNilSelector` — non-nil config and proxy ref; no construction/dial.
3. `TestNewProxyConnectionSelectorFailureDoesNotFallBackToDefaultDialer` — selector sentinel error is wrapped; returned connection is nil; fake direct dialer/call counter remains zero.
4. `TestNewProxyConnectionRejectsNilSelectedDialer` — `(nil, nil)` produces the defined error and no connection.
5. `TestNewProxyConnectionUsesSelectedDialerOnly` — selector sees the exact `ProxyRef`; returned connection `Start` invokes fake selected dialer once with the supplied context and URL; its sentinel error is returned as `websocket dial: ...`. Assert no second selector or fallback call.
6. `TestNewConnectionDirectDefaultRemainsAvailable` — preserve existing direct test coverage; no new proxy test may assert or require changed `NewConnection` behavior.

[RECOMMENDED] Keep fakes in `proxy_test.go` package `transport`, with counters guarded by the test goroutine. Tests must inspect call counts rather than attempting to infer route choice from a network failure. The factory need not be tested in this slice because it is deliberately untouched.

## Exact file map and change budget

| Path | Action | Scope |
|---|---|---|
| `internal/transport/proxy.go` | Add | The exact `ProxyRef`, selector interface/function adapter, and `NewProxyConnection` contract above. |
| `internal/transport/proxy_test.go` | Add | Fake-driven RED/GREEN tests listed above. No live proxy/server required. |
| `internal/transport/connection.go` | No change | Existing direct behavior is protected. If a future implementation insists on a change, it is out of this bounded spec and requires a revised review. |
| `internal/transport/connection_test.go` | No change | Existing direct transport regression coverage stays intact. |
| `internal/config/config.go`, `config.toml`, `config.example.toml` | No change | Real endpoint/config policy is undecided. |
| `internal/factory/engine_factory.go`, `internal/factory/replica_factory.go`, `cmd/zenbot/main.go` | No change | No engine injection, replica construction, or production composition. |
| `internal/command/dispatch_adapter.go`, `internal/command/registry.go`, `internal/command/replica.go`, help/command tests | No change | No public command activation, catalog fallback change, or Whiskey behavior work. |

## Explicit no-scope and non-negotiable safety checks

[RECOMMENDED] Do not do any of the following in this change:

- Register, expose, conditionally expose, or implement public `whiskey` dispatch; do not alter help text or command catalog behavior.
- Create a replica, probe a proxy, add recovery/failover/backup policy, change stop behavior, or modify `ReplicaManager`/`ManagedReplicaController`.
- Modify real production composition, `EngineOptions`, or default transport construction.
- Add proxy configuration syntax, proxy URL parsing, credentials, logging, dependencies, or network integration tests.
- Change generic `replica`, `replicaoff`, `replicastatus`, or policy-blocked `mine`, `restart`, `shutdown`, or `sql`.
- Implement a catch-all `if selected dialer == nil { selected dialer = websocket.DefaultDialer }`, retry after selector failure, or call `NewConnection` before selection validation.

Acceptance safety checks:

1. A source search confirms `NewProxyConnection` has no production caller outside `internal/transport/*_test.go` after this slice.
2. A source search confirms the canonical list in `RegisterUserUtilitiesWithDirectAgent` still omits `whiskey`.
3. `git diff --check` is clean and the diff contains only the two listed transport files (plus this handoff, if untracked documentation is included in review).
4. Existing direct transport tests pass unchanged.
5. All new tests use fake dialers and prove no selector/dialer call on rejected selection and exactly one selected-dialer call after valid selection.

## Verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after the future implementation:

```bash
go test ./internal/transport ./internal/config
git diff --check
git diff -- internal/transport/proxy.go internal/transport/proxy_test.go internal/transport/connection.go internal/transport/connection_test.go internal/config/config.go internal/factory/engine_factory.go internal/factory/replica_factory.go cmd/zenbot/main.go internal/command/dispatch_adapter.go internal/command/registry.go internal/command/replica.go
git diff --name-only
```

[RECOMMENDED] Treat any touched non-transport application file, any production caller, a configuration change, a command registration change, or an untested direct fallback as a failure of this bounded prerequisite. A later Whiskey proposal must separately satisfy the full capability gate in `.hermes/handoffs/whiskey-current-architecture.md` before it can activate a command.
