# `grant` / `access` authenticated-principal ingress unblocker

## Decision

**BLOCKED: the smallest safe prerequisite cannot be implemented from the transport/session data currently retained by Zenbot.** Do not expose `grant` or `access` yet.

The managed WebSocket transport yields only undifferentiated inbound `[]byte` frames. The engine dispatches a raw JSON string, and `cmd:"session"` is explicitly ignored. The chat path receives only fields decoded from the incoming payload; it has no authenticated per-user subject, authenticated session binding, signature/claim verification, or server-issued association between a chat event and an active user. `onlineAdd` data and `chat` data may include `userid` in observed payload examples, but that value is still only a server-frame payload field in the current process: it is not retained on `ChatMessage`, bound to a transport-authenticated session, or verified against an authenticated issuer.

Therefore neither nickname, trip, hash, nor `userid` may be used as a substitute principal. Matching an active user by nickname is already unsafe, and comparing more payload fields would remain unauthenticated inference. The only safe next core step is to establish an upstream **authenticated event-principal envelope** from a transport/provider that can attest the subject. If the upstream protocol cannot supply such an attestation, that is the precise product/protocol blocker; local Go changes alone cannot manufacture one.

## Observed module map

| Boundary | Evidence | Effect on an authenticated-principal design |
|---|---|---|
| WebSocket transport | `[OBSERVED]` `internal/transport/connection.go:Connection.readLoop` calls `ReadMessage()` and sends a copied `[]byte` to `Messages()`. `Connection.Start` calls `DialContext(..., nil)` and discards the HTTP response. | No retained server/session identity, headers, TLS peer claim, or per-frame provenance reaches consumers. A `Messages() <-chan []byte` frame has no metadata channel. |
| Engine transport loop | `[OBSERVED]` `internal/core/engine_impl.go:EngineImpl.StartContext` reads `e.Transport.Messages()` and calls `DispatchMessageContext(ctx, string(msg))`. | Any future envelope must cross this interface; a raw string dispatch is insufficient to carry a principal. |
| Event router | `[OBSERVED]` `EngineImpl.DispatchMessageContext` routes `"onlineAdd"` and `"chat"` separately and has an empty `case "session":`. | The current `session` command contributes no state. The router has no correlation state keyed by server-attested subject. |
| Payload models | `[OBSERVED]` `internal/model/chat_message.go:ChatMessage` maps nick/trip/hash/text and has no `UserId` or issuer/session field. `internal/model/user.go:User` maps `UserId` from `userId`; comments/examples show `userid` for online events and a chat example. | A decoded identity-shaped field is not an authenticated principal. Adding it to `ChatMessage` without transport/provider attestation would not close the security boundary. |
| Active-user lifecycle | `[OBSERVED]` `internal/listener/user_joined_listener.go:Notify` decodes `onlineAdd` then calls `Engine.AddActiveUser`. `EngineImpl.AddActiveUser` deduplicates by `model.IdentityKey(trip, hash, name)`; `UserLeftListener.Notify` locates removal by name. | This cache is derived from provider payloads, not a session-authenticated caller registry. It cannot authorize privileged writes. |
| Current principal resolution | `[OBSERVED]` `internal/listener/message/handlers.go:ResolveUserMetadata.Handle` scans `GetActiveUsers()` and picks the first `strings.EqualFold(u.Name, c.Message.Name)` match, then copies `u.Hash` into the message. `EngineImpl.GetActiveUserByName` uses the same first-match nickname scan. | Duplicate names are ambiguous and map traversal is nondeterministic. The resolver is an enrichment path, not an authorization boundary. |
| Command dispatch | `[OBSERVED]` `message.DispatchUserCommand.Handle` authorizes `c.Author` before executing the command. `[TEST-BACKED]` `internal/listener/message/dispatch_authorization_test.go` proves only that a supplied `Context.Author` is checked before execution. | Dispatch can use an authoritative principal once supplied, but its present author is not authoritative. |
| Access mutation | `[OBSERVED]` `internal/command/identity_commands.go:accessCommand.Execute` invokes `b.Security.Authorization.GrantTrip`. `[OBSERVED]` Saturn `AccessUserCommandImpl.java` has aliases `grant`/`access`; inherited `UserCommandBaseImpl.getAuthorizedRole()` returns `ADMIN`; `AuthorizationServiceImpl.grant` writes a role. | The write path and command catalog exist, but must remain unavailable until caller attribution is proven. |
| Registration/composition | `[OBSERVED]` `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` includes `access` when `Users.GroupB != nil && Security != nil`; `internal/factory/engine_factory.go:NewEngineWithOptions` constructs the raw transport and listeners. | The registrar currently checks neither a principal ingress capability nor a non-nil authorization writer. Any eventual exposure needs both. |
| Saturn comparison | `[OBSERVED]` Saturn `UserCommandBaseImpl.execute` calls `authorizationService.isUserAuthorized(command, chatMessage)`, whose `AuthorizationServiceImpl` authorizes using `chatMessage.getTrip()`. | Saturn source confirms functional role semantics, but is not evidence of an authenticated caller boundary; target must not reproduce this weakness for a persistent privileged mutation. |

