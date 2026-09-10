# Deferred Saturn Agent API Contract Architecture

**Status:** `[RECOMMENDED]` design for the deferred API-contract slice; no application code changed.
**Scope:** `AgentInvocation`, `AgentContext`, `AgentResult`, `AgentUserIdentity`, their construction/validation/JSON boundary, and migration of existing Zenbot callers. Routing, tool execution, turns, persistence, moderation, listeners, commands, and provider behavior are excluded.

## 1. Evidence and source boundary

All Saturn citations below are relative to the read-only checkout `/Users/ab/workspace/projects/saturn`; all Zenbot citations are relative to this repository. Claims are labeled `[OBSERVED]`, `[TEST-BACKED]`, `[LIMITATION]`, or `[RECOMMENDED]`.

- `[OBSERVED]` Saturn API definitions are records in `src/main/java/org/saturn/app/agent/api/AgentInvocation.java`, `AgentContext.java`, `AgentResult.java`, `AgentUserIdentity.java`, and enum `AgentInvocationMode.java`.
- `[OBSERVED]` The current Go public-looking DTO scaffold is `internal/agent/api/api.go`: `Invocation`, `Context`, `Memory`, `Identity`, `Result`, plus `ValidateInvocation`. It is not used by current Go agent runtime callers.
- `[OBSERVED]` The active Go execution contract is instead `internal/agent/runtime/contracts.go`, with private fields, accessors, aliases `AgentInvocation = Invocation`, `AgentContext = Context`, `AgentResult = Result`, constructors, `Runner`, `Sink`, and `InvocationFactory`.
- `[LIMITATION]` Saturn API records have no JSON/serialization annotations or custom serializer in the cited API directory. Focused API tests exercise constructors and values, not JSON bytes. The repository uses plain `new Gson()` in relevant code (for example `src/main/java/org/saturn/app/agent/llm/provider/openai/OpenAiCompatibleClient.java:38` and `src/main/java/org/saturn/app/agent/persistence/H2AgentSqlRepository.java:26`), but no source evidence establishes a wire schema for these four records. The target must therefore make JSON semantics explicit and test them rather than infer them from record names.

## 2. Saturn contract (exact observed shape)

### AgentInvocation

`src/main/java/org/saturn/app/agent/api/AgentInvocation.java:7-13` defines record components, in order:

```text
requestId: String
context: AgentContext
prompt: String
mode: AgentInvocationMode
currentMessageText: String
commandOriginated: boolean
```

The canonical record constructor (`:14-24`) rejects null/blank `requestId`, null `context`, null/blank `prompt`, and null `mode`. It does not reject blank `currentMessageText` and does not normalize text. Overloads (`:26-53`) provide these defaults: three-argument constructor => `DIRECT`, `currentMessageText=null`, `commandOriginated=false`; four-argument mode constructor => current text null/command false; five-argument constructor => command false; two-argument `(context,prompt)` constructor generates `UUID.randomUUID().toString()` and uses `DIRECT`, null current text, false command origin.

`[TEST-BACKED]` `src/test/java/org/saturn/app/agent/AgentInvocationTest.java:13-36` verifies command-origin default and rejection of null/blank request IDs and prompts. `AgentParticipationConfigTest.java:19-29` verifies the generated-ID/default-mode path indirectly.

### AgentContext

`src/main/java/org/saturn/app/agent/api/AgentContext.java:8-16` defines:

```text
room: String
nick: String
trip: String
hash: String
whisper: boolean
roomUsers: List<String>
capabilities: Set<AgentCapability>
moderationTarget: String
```

The canonical constructor (`:33-40`) requires non-null `room`, `nick`, `roomUsers`, and `capabilities`; `trip`, `hash`, and `moderationTarget` are nullable. It retains defensive immutable copies with `List.copyOf` and `Set.copyOf`. The six-argument overload (`:52-55`) defaults capabilities to empty and moderation target to null; the seven-argument overload (`:68-76`) defaults moderation target to null. Empty collections are valid.

`hasCapability` is set membership (`:85-87`). `memoryKey` (`:94-107`) is only a key derivation helper: room prefix is `"<room.length()>:<room>"`; public context appends `|public`; whisper context chooses nonblank trip, else nonblank hash, else nick and appends `|whisper|trip:<trip>`, `|whisper|hash:<hash>`, or `|whisper|nick:<nick>`. This helper does not lowercase or strip the selected values.

