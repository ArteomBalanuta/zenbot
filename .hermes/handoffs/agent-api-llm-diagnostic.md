# Agent API / Config / LLM / OpenAI QA Diagnostic

**Date:** 2026-08-30  
**Target:** `zenbot`  
**Reference:** read-only Saturn tree at `/Users/ab/workspace/projects/saturn`  
**Verdict:** QA FAIL is substantively correct. The Go slice builds and its existing gates pass, but several explicitly required behaviors are either incomplete or only incidentally implemented, and the API DTOs are not an exact Saturn contract.

## Executive diagnosis

The implementation handoff's statement that there were "no known build or test gaps" was too strong. The green commands prove compilation, package behavior covered by current tests, race safety for that coverage, vet, and build; they do not prove Saturn parity. The QA report correctly identifies six remaining categories:

1. OpenAI retry delay parses only integer seconds, not standard HTTP-date `Retry-After`.
2. Cancellation is wired through request and backoff paths, but there is no focused test that cancels during a real in-flight request or during a real retry sleep and asserts `errors.Is` identity.
3. Successful response provider diagnostics are discarded.
4. Defensive cloning handles only `map[string]any` and `[]any`; typed nested JSON-compatible maps/slices (and other alias-bearing representations) remain mutable through retained references.
5. `ValueReader.Bool`/`Int` silently return fallbacks on malformed values, unlike Saturn's strict environment readers; an absent `APIKeyEnv` does not resolve the Saturn default variable name `SATURN_AGENT_API_KEY`.
6. The Go API DTOs (`Invocation`, `Context`, `Memory`, `Result`, `Identity`) are a reduced, different shape, not a provable serialization-compatible implementation of Saturn's `AgentInvocation`, `AgentContext`, `AgentResult`, and `AgentUserIdentity`.

The first five are bounded-slice repairs. The sixth requires an explicit contract decision: amend the architecture to stop calling the reduced DTO an exact parity contract, or create a separate API-contract slice before changing callers. Do not silently expand into routing, turns, persistence, moderation, listeners, commands, or execution.

## Evidence map and call paths

### 1. HTTP-date `Retry-After` — required parity repair now

**Go path:** `internal/agent/llm/openai/client.go`

- `Client.Complete` builds an operation context at `Complete` lines 132-133, sends a context-bound request at lines 138-148, and on retryable status calls `cfg.Sleep(opCtx, retryAfter(resp.Header.Get("Retry-After"), cfg.RetryDelay, attempt))` at lines 174-178.
- `retryAfter` at lines 312-320 accepts only `strconv.Atoi(...)` integer seconds. Anything else, including an RFC 7231/HTTP-date value, falls back to exponential `RetryDelay`.
- `NewClient` installs `cfg.Now = time.Now` at lines 80-82, but `retryAfter` does not receive or use `Now`; this is a direct indication that deterministic date handling was not completed.
- `retryable` at lines 309-311 is already bounded to 408, 429, and 500-599; that part should not be regressed.

**Saturn basis:** `OpenAiCompatibleClient.complete` in `src/main/java/org/saturn/app/agent/llm/provider/openai/OpenAiCompatibleClient.java` (request/retry loop, lines 61-98 in the inspected source) retries transient statuses; `AgentConfig.retryBackoff` and `OpenAiCompatibleClient.backoff` define provider retry policy. The architecture explicitly requires “Respect `Retry-After` when valid, otherwise use `RetryDelay`” (`.hermes/handoffs/agent-api-llm-architecture.md`, OpenAI behavior section).

**Root cause:** parser was implemented as numeric-only and has no clock injection in its call signature. This is not a test-only omission: a valid server date is behaviorally treated as invalid.

**Minimal repair:** extend the private retry-delay helper to parse nonnegative integer seconds and valid HTTP-date using an injected clock (`Config.Now`), returning a nonnegative delay until that date; retain exponential fallback for invalid/past/overflow values. Keep delay bounded consistently with existing timeout policy. Add a deterministic test using a fixed `Now`, a first 429/503 response with `Retry-After: <HTTP-date>`, an injected `Sleep` recording duration, and a second successful response. Also test malformed date and negative/nonsensical values fall back, and that `Retry-After: 0` remains immediate.

### 2. Cancellation during request/backoff and error identity — implementation support, missing proof; repair tests now

