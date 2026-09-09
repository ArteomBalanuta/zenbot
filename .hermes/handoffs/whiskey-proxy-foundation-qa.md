# Whiskey proxy transport foundation — independent QA

## Result

The proxy seam is fail-closed and has no public Whiskey dispatch path.

An audit found two defects in the original untracked seam:

1. Selector errors were wrapped with `%w`, exposing selector text (which can include the opaque `ProxyRef` or credential detail).
2. A typed-nil dialer stored in the `Dialer` interface bypassed `dialer == nil`, allowing a connection that would panic when started.

## Fix and regression evidence

Changed only `internal/transport/proxy.go` and `internal/transport/proxy_test.go` (plus this QA handoff). `connection.go` and its direct tests were not modified.

### RED

```text
$ go test ./internal/transport -run '^(TestNewProxyConnectionSelectorFailureDoesNotFallBackOrLeakSelectionDetails|TestNewProxyConnectionRejectsTypedNilSelectedDialer)$' -count=1
--- FAIL: TestNewProxyConnectionSelectorFailureDoesNotFallBackOrLeakSelectionDetails (0.00s)
    proxy_test.go:64: selection details leaked in error: select proxy dialer: proxy-a credential rejected
--- FAIL: TestNewProxyConnectionRejectsTypedNilSelectedDialer (0.00s)
    proxy_test.go:94: conn=&{{ws://example.invalid <nil> 15000000000 5000000000 5000000000} ...} err=<nil>
FAIL
FAIL	zenbot/internal/transport	0.461s
FAIL
```

### GREEN

`NewProxyConnection` now returns the non-wrapping `select proxy dialer: selection failed` failure class and rejects both nil interface and typed-nil `Dialer` values before `NewConnection`.

```text
$ gofmt -w internal/transport/proxy.go internal/transport/proxy_test.go && go test ./internal/transport -run '^(TestNewProxyConnectionSelectorFailureDoesNotFallBackOrLeakSelectionDetails|TestNewProxyConnectionRejectsTypedNilSelectedDialer)$' -count=1
ok  	zenbot/internal/transport	0.383s
```

Focused tests verify:

- Missing ref does not invoke the selector.
- Nil selector constructs nothing.
- Selector error neither falls back to `Config.Dialer` nor exposes `ProxyRef` / selector error text / `errors.Is` cause.
- Nil and typed-nil selected dialers construct nothing; the nil-interface case also proves the supplied direct dialer has zero calls.
- Valid selection receives the exact ref and, on `Start`, invokes the selected dialer exactly once with the supplied context and URL; the pre-existing `Config.Dialer` remains unused.
- Existing direct `NewConnection(Config{Dialer:nil})` coverage remains unchanged in `connection_test.go` (`TestConnectionLocalRoundTripAndConcurrentWrites`, cancellation, close-before-start).

## Registration/reachability evidence

- Repository Go-source search for `NewProxyConnection(` returned exactly `internal/transport/proxy.go` and `internal/transport/proxy_test.go`; there is no production caller.
- Go-source search for `ProxyRef`, `ProxyDialerSelector`, and `DialerForProxy` returned only those two transport files.
- `RegisterUserUtilitiesWithDirectAgent` still has the replica-only append `"replica", "replicaoff", "replicastatus"`; no `whiskey` string occurs anywhere in `internal/command/dispatch_adapter.go`.
- No config, factory, core, main, command, registry, or direct transport source was changed by this work.

## Exact verification logs

```text
$ go test -race ./internal/transport -count=1
ok  	zenbot/internal/transport	1.421s

$ go test ./internal/config -count=1
ok  	zenbot/internal/config	0.240s

$ go test ./internal/transport -cover -count=1
ok  	zenbot/internal/transport	0.449s	coverage: 86.8% of statements

$ go vet ./...
(exit 0; no output)

$ go build ./...
(exit 0; no output)

$ git diff --check
(exit 0; no output)
```

```text
$ go test ./... -count=1
ok  	zenbot/cmd/zenbot	1.133s
ok  	zenbot/internal/agent/api	0.656s
ok  	zenbot/internal/agent/assemble	0.372s
ok  	zenbot/internal/agent/commandgateway	0.886s
ok  	zenbot/internal/agent/live	0.630s
ok  	zenbot/internal/agent/llm	0.892s
ok  	zenbot/internal/agent/llm/openai	1.553s
ok  	zenbot/internal/agent/moderation	1.791s
ok  	zenbot/internal/agent/participation	1.192s
?   	zenbot/internal/agent/persistence	[no test files]
ok  	zenbot/internal/agent/prompt	2.062s
?   	zenbot/internal/agent/room	[no test files]
?   	zenbot/internal/agent/routing	[no test files]
ok  	zenbot/internal/agent/runtime	2.094s
ok  	zenbot/internal/agent/sql	2.060s
ok  	zenbot/internal/agent/tool	1.885s
ok  	zenbot/internal/agent/tool/contract	2.168s
ok  	zenbot/internal/agent/tool/execution	2.348s
ok  	zenbot/internal/agent/turn	2.163s
ok  	zenbot/internal/command	10.960s
?   	zenbot/internal/common	[no test files]
ok  	zenbot/internal/config	1.882s
ok  	zenbot/internal/core	2.133s
ok  	zenbot/internal/factory	2.111s
ok  	zenbot/internal/listener	2.140s
ok  	zenbot/internal/listener/info	2.222s
ok  	zenbot/internal/listener/message	2.100s
ok  	zenbot/internal/listener/snapshot	2.585s
ok  	zenbot/internal/model	2.049s
ok  	zenbot/internal/relay	1.876s
?   	zenbot/internal/repository	[no test files]
ok  	zenbot/internal/repository/h2	38.629s
ok  	zenbot/internal/service	3.819s
?   	zenbot/internal/testutil/h2fixture	[no test files]
ok  	zenbot/internal/transport	1.974s
ok  	zenbot/internal/util	2.022s
```

The working tree remains broadly dirty from unrelated work. No reset, checkout, clean, stage, or commit was performed.
