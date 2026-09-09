# Typed semantic moderation reversals QA

## Verdict: PASS

The bounded reversal implementation meets the requested typed core/repository boundary and remains unactivated. No concrete scoped defect was found, so no production/test source was changed by this QA pass.

## Source parity evidence

Read-only Saturn `develop` at `/Users/ab/workspace/projects/saturn`:

- `ModServiceImpl.unmute(String)` enqueues `JsonPayloads.command("unmute", "hash", hash)` (`src/main/java/org/saturn/app/service/impl/ModServiceImpl.java:130-133`). Zenbot `EngineImpl.UnmuteTarget` calls the existing context-aware `sendModerationCommand(ctx, "unmute", map[string]string{"hash": target.Hash})` (`internal/core/moderation_reversal.go:20-31`), which JSON-marshals only `cmd` and `hash` then uses `sendOutboundContext` (`engine_impl.go:589-605`, `307-320`). Focused test observed exactly `{"cmd":"unmute","hash":"hash-a"}`.
- Saturn `ModServiceImpl.unshadowBan(String)` has no outbound protocol call and uses prepared delete bindings `target`, `target`, base64 UTF-8 target (`ModServiceImpl.java:155-164`; `SqlUtil.java:77-78`). Zenbot uses `ExecContext` with `DELETE FROM banned_users WHERE name=$1 OR trip=$2 OR hash=$3`, binding exact target twice and `base64.StdEncoding.EncodeToString([]byte(target))` (`internal/repository/h2/shadow_ban.go:28-36`).

## Scoped behavior evidence

- `NewModerationTarget(model.User)` snapshots exact `Name`, `Trip`, and raw `Hash`, rejects blank/whitespace name, and does not normalize values (`internal/core/moderation_target.go:10-26`). `TestNewModerationTargetCopiesListenerResolvedIdentity` mutates the source `model.User` after construction and confirms target values remain `author/trip/raw-hash`.
- The new API accepts a typed `ModerationTarget`, never a free-form target string. Repository removal receives only `target.Name`; search found no production call sites for either typed reversal beyond their definitions, so no human command, public `run_command`, model/tool loop, dispatch, `Ban`, or `Unban` path reaches them.
- Unmute rejects pre-cancelled contexts and blank hashes before sending. A zero-capacity outbound queue with a 20 ms deadline returns `context.DeadlineExceeded`; no reply path or raw-send fallback exists.
- Unshadowban rejects pre-cancelled contexts, blank names, and repositories lacking `ShadowBanReversalRepository` before delegation; it emits no queue payload. Repository errors are returned unchanged by core delegation (`moderation_reversal.go:45-49`) and SQL errors are returned by H2 (`shadow_ban.go:35-36`).
- Real-H2 tests prove name/trip/base64(name) matching, unrelated-row preservation, zero-row idempotence, and SQL-looking input treated as a bound value.

## Semantic activation status: disabled / fail-closed

- `SemanticModerationIngressReady()` is still hard-coded `false` (`internal/agent/participation/semantic_moderation.go:68-72`).
- `newLiveAgent` sets `SemanticModerationReady` from that function but does not populate `RoomParticipation.SemanticCandidate` (`cmd/zenbot/main.go:165`). Its zero value is nil.
- The focused Stage-A tests passed. No semantic ingress, provider/model route, public command/catalog change, authorization/dispatch change, or activation composition was introduced by this vertical.

Note: the Stage-A readiness comment still says the two operations are absent. It is stale after this vertical but has no runtime effect; Stage-A files were intentionally read-only in this QA scope, and readiness remains false.

## Commands and results

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

```text
go test ./internal/core -run 'Test.*(Moderation|Unmute|Unshadow)' -count=1
ok   zenbot/internal/core  0.476s

go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1
ok   zenbot/internal/repository/h2  2.363s

go test ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -run 'Test.*(Semantic|Moderation|LiveAgent)' -count=1
ok   zenbot/internal/agent/participation  0.197s
ok   zenbot/internal/agent/live  0.369s
ok   zenbot/cmd/zenbot  0.508s

go test -race ./internal/core ./internal/repository/h2 ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -count=1
ok   zenbot/internal/core  1.341s
ok   zenbot/internal/repository/h2  34.071s
ok   zenbot/internal/agent/participation  1.820s
ok   zenbot/internal/agent/live  1.783s
ok   zenbot/cmd/zenbot  2.057s

go test ./... -count=1
PASS: all packages; internal/repository/h2 33.371s

go build ./...
exit 0

gofmt -d [six scoped Go files]
no output

git diff --check
exit 0
```

`go vet ./...` emitted only the documented pre-existing warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files fixed by this QA

- No implementation files changed; no concrete scoped defect was found.
- `.hermes/handoffs/rapid-agent-moderation-reversals-qa.md` — this QA report.

No commit or push was performed.