`[TEST-BACKED]` `src/test/java/org/saturn/app/agent/AgentContextTest.java:18-32` verifies defensive copies and unmodifiable accessors; `:35-48` verifies the exact memory-key shape and precedence; `:51-76` verifies null rejection for required fields and valid empty capabilities. `src/test/java/org/saturn/app/agent/room/AgentQuietRegistryTest.java:51-58` verifies identity fallback and normalization separately from `memoryKey`.

### AgentInvocationMode

`src/main/java/org/saturn/app/agent/api/AgentInvocationMode.java:4-8` has exactly `DIRECT(true)`, `MENTION(true)`, `AMBIENT(false)`, `MODERATION(false)`. `requiresReply()` returns the stored mode property (`:10-28`). Java enum names are uppercase. No unknown enum can be constructed through the Java type system, but JSON parsing behavior is not covered by a Saturn API test.

### AgentResult

`src/main/java/org/saturn/app/agent/api/AgentResult.java:6` defines:

```text
correlationId: String
content: String
shouldReply: boolean
```

The canonical constructor (`:7-12`) rejects null/blank correlation IDs and null content; empty content is valid. The two-argument overload (`:20-22`) defaults `shouldReply=true`; `reply` (`:31-33`) does the same; `silent` (`:41-43`) returns empty content and `shouldReply=false`. There is no error-code field in Saturn `AgentResult`.

`[TEST-BACKED]` `AgentParticipationConfigTest.java:25-29` verifies reply/silent defaults and silent empty content. Numerous service/router tests construct the two-argument form; representative uses are `src/test/java/org/saturn/app/service/impl/AgentServiceImplTest.java:38,66,99`.

### AgentUserIdentity

`src/main/java/org/saturn/app/agent/api/AgentUserIdentity.java:9-14` is a one-component record `value:String`; its constructor rejects null/blank value. `from(AgentContext)`, `from(ChatMessage)`, and `from(User)` (`:22-47`) all delegate to the private precedence function (`:57-65`): nonblank trip wins, else nonblank hash, else required nick. The selected value is `strip()`-trimmed and lowercased with `Locale.ROOT` (`:73-75`), then prefixed exactly `trip:`, `hash:`, or `nick:`. Null nick reaches `Objects.requireNonNull` and fails.

`[TEST-BACKED]` `AgentQuietRegistryTest.java:51-58` verifies hash fallback despite differing nicks and normalized-nick fallback. `AgentQuietRegistry.java` usage shown by `:93-98` in the production search confirms the identity is a map key for room/user quiet state.

## 3. Null, omitted, and serialization semantics

- `[OBSERVED]` Saturn API classes do not declare Jackson/Gson annotations, `toJson`, `fromJson`, or custom adapters in their source files. No focused Saturn test asserts JSON for these records.
- `[OBSERVED]` Saturn distinguishes nullable values in the Java object model: `trip`, `hash`, `moderationTarget`, and `currentMessageText` may be null; required strings/collections may not be null. Empty strings are not equivalent to null for constructor validation, while blank trip/hash are treated as absent only by identity/key fallback helpers.
- `[LIMITATION]` Whether an external Gson caller emits null-valued record components is not proven by the Saturn API source/tests. Do not claim exact “omitted versus explicit null” wire behavior until a serialization experiment or project-specific adapter is located.
- `[RECOMMENDED]` Define a Go JSON DTO boundary with explicit pointers for nullable scalar fields (`*string` for trip/hash/moderationTarget/currentMessageText where null must be preserved) and non-pointer required strings/bools. Use `omitempty` only for fields whose absent and null states are intentionally identical; otherwise marshal explicit `null`. Keep internal value validation separate from wire decoding so omitted required fields fail deterministically.
- `[RECOMMENDED]` Use exact lower-camel JSON names matching the Java record component names (`requestId`, `context`, `prompt`, `mode`, `currentMessageText`, `commandOriginated`, `room`, `nick`, `trip`, `hash`, `whisper`, `roomUsers`, `capabilities`, `moderationTarget`, `correlationId`, `content`, `shouldReply`, `value`). This is a compatibility target, not an observed Saturn wire test.

## 4. Current Zenbot DTOs and callers

### Current DTO mismatch

`internal/agent/api/api.go:22-49` currently defines `Invocation{Mode, IdentityKey, Text, Room, CreatedOn}`, `Context{Invocation, Messages, Memory}`, `Memory`, `Identity{Key,DisplayName}`, and `Result{Text,NoReply,ErrorCode}`. These fields do not match Saturn component shape, and `ValidateInvocation` (`:51-62`) requires mode, nonblank room, and nonzero `CreatedOn`—requirements not present in Saturn `AgentInvocation` and lacking request ID/context/prompt validation. `Result.ErrorCode` is target-only; Saturn `AgentResult` has no error-code component.

