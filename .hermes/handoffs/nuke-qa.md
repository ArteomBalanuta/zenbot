# Moderator nuke remote-snapshot QA

## Verdict

**PASS — accepted for the scoped nuke remote-snapshot parity vertical.**

No production defect was proven in the audited slice. QA added regression coverage for the two previously unasserted acceptance edges: canonical-only capability-gated registration and the source-required 200 ms gap between every successful action (including final ban → lock).

## Contract audit

- `internal/command/dispatch_adapter.go`: canonical `nuke` is appended only on `common.RoomSnapshotSubmitter`; no fallback registration occurs without that capability.
- `internal/command/registry.go`: Saturn source authority has exactly `["nuke"]` and `MODERATOR`; the concrete `nukeCommand` replaces the generic `saturnCommand` route in `internal/command/handlers.go`. No nuke aliases were invented.
- `internal/listener/message/handlers.go`: dispatch authorizes `cmd.GetRole()` before `Execute`; the registered legacy adapter returns `MODERATOR`, so the nuke path cannot bypass moderator authorization through normal inbound dispatch.
- `internal/command/nuke.go`: pre-cancelled direct contexts are rejected; missing or all-`@` room input replies exactly `\\n Example: <prefix>nuke hotlinks`; all `@` characters are removed from the first argument. A valid request contains a random `nuke-` workflow ID, author, source channel, target channel, inbound whisper mode, failure reply, and `NukeRoomOperation`; it emits neither a master raw action nor an immediate reply.
- `internal/listener/snapshot/nuke_operation.go`: snapshot users are processed in order; each normalized nick is emitted as `{"cmd":"ban","nick":...}`, with 200 ms after every successful ban; `{"cmd":"lockroom"}` follows only if every ban succeeds. It owns raw JSON emission and receives `SendRaw` only from coordinator-created temporary session context.
- `internal/listener/snapshot/coordinator.go`: operation execution is followed by flush/close in coordinator ownership; the nuke operation creates/closes no session and parses no raw snapshot payload itself. The master engine has no nuke raw-send path.
- `internal/command/dispatch_adapter.go`: legacy inbound `Command` execution uses `context.Background()` because the legacy interface has no context/error channel. The concrete command’s direct pre-cancelled-context behavior is covered, but inbound cancellation propagation remains unavailable by design and is accurately documented in `nuke-implementation.md`.

## Additional QA coverage

- `TestNukeRegistrationRequiresRoomSnapshotSubmitter` now proves the submitter-enabled registry differs from the baseline by only canonical `nuke`, and verifies its moderator role and sole alias.
- `TestNukeRoomOperationUses200MillisecondDelayBetweenEveryAction` timestamps two bans and lock, requiring at least 175 ms between each emission to permit scheduler tolerance while enforcing the 200 ms production setting.
- Existing focused tests cover exact usage/reply mode, pre-cancelled context, request failure metadata, no immediate output, normalized raw payload order, and no lock after failed ban.

## Verification logs

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

```sh
gofmt -w internal/command/nuke_test.go internal/listener/snapshot/nuke_operation_test.go
go test ./internal/command ./internal/listener/snapshot -count=1
# ok  zenbot/internal/command            8.753s
# ok  zenbot/internal/listener/snapshot  1.063s

go test -race ./internal/command ./internal/listener/snapshot -count=1
# ok  zenbot/internal/command            33.754s
# ok  zenbot/internal/listener/snapshot  2.239s

go test ./... -count=1
# PASS; internal/command 10.219s, internal/listener/snapshot 2.495s,
# internal/repository/h2 40.117s; all remaining tested packages passed.

go test -race ./... -count=1
# PASS; internal/command 33.682s, internal/listener/snapshot 2.629s,
# internal/repository/h2 38.338s; all remaining tested packages passed.
# macOS linker emitted one non-fatal malformed LC_DYSYMTAB warning while linking
# zenbot/internal/agent/sql.test; the command exited 0 and its package passed.

go vet ./...
# PASS (no output)

go build ./...
# PASS (no output)

git diff --check
# PASS (no output)

go test -cover ./internal/command ./internal/listener/snapshot ./internal/core ./internal/factory ./cmd/zenbot -count=1
# ok  zenbot/internal/command            coverage: 71.6% of statements
# ok  zenbot/internal/listener/snapshot  coverage: 78.8% of statements
# ok  zenbot/internal/core               coverage: 51.5% of statements
# ok  zenbot/internal/factory            coverage: 56.4% of statements
# ok  zenbot/cmd/zenbot                  coverage: 33.7% of statements
```

## Files changed by this QA pass

- `internal/command/nuke_test.go` — canonical-only registration, alias, and role regression checks.
- `internal/listener/snapshot/nuke_operation_test.go` — default inter-action delay regression check.
- `.hermes/handoffs/nuke-qa.md` — this report.

## Limitations

- Tests exercise coordinator/session seams with fakes; no live websocket room session or live moderation action was performed.
- The legacy inbound command interface cannot carry cancellation context. This is a pre-existing interface limit, not an end-to-end cancellation guarantee.
- The working tree was already substantially dirty. No unrelated source was reset, cleaned, staged, committed, or intentionally changed by QA.
