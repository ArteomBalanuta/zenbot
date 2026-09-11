# Development and verification

## Prerequisites

Follow [local setup](getting-started.md#local-development): Go, a C compiler,
Java 21, and the checksum-verified H2 2.3.232 jar. Set `H2_JAR` to an absolute path.
Runtime H2 startup supports `JAVA`; several tests invoke `java` directly, so the
desired runtime must also be on `PATH`.

Go's SQL parser dependency uses CGO. `CGO_ENABLED=0` is not a supported substitute
for installing compiler tooling. `modernc.org/sqlite` remains necessary for
legacy import tests even though SQLite is not a runtime storage option.

## Verification commands

```sh
make check
make compile
go test -race ./...
make build
git diff --check
```

`make check` runs formatting validation, `go vet`, and all ordinary tests.
`make compile` writes only the ignored `target/zenbot` binary. `make build` builds
the image without starting a bot. Neither a successful compile nor a Docker
image build proves successful operation against a real room/provider.

The [CI workflow](../.github/workflows/ci.yml) provisions Go, Java, and a
checksum-verified H2 jar, then runs checks, compilation, full race tests, and a
Docker build. Actions are pinned to commit IDs. It has read-only repository
permissions and does not start a bot or enable the live-provider evaluation.

For iteration, run the packages and tests relevant to your change:

```sh
go test ./internal/command -run TestLastOnline -count=1
go test ./internal/repository/h2 -run TestPublicHistory -count=1
go test ./internal/agent/... -count=1
go test -race ./internal/agent/... -count=1
```

H2 integration tests start real Java processes on ephemeral loopback ports and
use temporary database directories. They fail when prerequisites are missing;
they do not silently switch to a mock database. Tests can take minutes on slower
machines, especially with race instrumentation. Avoid concurrently launching
several full database suites unless the machine has sufficient resources.

Regression tests with names containing `audit`, `parity`, or `red` are retained
because they assert current behavior. Their names do not make them generated
output or unfinished experiments.

## Repository map

| Path | Responsibility |
| --- | --- |
| `cmd/zenbot/` | Process composition, startup, host supervision, provider evaluation |
| `internal/config/` | TOML compatibility and validated configuration resolution |
| `internal/core/` | Connections, room state, lifecycle, replicas and operations |
| `internal/listener/` | Event chains and bounded remote-room snapshot workflows |
| `internal/command/` | Chat handlers, registry, authorization and agent gateway |
| `internal/agent/` | Model protocol, tools, context, memory, participation and execution |
| `internal/service/` | Application services and external-service formatting |
| `internal/repository/h2/` | Storage, embedded schema, upgrades and SQLite import |
| `internal/testutil/` | Portable, isolated real-H2 test setup |
| `resources/agent/` | Runtime prompt templates and tool copy; ship with the binary |
| `deploy/` | Explicit external-H2 diagnostic script |
| `docs/` | Maintained user/operator/contributor references |

## Adding a command or tool

Start with the [command reference](commands.md) and [agent extension guide](agent.md).
The catalog owns command identity, aliases, roles, and command-tool contracts.
Add the concrete handler and production registration, then test both direct chat
and agent invocation where supported. A generic command placeholder is not an
implementation. Keep `l` out of the agent tool inventory to prevent recursion.

Use the shared command authorization and observation/delivery boundaries rather
than creating an alternate execution path. Preserve the original message text
through storage and transport; normalize only fields whose semantics permit it.
Nickname selectors and opaque trip/hash identifiers have different rules.

Test missing data, unavailable services, canceled operations, failed sends,
privacy, and ambiguous remote outcomes as well as success. Receipts describe
verified effects, not inferred intention. Never retry an uncertain state-changing
operation merely because the transport returned an error.

## Prompt and provider changes

Prompt files are loaded from `resources/agent/`, not embedded. Keep these files
available from the process working directory when testing trimmed-path binaries.
Do not delete a prompt simply because its filename mentions correction: inspect
catalog callers and resource tests first.

The normal suite uses local fixtures. A separate, opt-in provider evaluation
contacts a configured model with synthetic conversations and simulated tools:

```sh
ZENBOT_PROVIDER_EVAL=1 \
ZENBOT_PROVIDER_EVAL_CONFIG="$PWD/config.toml" \
go test ./cmd/zenbot -run '^TestProviderToolLoopEvaluation$' -count=1 -v
```

This can incur costs. It does not construct a live chat engine, database, or room
action handler. `ZENBOT_PROVIDER_EVAL_TEMPERATURE` optionally sets a value from 0
through 2; otherwise the provider default is retained. Use private ignored config,
not committed credentials. Do not enable this flag in ordinary CI. Provider
evaluation is evidence about the tested model/settings, not a universal guarantee
of semantic correctness. Keep raw transcripts/logs out of the repository.

## Documentation and release hygiene

Update reference documentation with the code that changes it. Avoid maintaining
duplicate schema files or command tables with conflicting semantics. Source links
should be repository-relative, not absolute developer-machine paths.

Before sharing, review `git status --short`, staged changes, ignored files, and
Git history for sensitive material. Test a clean checkout with only declared
dependencies. Do not copy `config.toml`, `.env`, `database/`, `target/`, or private
agent work directories into release archives. Select a project license before
describing a release as open source; this checkout currently declares none.