## Security boundary and precise missing-data blocker

### Required invariant

For every privileged command, the user passed to `Engine.IsUserAuthorized` must originate from a provider-authenticated, unforgeable subject associated with **that exact inbound event**. The authorization lookup may use attributes of the resulting authoritative user record, but it must never select that record by `nick`, `trip`, `hash`, or a payload-only `userid`.

```text
[provider-authenticated subject + event binding]
                  |
                  v
Transport event envelope { payload, principal }
                  |
                  v
Engine routing preserves envelope unchanged
                  |
                  v
Message context.Author = principal's authoritative user snapshot
                  |
                  v
DispatchUserCommand -> IsUserAuthorized(author, ADMIN)
                  |
                  v
accessCommand -> AuthorizationRepository.GrantTrip
```

### What is missing

1. **Issuer-authenticated subject:** no upstream interface or protocol message currently identifies the human sender through credentials, a verified server session, a signed claim, or a trust boundary stronger than a decoded frame.
2. **Per-event binding:** `EngineTransport.Messages()` transmits bytes only. There is no value proving that a particular chat frame was emitted by a particular authenticated subject.
3. **Authoritative subject-to-user mapping:** no registry binds an authenticated opaque subject to an authoritative `model.User` lifecycle record. `TemporarySessionRegistry` under `internal/listener/snapshot` tracks internal snapshot cancellation IDs only; it is unrelated to chat callers and cannot supply this mapping.
4. **Fail-closed privileged admission:** `ResolveUserMetadata` continues with no author, but other handlers run before command dispatch. There is no explicit classification or audit outcome for a privileged command with absent/invalid principal.

### Smallest acceptable ingress contract (only after upstream support exists)

`[RECOMMENDED]` Do not add a broad identity service. Add one opaque envelope at the transport boundary, owned by the provider adapter:

```go
// The Subject is provider-issued and opaque to command/listener code.
// It MUST NOT be nick, trip, hash, or a raw unverified JSON field.
type InboundEvent struct {
    Payload   []byte
    Principal AuthenticatedPrincipal
}

type AuthenticatedPrincipal struct {
    Subject string // non-empty, stable for the provider's authenticated session
}

type EngineTransport interface {
    // Replace or parallel the raw frame stream only after the provider can
    // attest Principal for each event.
    Events() <-chan InboundEvent
}
```

The transport/provider adapter must define and test what authenticates `Subject` (for example, an authenticated upstream API/session whose event delivery binds an immutable account ID to each event). The engine then passes an `InboundEvent`, not a string, through routing. A dedicated internal resolver can map the opaque subject to an authoritative user snapshot held in a subject-keyed registry. It must reject missing, unknown, expired, and conflicting bindings. The command layer receives only that snapshot; it must not access the registry or payload identity fields.

If upstream can only send anonymous/chat payloads over this socket, the appropriate outcome is to keep `grant`/`access` disabled and use a separately authenticated administration channel. Do not retrofit payload matching as a workaround.

