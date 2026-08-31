# Rapid agent command-prose QA

## Verdict

**PASS with one hardening correction applied.** The bounded command-prose channel preserves the accepted one-call/two-completion ceiling and typed reply suppression prevents duplicate delivery and persistence after a delivered command.

## QA change

- Hardened `internal/agent/live/command_channel.go`: the concrete prose guard now derives aliases from the exact provider-visible `run_command` definition selected for the correction request, rather than independently rebuilding aliases from the registered descriptor. Malformed exposed parameters fail closed.
- Added `TestToolLoopDerivesCommandProseAliasesFromExposedRunCommandDefinition` in `internal/agent/live/tool_loop_test.go`. It mutates the exposed enum to only `ping` and proves inline `` `weather Tokyo` `` is not recognized, corrected, or sent. The test was RED before the hardening (the old path entered correction and exhausted its scripted second completion), then GREEN.

## Confirmed gates

- Guard aliases come solely from exposed `properties.command.enum`; unadvertised `whois`/`kick`, malformed schemas/calls, extra keys, non-string arguments, and case-mismatched structured calls fail closed.
- Recognition is limited to source-shaped inline backticks and complete backtick/tilde fenced blocks; ordinary prose does not match.
- Public no-call recognized prose gets only completion #2 with `run_command` and private closed `respond_without_command`; exactly one matching action or one nonblank command-free fallback is accepted. Invalid, multiple, unrelated, truncated, and repeated-prose correction outputs fail without a third completion.
- Corrected action reuses the existing registry, executor, ledger, descriptor validation, timeout/context, and gateway. It sends once and returns typed `SuppressReply`, with no evidence or ordinary reply persistence.
- Normal model-selected `run_command` suppresses only a recognized command-shaped final completion after successful execution; normal textual acknowledgements remain unchanged.
- Whisper prose is rejected with no correction definitions, gateway call, or second completion. Mandatory fresh history takes precedence. Runtime and direct paths turn `SuppressReply` into no delivery/no persistence and do not raise empty-result errors.
- Frozen three-tool composition and one-call/two-completion limits remain intact.

## Commands run

```text
go test ./internal/agent/turn -run 'Test.*(CommandProseGuard|Policy)' -count=1
go test ./internal/agent/live -run 'Test(ToolLoop.*(CommandProse|RunCommand|Fresh|Whisper)|CommandChannel|Runner.*CommandProse|Direct.*CommandProse)' -count=1
go test ./internal/agent/tool ./internal/agent/tool/execution ./internal/command -run 'Test(RunCommand|Executor|AgentCommandGateway)' -count=1
go test ./cmd/zenbot -run 'Test.*(LiveAgent|DirectAgent)' -count=1
go test ./internal/agent/turn ./internal/agent/live ./internal/agent/tool ./internal/agent/tool/execution ./internal/command ./cmd/zenbot -count=1
go test ./... -count=1
go build ./...
git diff --check
```

All passed. Informational `go vet ./...` remains nonzero only for the known unrelated copylocks warning at `internal/core/engine_impl.go:95:22`.

## Exclusions

No gateway, transport, listener, command catalog, moderation, SQL/repository schema, configuration, or protected-document changes were made. No commit or push was made.
