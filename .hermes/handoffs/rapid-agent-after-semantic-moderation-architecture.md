# Rapid vertical: typed semantic moderation reversals (`unmute` + `unshadowban`)

## Decision

**Implement only the two missing typed reversal operations and their narrow persistence/protocol adapters. Do not activate semantic ingress in this vertical.** Route this to **@senior-developer**.

This is the smallest source-grounded prerequisite after the fail-closed Stage-A ingress work: `captcha`, `mute`, `kick`, and `shadowban` already have authoritative target-side operations, while `unmute` and `unshadowban` do not. It deliberately does **not** create a general moderator-command bridge, loosen the public `run_command` loop, call `DispatchUserCommand`, or compose `SemanticCandidate` in `cmd/zenbot/main.go`.

## Evidence and source semantics

### [OBSERVED] Saturn command and agent boundary

- `src/main/java/org/saturn/app/agent/tool/RunCommandTool.java`, `MODERATION_COMMANDS` (lines 45-47), is exactly `{captcha, mute, unmute, kick, shadowban, unshadowban}`.
- Its `execute(AgentContext, JsonObject)` lowercases the alias, checks it against caller capabilities, and delegates to `SaturnCommandGateway.executeWithResult` (lines 159-181). `isTargetedModerationCommand` includes both reversals (lines 207-210).
- `src/main/java/org/saturn/app/agent/room/AgentRoomMessagePipeline.java`, `handleSemanticModeration` (lines 205-224), creates a bot context with `MODERATION_COMMANDS`, sets `moderationTarget` to `turn.message.getNick()`, submits `MODERATION`, and returns `CONTINUE`. The pipeline ordering is monitor → eligibility → prepare → quiet → mention → semantic → ambient (lines 94-102).
- `src/main/java/org/saturn/app/command/impl/moderator/UnMuteUserCommandImpl.java`, `execute`, authorizes `Role.MODERATOR`, takes its first argument as a **hash**, calls `ModService.unmute(hash)`, emits a confirmation, and returns success (lines 23-48).
- `src/main/java/org/saturn/app/command/impl/moderator/UnShadowBanUserCommandImpl.java`, `execute`, authorizes `Role.MODERATOR`; with a non-`-all` first argument it calls `ModService.unshadowBan(target)` (lines 23-55). The `-all` branch is excluded from the agent inventory and from this vertical.

### [OBSERVED] Saturn wire and persistence behavior

- `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java`, `unmute(String hash)`, enqueues exactly `JsonPayloads.command("unmute", "hash", hash)` (lines 130-133): the outbound protocol is `{ "cmd": "unmute", "hash": <raw hash> }`.
- `ModServiceImpl.unshadowBan(String target)` uses `SqlUtil.DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH`, binding the same input as `name` and `trip`, and `Base64(target UTF-8)` as `hash` (lines 155-170). It emits **no** protocol message.
- `src/main/java/org/saturn/app/util/SqlUtil.java`, `DELETE_FROM_BANNED_USERS_WHERE_NAME_OR_TRIP_OR_HASH`, is therefore the persistence contract to port: delete matching records, not a chat-server ban/unban action.
- `ModServiceImpl.shadowBan(BanRecord)` base64-encodes a raw hash before persisting it (`ModServiceImpl.java`, lines 43-80). This agrees with current Zenbot H2 storage.
- Saturn command tests fix the unmute wire contract: `src/test/java/org/saturn/app/command/impl/moderator/UnMuteUserCommandImplTest.java`, `executeUnmutesHashAndQueuesConfirmation`, expects `{ "cmd": "unmute", "hash": "hash-a"}` (lines 23-33).

### [LIMITATION] Source semantic target mismatch must be resolved explicitly

Saturn semantic ingress carries only the author **nick** in `AgentContext.moderationTarget` (`AgentRoomMessagePipeline.handleSemanticModeration`), but `RunCommandTool` requires the first textual argument of `unmute` to equal that nick while `UnMuteUserCommandImpl` interprets that same argument as a **hash**. A normal author whose nick differs from hash cannot satisfy both intended identities. This is an observed source inconsistency, not a target-side authorization gap to copy.

The typed target operation below preserves the downstream source wire/persistence semantics while removing model-supplied identity selection. It must be documented and tested as a safe target normalization, not represented as literal end-to-end `RunCommandTool` parity. No semantic ingress activation may rely on a claim that this inconsistency has been reproduced or silently fixed.

## Target seam and gap trace

### [OBSERVED] Existing target seams

