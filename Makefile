.DEFAULT_GOAL := help
.NOTPARALLEL:

IMAGE_NAME ?= zenbot
CONTAINER_NAME ?= zenbot
DOCKER ?= docker
GO ?= go
APP_DIR ?= /app
CONFIG_FILE ?= $(shell if [ -f "$(CURDIR)/config.toml" ]; then printf "%s" "$(CURDIR)/config.toml"; else printf "%s" "$(CURDIR)/config.example.toml"; fi)
ENV_FILE ?= $(CURDIR)/.env
DATABASE_DIR ?= $(CURDIR)/database
DATABASE_FILE ?= $(DATABASE_DIR)/zenbot.db
DATABASE_BACKUP_DIR ?= $(DATABASE_DIR)/backups
AGENT_API_KEY_ENV ?= SATURN_AGENT_API_KEY
PROFILING_HOST ?= 127.0.0.1
PROFILING_PORT ?= 6060
CONTAINER_PROFILING_PORT ?= 6060
PROFILE_SECONDS ?= 30
STOP_TIMEOUT ?= 30
TARGET_DIR ?= $(CURDIR)/target

.PHONY: help fmt format-check vet test check compile build prepare run start stop restart rm rmi clean rebuild logs shell ps status fresh-db db-check backup-db profile-goroutines profile-block profile-mutex profile-cpu

help:
	@printf "%s\n" \
		"make check     - Check formatting, vet, and run all Go tests" \
		"make compile   - Build the local target/zenbot binary" \
		"make build     - Build the self-contained Docker image" \
		"make run       - Recreate and run the container in detached mode" \
		"                 Uses ignored config.toml when present, otherwise config.example.toml" \
		"                 Loads ignored .env when present; agent/profiling overrides win over TOML" \
		"make start     - Start the existing container" \
		"make stop      - Stop the container if it exists" \
		"make restart   - Recreate and run the container" \
		"make rm        - Remove the container if it exists" \
		"make rmi       - Remove the Docker image if it exists" \
		"make clean     - Remove the container and image" \
		"make rebuild   - Clean, build, and run the image" \
		"make fresh-db  - Back up SQLite and remove the active database for a fresh start" \
		"make db-check  - Stop Zenbot and verify SQLite integrity" \
		"make backup-db - Stop Zenbot and create a consistent SQLite backup" \
		"make logs      - Follow container logs" \
		"make shell     - Open a shell in the running container" \
		"make ps        - Show matching containers" \
		"make status    - Show container status" \
		"make profile-goroutines - Print the current goroutine profile" \
		"make profile-block      - Open the blocking profile in pprof" \
		"make profile-mutex      - Open the mutex profile in pprof" \
		"make profile-cpu        - Capture and open a CPU profile (PROFILE_SECONDS=30)"

fmt:
	$(GO) fmt ./...

format-check:
	@files="$$(gofmt -l .)"; test -z "$$files" || { printf "Unformatted Go files:\n%s\n" "$$files"; exit 1; }

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

check: format-check vet test

compile:
	mkdir -p "$(TARGET_DIR)"
	$(GO) build -trimpath -o "$(TARGET_DIR)/zenbot" ./cmd/zenbot

build:
	$(DOCKER) build --pull -t "$(IMAGE_NAME)" .

prepare:
	@test -f "$(CONFIG_FILE)" || { echo "Configuration file not found: $(CONFIG_FILE)"; exit 1; }
	mkdir -p "$(DATABASE_DIR)"

run: prepare rm
	@set -eu; \
	set --; \
	if [ -f "$(ENV_FILE)" ]; then \
		set -- --env-file "$(ENV_FILE)"; \
	elif [ -n "$${$(AGENT_API_KEY_ENV):-}" ]; then \
		set -- --env "$(AGENT_API_KEY_ENV)"; \
	fi; \
	$(DOCKER) run -d \
		--init \
		--name "$(CONTAINER_NAME)" \
		"$$@" \
		-p "$(PROFILING_HOST):$(PROFILING_PORT):$(CONTAINER_PROFILING_PORT)" \
		-v "$(CONFIG_FILE):$(APP_DIR)/config.toml:ro" \
		-v "$(DATABASE_DIR):$(APP_DIR)/database" \
		"$(IMAGE_NAME)"

start:
	$(DOCKER) start "$(CONTAINER_NAME)"

stop:
	@if command -v "$(DOCKER)" >/dev/null 2>&1 && $(DOCKER) container inspect "$(CONTAINER_NAME)" >/dev/null 2>&1; then \
		$(DOCKER) stop --timeout "$(STOP_TIMEOUT)" "$(CONTAINER_NAME)"; \
	fi

