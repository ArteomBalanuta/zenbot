FROM golang:1.25.5-bookworm AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY resources resources
RUN CGO_ENABLED=0 go build -o ./target/zenbot ./cmd/zenbot

FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /app/target/zenbot /app/zenbot
COPY resources /app/resources
ENTRYPOINT ["/app/zenbot"]
