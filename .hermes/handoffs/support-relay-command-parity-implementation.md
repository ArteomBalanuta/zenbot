# Support-relay command parity implementation

## Delivered

Implemented the bounded USER support-replica relay vertical:

- `ws`, `wsay` route to a non-anonymous relay request.
- `wsa`, `wsayanon`, `anonsay` route to an anonymous relay request.
- Aliases are registered only when the master implements `common.SupportReplicaRelay`; a plain engine and a generic replica controller do not expose them.
- Each concrete command passes caller name, trip, parsed arguments excluding the alias, and route anonymity. It returns `FAILED` with no reply on cancellation, unavailable capability, or relay failure.
- `core.SupportReplicaRelay` only snapshots an already-managed replica exactly named `support`; it neither creates, stops, reconnects, nor mutates replicas.
- The relay uses exact configured `AdminTrips` membership only for source formatting. Admin input retains source characters; non-admin input retains only ASCII letters, digits, and spaces. Both retain the source trailing space.
- Java-compatible escaping covers quote, backslash, standard controls, other controls, and non-ASCII/surrogate pairs before route prefixes (`author: ` or `anon_from_hc: `).
- It sends one public, unaddressed outbound chat through the managed support engine. No invoking-room acknowledgement is emitted.
- Main installs the relay from the existing replica manager and existing `c.AdminTrips` before utility registration.
- `ws` and `wsa` were removed from the generic `msgchannel` fallback; `msgchannel` remains unchanged.

## Strict TDD record

Observed RED failures before the matching green changes:

```text
# capability contract tracer
go test ./internal/command -run '^TestSupportRelayAliases' -count=1
internal/command/support_relay_test.go:13:70: undefined: common.SupportRelayRequest
FAIL zenbot/internal/command [build failed]

# command forwarding tracer
go test ./internal/command -run '^TestSupportRelayCommands' -count=1
... Execute() = (FAILED, no Saturn implementation for "ws")
... Execute() = (FAILED, no Saturn implementation for "wsa")
FAIL zenbot/internal/command

# core/composition tracer
go test ./internal/core -run '^TestSupportRelay' -count=1
internal/core/support_relay_test.go:19:11: undefined: NewSupportReplicaRelay
FAIL zenbot/internal/core [build failed]

go test ./internal/core -run '^TestEngineSupportRelay' -count=1
engine.RelayToSupport undefined
engine.SetSupportReplicaRelay undefined
FAIL zenbot/internal/core [build failed]

# normal dispatch tracer
go test ./internal/command -run '^TestSupportRelayDispatch' -count=1
Unknown command
allowed requests=[] replies=[]
FAIL zenbot/internal/command
```

Each was followed immediately by its green implementation and rerun.

## Focused green gates

```text
go test ./internal/command -run 'TestSupportRelay|TestReplica' -count=1
ok zenbot/internal/command 1.643s

go test ./internal/core -run 'TestSupportRelay|TestEngineSupportRelay|TestReplicaManager' -count=1
ok zenbot/internal/core 0.256s

go test ./internal/factory ./cmd/zenbot -count=1
ok zenbot/internal/factory 0.383s
ok zenbot/cmd/zenbot 0.932s
```

## Final verification

```text
go test ./...
PASS: all packages (command 12.282s; core 1.443s; listener/message 16.622s)

git diff --check
exit 0, no output

git diff --cached --check
exit 0, no output
```

The repository remains deliberately dirty. No files were staged, reset, cleaned, restored, checked out, or committed.

## Scope confirmation

No generic `whiskey` proxy path, proxy transport/configuration, credentials, temporary sessions, remote `msgchannel`, mining, raw SQL, restart/shutdown lifecycle, agent command exposure, identity-policy source, or schema/persistence work was added.

## Files added or changed by this vertical

- `internal/common/support_relay.go`
- `internal/core/support_relay.go`
- `internal/core/support_relay_test.go`
- `internal/core/engine_impl.go`
- `internal/command/support_relay.go`
- `internal/command/support_relay_test.go`
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/registry.go`
- `cmd/zenbot/main.go`
