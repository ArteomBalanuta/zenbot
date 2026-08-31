# Identity QA / hardening handoff

## Verdict

QA PASS for the requested identity slice gates. This is not a claim that the overall Saturn migration is complete. The two supplied handoffs were independently inspected before testing, and the target was tested against their stated requirements.

Working directory: `/Users/ab/workspace/go-projects/zenbot`

## Exact files changed by QA

Production:

- `internal/service/security_service.go`
  - Changed `AuthorizeTrip` and `AuthorizeUser` to return persistence errors.
- `internal/command/identity_commands.go`
  - Propagates authorization persistence failure as `FAILED` and does not emit success output.
- `internal/repository/h2/identity.go`
  - Restricts `LastMessages` to `visibility='PUBLIC'`, retaining `(created_on DESC, id DESC)` ordering and default nonpositive count behavior.
- `internal/repository/h2/authorization.go`
  - Assigns `tx.Commit()` to the named error before returning so deferred rollback can react to commit failure.

Tests:

- `internal/command/identity_commands_test.go`
  - Added authorization/grant failure propagation, count-clamp, and all-role parsing coverage; retained alias, output/escaping, registration mutation-error coverage.
- `internal/repository/h2/identity_test.go`
  - Added real-H2 whisper exclusion/tie-order coverage and partial registration rollback coverage.
- `internal/repository/h2/authorization_identity_test.go`
  - Added real-H2 grant insert/update coverage for all six roles and configured wildcard/threshold authorization coverage.
- `internal/service/security_service_test.go`
  - Added authorization repository error propagation and no-in-memory-mutation coverage.
- `internal/listener/message/dispatch_authorization_test.go`
  - Added dispatch-level unauthorized-principal rejection coverage proving command execution is not reached.

No unrelated files were intentionally changed, reverted, or deleted by QA. Existing dirty and untracked migration work was preserved.

## Coverage added/verified

- `register/reg`: aliases and MODERATOR role; concrete dispatch; whitespace trimming; mutation error output; real-H2 transaction rollback after duplicate-trip failure; existing name/trip/link behavior.
- `authorize/auth`: alias and role; exact success path; persistence failure propagation; no success reply or `AdminTrips` mutation on failure.
- `grant/access`: ADMIN role and aliases; lowercase role normalization; grant failure propagation; all six accepted roles; real-H2 insert and update behavior.
- `messages/lastmessages`: alias and role; exact escaped output; >200-byte truncation path retained; >30 warning and clamp to 30; deterministic descending timestamp/id order; LEFT/JOINED exclusion; real-H2 WHISPER exclusion from the public history read.
- Authorization: real-H2 configured wildcard, persisted role threshold, unknown-trip fail-closed behavior; nil principal/role tests; dispatch rejects unauthorized principals before execution.

## Bugs fixed

1. `SecurityService.AuthorizeTrip` logged and swallowed `GrantTrip` errors, causing `auth` to report success. It now returns the original error through `authorizeCommand`.
2. `LastMessages` had no visibility predicate and could expose WHISPER rows. It now reads PUBLIC rows only, which is the safe behavior for the existing API because the method has no caller visibility/scope parameter.
3. `GrantTrip` returned `tx.Commit()` directly without assigning the named `err`, so the deferred rollback condition could not observe commit failure. Commit errors are now assigned and returned.

## Commands and actual results

### Formatting

- `gofmt -w internal/command/identity_commands.go internal/command/identity_commands_test.go internal/repository/h2/identity.go internal/repository/h2/identity_test.go internal/repository/h2/authorization.go internal/repository/h2/authorization_identity_test.go internal/service/security_service.go internal/service/security_service_test.go internal/listener/message/dispatch_authorization_test.go`
  - Exit `0`; no output.

### Required focused test command

- `go test -count=1 ./internal/command ./internal/repository/h2 ./internal/service`
  - Exit `0`
  - Actual output:
    - `ok  zenbot/internal/command  2.204s`
    - `ok  zenbot/internal/repository/h2  9.031s`
    - `ok  zenbot/internal/service  0.531s`

### Required race test

- `go test -race ./...`
  - Exit `0`
  - Actual result: all packages passed; packages without tests reported `[no test files]`. Passing packages included `internal/command`, `internal/core`, `internal/listener`, `internal/listener/info`, `internal/listener/message`, `internal/repository/h2`, and `internal/service`.

### Required vet/build

- `go vet ./...`
  - Exit `0`; no output.
- `go build ./...`
  - Exit `0`; no output.

### Worktree preservation inspection

- `git diff --stat`
  - Exit `0`.
  - Actual tracked-change summary ended with: `27 files changed, 501 insertions(+), 470 deletions(-)`.
  - The summary includes pre-existing unrelated tracked changes; newly added/untracked migration files are not represented by `git diff --stat`.
- `git status --short`
  - Exit `0`.
  - Actual status retained the pre-existing dirty paths including `Dockerfile`, `cmd/zenbot/main.go`, `go.mod`, listener/core/config files, and deleted legacy files, plus untracked `.hermes/`, `.idea/`, `MIGRATION_PLAN.md`, `internal/agent/`, migration command/listener/repository/service files, and `resources/`. QA added only the exact files listed above within that existing dirty tree.

## Remaining gaps

- `LastMessages(name, trip, count)` has no explicit visibility/scope argument. QA enforced PUBLIC-only reads to prevent WHISPER leakage; a future API design must add an explicit, authorized WHISPER scope if private history is required.
- `access` comma-target behavior still ignores individual grant errors and grants USER rather than the parsed role, matching the documented parity quirk; it was not silently changed.
- Registration rollback coverage exercises a real-H2 partial failure, but not every possible injected insert/link failure because the production method has no injectable DB seam.
- Full migration-plan command/catalog, agent, listener, and all 325-unit parity obligations remain outside this identity QA slice.
