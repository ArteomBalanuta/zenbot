FROM golang:1.25.5-bookworm AS build

# Install sqlite3, and dos2unix
RUN apt-get update && \
    apt-get install -y sqlite3 dos2unix libc6-dev ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod .
COPY go.sum .

COPY cmd cmd
COPY internal internal
COPY deploy deploy

# Fix line endings for shell scripts
RUN dos2unix deploy/*.sh

# Make the script executable and run it
RUN chmod +x deploy/create_db.sh
RUN mkdir database
RUN /bin/bash deploy/create_db.sh

# Build with CGO for SQLite3 but static linking
ENV CGO_ENABLED=1
RUN go build \
    -tags osusergo,netgo,sqlite_omit_load_extension \
    -ldflags="-w -s -linkmode external -extldflags '-static'" \
    -o ./target/zenbot \
    ./cmd/zenbot

RUN chmod +x ./target/zenbot


## RUNTIME
FROM scratch

WORKDIR /app
# Copy CA certificates
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/database/database.db /app/database/database.db
COPY --from=build /app/target/zenbot /app/zenbot

ENTRYPOINT ["/app/zenbot"]
