# Explicit moderator resurrect remote-snapshot QA

## Verdict: PASS

Independent source/spec audit found the bounded explicit `resurrect <nick> <from> <to>` path aligned with the approved architecture and Saturn reference. No production defect was proven, so no code or test changes were made by QA.

## Audited behavior

- Zero arguments are excluded; every non-three-argument invocation returns `FAILED, nil` after the exact reply ` <prefix>move <nick> <from> <to>`.
- The reviewed aliases (`move`, `recover`, `heal`, `resurrect`) are moderator-only, and registration adds the canonical only when both `common.LiveRoomMover` and `common.RoomSnapshotSubmitter` are implemented. Neither capability alone registers an alias.
- Only the nick is normalized. `from` and `to` are preserved as parsed.
- Core selects an exact managed engine before the host, never unwraps opaque `Replica` values, and forwards a live action through `ModerationOperations.KickNickTo`.
- A handled live failure returns failure without a remote snapshot submission; only `(handled=false, err=nil)` submits the fallback request.
- Fallback sets both `SourceChannel` and `TargetChannel` to `from`, so the temporary coordinated session joins the actual source room. It leaves raw transport action ownership to `KickOrResurrectOperation`.
- Snapshot operation uses `SameNick`, skips nil/noncanonical snapshot users, sends the exact `kick/nick/to` raw payload once only for a present target, and returns the exact absent reply ` <nick> isn't in the room` without sending raw data.
- Concrete command dispatch replaces generic `saturnCommand` fallback for `resurrect`; coordinator lifecycle remains unmodified and owned by the existing coordinator.

## Verification logs

```text
go test ./internal/command ./internal/listener/snapshot ./internal/core -count=1
ok      zenbot/internal/command             9.121s
ok      zenbot/internal/listener/snapshot   1.343s
ok      zenbot/internal/core                0.639s

go test ./cmd/zenbot -run '^TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster$' -count=1
ok      zenbot/cmd/zenbot 0.277s

go test -race ./internal/command ./internal/listener/snapshot ./internal/core -count=1
ok      zenbot/internal/command             37.707s
ok      zenbot/internal/listener/snapshot   2.286s
ok      zenbot/internal/core                1.567s

go test ./internal/command ./internal/listener/snapshot ./internal/core -cover -count=1
ok      zenbot/internal/command             coverage: 71.5% of statements
ok      zenbot/internal/listener/snapshot   coverage: 79.4% of statements
ok      zenbot/internal/core                coverage: 52.1% of statements

go test ./... -count=1
PASS (all packages; internal/repository/h2: 39.052s)

go vet ./...
PASS

go build ./...
PASS

git diff --check
PASS
```

## QA file changes

- Created this handoff only: `.hermes/handoffs/resurrect-qa.md`.
- No production or test source was modified: audit found no actual missed defect requiring a TDD repair.

## Limitations retained by design

- Legacy inbound dispatch invokes commands with `context.Background()`.
- The room-snapshot coordinator has no command-context/workflow cancellation bridge. Pre-submit direct cancellation is supported; post-submit caller cancellation remains intentionally outside this slice.
- The repository was already broadly dirty. QA did not reset, clean, checkout, stage, or commit any files.
