# DBZ help parity implementation

## Scope completed

Implemented only the `dbzhelp` registration and fixed-text delivery vertical:

- `dbzhelp`, `dbz`, and `dhelp` now register without `ServiceBundle().DBZ`.
- Mutable/read DBZ commands remain conditional on `b.DBZ != nil`.
- `dbzhelp` executes before DBZ-bundle acquisition, ignores inbound arguments and whisper status, and calls `Engine.SendChatMessage("", dbzHelpPayload, false)`.
- `dbzHelpPayload` has physical LFs, literal ASCII `\\u2009`, and a final LF.
- The stale integration expectation that `dbz` was unimplemented was removed; it is necessary because `dbz` is now a concrete registered alias.

No authorization, role/catalog metadata, DBZ state mechanics, services/repositories, identity, or transport helpers changed. Nothing was staged, committed, reset, checked out, cleaned, or stashed.

## Strict tracer evidence

### Unit 1 — registration

RED command:

```sh
go test -count=1 ./internal/command -run '^TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState$'
```

Observed RED (`exit 1`):

```text
--- FAIL: TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState
    dbz_test.go:43: dbzhelp was not registered
```

GREEN command (run immediately after the minimal canonical move):

```sh
go test -count=1 ./internal/command -run '^TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState$'
```

Observed GREEN (`exit 0`):

```text
ok  zenbot/internal/command  0.601s
```

### Unit 2 — source-exact public payload

RED command:

```sh
go test -count=1 ./internal/command -run '^TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload$'
```

Observed RED (`exit 1`):

```text
--- FAIL: TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload
    dbz_test.go:75: status=FAILED err=DBZ state unavailable
```

This is the required no-bundle guard failure. The minimal pre-bundle branch and payload constant were then added; the old `dbzhelp` switch case was removed.

GREEN command:

```sh
go test -count=1 ./internal/command -run '^TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload$'
```

Observed GREEN (`exit 0`):

```text
ok  zenbot/internal/command  0.447s
```

## Regression reconciliation

The first final package run exposed an existing integration assertion that `dbz` must not register:

```text
--- FAIL: TestRegisterUserUtilitiesDoesNotExposeUnknownOrUnimplementedCommands
    dispatch_integration_test.go:109: unexpectedly registered "dbz"
```

Its unknown/unimplemented-alias list was narrowed by removing only `dbz`, then the final gate was rerun.

## Final gate

```sh
go test -count=1 ./internal/command ./internal/listener/message
git diff --check && git diff --cached --check
```

Observed (`exit 0`):

```text
ok  zenbot/internal/command           10.267s
ok  zenbot/internal/listener/message   3.227s
```

Both diff checks produced no output. No cached diff exists.

## Files and worktree boundaries

Directly modified:

- `internal/command/dbz.go` — DBZ-help payload and pre-bundle public delivery branch.
- `internal/command/dbz_test.go` — the two required behavioral tracers.
- `internal/command/dispatch_adapter.go` — only `dbzhelp` added to unconditional canonicals and removed from the DBZ conditional list.
- `internal/command/dispatch_integration_test.go` — remove obsolete `dbz`-unknown expectation.
- `.hermes/handoffs/dbzhelp-implementation.md` — this evidence record.

`dispatch_adapter.go` was already dirty from concurrent unrelated work. Immediately before each edit, its exact DBZ fragment was inspected; there was no conflicting owner hunk in that fragment. The final whole-file unstaged stat includes concurrent changes: `63` lines in `dispatch_adapter.go`; the two DBZ-help canonical-list edits above are the only changes from this task. The remaining target-file stats were `dbz.go` `21`, `dbz_test.go` `47`, and `dispatch_integration_test.go` `2` changed lines. The broader dirty worktree was preserved.

## Blockers

None remaining.