## TDD sequence for the unblocker

No command implementation or `access` registration belongs in the first change. Run each focused RED test before its corresponding implementation.

1. **Transport/provider contract — RED:** a provider adapter cannot emit a privileged-eligible event when the upstream connection/session is absent, unauthenticated, or has no subject. **GREEN:** emit `InboundEvent` only with a nonblank authenticated opaque subject. Prove raw frames cannot be marked authenticated by callers.
2. **Exact event binding — RED:** two events with equal nick/trip/hash but different authenticated subjects preserve their separate subjects through the transport and `EngineImpl.StartContext` routing. **GREEN:** convert the engine boundary from `[]byte`/string to the envelope without deriving any value from payload identity fields.
3. **Subject registry — RED:** an event subject with no active authoritative binding, an expired binding, or a binding conflict yields no principal. **GREEN:** use a concurrency-safe subject-keyed authoritative snapshot registry; populate/revoke it only from authenticated provider lifecycle events, not `onlineAdd` JSON alone.
4. **Duplicate-name regression — RED:** two connected users named `alice`, one ADMIN and one regular, cannot cause the regular user's event to authorize as the ADMIN. **GREEN:** message context is populated strictly from event subject resolution; remove privileged dispatch's dependence on `ResolveUserMetadata` nickname scanning.
5. **Fail-closed admission — RED:** a syntactically valid `!grant target ADMIN` with a missing/invalid principal has no authorization lookup, no reply that reveals a selected user, and no repository write. **GREEN:** give privileged dispatch an explicit fail-closed result. Preserve non-privileged behavior only where separately reviewed.
6. **Authorized vertical — RED:** a provider-authenticated ADMIN principal maps to the expected authoritative user; `!grant target ADMIN` invokes only the existing narrow authorization repository write and replies to the resolved snapshot using the incoming whisper state. **GREEN:** compose the access adapter only when authenticated ingress and a non-nil writer are both available.
7. **Registration negative cases — RED:** engines lacking the authenticated ingress capability, subject registry, or `Security.Authorization` writer expose neither `grant` nor `access`, even if `Users.GroupB` and `Security` exist. **GREEN:** make the registrar capability-based and retain existing no-access baseline coverage.
8. **Non-bypass regression — RED:** direct construction of a message context using a nickname-derived user cannot reach the `access` mutation in production composition. **GREEN:** keep the authoritative-principal resolver internal to ingress; tests may use explicit trusted test fixtures, never payload helpers.

Focused validation after each green slice should include the responsible package, then at minimum:

```sh
go test ./internal/transport ./internal/core ./internal/listener ./internal/listener/message ./internal/command ./internal/service ./internal/repository/h2
git diff --check
```

## Risks and non-goals

- `[RISK]` Treating `userid` as safe merely because the server sends it changes the threat model without proving authenticity. It needs documented upstream authentication and event association before use.
- `[RISK]` The current transport's `DialContext` discards `*http.Response`; even a connection-level authenticated response would not provide per-chat-event identity and must not be overclaimed.
- `[RISK]` A connection-level identity for the bot is not a human caller identity. One room WebSocket carries messages from many users.
- `[RISK]` Changing all existing user-facing commands to fail closed may be a separate compatibility migration. The first scope should protect the privileged persistent mutation path only, while documenting pre-existing authorization debt.
- `[RISK]` `accessCommand` dereferences `b.Security.Authorization` after checking only `b.Security`; registration must require a non-nil writer before eventual exposure.
- `[NO-SCOPE]` No `grant`/`access` handler changes, no registration changes, no payload-field matching, no identity inferred from nick/trip/hash, no agent tool exposure, no protected-target/anti-escalation policy, no raw SQL interface, and no repository/source/test implementation in this architecture handoff.

## Developer tier

**Core/security transport owner (senior/staff), with upstream protocol ownership.** This work alters the trust boundary between an external provider and persistent authorization. It requires an explicit, testable provider authentication guarantee; it is not suitable for a command-only parity task or an agent-tool implementation.
