# Whiskey proxy transport foundation — implementation evidence

## Scope

- Added the unreachable, fail-closed proxy selection seam only in `internal/transport`.
- Did not modify direct transport behavior, composition, configuration, command registration, or Whiskey dispatch.

## Tracer 1 — missing proxy reference

### RED

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsMissingProxyRefWithoutCallingSelector$' -count=1
# zenbot/internal/transport [zenbot/internal/transport.test]
internal/transport/proxy_test.go:10:14: undefined: ProxyDialerSelectorFunc
internal/transport/proxy_test.go:10:43: undefined: ProxyRef
internal/transport/proxy_test.go:15:15: undefined: NewProxyConnection
FAIL	zenbot/internal/transport [build failed]
FAIL
```

Expected: the desired proxy transport API did not exist.

### GREEN

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsMissingProxyRefWithoutCallingSelector$' -count=1
ok  	zenbot/internal/transport	0.481s
```

## Tracer 2 — nil selector

### RED

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsNilSelector$' -count=1
--- FAIL: TestNewProxyConnectionRejectsNilSelector (0.00s)
    proxy_test.go:27: conn=<nil> err=proxy transport selection is unavailable
FAIL
FAIL	zenbot/internal/transport	0.375s
FAIL
```

### GREEN

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsNilSelector$' -count=1
ok  	zenbot/internal/transport	0.381s
```

## Tracer 3 — selector error

### RED

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionSelectorFailureDoesNotFallBackToDefaultDialer$' -count=1
--- FAIL: TestNewProxyConnectionSelectorFailureDoesNotFallBackToDefaultDialer (0.00s)
    proxy_test.go:55: conn=<nil> err=proxy transport selection is unavailable
FAIL
FAIL	zenbot/internal/transport	0.452s
FAIL
```

### GREEN

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionSelectorFailureDoesNotFallBackToDefaultDialer$' -count=1
ok  	zenbot/internal/transport	0.379s
```

## Tracer 4 — nil selected dialer

### RED

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsNilSelectedDialer$' -count=1
--- FAIL: TestNewProxyConnectionRejectsNilSelectedDialer (0.00s)
    proxy_test.go:69: conn=<nil> err=proxy transport selection is unavailable
FAIL
FAIL	zenbot/internal/transport	0.382s
FAIL
```

### GREEN

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionRejectsNilSelectedDialer$' -count=1
ok  	zenbot/internal/transport	0.378s
```

## Tracer 5 — exact selected dialer

### RED

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionUsesSelectedDialerOnly$' -count=1
--- FAIL: TestNewProxyConnectionUsesSelectedDialerOnly (0.00s)
    proxy_test.go:96: conn=<nil> err=proxy transport selection is unavailable
FAIL
FAIL	zenbot/internal/transport	0.441s
FAIL
```

### GREEN

```text
$ go test ./internal/transport -run '^TestNewProxyConnectionUsesSelectedDialerOnly$' -count=1
ok  	zenbot/internal/transport	0.379s
```

## Final verification

```text
$ gofmt -w internal/transport/proxy.go internal/transport/proxy_test.go
$ go test ./internal/transport -run '^TestNewProxyConnection' -count=1
ok  	zenbot/internal/transport	0.260s
$ go test ./internal/transport -count=1
ok  	zenbot/internal/transport	0.268s
$ go test ./internal/config -count=1
ok  	zenbot/internal/config	0.336s
$ git diff --check
(exit 0; no output)
```

Source search found `NewProxyConnection` only in `internal/transport/proxy.go` and `internal/transport/proxy_test.go`. `internal/command/dispatch_adapter.go` has no `whiskey` occurrence, so public registration remains omitted. The owned working-tree paths are only `internal/transport/proxy.go`, `internal/transport/proxy_test.go`, and this handoff; pre-existing unrelated worktree changes were left untouched.
