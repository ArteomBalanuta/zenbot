# Getting started

## Choose a deployment

Docker is the shortest path to a complete runtime. Local development is useful
when editing Go code or running focused tests. Both paths need network access
to the chat server. Optional weather, time, search, and agent features contact
their configured services; running ordinary commands does not require an LLM.

Use a private test room before granting moderation privileges. Do not point
development tests or experiments at a production database.

## Docker

Prerequisites: Docker Engine/Desktop, Make, and a shell.

```sh
cp config.example.toml config.toml
```

Edit the ignored `config.toml` before starting:

- Set `channel` to the room you intend to operate in.
- Set `nick` to the bot nickname and `trip` to its trip-generation secret.
- Set `adminTrips` to the public tripcode(s) allowed to administer the bot.
- Leave `[agent].enabled = false` until ordinary commands work.
- Keep `dbPath = "database/database"` for the default mounted storage.

The bot trip secret and a user's public tripcode are not interchangeable.
Never publish the bot's trip secret or model API key.

```sh
make build
make run
make logs
```

`make run` recreates the named container, mounts the configuration read-only, and
persists the database outside it. It uses `config.example.toml` if `config.toml`
is absent, but its placeholder identities are not a usable production setup.
Use `make stop` to stop the container without deleting its database.

In the configured room, try `*help`, `*ping`, and `*weather Chisinau`. Replace `*`
if you changed `cmdPrefix`. Help is sent privately. Server moderation rights
must be granted separately from the bot's configured role lists.

## Local development

Prerequisites:

- Go 1.24 or newer; the Docker build uses Go 1.25.5.
- A C compiler and platform development tools: the SQL parser requires CGO.
- Java 21 on `PATH` (the integration tests also invoke `java` directly).
- H2 **2.3.232**, matching the checksum-pinned Docker download. Startup checks
  H2 identity, not exact version compatibility; use the supported pinned jar.
- Make, `curl`, and `shasum` for the commands below.

From the repository root, download the same pinned jar used by the image:

```sh
mkdir -p .cache/h2
curl --fail --location --show-error \
  https://repo.maven.apache.org/maven2/com/h2database/h2/2.3.232/h2-2.3.232.jar \
  --output .cache/h2/h2-2.3.232.jar
printf '%s\n' '8dae62d22db8982c3dcb3826edb9c727c5d302063a67eef7d63d82de401f07d3  .cache/h2/h2-2.3.232.jar' | shasum -a 256 -c -
export H2_JAR="$PWD/.cache/h2/h2-2.3.232.jar"
```

Stop if checksum verification fails. Do not run an unverified jar. On systems
with `sha256sum` instead of `shasum`, use `sha256sum -c -` for the last check.
An existing pinned Maven-cache jar is also suitable; set `H2_JAR` to its absolute
path. Tests fall back to the current user's Maven cache, but the application
requires `H2_JAR` explicitly.

```sh
go mod download
make check
make compile
cp config.example.toml config.toml
# Edit config.toml before starting.
./target/zenbot
```

Do not overwrite an existing local configuration with the copy command. Run the
binary from the repository root: it reads `config.toml` from the working
directory, and the agent loads `resources/agent/` at runtime. The compiled binary
alone is not a complete agent distribution. The H2 schema and version string,
unlike the prompts, are embedded in the binary.

The application starts or connects to H2's loopback PostgreSQL-wire endpoint on
port 5435 and verifies H2 identity. Avoid sharing that port between unrelated
instances. Integration tests use their own temporary databases and ephemeral
ports, not the production port or database.

## Enable the agent

Configure the provider base URL, model if required, API-key environment variable,
and creator trip in `[agent]`, then enable it. The client appends
`/v1/chat/completions` to the base URL. A provider must support the tool-calling
contract; a generic text endpoint is insufficient.

For Docker, place only required overrides in an ignored `.env` or supply the
configured API-key variable to `make run`. The application itself does **not**
load `.env` files. Inside a container, `localhost` means the container, not your
host model server. Choose a reachable provider address appropriate to your
deployment; see [configuration](configuration.md).

Test `*l hello` in your test room. Read [agent execution](agent.md) before testing
state-changing commands. Provider calls may incur costs and transmit conversation
context. No live-provider evaluation runs by default in the test suite.
