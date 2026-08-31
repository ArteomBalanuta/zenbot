# Rapid agent final-response normalization QA

## PASS

Independent QA passed the deterministic final-response normalization vertical, with one source-parity Unicode defect repaired.

## Defect found and fixed

The Go port used `strings.TrimSpace` / `unicode.IsSpace` in the sanitizer and finalizer. Those remove non-breaking space (`U+00A0`), while Saturn's `String.isBlank` / `strip` / `stripTrailing` use Java `Character.isWhitespace`, which deliberately does not classify `U+00A0` as whitespace. This violated the required ordinary Unicode preservation and could turn an otherwise visible response into an empty-response error.

`internal/agent/live/response_sanitizer.go` now uses local `isJavaWhitespace` and `stripJavaWhitespace` helpers matching Saturn's Java whitespace boundary. The finalizer now uses that same source-shaped boundary for empty and exact-marker checks. ASCII-only trimming after embedded marker removal remains unchanged.

RED → GREEN evidence:

- `TestResponseSanitizerPreservesNonBreakingSpaceLikeSaturn` initially failed: `sanitize() = "ordinary", want "ordinary\\u00a0"`.
- `TestOutputFinalizerPreservesNonBreakingSpaceLikeSaturn` initially failed with `agent returned an empty response`.
- Both pass after the fix.

## Contract review

- Sanitizer order remains source-shaped: legacy marker/decorative-opening cleanup, legacy opening removal, documented boilerplate-line removal, exact list normalization, then trailing Java whitespace only.
- Empty sanitized content returns `agent returned an empty response`.
- Exact sanitized marker is silent only for non-required/ambient modes and returns `agent declined a required response` for required modes.
- Embedded markers are removed before ASCII-control trim; resulting empty text errors.
- Visible output is capped by Go runes without byte splitting or ellipsis.
- Config resolves file-default `maxOutputChars=8000`, applies runtime precedence, and rejects runtime zero/negative/over-limit values. Enabled live and direct composition use `outputFinalizer(resolved)`.
- Typed `SuppressReply` returns before finalization. Runtime sends before `AfterDelivery`; direct command sends before `PersistDelivery`; silent/error/suppressed paths do not invoke those persistence callbacks.

## Gates

Passed:

```sh
go test ./internal/agent/live -run 'Test(ResponseSanitizer|OutputFinalizer|MarkerFinalizer)' -count=1
go test ./internal/config -run 'Test.*Agent.*(Output|Config)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent|OutputFinalizer)' -count=1
go test ./internal/agent/live ./internal/agent/runtime ./internal/config ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

`go vet ./...` remains informational-only and reports the known unrelated warning:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Files modified by this QA

- `internal/agent/live/response_sanitizer.go`
- `internal/agent/live/response_sanitizer_test.go`
- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `.hermes/handoffs/rapid-agent-final-response-normalization-qa.md`

## Exclusions respected

No provider/completion, tool-loop/command-channel, gateway, listener/transport, persistence-schema, H2/SQL, Saturn-source, protected-document, commit, or push changes were made.