### Active runtime callers

`internal/agent/runtime/contracts.go` is the actual current caller seam. Constructors are called by:

- `internal/agent/runtime/runtime_test.go:16,24,45,83,118,154`;
- `internal/agent/assemble/assemble_test.go:28-30,36,49,77,90,109,122,129,160,169,180`;
- production runtime access is `internal/agent/runtime/runtime.go:56-114`, notably room locking via `invocation.Context().Room()` and delivery via `ShouldReply()`;
- production assembly access is `internal/agent/assemble/assemble.go:64-111,240-279`, using context identity/capabilities/users and invocation prompt/mode.

`internal/agent/api` has no imports or construction call sites in the visible target source; `internal/agent/api/api_test.go` only tests direct struct literals and JSON for the reduced DTOs (`:9-29`). Therefore replacing those structs immediately would break their package tests and any hidden/future callers, while changing runtime types would break the current private runtime/assembler seam.

### Existing identity helper mismatch

`internal/model/records.go:49-57` defines `IdentityKey(trip, hash, nick)`: trip/hash are trimmed but not lowercased; nick is lowercased after trim. This differs from Saturn `AgentUserIdentity`, which strips and lowercases whichever identity source wins. It is used by snapshot/engine code (for example `internal/listener/snapshot/snapshot.go:72` and `internal/core/engine_impl.go:359`), so it must not be silently changed as part of this agent API slice.

## 5. Minimal compatible target design

`[RECOMMENDED]` Keep `internal/agent/runtime` as the execution seam and introduce exact Saturn-shaped contracts in `internal/agent/api` rather than aliasing the incompatible reduced structs.

### Package/file map

1. `internal/agent/api/invocation.go`: `InvocationMode` (or retain it in `api.go`), exact `Invocation` with six Saturn fields, constructors mirroring all five Java overload behaviors, `ValidateInvocation`, and explicit JSON marshal/unmarshal tests.
2. `internal/agent/api/context.go`: exact eight-field `Context`, defensive-copy constructors, capability membership, `MemoryKey`, and nullable-field handling.
3. `internal/agent/api/result.go`: exact three-field `Result`, `Reply` and `Silent` factories, validation, and no `ErrorCode` in the Saturn-compatible type.
4. `internal/agent/api/identity.go`: `UserIdentity`, `FromContext`, and a narrow generic/value helper for `(trip,hash,nick)`; do not couple this API package to listener/model types unless a separate adapter package is required.
5. `internal/agent/api/json_test.go` and focused type tests: golden JSON, omitted/null cases, constructor defaults, validation, defensive copies, and identity vectors.
6. `internal/agent/runtime/adapters.go`: migration adapters between `api.*` and `runtime.*`, only if callers cannot be migrated atomically. Adapters must document lossy fields (runtime currently has no capabilities type equivalence issue only if mapped explicitly; runtime lacks a first-class identity value and `Result` has target-only `errorCode`).

### Interfaces and migration strategy

- `[RECOMMENDED]` Do not add a broad interface merely to hide DTO differences. The existing `runtime.Runner`, `runtime.Sink`, and `runtime.InvocationFactory` in `contracts.go:114-127` remain the narrow behavioral seams.
- Preferred migration is an atomic caller migration: make runtime `Invocation`/`Context`/`Result` aliases or wrappers around `api` contracts, then update runtime and assembler accessors while preserving constructor names through compatibility functions. This preserves the current runner/sink signatures as much as Go permits.
- If staged migration is required, add explicit `api.ToRuntime`/`api.FromRuntime` adapters or `runtime.New...FromAPI` functions. Do not use field-name guessing or JSON round-tripping as an adapter.
- Preserve `runtime.Result.ErrorCode` only in a runtime execution/error envelope, not in the Saturn-compatible API result. Mapping API result to runtime result should set empty error code; mapping runtime error results to Saturn result requires an explicit policy because Saturn has no error-code field and no observed error-result constructor.
- Preserve current `runtime.NewContext` and `NewContextWithCapabilities` signatures during the first migration step; add a capability conversion only where `api.AgentCapability` and runtime `Capability` are proven equivalent by source. Existing `runtime.Context` currently stores capabilities as a slice and maps nil input to an empty copied slice (`contracts.go:42-54`), while Saturn requires non-null set semantics.

## 6. Breaking changes and validation rules

