# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.5

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY VERSION version.go ./
COPY cmd ./cmd
COPY internal ./internal
COPY resources ./resources
# Both the embedded SQLite driver and SQL policy parser are pure Go.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zenbot ./cmd/zenbot

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates sqlite3 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/zenbot /app/zenbot
COPY --from=build /src/resources /app/resources
COPY config.example.toml /app/config.toml
RUN mkdir -p /app/database
VOLUME ["/app/database"]
EXPOSE 6060
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/zenbot"]
