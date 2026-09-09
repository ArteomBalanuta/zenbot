# Automove listener-order implementation

## Scope

Recovery step 7 only. `UserJoinedListener` can now compose a second narrow `JoinAutomation` hook as automove and invokes it only after the existing semantic automation, subscription share, and presence log. No factory, main, command, core, repository, config, or standalone automove-policy file was changed.

## RED → GREEN record

### Ordering composition

Added exactly one focused test: `TestUserJoinedListenerRunsAutoMoveLastAfterExistingJoinEffects`. It proves both automations see the active user and records this order:

```text
semantic → share → log → automove
```

RED before production changes:

```text
$ go test ./internal/listener -run '^TestUserJoinedListenerRunsAutoMoveLastAfterExistingJoinEffects$' -count=1
# zenbot/internal/listener [zenbot/internal/listener.test]
internal/listener/user_joined_listener_test.go:93:2: undefined: NewUserJoinedListenerWithAutomations
FAIL    zenbot/internal/listener [build failed]
FAIL
```

Minimum GREEN change: add `autoMove JoinAutomation`, invoke it after `LogPresence`, and add `NewUserJoinedListenerWithAutomations(e, automation, autoMove JoinAutomation)`. `JoinAutomation` is the narrow `OnJoin(context.Context, *model.User)` contract; no automove concrete dependency was introduced.

GREEN:

```text
$ gofmt -w internal/listener/user_joined_listener.go internal/listener/user_joined_listener_test.go && go test ./internal/listener -run '^TestUserJoinedListenerRunsAutoMoveLastAfterExistingJoinEffects$' -count=1
ok      zenbot/internal/listener    0.435s
```

No separate malformed-payload RED→GREEN cycle was required: the pre-existing `Notify` parse-error return is before **all** automation hooks, and `TestUserJoinedListenerInvokesTrustedAutomationAfterRegistrationAndIgnoresMalformed` already proves malformed input invokes no `JoinAutomation`. The newly composed hook is reached only after that same return point.

## Verification

```text
$ go test ./internal/listener -run 'Test(UserJoinedListener|AutoMoveJoin)' -count=1
ok      zenbot/internal/listener    0.256s

$ go test -race ./internal/listener -run 'Test(UserJoinedListener|AutoMoveJoin)' -count=1
ok      zenbot/internal/listener    1.387s

$ go test ./internal/listener -count=1
ok      zenbot/internal/listener    0.245s

$ git diff --check
(exit 0; no output)
```

## Exclusions retained

- No production construction/wiring was added in `internal/factory` or `cmd/zenbot/main.go`.
- Temporary-session dummy listener paths remain untouched.
- `AutoMoveJoinAutomation` and its policy behavior remain unchanged.
- No reset, clean, checkout, stage, or commit was performed.