### Breaks to plan explicitly

- Replacing `api.Invocation` fields breaks direct literals using `Mode`, `IdentityKey`, `Text`, `Room`, and `CreatedOn` (`internal/agent/api/api_test.go:10,14`).
- Replacing `api.Context`, `api.Memory`, `api.Identity`, or `api.Result` breaks JSON consumers and direct literals; `ErrorCode` cannot remain in an exact Saturn `Result` without making it a non-parity extension.
- Changing runtime contracts from `runtime.*` to `api.*` affects `Runner`, `Sink`, `Assembler`, and all tests listed above. Type aliases can reduce source breakage, but accessor/constructor changes remain a migration concern.
- Changing `model.IdentityKey` to Saturn normalization would alter unrelated snapshot/engine identity behavior and is excluded.

### Required validation

- Invocation: reject blank request ID, nil context, blank prompt, and nil/unknown mode; allow nil current-message text and either boolean value for command origin.
- Context: reject nil room, nick, room-users, and capabilities; allow nil trip/hash/moderation target; preserve empty collections; copy input collections and return copies/unmodifiable views.
- Result: reject blank correlation ID and nil content; allow empty content; default two-argument result to reply and silent result to empty content/no reply.
- Identity: reject blank identity value; `FromContext` requires nonnil context and nonnil nick when trip/hash are absent; precedence is nonblank trip > nonblank hash > nick; selected source is strip + `Locale.ROOT`-equivalent lowercase.
- JSON: reject malformed/unknown modes; test missing required fields separately from explicit null; preserve null for nullable Saturn fields; preserve empty strings/arrays when present; never infer `CreatedOn` or `IdentityKey` because neither is a Saturn component.

## 7. Test plan and verification

`[RECOMMENDED]` Add focused table-driven tests before migration:

1. Every invocation constructor overload: generated/non-generated ID, defaults, all modes, null current text, command-origin flag, and rejection cases.
2. Context constructors: six/seven/eight-argument shapes, null/empty distinctions, defensive copy and returned-copy mutation, capability membership, exact `MemoryKey` vectors.
3. Result factories and constructor: reply/silent defaults, empty content, null/blank rejection, and proof that parity result has no serialized error code.
4. Identity vectors: trip/hash/nick precedence, blanks, whitespace stripping, Unicode/`Locale.ROOT` lowercasing, null nick failure, and equality/hash behavior.
5. JSON golden tests: all fields, null nullable fields, omitted nullable fields, empty collections, uppercase modes, unknown mode, missing required fields, and round-trip preservation. These tests define the target because Saturn has no focused JSON evidence.
6. Caller migration tests: compile/use existing runtime and assembler tests unchanged where compatibility is promised; add explicit adapter tests for every lossy field and error mapping.

Run after implementation (not performed by this analysis): `gofmt` on task-owned files; focused `go test ./internal/agent/api ./internal/agent/runtime ./internal/agent/assemble`; `go test -race` for those packages; then `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`. Confirm only intended API/migration/test files changed.

## 8. Complexity, risk, and exclusions

- Complexity: **medium**. The data types are small; risk is concentrated in Go compatibility, JSON null/omission decisions, defensive copying, and migration of the separate runtime seam.
- Highest risks: accidentally preserving reduced DTO semantics under Saturn names; silently converting null to empty string; using `model.IdentityKey` as a substitute despite different normalization; losing `errorCode`; and breaking hidden callers through struct-literal field removal.
- `[LIMITATION]` No Saturn API JSON golden tests or API-specific serializer were found, so exact external wire behavior remains an evidence gap. The target contract must record its chosen policy in tests rather than label it observed parity.
- Exclusions: routing and classification; tool contracts/execution; turns/freshness; persistence/schema/memory; moderation; listeners and lifecycle; commands; LLM/provider behavior; config; changes to Saturn; changes to unrelated `model.IdentityKey`; and application-code implementation in this slice.

## 9. Analysis checklist

- `[OBSERVED]` Exact Saturn fields, overloads, validation, helper behavior, and enum values cited.
- `[TEST-BACKED]` Constructor, defensive-copy, memory-key, identity, and result behavior tied to Saturn tests.
- `[OBSERVED]` Current Go DTOs and runtime call sites identified.
- `[RECOMMENDED]` Minimal package map, interfaces, adapters, validation, JSON policy, test plan, risks, and exclusions separated from facts.
- `[LIMITATION]` Missing Saturn JSON evidence called out explicitly; no speculative wire behavior presented as fact.
