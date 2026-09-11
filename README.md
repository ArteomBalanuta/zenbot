# Zenbot

A Go bot for Hack.Chat rooms, with role-aware commands, moderation, persistent
history and mail, replica management, and an optional LLM agent.

Zenbot retains many Saturn command aliases while using a Go runtime and a
file-backed H2 database. Java is required for H2; SQLite is used only to import
legacy databases. The agent is optional and disabled in the example configuration.

## Start here

- [Installation and first run](docs/getting-started.md)
- [Configuration reference](docs/configuration.md)
- [Commands and permissions](docs/commands.md)
- [Architecture](docs/architecture.md) and [agent execution](docs/agent.md)
- [Operations, backups, and troubleshooting](docs/operations.md)
- [Development and testing](docs/development.md)
- [Security and privacy](SECURITY.md)
- [Complete documentation index](docs/index.md)

## Docker setup: download, configure, start

You do **not** need to install Go, Java, H2, or Make on your computer for this
path. Docker builds the bot and packages its runtime dependencies automatically.
You only need Docker, the project files, and a text editor. The first build takes
longer because it downloads dependencies; later builds reuse the cache.

### 1. Install Docker and get Zenbot

Install and start [Docker Desktop or Docker Engine](https://docs.docker.com/get-started/get-docker/).
The commands below use a macOS/Linux terminal or a Windows WSL terminal with
Docker integration enabled; they are not PowerShell commands.

On GitHub, choose **Code → Download ZIP**, extract it, and open a terminal in the
extracted folder. Alternatively, if you have Git:

```sh
git clone https://github.com/ArteomBalanuta/zenbot.git
cd zenbot
```

Check that Docker is running:

```sh
docker info
```

If this cannot connect to Docker, start Docker Desktop/the Docker service before
continuing. On Linux, your user also needs permission to use Docker.

### 2. Set your room and bot identity

For a fresh installation:

```sh
cp config.example.toml config.toml
mkdir -p database
touch .env
```

Do not copy over an existing `config.toml` when updating. Open `config.toml` in
your text editor and change these existing values near the top:

```toml
channel = "YOUR_TEST_ROOM"
nick = "YourBot"
trip = "YOUR_PRIVATE_BOT_TRIP_SECRET"
adminTrips = "YOUR_PUBLIC_ADMIN_TRIPCODE"
```

Use a room you control. `trip` is the bot's **private trip-generation secret**;
`adminTrips` contains your **public tripcode**, not your password. Keep the rest
of the example settings initially, including `dbPath = "database/database"`
and `[agent].enabled = false`. Save the file.

The empty `.env` is ready for optional API credentials later. Both files are
ignored by Git; never share their real contents. Server-side moderator rights
must be granted to the bot separately from configuring its administrator list.

### 3. Build and start

Run these commands from the same project folder:

```sh
docker build --pull -t zenbot .
docker run -d --init --name zenbot \
  --env-file .env \
  --mount "type=bind,source=$PWD/config.toml,target=/app/config.toml,readonly" \
  --mount "type=bind,source=$PWD/database,target=/app/database" \
  zenbot
docker logs -f zenbot
```

Press **Ctrl+C** to stop following logs; the bot keeps running in the background.
Open your configured Hack.Chat room and send `*help` or `*ping` to check it.
Help arrives privately. No web dashboard or inbound port is needed for ordinary
bot operation.

The image contains the binary, Java 21, checksum-pinned H2 2.3.232, and prompt
resources. Configuration is mounted read-only; persistent data stays in your
host's `database/` folder even when the container is removed. Keep that folder
private and [back it up](docs/operations.md#backups-and-restores).

If you already have Make installed, `make build && make run` is the shorter
equivalent using the same files. The Make route additionally publishes the
optional profiler port on host loopback; profiling itself remains disabled.

### 4. Optional: enable AI

The bot works without AI. The Docker image does **not** include a model server
or API subscription. To use one you already have, edit the existing `[agent]`
section in `config.toml`:

- Set `enabled = true`.
- Set `endpoint` to the provider's reachable base URL, without `/v1` or
  `/v1/chat/completions`; Zenbot appends the latter route.
- Set `model` to the provider's model name if required.
- Replace `creatorTrip` with your public tripcode.

If the provider requires a key, add `SATURN_AGENT_API_KEY=your-real-key` to the
private `.env` file in your editor. Do not put real credentials in shell commands
or committed examples. Container `localhost` is not your host computer; consult
[Docker networking](https://docs.docker.com/desktop/features/networking/) and
the [configuration guide](docs/configuration.md) when using a host-side model.

Apply these changes by stopping/removing the existing container and repeating
the `docker run` command from step 3:

```sh
docker stop --timeout 30 zenbot
docker rm zenbot
# Repeat the docker run command above, then try: *l hello
```

Changing `.env` requires container recreation, not just `docker restart`. AI
requests can incur provider costs and send conversation context to that provider.
Read [agent behavior and limitations](docs/agent.md) before enabling moderation.

### Everyday commands and updates

| Task | Command |
| --- | --- |
| Show container state | `docker ps -a --filter name=zenbot` |
| Follow logs | `docker logs -f zenbot` |
| Stop safely | `docker stop --timeout 30 zenbot` |
| Start the existing container | `docker start zenbot` |
| Apply config/environment changes | Stop/remove the container, then repeat step 3's `docker run` |

To update a Git checkout, back up first, run `git pull --ff-only`, rebuild the
image, then stop/remove the old container and repeat `docker run`. For a ZIP
installation, use the new source files while preserving your private
`config.toml`, `.env`, and `database/`. Do not run two instances against the same
database. Container removal alone does not delete the bind-mounted data.

If Docker reports that the name `zenbot` is already in use, inspect the existing
container before removing it; it may be your running bot. Startup failures can
also come from unchanged placeholders or an unreachable provider. See
[troubleshooting](docs/operations.md#troubleshooting). Logs can contain private
chat content, so redact them before sharing.

For development without Docker, follow [local setup](docs/getting-started.md#local-development).

## What it does

- User lookup, public history, private notes, queued mail, and subscriptions.
- Weather, time, room lists, and other chat utilities.
- Moderation commands, auditing, and configurable automation.
- Process-owned host recovery and replicas, plus bounded temporary room sessions.
- Optional model-driven tool execution with authorization, deadlines, bounded
  context, durable memory, and explicit action/delivery outcomes.

The agent is not a guarantee of correct reasoning or successful remote actions.
Treat unconfirmed actions as unconfirmed, and read the
[agent limitations](docs/agent.md) before enabling it in a populated room.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and
[development](docs/development.md) for prerequisites and test commands.
Changes should include focused regression tests and update the relevant reference
documentation.

No project license has been declared in this repository. Public visibility alone
is not a license grant; maintainers should select an appropriate license before
advertising the project as open source. Dependencies retain their own licenses.
