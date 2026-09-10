# SQL Utility Group B Runtime-Parity Implementation

## Outcome

Implemented the repaired Group B runtime-parity slice only. This is **not** a claim of overall Saturn migration completion.

## TDD evidence

- **RED:** Added `internal/command/runtime_parity_red_test.go` before production wiring and ran:
  `go test ./internal/command -run 'TestRuntimeParity' -count=1`.
  It failed as expected: remove resolved to the fallback/no mutation, invalid selectors succeeded, and messages panicked through the legacy service path.
- **GREEN:** After the smallest wiring changes, focused command/service/repository/factory tests passed, including the new runtime tests and new H2 selector tests.

## Implemented semantics

- Added real `removeCommand`; `handlers.go:newCommand` constructs it.
- Added `remove` to `RegisterUserUtilities`; existing catalog aliases `del`, `delete`, and `remove` are exposed by one adapter registration.
- Added narrow `repository.SaturnAuthorizedDeleteRepository` capability and `UserService.DeleteIdentity` bridge. H2 keeps authorization context internals private.
- H2 selector delete trims input, matches name or trip case-insensitively, requires exactly one distinct `(name, trip)`, resolves both stored values, then invokes the existing authorized two-field delete. Blank, missing, and ambiguous selectors return an error and zero result without deletion.
- `messagesCommand` uses `SaturnLastMessages(ctx, nil, trip, count)` when Group B is present and adapts typed results to `name#trip`; legacy fallback remains for compatibility-only engines without Group B.
- `!users` uses Group B registered-user rows when available and retains legacy fallback for compatibility-only engines.
- Mail’s unregistered-recipient directory uses `MailService.SaturnRegisteredUsers(ctx)` and preserves the existing explanatory response format.
- Remove success output is `User has been removed successfully`; failures report `Something went wrong deleting the user` (usage reports the Saturn-style example).

## Files changed by this slice

- `internal/command/dispatch_adapter.go`
- `internal/command/handlers.go`
- `internal/command/identity_commands.go`
- `internal/command/mail_notes.go`
- `internal/command/users_nicks.go`
- `internal/command/remove.go` (new)
- `internal/command/runtime_parity_red_test.go` (new)
- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `internal/service/services.go`

No changes were made to `internal/factory/engine_factory.go` or `internal/command/registry.go`; their existing Group B injection and catalog registration were retained.

## Verification results

- Focused: `go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/factory -count=1` — PASS.
- Full: `go test ./...` — PASS.
- Race: `go test -race ./...` — PASS. macOS linker emitted an existing-looking malformed `LC_DYSYMTAB` warning for one test binary, but exit status was 0.
- Static/build: `go vet ./...` — PASS; `go build ./...` — PASS.
- Makefile: `make test` — PASS; `make vet` — PASS; `make build` — PASS.
- Hygiene: `gofmt` applied to changed Go files; `git diff --check` — PASS.
- Protected `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` have no diff. Saturn checkout was not modified by this work; it had pre-existing dirty files when inspected.

## Limitations and explicit exclusions

- No broad Saturn migration, Group C, row #325, schema redesign, transport/provider changes, or speculative abstractions.
- Existing legacy compatibility fallbacks remain where Group B is absent; they do not affect configured Group B runtime paths.
- The repository’s pre-existing staged/dirty/untracked migration work was preserved and not normalized or reverted.
- Saturn’s own pre-existing dirty checkout was left untouched.
