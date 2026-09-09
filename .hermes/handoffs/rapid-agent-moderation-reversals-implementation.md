# Implementation handoff: typed semantic moderation reversals

## Status

**Implemented and verified. Semantic moderation remains inactive.**

- `SemanticModerationIngressReady()` remains `false` in `internal/agent/participation/semantic_moderation.go`.
- `cmd/zenbot/main.go` still leaves `RoomParticipation.SemanticCandidate` nil.
- No provider/model ingress, moderation tool loop, public command catalog, `run_command`, generic dispatch, authorization path, aliases, or `-all` behavior was added.

## Exact implementation files

Created:

- `internal/core/moderation_target.go`
- `internal/core/moderation_reversal.go`
- `internal/core/moderation_reversal_test.go`
- `.hermes/handoffs/rapid-agent-moderation-reversals-implementation.md`

Modified:

- `internal/repository/repository.go`
- `internal/repository/h2/shadow_ban.go`
- `internal/repository/h2/shadow_ban_test.go`

No pre-existing semantic Stage-A source/test/handoff file was modified by this vertical. Existing dirty Stage-A files and the architecture handoff were preserved. Saturn was not modified.

## Delivered behavior

- `core.NewModerationTarget(model.User)` creates a value-copy identity snapshot, rejects a blank name, and preserves exact `Name`, `Trip`, and raw `Hash` values without normalization.
- `core.ModerationReverser` exposes only `UnmuteTarget(context.Context, ModerationTarget)` and `UnshadowBanTarget(context.Context, ModerationTarget)`.
- `EngineImpl.UnmuteTarget` rejects canceled contexts and blank hashes, then sends only JSON marshaled `{"cmd":"unmute","hash":"<raw listener hash>"}` through `sendModerationCommand`/the existing context-aware outbound path. It has no reply path and no raw-send fallback.
- `EngineImpl.UnshadowBanTarget` rejects canceled contexts and blank names, requires only `repository.ShadowBanReversalRepository`, and delegates the exact captured `target.Name`; it emits no outbound protocol payload or chat reply.
- `h2.Database.RemoveShadowBanBySourceTarget` uses `ExecContext` with:
  ```sql
  DELETE FROM banned_users WHERE name=$1 OR trip=$2 OR hash=$3
  ```
  It binds the exact target as name/trip and `base64.StdEncoding.EncodeToString([]byte(target))` as hash. Zero-row deletes return nil and are idempotent.

## Source trace

- Saturn wire source: `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java`, `unmute(String)` queues `JsonPayloads.command("unmute", "hash", hash)`; the Go method sends the same two fields with the captured raw hash.
- Saturn persistence source: `ModServiceImpl.unshadowBan(String)` uses the `banned_users` name/trip/base64(target) deletion contract and does not queue a protocol message; the H2 method ports that predicate as parameterized SQL.
- Zenbot authority seam: `internal/listener/message/handlers.go`, `ResolveUserMetadata.Handle`, resolves/overwrites identity metadata. This vertical accepts only `model.User` at target construction and does not add any lookup or model-text target path.
- Zenbot outbound seam: `internal/core/engine_impl.go`, `sendModerationCommand` → `sendOutboundContext` preserves cancellation/deadline behavior for unmute.
- Existing H2 storage seam: `internal/repository/h2/shadow_ban.go`, `PersistShadowBan`, already stores standard-base64 hashes; it is unchanged apart from the narrow removal method.

## TDD evidence

RED was observed before production implementation:

```text
go test ./internal/core -run 'Test.*(Moderation|Unmute|Unshadow)' -count=1
# FAIL: undefined NewModerationTarget, ModerationTarget, UnmuteTarget, UnshadowBanTarget

go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1
# FAIL: Database.RemoveShadowBanBySourceTarget undefined
```

The focused tests cover copied target identity, blank-name/hash rejection, canceled contexts, blocked outbound deadline, exact unmute JSON, absence of unshadowban output, unavailable repository capability, exact name delegation, real-H2 name/trip/base64(name) deletion, zero-row idempotence, and SQL-looking target safety.

## Verification evidence

All commands ran from `/Users/ab/workspace/go-projects/zenbot` and passed:

```text
gofmt -w internal/core/moderation_target.go internal/core/moderation_reversal.go internal/core/moderation_reversal_test.go internal/repository/repository.go internal/repository/h2/shadow_ban.go internal/repository/h2/shadow_ban_test.go

go test ./internal/core -run 'Test.*(Moderation|Unmute|Unshadow)' -count=1
ok   zenbot/internal/core  0.500s

go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1
ok   zenbot/internal/repository/h2  2.665s

go test ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -run 'Test.*(Semantic|Moderation|LiveAgent)' -count=1
ok   zenbot/internal/agent/participation  0.325s
ok   zenbot/internal/agent/live  0.342s
ok   zenbot/cmd/zenbot  0.605s

go test -race ./internal/core ./internal/repository/h2 ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -count=1
ok   zenbot/internal/core  1.412s
ok   zenbot/internal/repository/h2  34.120s
ok   zenbot/internal/agent/participation  1.776s
ok   zenbot/internal/agent/live  2.121s
ok   zenbot/cmd/zenbot  2.246s

go test ./... -count=1
# passed: all listed packages; no failures

go build ./...
# exit 0

git diff --check
# exit 0
```

No commit or push was performed.
