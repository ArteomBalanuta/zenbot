# SQL Utility Group B Runtime-Parity QA

## Verdict

**PASS for the owned Group B runtime-parity slice**, with the mail branch integration-test limitation recorded below. The implementation is reachable through the real command factory/registration paths, preserves the narrow H2 authorization boundary, and passed focused, full, race, vet, build, Makefile, formatting, and diff checks.

This report is independent of the implementation handoff. The repository has a large pre-existing staged migration baseline; attribution below treats the implementation's unstaged diff and untracked files as task-owned and preserves all other staged/dirty/untracked work.

## Scope and exact task-owned files

The implementation diff contained these modified files:

- `internal/command/dispatch_adapter.go`
- `internal/command/handlers.go`
- `internal/command/identity_commands.go`
- `internal/command/mail_notes.go`
- `internal/command/users_nicks.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`
- `internal/repository/sql_util_group_b.go`
- `internal/service/services.go`

The implementation also added these untracked files:

- `internal/command/remove.go`
- `internal/command/runtime_parity_red_test.go`

`internal/command/mail_notes.go` **was in fact changed**. It now calls `MailService.SaturnRegisteredUsers(ctx)` and formats the typed rows; it is not an unchanged-file parity claim.

No changes were made to `internal/factory/engine_factory.go` or `internal/command/registry.go`; their existing Group B injection and remove catalog/aliases were inspected and retained.

## Behavioral verification

### Remove dispatch, aliases, selector resolution, and errors — PASS

- `RegisterAll` retains one canonical `remove` definition with aliases `del`, `delete`, and `remove`, role `MODERATOR`; no duplicate catalog entry was introduced.
- `RegisterUserUtilities` now conditionally registers canonical `remove` when the user Group B capability is present. The existing adapter expands the one definition to all three aliases.
- `newCommand` now constructs `removeCommand`; the old `saturnCommand` `remove accepted` placeholder is no longer reachable through the registered Group B path.
- `removeCommand` trims the first argument, rejects blank/missing selectors, invokes `UserService.DeleteIdentity(ctx, selector)`, and preserves Saturn success text (`User has been removed successfully`) and failure text (`Something went wrong deleting the user`).
- `UserService.DeleteIdentity` uses the narrow optional `repository.SaturnAuthorizedDeleteRepository` capability and returns `authorized Group B delete unavailable` when absent.
- H2 `DeleteIdentityAuthorized` trims input, uses parameterized case-insensitive name-or-trip matching over `trip_names JOIN names JOIN trips`, requires exactly one distinct `(name, trip)` result, resolves both stored fields, then creates the private H2 authorization context and calls the existing two-field delete.
- Blank, missing, and ambiguous selectors return an error and zero result before deletion. The existing ordinary two-string `DeleteIdentity` authorization check remains intact.
- Focused tests cover all three aliases, trimmed selector forwarding, successful mutation, invalid/no-delete behavior, unique case-insensitive H2 resolution, and ambiguous H2 resolution.

### Last-message runtime routing and output — PASS

- With Group B present, `messagesCommand` calls `UserService.SaturnLastMessages(ctx, nil, trip, count)` and cannot fall through to the legacy `IdentityRepository.LastMessages` path.
- Typed `SaturnLastMessage` rows are explicitly adapted to the existing `name#trip` rendering using the command trip, since the typed result intentionally has no trip field.
- Existing cap/default/output handling remains: command-level values above 30 are capped; non-positive values are forwarded so the Group B repository applies Saturn's default of 5; message truncation and Java escaping remain in place.
- The focused runtime test records nil name, trip, and count and verifies non-empty adapted output. Existing command/H2 tests cover truncation/escaping and repository ordering/filtering.

### `!users` Group B routing — PASS

- `usersCommand` prefers `UserService.SaturnRegisteredUsers(ctx)` whenever Group B is present, adapts rows to the existing table formatter, and retains a legacy fallback only for compatibility-only engines without Group B.
- The focused runtime test provides only Group B registered-user rows and verifies the existing user-facing output contains the returned name.