1. Listener metadata is authoritative: `internal/listener/message/handlers.go`, `ResolveUserMetadata.Handle`, resolves the active `model.User` and overwrites `c.Message.Hash` from it (lines 15-26). `model.User` contains `Name`, `Trip`, `Hash`, `IsBot`, and `Isme` in `internal/model/user.go`.
2. Stage-A currently uses only `c.Author.Name`: `internal/agent/live/participation.go`, `RoomParticipation.Handle` (lines 21-31), refuses missing/bot/self authors, but passes `ModerationTarget string` onward.
3. The invocation factory creates the bot/system principal and supplies only `MODERATION_COMMANDS`: `internal/agent/participation/invocation.go`, `CreateModeration` (lines 57-74). It neither grants the alleged author authority nor accepts model identity.
4. Existing authoritative operations live in `internal/core/engine_impl.go`: `EnableCaptcha`, `MutePrincipal`, `KickPrincipal`, and `ShadowBan`. `ShadowBan` requires `repository.ShadowBanRepository` and persists the resolved active `model.User`; `MutePrincipal` and `KickPrincipal` emit fixed JSON through `sendModerationCommand` (lines 523-605).
5. H2 already stores source-compatible shadow-ban records: `internal/repository/h2/shadow_ban.go`, `PersistShadowBan`, base64-encodes `model.User.Hash` and inserts `trip,name,hash,reason,created_on`; `internal/repository/h2/database.go`, `bootstrap`, creates `banned_users` (lines 350-355).
6. The public model-facing command path is intentionally unsuitable: `internal/agent/tool/run_command.go`, `RunCommand`, has a closed informational alias enum; `internal/agent/live/tool_loop.go`, `NewBoundedToolLoop` and `frozenPublicTools`, freeze exactly history, room-users, and that public `run_command` (lines 280-315). `internal/listener/message/handlers.go`, `DispatchUserCommand.Handle`, uses a user principal and `IsUserAuthorized` (lines 167-189).
7. `internal/agent/participation/semantic_moderation.go`, `SemanticModerationIngressReady`, remains `false`; the Stage-A QA handoff records that this is intentional until all six aliases have reviewed typed operations.

### [GAP]

There is no target-side `Unmute…` method, no typed shadow-ban removal repository method, and no immutable canonical target with both name and raw hash. The regular command catalog only replies `"unmute accepted"` / `"unshadowban accepted"` (`internal/command/registry.go`, `saturnCommand.Execute`, lines 95-96); it is not an implementation seam for autonomous moderation.

## Bounded design

### Typed interfaces and modules

Add a small, non-agent API at the core/repository boundary:

```go
// internal/core/moderation_target.go (new)
type ModerationTarget struct {
    Name string
    Trip string
    Hash string // raw listener-resolved hash; never model input
}

func NewModerationTarget(user model.User) (ModerationTarget, error)

// internal/core/moderation_reversal.go (new)
type ModerationReverser interface {
    UnmuteTarget(context.Context, ModerationTarget) error
    UnshadowBanTarget(context.Context, ModerationTarget) error
}

// internal/repository/repository.go (extend)
type ShadowBanReversalRepository interface {
    RemoveShadowBanBySourceTarget(context.Context, string) error
}
```

`EngineImpl` implements `ModerationReverser`:

- `UnmuteTarget` rejects a canceled context and blank `target.Hash`, then emits only `{"cmd":"unmute","hash":<target.Hash>}` through the existing context-aware outbound path. It must not accept a free-form string, normalize the hash, emit a room confirmation, or fall back to `SendRawMessage`.
- `UnshadowBanTarget` rejects a canceled context and blank `target.Name`, asserts `Repository` implements `ShadowBanReversalRepository`, and invokes `RemoveShadowBanBySourceTarget(ctx, target.Name)`. It emits no chat-server protocol payload and no room reply.
- `h2.Database.RemoveShadowBanBySourceTarget` executes the source-shaped parameterized delete: `DELETE FROM banned_users WHERE name=$1 OR trip=$2 OR hash=$3`, with name/trip = the exact authoritative `target.Name` and hash = standard base64 encoding of its UTF-8 bytes. A zero-row delete is a successful idempotent reconciliation, matching Saturn's ignored update count.

The raw hash is required only by typed unmute. The name is deliberately the source-shaped selector for typed unshadowban; trip and raw hash are retained in `ModerationTarget` to prove its listener-originated identity and make accidental use of model text unnecessary, not to broaden the source delete predicate.

### Authoritative trust and authorization model