restart: run

rm: stop
	@if command -v "$(DOCKER)" >/dev/null 2>&1 && $(DOCKER) container inspect "$(CONTAINER_NAME)" >/dev/null 2>&1; then \
		$(DOCKER) rm "$(CONTAINER_NAME)"; \
	fi

rmi:
	@if command -v "$(DOCKER)" >/dev/null 2>&1 && $(DOCKER) image inspect "$(IMAGE_NAME)" >/dev/null 2>&1; then \
		$(DOCKER) rmi "$(IMAGE_NAME)"; \
	fi

clean: rm rmi

rebuild: clean build run

# Maintenance must fail when Docker is unavailable; a failed inspect must never
# be mistaken for proof that the writer has stopped.
.PHONY: maintenance-stop
maintenance-stop:
	@set -eu; \
	$(DOCKER) info >/dev/null; \
	containers="$$($(DOCKER) container ls -a --format '{{.Names}}')"; \
	if printf '%s\n' "$$containers" | grep -Fxq "$(CONTAINER_NAME)"; then \
		$(DOCKER) stop --timeout "$(STOP_TIMEOUT)" "$(CONTAINER_NAME)"; \
	fi; \
	$(DOCKER) image inspect "$(IMAGE_NAME)" >/dev/null

fresh-db: backup-db
	rm -f "$(DATABASE_FILE)" "$(DATABASE_FILE)-wal" "$(DATABASE_FILE)-shm" "$(DATABASE_FILE)-journal"
	@echo "SQLite will create a fresh schema on the next Zenbot startup."

db-check: maintenance-stop
	@test -s "$(DATABASE_FILE)" || { echo "SQLite database not found or empty: $(DATABASE_FILE)"; exit 1; }
	@set -eu; \
	result="$$($(DOCKER) run --rm --entrypoint sqlite3 \
		--mount "type=bind,source=$(abspath $(dir $(DATABASE_FILE))),target=/data" \
		"$(IMAGE_NAME)" "/data/$(notdir $(DATABASE_FILE))" 'PRAGMA integrity_check;')"; \
	printf '%s\n' "$$result"; test "$$result" = ok

backup-db: maintenance-stop
	@test -s "$(DATABASE_FILE)" || { echo "SQLite database not found or empty: $(DATABASE_FILE)"; exit 1; }
	@set -eu; \
	mkdir -p "$(DATABASE_BACKUP_DIR)"; \
	backup_dir="$$(mktemp -d "$(abspath $(DATABASE_BACKUP_DIR))/snapshot-XXXXXXXX")"; \
	$(DOCKER) run --rm --entrypoint sqlite3 \
		--mount "type=bind,source=$(abspath $(dir $(DATABASE_FILE))),target=/data" \
		--mount "type=bind,source=$$backup_dir,target=/backup" \
		"$(IMAGE_NAME)" "/data/$(notdir $(DATABASE_FILE))" '.backup /backup/zenbot.db'; \
	test -s "$$backup_dir/zenbot.db"; \
	result="$$($(DOCKER) run --rm --entrypoint sqlite3 \
		--mount "type=bind,source=$$backup_dir,target=/backup" \
		"$(IMAGE_NAME)" /backup/zenbot.db 'PRAGMA journal_mode=DELETE; PRAGMA integrity_check;')"; \
	test "$$result" = "$$(printf 'delete\nok')" || { printf '%s\n' "$$result"; exit 1; }; \
	echo "Database backup created: $$backup_dir/zenbot.db"

logs:
	$(DOCKER) logs -f "$(CONTAINER_NAME)"

shell:
	$(DOCKER) exec -it "$(CONTAINER_NAME)" sh

ps:
	$(DOCKER) ps -a --filter "name=$(CONTAINER_NAME)"

status:
	$(DOCKER) inspect --format '{{.Name}} {{.State.Status}}' "$(CONTAINER_NAME)"

profile-goroutines:
	curl --fail --silent --show-error "http://$(PROFILING_HOST):$(PROFILING_PORT)/debug/pprof/goroutine?debug=2"

profile-block:
	$(GO) tool pprof -http=:0 "http://$(PROFILING_HOST):$(PROFILING_PORT)/debug/pprof/block"

profile-mutex:
	$(GO) tool pprof -http=:0 "http://$(PROFILING_HOST):$(PROFILING_PORT)/debug/pprof/mutex"

profile-cpu:
	$(GO) tool pprof -http=:0 "http://$(PROFILING_HOST):$(PROFILING_PORT)/debug/pprof/profile?seconds=$(PROFILE_SECONDS)"