### Mail registered-user routing and output — PASS, with test limitation

- The unregistered-recipient branch in `mailCommand` now calls `MailService.SaturnRegisteredUsers(ctx)` and does not call the old direct-DB `MailService.RegisteredUsers()` formatter.
- The typed directory formatter preserves Saturn's escaped `\\n` row convention and the existing explanatory response text. A regression test verifies `Merc trip\\n` exactly.
- `MailService.Queue` remains the existing direct DB validation/queue path; only the registered-user directory branch was changed, matching the architecture scope and Saturn `executeMail` behavior.
- **Limitation:** the command test double uses the concrete `MailService`, so there is no isolated command-level test that drives `Queue` to `user not registered` and observes the complete reply. Source inspection plus the tested formatter and service delegator establish the routing, and all full suites pass, but a future test could add real-H2 mail-command coverage.

### Exported capability boundary — PASS

Only the narrow `SaturnAuthorizedDeleteRepository.DeleteIdentityAuthorized(context.Context, string)` capability is exported. H2's authorization key, context constructor, authorized-context predicate, and unauthorized error remain private. Commands do not import the H2 package.

### Fallback behavior — PASS

Legacy message and `!users` paths remain available only when Group B is absent, preserving compatibility-only engines. Configured Group B engines take the new runtime paths. The remove command is exposed only when the Group B user capability is present, avoiding a registered command with no executable delete seam.

## Actual command results

All commands were run from `/Users/ab/workspace/go-projects/zenbot`.

- `go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/factory -count=1` — **PASS**
- `go test ./internal/command -run 'TestRuntimeParity' -count=1` — **PASS** (including mail directory regression)
- `go test ./... -count=1` — **PASS**
- `go test -race ./... -count=1` — **PASS**
  - macOS linker emitted a warning about a malformed `LC_DYSYMTAB` in one test binary; exit status remained 0 and tests passed.
- `go vet ./...` — **PASS**
- `go build ./...` — **PASS**
- `make test` — **PASS**
- `make vet` — **PASS**
- `make build` — **PASS**
- `gofmt -w` on the two changed Go files — **PASS**
- `gofmt -l` on task-owned Go files — clean
- `git diff --check` — **PASS**

`make vet`/`make build` run the Makefile's `go fmt ./...`; that temporarily removed a trailing newline from unrelated staged `internal/factory/engine_factory_test.go`. It was immediately restored from the index. Final status shows no worktree modification for that unrelated file.

## Dirty-tree, protected-source, and Saturn attribution

- The initial tree contained extensive pre-existing staged migration work, including staged additions/modifications/deletions across the repository, plus untracked architecture/implementation handoff files.
- Task-owned unstaged changes were limited to the nine files listed above; the two new command files were untracked. No unrelated staged work was reset, normalized, or reverted.
- `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` are staged baseline additions, but have no task-owned worktree diff and were not modified by this slice.
- Saturn at `/Users/ab/workspace/projects/saturn` was inspected read-only. Its pre-existing dirty files remained unchanged; no Saturn source was edited.
- The implementation files are gofmt-clean and `git diff --check` is clean.

## Explicit NOT COMPLETE boundaries

- **NOT COMPLETE: Group C.** This QA does not implement, verify, or claim SQL utility Group C or any unrelated migration group.
- **NOT COMPLETE: Saturn audit row #325.** The overall 325-unit migration inventory remains open; this slice does not close the final audit row or any broad row ledger.
- **NOT COMPLETE: overall Saturn-to-Zenbot migration.** Full migration closure remains explicitly not complete, including remaining command/service/listener/agent/persistence/SQLite-elimination work outside this slice.
- **NOT COMPLETE:** protected-document updates, schema redesign, broad security hardening, generalized DI, or unrelated dirty-tree cleanup.
