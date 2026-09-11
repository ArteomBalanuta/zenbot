# Operations and troubleshooting

## Runtime files and ownership

The default container runs in `/app`, reads `/app/config.toml`, and stores the H2
database under `/app/database`. `make run` mounts the selected host configuration
read-only and the selected host database directory read/write. Replacing a
container does not remove that mounted database.

The process owns its master connection, managed replicas, agent runtime, and any
H2 child it starts. Temporary remote-room snapshots are bounded operations, not
managed replicas. A successful websocket write proves a request was sent, not that
the server applied an action. See [commands](commands.md) for individual semantics.

Do not run two bot instances against the same database stem. Local application
startup uses H2 port 5435. It can attach to an existing listener and then validates
the H2 identity; do not treat an occupied port as proof that the intended database
is already running. Test fixtures use separately owned ephemeral ports.

`deploy/h2-server.sh` is retained for explicit external-server diagnostics. It is
not a prerequisite for normal startup. Its environment options are `H2_JAR`,
`H2_PORT` (5435), and `H2_BASE_DIR` (`./data`). Stop diagnostic servers when done.

## Lifecycle commands

| Command | Effect |
| --- | --- |
| `make build` | Build the Docker image; does not deploy it |
| `make run` / `make restart` | Stop/remove the named container and recreate it |
| `make start` | Start an existing stopped container |
| `make stop` | Stop the container if present |
| `make rm` | Stop and remove the named container |
| `make clean` | Remove container and image, not the mounted database |
| `make rebuild` | Clean, build, and run |
| `make logs` | Follow container logs; potentially sensitive |
| `make status` / `make ps` | Inspect container state |

Overrides such as `CONTAINER_NAME`, `IMAGE_NAME`, `CONFIG_FILE`, and `DATABASE_DIR`
are Make variables, not application TOML settings. Inspect `make -n <target>`
before operating a nondefault deployment. See [configuration](configuration.md).

Changes to an ignored configuration file take effect on process recreation.
The `prefix` chat command is a live change, not a configuration-file editor.
Host restart/shutdown chat commands submit lifecycle requests; their acknowledgments
do not confirm a completed restart or shutdown.

## Backups and restores

```sh
make backup-db
# Confirm the reported backup file exists, then restart explicitly:
make start
```

`backup-db` stops the container before copying the H2 file into
`database/backups/`. It deliberately leaves the container stopped. Protect these
backups like the live database: they can contain private messages and credentials
stored by application features. Copy them to independently protected storage;
backups in the same directory are not protection against disk loss.

For a restore, stop **every** process using the database. Preserve a copy of the
current database before replacing it with a known compatible backup. Restore to
the configured database stem (default `database/database.mv.db`), preserving file
permissions. Run `make db-check` and then `make start`; verify application behavior
in a test room. Do not copy live H2 files or assume older binaries understand a
newer schema. This project has no automated downgrade migration.

```sh
make db-check
```

`db-check` also stops the container and leaves it stopped. It requires a built
image and queries the stopped database with the pinned H2 jar in read-only mode.
It checks the version and application-table count, not every application invariant
or backup's logical correctness.

**Destructive reset:** `make fresh-db` archives the legacy SQLite input but
deletes the active H2 database and its trace/lock sidecars. Back up first. The
target name must not be mistaken for a lossless migration or restore operation.

## SQLite import and H2 upgrades

H2 is the only runtime store. Startup applies the embedded schema and versioned
upgrade logic in `internal/repository/h2/`. If a legacy `<dbPath>.db` SQLite file
exists and the corresponding H2 application tables are empty, the migrator reads
SQLite, recreates tables/indexes, imports rows, restores identity counters, and
verifies row counts before committing. It archives the SQLite source and sidecars
only after successful import. If both stores contain data, startup fails instead
of merging potentially conflicting records.

Keep an independent copy of the original SQLite files before migration. Do not
delete a source merely because an H2 file appeared; verify the import completed
and the expected records are available. For schema changes, use the
[development tests](development.md), including migration and schema-upgrade cases.

## Profiling

Profiling is opt-in. A typical container configuration is:

```toml
[profiling]
enabled = true
listenAddress = "0.0.0.0:6060"
slowCommandThresholdMillis = 250
slowStageThresholdMillis = 25
slowTransportThresholdMillis = 25
blockProfileRate = 1000000
mutexProfileFraction = 5
```

The Make defaults publish port 6060 on host loopback only. For a local process,
prefer `127.0.0.1:6060` as the listen address. Pprof has no application-level
authentication; never expose it publicly. Keep `CONTAINER_PROFILING_PORT` aligned
with the configured internal port if changing it.

```sh
make profile-goroutines
make profile-block
make profile-mutex
make profile-cpu PROFILE_SECONDS=30
```

`command.profile.started` includes websocket queue delay, JSON parse duration, and
queue depth. Stage records identify slow listeners, authorization, handlers, and
auditing. `transport.profile.inbound_enqueue` indicates reader backpressure;
`transport.profile.write` separates writer-lock and network-write delay. A long
earlier synchronous listener can delay later messages in the same consumer.

Structured regular-command profiling excludes the direct `l` command, but runtime
CPU/goroutine profiles are process-wide. Use `zenbot.command` and `zenbot.room`
pprof labels and isolate the workload when diagnosing command latency. Disable
profiling when finished if you do not need its overhead or exposed endpoint.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Cannot read configuration | Working directory and `config.toml`; Docker `CONFIG_FILE` mount |
| H2 jar/runtime unavailable | Absolute `H2_JAR`, Java on `PATH`, pinned version/checksum |
| H2 startup or identity error | Port 5435 ownership, file permissions, existing server version |
| Tests fail on H2 setup | Install the jar; do not skip integration tests to hide missing prerequisites |
| Bot connects but cannot moderate | Server-side moderator rights and application caller role are separate |
| Agent disabled/unavailable | `[agent].enabled`, reachable base endpoint, creator trip, API key and capability gates |
| Provider unreachable in Docker | Container `localhost` is not the host; check deployment-specific networking |
| Unexpected config value | `.env` precedence, TOML alias priority, resolver defaults; consult configuration guide |
| Empty lookup | No public records may exist; `nicks`/`t2n` is trip-to-nick, not name-to-trip |
| Mail not automatically retried | An ambiguous send is deliberately persisted as uncertain to avoid duplicates |
| Action outcome unknown | Inspect authorized server state; do not blindly repeat a potentially completed action |

Share sanitized errors and versions, not raw room logs or database files. See
[security](../SECURITY.md) for the limits of existing logging and data retention.
