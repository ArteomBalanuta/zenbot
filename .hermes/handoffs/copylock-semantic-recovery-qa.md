# Copylock + semantic moderation recovery QA

## Scope and worktree discipline

Audited only the delegated checkpoint paths in the pre-existing dirty worktree. No reset, clean, checkout, staging, commit, Saturn change, or activation was performed. No defect was found that warranted a task-owned repair; therefore no test-first repair cycle was opened.

## Source/diff inspection

### Part A: `EngineImpl` construction and prefix behavior

- `core.NewEngineImpl` accepts `*EngineImpl`, installs `hostRelay` on that same allocation, and returns it. The only construction call site (`factory.NewEngineWithOptions`) passes `&core.EngineImpl{...}`.
- `EngineImpl` contains synchronization state (`prefixMu`, existing mutexes, atomics). The pointer-owned constructor therefore does not copy synchronization state. `TestNewEngineImplRetainsCallerOwnedPointerWithoutCopyingLocks` locks the caller-owned mutex before construction and verifies pointer identity.
- Prefix synchronization was inspected: `UpdatePrefix` and replica `setPrefix` take `prefixMu` for writes; `GetPrefix` takes `RLock`. Prefix-state tests and race tests pass.
- No other `NewEngineImpl` call site exists. The factory compatibility fallback constructs a fresh pointer literal directly and does not copy an existing engine.

### Fail-closed semantic moderation construction

- `SourceModerationAliases()` is exactly the six-action source inventory: `captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban`.
- `ModerationGateway` parses only that closed enum and invokes only typed reviewed core operations: `EnableCaptcha`, `MutePrincipal`, `UnmuteTarget`, `KickPrincipal`, `ShadowBan`, and `UnshadowBanTarget`.
- `ModerationAction` has a closed schema with only required `action`; schema validation rejects extra fields. Its descriptor does not expose `target`, `arguments`, `hash`, or `nick`.
- Production-reference audit found no composition of `ModerationGateway` or `ModerationAction` into `run_command`, the public bounded tool loop, command dispatch/authorization, `main`, or runtime. `cmd/zenbot/main.go` continues to freeze exactly `user_message_history`, `room_users`, and `run_command` in the public loop.
- `SemanticModerationIngressReady()` returns literal `false`. `main` assigns that value only to `Pipeline.SemanticModerationReady` and does not compose `RoomParticipation.SemanticCandidate`; consequently semantic candidate transport remains inert string-target scaffolding.
- Listener scaffolding carries only a canonical resolved author name as `ModerationTarget`; it never derives a target from model output/tool arguments. Candidate and target are dropped unless a composer supplies `SemanticCandidate`, which current production composition does not.
- No nick/hash policy was selected or enabled. Public `run_command`, human dispatch/authorization, and raw protocol construction in agent packages were not changed by QA.

## Test logs

All commands were run from `/Users/ab/workspace/go-projects/zenbot`.

```text
$ go test ./internal/core ./internal/factory ./internal/agent/commandgateway ./internal/agent/tool ./internal/agent/participation ./internal/agent/live ./cmd/zenbot
ok  zenbot/internal/core                 0.394s
ok  zenbot/internal/factory              0.462s
ok  zenbot/internal/agent/commandgateway 0.812s
ok  zenbot/internal/agent/tool           0.641s
ok  zenbot/internal/agent/participation  (cached)
ok  zenbot/internal/agent/live           1.023s
ok  zenbot/cmd/zenbot                    1.163s

$ go test -race ./internal/core ./internal/factory ./internal/agent/commandgateway ./internal/agent/tool ./internal/agent/participation ./internal/agent/live
ok  zenbot/internal/core                 1.434s
ok  zenbot/internal/factory              1.446s
ok  zenbot/internal/agent/commandgateway 1.755s
ok  zenbot/internal/agent/tool           1.869s
ok  zenbot/internal/agent/participation  2.490s
ok  zenbot/internal/agent/live           2.317s

$ go vet ./...
exit 0 (no output)

$ go test ./...
exit 0; all packages passed (including `internal/command` in 8.338s and `internal/repository/h2` in 35.717s)

$ go build ./...
exit 0 (no output)

$ git diff --check
exit 0 (no output)
```

### Focused-package coverage obtained

```text
core:                 51.2%
factory:              55.9%
agent/commandgateway: 51.9%
agent/tool:           61.3%
agent/participation:  87.4%
agent/live:           76.5%
```

## Files changed by this QA pass

- Created: `.hermes/handoffs/copylock-semantic-recovery-qa.md`
- Modified task-owned source/tests by QA: none.

## Verdict

**PASS — no-fix.** Pointer-owned construction prevents mutex copying at the constructor seam; focused/race/full/static/build/diff gates pass. Semantic moderation remains uncomposed and fail-closed (`SemanticModerationIngressReady() == false`), with only inactive string-target construction scaffolding retained.
