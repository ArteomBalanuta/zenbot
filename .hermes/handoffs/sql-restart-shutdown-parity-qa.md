# QA — SQL / restart / shutdown parity

## Verdict

**FAIL for full source-exact lifecycle parity; PASS for the SQL vertical and the tested command/catalog boundaries.**

The implementation correctly registers the exact concrete aliases under their capabilities, preserves ADMIN/user-trip authorization routing, keeps commands out of the agent gateway, uses raw `QueryContext` without agent-SQL policy, reproduces the tested source parser/rendering/reply behavior, and passes all automated gates.

However, production `main` creates its supervisor with `build`, `start`, and `rebind` callbacks all nil (`cmd/zenbot/main.go:258-264`). Before this QA, a live `restart` would stop the current master and return nil without constructing a fresh master. That violates the binding requirement for a fresh join-capable master graph and leaves the running bot stopped. The QA hardening prevents this destructive partial restart, but does not turn it into parity: restart now produces the existing source-style successful/no-reply command result after the handler logs the controller failure. Full lifecycle parity must not be declared until main owns a real fresh-master builder/start/rebind graph.

## QA fixes

1. `cmd/zenbot/host_supervisor.go`
   - `Restart` now fails before retiring the current master if no fresh-master builder is configured.
2. `cmd/zenbot/host_supervisor_test.go`
   - Added a RED→GREEN regression: `TestHostSupervisorRestartWithoutFreshBuilderFailsBeforeRetiringMaster`.
3. `internal/command/sql.go`
   - After `RawSQLQuery.Query` returns, cancellation now wins over the raw driver-error reply path, returning `FAILED, ctx.Err()` with no reply.
4. `internal/command/sql_test.go`
   - Added a RED→GREEN regression: `TestSQLCommandCancellationDuringQueryDoesNotReply`.

Both defects were first observed RED:

```text
TestHostSupervisorRestartWithoutFreshBuilderFailsBeforeRetiringMaster
restart without a fresh-master builder succeeded

TestSQLCommandCancellationDuringQueryDoesNotReply
status=SUCCESSFUL err=<nil>, want FAILED context.Canceled
```

Both focused tests were green after the minimal fixes.

## Contract audit

### Passed

- Catalog/alias/role: `sql`; `restart`/`reload`/`re`; `exit`/`quit`/`shutdown`, ADMIN metadata, concrete handlers, and final fallback allowlist coverage pass.
- Authorization: SQL stays on ordinary ADMIN authorization; restart/shutdown use the optional source-shaped lifecycle authorizer for configured `UserTrips` (including `x`) or ADMIN authorization. Unauthorized listener dispatch precedes query/controller effects.
- Lifecycle command behavior: parse-free/no-reply, cancellation before controller calls, controller errors logged and source-style successful/no-reply, dispatch-release barrier and race coverage.
- SQL: raw text literal `sql ` split quirks; literal `\\n` conversion; malformed/case-mismatch no query/reply; `QueryContext`; all rows/columns drained; `null`; raw driver-error reply; public/whisper routing; Saturn ASCII/UTF-16/Java-escaping goldens; best-effort failed-send regression.
- Capability/policy: SQL is gated on a non-nil raw-query capability; lifecycle aliases are gated on a controller; the agent gateway exposes none of these aliases. No human SQL path imports/consults `internal/agent/sql`; no query restriction, proxy, credential, schema, or real process action was introduced.
- Lifecycle shutdown remains host-only/non-exiting in the injected supervisor path. No tests issue an OS signal, `os.Exit`, process kill, database close, or real host lifecycle action.

### Remaining blocker

`cmd/zenbot/main.go` must replace the nil supervisor callbacks with a composition-root factory that, on restart, retires the old master, builds and starts a fresh master, and rebinds all master-capturing dependencies (room-agent/direct submission, snapshot reply sink, directory/replica wiring) without stopping replicas, agent runtime, temporary sessions, H2, or the DB. The current hardening is deliberately fail-closed rather than a substitute for that work.

## Verification witnessed

```text
go test ./internal/service ./internal/core ./internal/command ./internal/listener ./internal/listener/message ./cmd/zenbot -run 'Test(HostLifecycle|HostSupervisor|MainProductionHostLifecycle|Restart|Shutdown|UserChatListenerLifecycle|SQL|UserChatListenerSQL|FactoryWiresRawH2SQL|AdminModeratorCatalog)' -count=1
PASS

go test -race -count=1 ./internal/core ./internal/command ./internal/listener ./cmd/zenbot
PASS

go test -count=1 ./...
PASS

go vet ./...
PASS

go build ./...
PASS

git diff --check
PASS
```

The full suite includes `internal/repository/h2` and completed successfully. The tree remains broadly dirty/untracked; `git diff --check` cannot inspect untracked files, so all QA-touched Go files were also checked with `gofmt -d` (no output). No staging, reset, restore, clean, or commit was performed.
