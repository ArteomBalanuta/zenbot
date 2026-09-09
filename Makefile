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
DATABASE_STEM ?= $(DATABASE_DIR)/database
DATABASE_FILE ?= $(DATABASE_STEM).mv.db
LEGACY_DATABASE_FILE ?= $(DATABASE_STEM).db
DATABASE_BACKUP_DIR ?= $(DATABASE_DIR)/backups
CONTAINER_DATABASE_STEM ?= $(APP_DIR)/database/$(notdir $(DATABASE_STEM))
H2_CHECK_URL ?= jdbc:h2:file:$(CONTAINER_DATABASE_STEM);ACCESS_MODE_DATA=r;IFEXISTS=TRUE
AGENT_API_KEY_ENV ?= SATURN_AGENT_API_KEY
STOP_TIMEOUT ?= 30
TARGET_DIR ?= $(CURDIR)/target

.PHONY: help fmt format-check vet test check compile build prepare run start stop restart rm rmi clean rebuild logs shell ps status fresh-db db-check backup-db

help:
	@printf "%s\n" \
		"make check     - Check formatting, vet, and run all Go tests" \
		"make compile   - Build the local target/zenbot binary" \
		"make build     - Build the self-contained Docker image" \
		"make run       - Recreate and run the container in detached mode" \
		"                 Uses ignored config.toml when present, otherwise config.example.toml" \
		"                 Loads ignored .env when present; environment values override TOML" \
		"make start     - Start the existing container" \
		"make stop      - Stop the container if it exists" \
		"make restart   - Recreate and run the container" \
		"make rm        - Remove the container if it exists" \
		"make rmi       - Remove the Docker image if it exists" \
		"make clean     - Remove the container and image" \
		"make rebuild   - Clean, build, and run the image" \
		"make fresh-db  - Archive legacy SQLite and recreate H2 on next startup" \
		"make db-check  - Stop Zenbot and verify the H2 file" \
		"make backup-db - Stop Zenbot and create a consistent H2 file backup" \
		"make logs      - Follow container logs" \
		"make shell     - Open a shell in the running container" \
		"make ps        - Show matching containers" \
		"make status    - Show container status"

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

fresh-db: stop
	@set -eu; \
	mkdir -p "$(DATABASE_DIR)" "$(DATABASE_BACKUP_DIR)"; \
	if [ -f "$(LEGACY_DATABASE_FILE)" ]; then \
		stamp="$$(date +%Y%m%d-%H%M%S)"; \
		for source in "$(LEGACY_DATABASE_FILE)" "$(LEGACY_DATABASE_FILE)-wal" "$(LEGACY_DATABASE_FILE)-shm"; do \
			if [ -f "$$source" ]; then mv "$$source" "$(DATABASE_BACKUP_DIR)/$$(basename "$$source").$$stamp"; fi; \
		done; \
		echo "Legacy SQLite input archived under $(DATABASE_BACKUP_DIR)."; \
	fi; \
	rm -f "$(DATABASE_FILE)" "$(DATABASE_STEM).trace.db" "$(DATABASE_STEM).lock.db"; \
	echo "H2 will create a fresh schema on the next Zenbot startup."

db-check: stop
	@test -s "$(DATABASE_FILE)" || { echo "H2 database not found or empty: $(DATABASE_FILE)"; exit 1; }
	@$(DOCKER) image inspect "$(IMAGE_NAME)" >/dev/null 2>&1 || { echo "Docker image not found: $(IMAGE_NAME). Run make build first."; exit 1; }
	$(DOCKER) run --rm \
		--entrypoint java \
		-v "$(DATABASE_DIR):$(APP_DIR)/database" \
		"$(IMAGE_NAME)" \
		-cp /opt/h2/h2.jar org.h2.tools.Shell \
		-url "$(H2_CHECK_URL)" \
		-user sa \
		-password "" \
		-sql "SELECT H2VERSION(); SELECT COUNT(*) AS APPLICATION_TABLES FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='PUBLIC'"

backup-db: stop
	@test -s "$(DATABASE_FILE)" || { echo "H2 database not found or empty: $(DATABASE_FILE)"; exit 1; }
	@set -eu; \
	mkdir -p "$(DATABASE_BACKUP_DIR)"; \
	backup="$(DATABASE_BACKUP_DIR)/database-$$(date +%Y%m%d-%H%M%S).mv.db"; \
	cp "$(DATABASE_FILE)" "$$backup"; \
	test -s "$$backup"; \
	echo "Database backup created: $$backup"

logs:
	$(DOCKER) logs -f "$(CONTAINER_NAME)"

shell:
	$(DOCKER) exec -it "$(CONTAINER_NAME)" sh

ps:
	$(DOCKER) ps -a --filter "name=$(CONTAINER_NAME)"

status:
	$(DOCKER) inspect --format '{{.Name}} {{.State.Status}}' "$(CONTAINER_NAME)"
