# DBZ Migration Implementation Handoff

## Result
Implemented the DBZ persistence/service/command slice against the existing Zenbot seams. No schema objects were added or changed; `internal/repository/h2/schema-h2.sql` and `resources/schema-h2.sql` were verified byte-identical.

## Transaction and error policy
- **Registration policy: source-compatible, non-atomic.** `RegisterCharacter` executes character insert, exact-name ID lookup, then stats insert as independent database operations. This preserves Saturn's possible orphan-character behavior and duplicate-name semantics rather than silently adapting to `WithTx`.
- Level-up remains two independent updates. Stat mutation has no SQL free-point guard. Strength decrements free points; agility/vitality/energy do not. These are intentional source quirks.
- Repository methods return context-aware errors and safely close `database/sql` rows. The compatibility command boundary preserves source-visible behavior: registration reports success after service errors are swallowed at the command boundary; read failures return `FAILED`; mutation return errors are ignored where Saturn ignored them.
- Go cannot define both existing identity `Database.Register(string,string,Role) error` and DBZ's proposed `Register(context.Context,string,func()int64)(int64,error)`. The narrow interface therefore uses the explicit collision-free name `RegisterCharacter`; this is documented target adaptation, while the H2 SQL behavior is unchanged.

## Task-owned implementation files
- `internal/repository/dbz.go`
- `internal/repository/h2/dbz.go`
- `internal/repository/h2/dbz_test.go`
- `internal/service/dbz.go`
- `internal/service/dbz_test.go`
- `internal/service/services.go` (DBZ bundle field)
- `internal/command/dbz.go`
- `internal/command/dbz_test.go`
- `internal/command/handlers.go` (canonical propagation and concrete DBZ construction)
- `internal/command/registry.go` (DBZ role changed to `model.REGULAR`)
- `internal/command/dispatch_adapter.go` (conditional DBZ runtime registration)
- `internal/factory/engine_factory.go` (DBZ service wiring when repository supports interface)

The repository already contained unrelated dirty/untracked migration work in several shared files; it was preserved. No Saturn source was modified.

## Implemented behavior
- Named H2 methods cover character registration, level-up/free-stat grant, all four stat mutations, joined stats read/render data, and free-stat read.
- Exact stats rendering and no-row message are implemented.
- Six canonical handlers cover all required aliases: `dbzregister/dreg/dr`, `dbzstats/dstats/dstat/ds`, `dbzstr/dstr/daddstr`, `dfight/df`, `dbzhelp/dbz/dhelp`, and `dspawn`.
- DBZ commands use `model.REGULAR`, source argument usage/error text, source strength-only free-stat guard, source fight/level-up behavior, and source help text/separators.
- Enemy state is process-local and protected by a mutex; spawn appends and fight removes only the first matching enemy.
- DBZ aliases are registered by `RegisterUserUtilities` only when `Bundle.DBZ != nil`; DBZ-less engines do not expose generic placeholders.

## Verification (actual command output)

Focused tests:
```
ok  zenbot/internal/command  2.262s
ok  zenbot/internal/repository/h2  9.758s
ok  zenbot/internal/service  0.487s
```

Race suite:
```
go test -race ./...  => PASS
```
All packages passed; command, H2, service, listener, core, agent, model, and repository packages were reported `ok` or `[no test files]`.

Static/build checks:
```
go vet ./...  => PASS (no output)
go build ./... => PASS (no output)
```

Schema check:
```
schema consistency: identical
```

## Remaining gaps
- The focused H2 test covers registration, initial values, level-up, strength, and agility quirks; exhaustive metadata/foreign-key inspection, duplicate-name capture, injected mid-registration failure, and every H2 read/error branch are not yet separately represented in DBZ-specific tests.
- Listener-level golden coverage for every alias, whisper payload, role rejection, and DBZ-less dispatch is not added in this slice; existing command/dispatch suites and the concrete alias tests pass.
- Existing catalog remains the reviewed 64-definition catalog; DBZ definitions are concrete when runtime-enabled and are not registered by the legacy adapter when DBZ is absent.
