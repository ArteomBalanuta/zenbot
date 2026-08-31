# Saturn Agent API Contract QA

## Verdict

**PASS** for the scoped implementation, after repairing three independently identified contract defects. The final target worktree passes all requested focused and repository-wide verification gates. Saturn was read-only and was not modified.

## Scope and comparison basis

Compared the final Go implementation with the exact Saturn API sources and focused tests:

- Saturn API: `src/main/java/org/saturn/app/agent/api/AgentInvocation.java`, `AgentContext.java`, `AgentInvocationMode.java`, `AgentResult.java`, `AgentUserIdentity.java`, `AgentCapability.java`.
- Saturn tests: `AgentInvocationTest`, `AgentContextTest`, `AgentParticipationConfigTest`, and `AgentQuietRegistryTest`.
- Target implementation: `internal/agent/api/api.go`, `result.go`, `identity.go`, `api_test.go`, and explicit compatibility boundary `internal/agent/runtime/adapters.go`.

## QA repairs made

1. Added the generated-ID `(context, prompt, mode)` invocation constructor behavior and made an explicitly supplied null mode invalid rather than silently defaulting it.
2. Preserved non-nil empty room-user/capability collections and defensive-copy semantics; capability input is deduplicated to match Java `Set.copyOf` semantics.
3. Preserved explicit JSON empty strings for nullable `trip` and `hash` instead of converting them to null during decode.
4. Matched Java `String.strip()`/`isBlank()` behavior more closely for identity and invocation validation: internal whitespace is preserved, Java whitespace boundaries are stripped, and locale-independent lowercasing is used.
5. Removed an extra Go-only invocation room-blank rejection. Saturn requires a non-null context, but does not reject an empty room string.

Intentional files changed by this QA pass:

- `internal/agent/api/api.go`
- `internal/agent/api/result.go`
- `internal/agent/api/identity.go`
- `internal/agent/api/api_test.go`
- `internal/agent/runtime/adapters.go` (gofmt only during QA; no behavioral change)
- `.hermes/handoffs/agent-api-contract-qa.md` (this report)

The prior implementation files above were already scoped to the Agent API contract. No unrelated target files were edited by QA. The existing unrelated dirty/untracked target worktree was preserved. Saturn remains untouched.

## Contract coverage checked

- Invocation fields, all Java constructor shapes represented by Go constructors, generated UUID path, `DIRECT` defaults, null current-message handling, command-origin defaults/flag, all four modes, blank request/prompt rejection, null context rejection at JSON boundary, and unknown/null mode rejection.
- Context nullable `trip`, `hash`, and `moderationTarget`; required room/nick/collections at JSON boundary; empty collections; defensive input and accessor copies; capability membership and Set-like deduplication.
- `MemoryKey` public/whisper formatting, trip > hash > nick precedence, no normalization of key components, and Java UTF-16 code-unit room length (including supplementary Unicode characters via `utf16.Encode`).
- Result three-component shape only; reply default, explicit reply flag, silent empty-content/no-reply factory, blank correlation rejection, empty content acceptance, required JSON field/null rejection, and no serialized `errorCode`.
- Identity direct-value validation, context nil rejection, trip > hash > nick precedence, blank fallback behavior, Java-strip-equivalent boundary handling, internal whitespace preservation, and locale-independent lowercasing.
- Explicit lower-camel JSON names, required-field omitted/null rejection, nullable-field preservation, empty arrays/strings, round-trip handling, and unknown invocation mode rejection.
- Runtime adapters are explicit rather than JSON/field-guessing adapters. API-to-runtime nullable scalar conversion is documented as lossy; runtime results carrying an error code are rejected because Saturn `AgentResult` has no error-code component. Runtime timestamps are not exported.

## Actual verification results

All commands below were run in `/Users/ab/workspace/go-projects/zenbot` after the final repairs and exited 0:

```text
gofmt -w internal/agent/api/api.go internal/agent/api/result.go internal/agent/api/identity.go internal/agent/api/api_test.go internal/agent/runtime/adapters.go

go test -count=1 ./internal/agent/api ./internal/agent/runtime ./internal/agent/assemble
ok   zenbot/internal/agent/api
ok   zenbot/internal/agent/runtime
ok   zenbot/internal/agent/assemble

go test -race -count=1 ./internal/agent/api ./internal/agent/runtime ./internal/agent/assemble
ok   zenbot/internal/agent/api
ok   zenbot/internal/agent/runtime
ok   zenbot/internal/agent/assemble

go test -count=1 ./...
PASS: all listed target packages passed, including internal/agent/api, runtime, assemble, command, config, core, factory, listeners, model, repository/h2, service, and transport.

go test -race -count=1 ./...
PASS: all listed target packages passed with the race detector.

go vet ./...
PASS (no output)

go build ./...
PASS (no output)

git diff --check
PASS (no output)
```

## Limitations

Saturn has no focused JSON golden tests, JSON annotations, or custom serializer for these four records. Therefore the lower-camel/null/omitted JSON behavior is an explicit, tested Go boundary policy aligned with Saturn component names and object-model nullability; it is not a claim that Saturn wire bytes were independently observed. Go cannot overload constructors like Java, so variadic constructors provide the equivalent call shapes and return errors for invalid inputs.

The Go capability type remains a string-backed API type with the four Saturn capability constants; membership and Set-like collection semantics are enforced. This preserves an extensible Go boundary while known Saturn capability behavior is covered; callers requiring Java compile-time enum closure are outside Go's type-system equivalence.

## Deferred/excluded scope

Deferred or explicitly excluded: routing/classification, tool execution, turns/freshness, persistence/schema/memory stores, moderation behavior, listeners/lifecycle, commands, provider/config behavior, migration of all runtime callers, changes to `internal/model.IdentityKey`, and any Saturn source changes. Existing runtime/assembler production signatures remain on their private seam; the explicit adapters are the staged migration boundary.