**Go request path:** `Complete` derives `opCtx` from caller context and timeout (lines 132-133), then creates each `http.Request` with `http.NewRequestWithContext(reqCtx, ...)` (lines 138-140). On `HTTP.Do` failure it checks `opCtx.Err()` at lines 149-153 and returns `contextError`; on retry sleep it passes `opCtx` to `cfg.Sleep` at lines 154-157 and 174-177. `sleepContext` selects timer versus `ctx.Done()` at lines 328-336.

**Go error path:** `contextError` at lines 338-344 returns `*llm.LlmError{Code: "cancelled"|"timeout", Err: err}`. `LlmError.Unwrap` at `internal/agent/llm/client.go` lines 164-177 exposes the underlying context error, so `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` are intended to work.

**What is already present:** caller cancellation before a request is covered by `TestCompleteTransportErrorCancellationAndTimeout` in `internal/agent/llm/openai/client_test.go` lines 115-128. Existing code also passes context through the real request and default sleep.

**What is not proven:** current tests use `Sleep: noSleep` for retry cases and do not cancel a context while a real handler/transport is blocked. They therefore do not prove interruption of in-flight I/O. They also do not run a real timed backoff, cancel it, and assert error identity. The QA test that invokes Saturn's private `backoff` equivalent is not a substitute for the Go client operation path.

**Minimal repair:** add two focused tests in `internal/agent/llm/openai`:

- **Request cancellation:** use a custom `RoundTripper` that blocks until `r.Context().Done()`, start `Complete` in a goroutine, cancel the caller context, assert prompt completion, one attempt, `errors.Is(err, context.Canceled)`, and `errors.As(err, *llm.LlmError)` with `Code == "cancelled"`.
- **Backoff cancellation:** use a first 503 response, default/context-aware sleep or an injected sleep that blocks on its context, cancel after the first response, assert no second request, prompt return, `errors.Is(err, context.Canceled)`, and the same LlmError code. Repeat with a deadline if timeout identity is part of the acceptance contract and assert `errors.Is(..., context.DeadlineExceeded)`.

Do not change the intended wrapper identity unless these tests demonstrate a real loss; the current `Unwrap` design is compatible with identity assertions.

### 3. Provider diagnostics preservation — required repair now

**Go path:** `responseEnvelope` in `internal/agent/llm/openai/client.go` lines 240-255 decodes `choices` and `usage` only. `decodeResponse` maps usage and calls `llm.NewLlmResponseWithMetadata(content, calls, reason, e.Usage, nil)` at lines 257-292, hard-coding successful diagnostics to `nil`. The provider-neutral storage exists in `LlmResponse.providerDiagnostics` and accessor `ProviderDiagnostics()` at `internal/agent/llm/client.go` lines 111-149, but it is never populated by OpenAI success decoding.

**Already satisfied:** non-success HTTP diagnostics are preserved in `httpError` at `client.go` lines 294-306: provider `error.code`, `error.message`, and a bounded body snippet are carried on `LlmError`, while `Error()` avoids printing the body snippet/token. QA's token-safety test covers this.

**Root cause:** response metadata model was added for usage, but decoder schema/forwarding was not extended for provider fields.

**Minimal repair:** define the supported provider-diagnostics representation explicitly (prefer a deep-cloned JSON-compatible `map[string]any` containing provider metadata not otherwise modeled, or a documented envelope field) and decode/forward it from successful responses. Preserve unknown diagnostics without putting secrets in the primary error string. Add an `httptest` response containing diagnostic fields (for example provider request ID, system fingerprint, and an extra nested provider object), then assert `ProviderDiagnostics()` retains values and that mutating the returned map cannot mutate the response. If the intended contract is only HTTP error diagnostics, amend the architecture and remove “successful provider diagnostics” from acceptance; under the current architecture and QA requirement, it is in-scope.

### 4. Complete deep JSON-compatible defensive copying — required repair now

**Go path:** `internal/agent/llm/client.go`:

- `NewLlmRequest` clones `tools`, `responseFormat`, and `projection` through `cloneAnySlice`/`cloneValue` (lines 102-109).
- `NewLlmToolCall` clones only `map[string]any` (lines 70-86); `Arguments()` returns `cloneMap` (line 90).
- `cloneValue` at lines 237-246 recurses only over dynamic `map[string]any` and `[]any`; all other values are returned as-is.
- OpenAI has a duplicate clone implementation in `internal/agent/llm/openai/client.go` lines 346-368 for `Config.Options`, with the same limitation.

