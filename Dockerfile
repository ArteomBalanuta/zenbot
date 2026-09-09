# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.5
ARG JAVA_VERSION=21
ARG H2_VERSION=2.3.232

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY resources ./resources
# pg_query_go backs the SQL policy with libpg_query and therefore requires CGO.
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zenbot ./cmd/zenbot

FROM eclipse-temurin:${JAVA_VERSION}-jre AS h2
ARG H2_VERSION
ARG H2_SHA256=8dae62d22db8982c3dcb3826edb9c727c5d302063a67eef7d63d82de401f07d3
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && mkdir -p /opt/h2 \
    && curl --fail --location --show-error --silent \
       "https://repo.maven.apache.org/maven2/com/h2database/h2/${H2_VERSION}/h2-${H2_VERSION}.jar" \
       --output /opt/h2/h2.jar \
    && echo "${H2_SHA256}  /opt/h2/h2.jar" | sha256sum --check --strict \
    && rm -rf /var/lib/apt/lists/*

FROM eclipse-temurin:${JAVA_VERSION}-jre
WORKDIR /app
ENV H2_JAR=/opt/h2/h2.jar \
    JAVA=java
COPY --from=build /out/zenbot /app/zenbot
COPY --from=build /src/resources /app/resources
COPY --from=h2 /opt/h2/h2.jar /opt/h2/h2.jar
COPY config.example.toml /app/config.toml
RUN mkdir -p /app/database
VOLUME ["/app/database"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/zenbot"]