- **Authority:** only the listener-resolved active `message.Context.Author` may construct `ModerationTarget`; do not construct it from `ChatMessage`, prompt text, `api.Context.ModerationTarget`, tool JSON, or provider output.
- **Principal:** the future autonomous caller remains the configured bot/system identity produced by `InvocationFactory.CreateModeration`, with exactly `api.ModerationCommands`; the alleged author never obtains capabilities.
- **Operation selection:** this vertical does not expose either operation to an LLM, so no model call can select alias or target yet. A later MODERATION-only gateway may select a reviewed alias, but it must pass the pre-captured typed target and must never take an argument string as the target.
- **No user-command reuse:** prohibit `common.BuildCommand`, `DispatchUserCommand`, `commandgateway.Gateway`, `Ban`, `Unban`, and `run_command`. Their authorization and reply behavior are for human commands/public informational tools, not autonomous actions.

### Proposed control/data flow (not activated here)

```text
active-user roster
   │ ResolveUserMetadata (authoritative Name/Trip/Hash)
   ▼
listener-only target capture ──► ModerationTarget{Name, Trip, raw Hash}
   │                                 │ held request-locally for a later
   │                                 │ MODERATION-only action loop
   │                                 ├─ UnmuteTarget ─► fixed outbound JSON
   │                                 │                   {cmd:"unmute", hash:raw Hash}
   │                                 └─ UnshadowBanTarget ─► H2 source-shaped DELETE
   ▼
Stage-A semantic candidate remains disabled; no provider/model ingress
```

The target must be copied when captured. It must not look the user up again by a model-selected name at execution time; leaving the room afterward does not convert a captured identity into an arbitrary lookup. The execution context cancellation/deadline remains authoritative for both branches.

## File map

| Path | Change | Purpose |
|---|---|---|
| `internal/core/moderation_target.go` | new | Validate/copy the listener-resolved target identity. |
| `internal/core/moderation_reversal.go` | new | `EngineImpl.UnmuteTarget` and `EngineImpl.UnshadowBanTarget`; fixed wire/persistence delegation. |
| `internal/core/moderation_reversal_test.go` | new | Core protocol, validation, repository capability, cancellation, and no-output tests. |
| `internal/repository/repository.go` | modify | Add only `ShadowBanReversalRepository`; do not widen `Repository`. |
| `internal/repository/h2/shadow_ban.go` | modify | Add parameterized source-shaped delete and keep existing insert untouched. |
| `internal/repository/h2/shadow_ban_test.go` | modify | Real-H2 deletion/matching/no-match tests. |
| `internal/agent/live/participation.go` | no change in this vertical | Existing Stage-A adapter remains fail-closed and target remains a string. A later activation vertical needs a separately reviewed typed target transport. |
| `internal/agent/participation/semantic_moderation.go` | no change in this vertical | Keep `SemanticModerationIngressReady() == false`. |
| `cmd/zenbot/main.go` | no change in this vertical | Keep `SemanticCandidate` nil; do not enable provider ingress. |
| `internal/agent/tool/run_command.go`, `internal/agent/live/tool_loop.go` | no change | Public three-tool loop remains frozen. |

No config, migration-plan, frozen-audit, Saturn-tree, or existing handoff changes belong in this vertical.

## H2 impact

No schema migration is required: `banned_users(trip,name,hash,reason,created_on)` already exists and its hash encoding already matches Saturn insert behavior. The only H2 change is a parameterized delete operation against that table. It must use `ExecContext`, preserve context cancellation, and return database errors rather than log-and-swallow them; the core action then reports failure through its normal caller path. This is a consciously safer Go error boundary than Saturn's `SQLException` logging, while retaining the successful-delete semantics.

## Compatibility invariants and exclusions

1. Preserve exact unmute wire field names and raw-hash value; never send `nick`, `trip`, `ban`, `unban`, or an unquoted hand-built payload.
2. Preserve source unshadowban as local `banned_users` deletion, using source's name/trip/base64(name) predicate; do not send a remote unban command.
3. Preserve base64 standard encoding of UTF-8 bytes for stored/matched hash fields.
4. A blank required identity, unavailable repository capability, canceled context, or blocked outbound queue fails closed and produces no partial action.
5. No success/failure room reply, tool evidence, memory append, retry, or new audit/evidence persistence is introduced. This agrees with the current MODERATION lifecycle: `internal/agent/live/runner.go`, `AfterDelivery`, appends only replies (lines 182-195).
6. Do not implement `-all`, aliases (`undumb`, `shadowmercy`, `unblock`), `ban`, `unban`, human command confirmations, or model textual target parsing.
7. Do not alter the source complete alias list or return `true` from `SemanticModerationIngressReady`; the prerequisite becomes available but activation still requires a separately reviewed all-six-alias MODERATION-only gateway/tool-loop composition.