**Current test limitation:** `internal/agent/llm/client_test.go` lines 8-19 checks nested `map[string]any`, but not `map[string]string`, `[]string`, `[]map[string]string`, typed structs/arrays where JSON-compatible fields are exposed through aliases, or `json.RawMessage`/`[]byte` aliases. The OpenAI path's `Options` clone is also not independently tested.

**Root cause:** type-switch cloning assumes the representation produced by `encoding/json` rather than the broader set of Go values accepted by `any` and used by callers. Returning typed maps/slices unchanged retains mutable aliases.

**Minimal repair:** centralize a single recursive JSON-compatible copier and use it for LLM DTO fields and OpenAI options. At minimum recurse through maps with string keys, slices/arrays, pointers/interfaces, and `json.RawMessage`/byte slices as appropriate; reject or normalize unsupported values consistently rather than silently retaining mutable references. Preserve nil versus present values. Add table/property-style tests covering nested typed maps/slices, mixed `map[string]any` values containing typed children, response-format/tools/options, constructor input mutation, accessor-return mutation, and race-enabled alias checks. The acceptance criterion is that no mutable JSON-compatible input retained by a DTO or returned accessor can mutate internal state.

### 5. Strict config scalar parsing and default API-key semantics — required repair now

**Go path:** `internal/config/value_reader.go`:

- `lookup` implements runtime > environment > file lookup at lines 16-26.
- `Bool` at lines 34-43 returns the fallback when `strconv.ParseBool` fails.
- `Int` at lines 45-54 returns the fallback when `strconv.Atoi` fails.
- Thus malformed values are silently converted into unrelated defaults.

`internal/config/agent_config.go`:

- `Resolve` obtains all scalars with `r.Bool`/`r.Int` at lines 63-73, so the silent fallback is on the active configuration path.
- It leaves `APIKeyEnv` empty when absent (line 68), and only looks up a secret if `v.APIKeyEnv != ""` (lines 96-99). It therefore does not implement Saturn's default variable-name semantics.
- Current test `internal/config/agent_config_test.go` lines 19-32 checks numeric defaults and invalid manually-constructed values, but has no malformed scalar source test and no absent-`APIKeyEnv`/`SATURN_AGENT_API_KEY` test.

**Saturn basis:** `AgentConfigLoader.load` defaults `apiKeyEnv` to `SATURN_AGENT_API_KEY` at `src/main/java/org/saturn/app/agent/config/AgentConfigLoader.java` lines 39-43. `AgentConfigValueReader.readLong` at lines 69-84 throws `IllegalArgumentException("<ENV> must be an integer")` for malformed environment values; `readBoolean` at lines 109-126 throws `"<ENV> must be true or false"`. `toInt` at lines 128-141 rejects overflow. `AgentConfig` validates positive limits/durations and nonnegative retries/backoff at lines 78-105.

**Minimal repair:** add error-returning strict scalar APIs (or a strict `Resolve` path) that distinguish absent/blank from malformed and name the source key; propagate errors from `Resolve` rather than falling back. Preserve runtime > environment > file precedence. Define `DefaultAPIKeyEnv = "SATURN_AGENT_API_KEY"`; when no configured name is supplied, resolve that variable name and retrieve its secret, while keeping the variable name out of serialized secrets. Add tests for malformed runtime/environment/file bool and int, integer overflow, precedence, absent APIKeyEnv with populated default env secret, explicit APIKeyEnv override, and blank-value semantics. Avoid changing unrelated config fields or inventing Saturn limits not evidenced by the source.

### 6. Exact API DTO shape/serialization parity — contract mismatch, architecture decision required

**Go current shape:** `internal/agent/api/api.go` defines:

- `InvocationMode` and uppercase constants at lines 9-19 (this claim is satisfied for enum spelling/validation).
- `Invocation` with only `mode`, `identityKey`, `text`, `room`, `createdOn` at lines 22-27.
- `Context` with `invocation`, `messages []string`, and `memory []Memory` at lines 29-32.
- `Memory` with identity/content/timestamps at lines 34-39.
- `Identity` with `key` and `displayName` at lines 41-43.
- `Result` with `text`, `noReply`, and optional `errorCode` at lines 45-49.
- `ValidateInvocation` only validates mode, nonblank room, and nonzero timestamp at lines 51-61.

