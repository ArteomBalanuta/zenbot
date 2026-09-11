# Operations and troubleshooting

## Runtime files and ownership

The default container runs in `/app`, reads `/app/config.toml`, and stores the SQLite
database under `/app/database`. `make run` mounts the selected host configuration
read-only and the selected host database directory read/write. Replacing a
container does not remove that mounted database.

The process owns its master connection, managed replicas, agent runtime, and
embedded SQLite connection. Temporary remote-room snapshots are bounded operations, not
managed replicas. A successful websocket write proves a request was sent, not that
the server applied an action. See [commands](commands.md) for individual semantics.

Do not run two bot instances against the same database file. SQLite runs inside
the bot process; it has no database server or listening port. Test fixtures use
separate temporary files.

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

`backup-db` stops the named container and uses SQLite's backup API through the
image's `sqlite3` CLI to create `database/backups/snapshot-XXXXXXXX/zenbot.db`.
This includes committed data still in the WAL; a plain copy of the live main
file would not. It verifies the backup with `PRAGMA integrity_check` and fails
if Docker, stopping, backup, or validation fails. It deliberately leaves the
container stopped. Stop any native processes using this file yourself. Protect these
backups like the live database: they can contain private messages and credentials
stored by application features. Copy them to independently protected storage;
backups in the same directory are not protection against disk loss.

For a restore, stop **every** process using the database. Preserve a copy of the
current database before replacing it with a known compatible backup. Restore to
the configured database filename (default `database/zenbot.db`), preserving file
permissions. Archive the current main file and its `-wal`, `-shm`, and `-journal`
sidecars together before restoring; stale sidecars must not accompany the restored
file. Run `make db-check` and then `make start`; verify application behavior
in a test room. Do not copy live SQLite files or assume older binaries understand a
newer schema. This project has no automated downgrade migration.

```sh
make db-check
```

`db-check` also stops the container and leaves it stopped. It requires a built
image and runs `PRAGMA integrity_check` through its SQLite CLI. The mount is
writable so SQLite can recover journals or create required sidecars. It requires
the result to be `ok`, but does not check every application invariant
or backup's logical correctness.

**Destructive reset:** `make fresh-db` first creates and validates a backup,
then removes the active SQLite file and its WAL/shared-memory/rollback-journal
sidecars. A failed backup prevents removal. Stop all other writers first. Startup
creates a fresh schema; restore the reported backup to recover the previous data.

## SQLite schema upgrades

SQLite is the only runtime store. Startup applies the embedded schema and upgrade
logic in `internal/repository/sqlite/`. The `dbPath` value is the exact filename;
suffixes are neither added nor stripped. This release does not import other
database formats. Preserve old deployments and their backups separately; changing
`dbPath` creates a new store rather than transferring data.

Supported SQLite upgrades are transactional: they retain identity sequences,
message visibility and encoding, relationships, and supported indexes/triggers.
An incompatible future schema, invalid foreign keys, null summary identities,
or unsupported extra columns in a table requiring a rebuild cause startup to
fail rather than silently discard data. Investigate the reported schema issue
against a backup; do not use `fresh-db` as an upgrade repair shortcut.

Keep an independent backup before upgrading. For schema changes, use the
[development tests](development.md), including schema-upgrade cases.

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
| SQLite cannot open database | Exact `dbPath`, directory permissions, available disk space, valid SQLite file |
| SQLite database busy | Another process using the same file; stop duplicate bot instances |
| Backup/check fails | Docker availability, built image, configured database filename and permissions |
| Bot connects but cannot moderate | Server-side moderator rights and application caller role are separate |
| Agent disabled/unavailable | `[agent].enabled`, reachable base endpoint, creator trip, API key and capability gates |
| Provider unreachable in Docker | Container `localhost` is not the host; check deployment-specific networking |
| Unexpected config value | `.env` precedence, TOML alias priority, resolver defaults; consult configuration guide |
| Empty lookup | No public records may exist; `nicks`/`t2n` is trip-to-nick, not name-to-trip |
| Mail not automatically retried | An ambiguous send is deliberately persisted as uncertain to avoid duplicates |
| Action outcome unknown | Inspect authorized server state; do not blindly repeat a potentially completed action |

Share sanitized errors and versions, not raw room logs or database files. See
[security](../SECURITY.md) for the limits of existing logging and data retention.
