#!/bin/sh
# Pinned H2 PostgreSQL-wire prerequisite: H2 2.3.232
exec java -cp "${H2_JAR:?set H2_JAR to h2-2.3.232.jar}" org.h2.tools.Server -pg -pgPort "${H2_PORT:-5435}" -ifNotExists -baseDir "${H2_BASE_DIR:-./data}"