**Saturn exact shape:**

- `AgentInvocation` record in `src/main/java/org/saturn/app/agent/api/AgentInvocation.java` lines 7-24 has `requestId`, non-null `context`, nonblank `prompt`, non-null `mode`, `currentMessageText`, and `commandOriginated`, with constructors/defaults at lines 26-53.
- `AgentContext` record in `AgentContext.java` lines 8-16 has `room`, `nick`, `trip`, `hash`, `whisper`, `roomUsers`, `capabilities`, and `moderationTarget`; its constructor requires identity/collections and defensively copies collections at lines 33-40. `memoryKey()` at lines 94-108 is behavior, not equivalent to the Go `Memory` list.
- `AgentResult` record in `AgentResult.java` lines 6-12 has `correlationId`, `content`, `shouldReply`, requires nonblank correlation ID and non-null content; `reply`/`silent` factories are lines 31-43.
- `AgentUserIdentity` record in `AgentUserIdentity.java` lines 9-14 is a single validated `value`, with `from(AgentContext|ChatMessage|User)` and trip/hash/nick precedence at lines 22-64.
- `AgentInvocationMode` enum in `AgentInvocationMode.java` lines 4-28 also carries `requiresReply`, which is absent from Go's mode type.

**Root cause:** the architecture inventory and implementation accepted a reduced Go DTO as if it could satisfy the exact Saturn API contract. The reduced types are not a field rename or serialization-tag issue; they model different concepts and omit validation/default/identity semantics. Current Go tests (`internal/agent/api/api_test.go`) prove only the reduced contract: uppercase mode, reduced validation, and empty `Result` serialization. They cannot establish Saturn parity.

**Required decision:** do not make an opportunistic incompatible rewrite in this diagnostic. Amend `agent-api-llm-architecture.md` to state that the current API section is a reduced internal compatibility DTO, not exact Saturn parity, **or** create a separately reviewed API-contract slice that maps all Saturn records, constructors, nullable/omitted JSON semantics, identity derivation, and call-site migration. A separate slice is safer because exact API changes can affect routing/assembly/runtime consumers, which are explicitly excluded here.

**If kept in this slice:** acceptance needs a dedicated `internal/agent/api` contract test suite with exact JSON fixtures and round trips for every Saturn field, null/omitted distinctions, enum validation, constructor defaults, required-field failures, defensive collection copies, `AgentUserIdentity` precedence/normalization, and `AgentResult` reply/silent semantics. The architecture must list the chosen Go names and wire names explicitly.

## Claims already satisfied (do not duplicate as repairs)

- Uppercase `DIRECT`, `MENTION`, `AMBIENT`, `MODERATION` serialization and unknown-mode rejection: `api.go` lines 9-19; `api_test.go` lines 21-24.
- Current reduced invocation validation for mode/room/timestamp: `api.go` lines 51-61; `api_test.go` lines 9-19. This is not Saturn's full invocation validation.
- Endpoint trailing-slash normalization and `/v1/chat/completions` path: `agent_config.go` lines 74-77 and OpenAI `endpointURL` lines 308-308; existing OpenAI tests.
- Bearer header, JSON content type, non-streaming request, model/token/options/tool serialization, null assistant content, tool-call mapping, malformed successful-response detection, and ordinary permanent-4xx non-retry are covered by `openai/client.go` and its tests.
- Retryable HTTP status is bounded to 408/429/500-599 in Go (`retryable`, lines 309-311); the old arbitrary `>=500` defect is fixed.
- HTTP error diagnostics are retained in typed `LlmError` (`httpError`, lines 294-306), and the primary error string does not include the provider snippet/token. This does not satisfy successful-response diagnostics.
- Context is attached to requests and default sleep is context-aware; the remaining issue is direct end-to-end coverage and proof of `errors.Is` identity, not an established need to remove the wrapper.
- Existing focused commands in the QA handoff all passed; this diagnostic does not reinterpret green tests as parity proof.

## Explicit exclusions