## Complexity and risk

**Complexity: small (two core methods, one narrow repository capability, one existing H2 file).**

**Risk: high authorization sensitivity, moderate data-destructive persistence risk.** `unmute` changes remote moderation state; `unshadowban` deletes local enforcement records. The source nick/hash mismatch increases parity risk. Mitigations are typed listener-only identity, fixed operation methods, context-aware I/O, no model exposure, parameterized SQL, and senior review. Any proposal to combine this with model/provider ingress or the public command path changes the risk class to large and is out of scope.

## Red–green test plan

Write tests before implementation.

1. **RED core target construction:** reject blank `Name`; reject blank `Hash` for unmute; prove copied target does not mutate when the original `model.User` changes.
2. **RED unmute wire:** with a buffered engine queue and `{Name:"author", Hash:"hash-a"}`, assert exactly `{"cmd":"unmute","hash":"hash-a"}`; assert no chat reply. Reject blank hash/canceled context before queue write; verify a blocked queue honors deadline.
3. **RED unshadowban core boundary:** use a repository stub implementing only the new reversal interface; assert it receives exactly `target.Name`; unavailable repository and blank name fail with no outbound message. Assert no lookup by arbitrary string and no `Ban`/`Unban` call.
4. **RED real H2:** seed rows whose `name`, `trip`, and base64 hash respectively equal an authoritative name, plus a nonmatching row. `RemoveShadowBanBySourceTarget("author")` removes precisely source-matched rows. A second call succeeds with zero rows. Verify SQL injection-looking name is a value, not executable SQL.
5. **GREEN implementation:** add the smallest methods/interfaces required to satisfy those tests; retain current `PersistShadowBan` test unchanged.
6. **Regression:** existing Stage-A tests still prove ingress is disabled: `TestSemanticModerationIngressStaysDisabledUntilAllSourceAliasesHaveTypedOperations` and `TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig` (which asserts both `SemanticCandidate == nil` and `SemanticModerationReady == false`).

## Minimal rapid verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation:

```bash
gofmt -w internal/core/moderation_target.go internal/core/moderation_reversal.go internal/core/moderation_reversal_test.go internal/repository/repository.go internal/repository/h2/shadow_ban.go internal/repository/h2/shadow_ban_test.go
go test ./internal/core -run 'Test.*(Moderation|Unmute|Unshadow)' -count=1
go test ./internal/repository/h2 -run 'Test.*ShadowBan' -count=1
go test ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -run 'Test.*(Semantic|Moderation|LiveAgent)' -count=1
go test -race ./internal/core ./internal/repository/h2 ./internal/agent/participation ./internal/agent/live ./cmd/zenbot -count=1
git diff --check
```

Run `go test ./... -count=1` and `go build ./...` before accepting the implementation if the rapid checks pass. `go vet ./...` remains informational only if it reports the pre-existing `internal/core/engine_impl.go:95` lock-by-value warning documented in the Stage-A QA evidence.

## Relationship to semantic ingress activation

This vertical closes only the **operation** half of the `unmute`/`unshadowban` gate. It does not close the **transport/composition** half:

- Stage-A stays deliberately fail-closed: `SemanticModerationIngressReady()` remains false and `newLiveAgent` leaves `RoomParticipation.SemanticCandidate` nil.
- A subsequent senior-reviewed vertical must introduce a separately frozen MODERATION-only tool/gateway loop that maps the complete six-alias source set to six typed operations, carries a captured typed target rather than the current string-only `ModerationTarget`, verifies one-action/cancellation/silent-lifecycle behavior, and only then changes readiness/composition.
- That activation vertical must resolve the documented Saturn nick/hash inconsistency explicitly and prove it cannot let model output select a target. It must not be folded into this reversal implementation.

## Completion checklist

- [ ] Exactly two reversal operations exist behind typed core/repository boundaries.
- [ ] Unmute protocol and unshadowban H2 deletion match the source semantics above.
- [ ] Listener-originated identity is the only operation target source.
- [ ] Existing semantic staging files and fail-closed composition are unchanged.
- [ ] Focused core/H2/staging regressions, race checks, diff check, full tests, and build pass.
- [ ] Senior review confirms no public command, model/provider ingress, broad moderator migration, or `-all` behavior entered scope.
