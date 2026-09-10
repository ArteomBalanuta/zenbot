# Rapid agent final-response normalization implementation

## Delivered scope

- Added the pure live-local `responseSanitizer`, porting Saturn's deterministic legacy persona removal and list normalization.
- Added `live.OutputFinalizer`: sanitize, reject sanitized empty output, exact no-reply marker behavior by `Mode.RequiresReply`, remove embedded marker text, ASCII control whitespace trim, and rune-count output cap with no ellipsis.
- Added `[agent] maxOutputChars`, resolved with default `8000`, runtime precedence, and resolved validation range `1..1000000`.
- Production direct `l` and live runtime construct identically configured `OutputFinalizer` values through `outputFinalizer(resolved)`; disabled branches remain before agent composition.
- Kept command `SuppressReply` before finalization unchanged. Existing send-before-persistence paths consume only the finalizer result/completion artifact.

## Files changed

- `internal/agent/live/response_sanitizer.go` (new)
- `internal/agent/live/response_sanitizer_test.go` (new)
- `internal/agent/live/runner.go`
- `internal/agent/live/runner_test.go`
- `internal/config/agent_config.go`
- `internal/config/agent_config_participation_test.go`
- `cmd/zenbot/main.go`
- `cmd/zenbot/live_agent_test.go`
- `config.example.toml`

## RED → GREEN evidence

1. Sanitizer tests first failed with `undefined: responseSanitizer`; adding the pure implementation made `go test ./internal/agent/live -run 'TestResponseSanitizer' -count=1` pass.
2. Finalizer behavior tests first failed with `undefined: OutputFinalizer`; adding `OutputFinalizer` and the compatibility delegation made `go test ./internal/agent/live -run 'Test(ResponseSanitizer|OutputFinalizer|MarkerFinalizer)' -count=1` pass.
3. Config tests first failed with `MaxOutputChars undefined` / `unknown field MaxOutputChars`; adding resolved config support made `go test ./internal/config -run 'TestAgentConfig(OutputBound|ParticipationDefaults)' -count=1` pass.
4. Composition test first failed with `undefined: outputFinalizer`; adding the shared constructor and replacing both production constructions made `go test ./cmd/zenbot -run 'TestOutputFinalizerUsesResolvedMarkerAndBound' -count=1` pass.
5. A source-shape regression for an interior boilerplate line first failed: `boilerplate removal = "first\n\nsecond", want "first\nsecond"`; filtering legacy lines made its focused sanitizer test pass.

## Semantics and adaptations

- Required direct/mention exact marker returns `agent declined a required response`; ambient exact marker is silent.
- Embedded marker text is removed but residual text is visible. Empty content after sanitation/removal returns `agent returned an empty response`.
- The cap uses `[]rune`, so valid Unicode is not split and length is not byte-based.
- This ports only Saturn's deterministic post-correction contract. No correction completion, retry, provider call, tool protocol, persistence schema, transport, or command-channel change was made.
- `MarkerFinalizer` remains only as a compatibility adapter delegating to a safe 8000-rune `OutputFinalizer`; production uses `OutputFinalizer` directly.

## Gates

- Focused rapid gates passed:
  - `go test ./internal/agent/live -run 'Test(ResponseSanitizer|OutputFinalizer|MarkerFinalizer|Runner.*(Final|CommandProse)|Direct.*(Final|CommandProse))' -count=1`
  - `go test ./internal/config -run 'Test.*Agent.*(Output|Config)' -count=1`
  - `go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent|OutputFinalizer)' -count=1`
  - `go test ./internal/agent/live ./internal/agent/runtime ./internal/config ./cmd/zenbot -count=1`
- Full `go test ./... -count=1`, `go build ./...`, and `git diff --check` passed after the final sanitizer regression refinement.
- `go vet ./...` reaches the known unrelated pre-existing warning: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`.