Do not repair or expand routing, invocation construction from chat events, tool selection/execution, turns, persistence, moderation policy, listeners, commands, or runtime orchestration. Saturn references to `AgentContext.memoryKey`, routing factories, persistence context providers, or tool execution are used only to establish DTO shape/ownership and must not trigger implementation in this slice. Do not modify the Saturn checkout. Preserve unrelated dirty/untracked files in `zenbot`.

## Priority order

1. Add regression tests for request/backoff cancellation and `errors.Is` identity (low-risk proof of existing intended behavior).
2. Implement and test HTTP-date `Retry-After` with injected clock; this is a direct protocol defect.
3. Implement strict config parse errors and `SATURN_AGENT_API_KEY` default; this prevents silent misconfiguration and secret lookup mismatch.
4. Preserve successful provider diagnostics and test defensive accessor isolation.
5. Replace/centralize deep JSON-compatible copying and add typed-alias tests, including OpenAI options.
6. Resolve the API contract decision: architecture amendment versus separate API-contract slice; only then implement exact DTOs and fixtures.
7. Re-run focused tests, `go test -race ./internal/agent/... ./internal/config/...`, then full project gates without touching excluded areas.

## Acceptance criteria

- A valid HTTP-date `Retry-After` controls the recorded retry sleep relative to an injected clock; invalid dates use documented fallback; no retry policy regressions.
- Cancellation while transport is blocked and while backoff is active returns promptly, performs no later attempt, and satisfies `errors.Is(err, context.Canceled)` (or deadline identity for deadline cancellation), while retaining typed `LlmError` code.
- Successful provider diagnostic JSON is retained and independently defensively copied; HTTP error diagnostics remain bounded and token-safe.
- Mutating every supported mutable JSON-compatible input after construction, or every accessor result, cannot mutate DTO/client state; tests cover typed nested maps/slices and raw JSON bytes.
- Malformed scalar sources return actionable field/source errors; valid precedence remains runtime > environment > file > defaults; absent `APIKeyEnv` uses `SATURN_AGENT_API_KEY` and explicit names still override it.
- The architecture explicitly labels the API DTOs as exact parity or reduced internal DTOs. If exact parity is selected, fixture tests establish field names, defaults, null/omitted behavior, validation, and defensive collection semantics against the Saturn records.
- No application code outside this bounded slice or any Saturn file is modified; unrelated worktree changes remain intact.

## Files implicated

**Go:**
- `internal/agent/api/api.go`, `internal/agent/api/api_test.go`
- `internal/agent/llm/client.go`, `internal/agent/llm/client_test.go`
- `internal/agent/llm/openai/client.go`, `internal/agent/llm/openai/client_test.go`, `internal/agent/llm/openai/qa_test.go`
- `internal/config/value_reader.go`, `internal/config/agent_config.go`, `internal/config/agent_config_test.go`

**Architecture/handoff:**
- `.hermes/handoffs/agent-api-llm-architecture.md` (API scope/contract wording needs amendment or an explicit follow-on slice)
- `.hermes/handoffs/agent-api-llm-implementation.md` (overstates completeness at lines 38-39)
- `.hermes/handoffs/agent-api-llm-qa.md` (FAIL findings are materially confirmed)

**Saturn evidence:**
- `src/main/java/org/saturn/app/agent/api/AgentInvocation.java`
- `AgentContext.java`, `AgentResult.java`, `AgentUserIdentity.java`, `AgentInvocationMode.java`
- `src/main/java/org/saturn/app/agent/config/AgentConfig.java`
- `AgentConfigLoader.java`, `AgentConfigValueReader.java`
- `src/main/java/org/saturn/app/agent/llm/LlmMessage.java`, `LlmRequest.java`, `LlmResponse.java`, `LlmToolCall.java`
- `src/main/java/org/saturn/app/agent/llm/provider/openai/OpenAiCompatibleClient.java`
- Focused tests: `src/test/java/org/saturn/app/agent/AgentInvocationTest.java`, `AgentContextTest.java`, `src/test/java/org/saturn/app/agent/config/AgentConfigLoaderTest.java`, `AgentConfigValueReaderTest.java`, `src/test/java/org/saturn/app/agent/llm/provider/openai/OpenAiCompatibleClientTest.java`, `OpenAiCompatibleClientConstructionTest.java`
