# Text, Identity, and Weather Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans for inline execution. Steps use checkbox syntax for tracking. The user explicitly authorizes independent decisions and implementation without approval pauses.

**Goal:** Repair text fidelity, nickname target equivalence, and weather behavior throughout the existing command pipeline.

**Architecture:** Plain application text is serialized once at the chat/protocol boundary. Normalize only raw semantic nickname operands, never trusted source identities or opaque values. Preserve current weather safety fixes and Saturn's visible formatting.

**Tech Stack:** Go, encoding/json, Gorilla WebSocket, H2-backed repositories, HTTP provider fixtures.

**Spec:** `docs/superpowers/specs/2026-09-11-text-identity-weather-design.md`

## Global constraints

- Preserve synchronous action execution, cancellation/receipt handling, authorization and LLM planning behavior.
- No live database rewrites, reference-repository edits, deployment, or moderation requests.
- Distinguish LF from literal backslash+n at every boundary.
- Preserve canonical source names and opaque trip/hash/channel values.
- Leave this goal active until the full requested audit is verified.

## 1. Text producers and chat serialization

Files: `internal/core/engine_impl.go`, `internal/core/chat_text_contract_test.go`, `internal/core/engine_impl_test.go`, `internal/core/subscriptions_test.go`; formatting sites enumerated in the design's audit inventory.

Interfaces stay unchanged: `SendChatMessage(author, message string, whisper bool) (string, error)`, `SendWhisperMessage(author, payload string) (string, error)`, `SendAddressedMessage(author, payload string, whisper bool) (string, error)`. Their payloads and receipts become strictly plain text. Transport methods still accept JSON frames.

- [x] Add a failing table-driven regression comparing decoded JSON and receipt against exact plain text for all six send variants. Include LF, literal backslash+n, Windows paths, CR/CRLF, quotes, tabs, NUL, and Unicode.
- [x] Run `go test ./internal/core -run '^TestChatSendersPreservePlainTextAtJSONBoundary$' -count=1`: five addressed variants fail due to mutation; unaddressed public passes.
- [x] Remove sender unescaping and redundant whitespace replacements; keep safe JSON serialization. A producer uses `"first\nsecond"`, never `"first\\nsecond"`, for intended line breaks.
- [x] Replace legacy encoded formatting in commands, services, join/mail listeners, snapshot lists, and user-data display. Remove extra Java/JSON escaping in SQL, history, support relay, and search. Update old tests that asserted encoded intermediate strings, retaining exact decoded-output checks.
- [x] Add producer regressions for reported SeenRecently output, multiline YouTube metadata, arbitrary history/search/SQL text. Run the matching service/command/listener/core tests before and after each fix.
- [x] Test real engine output over a local WebSocket server that decodes each JSON frame and accepts a following frame, proving multiline/control text does not corrupt framing.

## 2. Stored text fidelity

Files: `internal/service/mail_notes.go`, `internal/service/mail_delivery.go`, relevant H2 migrations/repository definitions, and mail/note integration tests. Interfaces: `QueueResolved(...) (string,error)`, `Pending(...) ([]model.Mail,error)`, `NoteService.List(...) ([]string,error)`.

- [x] Trace persisted mail provenance and schema migrations before choosing compatibility handling. Never auto-detect text encoding by the presence of backslashes.
- [x] Add regressions that persist then retrieve both `"line\nnext"` and `"literal \\n"` with quotes/backslashes/Unicode through the real repository.
- [x] Remove redundant note-list escaping. Persist new mail as plain text under an explicit versioned contract if legacy decoding is needed; preserve receipt/delivery state transitions.
- [x] Run `go test ./internal/service ./internal/command ./internal/repository/h2 ./internal/listener/message -count=1` and inspect failures, including existing pending-mail compatibility tests.

## 3. Nickname surface and propagation

Files: `internal/util/identity.go`, raw target consumers in `internal/command`, `internal/service/mail_notes.go`, and `internal/agent/tool/user_message_history.go`; existing target-boundary, identity, mail, and gateway tests.

- [x] Enumerate command definitions/aliases and classify operand types, including mixed name/trip/hash selectors and direct service callers.
- [x] Add regressions for missed raw nickname paths (registration is confirmed), using both `merc` and `@merc`. Include propagation to persistence and actual outbound nick fields.
- [x] Replace duplicate manual normalization with `util.NormalizeNickTarget(&raw)` at the owner of raw ingress. Keep resolved helper calls literal. Preserve opaque-selector precedence with separate raw and normalized values where the API accepts multiple identity types.
- [x] Run focused nickname, moderation boundary, identity, mail and agent-tool tests. Check `@@merc` resolves a literal source nickname `@merc` once, never twice.

## 4. Weather command and reference comparison

Files: `internal/service/weather_time.go`, `internal/command/services.go`, `internal/service/weather_time_audit_test.go`, `internal/command/utility_audit_regression_test.go`, plus a new real-engine command integration test.

- [x] Read current Saturn command/service/DTO/alignment and Zenbot command/service/transport; note dirty Saturn service provenance.
- [x] Probe the exact default geocoder and forecast requests for Chisinau: both return HTTP 200 with valid coordinates and forecast payloads.
- [x] Compare committed Saturn reference and WMO icon mapping, full request parsing, missing/null metrics, time axes, units and formatting; retain Zenbot correctness improvements.
- [x] Add registered-command fixture tests for `*weather chisinau`, `*w chisinau`, and `*today chisinau`, asserting query values, 17 aligned rows, units/icons/timestamps, and decoded final frame.
- [x] Run an opt-in read-only live-provider command test, with outbound room delivery captured locally; inspect available runtime/log evidence without claiming an unproven historical failure cause.
- [x] Fix only proven deviations and rerun malformed/null/cancellation/encoding/service and command tests.

## 5. Completion audit

- [x] Re-run whole-codebase searches for literal/actual newlines, Replace/Quote/Marshal/Unquote calls, nickname stripping and all nickname consumers. Classify residual cases in the final audit report.
- [x] Run `go test ./...`, relevant `go test -race` suites, `go vet ./...`, and `git diff --check`.
- [x] Review full changes against the original objective: no masked literal escapes, data loss, unsafe framing, missed aliases, normalization of opaque identities, or unsupported weather success claims.
- [x] Record root causes, fixes, verification and any deployment limits in `docs/audits/2026-09-11-text-identity-weather.md`; mark goal complete only when every requirement is supported by current evidence.
