# H2 PostgreSQL-wire prerequisite

Zenbot production persistence is real H2, reached through H2's PostgreSQL compatibility server and `github.com/jackc/pgx/v5/stdlib`. It does not embed a JVM.

Pinned prerequisite: H2 **2.3.232** (`h2-2.3.232.jar`) and a Java runtime. Start the server before Zenbot:

```sh
H2_JAR=/opt/h2/h2-2.3.232.jar H2_BASE_DIR=/var/lib/zenbot H2_PORT=5435 ./deploy/h2-server.sh
```

The server uses `org.h2.tools.Server -pg -pgPort 5435 -baseDir /var/lib/zenbot`. Database stem `zenbot` maps to `/var/lib/zenbot/zenbot.mv.db`; Zenbot verifies `SELECT H2VERSION()` and bootstraps the schema transactionally. Startup fails closed if Java, the jar, the server, the database, or the H2 identity check is unavailable.

The adapter uses the PostgreSQL-wire subset implemented by H2: connection, parameterized DML/queries, transactions, identity columns, constraints, and metadata needed by `resources/schema-h2.sql`. The H2 PG server is an explicit operational boundary.

This checkout contains the migration foundation and a real-H2 integration test. The remaining Saturn command implementations, listener chains, services, lifecycle, and full agent runtime are not yet complete.
